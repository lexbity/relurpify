# Testsuite

Agent-level integration tests for Relurpify. Each test runs a real agent against a real (or replayed) LLM and asserts on outputs, file mutations, state, and agent-internal metrics.

Unit tests (`go test ./...`) require no external services. Agent tests require Ollama running with `qwen2.5-coder:14b` loaded.

---

## Contents

1. [Quick Start](#quick-start)
2. [Suite Files](#suite-files)
3. [Running Tests](#running-tests)
4. [YAML Schema Reference](#yaml-schema-reference)
   - [Suite Header](#suite-header)
   - [Case Fields](#case-fields)
   - [Setup](#setup)
   - [Expect Assertions](#expect-assertions)
   - [Euclo Assertions](#euclo-assertions)
   - [Overrides](#overrides)
5. [Writing New Tests](#writing-new-tests)
6. [How Execution Works](#how-execution-works)
7. [Performance Baselines](#performance-baselines)
8. [testfu: Meta-Testing](#testfu-meta-testing)
9. [Troubleshooting](#troubleshooting)
10. [Quick Reference](#quick-reference)

---

## Quick Start

```bash
# Build
go build -o dev-agent ./app/dev-agent-cli

# Run all stable suites (requires Ollama)
./dev-agent agenttest run

# Run a specific suite
./dev-agent agenttest run --suite testsuite/agenttests/react.testsuite.yaml

# Run the fast live Euclo bug-hunting loop
./dev-agent agenttest run --suite testsuite/agenttests/euclo.rapid.testsuite.yaml --tier live-flaky

# Run all suites for one agent
./dev-agent agenttest run --agent euclo

# Run cases with a specific tag
./dev-agent agenttest run --tag coverage-matrix

# Run a single case by name
./dev-agent agenttest run --suite testsuite/agenttests/euclo.code.testsuite.yaml --case basic_edit_task

# Skip AST indexing (default: true — keeps runs fast)
./dev-agent agenttest run --skip-ast-index=false
```

---

## Suite Files

All canonical suites live in `testsuite/agenttests/`. The naming convention is
`{agent}.{mode}.testsuite.yaml` for mode-specific suites and `{agent}.testsuite.yaml`
for cross-cutting suites.

| Suite | Agent | Description |
|---|---|---|
| `react.testsuite.yaml` | react | ReAct paradigm: tool use, multi-step, error recovery |
| `planner.testsuite.yaml` | planner | HTN planner: plan generation and execution |
| `htn.testsuite.yaml` | htn | Hierarchical task network decomposition |
| `rewoo.testsuite.yaml` | rewoo | Reason-plan-act without observation loops |
| `reflection.testsuite.yaml` | reflection | Self-correction and reflection behavior |
| `euclo.code.testsuite.yaml` | euclo | Code mode: edit, verify, repair |
| `euclo.debug.testsuite.yaml` | euclo | Debug mode: reproduce, localize, patch |
| `euclo.review.testsuite.yaml` | euclo | Review mode: analysis and suggestions |
| `euclo.planning.testsuite.yaml` | euclo | Planning mode: structured plan generation |
| `euclo.classification.testsuite.yaml` | euclo | Mode auto-classification and disambiguation |
| `euclo.capability_interactions.testsuite.yaml` | euclo | Capability composition and contract enforcement |
| `euclo.chat.testsuite.yaml` | euclo | Chat mode: ask, inspect, implement |
| `euclo.archaeology.testsuite.yaml` | euclo | Archaeology mode: explore → compile-plan → implement |
| `euclo.rapid.testsuite.yaml` | euclo | Fast live-model bug hunting across debug + archaeology |
| `euclo.intent_fidelity.testsuite.yaml` | euclo | Intent recovery from conflicting/incomplete signals |
| `euclo.performance_context.testsuite.yaml` | euclo | Performance and context pressure baselines |
| `testfu.smoke.testsuite.yaml` | testfu | testfu agent self-tests: list, run-suite, tag-filter |
| `testfu.euclo.testsuite.yaml` | testfu | testfu-orchestrated euclo suite runs |
| `testfu.paradigm.testsuite.yaml` | testfu | Full paradigm sweeps via testfu |

### Tiers and Lanes

Every suite declares a `tier` and cases can declare `tags`. These control which runs
include them:

| Tier | Meaning |
|---|---|
| `smoke` | Fast, always-run sanity checks |
| `stable` | Default — included in standard CI runs |
| `live-flaky` | Known to be LLM-non-deterministic; excluded from gates |
| `quarantined` | Explicitly broken; excluded unless `--include-quarantined` |

| Lane | Cases included |
|---|---|
| `pr-smoke` | `tier: smoke` + `tag: smoke` |
| `merge-stable` | `tier: stable` |
| `quarantined-live` | `tier: quarantined` |

Tags used across suites:

| Tag | Cases |
|---|---|
| `coverage-matrix` | Representative cross-mode behavioral coverage |
| `performance-baseline` | Cases with committed baseline files |
| `smoke` | Fast, high-signal subset of a suite |
| `level:1` | Local repair — single function, obvious fix |
| `level:2` | Scoped implementation — multi-file or clear spec |
| `level:3` | Intent recovery — conflicting/incomplete signals |
| `level:4` | Architectural alignment — cross-cutting patterns |

```bash
# Run by tag
./dev-agent agenttest run --tag level:1        # 19 cheap cases, fast smoke
./dev-agent agenttest run --tag level:3        # intent fidelity regression
./dev-agent agenttest run --tag performance-baseline
```

---

## Running Tests

### CLI Flags

```
--suite <path>              Run a specific .testsuite.yaml file
--agent <name>              Run all {name}.testsuite.yaml + {name}.*.testsuite.yaml files
--case <name>               Run only this case (requires --suite)
--tag <tag>[,<tag>...]      Filter to cases with matching tags (comma-separated, OR logic)
--lane <lane>               pr-smoke | merge-stable | quarantined-live
--profile <profile>         Override execution profile (ci-live, ci-replay, live, replay)
--strict                    Enable strict assertion mode
--timeout <duration>        Per-case timeout (e.g., 90s, 2m)
--model <name>              Override model for all cases
--endpoint <url>            Override LLM endpoint

--skip-ast-index            Skip AST indexing (default: true)
--bootstrap-timeout <dur>   Timeout for agent initialization (default: 30s)
--max-iterations <n>        Override max agent iterations
--debug-llm                 Verbose LLM request/response logging
--debug-agent               Verbose agent state logging
--output-dir <path>         Where to write run artifacts

--ollama-reset none|model|server   Reset strategy on failure
--ollama-reset-between             Reset before each case
--ollama-reset-on <regex>          Trigger reset+retry when error matches pattern (repeatable)
--ollama-bin <path>                Path to ollama binary (default: ollama)
--ollama-service <name>            systemd service name for server restarts (default: ollama)

--include-quarantined       Include quarantined suites
--tier <tier>               Filter by tier
--max-retries <n>           Retries per case (-1 = 0, default: 3)
```

### Common Invocations

```bash
# Fast: level:1 smoke run (~19 cases)
./dev-agent agenttest run --tag level:1 --timeout 90s

# Rapid Euclo iteration against a live Ollama model
./dev-agent agenttest run --suite testsuite/agenttests/euclo.rapid.testsuite.yaml --tier live-flaky

# Or filter the rapid cases across suites
./dev-agent agenttest run --agent euclo --tag rapid-iteration --tier live-flaky

# CI gate: all stable euclo cases
./dev-agent agenttest run --agent euclo --profile ci-live

# Unload model between cases (reduces OOM risk)
./dev-agent agenttest run --agent euclo --ollama-reset model --ollama-reset-between

# Auto-retry on timeout
./dev-agent agenttest run \
  --agent euclo \
  --ollama-reset server \
  --ollama-reset-on "context deadline exceeded" \
  --timeout 120s

# Reproducible GOCACHE run
GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache \
  ./dev-agent agenttest run --agent react
```

### Output Artifacts

Each run writes to `relurpify_cfg/test_runs/{agent}/{run_id}/`:

```
report.json                     # SuiteReport — overall results
artifacts/{case}__{model}/
  tape.jsonl                    # LLM interaction tape (if recording enabled)
  context.snapshot.json         # Full task context state at completion
  token_usage.json              # Prompt/completion/total tokens + LLM call count
  framework_perf.json           # Framework hot-path counters (rescans, reads, etc.)
  phase_metrics.json            # Per-phase duration, tokens, LLM calls
  memory_outcome.json           # Memory/workflow record counts
  changed_files.json            # Files modified during the case
  performance_warnings.json     # Baseline comparison results (if baseline exists)
  model.provenance.json         # Model name, digest, source
tmp/{case}__{model}/workspace/  # Isolated workspace used during execution
logs/{case}__{model}.log        # Full execution log
```

---

## YAML Schema Reference

### Suite Header

```yaml
apiVersion: relurpify/v1alpha1    # required, literal
kind: AgentTestSuite              # required, literal

metadata:
  name: my.suite                  # required — used in CLI --agent filter and tape paths
  description: "..."              # optional
  owner: team-name                # optional
  tier: stable                    # smoke | stable | live-flaky | quarantined
  quarantined: false              # true disables the entire suite

spec:
  agent_name: euclo               # required — agent to run cases against
  manifest: relurpify_cfg/agent.manifest.yaml  # required — agent manifest path

  execution:
    profile: ci-live              # live | record | replay | developer-live | ci-live | ci-replay
    strict: true                  # auto-enabled for ci-* profiles
    timeout: 90s                  # default per-case timeout; overridable per case

  workspace:
    strategy: derived             # only "derived" is supported
    exclude:                      # glob patterns excluded when copying workspace
      - .git/**
      - .gocache/**
      - .gomodcache/**
      - relurpify_cfg/test_runs/**

  models:
    - name: qwen2.5-coder:14b    # model identifier
      endpoint: http://localhost:11434  # Ollama endpoint

  recording:
    mode: off                     # off | record | replay
    strategy: live                # live | replay-if-golden | replay-only
    # tape: path/to/tape.jsonl   # auto-generated if omitted
```

### Case Fields

```yaml
cases:
  - name: my_case                 # required — unique within suite
    description: "..."            # optional
    task_type: code_modification  # hint: analysis | code_modification (not enforced)
    tags: [stable, level:1]       # optional — for CLI tag filtering
    timeout: 120s                 # overrides suite execution.timeout

    prompt: >
      The instruction given to the agent. Supports multi-line YAML.

    context:                      # optional — becomes task.Context map
      euclo.mode: code            # force euclo mode (code|debug|review|planning|chat|archaeology)
      euclo.interaction_state:    # seed prior phase state for resume cases
        mode: archaeology
        current_phase: compile-plan
        phases_executed: [explore]
        phase_states:
          explore.done: true
      workflow_id: wf-123         # passed to agent as context
      context_file_contents:      # inline file content for agent context (no disk read needed)
        - path: path/to/file.go
          content: |
            package main
            ...

    requires:                     # optional — skip case if requirements unmet
      executables: [git, docker]  # required system binaries
      tools: [file_write]         # required agent tools

    setup:                        # optional — pre-case state initialization
      # (see Setup section below)

    expect:                       # required — assertions evaluated after execution
      # (see Expect Assertions section below)

    overrides:                    # optional — per-case runtime config
      # (see Overrides section below)
```

### Setup

```yaml
setup:
  # Files to create in the isolated workspace before the case runs
  files:
    - path: testsuite/fixtures/myfile.go   # relative to workspace root
      content: |
        package main
        func Foo() {}
      mode: "0644"                         # optional, default 0644

  # Initialize a git repository in the workspace
  git_init: false

  # Pre-populate the agent's memory store
  memory:
    declarative:
      - record_id: my-fact
        kind: fact
        title: "Some context"
        content: "The value of X is 42."
        status: accepted
    procedural:
      - routine_id: my-routine
        kind: pattern
        name: "Error handling convention"
        description: "Always return (T, error)"
        inline_body: |
          func Foo() (string, error) { return "", nil }

  # Pre-populate workflow state (for archaeology/resume cases)
  workflows:
    - workflow:
        workflow_id: wf-my-workflow
        task_type: code_modification
        instruction: "Add error wrapping"
        status: running
      runs:
        - run_id: my-run
          status: running
      knowledge:
        - record_id: exploration-result
          kind: fact
          title: "Prior exploration"
          content: "Package has 2 files. Types: Foo (string), Bar (int)."
          status: accepted
      checkpoints:
        - checkpoint_id: phase-checkpoint-1
          task_id: task-123
          stage_name: explore
          stage_index: 0
          context_state:
            euclo.interaction_state:
              mode: archaeology
              phases_executed: [explore]
          result_data:
            summary: "Exploration complete"
```

### Expect Assertions

All assertions in `expect:` are hard failures — none are silently skipped. Multiple
failures are concatenated and reported together.

```yaml
expect:
  # ── Overall result ────────────────────────────────────────────────────────
  must_succeed: true           # agent must complete without error
                               # if false: the case passes even if agent fails

  # ── Output text ───────────────────────────────────────────────────────────
  output_contains:             # all substrings must appear in agent's output
    - "error wrapping"
    - "fmt.Errorf"

  output_regex:                # all patterns must match (re2 syntax, flags inline)
    - (?i)(error|wrapping)     # case-insensitive
    - "fmt\\.Errorf"

  # ── File mutations ────────────────────────────────────────────────────────
  files_changed:               # glob patterns — at least one file must match each
    - testsuite/fixtures/calc.go
    - "testsuite/fixtures/*.go"

  no_file_changes: true        # no files may be modified (mutually exclusive with files_changed)

  files_contain:               # workspace files must contain these substrings
    - path: testsuite/fixtures/calc.go
      contains:
        - "a + b"
        - "return result"

  # ── Tool calls ────────────────────────────────────────────────────────────
  tool_calls_must_include:     # these tools must be called at least once
    - file_read
    - file_write

  tool_calls_must_exclude:     # these tools must NOT be called
    - shell_exec

  tool_calls_in_order:         # these tools must be called in this order (non-contiguous ok)
    - file_read
    - file_write

  max_tool_calls: 10           # total tool invocations must not exceed this

  # ── Token / LLM call limits ───────────────────────────────────────────────
  llm_calls: 3                 # exact number of LLM API calls required
  max_prompt_tokens: 8000      # prompt tokens must not exceed
  max_completion_tokens: 2000  # completion tokens must not exceed
  max_total_tokens: 10000      # total tokens must not exceed

  # ── State and memory ──────────────────────────────────────────────────────
  state_keys_must_exist:       # these keys must be set in task context after execution
    - euclo.interaction_state
    - euclo.artifacts
    - testfu.report

  memory_records_created: 1    # at least this many memory records written
  workflow_state_updated: true # workflow state must be modified

  # ── Euclo-specific ────────────────────────────────────────────────────────
  euclo:
    # (see Euclo Assertions section below)
```

### Euclo Assertions

```yaml
euclo:
  # Mode and profile
  mode: code                            # resolved mode must match
  profile: edit_verify_repair           # resolved execution profile must match

  # Phase tracking
  phases_executed:                      # all listed phases must have run
    - intent
    - propose
    - verify
  phases_skipped:                       # all listed phases must have been skipped
    - explore

  # Artifact production
  artifacts_produced:                   # these artifact kinds must be present
    - exploration                       # normalized: "exploration" → "euclo.explore"
    - plan                              # normalized: "plan" → "euclo.plan"
    - verification                      # normalized: "verification" → "euclo.verification"

  # Artifact chain (producer → consumer relationships)
  artifact_chain:
    - kind: exploration
      produced_by_phase: explore
      consumed_by_phase: compile-plan
      content_contains:
        - "types.go"

  # Recovery
  recovery_attempted: true              # recovery mechanism must have triggered
  recovery_strategies:                  # these strategy IDs must appear in recovery trace
    - fallback_path

  # Interaction frames (euclo.interaction_recording)
  min_frames_emitted: 1
  max_frames_emitted: 10
  frame_kinds_emitted:                  # these frame kinds must appear
    - proposal
    - session_resume
  frame_kinds_must_exclude:             # these frame kinds must NOT appear
    - transition

  # State transitions
  min_transitions_proposed: 1
  max_transitions_proposed: 5
```

#### Artifact kind normalization

The `artifacts_produced` assertion normalizes short names to their canonical form:

| YAML value | Canonical kind |
|---|---|
| `exploration` | `euclo.explore` |
| `analysis` | `euclo.analyze` |
| `plan_candidates` | `euclo.plan_candidates` |
| `plan` | `euclo.plan` |
| `edit_intent` | `euclo.edit_intent` |
| `verification` | `euclo.verification` |
| Other | prefixed with `euclo.` if not already |

### Overrides

Per-case overrides take effect after suite-level config is resolved:

```yaml
overrides:
  max_iterations: 6            # cap agent iterations for this case

  # Restrict which capabilities the agent can use
  allowed_capabilities:
    - name: file_read
      kind: tool
    - name: file_write
      kind: tool
  restrict_capabilities: true  # if true, ONLY listed capabilities are available

  # Per-case model override
  model:
    name: qwen2.5-coder:7b
    endpoint: http://localhost:11434

  # Per-case recording override
  recording:
    mode: replay
    tape: testsuite/agenttests/tapes/my.suite/my_case__qwen2_5_coder_14b.tape.jsonl

  extra_env:
    MY_VAR: "value"             # environment variables for shell tool execution

  bootstrap_timeout: 60s       # agent initialization timeout for this case
```

---

## Writing New Tests

### Adding a case to an existing suite

1. Open the relevant `.testsuite.yaml` file.
2. Append a new entry to `spec.cases`.
3. Required fields: `name`, `prompt`, `expect`.
4. If the case modifies files, provide `setup.files` with the fixture content inline.
5. Use `context: euclo.mode: <mode>` to force a specific euclo mode rather than relying on auto-classification.

**Minimal code-modification case:**
```yaml
- name: my_new_case
  description: "What this tests."
  task_type: code_modification
  tags: [stable, level:1]
  setup:
    files:
      - path: testsuite/fixtures/my_fixture.go
        content: |
          package main

          func Broken() int { return 0 }
  prompt: "Fix Broken() in testsuite/fixtures/my_fixture.go to return 42."
  context:
    euclo.mode: code
    context_file_contents:
      - path: testsuite/fixtures/my_fixture.go
        content: |
          package main

          func Broken() int { return 0 }
  expect:
    must_succeed: true
    files_changed:
      - testsuite/fixtures/my_fixture.go
    files_contain:
      - path: testsuite/fixtures/my_fixture.go
        contains:
          - "return 42"
```

**Minimal analysis case:**
```yaml
- name: explain_something
  description: "Agent can explain what X is."
  task_type: analysis
  tags: [stable, level:1]
  prompt: "What does the Stringer interface do in Go?"
  context:
    euclo.mode: chat
  expect:
    must_succeed: true
    no_file_changes: true
    output_regex:
      - (?i)(string|format|method)
    state_keys_must_exist:
      - euclo.interaction_state
```

### Creating a new suite

1. Create `testsuite/agenttests/{agent}.{mode}.testsuite.yaml`.
2. Copy the suite header from an existing suite and update `metadata.name`, `spec.agent_name`, and `spec.manifest`.
3. Every committed suite must declare CI metadata explicitly:
   ```yaml
   metadata:
     tier: stable
     quarantined: false
   spec:
     execution:
       profile: ci-live
       strict: true   # or false for non-strict suites
   ```
4. Add the new suite as a smoke case in the corresponding `testfu.*.testsuite.yaml` if one exists.

### Tag conventions

- Add `level:1`–`level:4` tags to every case for filterable smoke runs.
- Add `coverage-matrix` to cases that represent the minimal behavioral coverage set.
- Add `smoke` to cases suitable for fast PR gates.
- Add `stable` to cases that are deterministic and should pass in CI.

### Fixture file paths

Use `testsuite/fixtures/` for small, case-specific fixtures inline in the YAML.
Use sub-directories to avoid name collisions across suites:

```
testsuite/fixtures/arch_pkg/      # archaeology suite fixtures
testsuite/fixtures/nav_pkg/       # performance navigation fixtures
testsuite/fixtures/intent_api.go  # intent fidelity fixtures
```

### context_file_contents vs setup.files

- `setup.files` writes files to disk before the case runs. Use this for cases where the agent needs to *read* files via file tools.
- `context.context_file_contents` injects file content directly into the agent's context map (no disk read needed). Use this when you want the agent to work from its context budget rather than a tool call.
- Both can be used together: `setup.files` writes the ground truth, `context.context_file_contents` provides the initial view.

---

## How Execution Works

### 1. Workspace isolation

Each case runs in a freshly derived isolated workspace:

1. **Copy** — the target workspace is copied to a temp directory, excluding patterns in `spec.workspace.exclude`.
2. **Overlay** — `setup.files` entries are written into the temp workspace.
3. **Config** — a minimal `relurpify_cfg/` is materialized with the agent manifest.
4. **Snapshot** — a SHA-256 hash of every file is recorded before execution starts.
5. **Execute** — the agent runs against the isolated workspace.
6. **Diff** — file hashes are recomputed; changed files are determined by hash delta.

All assertions that reference file paths resolve against the isolated workspace.

### 2. Agent bootstrap

The runner calls the standard bootstrap path (`BootstrapAgentRuntime`) with the manifest from `spec.manifest`. This:
- Resolves the agent definition from the manifest
- Registers capabilities (tools, relurpic, MCP)
- Applies the effective capability policy
- Connects to Ollama at the declared endpoint

`--skip-ast-index=true` (default) skips the AST/code-index warmup, making runs ~5–10s faster per case.

### 3. Assertion evaluation

After execution, assertions are evaluated in this order:

1. `output_contains`, `output_regex` — matched against agent's final output string
2. `files_contain`, `files_changed`, `no_file_changes` — matched against workspace diff
3. `tool_calls_must_include/exclude`, `tool_calls_in_order`, `max_tool_calls` — from telemetry events
4. `llm_calls`, `max_*_tokens` — from token usage telemetry
5. `memory_records_created`, `workflow_state_updated` — from persistence store outcome
6. `state_keys_must_exist` — nested path lookup into task context snapshot
7. `euclo.*` — read from euclo state keys in context snapshot
8. `must_succeed` — evaluated last; if false, agent failure is allowed

All failures accumulate — a single run reports every failing assertion, not just the first.

### 4. Failure classification

| `FailureKind` | Meaning |
|---|---|
| `"infra"` | Timeout, connection refused, missing file, Ollama error |
| `"assertion"` | An `expect:` assertion was not satisfied |
| `"agent"` | Agent returned a non-success result |
| `""` | Success |

Infrastructure failures trigger the retry loop (up to `--max-retries`, default 3). Assertion failures do not retry.

### 5. Recording and replay

When `recording.mode: record` is set (or profile is `record`/`ci-replay`), all LLM
interactions are written to a tape file. When `mode: replay`, the runner reads from the
tape instead of hitting Ollama — making runs fully deterministic and offline.

**Tape path pattern:**
```
testsuite/agenttests/tapes/{suite_name}/{case_name}__{sanitized_model}.tape.jsonl
```

**Strategy options:**

| Strategy | Behavior |
|---|---|
| `replay-if-golden` | Replay if tape exists, otherwise run live |
| `replay-only` | Fail if tape is missing |
| `live` (default) | Always run live |

### 6. Context seeding for euclo cases

The `context:` block maps directly to `task.Context`. The euclo agent reads specific
keys before execution:

| Context key | Effect |
|---|---|
| `euclo.mode` | Forces a specific interaction mode (bypasses auto-classification) |
| `euclo.interaction_state` | Seeds prior phase checkpoint (for resume cases) |
| `euclo.interaction_script` | Provides scripted interaction steps |
| `workflow_id` | Wires the run to a pre-seeded workflow record |
| `context_file_contents` | Injects file content into agent context budget |

---

## Performance Baselines

Cases tagged `performance-baseline` are compared against committed baseline JSON files
after execution. A warning (not a failure) is emitted when actual metrics materially
exceed the baseline.

### Baseline file path

```
testsuite/agenttests/tapes/{suite_name}/{case_name}__{sanitized_model}.baseline.json
```

Name sanitization: all non-alphanumeric characters become `_`, leading/trailing `_` stripped.
Example: `qwen2.5-coder:14b` → `qwen2_5_coder_14b`

### Baseline format

```json
{
  "model": "qwen2.5-coder:14b",
  "recorded_at": "2026-03-31",
  "llm_calls": 4,
  "total_tokens": 3500,
  "duration_ms": 45000,
  "phases": {
    "intent":   { "llm_calls": 1, "tokens": 800 },
    "propose":  { "llm_calls": 2, "tokens": 2000 },
    "verify":   { "llm_calls": 1, "tokens": 700 }
  },
  "framework": {
    "ContextBudgetRescanCount": 1,
    "ProgressiveFileReadCount": 2
  }
}
```

### Comparison thresholds

| Metric | Multiplier | Example |
|---|---|---|
| LLM calls | 1.5× | baseline 4 → warn if actual > 6 |
| Total tokens | 2.0× | baseline 3500 → warn if actual > 7000 |
| Duration | 3.0× | baseline 45s → warn if actual > 135s |

Framework counter comparisons use the same 1.5× multiplier.

### Recording a new baseline

Run the case once to get actual metrics, then write the baseline:

```bash
./dev-agent agenttest run \
  --suite testsuite/agenttests/euclo.performance_context.testsuite.yaml \
  --case performance_large_file_navigation

# Check the output artifacts
cat relurpify_cfg/test_runs/euclo/*/artifacts/performance_large_file_navigation__qwen2_5_coder_14b/token_usage.json
cat relurpify_cfg/test_runs/euclo/*/artifacts/performance_large_file_navigation__qwen2_5_coder_14b/framework_perf.json

# Write the baseline at the expected path
# testsuite/agenttests/tapes/{suite_name}/{case}__{model}.baseline.json
```

---

## testfu: Meta-Testing

`testfu` is a named agent that runs other agent test suites programmatically. It is used
to compose suite runs, apply tag filters across multiple suites, and produce structured
pass/fail reports from within the agent framework itself.

### Context keys consumed by testfu

| Key | Value | Effect |
|---|---|---|
| `action` | `run_suite` | Run a specific suite file |
| `action` | `run_case` | Run a single case from a suite |
| `action` | `list_suites` | List all discoverable suites |
| `suite_path` | path | Suite to run (with `run_suite` or `run_case`) |
| `case_name` | name | Case to run (with `run_case`) |
| `agent_name` | name | Glob all suites for this agent (replaces `suite_path`) |
| `tags` | `tag1,tag2` | Filter cases by tag (OR logic) |
| `timeout` | `60s` | Per-case timeout passed to inner runs |

### State keys written by testfu

| Key | Type | Content |
|---|---|---|
| `testfu.report` | object | SuiteReport for the inner run |
| `testfu.passed` | bool | Whether all cases passed |
| `testfu.failed_cases` | []string | Names of failed cases |
| `testfu.agent_suites_report` | map | Per-suite SuiteReports (for `agent_name` runs) |
| `testfu.total_passed` | int | Total cases passed across all suites |
| `testfu.total_failed` | int | Total cases failed across all suites |
| `testfu.total_skipped` | int | Total cases skipped across all suites |

### Example testfu case

```yaml
- name: euclo_code_smoke
  tags: [stable, euclo]
  task_type: analysis
  timeout: "600s"
  prompt: "Run the euclo code test suite."
  context:
    action: run_suite
    suite_path: testsuite/agenttests/euclo.code.testsuite.yaml
    timeout: 90s
  expect:
    must_succeed: true
    no_file_changes: true
    state_keys_must_exist:
      - testfu.report
      - testfu.passed
```

### Running all suites for an agent via testfu

```yaml
- name: paradigm_euclo
  task_type: analysis
  timeout: "900s"
  context:
    agent_name: euclo
    timeout: 90s
  expect:
    state_keys_must_exist:
      - testfu.agent_suites_report
      - testfu.total_passed
      - testfu.total_failed
```

The `agent_name` key causes testfu to glob:
- `{workspace}/testsuite/agenttests/euclo.testsuite.yaml`
- `{workspace}/testsuite/agenttests/euclo.*.testsuite.yaml`

Time is distributed across suites using a budgeted allocation:
`per_suite_timeout = remaining_time / pending_suites` (floor: 30s).

---

## Troubleshooting

### "connection refused" / "dial tcp"
Ollama is not running. Start it: `ollama serve` or `systemctl start ollama`.

### "no such model" / model load error
Model is not pulled: `ollama pull qwen2.5-coder:14b`.

### Case times out
- Increase `--timeout`: `./dev-agent agenttest run --timeout 180s`
- Or set per-case: `timeout: 180s` in the case YAML.
- Add `--ollama-reset model --ollama-reset-between` to ensure a clean model state.

### Assertion fails with "expected changed file X but no files changed"
The agent read the file from `context.context_file_contents` and never called a write
tool. Either remove `context_file_contents` so the agent must use `file_read`, or
ensure the prompt clearly directs the agent to write the file.

### "assertion: tape exhausted" (replay mode)
The tape has fewer LLM exchanges than the current execution needs. Re-record:
set `recording.mode: record`, run once, commit the new tape, then switch back to `replay`.

### File changes not detected
If the agent writes to a path that matches a `workspace.ignore_changes` pattern or
`workspace.exclude` pattern, it won't appear in `ChangedFiles`. Check those patterns.

### Euclo mode assertion fails ("expected mode: X, got: Y")
Mode is auto-classified from the prompt. Use `context: euclo.mode: X` to force it.

### State key assertion fails
The state key may be written under a different path. Check `context.snapshot.json` in the
run artifacts to see what keys were actually set.

---

## Quick Reference

### Levels tag meanings

| Tag | Definition |
|---|---|
| `level:1` | Single function, obvious fix, minimal context needed |
| `level:2` | Multi-file or structured spec, clear implementation path |
| `level:3` | Conflicting/incomplete signals — agent must infer correct intent |
| `level:4` | Cross-cutting patterns — agent must follow architectural conventions |

### Euclo modes

| Mode | When used |
|---|---|
| `code` | Direct code modification tasks |
| `debug` | Reproducing and fixing failures |
| `review` | Analysis and code review |
| `planning` | Architecture and migration planning |
| `chat` | Q&A, explanation, small inline implementations |
| `archaeology` | Explore → compile-plan → implement-plan workflow |

### Key euclo context state keys

| Key | Written by | Contains |
|---|---|---|
| `euclo.interaction_state` | orchestrator | `mode`, `phases_executed`, `current_phase` |
| `euclo.artifacts` | orchestrator | array of `{kind, summary, payload}` |
| `euclo.interaction_recording` | recording | `{frames: [], transitions: []}` |
| `euclo.mode_resolution` | classifier | `{mode_id: "code"}` |
| `euclo.execution_profile_selection` | classifier | `{profile_id: "edit_verify_repair"}` |
| `euclo.recovery_trace` | recovery | `{attempts: [{strategy, level}]}` |

### Name sanitization (for tape/baseline paths)

Non-alphanumeric characters → `_`, then leading/trailing `_` stripped.

```
qwen2.5-coder:14b  →  qwen2_5_coder_14b
my_case_name       →  my_case_name
my.case.name       →  my_case_name
```
