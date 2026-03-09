# Testing

## Synopsis

Relurpify has two testing layers. Unit tests cover framework internals deterministically and require no running services. Agent tests run full workflows from committed YAML suites and usually require Ollama, but they can be replayed from tape for reproducibility.

---

## Why Two Layers

Unit tests answer: *does the framework behave correctly?* They exercise manifest validation, permissions, tool logic, persistence, and runtime plumbing.

Agent tests answer: *does the agent actually solve the problem?* They execute the full stack: prompt -> agent reasoning -> tool calls -> result. Because model output is non-deterministic, suites assert on structured outcomes such as changed files, required tool calls, and output snippets instead of exact strings.

Testsuites run against derived temporary workspaces so they do not reuse the live workspace's `relurpify_cfg/` state directly.

---

## Unit Tests

Unit tests live alongside the packages they test and require no external dependencies.

```bash
go test ./...
```

Useful variants:

```bash
# With coverage
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out

# Single package
go test ./framework/runtime/...
go test ./framework/graph/...
```

LLM-dependent unit tests use tape-backed models where needed, so Ollama is not required for normal unit test runs.

---

## Agent Tests

Agent tests are YAML-driven suites stored in `testsuite/agenttests/`. Run them with:

```bash
go run ./cmd/dev-agent agenttest run
```

### Prerequisites

For live runs, Ollama must be available with the target model pulled:

```bash
ollama pull qwen2.5-coder:14b
```

### Running

```bash
# Run every discovered suite
go run ./cmd/dev-agent agenttest run

# Run one suite explicitly
go run ./cmd/dev-agent agenttest run \
    --suite testsuite/agenttests/coding.go.testsuite.yaml

# Filter discovered suites by agent prefix and raise timeout
go run ./cmd/dev-agent agenttest run \
    --agent coding \
    --timeout 120s
```

Without `--suite`, the runner searches `testsuite/agenttests/` for `*.testsuite.yaml`.

Useful flags:

```bash
--suite <path>                 Repeatable suite path
--agent <name>                 Filter discovered suites by agent prefix
--out <dir>                    Artifact output directory
--sandbox                      Run tool execution via gVisor/docker
--timeout 120s                 Per-case timeout
--model <name>                 Override model for all cases
--endpoint <url>               Override Ollama endpoint
--max-iterations <n>           Override agent loop limit
--debug-llm                    Enable verbose LLM telemetry
--debug-agent                  Enable verbose agent logging
--ollama-reset none|model|server
--ollama-reset-between
--ollama-reset-on <regex>      Repeatable reset trigger
```

---

## Suite Format

Suites use the versioned `AgentTestSuite` schema:

```yaml
apiVersion: relurpify/v1alpha1
kind: AgentTestSuite
metadata:
  name: coding.go
  description: Go-focused prompts for the coding agent
spec:
  agent_name: coding
  manifest: relurpify_cfg/agents/coding-go.yaml
  workspace:
    strategy: derived
    template_profile: default
    exclude:
      - .git/**
      - relurpify_cfg/test_runs/**
  models:
    - name: qwen2.5-coder:14b
      endpoint: http://localhost:11434
  recording:
    mode: off
  cases:
    - name: easy_fix_bug_and_run_tests
      task_type: code_modification
      prompt: >
        Fix the failing test in testsuite/agenttest_fixtures/gosuite/mathutil,
        then run cli_go test and confirm it passes.
      context:
        mode: code
      setup:
        files:
          - path: testsuite/agenttest_fixtures/gosuite/mathutil/mathutil.go
            content: |
              package mathutil
      overrides:
        allowed_capabilities:
          - name: cli_go
            kind: tool
        control_flow: pipeline
      expect:
        must_succeed: true
        files_changed:
          - testsuite/agenttest_fixtures/gosuite/mathutil/mathutil.go
        tool_calls_must_include:
          - cli_go
```

### Top-Level Fields

