# Contributing to agent-benchmark

## Development Setup

```bash
git clone https://github.com/mykhaliev/agent-benchmark.git
cd agent-benchmark
go build ./...
```

## Running Tests

```bash
go test ./...
```

## Generating Sample Reports

To generate HTML reports for manual inspection:

```bash
go run test/generate_reports.go
```

Reports are written to `generated_reports/` (gitignored).

## Test Structure

Tests live in the `test/` directory:

| File | Purpose |
|------|---------|
| `agent_test.go` | Agent execution tests |
| `engine_test.go` | Test engine orchestration |
| `model_test.go` | Data model validation |
| `templates_test.go` | Template helper tests |
| `report_test.go` | HTML/JSON report generation |
| `report_fixtures.go` | Realistic test data for report tests |
| `mocks.go` | Shared test utilities |
| `generate_reports.go` | Script to generate sample HTML reports |

## Submitting Changes

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run `go test ./...` and ensure all tests pass
5. Submit a pull request
