#!/bin/bash
# load-json.sh - Import JSON-formatted epics, stories, and tasks into Drover
#
# Usage:
#   ./load-json.sh <file.jsonl>
#
# JSON Format (JSONL - one JSON object per line):
#   {"id": "EPIC-001", "type": "epic", "title": "...", "description": "..."}
#   {"id": "STORY-001", "type": "story", "epic_id": "EPIC-001", "title": "...", "description": "...", "priority": 10}
#   {"id": "TASK-001", "type": "task", "story_id": "STORY-001", "title": "...", "description": "...", "priority": 5}
#
# Note: Priority is an integer (higher = more urgent)
#       Labels are not supported (Drover doesn't have this feature yet)
#
# Requirements:
#   - jq (for JSON parsing)

# Don't exit on error - handle failures per-item
# set -e

DROVER="./drover"

# Associative arrays to map external IDs to Drover IDs
declare -A EPIC_IDS
declare -A STORY_IDS

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Map priority strings to integers
map_priority() {
    case $1 in
        critical|urgent) echo 10 ;;
        high) echo 7 ;;
        normal|medium) echo 5 ;;
        low) echo 2 ;;
        *) echo "$1" ;;
    esac
}

# Check if jq is installed
if ! command -v jq &> /dev/null; then
    echo -e "${RED}Error: jq is required but not installed${NC}"
    echo "Install with: apt-get install jq  # Debian/Ubuntu"
    echo "            brew install jq        # macOS"
    exit 1
fi

# Check if file argument provided
if [ -z "$1" ]; then
    echo -e "${RED}Error: No input file specified${NC}"
    echo "Usage: $0 <file.jsonl>"
    exit 1
fi

INPUT_FILE="$1"

# Check if file exists
if [ ! -f "$INPUT_FILE" ]; then
    echo -e "${RED}Error: File not found: $INPUT_FILE${NC}"
    exit 1
fi

echo "=== Drover JSON Import ==="
echo "File: $INPUT_FILE"
echo ""

# Counter for stats
epic_count=0
story_count=0
task_count=0

# Helper function to extract ID from drover output
extract_id() {
    grep -oP '(?<=Created task )[a-z0-9-]+|(?<=Created epic )[a-z0-9-]+|(?<=Created )[a-z0-9-]+\-[0-9]+' || echo ""
}

# Read JSONL file line by line
while IFS= read -r line || [ -n "$line" ]; do
    # Skip empty lines and comments
    [[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue

    # Parse JSON fields
    type=$(echo "$line" | jq -r '.type // empty' 2>/dev/null)
    id=$(echo "$line" | jq -r '.id // ""' 2>/dev/null)
    title=$(echo "$line" | jq -r '.title // ""' 2>/dev/null)
    desc=$(echo "$line" | jq -r '.description // ""' 2>/dev/null)
    priority=$(echo "$line" | jq -r '.priority // "normal"' 2>/dev/null)
    epic_id_ref=$(echo "$line" | jq -r '.epic_id // ""' 2>/dev/null)
    story_id_ref=$(echo "$line" | jq -r '.story_id // ""' 2>/dev/null)
    story_points=$(echo "$line" | jq -r '.story_points // ""' 2>/dev/null)
    estimated_hours=$(echo "$line" | jq -r '.estimated_hours // ""' 2>/dev/null)

    # Skip if type is missing
    if [[ -z "$type" ]]; then
        echo -e "${YELLOW}Warning: Skipping invalid line (missing 'type')${NC}"
        continue
    fi

    case $type in
        epic)
            echo -e "${GREEN}[EPIC]${NC} $id: $title"

            # Build epic command (epic add only supports -d for description)
            cmd="$DROVER epic add \"$title\" -d \"$desc\""

            output=$(eval "$cmd" 2>&1)
            drover_id=$(extract_id <<< "$output")

            if [[ -n "$drover_id" ]]; then
                EPIC_IDS[$id]=$drover_id
                echo "       -> $drover_id"
                ((epic_count++))
            else
                echo -e "       ${RED}Failed to create epic${NC}"
                echo "       Output: $output"
            fi
            ;;

        story)
            # Resolve epic ID reference
            drover_epic_id="${EPIC_IDS[$epic_id_ref]}"

            if [[ -z "$drover_epic_id" ]]; then
                echo -e "${YELLOW}Warning: Skipping story $id - epic '$epic_id_ref' not found${NC}"
                continue
            fi

            echo -e "${GREEN}[STORY]${NC} $id: $title (epic: $epic_id_ref)"

            # Map priority string to integer
            priority_int=$(map_priority "$priority")

            # Build task command for story
            cmd="$DROVER add \"$title\" --epic $drover_epic_id -d \"$desc\" -p $priority_int --skip-validation"

            output=$(eval "$cmd" 2>&1)
            drover_id=$(extract_id <<< "$output")

            if [[ -n "$drover_id" ]]; then
                STORY_IDS[$id]=$drover_id
                echo "       -> $drover_id"
                ((story_count++))
            else
                echo -e "       ${RED}Failed to create story${NC}"
                echo "       Output: $output"
            fi
            ;;

        task)
            # Resolve story ID reference (story is a parent task)
            drover_parent_id="${STORY_IDS[$story_id_ref]}"

            if [[ -z "$drover_parent_id" ]]; then
                echo -e "${YELLOW}Warning: Skipping task $id - story '$story_id_ref' not found${NC}"
                continue
            fi

            echo -e "${GREEN}[TASK]${NC} $id: $title (story: $story_id_ref)"

            # Map priority string to integer
            priority_int=$(map_priority "$priority")

            # Build task command (use --parent for story reference)
            cmd="$DROVER add \"$title\" --parent $drover_parent_id -d \"$desc\" -p $priority_int --skip-validation"

            output=$(eval "$cmd" 2>&1)
            drover_id=$(extract_id <<< "$output")

            if [[ -n "$drover_id" ]]; then
                echo "       -> $drover_id"
                ((task_count++))
            else
                echo -e "       ${RED}Failed to create task${NC}"
                echo "       Output: $output"
            fi
            ;;

        *)
            echo -e "${YELLOW}Warning: Skipping unknown type '$type'${NC}"
            ;;
    esac

done < "$INPUT_FILE"

echo ""
echo "=== Import Complete ==="
echo "Epics:  $epic_count"
echo "Stories: $story_count"
echo "Tasks:  $task_count"
echo ""
echo "Run './drover status' to see all tasks"
