# Testing

## Synopsis

Relurpify has two testing layers. Unit tests cover framework internals deterministically and require no running services. Agent tests run real agent workflows and require Ollama, but can be made deterministic through recording and replay.

---

## Why Two Layers

Unit tests answer: *does the framework behave correctly?* They test the permission manager, graph runtime, manifest validation, tool logic — things that have deterministic correct answers independent of what a language model says.

Agent tests answer: *does the agent actually solve the problem?* They test the full stack: instruction → agent reasoning → tool calls → result. Because LLM output is non-deterministic, agent tests use structured assertions (did it call the right tools? did it produce output containing the right content?) rather than exact string matching. Recording and replay makes them reproducible.

---

## Unit Tests

Unit tests live alongside the packages they test and require no external dependencies.

```bash
go test ./...
```

All unit tests pass without Ollama running. LLM-dependent paths use the `TapeModel` replay mechanism in test mode.

```bash
# With coverage
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out

# Single package
go test ./framework/runtime/...
go test ./framework/graph/...

# Browser stack only
go test ./framework/browser/... ./app/relurpish/runtime
```

---

## Agent Tests

Agent tests are YAML-driven test suites that run full agent workflows. They live in `testsuite/agenttests/` and are executed with the `agenttest run` command.

### Prerequisites

Ollama must be running with the target model pulled:

```bash
ollama pull qwen2.5-coder:14b
```

### Running

```bash
# Run all test suites in testsuite/agenttests/
go run ./cmd/coding-agent agenttest run

# Run a specific suite
go run ./cmd/coding-agent agenttest run \
    --suite testsuite/agenttests/coding.go.testsuite.yaml

# Run with a specific agent and timeout
go run ./cmd/coding-agent agenttest run \
    --agent coding-go \
    --suite testsuite/agenttests/coding.go.testsuite.yaml \
    --timeout 120s
```

Without `--suite`, the runner searches `testsuite/agenttests/` for all `*.testsuite.yaml` files.

---

## Test Suite Format

Each suite is a committed YAML file defining a set of test cases:

```yaml
name: Go Coding Agent Tests
agent: coding-go
timeout: 90s

tests:
  - name: Read and summarise a file
    instruction: "Summarise README.md in one paragraph"
    assertions:
      - type: output_contains
        value: "Relurpify"
      - type: no_error

  - name: Run the test suite
    instruction: "Run the Go test suite and report results"
    assertions:
      - type: tool_called
        value: run_tests
      - type: no_error

  - name: File modification
    instruction: "Add a comment to the top of tools/files.go"
    setup:
      - type: snapshot_file
        path: tools/files.go
    assertions:
      - type: file_modified
        path: tools/files.go
    teardown:
      - type: restore_snapshot
        path: tools/files.go
```

### Assertions

| Type | What it checks |
|------|----------------|
| `output_contains` | Final response includes the given string |
| `output_matches` | Final response matches a regex |
| `no_error` | Run completed without error |
| `tool_called` | Named tool was invoked at least once |
| `file_modified` | Named file was changed |
| `file_unchanged` | Named file was not changed |

### Setup and Teardown

`setup` and `teardown` steps run before and after each test. The `snapshot_file` + `restore_snapshot` pattern is standard for tests that modify files — it ensures a clean slate between runs and prevents one test from poisoning the next.

---

## Recording Mode

Recording mode makes agent tests deterministic by recording all LLM interactions to a tape file and replaying them on subsequent runs.

### How It Works

```
capture mode:
  Real Ollama call → response → stored in tape file

replay mode:
  Tape file → response → no Ollama call needed
```

The tape is a structured log of requests and responses. Replay mode matches incoming requests to recorded entries and returns the stored response — the agent behaves identically every time.

### Modes

| Mode | Flag | Behaviour |
|------|------|-----------|
| `off` | `--recording-mode off` | Normal live calls |
| `capture` | `--recording-mode capture` | Record interactions to tape |
| `replay` | `--recording-mode replay` | Replay from tape; no Ollama needed |

### Recommended Workflow

```bash
# 1. Record with Ollama running
go run ./cmd/coding-agent agenttest run \
    --recording-mode capture \
    --suite testsuite/agenttests/coding.go.testsuite.yaml

# 2. Commit the tape file (stored in testsuite/agenttest_fixtures/)

# 3. CI replays without Ollama
go run ./cmd/coding-agent agenttest run \
    --recording-mode replay \
    --suite testsuite/agenttests/coding.go.testsuite.yaml
```

Recording mode can also be toggled from the TUI Settings pane (pane 4, Recording row).

---

## Sandbox Mode

Agent tests can run in an isolated copy of the workspace to prevent test pollution:

```bash
go run ./cmd/coding-agent agenttest run \
    --sandbox \
    --suite testsuite/agenttests/coding.go.testsuite.yaml
```

Sandbox mode copies the workspace to a temporary directory before running and cleans up afterwards. File modifications made by the agent during the test do not affect your actual workspace.

Note: `--sandbox` for agenttest uses `LocalCommandRunner` (host execution, no gVisor). gVisor-sandboxed execution is the production path via `relurpish`.

---

## CI Integration

```bash
# Run all suites in CI
RELURPIFY_AGENTTEST_SUITE=testsuite/agenttests/coding.go.testsuite.yaml \
    ./scripts/ci.sh
```

In CI, use `--recording-mode replay` so Ollama is not required.

### Browser CI

Browser support now has its own CI entry point:

```bash
./scripts/browser-ci.sh
```

This runs:

- browser package tests
- runtime/provider tests that cover browser session wiring
- optional repeated browser stress tests when `RELURPIFY_BROWSER_STRESS=1`

Example:

```bash
# Standard browser gating
./scripts/browser-ci.sh

# Include repeated localhost stress runs
RELURPIFY_BROWSER_STRESS=1 ./scripts/browser-ci.sh
```

If a CI environment intentionally does not validate browser support, set:

```bash
RELURPIFY_BROWSER_CI=0 ./scripts/ci.sh
```

### Browser Release Gate

Browser support should not be considered releasable unless the following all pass:

- `go test ./framework/browser/... ./app/relurpish/runtime`
- real localhost integration tests for CDP, Classic WebDriver, and BiDi on a supported Chromium/ChromeDriver pair
- repeated stress runs with `RELURPIFY_BROWSER_STRESS=1`
- cleanup tests confirming browser processes and temporary profiles are removed on shutdown
- parallel session isolation tests in the runtime provider layer

---

## See Also

- [Architecture](architecture.md) — how agenttest fits into the overall system
- [Agents](agents.md) — which agent manifests to test against
- [TUI](tui.md) — recording mode toggle in Settings pane
