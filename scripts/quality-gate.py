#!/usr/bin/env python3
import sys
import os
import re

# Premium styling terminal colors
BLUE = '\033[94m'
GREEN = '\033[92m'
YELLOW = '\033[93m'
RED = '\033[91m'
BOLD = '\033[1m'
NC = '\033[0m'

# Default CRAP threshold limit
DEFAULT_CRAP_LIMIT = 30.0

def calculate_crap(complexity, coverage):
    """
    CRAP = C^2 * (1 - Cov)^3 + C
    complexity: estimated cyclomatic complexity (int)
    coverage: coverage fraction from 0.0 to 1.0 (float)
    """
    return (complexity ** 2) * ((1.0 - coverage) ** 3) + complexity

def estimate_complexity(file_path):
    """
    Estimate cyclomatic complexity by counting control flow decision points.
    Decision points = 1 + counts of (if, for, case, &&, ||, select, catch, while)
    """
    if not os.path.exists(file_path):
        return 0
        
    complexity = 1
    
    # Regular expressions for control flow keywords
    # Matches Go and TS/JS decision structures
    patterns = [
        r'\bif\b',
        r'\bfor\b',
        r'\bwhile\b',
        r'\bcase\b',
        r'\bselect\b',
        r'\bcatch\b',
        r'&&',
        r'\|\|'
    ]
    
    combined_pattern = re.compile('|'.join(patterns))
    
    try:
        with open(file_path, 'r', encoding='utf-8', errors='ignore') as f:
            for line in f:
                # Strip comments to prevent false positives in text/docs
                line_stripped = strip_comments(line, file_path)
                matches = combined_pattern.findall(line_stripped)
                complexity += len(matches)
    except Exception as e:
        print(f"{RED}Error reading {file_path}: {e}{NC}")
        
    return complexity

def strip_comments(line, file_path):
    """
    Remove comments from line based on file type.
    """
    if file_path.endswith(('.go', '.ts', '.tsx', '.js', '.jsx')):
        # Double slash comments
        idx = line.find('//')
        if idx != -1:
            return line[:idx]
    return line

def parse_go_coverage(cover_file):
    """
    Parse Go coverage profile (coverage.out)
    Returns a dict mapping filenames to their coverage fraction [0.0 - 1.0].
    """
    cov = {}
    if not cover_file or not os.path.exists(cover_file):
        return cov
        
    try:
        with open(cover_file, 'r') as f:
            for line in f:
                # Format: github.com/owner/repo/path/file.go:line.col,line.col statements count
                match = re.match(r'^([^:]+):(\d+)\.\d+,(\d+)\.\d+\s+(\d+)\s+(\d+)', line)
                if match:
                    filepath, _, _, statements, count = match.groups()
                    statements = int(statements)
                    count = int(count)
                    
                    # Normalize local filename path (strip import path prefix)
                    # e.g., github.com/cloud-shuttle/drover/pkg/db/db.go -> pkg/db/db.go
                    # We will match by basename/suffix later if needed
                    filename = filepath.split('/')[-1]
                    
                    if filepath not in cov:
                        cov[filepath] = {"covered": 0, "total": 0}
                    cov[filepath]["total"] += statements
                    if count > 0:
                        cov[filepath]["covered"] += statements
    except Exception as e:
        print(f"{YELLOW}Warning: Error parsing coverage file {cover_file}: {e}{NC}")
        
    # Convert statement counts to fractions
    result = {}
    for path, stats in cov.items():
        if stats["total"] > 0:
            result[path] = stats["covered"] / stats["total"]
        else:
            result[path] = 1.0
    return result

def find_matching_coverage(local_file, coverage_dict):
    """
    Match a local relative file path to the keys in the coverage dict.
    Go coverage paths use import names, e.g., github.com/cloud-shuttle/drover/pkg/db/db.go
    """
    for cov_path, fraction in coverage_dict.items():
        if cov_path.endswith(local_file):
            return fraction
    return 0.0 # Default to 0% coverage if not found in coverage profile

