# Project Planning with Drover

This guide shows you how to start a new project using Drover with Claude to plan epics and tasks.

## Overview

The workflow is:

1. **Initialize** a Drover project
2. **Plan** with Claude to create epics and tasks from your idea
3. **Run** Drover to execute everything in parallel

## Step 1: Initialize a Project

```bash
cd /path/to/your/project
drover init
```

This creates `.drover/` directory with:
- `.drover/drover.db` - SQLite database for tasks
- `.drover/worktrees/` - Git worktrees for parallel execution

## Step 2: Plan with Claude

### Option A: Interactive Planning (Recommended)

Start a Claude Code session to plan your project:

```bash
# Start Claude Code in planning mode
claude

# Then paste your project idea:
```

**Example prompt for Claude:**

```
I want to build a TODO application with the following features:

- User authentication (signup, login, logout)
- Create, read, update, delete todos
- Organize todos into projects/folders
- Share projects with other users
- Real-time collaboration
- Mobile-friendly responsive UI

Tech stack I want to use:
- Backend: Go with Gin framework
- Frontend: React with TypeScript
- Database: PostgreSQL
- Auth: JWT tokens

Please help me plan this by creating epics and tasks for Drover.
For each epic, give me the exact drover commands to run.
```

**Claude should respond with something like:**

```
# Epic 1: Project Setup
drover epic add "Project Setup" -d "Initialize repository, database, and development environment"

drover add "Initialize Go backend with Gin" --epic <epic-id> -d "Set up Go module, Gin router, basic folder structure"
drover add "Initialize React frontend with TypeScript" --epic <epic-id> -d "Set up React app with TypeScript, routing, and state management"
drover add "Configure PostgreSQL database schema" --epic <epic-id> -d "Design and create initial database tables for users, todos, projects"
drover add "Set up development environment" --epic <epic-id> -d "Docker compose for local development, hot-reload setup"

# Epic 2: Authentication
drover epic add "Authentication" -d "User signup, login, logout with JWT"

drover add "Design user database schema" --epic <epic-id> -d "Create users table with email, password_hash, created_at"
drover add "Implement signup API endpoint" --epic <epic-id> -d "POST /auth/signup with validation and password hashing"
drover add "Implement login API endpoint" --epic <epic-id> -d "POST /auth/login returning JWT token"
drover add "Implement JWT middleware" --epic <epic-id> -d "Authentication middleware for protected routes"
drover add "Build login/signup UI" --epic <epic-id> -d "React forms for authentication with validation"

# Epic 3: Core TODO Features
drover epic add "Core TODO Features" -d "CRUD operations for todos"

drover add "Design todo database schema" --epic <epic-id> -d "Create todos table with title, description, status, project_id, user_id"
drover add "Implement create todo API" --epic <epic-id> -d "POST /todos with validation and user ownership"
drover add "Implement list todos API" --epic <epic-id> -d "GET /todos with filtering by project and status"
drover add "Implement update todo API" --epic <epic-id> -d "PUT /todos/:id for updating title, description, status"
drover add "Implement delete todo API" --epic <epic-id> -d "DELETE /todos/:id with ownership check"
drover add "Build todo list UI" --epic <epic-id> -d "React component displaying todos with create/edit/delete actions"

# Epic 4: Projects/Folders
drover epic add "Project Organization" -d "Organize todos into projects"

drover add "Design project database schema" --epic <epic-id> -d "Create projects table with name, user_id, and shared users"
drover add "Implement project CRUD APIs" --epic <epic-id> -d "Full CRUD for projects with user ownership"
drover add "Add project filtering to todos API" --epic <epic-id> -d "Filter todos by project_id"
drover add "Build project management UI" --epic <epic-id> -d "Sidebar with project list and creation"

# Epic 5: Sharing & Collaboration
drover epic add "Sharing & Collaboration" -d "Share projects with other users"

drover add "Add project sharing database schema" --epic <epic-id> -d "Create project_members table for many-to-many relationship"
drover add "Implement share project API" --epic <epic-id> -d "POST /projects/:id/share to add users"
drover add "Implement permission checks" --epic <epic-id> -d "Middleware to check if user has access to project"
drover add "Build sharing UI" --epic <epic-id> -d "Invite users by email to projects"

# Epic 6: Real-time Collaboration
drover epic add "Real-time Updates" -d "WebSocket updates for collaboration"

drover add "Set up WebSocket server" --epic <epic-id> -d "WebSocket endpoint for real-time todo updates"
drover add "Broadcast todo changes" --epic <epic-id> -d "Send updates to all connected users in a project"
drover add "Implement real-time UI updates" --epic <epic-id> -d "React hooks for WebSocket connection and live updates"

# Epic 7: Testing & Deployment
drover epic add "Testing & Deployment" -d "Test coverage and production deployment"

drover add "Write unit tests for API endpoints" --epic <epic-id> -d "Go tests for all endpoints with mock database"
drover add "Write frontend tests" --epic <epic-id> -d "Jest tests for React components"
drover add "Set up CI/CD pipeline" --epic <epic-id> -d "GitHub Actions for testing and deployment"
drover add "Deploy to production" --epic <epic-id> -d "Docker containers, database migrations, monitoring"

# Add dependencies where needed
# For example, authentication should complete before todo features:
drover add "Build todo list UI" --epic <core-todo-epic-id> --blocked-by <auth-ui-task-id>
```

