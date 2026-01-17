# Contributing to Drover

**Navigation:**
- [Documentation Index](./docs/index.md) - Complete documentation hub
- [Feature Specifications](./spec/) - Detailed feature specifications
- [Design Documents](./design/) - Architecture and design decisions

---

Thank you for your interest in contributing to Drover! This document provides guidelines for contributing.

## Development Setup

### Prerequisites

- Go 1.24+ (or 1.22+)
- Git
- [Claude Code CLI](https://claude.ai/code) installed and authenticated
- Docker (for testing observability stack)

### Clone and Build

```bash
# Clone the repository
git clone https://github.com/cloud-shuttle/drover.git
cd drover

# Build
go build -o drover ./cmd/drover

# Run tests
go test ./...

# Run tests with race detector
go test -race ./...
```

## Project Structure

```
drover/
├── cmd/drover/           # CLI entry point
├── internal/
│   ├── config/          # Configuration management
│   ├── db/              # Database operations (SQLite/PostgreSQL)
│   ├── executor/        # Claude Code execution
│   ├── git/             # Git worktree management
│   ├── template/        # Task templates
│   └── workflow/        # DBOS workflow orchestration
├── pkg/
│   ├── telemetry/       # OpenTelemetry observability
│   └── types/           # Shared data structures
├── design/              # Design documentation
├── scripts/
│   └── telemetry/       # Observability configs and docs
└── docs/                # Additional documentation
```

## Running Tests

### Unit Tests

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests with coverage
go test -cover ./...

# Run tests for specific package
go test ./internal/workflow/...
```

### Integration Tests

The `internal/executor` package has tests that use a mock Claude script. These tests verify:
- Task execution with timeout
- Error handling
- Context cancellation

### Observability Tests

To test the observability stack:

```bash
# Run the automated test script
./scripts/telemetry/test-stack.sh
```

This script:
1. Validates Docker setup
2. Starts ClickHouse, OTel Collector, and Grafana
3. Runs Drover with telemetry enabled
4. Verifies traces are collected

## Code Style

### Formatting

We use standard Go formatting:

```bash
go fmt ./...
```

### Linting

```bash
# Run golangci-lint if available
golangci-lint run

# Or use go vet
go vet ./...
```

### Conventions

- **Errors**: Wrap errors with context using `fmt.Errorf("...: %w", err)`
- **Logging**: Use standard `log` package with emoji prefixes for visibility
- **Comments**: Document exported functions and complex logic
- **Attributes**: Use `drover.` prefix for all OTel attributes

## Adding Telemetry

When adding new features, include observability:

```go
import "github.com/cloud-shuttle/drover/pkg/telemetry"

func MyOperation(ctx context.Context) error {
    // Start a span
    ctx, span := telemetry.StartTaskSpan(ctx, "drover.my.operation",
        telemetry.TaskAttrs(id, title, state, priority, 1)...)
    defer span.End()

    // Your logic here
    err := doSomething(ctx)
    if err != nil {
        telemetry.RecordError(span, err, "OperationFailed", "my_category")
        return err
    }

    // Record metrics
    telemetry.RecordTaskCompleted(ctx, workerID, projectID, duration)
    return nil
}
```

## Submitting Changes

### Pull Request Process

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Run tests (`go test ./...`)
5. Commit with a clear message
6. Push to your fork (`git push origin feature/amazing-feature`)
7. Open a pull request

### Commit Message Format

We follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add support for parallel epic execution

- Added epic-level parallel execution
- Updated dependency resolution for epics
- Added tests for epic workflow

Co-Authored-By: Claude <noreply@anthropic.com>
```

Types: `feat`, `fix`, `docs`, `chore`, `refactor`, `test`

## Adding New Commands

To add a new CLI command:

1. Create the command function in `cmd/drover/commands.go`
2. Add to `rootCmd.AddCommand()` in `main.go`
3. Add tests in a new `cmd/drover/mycommand_test.go`
4. Update this document with usage examples

## Testing Observability

When testing changes that affect telemetry:

1. Start the observability stack:
   ```bash
   docker compose -f docker-compose.telemetry.yaml up -d
   ```

2. Enable telemetry:
   ```bash
   export DROVER_OTEL_ENABLED=true
   ```

3. Run your feature:
   ```bash
   ./drover run
   ```

4. Verify traces:
   ```bash
   curl 'http://localhost:8123/?query=SELECT%20count()%20FROM%20otel_traces'
   ```

5. Check Grafana: http://localhost:3000

## Design Documentation

For larger changes, consider updating the design docs:

- `design/DESIGN.md` - Overall architecture and design decisions
- `design/architecture.md` - System architecture diagrams
- `design/sequence.md` - Sequence diagrams for key workflows
- `design/state-machine.md` - State transition logic

## Getting Help

- **GitHub Issues**: Bug reports and feature requests
- **Discussions**: Design questions and RFCs
- **Pull Requests**: Code changes and improvements

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
