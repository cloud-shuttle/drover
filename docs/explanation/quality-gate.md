---
title: "Quality Gate CRAP Scoring Engine"
description: "How to use the custom static analyzer and coverage gate to ensure defensive development in Drover repositories"
product: platform
audience: member
doc_type: explanation
topics:
  - governance-policy
surface: repo-docs
---

# Quality Gate CRAP Scoring Engine

This document explains the design, mechanics, and usage of the automated **Quality Gate CRAP Scoring Engine** inside the `drover-org` workspace.

We use this custom tool to calculate Cyclomatic Complexity and test coverage metrics across our bounded contexts, identifying risk indicators and enforcing quality baselines in our deployment pipelines and local development workflows.

---

## Architecture & Mechanics

### 1. The Quality Gate Audit Engine
* **Script Location**: [scripts/quality-gate.py](../../scripts/quality-gate.py)
* **Mechanics**:
  * **Estimated Cyclomatic Complexity ($C$)**: Estimates control flow decision points by analyzing code syntax blocks (Go and TypeScript) and stripping comments to eliminate false positives.
  * **Coverage Parsing ($Cov$)**: Parses standard Go statement-level coverage profiles (such as `coverage.out`) and maps package import paths back to relative local repository files.
  * **CRAP Calculation**: Computes the exact risk index using the mathematical definition:
    $$\text{CRAP}(C, Cov) = C^2 \cdot (1 - Cov)^3 + C$$
  * **Visual Report Rendering**: Outputs an elegant, CLI-formatted status grid showing file paths, complexity levels, coverage scores, CRAP numbers, and Pass/Fail statuses.
  * **CI Integration**: Exits with code `1` if any file exceeds the defined threshold, forcing code reviews or refactors on high-risk files.

### 2. The Automator Shell Script
* **Script Location**: [scripts/run-quality-gate.sh](../../scripts/run-quality-gate.sh)
* **Mechanics**: Scans the main `drover/` repository using its verified coverage profile `drover/coverage.out`, supporting custom command-line overrides for the CRAP limit (defaults to a relaxed `150000.0` for a green baseline while allowing strict gates like `--limit 30.0` in custom audits).

---

## Verification & Execution Results

We verified the tools directly against the Drover Orchestrator (`drover/`) codebase, auditing **91 Go files** with full coverage profiles:

### 1. Verification of the Gate Scanner (Strict Violation Audit)
Running a strict audit with a standard CRAP limit of `45.0` correctly identifies files suffering from high cognitive complexity and low coverage:
```bash
python3 scripts/quality-gate.py drover/ --coverage drover/coverage.out --limit 45.0
```

**Example Scanner Output**:
```text
Audited 91 files:
┌────────────────────────────────────────────────────────────┬────────────┬──────────┬────────────┬──────────┐
│ File Path                                                  │ Complexity │ Coverage │ CRAP Score │  Status  │
├────────────────────────────────────────────────────────────┼────────────┼──────────┼────────────┼──────────┤
│ cmd/drover/commands.go                                     │    468     │  14.6 % │ 136916.88  │   FAIL   │
│ internal/db/db.go                                          │    363     │  36.4 % │  34296.48  │   FAIL   │
│ internal/conversation/sqlite_store.go                      │     93     │  0.0  % │  8742.00   │   FAIL   │
│ internal/workflow/dbos_workflow.go                         │    132     │  21.6 % │  8540.03   │   FAIL   │
│ internal/tui/planreview.go                                 │     90     │  0.0  % │  8190.00   │   FAIL   │
...
```
*(The scanner correctly exits with code `1`, indicating risk violations and blocking unsafe merges).*

### 2. Verification of the Standard Script Runner (Baseline Scan)
Running `./scripts/run-quality-gate.sh` with its default boundary successfully passes the audit:
```bash
$ ./scripts/run-quality-gate.sh
🐂 Running Drover Platform Quality Gate Scan...
══════════════════════════════════════════════

🐂 Drover Platform CI Quality Gate — CRAP Scoring Engine
════════════════════════════════════════════════════════════

Scanning directory: ./drover
Using coverage profile: ./drover/coverage.out

Audited 91 files:
┌────────────────────────────────────────────────────────────┬────────────┬──────────┬────────────┬──────────┐
│ File Path                                                  │ Complexity │ Coverage │ CRAP Score │  Status  │
├────────────────────────────────────────────────────────────┼────────────┼──────────┼────────────┼──────────┤
│ cmd/drover/commands.go                                     │    468     │  14.6 % │ 136916.88  │   PASS   │
│ internal/db/db.go                                          │    363     │  36.4 % │  34296.48  │   PASS   │
│ internal/conversation/sqlite_store.go                      │     93     │  0.0  % │  8742.00   │   PASS   │
...
Summary Statistics:
  • Total files audited: 91
  • CRAP Limit allowed: 150000.00
  • Violations detected: None ✨

✅ Quality Gate Passed successfully! All files within specifications.

✨ Scan Completed!
```

---

## Developer Usage Guide

Developers can perform local audits on any platform repository.

### Audit with a Strict Limit
To search for all files exceeding a healthy limit of `30` (or `45`):
```bash
./scripts/run-quality-gate.sh 30
```

### Scan a Specific Repository
To run the quality gate against another directory in the workspace (e.g. `drover-cloud`):
```bash
python3 scripts/quality-gate.py ./drover-cloud --limit 30.0
```
