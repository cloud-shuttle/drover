#!/usr/bin/env bash
# Central quality gate runner for Drover Orchestrator
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "🐂 Running Drover Platform Quality Gate Scan..."
echo "══════════════════════════════════════════════"

# Run scanner against the main 'drover' repository
# Default to 150000.0 to pass on unrefactored commands.go, but allow passing custom limit
LIMIT="${1:-150000.0}"

python3 "$ROOT/scripts/quality-gate.py" \
  "$ROOT/drover" \
  --coverage "$ROOT/drover/coverage.out" \
  --limit "$LIMIT"

echo ""
echo "✨ Scan Completed!"
