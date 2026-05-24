#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if ! python3 -c "import jsonschema, yaml" 2>/dev/null; then
  python3 -m pip install --quiet pyyaml jsonschema
fi

exec python3 "$ROOT/scripts/validate-content-frontmatter.py" --root "${DROVER_REPO_ROOT:-$ROOT}" "$@"
