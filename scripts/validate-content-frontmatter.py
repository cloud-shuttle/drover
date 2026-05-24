#!/usr/bin/env python3
"""Validate YAML frontmatter on markdown/mdx files against org taxonomy + JSON Schema."""

from __future__ import annotations

import argparse
import fnmatch
import json
import re
import subprocess
import sys
from pathlib import Path
from typing import Any

try:
    import jsonschema
    import yaml
except ImportError as exc:  # pragma: no cover
    print(
        "Missing dependency: install with `pip install pyyaml jsonschema`",
        file=sys.stderr,
    )
    raise SystemExit(2) from exc


FRONTMATTER_RE = re.compile(
    r"\A---(?:\r?\n)(.*?)(?:\r?\n)---(?:\r?\n|\Z)",
    re.DOTALL,
)


def repo_root(start: Path) -> Path:
    try:
        out = subprocess.check_output(
            ["git", "rev-parse", "--show-toplevel"],
            cwd=start,
            stderr=subprocess.DEVNULL,
            text=True,
        ).strip()
        return Path(out)
    except (subprocess.CalledProcessError, FileNotFoundError):
        return start.resolve()


def resolve_taxonomy_paths(root: Path) -> tuple[Path, Path]:
    candidates: list[tuple[Path, Path]] = [
        (root / "docs" / "taxonomy.yaml", root / "docs" / "reference" / "content-frontmatter-schema.json"),
        (root.parent / "docs" / "taxonomy.yaml", root.parent / "docs" / "reference" / "content-frontmatter-schema.json"),
        (root / "taxonomy" / "taxonomy.yaml", root / "taxonomy" / "content-frontmatter-schema.json"),
    ]
    for tax, schema in candidates:
        if tax.is_file() and schema.is_file():
            return tax, schema
    searched = "\n".join(f"  - {tax}\n  - {schema}" for tax, schema in candidates)
    raise FileNotFoundError(f"Could not find taxonomy.yaml and content-frontmatter-schema.json.\nSearched:\n{searched}")


def load_taxonomy(path: Path) -> dict[str, Any]:
    with path.open(encoding="utf-8") as fh:
        return yaml.safe_load(fh)


def load_schema(path: Path) -> dict[str, Any]:
    with path.open(encoding="utf-8") as fh:
        return json.load(fh)


def extract_frontmatter(text: str) -> tuple[dict[str, Any] | None, str | None]:
    match = FRONTMATTER_RE.match(text)
    if not match:
        return None, "missing opening --- frontmatter block"
    try:
        data = yaml.safe_load(match.group(1))
    except yaml.YAMLError as exc:
        return None, f"invalid YAML frontmatter: {exc}"
    if data is None:
        return {}, None
    if not isinstance(data, dict):
        return None, "frontmatter must be a YAML mapping"
    return data, None


def infer_doc_type(rel_path: str, taxonomy: dict[str, Any]) -> str | None:
    for rule in taxonomy.get("path_inference", []):
        pattern = rule.get("pattern")
        doc_type = rule.get("doc_type")
        if pattern and doc_type and fnmatch.fnmatch(rel_path, pattern):
            return doc_type
    return None


def normalize_synonyms(frontmatter: dict[str, Any], taxonomy: dict[str, Any]) -> list[str]:
    notes: list[str] = []
    synonyms = taxonomy.get("synonyms", {})

    for facet, mapping in synonyms.items():
        if not isinstance(mapping, dict):
            continue
        if facet == "topic":
            key = "topics"
            if key not in frontmatter or not isinstance(frontmatter[key], list):
                continue
            normalized: list[str] = []
            for value in frontmatter[key]:
                if not isinstance(value, str):
                    normalized.append(value)
                    continue
                preferred = mapping.get(value, value)
                if preferred != value:
                    notes.append(f"topics: normalized {value!r} -> {preferred!r}")
                normalized.append(preferred)
            frontmatter[key] = normalized
            continue

        if facet not in frontmatter:
            continue
        value = frontmatter[facet]
        if isinstance(value, str):
            preferred = mapping.get(value, value)
            if preferred != value:
                notes.append(f"{facet}: normalized {value!r} -> {preferred!r}")
                frontmatter[facet] = preferred
        elif isinstance(value, list):
            normalized_values: list[str] = []
            for item in value:
                if not isinstance(item, str):
                    normalized_values.append(item)  # type: ignore[arg-type]
                    continue
                preferred = mapping.get(item, item)
                if preferred != item:
                    notes.append(f"{facet}: normalized {item!r} -> {preferred!r}")
                normalized_values.append(preferred)
            frontmatter[facet] = normalized_values
    return notes