def audit_directory(root_dir, coverage_file=None, crap_limit=DEFAULT_CRAP_LIMIT, file_extensions=('.go', '.ts', '.tsx')):
    """
    Audit all code files in the directory.
    """
    print(f"\n{BLUE}Scanning directory: {BOLD}{root_dir}{NC}")
    if coverage_file:
        print(f"{BLUE}Using coverage profile: {BOLD}{coverage_file}{NC}")
    else:
        print(f"{YELLOW}No coverage profile provided. Assuming 0% coverage for CRAP scoring.{NC}")
        
    coverage_dict = parse_go_coverage(coverage_file)
    
    files_audited = 0
    violations = []
    audit_results = []
    
    for dirpath, _, filenames in os.walk(root_dir):
        # Exclude directories
        if any(part in dirpath for part in ['.git', 'node_modules', 'vendor', '.tmp', 'dist', 'testdata']):
            continue
            
        for filename in filenames:
            if filename.endswith(file_extensions) and not filename.endswith('_test.go'):
                full_path = os.path.join(dirpath, filename)
                rel_path = os.path.relpath(full_path, root_dir)
                
                # Estimate complexity
                complexity = estimate_complexity(full_path)
                
                # Retrieve coverage fraction
                cov_fraction = find_matching_coverage(rel_path, coverage_dict)
                
                # Compute CRAP score
                crap_score = calculate_crap(complexity, cov_fraction)
                
                files_audited += 1
                
                result = {
                    "file": rel_path,
                    "complexity": complexity,
                    "coverage": cov_fraction * 100.0,
                    "crap": crap_score
                }
                audit_results.append(result)
                
                if crap_score > crap_limit:
                    violations.append(result)
                    
    # Sort results by CRAP score descending
    audit_results.sort(key=lambda x: x["crap"], reverse=True)
    
    # Print Premium Visual Report
    print(f"\n{BOLD}Audited {files_audited} files:{NC}")
    print(f"┌{'─'*60}┬{'─'*12}┬{'─'*10}┬{'─'*12}┬{'─'*10}┐")
    print(f"│ {'File Path':<58} │ {'Complexity':^10} │ {'Coverage':^8} │ {'CRAP Score':^10} │ {'Status':^8} │")
    print(f"├{'─'*60}┼{'─'*12}┼{'─'*10}┼{'─'*12}┼{'─'*10}┤")
    
    # Show top 15 highest risk files or those with violations
    show_limit = max(15, len(violations))
    for r in audit_results[:show_limit]:
        status_str = f"{RED}{BOLD}FAIL{NC}" if r["crap"] > crap_limit else f"{GREEN}PASS{NC}"
        file_path_short = r["file"]
        if len(file_path_short) > 56:
            file_path_short = "..." + file_path_short[-53:]
            
        print(f"│ {file_path_short:<58} │ {r['complexity']:^10d} │ {r['coverage']:^6.1f}% │ {r['crap']:^10.2f} │ {status_str:^17} │")
        
    print(f"└{'─'*60}┴{'─'*12}┴{'─'*10}┴{'─'*12}┴{'─'*10}┘")
    
    # Print Summary statistics
    print(f"\n{BOLD}Summary Statistics:{NC}")
    print(f"  • Total files audited: {files_audited}")
    print(f"  • CRAP Limit allowed: {crap_limit:.2f}")
    if len(violations) > 0:
        print(f"  • Violations detected: {RED}{BOLD}{len(violations)}{NC}")
        for v in violations:
            print(f"    - {RED}{v['file']}{NC} (CRAP: {BOLD}{v['crap']:.2f}{NC}, Complexity: {v['complexity']}, Coverage: {v['coverage']:.1f}%)")
    else:
        print(f"  • Violations detected: {GREEN}{BOLD}None{NC} ✨")
        
    return len(violations)

def main():
    print(f"\n{BLUE}{BOLD}🐂 Drover Platform CI Quality Gate — CRAP Scoring Engine{NC}")
    print("═" * 60)
    
    # Argument parser
    import argparse
    parser = argparse.ArgumentParser(description="Audit Drover files for Cyclomatic Complexity and CRAP scores.")
    parser.add_argument("directory", help="The target repository or package directory to scan.")
    parser.add_argument("--coverage", help="Path to Go coverage.out profile.")
    parser.add_argument("--limit", type=float, default=DEFAULT_CRAP_LIMIT, help=f"CRAP score threshold limit (default: {DEFAULT_CRAP_LIMIT})")
    
    args = parser.parse_args()
    
    if not os.path.exists(args.directory):
        print(f"{RED}Error: Target directory '{args.directory}' does not exist.{NC}")
        sys.exit(2)
        
    violations_count = audit_directory(args.directory, args.coverage, args.limit)
    
    if violations_count > 0:
        print(f"\n{RED}{BOLD}❌ Quality Gate Failed: CRAP violations detected!{NC}")
        print("Please refactor complex functions or increase test coverage to pass the gate.")
        sys.exit(1)
        
    print(f"\n{GREEN}{BOLD}✅ Quality Gate Passed successfully! All files within specifications.{NC}")
    sys.exit(0)

if __name__ == '__main__':
    main()
