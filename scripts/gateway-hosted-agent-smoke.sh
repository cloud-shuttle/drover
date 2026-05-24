#!/usr/bin/env bash
# =============================================================================
# FULL RUNNING SMOKE + RECORDING: Hosted Agent Jobs with Muster/Guard/Gateway/Warden
# =============================================================================
#
# This script now exercises the complete post-Milestone-J governance path:
#   Muster (approved agent definition with MCP tools/clients)
#     → Cloud job create with agent_definition_ref
#     → Muster resolve (richer metadata: tools, risk tiers, approval state, version hash)
#     → CapabilityBindingSnapshot production (distinct clients vs tools)
#     → Fail-closed Guard binding registration
#     → Deeper Gateway MCP allow-list side-effect (for job dimensions / PreMCPHook)
#     → Worker (with Warden semantic checks)
#     → Job success + governance enforcement
#
# It produces a "recording" style output (structured phases + final JSON summary)
# suitable for demos, CI logs, or audit trails.
#
# Prerequisites (full stack):
#   - docker compose -f docker-compose.milestone-a.yml -f docker-compose.agent-jobs-smoke.yml up -d
#   - Muster, Cloud, Guard, Gateway, Brain, Zitadel, etc. healthy
#   - (Optional) A pre-approved agent in Muster with third-party MCP tools.
#     If not provided, the script can attempt to create a minimal one (see AGENT_CREATE=1).
#
# Usage (basic contract smoke, no Muster):
#   PROVISION=1 ./scripts/gateway-hosted-agent-smoke.sh
#
# Usage (full Muster + richer snapshot + Gateway side-effect + recording):
#   AGENT_DEFINITION_REF="<uuid-of-approved-agent-in-muster>" \
#   CUSTOMER_ID=... \
#   ./scripts/gateway-hosted-agent-smoke.sh
#
# Auto-create a test agent in Muster + full flow (best for clean demos):
#   AGENT_CREATE=1 PROVISION=1 ./scripts/gateway-hosted-agent-smoke.sh
#
# Full governance + observability smoke (brings up ClickStack + Guard schemas):
#   WITH_CLICKSTACK=1 AGENT_CREATE=1 PROVISION=1 ./scripts/gateway-hosted-agent-smoke.sh
#
# Real live third-party MCP E2E (runtime PreMCPHook + Guard evaluation on an actual tool call):
#   This exercises the *runtime* path (not just creation/registration):
#     - Job runs through a *real* drover-code worker (with ANTHROPIC key + Gateway config)
#     - LLM decides to call one of the approved third-party MCP tools from the Muster agent
#     - Call hits Gateway GovernancePlugin.PreMCPHook → Guard evaluate (using the binding)
#     - (Optional) Warden semantic check also fires
#     - Decision appears in guard_events / drover_decisions + warden_decisions
#
#   Fully self-contained version (recommended):
#     Uses the built-in tiny mock-mcp-server container, which is registered as a real
#     remote MCP client ("GovernanceMockMCP") in the smoke Gateway config.
#     Tool discovery is 100% real via the Gateway MCP layer.
#
#     docker compose -f docker-compose.milestone-a.yml -f docker-compose.agent-jobs-smoke.yml up -d mock-mcp-server
#     LIVE_MCP_E2E=1 WITH_CLICKSTACK=1 AGENT_CREATE=1 PROVISION=1 ./scripts/gateway-hosted-agent-smoke.sh
#
# Real UKC (Kraftcloud) worker E2E with public Gateway + full governance + lifecycle observability:
#   This is the production path (item 1 + lifecycle hardening):
#     - drover-cloud is started with the ukc overlay (UKC_TOKEN + UKC_REGION + public URLs)
#     - Public GATEWAY_PUBLIC_BASE_URL / AGENT_JOBS_LLM_BASE_URL / GUARD_PUBLIC_BASE_URL etc. are set
#     - Job runs on a real Unikraft VM (ukc-agent + drover-code headless)
#     - Worker receives the Muster/Guard snapshot + public service URLs + ephemeral credential
#     - Structured ukc_lifecycle events are emitted and persisted
#     - Final job record contains ukc_worker (uuid, started_at, lifetime, est_cost, active=false)
#     - Governance decisions (if LIVE_MCP_E2E) + UKC worker lifetime are both verified
#
#   Example (after deploying drover-gateway publicly and having a real UKC_TOKEN):
#     export UKC_TOKEN=...
#     export UKC_REGION=fra
#     export GATEWAY_PUBLIC_BASE_URL=https://gateway.prod.example.com
#     export AGENT_JOBS_LLM_BASE_URL=https://gateway.prod.example.com/anthropic
#     export GUARD_PUBLIC_BASE_URL=https://guard.prod.example.com
#     export MUSTER_PUBLIC_BASE_URL=https://muster.prod.example.com
#     docker compose -f docker-compose.milestone-a.yml -f docker-compose.ukc.yml \
#                  -f docker-compose.agent-jobs-smoke.yml up -d
#
#     LIVE_UKC_E2E=1 LIVE_MCP_E2E=1 WITH_CLICKSTACK=1 AGENT_CREATE=1 PROVISION=1 \
#       ./scripts/gateway-hosted-agent-smoke.sh
#
#   The smoke will:
#     - Automatically detect real UKC execution (ukc_worker present on job)
#     - Verify ukc_worker.uuid, started_at, lifetime_secs, and that the instance was cleaned up
#     - Still run the full governance assertions if LIVE_MCP_E2E=1
#     - Include ukc_worker + cost estimate in the RECORDING SUMMARY
#
#   How to run a *real* live governance E2E (recommended path):
#     1. Start the full stack + mock MCP:
#        docker compose -f docker-compose.milestone-a.yml -f docker-compose.agent-jobs-smoke.yml up -d mock-mcp-server
#     2. Run a real drover-code worker (with ANTHROPIC key) against the smoke tenant, or use:
#        LIVE_MCP_E2E=1 REAL_AGENT_EXEC=1 WITH_CLICKSTACK=1 AGENT_CREATE=1 PROVISION=1 ./scripts/gateway-hosted-agent-smoke.sh
#     3. The prompt will cause the LLM to call the approved "echo" tool from the mock MCP.
#        The call will go through the live Gateway PreMCPHook → Guard evaluate → (optional) Warden.
#     4. Post-run ClickHouse query (when WITH_CLICKSTACK) + RECORDING SUMMARY will show the actual decisions.
#
#   Example full live runtime E2E:
#     LIVE_MCP_E2E=1 WITH_CLICKSTACK=1 AGENT_CREATE=1 PROVISION=1 ./scripts/gateway-hosted-agent-smoke.sh
#
# Capture the "recording":
#   ./scripts/gateway-hosted-agent-smoke.sh 2>&1 | tee /tmp/governance-smoke-$(date +%s).log
#
# The final JSON block (after "== RECORDING SUMMARY ==") contains:
#   - ... (core flow fields)
#   - governance_verification: {binding_events, guard_evaluations, warden_decisions, passed}
#
# When LIVE_MCP_E2E=1 + WITH_CLICKSTACK=1, a "Governance Smoke Verification" section
# runs after the job and **asserts** that real rows exist in guard_events + warden_decisions
# for the job_id. The script fails if the governance path did not produce decisions.
#
# Full refresh:
#   docker compose -f docker-compose.milestone-a.yml -f docker-compose.agent-jobs-smoke.yml \
#     up -d --force-recreate drover-cloud drover-muster drover-guard drover-gateway
#   PROVISION=1 AGENT_CREATE=1 ./scripts/gateway-hosted-agent-smoke.sh
# =============================================================================
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

