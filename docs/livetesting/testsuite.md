# Testsuite Package

The `testsuite/` tree is the repository’s YAML-driven live integration and baseline harness for agents. It exercises complete agent workflows against the real runtime stack, then records structured artifacts that can be used for regression checks, performance baselines, and debugging.

This document explains:

- what the testsuite package covers
- how the runner executes a case end to end
- what the different suite families are used for
- what is benchmarked
- how to run the suites locally and in CI

The canonical suite authoring guide remains [`testsuite/README.md`](/home/lex/Public/Relurpify/testsuite/README.md). This document is the technical overview.

---

## Purpose

The testsuite package exists to validate agent behavior through the real orchestration path rather than through isolated mocks.

It answers questions like:

- does the agent choose the right mode or execution profile?
- does it call the right tools in the right situations?
- does it mutate the workspace when mutation is intended?
- does it avoid mutation when the case is analysis-only?
- does it surface the expected runtime state and artifacts?
- does it stay within acceptable iteration and token budgets?

The package is intentionally integration-heavy. Unit tests cover framework internals; testsuites prove that the framework, runtime, capabilities, tools, and model interaction all compose correctly.

---

## What It Tests

The testsuite package currently covers four broad areas:

### 1. Agent behavior

These cases validate that agents solve real tasks, not just that they format prompts correctly.

Examples:

- code repair and verification loops
- debug localization and patching
- review and inspection
- planning and staged execution
- archaeology-style exploration and plan compilation

### 2. Capability selection and routing

Many cases assert on the agent’s runtime selection state:

- resolved mode
- selected execution profile
- behavior family
- primary relurpic capability
- supporting capabilities
- recipe IDs

This is especially important for Euclo, where a single top-level agent can route into different relurpic capabilities and execution families depending on mode, profile, and task signals.

### 3. Tool calling

Suites can assert:

- that particular tools were called
- that certain tools were not called
- that tool calls stayed under a cap
- that tool calls occurred in a required order when the flow is deterministic enough

This is the main mechanism for checking whether an agent can actually use the platform surface.

### 4. Runtime artifacts and state

Cases may assert on:

- changed files
- output text or regex matches
- context keys
- workflow state updates
- memory record creation
- Euclo-specific proof surface fields
- performance baseline artifacts

---

## What It Benchmarks

The testsuite package also carries performance-oriented checks, not just correctness checks.

Current benchmark-style signals include:

- prompt token count
- completion token count
- total token count
- LLM call count
- total run duration
- phase duration
- per-case tool invocation counts
- framework hot-path counters
- baseline warnings when performance regresses

The performance baseline cases use committed baseline files and compare current results against the baseline during execution. The resulting artifacts are written alongside the rest of the case output.

This means the testsuite package is doing two jobs at once:

- correctness regression testing
- lightweight end-to-end performance characterization

---

## Suite Families

Canonical suites live in `testsuite/agenttests/` and follow the naming convention `{agent}.{mode}.testsuite.yaml` for mode-specific suites.

Important Euclo suites:

- `euclo.code.testsuite.yaml`
- `euclo.debug.testsuite.yaml`
- `euclo.review.testsuite.yaml`
- `euclo.planning.testsuite.yaml`
- `euclo.chat.testsuite.yaml`
- `euclo.archaeology.testsuite.yaml`
- `euclo.rapid.testsuite.yaml`
- `euclo.performance_context.testsuite.yaml`

The rapid family is the live-model validation slice. It is split into mode-specific suites:

- `euclo.rapid.chat.testsuite.yaml`
- `euclo.rapid.debug.testsuite.yaml`
- `euclo.rapid.archaeology.testsuite.yaml`

The legacy aggregate `euclo.rapid.testsuite.yaml` is still available for compatibility, but the split rapid suites are the canonical live passes.

---

## How It Works

At a high level, a case runs through the following pipeline:

1. The runner reads the YAML suite and case definition.
2. A derived workspace is created.
3. Files from `setup.files` are materialized into the workspace.
4. Optional memory, workflow, and git state seeds are applied.
5. The agent is invoked against the selected model and endpoint.
6. The agent calls tools through the real runtime surface.
7. The runner records output, state, file changes, tool calls, and artifacts.
8. Expectations are evaluated against the final result.
9. A run directory is written under `relurpify_cfg/test_runs/{agent}/{run_id}/`.

### Workspace model

The runner does not operate in the live checkout directly. Instead, it clones a derived workspace and layers a testsuite-specific `relurpify_cfg/` into that workspace.

That means:

- the test run is isolated from the developer’s live worktree
- file changes are captured as case artifacts
- run output is reproducible without polluting the source checkout

### Expectation model

Expectations are structural, not string-equality based.

The runner can verify:

- success or failure
- file mutations
- tool usage
- state keys
- memory writes
- workflow tension state
- Euclo runtime properties
- output content and regexes

This is important because agent output is non-deterministic. The testsuite is designed to validate stable behavior contracts, not exact phrasing.

---

## Euclo Baseline Model

Euclo suites are the most specialized part of the testsuite package.

Euclo cases typically assert that the agent selected the right:

- mode
- execution profile
- behavior family
- primary relurpic capability
- supporting relurpic capabilities
- recipe IDs

For example:

- debug cases should usually resolve to `reproduce_localize_patch` and `stale_assumption_detection`
- chat cases should usually resolve to `direct_change_execution`
- archaeology cases in planning mode should usually resolve to `gap_analysis`

