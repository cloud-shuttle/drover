# drover roadmap

Last updated 18 Nov 2026. Status: **ACTIVE**. Public parallel coordinator. Last code change 15 Nov 2026 (per-type agent binaries, Drover Code wired in). Already ships `pkg/telemetry` (OpenTelemetry) and `internal/sessionevent`, both of which the M1 trace spine promotes into drover-libs.

## The Drover gap program

In November 2026 Drover was compared with Vercel eve 1.0. Drover leads on isolation, the model gateway and governance, and trails on the layer people touch. The program closes five gaps across the repos: durable multi-turn sessions (drover-kelpies), a Slack-first channels layer (drover-cloud), unified OpenTelemetry tracing with a run viewer (drover-libs, Cloud console), a TypeScript client SDK (drover-cloud), and an extension bundle format (drover-muster, drover-registry). Four capabilities are deferred to Later: shared multi-person sessions, ambient channel listening, agents editing their own code, and session prewarm and migration.

Program tracking issue: in [cloud-shuttle/drover-cloud](https://github.com/cloud-shuttle/drover-cloud/issues?q=is%3Aissue+label%3Aroadmap+Tracking) (private). Sessions run on DBOS, not Temporal (ADR 0005 in drover-kelpies). Dates and day counts are estimates.

| Milestone | Due (estimate) |
|---|---|
| M0 Truth-up | 27 Nov 2026 |
| M1 Trace spine | 18 Dec 2026 |
| M2 Durable sessions | 5 Feb 2027 |
| M3 Channels and SDK | 5 Mar 2027 |
| M4 Extensions | 2 Apr 2027 |
| Later | after M4 |

## This repo in the program

### M1 Trace spine (due 18 Dec 2026)

- [Consume drover-libs pkg/telemetry and pkg/sessionevent](https://github.com/cloud-shuttle/drover/issues/35)

### Later (after M4)

- [Later: session prewarm and migration, building on worktree prewarming](https://github.com/cloud-shuttle/drover/issues/36)

Each item is a GitHub issue labelled `roadmap` with a `gap:*` label and a milestone: [roadmap issues in this repo](https://github.com/cloud-shuttle/drover/issues?q=is%3Aissue+label%3Aroadmap).

## Proposed releases

- next minor release at M1 (shared telemetry and sessionevent, no API break)

## Existing roadmap epics

These predate the gap program and stay open:

- [#33 Epic: Performance and extensibility](https://github.com/cloud-shuttle/drover/issues/33)
- [#32 Epic: Platform integration: orchestrator runs from Drover Cloud](https://github.com/cloud-shuttle/drover/issues/32)
- [#31 Epic: Beads integration and multiplayer](https://github.com/cloud-shuttle/drover/issues/31)
- [#30 Epic: LLM proxy mode](https://github.com/cloud-shuttle/drover/issues/30)

## How this file is kept current

Update this file in the same PR that closes a roadmap issue or moves a milestone. Status words used across Drover: ACTIVE (code changed in the last 60 days), STALE (no code change for 61 to 180 days), PROTOTYPE (experimental or scaffold), SUPERSEDED (replaced by another repo).
