#!/bin/bash
set -e

# Shared helpers for task/epic loading scripts.
# Designed to be compatible with macOS default bash (3.2):
# - no associative arrays
# - no GNU grep (-P)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Path to drover binary. Can be overridden by env var.
DROVER="${DROVER:-$SCRIPT_DIR/drover}"

# Project directory to operate on. Can be overridden by env var.
PROJECT_DIR="${PROJECT_DIR:-$(pwd)}"

run_drover() {
  (cd "$PROJECT_DIR" && "$DROVER" "$@")
}

extract_id() {
  # Parses lines like:
  #   ✅ Created task task-123.1
  #   ✅ Created epic epic-abc: My Epic
  # Returns the first matching ID.
  #
  # Use sed -E (BSD/macOS compatible) instead of grep -P.
  sed -nE 's/^.*Created (task|epic) ([A-Za-z0-9._-]+).*$/\2/p' | head -n 1
}

set_epic_id() {
  # Store epic ID in a variable like EPIC_ID_E1 or EPIC_ID_worktree
  local key="$1"
  local id="$2"
  eval "EPIC_ID_${key}=\"${id}\""
}

get_epic_id() {
  local key="$1"
  eval "printf '%s' \"\${EPIC_ID_${key}:-}\""
}

create_epic() {
  local key="$1"
  local title="$2"
  local desc="$3"

  local output id
  output="$(run_drover epic add "$title" -d "$desc")"
  id="$(printf '%s\n' "$output" | extract_id)"
  if [ -z "$id" ]; then
    echo "ERROR: Could not parse epic id for key '$key' from output:" >&2
    echo "$output" >&2
    return 1
  fi
  set_epic_id "$key" "$id"
  echo "  $key: $id"
}

load_epics() {
  echo ""
  echo "=== Creating Epics ==="
  echo ""

  # Roborev-inspired epics
  create_epic "E1" "E1 - Event Streaming System" "Real-time JSONL event output for task lifecycle events."
  create_epic "E2" "E2 - Project-Level Configuration" "Support per-project .drover.toml configuration (guidelines, worker limits, agent preferences)."
  create_epic "E3" "E3 - Context Window Management" "Detect large content and substitute references with fetch instructions."
  create_epic "E4" "E4 - Structured Task Outcomes" "Extract Pass/Fail/Blocked verdicts and structured summaries from agent output."
  create_epic "E5" "E5 - Enhanced CLI Job Controls" "Cancel, retry, resolve and other operational controls."
  create_epic "E6" "E6 - Task Context Carrying" "Inject recent task context into prompts for better continuity."

  # Worktree / dashboard epics referenced by the loader scripts
  create_epic "worktree" "Worktree Pre-warming & Caching" "Pool warm worktrees and share dependency caches to reduce cold-start time."
  create_epic "observability" "Enhanced Observability Dashboard" "Add success metrics, worker utilization, and live activity feed."
  create_epic "subtasks" "Agent-Spawned Sub-Tasks" "Allow agents to spawn subtasks via tools and optionally await completion."
  create_epic "hitl" "Human-in-the-Loop Intervention" "Pause/resume tasks, inject guidance, and provide a worktree browser."
  create_epic "multiplayer" "Session Handoff & Multiplayer" "Export/import sessions and operator attribution."
  create_epic "cli" "CLI Ergonomics & Quick Capture" "Quick task capture, watch mode, prompt integration."
}

