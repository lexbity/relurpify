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
go run ./app/dev-agent-cli agenttest run
```

## Live Coverage Targets

The live `dev-agent agenttest` layer is intended to prove agent behavior against
the real runtime surface, not just prompt formatting or isolated tool stubs.

The acceptance target for the canonical live suite catalog is:

- every shipped primary agent implementation has at least one canonical live suite
- every shipped `CodingAgent` mode has at least one canonical live suite
- read/write/execute flows are covered for the main language manifests
- pipeline and architect control-flow paths both have live coverage
- workspace lifecycle features such as derived workspaces, fixture layering, and `git_init` are exercised by at least one suite
- replay-backed suites exist for the stable CI subset, even when broader coverage also runs live

Newer framework features must also be covered at the live agenttest layer, not
only by unit tests.

Current target areas:

- retrieval-backed runtime memory using the SQLite runtime memory store
- workflow retrieval hydration for planner, architect, and pipeline flows
- browser-driven execution against fixture-backed pages
- persistence and recall across repeated runs or resumed workflows
- retrieval and memory provenance surfacing in the agent-visible prompt/output path

### Acceptance Criteria For New Harness Work

The live harness work is considered sufficient when all of the following are
true:

- suites can opt into the newer SQLite-backed runtime memory/retrieval path instead of always using `HybridMemory`
- suites can seed runtime memory, workflow knowledge, and retrieval-relevant state before execution
- expectations can assert retrieval-specific outcomes such as hydrated workflow retrieval state, citations, or persisted retrieval artifacts
- at least one canonical live suite proves runtime memory retrieval through actual agent execution
- at least one canonical live suite proves workflow retrieval hydration in an architect or pipeline execution path
- at least one canonical live suite proves a browser-driven flow against local fixture content
- a replay-capable subset exists for CI so newer feature coverage is not live-only

### Current Non-Goals

The live agenttest layer does not need to replace fine-grained framework unit
tests. In particular, it is not the right place to exhaustively validate:

- retrieval ranking math or compaction internals
- low-level SQLite schema migrations in isolation
- browser backend protocol edge cases across every transport
- every possible model-dependent phrasing outcome

Those remain package-level test responsibilities. Agenttests should instead
prove that these features are reachable and functional through the real agent
runtime.

### Prerequisites

For live runs, Ollama must be available with the target model pulled:

```bash
ollama pull qwen2.5-coder:14b
```

### Running

```bash
# Run every discovered suite
go run ./app/dev-agent-cli agenttest run

# Run one suite explicitly
go run ./app/dev-agent-cli agenttest run \
    --suite testsuite/agenttests/coding.go.testsuite.yaml

# Filter discovered suites by agent prefix and raise timeout
go run ./app/dev-agent-cli agenttest run \
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
  memory:
    backend: sqlite_runtime
    retrieval:
      embedder: test
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
        memory:
          declarative:
            - record_id: fact-1
              scope: project
              kind: project-knowledge
              summary: Prior retrieval-backed memory
              content: Runtime memory is mirrored into retrieval tables.
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
        state_keys_must_exist:
          - react.final_output
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
| `spec.memory` | Optional live memory backend configuration (`hybrid` or `sqlite_runtime`) |
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
| `browser_fixtures` | Local fixture routes served before execution and injected into task context |
| `setup.files` | Files materialized before the case runs |
| `setup.git_init` | Initialize a git repository before execution |
| `setup.memory` | Seed runtime memory before execution |
| `setup.workflows` | Seed workflow/run/knowledge records before execution |
| `requires.executables` | Skip unless required binaries exist |
| `requires.tools` | Skip unless required tools are registered |
| `expect` | Structured expectations for output, files, and tool calls |
| `overrides` | Per-case overrides for model, recording, memory backend, env, allowed tools, or control flow |

`derived` is the required strategy. It copies the worktree into a temp run workspace, materializes a fresh `relurpify_cfg/` from a testsuite template profile, and then layers suite/case overrides on top.

Browser-backed cases can define `browser_fixtures` with inline HTML or fixture
files. The harness injects fixture URLs into `task.Context.browser_fixtures`.
For browser automation these URLs are rewritten to container-reachable
`host.docker.internal` addresses, and the original host-loopback URLs remain
available under `local_base_url` and `local_urls`.

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
| `state_keys_must_exist` | Named context state keys were present at completion |

### Memory Backends

The live runner supports two memory backends:

```yaml
spec:
  memory:
    backend: hybrid
```

or:

```yaml
spec:
  memory:
    backend: sqlite_runtime
    retrieval:
      embedder: test
```

Notes:

- `hybrid` is the backward-compatible default
- `sqlite_runtime` provisions `relurpify_cfg/memory/runtime_memory.db`
- `retrieval.embedder: test` enables a deterministic test embedder for dense retrieval coverage in agenttests

### Seeding Runtime Memory

Suites can seed runtime memory before the baseline workspace snapshot:

```yaml
setup:
  memory:
    declarative:
      - record_id: fact-1
        scope: project
        kind: project-knowledge
        title: Retrieval mirror
        summary: Declarative memory should be searchable.
        content: The runtime store mirrors declarative records into retrieval.
    procedural:
      - routine_id: routine-1
        scope: project
        kind: capability-composition
        name: checkpoint-and-summarize
        summary: Reusable verification routine
```

`procedural` seeds require the `sqlite_runtime` backend.

### Seeding Workflow Retrieval State

Suites can also seed workflow history directly into the workflow state store:

```yaml
setup:
  workflows:
    - workflow:
        workflow_id: wf-1
        instruction: Use retrieval-backed planning context
      runs:
        - run_id: seed-run
      knowledge:
        - record_id: knowledge-1
          kind: decision
          title: Prior decision
          content: Use retrieval-backed planning context.
```

This is intended for architect and pipeline cases that should hydrate
`planner.workflow_retrieval`, `architect.workflow_retrieval`, or
`pipeline.workflow_retrieval`.

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
go run ./app/dev-agent-cli agenttest run \
    --sandbox \
    --suite testsuite/agenttests/coding.go.testsuite.yaml
```

Tests already derive an isolated temp workspace by default. `strategy: derived` is the supported workspace mode.

---

## CI

Typical CI replay run:

```bash
go run ./app/dev-agent-cli agenttest run \
    --suite testsuite/agenttests/coding.go.testsuite.yaml \
    --timeout 120s
```

For CI stability:

- commit suites with `recording.mode: replay`
- pin the model name in `spec.models`
- use `--ollama-reset` and `--ollama-reset-between` only when live Ollama runs are unavoidable

---