| Field | Purpose |
|------|---------|
| `spec.agent_name` | Agent preset used by the runner |
| `spec.manifest` | Manifest path relative to the repo/workspace |
| `spec.workspace.strategy` | Must be `derived` |
| `spec.workspace.template_profile` | Testsuite template profile copied into the temp `relurpify_cfg/` |
| `spec.workspace.exclude` | Globs omitted from the derived temp workspace |
| `spec.workspace.ignore_changes` | Paths ignored in change assertions |
| `spec.workspace.files` | Additional files layered onto the derived workspace before execution |
| `spec.models` | One or more model/endpoint combinations |
| `spec.recording` | Default tape mode/path for all cases |
| `spec.cases` | Individual prompts plus setup and expectations |

### Case Fields

| Field | Purpose |
|------|---------|
| `name` | Stable case identifier used in artifact directories |
| `description` | Optional human-readable note |
| `task_type` | Task classification hint |
| `prompt` | The user instruction sent to the agent |
| `context` | Extra task context such as `mode: debug` |
| `metadata` | Extra task metadata |
| `setup.files` | Files materialized before the case runs |
| `setup.git_init` | Initialize a git repository before execution |
| `requires.executables` | Skip unless required binaries exist |
| `requires.tools` | Skip unless required tools are registered |
| `expect` | Structured expectations for output, files, and tool calls |
| `overrides` | Per-case overrides for model, recording, env, allowed tools, or control flow |

`derived` is the required strategy. It copies the worktree into a temp run workspace, materializes a fresh `relurpify_cfg/` from a testsuite template profile, and then layers suite/case overrides on top.

### Expectations

| Field | What it checks |
|------|----------------|
| `must_succeed` | Case must complete without agent/runtime failure |
| `output_contains` | Final response contains all listed substrings |
| `output_regex` | Final response matches all listed regexes |
| `no_file_changes` | No workspace files changed |
| `files_changed` | Named files were modified |
| `tool_calls_must_include` | Required tool names were called |
| `tool_calls_must_exclude` | Forbidden tool names were not called |
| `max_tool_calls` | Hard cap on total tool invocations |

---

## Recording

Recording is configured in suite YAML, not through a dedicated CLI flag. The runner wraps the active model with a tape model when `spec.recording` or `case.overrides.recording` is set.

How it works:

```text
record mode:
  Real Ollama call -> response -> stored in tape file

replay mode:
  Tape file -> response -> no Ollama call needed
```

Modes:

| Mode | Behaviour |
|------|-----------|
| `off` | Normal live Ollama calls |
| `record` | Write responses to a tape file |
| `replay` | Replay from tape; no Ollama needed |

Example:

```yaml
spec:
  recording:
    mode: record
    tape: testsuite/agenttest_fixtures/coding.go.tape.jsonl
```

Run once with `mode: record`, then switch the same suite to `mode: replay` for deterministic reruns.

The TUI also exposes a recording mode toggle in Settings for interactive sessions.

---

## Sandbox And Workspace Isolation

You can isolate command execution with:

```bash
go run ./cmd/dev-agent agenttest run \
    --sandbox \
    --suite testsuite/agenttests/coding.go.testsuite.yaml
```

Tests already derive an isolated temp workspace by default. `strategy: derived` is the supported workspace mode.

---

## CI

Typical CI replay run:

```bash
go run ./cmd/dev-agent agenttest run \
    --suite testsuite/agenttests/coding.go.testsuite.yaml \
    --timeout 120s
```

For CI stability:

- commit suites with `recording.mode: replay`
- pin the model name in `spec.models`
- use `--ollama-reset` and `--ollama-reset-between` only when live Ollama runs are unavoidable

---

## See Also

- [Architecture](architecture.md) — how agenttest fits into the overall system
- [Agents](agents.md) — which agent manifests to test against
- [TUI](Relurpish_TUI.md) — recording mode toggle in Settings pane