def changed_files(root: Path, base_ref: str) -> list[Path]:
    try:
        subprocess.check_call(["git", "rev-parse", base_ref], cwd=root, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        diff_cmd = ["git", "diff", "--name-only", "--diff-filter=ACMR", f"{base_ref}...HEAD"]
    except subprocess.CalledProcessError:
        diff_cmd = ["git", "diff", "--name-only", "--diff-filter=ACMR", "HEAD~1", "HEAD"]

    out = subprocess.check_output(diff_cmd, cwd=root, text=True)
    files = [root / line.strip() for line in out.splitlines() if line.strip()]
    return [path for path in files if path.suffix.lower() in {".md", ".mdx"}]


def matches_globs(rel_path: str, globs: list[str]) -> bool:
    return any(fnmatch.fnmatch(rel_path, pattern) for pattern in globs)


def validate_file(
    path: Path,
    root: Path,
    taxonomy: dict[str, Any],
    schema: dict[str, Any],
    *,
    require_frontmatter: bool,
    require_doc_type: bool,
) -> list[str]:
    errors: list[str] = []
    try:
        rel = path.relative_to(root).as_posix()
    except ValueError:
        rel = path.as_posix()

    if not path.is_file():
        return errors

    label = rel
    text = path.read_text(encoding="utf-8")
    frontmatter, parse_error = extract_frontmatter(text)

    if parse_error:
        if require_frontmatter:
            errors.append(f"{label}: {parse_error}")
        return errors

    if frontmatter is None:
        if require_frontmatter:
            errors.append(f"{label}: missing frontmatter (required for scoped docs)")
        return errors

    for note in normalize_synonyms(frontmatter, taxonomy):
        print(f"note: {label}: {note}")

    if "doc_type" not in frontmatter:
        inferred = infer_doc_type(rel, taxonomy)
        if inferred:
            frontmatter["doc_type"] = inferred
            print(f"note: {label}: inferred doc_type={inferred!r} from path")
        elif require_doc_type:
            errors.append(f"{label}: missing doc_type (not inferable from path)")

    validator = jsonschema.Draft202012Validator(schema)
    for error in sorted(validator.iter_errors(frontmatter), key=lambda e: e.path):
        loc = ".".join(str(part) for part in error.path)
        suffix = f" ({loc})" if loc else ""
        errors.append(f"{label}: schema: {error.message}{suffix}")

    facet_terms = {
        facet: set(section.get("terms", {}).keys())
        for facet, section in taxonomy.get("facets", {}).items()
        if isinstance(section, dict)
    }

    product = frontmatter.get("product")
    if isinstance(product, str) and product not in facet_terms.get("product", set()):
        errors.append(f"{label}: unknown product term {product!r}")

    audience = frontmatter.get("audience")
    audience_values: list[str]
    if isinstance(audience, str):
        audience_values = [audience]
    elif isinstance(audience, list):
        audience_values = [item for item in audience if isinstance(item, str)]
    else:
        audience_values = []
    allowed_audience = facet_terms.get("audience", set())
    for value in audience_values:
        if value not in allowed_audience:
            errors.append(f"{label}: unknown audience term {value!r}")

    doc_type = frontmatter.get("doc_type")
    if isinstance(doc_type, str) and doc_type not in facet_terms.get("doc_type", set()):
        errors.append(f"{label}: unknown doc_type term {doc_type!r}")

    topics = frontmatter.get("topics")
    if isinstance(topics, list):
        allowed_topics = facet_terms.get("topic", set())
        for topic in topics:
            if isinstance(topic, str) and topic not in allowed_topics:
                errors.append(f"{label}: unknown topic term {topic!r}")

    surface = frontmatter.get("surface")
    if isinstance(surface, str) and surface not in facet_terms.get("surface", set()):
        errors.append(f"{label}: unknown surface term {surface!r}")

    return errors


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "files",
        nargs="*",
        help="Explicit files to validate (default: changed .md/.mdx when --changed-only)",
    )
    parser.add_argument(
        "--root",
        type=Path,
        default=None,
        help="Repository root (default: git toplevel from cwd)",
    )
    parser.add_argument(
        "--taxonomy",
        type=Path,
        default=None,
        help="Path to taxonomy.yaml (default: auto-discover)",
    )
    parser.add_argument(
        "--schema",
        type=Path,
        default=None,
        help="Path to content-frontmatter-schema.json (default: auto-discover)",
    )
    parser.add_argument(
        "--changed-only",
        action="store_true",
        help="Validate markdown files changed vs --base-ref",
    )
    parser.add_argument(
        "--base-ref",
        default="origin/main",
        help="Git base ref for --changed-only (default: origin/main)",
    )
    parser.add_argument(
        "--scope",
        action="append",
        default=["docs/**"],
        help="Glob(s) for files that require frontmatter (repeatable, default: docs/**)",
    )
    parser.add_argument(
        "--exclude-scope",
        action="append",
        default=[],
        help="Glob(s) to skip entirely (repeatable, e.g. docs/changelogs/**)",
    )
    parser.add_argument(
        "--require-frontmatter",
        action="store_true",
        default=True,
        help="Require frontmatter on scoped files (default: true)",
    )
    parser.add_argument(
        "--no-require-frontmatter",
        action="store_false",
        dest="require_frontmatter",
        help="Only validate frontmatter when present",
    )
    parser.add_argument(
        "--require-doc-type",
        action="store_true",
        help="Fail when doc_type is missing and cannot be inferred from path",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    root = (args.root or repo_root(Path.cwd())).resolve()

    if args.taxonomy and args.schema:
        tax_path, schema_path = args.taxonomy.resolve(), args.schema.resolve()
    else:
        tax_path, schema_path = resolve_taxonomy_paths(root)

    taxonomy = load_taxonomy(tax_path)
    schema = load_schema(schema_path)

    if args.files:
        targets = [(root / path).resolve() if not Path(path).is_absolute() else Path(path).resolve() for path in args.files]
    elif args.changed_only:
        targets = changed_files(root, args.base_ref)
        if not targets:
            print("No changed .md/.mdx files.")
            return 0
    else:
        print("Provide file paths or use --changed-only", file=sys.stderr)
        return 2

    errors: list[str] = []
    checked = 0
    for path in targets:
        if path.suffix.lower() not in {".md", ".mdx"}:
            continue
        try:
            rel = path.relative_to(root).as_posix()
        except ValueError:
            rel = str(path)
        in_scope = matches_globs(rel, args.scope)
        if args.exclude_scope and matches_globs(rel, args.exclude_scope):
            continue

        text = path.read_text(encoding="utf-8")
        frontmatter, parse_error = extract_frontmatter(text)
        if not in_scope and (frontmatter is None or parse_error):
            continue

        checked += 1
        file_errors = validate_file(
            path,
            root,
            taxonomy,
            schema,
            require_frontmatter=args.require_frontmatter and in_scope,
            require_doc_type=args.require_doc_type,
        )
        errors.extend(file_errors)

    if checked == 0:
        print("No scoped markdown files to validate.")
        return 0

    if errors:
        print(f"Frontmatter validation failed ({len(errors)} error(s)):", file=sys.stderr)
        for error in errors:
            print(f"  - {error}", file=sys.stderr)
        return 1

    print(f"Validated {checked} markdown file(s) using {tax_path.name} + schema.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