That distinction matters because Euclo is not just a single behavior. It is a named-agent shell that composes relurpic capabilities, framework policy, and execution profiles.

The rapid Euclo suites were tightened specifically to be a baseline for capability validation, not a transcript of one transient model trace.

---

## Output Artifacts

Each case writes a structured run directory under:

`relurpify_cfg/test_runs/{agent}/{run_id}/`

Common artifacts include:

- `report.json`
- `artifacts/{case}__{model}/tape.jsonl`
- `artifacts/{case}__{model}/context.snapshot.json`
- `artifacts/{case}__{model}/token_usage.json`
- `artifacts/{case}__{model}/framework_perf.json`
- `artifacts/{case}__{model}/phase_metrics.json`
- `artifacts/{case}__{model}/memory_outcome.json`
- `artifacts/{case}__{model}/changed_files.json`
- `artifacts/{case}__{model}/performance_warnings.json`
- `artifacts/{case}__{model}/model.provenance.json`
- `logs/{case}__{model}.log`
- `tmp/{case}__{model}/workspace/`

These artifacts are the basis for debugging failed runs and for comparing current behavior against baselines.

---

## Run Modes

The suite supports live and replay-oriented execution through the dev-agent CLI.

Typical execution profiles:

- `ci-live`
- `ci-replay`
- `live`
- `replay`

The suite itself can be used in:

- deterministic unit-style verification of runner behavior
- live model regression passes
- replayed runs when a tape exists

---

## How To Run

### Build the CLI

```bash
go build -o dev-agent ./app/dev-agent-cli
```

### Run all suites

```bash
./dev-agent agenttest run
```

### Run a specific suite

```bash
./dev-agent agenttest run --suite testsuite/agenttests/euclo.debug.testsuite.yaml
```

### Run the rapid Euclo live baseline

```bash
./dev-agent agenttest run --suite testsuite/agenttests/euclo.rapid.chat.testsuite.yaml --tier live-flaky
./dev-agent agenttest run --suite testsuite/agenttests/euclo.rapid.debug.testsuite.yaml --tier live-flaky
./dev-agent agenttest run --suite testsuite/agenttests/euclo.rapid.archaeology.testsuite.yaml --tier live-flaky
```

### Run the legacy aggregate rapid suite

```bash
./dev-agent agenttest run --suite testsuite/agenttests/euclo.rapid.testsuite.yaml --tier live-flaky
```

### Run all suites for one agent

```bash
./dev-agent agenttest run --agent euclo
```

### Run a single case

```bash
./dev-agent agenttest run \
  --suite testsuite/agenttests/euclo.rapid.debug.testsuite.yaml \
  --case rapid_debug_single_bug
```

### Filter by tag

```bash
./dev-agent agenttest run --tag level:1
./dev-agent agenttest run --tag rapid-iteration
./dev-agent agenttest run --tag performance-baseline
```

### Tune the runtime

```bash
./dev-agent agenttest run \
  --agent euclo \
  --timeout 120s \
  --max-iterations 6 \
  --backend-reset model \
  --backend-reset-between
```

### Disable AST bootstrap for live runs

```bash
./dev-agent agenttest run --agent euclo --skip-ast-index=false
```

---

## Required Prerequisites

For live runs:

- Ollama must be available
- the target model must be loaded

Example:

```bash
ollama pull qwen2.5-coder:14b
```

The suite defaults to `qwen2.5-coder:14b` for the Euclo live catalog unless overridden.

---

## Common Assertions

When authoring or reviewing a suite, the most useful assertions are usually:

- `files_changed`
- `no_file_changes`
- `files_contain`
- `tool_calls_must_include`
- `tool_calls_must_exclude`
- `max_tool_calls`
- `output_regex`
- `state_keys_must_exist`
- `state_keys_not_empty`
- `workflow_has_tensions`
- `euclo.mode`
- `euclo.profile`
- `euclo.behavior_family`
- `euclo.primary_relurpic_capability`

For rapid live baselines, the best signal usually comes from:

- required tool families
- forbidden tool families
- bounded tool counts
- output/state invariants

Exact tool ordering should only be used when the flow is deterministic enough to justify it.

---

## Practical Guidance

When a case fails, the first thing to inspect is usually:

- `report.json` for the high-level failure reason
- `changed_files.json` for workspace mutations
- `context.snapshot.json` for runtime state
- `token_usage.json` for iteration or token pressure
- `logs/{case}__{model}.log` for the live execution trace

If the failure is model-dependent rather than harness-dependent, the right fix is usually to adjust the behavioral baseline:

- make the assertion more capability-focused
- relax incidental ordering
- keep explicit tool requirements
- preserve the intended runtime family check

If the failure is harness-dependent, the runner or expectation logic needs to be corrected instead.

---

## Related Docs

- [`testsuite/README.md`](/home/lex/Public/Relurpify/testsuite/README.md)
- [`testsuite/agenttests/README.md`](/home/lex/Public/Relurpify/testsuite/agenttests/README.md)
- [`docs/framework/testing.md`](/home/lex/Public/Relurpify/docs/framework/testing.md)
- [`docs/framework/relurpic-capabilities.md`](/home/lex/Public/Relurpify/docs/framework/relurpic-capabilities.md)
- [`docs/agents/euclo.md`](/home/lex/Public/Relurpify/docs/agents/euclo.md)
