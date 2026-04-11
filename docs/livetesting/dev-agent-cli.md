# `dev-agent` CLI Technical Overview

The `app/dev-agent-cli` package is the repository’s development-facing command-line entrypoint for Relurpify. It is the scripted control plane for local agent execution, Euclo catalog inspection, live integration tests, and workspace/service inspection.

The binary is intentionally developer-oriented. It is not the end-user TUI. The main purpose is to let the repository exercise the real runtime stack from the shell with reproducible flags, deterministic artifacts, and YAML-driven suites.

---

## Scope

This package covers:

- root CLI bootstrap and global config loading
- Euclo capability, trigger, journey, benchmark, and baseline commands
- `agenttest` live integration suite execution, promotion, and tape inspection
- workspace, service, session, skill, config, and archaeology helpers
- shared CLI utility code for YAML, workspace, and value handling

The package is organized around Cobra command wiring plus a set of local execution helpers. The implementation is split into focused files rather than one monolith.

---

## Command Tree

The root command is defined in [app/dev-agent-cli/root.go](/home/lex/Public/Relurpify/app/dev-agent-cli/root.go). It exposes the `dev-agent` binary and wires the CLI subcommands.

Top-level commands:

- `start`
- `euclo`
- `workspace`
- `service`
- `archaeo`
- `agents`
- `skill`
- `config`
- `session`
- `agenttest`

The root command also sets persistent flags:

- `--workspace`
- `--config`
- `--sandbox-backend`

Bootstrap behavior:

- the workspace defaults to the current working directory if omitted
- the config file is resolved from the workspace and falls back to `relurpify.yaml` if needed
- the global config is loaded before any command runs

The actual process entrypoint is [app/dev-agent-cli/main.go](/home/lex/Public/Relurpify/app/dev-agent-cli/main.go), which only calls `Execute()`.

---

## File Layout

The package is now split by responsibility.

### Core bootstrap

- [app/dev-agent-cli/root.go](/home/lex/Public/Relurpify/app/dev-agent-cli/root.go)
- [app/dev-agent-cli/main.go](/home/lex/Public/Relurpify/app/dev-agent-cli/main.go)
- [app/dev-agent-cli/util.go](/home/lex/Public/Relurpify/app/dev-agent-cli/util.go)

### Euclo CLI

- [app/dev-agent-cli/euclo_cmd.go](/home/lex/Public/Relurpify/app/dev-agent-cli/euclo_cmd.go)
- [app/dev-agent-cli/euclo_ops.go](/home/lex/Public/Relurpify/app/dev-agent-cli/euclo_ops.go)
- [app/dev-agent-cli/euclo_render.go](/home/lex/Public/Relurpify/app/dev-agent-cli/euclo_render.go)
- [app/dev-agent-cli/euclo_types.go](/home/lex/Public/Relurpify/app/dev-agent-cli/euclo_types.go)
- [app/dev-agent-cli/euclo_helpers.go](/home/lex/Public/Relurpify/app/dev-agent-cli/euclo_helpers.go)
- [app/dev-agent-cli/euclo_catalog.go](/home/lex/Public/Relurpify/app/dev-agent-cli/euclo_catalog.go)
- [app/dev-agent-cli/euclo_journey.go](/home/lex/Public/Relurpify/app/dev-agent-cli/euclo_journey.go)
- [app/dev-agent-cli/euclo_wiring.go](/home/lex/Public/Relurpify/app/dev-agent-cli/euclo_wiring.go)

### Agent test harness

- [app/dev-agent-cli/agenttest_cmd.go](/home/lex/Public/Relurpify/app/dev-agent-cli/agenttest_cmd.go)
- [app/dev-agent-cli/agenttest_promote.go](/home/lex/Public/Relurpify/app/dev-agent-cli/agenttest_promote.go)
- [app/dev-agent-cli/agenttest_tapes.go](/home/lex/Public/Relurpify/app/dev-agent-cli/agenttest_tapes.go)
- [app/dev-agent-cli/agenttest_lane.go](/home/lex/Public/Relurpify/app/dev-agent-cli/agenttest_lane.go)

### Other CLI helpers

- [app/dev-agent-cli/agents.go](/home/lex/Public/Relurpify/app/dev-agent-cli/agents.go)
- [app/dev-agent-cli/archaeo.go](/home/lex/Public/Relurpify/app/dev-agent-cli/archaeo.go)
- [app/dev-agent-cli/config.go](/home/lex/Public/Relurpify/app/dev-agent-cli/config.go)
- [app/dev-agent-cli/service.go](/home/lex/Public/Relurpify/app/dev-agent-cli/service.go)
- [app/dev-agent-cli/session.go](/home/lex/Public/Relurpify/app/dev-agent-cli/session.go)
- [app/dev-agent-cli/skill.go](/home/lex/Public/Relurpify/app/dev-agent-cli/skill.go)
- [app/dev-agent-cli/start.go](/home/lex/Public/Relurpify/app/dev-agent-cli/start.go)
- [app/dev-agent-cli/workspace.go](/home/lex/Public/Relurpify/app/dev-agent-cli/workspace.go)

