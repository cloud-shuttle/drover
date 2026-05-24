#!/usr/bin/env bash
# Central quality gate runner for Drover Orchestrator
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "🐂 Running Drover Platform Quality Gate Scan..."
echo "══════════════════════════════════════════════"

# Run scanner against the repository source tree.
# NOTE: "$ROOT/drover" was previously passed here, which resolved to the compiled
# binary rather than a source directory, causing the scanner to audit 0 files.
# Fixed to "$ROOT" so all Go source under cmd/, internal/, and pkg/ is scanned.
#
# Phase-1 CI limit: 10000 (blocks only the single highest-CRAP outlier, db.go).
# Lower this over time as coverage improves: 5000 → 1000 → 30 (the true target).
LIMIT="${1:-10000}"

python3 "$ROOT/scripts/quality-gate.py" \
  "$ROOT" \
  --coverage "$ROOT/coverage.out" \
  --limit "$LIMIT"

echo ""
echo "✨ Scan Completed!"