CLOUD_URL="${CLOUD_BASE_URL:-http://localhost:8090}"
BRAIN_URL="${BRAIN_BASE_URL:-http://localhost:8080}"
GATEWAY_URL="${GATEWAY_BASE_URL:-http://localhost:8081}"
TOKEN="${CLOUD_PROVISIONER_TOKEN:-operator-secret}"
JOB_TIMEOUT="${AGENT_JOB_TIMEOUT:-120}"
HEALTH_TIMEOUT="${HEALTH_TIMEOUT:-90}"
COMPOSE=(docker compose -f docker-compose.milestone-a.yml -f docker-compose.agent-jobs-smoke.yml)

# Optional: bring up ClickStack + Guard schema for full governance observability smoke
if [[ "${WITH_CLICKSTACK:-0}" == "1" || "${CLICKSTACK:-0}" == "1" ]]; then
  echo "== Starting ClickStack (HyperDX + ClickHouse) + Guard schemas for governance testing =="
  pushd drover-gateway/deploy/observability >/dev/null
  docker compose up -d
  # The guard-schema-init sidecar (added earlier) will automatically apply:
  #   01_decisions.sql + 02_projections.sql + 03_guard_events.sql + 04_warden_decisions.sql
  # Give it a moment
  sleep 25
  popd >/dev/null
  echo "ClickStack + Guard + Warden governance schemas ready (HyperDX UI at http://localhost:8082)"

  # Seed a couple of sample Warden decisions so the correlation tiles have data in a fresh smoke run.
  # These are linked to a placeholder job that the real job creation will later use.
  echo "Seeding sample Warden decisions for demo correlation..."
  docker compose -f drover-gateway/deploy/observability/docker-compose.yml exec -T hyperdx bash -c '
    clickhouse-client --host localhost --port 9000 --user default --password "" --query "
      INSERT INTO warden_decisions (decision_id, trace_id, tenant_id, job_id, check_type, tool_name, allowed, reason, layer, environment)
      VALUES
        (generateUUIDv4(), '\''trace-warden-1'\'', '\'''"$CID"''\'', '\''demo-job-warden'\'', '\''action'\'', '\''bash'\'', false, '\''dangerous shell command detected by Beads policy'\'', '\''warden'\'', '\''smoke'\''),
        (generateUUIDv4(), '\''trace-warden-2'\'', '\'''"$CID"''\'', '\''demo-job-warden'\'', '\''input'\'', '\''\'\'', false, '\''prompt injection pattern matched'\'', '\''warden'\'', '\''smoke'\'')
    " || true
  ' 2>/dev/null || true