---

## Euclo CLI

The Euclo surface is the most specialized part of the package. It provides a local command tree for inspecting the Euclo capability catalog and for running deterministic local execution flows.

Implemented in:

- [app/dev-agent-cli/euclo_cmd.go](/home/lex/Public/Relurpify/app/dev-agent-cli/euclo_cmd.go)
- [app/dev-agent-cli/euclo_ops.go](/home/lex/Public/Relurpify/app/dev-agent-cli/euclo_ops.go)
- [app/dev-agent-cli/euclo_render.go](/home/lex/Public/Relurpify/app/dev-agent-cli/euclo_render.go)
- [app/dev-agent-cli/euclo_types.go](/home/lex/Public/Relurpify/app/dev-agent-cli/euclo_types.go)

### Command groups

- `dev-agent euclo capabilities`
- `dev-agent euclo baseline`
- `dev-agent euclo triggers`
- `dev-agent euclo journey`
- `dev-agent euclo benchmark`

### Capabilities

The capabilities surface resolves Euclo capability metadata from the catalog and renders either human-readable tables or JSON.

Supported operations:

- `capabilities list`
- `capabilities show <capability-id>`
- `capabilities run --capability <selector>`
- `capabilities matrix --capability <selector>`

The capability catalog is assembled by [app/dev-agent-cli/euclo_catalog.go](/home/lex/Public/Relurpify/app/dev-agent-cli/euclo_catalog.go). It merges:

- the core relurpic registry
- Euclo mode metadata
- supplemental CLI-local capability entries

### Baselines

The baseline surface exists to run deterministic capability-only checks.

Supported operations:

- `baseline list`
- `baseline show <capability-id>`
- `baseline run --capability <selector>`

Baseline reports are intentionally exact and label benchmark aggregation as disabled. Baseline-eligible entries are taken from the Euclo catalog projection.

### Triggers

The triggers surface inspects and resolves trigger phrases for a mode.

Supported operations:

- `triggers list --mode <mode>`
- `triggers resolve --mode <mode> --text <input>`
- `triggers fire --mode <mode> --phrase <phrase>`
- `triggers script --mode <mode> --file <script.yaml>`

Trigger resolution uses the same catalog-backed source of truth as capability lookup.

### Journey

The journey surface runs ordered YAML scripts against the local harness.

Supported operations:

- `journey run --file <script.yaml>`
- `journey step --mode <mode> ...`
- `journey resume --run <id>`
- `journey promote --run <id>`

Journey scripts are validated by [app/dev-agent-cli/euclo_types.go](/home/lex/Public/Relurpify/app/dev-agent-cli/euclo_types.go). The engine itself lives in [app/dev-agent-cli/euclo_journey.go](/home/lex/Public/Relurpify/app/dev-agent-cli/euclo_journey.go).

The journey report captures:

- step transcript entries
- frame records
- response records
- transition records
- terminal state
- validation failures

### Benchmark

The benchmark surface runs a local matrix and can optionally drive journeys per row.

Supported operations:

- `benchmark run --matrix <matrix.yaml>`
- `benchmark compare --baseline <file>`
- `benchmark matrix --capability <selector>`

The matrix supports:

- capability selectors
- model sets
- provider sets
- axis ordering
- optional journey scripts

The summary output includes total cases, success counts, and unique capability/model/provider axes.

### JSON output

All Euclo commands respect the inherited `--json` flag and emit machine-readable results when requested.

---

## Agenttest Harness

The `agenttest` command tree drives repository integration suites with real runtime execution.

Implemented in:

- [app/dev-agent-cli/agenttest_cmd.go](/home/lex/Public/Relurpify/app/dev-agent-cli/agenttest_cmd.go)
- [app/dev-agent-cli/agenttest_promote.go](/home/lex/Public/Relurpify/app/dev-agent-cli/agenttest_promote.go)
- [app/dev-agent-cli/agenttest_tapes.go](/home/lex/Public/Relurpify/app/dev-agent-cli/agenttest_tapes.go)
- [app/dev-agent-cli/agenttest_lane.go](/home/lex/Public/Relurpify/app/dev-agent-cli/agenttest_lane.go)

### Command groups

- `agenttest run`
- `agenttest promote`
- `agenttest refresh`
- `agenttest tapes`

### Run

`agenttest run` resolves suites from either:

- explicit `--suite` paths
- canonical suite discovery in `testsuite/agenttests/`

It applies:

- lane presets
- tier filtering
- profile filtering
- quarantined filtering
- case name and tag filtering

It then runs the selected suites through the real `testsuite/agenttest` runner and prints summary output plus performance/benchmark summaries when present.

Model profile selection is applied during live runs using the checkout's
`relurpify_cfg/model_profiles/` registry. The selected provider and model are
resolved to a `platform/llm.ModelProfile`, then attached to the Ollama client
before the case starts.

### Promote

`agenttest promote` copies recorded tapes out of a completed run into the golden tape directory.

Promotion behavior includes:

- tape copy
- optional interaction tape copy
- benchmark artifact promotion when the suite classification is `benchmark`
- lineage metadata written as `*.promotion.json`

