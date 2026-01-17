# Drover Architecture Diagram

**Navigation:**
- [Documentation Index](../docs/index.md) - Central documentation hub
- [Durable Workflows Spec](../spec/durable-workflows.md) - Workflow engine specification
- [Design Overview](./DESIGN.md) - Comprehensive design document
- [State Machine](./state-machine.md) - Task state transitions
- [Sequence Diagrams](./sequence.md) - Runtime flow

---

```mermaid
flowchart TB
    subgraph CLI["🖥️ Command Line Interface"]
        init["drover init"]
        run["drover run"]
        add["drover add"]
        status["drover status"]
        resume["drover resume"]
    end

    subgraph DBOS["⚡ DBOS Durable Workflow Engine"]
        subgraph MainWorkflow["DroverRunWorkflow"]
            loop["Orchestration Loop"]
            discover["Discover Ready Tasks"]
            claim["Claim Tasks"]
            enqueue["Enqueue to Workers"]
            collect["Collect Results"]
            sleep["Durable Sleep"]
        end

        subgraph Queue["DBOS Queue"]
            q1["Task Queue"]
            conc["Concurrency Control"]
        end

        subgraph TaskWorkflows["ExecuteTaskWorkflow (×N)"]
            tw1["Worker 1"]
            tw2["Worker 2"]
            tw3["Worker 3"]
            twN["Worker N"]
        end
    end

    subgraph Execution["🔧 Task Execution"]
        subgraph Worktrees["Git Worktrees"]
            wt1[".drover/worktrees/task-1"]
            wt2[".drover/worktrees/task-2"]
            wt3[".drover/worktrees/task-3"]
            wtN[".drover/worktrees/task-N"]
        end

        subgraph Agents["AI Agents (Pluggable)"]
            cc1["Agent 1 (Claude/Codex/Amp)"]
            cc2["Agent 2 (Claude/Codex/Amp)"]
            cc3["Agent 3 (Claude/Codex/Amp)"]
            ccN["Agent N (Claude/Codex/Amp)"]
        end
    end

    subgraph Storage["💾 Persistent Storage"]
        subgraph Postgres["PostgreSQL / SQLite"]
            tasks[("tasks")]
            epics[("epics")]
            deps[("task_dependencies")]
            dbos_state[("dbos_workflow_*")]
        end
    end

    subgraph Git["📁 Git Repository"]
        main["main branch"]
        branches["drover/* branches"]
        commits["Task Commits"]
    end

    %% CLI to DBOS
    run --> loop
    resume --> loop
    add --> tasks
    init --> Postgres
    status --> tasks

    %% Main workflow loop
    loop --> discover
    discover --> claim
    claim --> enqueue
    enqueue --> q1
    collect --> sleep
    sleep --> loop

    %% Queue to workers
    q1 --> conc
    conc --> tw1 & tw2 & tw3 & twN

    %% Workers to worktrees
    tw1 --> wt1
    tw2 --> wt2
    tw3 --> wt3
    twN --> wtN

    %% Worktrees to Claude
    wt1 --> cc1
    wt2 --> cc2
    wt3 --> cc3
    wtN --> ccN

    %% Database interactions
    discover -.-> tasks
    claim -.-> tasks
    collect -.-> tasks
    tw1 & tw2 & tw3 & twN -.-> dbos_state
    tasks -.-> deps
    tasks -.-> epics

    %% Git interactions
    wt1 & wt2 & wt3 & wtN --> branches
    branches --> commits
    commits --> main

    %% Styling
    classDef cli fill:#e1f5fe,stroke:#01579b
    classDef dbos fill:#fff3e0,stroke:#e65100
    classDef exec fill:#f3e5f5,stroke:#7b1fa2
    classDef storage fill:#e8f5e9,stroke:#2e7d32
    classDef git fill:#fce4ec,stroke:#c2185b

    class init,run,add,status,resume cli
    class loop,discover,claim,enqueue,collect,sleep,q1,conc,tw1,tw2,tw3,twN dbos
    class wt1,wt2,wt3,wtN,cc1,cc2,cc3,ccN exec
    class tasks,epics,deps,dbos_state storage
    class main,branches,commits git
```

## Component Organization

```mermaid
graph LR
    subgraph External["External Dependencies"]
        agents["☁️ AI Agents<br/>(Claude/Codex/Amp)"]
        git["📁 Git<br/>(Version Control)"]
        pg["🐘 PostgreSQL<br/>(Production)"]
        sqlite["📄 SQLite<br/>(Development)"]
    end

    subgraph Drover["Drover Application"]
        subgraph cmd["cmd/"]
            main_go["main.go<br/><i>CLI entry point</i>"]
        end

        subgraph internal["internal/"]
            workflows["workflows.go<br/><i>DBOS workflows</i>"]
            database["database.go<br/><i>Task state</i>"]
            executor["executor/<br/><i>Agent interface & impls</i>"]
            gitops["git.go<br/><i>Worktree ops</i>"]
        end

        subgraph pkg["pkg/"]
            models["models.go<br/><i>Task, Epic types</i>"]
            config["config.go<br/><i>Configuration</i>"]
        end
    end

    subgraph Libraries["Go Libraries"]
        dbos["github.com/dbos-inc/<br/>dbos-transact-golang"]
        cobra["github.com/spf13/cobra"]
        sqlx["database/sql"]
    end

    main_go --> cobra
    main_go --> workflows
    workflows --> dbos
    workflows --> database
    workflows --> executor
    database --> sqlx
    database --> models
    executor --> gitops
    executor --> claude
    gitops --> git
    sqlx --> pg
    sqlx --> sqlite
    config --> models

    classDef external fill:#f5f5f5,stroke:#9e9e9e
    classDef app fill:#e3f2fd,stroke:#1976d2
    classDef lib fill:#fff8e1,stroke:#f57c00

    class claude,git,pg,sqlite external
    class main_go,workflows,database,executor,gitops,models,config app
    class dbos,cobra,sqlx lib
```

## Directory Structure

```
drover/
├── cmd/
│   └── drover/
│       ├── main.go          # CLI entry point
│       └── commands.go      # Command implementations
├── internal/
│   ├── config/              # Configuration loading
│   ├── db/                  # Database operations
│   ├── executor/            # Agent interface (Claude/Codex/Amp)
│   ├── workflow/            # DBOS workflow definitions
│   └── git/                 # Git worktree management
├── pkg/
│   └── types/               # Shared types (Task, Epic, etc.)
├── design/
│   ├── DESIGN.md            # This document
│   ├── sequence.md          # Runtime flow diagrams
│   └── state-machine.md     # Task state transitions
├── go.mod
├── go.sum
└── README.md
```

## Data Flow

```
User Input → CLI Command → DBOS Workflow → Database Query
                                        ↓
                                  DBOS Queue
                                        ↓
                            ┌───────────┼───────────┐
                            ↓           ↓           ↓
                        Worker 1    Worker 2    Worker N
                            ↓           ↓           ↓
                      Worktree 1  Worktree 2  Worktree N
                            ↓           ↓           ↓
                      Agent (Claude/Codex/Amp)  ...
                            ↓           ↓           ↓
                      Git Commit   Git Commit   Git Commit
                            └───────────┼───────────┘
                                        ↓
                                  Merge to Main
                                        ↓
                                  Update Database
                                        ↓
                            Unblock Dependent Tasks
```