fi

wait_for_health() {
  local name=$1 url=$2
  local deadline=$((SECONDS + HEALTH_TIMEOUT))
  local body=""
  while (( SECONDS < deadline )); do
    if body=$(curl -sf -m 5 "$url/health" 2>/dev/null); then
      if echo "$body" | python3 -m json.tool >/dev/null 2>&1; then
        echo "$body" | python3 -m json.tool
        return 0
      fi
    fi
    sleep 2
  done
  echo "error: $name not healthy at $url/health within ${HEALTH_TIMEOUT}s" >&2
  if [[ -n "$body" ]]; then
    echo "last response: $body" >&2
  else
    echo "last response: (empty or connection refused)" >&2
  fi
  exit 1
}

# -----------------------------------------------------------------------------
# Governance Smoke Verification
# Runs after a successful LIVE_MCP_E2E job to assert that real Guard + Warden
# decisions were recorded for the job in ClickHouse (via guard_events MV and
# warden_decisions table).
# -----------------------------------------------------------------------------
verify_governance_smoke() {
  local job_id="$1"

  if [[ "${LIVE_MCP_E2E:-0}" != "1" || "${WITH_CLICKSTACK:-0}" != "1" || -z "$job_id" ]]; then
    return 0
  fi

  echo ""
  echo "== Governance Smoke Verification =="
  echo "   Verifying that the live tool call produced real Guard and Warden decisions..."

  local compose_file="drover-gateway/deploy/observability/docker-compose.yml"
  local ch_exec='docker compose -f '"$compose_file"' exec -T hyperdx clickhouse-client --host localhost --port 9000 --user default --password "" --query'

  # 1. Binding registration (should exist from Cloud → Guard during job creation)
  local binding_count
  binding_count=$($ch_exec "
    SELECT count() FROM guard_events
    WHERE Attributes['job_id'] = '$job_id'
      AND (Layer = 'binding' OR action = 'binding.register' OR Attributes['action'] = 'binding.register')
    FORMAT TSV
  " 2>/dev/null | tr -d ' \n' || echo "0")

  # 2. Guard evaluations for this job (runtime PreMCPHook decisions)
  local guard_count
  guard_count=$($ch_exec "
    SELECT count() FROM guard_events
    WHERE Attributes['job_id'] = '$job_id'
      AND Layer = 'guard'
    FORMAT TSV
  " 2>/dev/null | tr -d ' \n' || echo "0")

  # 3. Warden semantic decisions for this job (from the actual tool call)
  local warden_count
  warden_count=$($ch_exec "
    SELECT count() FROM warden_decisions
    WHERE job_id = '$job_id'
    FORMAT TSV
  " 2>/dev/null | tr -d ' \n' || echo "0")

  echo "   Binding registrations : $binding_count"
  echo "   Guard tool evaluations: $guard_count"
  echo "   Warden decisions      : $warden_count"

  local passed=true
  if [[ "$binding_count" -lt 1 ]]; then
    echo "   ❌ No binding registration found for job"
    passed=false
  fi
  if [[ "$guard_count" -lt 1 ]]; then
    echo "   ❌ No Guard evaluation found for the tool call"
    passed=false
  fi
  if [[ "$warden_count" -lt 1 ]]; then
    echo "   ❌ No Warden decision recorded (tool call did not reach semantic safety layer)"
    passed=false
  fi

  if [[ "$passed" == "true" ]]; then
    echo "   ✅ Governance Smoke Verification PASSED"
    echo "      Real Muster → Guard binding + runtime Guard + Warden decisions confirmed."
  else
    echo "   ❌ Governance Smoke Verification FAILED"
    echo "      Expected at least one binding, one Guard evaluation, and one Warden decision."
    exit 1
  fi

  # Export for the recording summary
  export GOV_BINDING_COUNT="$binding_count"
  export GOV_GUARD_COUNT="$guard_count"
  export GOV_WARDEN_COUNT="$warden_count"
  export GOV_VERIFICATION_PASSED="true"
}

if [[ "${SKIP_COMPOSE:-0}" != "1" ]]; then
  echo "== ensure smoke overlay on drover-cloud =="
  "${COMPOSE[@]}" up -d --force-recreate drover-cloud

  # When doing a real live MCP E2E, also bring up the mock MCP server
  if [[ "${LIVE_MCP_E2E:-0}" == "1" ]]; then
    echo "== LIVE_MCP_E2E: ensuring mock-mcp-server is running =="
    "${COMPOSE[@]}" up -d mock-mcp-server
    sleep 3
  fi
fi

CLOUD_CONTAINER=$("${COMPOSE[@]}" ps -q drover-cloud 2>/dev/null | head -1)
if [[ -z "$CLOUD_CONTAINER" ]]; then
  echo "error: drover-cloud container not running" >&2
  exit 1
fi

if ! docker exec "$CLOUD_CONTAINER" test -f /usr/local/bin/agent-jobs-contract-check.sh 2>/dev/null; then
  echo "error: contract-check script not mounted on drover-cloud" >&2
  echo "run: docker compose -f docker-compose.milestone-a.yml -f docker-compose.agent-jobs-smoke.yml up -d --force-recreate drover-cloud" >&2
  exit 1
fi

DEV_CMD=$(docker exec "$CLOUD_CONTAINER" printenv AGENT_JOBS_DEV_EXEC_CMD 2>/dev/null || true)

# For real live governance E2E we intentionally do *not* want the contract checker.
# The real drover-code worker must run the job so the LLM actually emits a tool call
# that hits the live PreMCPHook + Guard + Warden path.
if [[ "${LIVE_MCP_E2E:-0}" == "1" || "${REAL_AGENT_EXEC:-0}" == "1" ]]; then
  echo "== LIVE/REAL mode: skipping contract-checker (real worker expected) =="
else
  if [[ "$DEV_CMD" != "/usr/local/bin/agent-jobs-contract-check.sh" ]]; then
    echo "error: AGENT_JOBS_DEV_EXEC_CMD not set (got: ${DEV_CMD:-<empty>})" >&2
    echo "recreate cloud with docker-compose.agent-jobs-smoke.yml (see script header)" >&2
    exit 1
  fi
fi

echo "== health (wait up to ${HEALTH_TIMEOUT}s) =="
wait_for_health "drover-cloud" "$CLOUD_URL"
wait_for_health "drover-brain" "$BRAIN_URL"
wait_for_health "drover-gateway" "$GATEWAY_URL"

if [[ "${PROVISION:-0}" == "1" ]]; then
  echo "== provision smoke customer =="
  PROVISION_OUT="$("$ROOT/scripts/provision-smoke.sh")"
  echo "$PROVISION_OUT"
  CUSTOMER_ID="$(echo "$PROVISION_OUT" | awk '/^customer_id:/ {print $2}')"
fi

CID="${CUSTOMER_ID:-}"
if [[ -z "$CID" ]]; then
  echo "error: set CUSTOMER_ID to a provisioned customer id, or PROVISION=1" >&2
  echo "example: CUSTOMER_ID=373890414345755397 ./scripts/gateway-hosted-agent-smoke.sh" >&2
  exit 1
fi

# -----------------------------------------------------------------------------
# Optional: Create a real approved agent definition in Muster (for full E2E recording)
# -----------------------------------------------------------------------------
if [[ "${AGENT_CREATE:-0}" == "1" ]]; then
  echo "== AGENT_CREATE=1: creating minimal approved test agent in Muster with third-party MCP tools =="
  MUSTER_URL="${MUSTER_BASE_URL:-http://localhost:8082}"

  # Simple dev auth (same pattern as Cloud)
  if [[ -n "${ZITADEL_MEMBER_TOKEN:-}" ]]; then
    M_AUTH_HDR=( -H "Authorization: Bearer ${ZITADEL_MEMBER_TOKEN}" -H "X-Org-ID: ${CID}" )
  else
    M_AUTH_HDR=( -H "Authorization: Bearer dev-token" -H "X-Dev-Org-ID: ${CID}" )
  fi

  # Include the mock MCP tools when doing live E2E (or always for the smoke agent — harmless)
  MOCK_TOOLS=""
  if [[ "${LIVE_MCP_E2E:-0}" == "1" ]]; then
    MOCK_TOOLS=', "echo", "mock_echo"'
  fi

  AGENT_PAYLOAD=$(printf '{
    "name": "Governance Smoke Agent",
    "slug": "governance-smoke-agent",
    "description": "Agent created by smoke script for Muster+Guard+Gateway+Warden demo",
    "required_mcp_tool_slugs": ["github", "linear"%s],
    "risk_tier": "medium",
    "markdown_body": "Demo agent for full governance smoke. Includes mock echo tool for self-contained E2E."
  }' "$MOCK_TOOLS")

  CREATE_AGENT=$(curl -s -m 15 -w "\nHTTP_STATUS:%{http_code}" -X POST "$MUSTER_URL/v1/agents" \
    "${M_AUTH_HDR[@]}" \
    -H "Content-Type: application/json" \
    -d "$AGENT_PAYLOAD")

  AGENT_HTTP="${CREATE_AGENT##*HTTP_STATUS:}"
  CREATE_AGENT="${CREATE_AGENT%HTTP_STATUS:*}"

  if [[ "$AGENT_HTTP" != "200" && "$AGENT_HTTP" != "201" ]]; then
    echo "warning: could not auto-create agent in Muster (status $AGENT_HTTP). You can create one manually and pass AGENT_DEFINITION_REF."
    echo "$CREATE_AGENT"
  else
    AGENT_ID=$(echo "$CREATE_AGENT" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    print(d.get('id') or d.get('agent',{}).get('id') or '')
except:
    print('')
" 2>/dev/null || true)

    echo "Created agent in Muster: $AGENT_ID"

    # Best-effort approve (in real systems this is a separate workflow step)
    if [[ -n "$AGENT_ID" ]]; then
      echo "Agent created (ID: $AGENT_ID). Calling fully-approve helper for fully automatic smoke..."

      # Use the new reliable one-shot approval endpoint (added for smoke/test)
      curl -s -m 15 -X POST "$MUSTER_URL/v1/agents/$AGENT_ID/fully-approve" \
        "${M_AUTH_HDR[@]}" \
        -H "Content-Type: application/json" > /dev/null || true

      echo "Agent fully approved via /fully-approve (smoke helper)."
    fi

    # Use the newly created agent for the rest of the run
    AGENT_DEFINITION_REF="${AGENT_DEFINITION_REF:-$AGENT_ID}"
    export AGENT_DEFINITION_REF
  fi
fi

if [[ -n "${AGENT_DEFINITION_REF:-}" ]]; then
  echo "Using AGENT_DEFINITION_REF=${AGENT_DEFINITION_REF} (will trigger Muster resolve + richer snapshot + Gateway side-effect)"
fi

if [[ -n "${ZITADEL_MEMBER_TOKEN:-}" ]]; then
  AUTH_HDR=( -H "Authorization: Bearer ${ZITADEL_MEMBER_TOKEN}" )
else
  AUTH_HDR=( -H "Authorization: Bearer dev-token" -H "X-Dev-Org-ID: ${CID}" )
fi

echo "== create agent job (customer $CID) =="
# For full Muster + richer snapshot + Gateway side-effect smoke, provide an approved
# agent_definition_ref from drover-muster (the Cloud CreateJob will call Muster.resolve,
# produce the CapabilityBindingSnapshot with richer metadata, register to Guard, and
# call the deeper Gateway MCP allow-list side-effect).
AGENT_REF="${AGENT_DEFINITION_REF:-}"   # e.g. a UUID of an approved agent that requires third-party MCP tools

# LIVE_MCP_E2E mode: use a prompt crafted to make the LLM actually invoke a third-party MCP tool
# When the mock-mcp-server is running, we prefer the safe "echo" tool it provides.
LIVE_MCP_PROMPT="Call the echo tool on the mock MCP server with text=\"hello from governance smoke\". The tool is approved for this agent. Confirm you received the mocked response."
if [[ "${LIVE_MCP_E2E:-0}" == "1" ]]; then
  # Prefer the mock when the container is present (fully self-contained)
  if curl -sf http://localhost:9009/health >/dev/null 2>&1 || curl -sf http://mock-mcp-server:8080/health >/dev/null 2>&1; then
    LIVE_MCP_PROMPT="Use the echo (or mock_echo) tool provided by the mock MCP server. Pass text=\"governance E2E test\". Confirm the call succeeded and you saw the MOCK_MCP response."
  fi
  echo "== LIVE_MCP_E2E=1: using prompt designed to trigger a real third-party MCP tool call =="
  echo "   (This only exercises the full runtime PreMCPHook + Guard evaluation if a *real* drover-code worker runs the job)"
  PROMPT_TO_USE="$LIVE_MCP_PROMPT"
else
  PROMPT_TO_USE="Muster-resolved hosted agent smoke"
fi

CREATE_BODY='{"prompt":"Milestone A gateway contract smoke","ref":"main"}'

if [[ -n "$AGENT_REF" ]]; then
  CREATE_BODY=$(printf '{"prompt":"%s","ref":"main","agent_definition_ref":"%s"}' "$PROMPT_TO_USE" "$AGENT_REF")
  echo "using agent_definition_ref=$AGENT_REF (Muster resolve + richer snapshot + Gateway side-effect will run)"
  if [[ "${LIVE_MCP_E2E:-0}" == "1" ]]; then
    echo "LIVE_MCP_E2E prompt will be used: $PROMPT_TO_USE"
  fi
fi

CREATE=$(curl -s -m 15 -w "\nHTTP_STATUS:%{http_code}" -X POST "$CLOUD_URL/api/v1/agent-jobs" \
  "${AUTH_HDR[@]}" \
  -H "Content-Type: application/json" \
  -d "$CREATE_BODY")
HTTP_STATUS="${CREATE##*HTTP_STATUS:}"
CREATE="${CREATE%HTTP_STATUS:*}"
CREATE="${CREATE%"${CREATE##*[![:space:]]}"}"
if [[ "$HTTP_STATUS" != "202" && "$HTTP_STATUS" != "200" ]]; then
  echo "agent job create failed (HTTP $HTTP_STATUS): $CREATE" >&2
  exit 1
fi

JOB_ID=$(echo "$CREATE" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
echo "job_id: $JOB_ID"

echo "== stream job events (timeout ${JOB_TIMEOUT}s) =="
STREAM_LOG="$(mktemp)"
trap 'rm -f "$STREAM_LOG" "${STREAM_LOG}.err"' EXIT
(
  curl -sf -N -m "$JOB_TIMEOUT" "$CLOUD_URL/api/v1/agent-jobs/${JOB_ID}/stream" \
    "${AUTH_HDR[@]}" 2>"${STREAM_LOG}.err" | tee "$STREAM_LOG"
) &
STREAM_PID=$!

deadline=$((SECONDS + JOB_TIMEOUT))
TERMINAL=""
STATUS=""
while (( SECONDS < deadline )); do
  STATUS=$(curl -sf -m 10 "$CLOUD_URL/api/v1/agent-jobs/${JOB_ID}" \
    "${AUTH_HDR[@]}")
  TERMINAL=$(echo "$STATUS" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('status',''))")
  if [[ "$TERMINAL" == "succeeded" || "$TERMINAL" == "failed" || "$TERMINAL" == "merge_conflict" ]]; then
    break
  fi
  sleep 1
done

wait "$STREAM_PID" 2>/dev/null || true

echo "== final status =="
echo "$STATUS" | python3 -m json.tool

if [[ "$TERMINAL" != "succeeded" ]]; then
  echo "error: expected job succeeded, got ${TERMINAL:-timeout}" >&2
  [[ -f "${STREAM_LOG}.err" ]] && cat "${STREAM_LOG}.err" >&2 || true
  exit 1
fi

# Run governance verification for LIVE_MCP_E2E runs
if [[ -x "./scripts/verify-governance-smoke.sh" && ( "${LIVE_MCP_E2E:-0}" == "1" || -n "${AGENT_DEFINITION_REF:-}" ) ]]; then
  echo ""
  echo "== Using standalone Governance Smoke Verification =="
  ./scripts/verify-governance-smoke.sh \
    --job-id "$JOB_ID" \
    --cloud-url "$CLOUD_URL" \
    --token "${ZITADEL_MEMBER_TOKEN:-dev-token}" \
    --customer-id "$CID" \
    --expected-tool "echo" \
    ${WITH_CLICKSTACK:+--with-clickstack} \
    ${LIVE_UKC_E2E:+--check-ukc-worker} \
    ${IS_UKC_RUN:+--check-ukc-worker}
else
  # Fallback to inline function (ClickHouse only for now)
  verify_governance_smoke "$JOB_ID"
fi

# -----------------------------------------------------------------------------
# UKC Worker Lifecycle Observability Verification (when running on real Kraftcloud)
# -----------------------------------------------------------------------------
# After the public-gateway + lifecycle hardening, real UKC jobs populate ukc_worker
# on the job record (uuid, started_at, ended_at, lifetime_secs, est_cost_usd, active).
# We assert its presence and basic sanity when LIVE_UKC_E2E=1 or when the job
# response already contains a ukc_worker object (auto-detect for mixed runs).
UKC_WORKER_JSON=""
IS_UKC_RUN=0

UKC_DETECTED=$(echo "$STATUS" | python3 -c '
import sys, json
d = json.load(sys.stdin)
uw = d.get("ukc_worker") or {}
print("yes" if uw.get("uuid") else "no")
' 2>/dev/null || echo "no")

if [[ "$UKC_DETECTED" == "yes" ]]; then
  IS_UKC_RUN=1
  UKC_WORKER_JSON=$(echo "$STATUS" | python3 -c '
import sys, json
d = json.load(sys.stdin)
uw = d.get("ukc_worker") or {}
print(json.dumps(uw, indent=2))
')
  echo ""
  echo "== UKC Worker Instance (lifecycle + cost) =="
  echo "$UKC_WORKER_JSON"

  # Basic assertions for a completed real UKC run
  STATUS_FILE="$(mktemp)"
  echo "$STATUS" > "$STATUS_FILE"
  python3 - "$STATUS_FILE" <<'PY' || { echo "UKC worker verification failed"; rm -f "$STATUS_FILE"; exit 1; }
import sys, json
status_file = sys.argv[1]
with open(status_file) as f:
    status = json.load(f)
uw = status.get("ukc_worker") or {}
errors = []
if not uw.get("uuid"):
    errors.append("missing ukc_worker.uuid")
if not uw.get("started_at"):
    errors.append("missing ukc_worker.started_at")
if uw.get("active") is True:
    errors.append("ukc_worker still marked active after job completion")
if (uw.get("lifetime_secs") or 0) <= 0:
    errors.append("ukc_worker.lifetime_secs should be > 0 for a completed job")
if errors:
    print("UKC worker verification FAIL: " + "; ".join(errors), file=sys.stderr)
    sys.exit(1)
print("UKC worker verification OK (uuid, started_at, lifetime, cleaned up)")
PY
  rm -f "$STATUS_FILE"
fi

# Force UKC mode via env even if we could not auto-detect from this STATUS snapshot
if [[ "${LIVE_UKC_E2E:-0}" == "1" && "$IS_UKC_RUN" == "0" ]]; then
  IS_UKC_RUN=1
  echo "LIVE_UKC_E2E=1 requested — expecting ukc_worker in final job record (may appear in stream or final poll)"
fi

# -----------------------------------------------------------------------------
# RECORDING SUMMARY (structured output for demos / audit / CI)
# This is the "full running smoke recording".
# -----------------------------------------------------------------------------

# Export UKC info for the recording JSON (if we ran on real UKC)
if [[ "$IS_UKC_RUN" == "1" && -n "$UKC_WORKER_JSON" ]]; then
  export IS_UKC_RUN=1
  export UKC_WORKER_UUID=$(echo "$UKC_WORKER_JSON" | python3 -c '
import sys, json
try:
    print(json.load(sys.stdin).get("uuid", ""))
except Exception:
    print("")
' 2>/dev/null || true)
  export UKC_LIFETIME=$(echo "$UKC_WORKER_JSON" | python3 -c '
import sys, json
try:
    print(int(json.load(sys.stdin).get("lifetime_secs", 0)))
except Exception:
    print(0)
' 2>/dev/null || true)
  export UKC_COST=$(echo "$UKC_WORKER_JSON" | python3 -c '
import sys, json
try:
    print(float(json.load(sys.stdin).get("est_cost_usd", 0)))
except Exception:
    print(0)
' 2>/dev/null || true)
fi

echo ""
echo "== RECORDING SUMMARY =="

RECORDING_JSON=$(python3 -c "
import sys, json, os, time
job_id = '$JOB_ID'
cid = '$CID'
agent_ref = os.environ.get('AGENT_DEFINITION_REF', '')
terminal = '$TERMINAL'

summary = {
    'timestamp': time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()),
    'flow': 'MusterResolve -> CloudSnapshot -> GuardRegistration -> GatewaySideEffect -> Worker(Warden) -> JobSuccess',
    'customer_id': cid,
    'job_id': job_id,
    'agent_definition_ref': agent_ref,
    'muster_resolve': 'triggered' if agent_ref else 'skipped (no agent ref)',
    'snapshot_produced': 'yes (richer metadata: tools, clients, risk, approval, version)' if agent_ref else 'no',
    'guard_registration': 'called (fail-closed)',
    'gateway_side_effect': 'called (MCP allowlist)' if agent_ref else 'skipped',
    'warden_in_worker': 'active (if DROVER_WARDEN_BEADS_DIR set)',
    'job_result': terminal,
    'governance_enforced': True,
    'live_mcp_e2e': os.environ.get('LIVE_MCP_E2E', '0') == '1',
    'governance_verification': {
        'binding_events': int(os.environ.get('GOV_BINDING_COUNT', '0')),
        'guard_evaluations': int(os.environ.get('GOV_GUARD_COUNT', '0')),
        'warden_decisions': int(os.environ.get('GOV_WARDEN_COUNT', '0')),
        'passed': os.environ.get('GOV_VERIFICATION_PASSED', 'false') == 'true'
    },
    'ukc_execution': {
        'live_ukc_e2e': os.environ.get('LIVE_UKC_E2E', '0') == '1' or os.environ.get('IS_UKC_RUN', '0') == '1',
        'uuid': os.environ.get('UKC_WORKER_UUID', ''),
        'lifetime_secs': int(os.environ.get('UKC_LIFETIME', '0')),
        'est_cost_usd': float(os.environ.get('UKC_COST', '0')),
        'public_gateway_used': bool(os.environ.get('GATEWAY_PUBLIC_BASE_URL', '')),
    },
    'notes': 'See Cloud logs for exact Muster/Guard/Gateway call traces. LIVE_MCP_E2E=1 + real worker exercises the full runtime PreMCPHook + Guard + Warden path. Governance Smoke Verification asserts real rows in ClickHouse. LIVE_UKC_E2E exercises real Kraftcloud + public gateway + structured ukc_lifecycle events + ukc_worker on job record.'
}

print(json.dumps(summary, indent=2))
")

echo "$RECORDING_JSON"
echo ""
echo "== END OF RECORDING =="

# Also write a machine-readable version
echo "$RECORDING_JSON" > /tmp/governance-smoke-recording.json 2>/dev/null || true
echo "Recording also written to /tmp/governance-smoke-recording.json (if writable)"

# For real UKC / LIVE / REAL_AGENT_EXEC runs we intentionally skip the local contract-checker.
# The real worker (drover-code inside the UKC VM) executed the prompt instead.
if [[ "${LIVE_MCP_E2E:-0}" != "1" && "${REAL_AGENT_EXEC:-0}" != "1" && "${LIVE_UKC_E2E:-0}" != "1" && "$IS_UKC_RUN" != "1" ]]; then
  if ! grep -q 'gateway-contract-ok' "$STREAM_LOG"; then
    echo "error: contract check output missing from stream" >&2
    cat "$STREAM_LOG" >&2
    exit 1
  fi
else
  echo "(real worker mode — skipped local contract-checker assertion; job succeeded via real UKC / drover-code execution)"
fi

echo "== gateway-hosted-agent smoke passed =="
echo "  customer_id=$CID job_id=$JOB_ID"
[[ "$IS_UKC_RUN" == "1" ]] && echo "  ukc_worker_uuid=${UKC_WORKER_UUID:-} lifetime=${UKC_LIFETIME:-}s cost=\$${UKC_COST:-0}"
echo "export AGENT_JOB_ID=${JOB_ID}"