### Refresh

`agenttest refresh` re-runs a suite in live mode, then promotes the resulting tapes if every case passed.

### Tapes

`agenttest tapes` inspects golden tape coverage and staleness.

It reports:

- whether a tape is present
- whether the tape is stale
- whether a golden baseline exists
- whether a baseline is missing

This command is useful for drift detection and coverage auditing.

---

## Shared Utility Layer

The CLI-level helpers in [app/dev-agent-cli/util.go](/home/lex/Public/Relurpify/app/dev-agent-cli/util.go) handle:

- workspace resolution
- config loading and map mutation
- simple value parsing
- one-line pretty printing
- session directory resolution
- filename sanitization
- string normalization
- de-duplication and collection helpers

These helpers are used across both Euclo and agenttest command trees.

---

## Runtime and Catalog Model

The Euclo CLI intentionally projects runtime concepts into CLI-local DTOs rather than exposing the raw runtime structs directly.

Key types live in [app/dev-agent-cli/euclo_types.go](/home/lex/Public/Relurpify/app/dev-agent-cli/euclo_types.go):

- `CapabilityCatalogEntry`
- `TriggerCatalogEntry`
- `EucloJourneyScript`
- `EucloJourneyStep`
- `EucloBenchmarkMatrix`
- `EucloBenchmarkAxisSpec`
- `EucloCapabilityRunResult`
- `EucloJourneyReport`
- `EucloBenchmarkReport`
- `EucloBaselineReport`

This layer exists so the CLI can:

- keep its JSON schema stable
- normalize runtime and catalog metadata
- enforce local validation before execution
- render deterministic summaries for humans and CI

The catalog assembly itself is in [app/dev-agent-cli/euclo_catalog.go](/home/lex/Public/Relurpify/app/dev-agent-cli/euclo_catalog.go), and the local journey execution engine is in [app/dev-agent-cli/euclo_journey.go](/home/lex/Public/Relurpify/app/dev-agent-cli/euclo_journey.go).

---

## Artifacts

The CLI writes and reads a number of structured artifact files. Common ones include:

- `tape.jsonl`
- `interaction.tape.jsonl`
- `baseline.json`
- `benchmark_report.json`
- `benchmark_score.json`
- `benchmark_comparison.json`
- `provider.provenance.json`
- `*.promotion.json`

These artifacts are used by the promotion and reporting flows and are designed to be machine-readable.

---

## Testing

The package has focused tests around:

- command wiring
- catalog assembly
- Euclo script validation
- journey execution
- benchmark matrix behavior
- promotion and lineage output
- tape reporting

Primary test files:

- [app/dev-agent-cli/euclo_test.go](/home/lex/Public/Relurpify/app/dev-agent-cli/euclo_test.go)
- [app/dev-agent-cli/euclo_catalog_test.go](/home/lex/Public/Relurpify/app/dev-agent-cli/euclo_catalog_test.go)
- [app/dev-agent-cli/euclo_phase3_test.go](/home/lex/Public/Relurpify/app/dev-agent-cli/euclo_phase3_test.go)
- [app/dev-agent-cli/agenttest_test.go](/home/lex/Public/Relurpify/app/dev-agent-cli/agenttest_test.go)
- [app/dev-agent-cli/euclo_wiring_test.go](/home/lex/Public/Relurpify/app/dev-agent-cli/euclo_wiring_test.go)

The package is expected to stay green under:

- `go test ./app/dev-agent-cli`
- `go test ./...`

---

## Practical Notes

- The CLI assumes the current workspace is a Relurpify checkout or a compatible workspace root.
- Euclo uses catalog-backed selection, so selectors can be exact IDs, prefixes, mode names, or trigger phrases depending on the subcommand.
- `agenttest` runs real integration suites and should be treated as expensive compared with ordinary unit tests.
- Promotion flows are intentionally conservative and write lineage metadata so future cleanup can audit where each golden artifact came from.
- The package is currently split across many files. That split is intentional: it keeps command wiring, execution, rendering, and helper logic separate enough to maintain.

---

## Where To Look First

If you are extending the CLI, start here:

- command tree changes: [app/dev-agent-cli/root.go](/home/lex/Public/Relurpify/app/dev-agent-cli/root.go)
- Euclo command behavior: [app/dev-agent-cli/euclo_cmd.go](/home/lex/Public/Relurpify/app/dev-agent-cli/euclo_cmd.go)
- Euclo execution helpers: [app/dev-agent-cli/euclo_ops.go](/home/lex/Public/Relurpify/app/dev-agent-cli/euclo_ops.go)
- Euclo rendering/output: [app/dev-agent-cli/euclo_render.go](/home/lex/Public/Relurpify/app/dev-agent-cli/euclo_render.go)
- agenttest behavior: [app/dev-agent-cli/agenttest_cmd.go](/home/lex/Public/Relurpify/app/dev-agent-cli/agenttest_cmd.go)
- shared utilities: [app/dev-agent-cli/util.go](/home/lex/Public/Relurpify/app/dev-agent-cli/util.go)