### Option B: Scripted Planning

Create a shell script from Claude's output:

```bash
# plan.sh
#!/bin/bash

# Create epics
AUTH_EPIC=$(drover epic add "Authentication" -d "User signup, login, logout" | grep -oP 'Created epic \K[^:]+')
TODO_EPIC=$(drover epic add "TODO Features" -d "CRUD operations for todos" | grep -oP 'Created epic \K[^:]+')

# Create tasks in Authentication epic
drover add "Design user schema" --epic $AUTH_EPIC
drover add "Implement signup API" --epic $AUTH_EPIC --blocked-by <design-schema-task-id>
drover add "Implement login API" --epic $AUTH_EPIC --blocked-by <signup-task-id>

# Create tasks in TODO Features epic
drover add "Design todo schema" --epic $TODO_EPIC
drover add "Implement create todo API" --epic $TODO_EPIC --blocked-by <todo-schema-task-id>
# ... etc
```

## Step 3: Review Your Plan

Before running, check your plan:

```bash
# Show all epics
drover epic

# Show project status
drover status
```

## Step 4: Execute

```bash
# Run all tasks in parallel
drover run

# Or with specific number of workers
drover run --workers 4
```

Drover will:
1. Claim tasks respecting dependencies
2. Create git worktrees for parallel execution
3. Run Claude Code in each worktree
4. Commit changes per task
5. Merge to main branch
6. Unblock dependent tasks automatically

## Example: Minimal Quickstart

```bash
# 1. Initialize
mkdir my-app && cd my-app
git init
drover init

# 2. Have Claude create a simple plan
# Ask Claude: "Create a 3-epic plan for a blog API with Go and Gin"
# Then run the commands it gives you

# Example:
drover epic add "Setup" -d "Project initialization"
drover epic add "API" -d "Blog CRUD endpoints"
drover epic add "Frontend" -d "Simple web UI"

# Add tasks (example)
drover add "Initialize Go project" --epic <setup-id>
drover add "Create post model" --epic <api-id> --blocked-by <init-task-id>
drover add "GET /posts endpoint" --epic <api-id> --blocked-by <model-task-id>
drover add "POST /posts endpoint" --epic <api-id> --blocked-by <model-task-id>
drover add "HTML templates" --epic <frontend-id>

# 3. Run
drover run
```

## Tips for Better Planning

1. **Start with Claude to brainstorm**: Use Claude to explore the problem space before creating tasks
2. **Granular tasks**: Smaller tasks are better (1-2 hours each) for parallel execution
3. **Clear dependencies**: Use `--blocked-by` to ensure tasks execute in correct order
4. **Descriptive titles**: Task titles should be self-contained instructions
5. **Add context**: Use `-d` for additional context Claude might need

## Advanced: Use Beads Integration

If you have existing tasks in `.beads/beads.jsonl`:

```bash
# Import from Beads to Drover
drover sync --from-beads

# Export Drover tasks to Beads format
drover sync --to-beads
```

## Troubleshooting

**Task fails?**
```bash
# Check status
drover status

# Resume incomplete tasks
drover resume
```

**Need to replan?**
```bash
# Tasks are just data in SQLite
# You can manually update or recreate tasks
sqlite3 .drover/drover.db
```
