# Dev-Agent CLI Euclo Harness Rework Plan

## Summary

This plan reworks `app/dev-agent-cli` from a general development runner into a
semantic harness for Euclo. The goal is to make Euclo testable in a
combinatoric, reproducible way against live LLM models and provider backends
without coupling the CLI to the current relurpish UI shell.

The CLI should expose Euclo's interaction contract directly:

- mode selection
- submode selection
- semantic trigger invocation
- context add/remove and context proposal flows
- plan promotion and workflow continuation
- HITL approval and denial events
- background service interactions
- relurpic capability selection and execution
- context lifecycle and restoration events
- benchmark and scoring execution

The rework is intentionally multi-phase. It should land as a durable testing
surface, not an MVP wrapper around existing agent execution.

Related references:

- [`docs/framework/relurpic-capabilities.md`](/home/lex/Public/Relurpify/docs/framework/relurpic-capabilities.md)
- [`docs/agents/euclo.md`](/home/lex/Public/Relurpify/docs/agents/euclo.md)
- [`docs/framework/testing.md`](/home/lex/Public/Relurpify/docs/framework/testing.md)
- [`testsuite/README.md`](/home/lex/Public/Relurpify/testsuite/README.md)

---

## Problem Statement

The current CLI can run Euclo, but it does not yet model Euclo's interaction
contract as a first-class test harness.

Today:

- `dev-agent start` executes an agent once from an instruction.
- `agenttest` can inject scripted interaction via `euclo.interaction_script`.
- Euclo consumes those scripts in runtime, but the CLI still treats them as task
  payload data, not as a dedicated semantic trigger surface.
- The testsuite catalog is organized mostly by mode, while the actual coverage
  need is capability-first, trigger-first, and journey-first.

That is sufficient for basic coverage. It is not sufficient for:

- exhaustive capability validation
- deterministic workflow automation
- combinatoric model/provider coverage
- live benchmark runs with clean separation from functional tests

This plan corrects that boundary.

---

## Goals

1. Expose Euclo interaction triggers directly from the CLI.
2. Make every relurpic capability testable in a live harness.
3. Support deterministic orchestration around inherently stochastic LLM calls.
4. Separate capability baselines, user journey tests, and benchmark runs.
5. Support combinatoric testing across:
   - models
   - providers
   - Euclo modes
   - submodes
   - trigger families
   - workspace shapes
   - context states
6. Preserve the existing generic `agenttest` runner while extending it with
   Euclo-specific semantics.
7. Keep relurpish as one UX over the contract, not the contract itself.

---

## Non-Goals

- Reproducing the relurpish UI inside the CLI.
- Hard-coding Euclo to a single provider.
- Treating benchmark scoring as the same thing as functional validation.
- Collapsing archaeology, debug, and chat into one shared suite bucket.
- Making deterministic LLM output a requirement.

Deterministic here means:

- deterministic harness inputs
- deterministic sequencing
- deterministic artifact capture
- deterministic scoring rules
- deterministic replay and promotion

It does not mean the model response itself is identical across all runs.

---

## Current-State Assessment

### What already exists

- `dev-agent-cli` can launch Euclo through `start`.
- `agenttest` supports derived workspaces, tape recording, backend reset
  controls, and Euclo-specific expectations.
- Euclo already has a trigger registry and a phase-machine interaction model.
- `named/euclo/internal/agentstate` can build a scripted test emitter from task
  context.
- The testsuite schema already carries:
  - task prompts
  - interaction scripts
  - Euclo-specific assertion blocks
  - workflow seeds
  - memory seeds
  - browser fixtures

### What is missing

- No CLI surface for semantic trigger selection or trigger-flow execution.
- No first-class way to list or execute relurpic capabilities as a catalog.
- No explicit separation between capability baseline tests and workflow journey
  tests.
- No benchmark lane that is clearly distinct from pass/fail suites.
- No built-in combinatoric matrix runner for model/provider/capability coverage.
- No explicit runtime contract around user-visible trigger events versus
  internal context lifecycle events.

---

## Target Architecture

The target architecture has four layers.

### 1. CLI contract layer

This layer exposes Euclo semantics to humans and automation.

It should let callers say:

- "run this capability"
- "fire this trigger"
- "execute this user journey"
- "benchmark this capability family against these models"
- "replay this exact trigger sequence"
- "list the triggers/capabilities available in this workspace"

### 2. Harness orchestration layer

This layer decides:

- how a case is instantiated
- which workspace is materialized
- which backend resets occur between cases
- which model/provider pair is active
- whether the run is live, recorded, replayed, or benchmarked
- how scripted interactions are injected
- how outputs and artifacts are classified

### 3. Euclo runtime contract layer

This layer is already largely present in `named/euclo`.

It includes:

- mode selection
- phase machines
- trigger resolution
- capability execution
- context proposal and restoration
- memory and workflow state
- archaeology-owned BKC semantics

### 4. Evaluation layer

This layer turns execution artifacts into:

- capability assertions
- journey assertions
- benchmark scores
- regression deltas
- provider comparison reports

---

## CLI Surface Map

The current `dev-agent-cli` already has groups for `start`, `service`,
`archaeo`, `agents`, `session`, and `agenttest`.

This rework adds a new Euclo-oriented group and extends `agenttest`.

### Existing commands to preserve

- `dev-agent start`
- `dev-agent agenttest run`
- `dev-agent agenttest promote`
- `dev-agent agenttest refresh`
- `dev-agent agenttest tapes`
- `dev-agent service`
- `dev-agent session`
- `dev-agent agents`
- `dev-agent archaeo`

### New top-level Euclo group

`dev-agent euclo`

This group is the semantic harness entrypoint.

#### `dev-agent euclo capabilities`

Purpose: inspect and execute the relurpic capability catalog.

Subcommands:

- `list`
- `show <capability-id>`
- `run --capability <id>`
- `matrix --capability <selector>`

Outputs:

- capability metadata
- mode ownership
- trigger bindings
- required context shape
- expected artifact kinds
- expected interaction frames
- runtime family and ownership boundaries

#### `dev-agent euclo triggers`

Purpose: inspect and fire user-visible semantic triggers.

Subcommands:

- `list --mode <mode>`
- `resolve --mode <mode> --text <input>`
- `fire --mode <mode> --phrase <text>`
- `script --mode <mode> --file <script.yaml>`

Trigger payloads should be first-class and not depend on UI-specific phrasing.

#### `dev-agent euclo journey`

Purpose: execute a complete user journey across phases and modes.

Subcommands:

- `run --suite <suite>`
- `step --mode <mode> --phase <phase>`
- `resume --run <run-id>`
- `promote --run <run-id>`

This is where explore -> plan -> implement, chat -> implement, and debug ->
localize -> patch sequences are executed as semantic flows.

#### `dev-agent euclo benchmark`

Purpose: run benchmark/score suites only.

Subcommands:

- `run --suite <suite>`
- `compare --baseline <path>`
- `matrix --suite <suite> --models ... --providers ...`

Benchmark runs must be isolated from ordinary functional pass/fail runs.

### `agenttest` extensions

`agenttest` remains the generic suite runner, but it should gain Euclo-aware
layering.

Proposed additions:

- `--layer capability-baseline|journey|benchmark`
- `--capability <selector>`
- `--trigger <name>`
- `--model-set <file>`
- `--provider-set <file>`
- `--matrix <file>`
- `--journey-script <file>`
- `--deterministic-seed <value>`
- `--score-only`
- `--record-only`
- `--replay-only`

The important point is not the exact flag names. It is that the CLI should be
able to express Euclo's semantic contract without burying it inside a generic
prompt runner.

---

## Test Taxonomy

The tests should be split into three top-level layers.

### Layer 1: Capability Baselines

Purpose: prove that individual capabilities, tool surfaces, and relurpic
capabilities behave correctly in isolation.

Includes:

- tool capability checks
- relurpic capability checks
- transition guards
- context-management primitives
- artifact emission contracts
- BKC-adjacent wrapper behavior where Euclo owns the wrapper
- provider-specific capability mapping where the provider changes execution

Examples:

- `euclo:chat.ask`
- `euclo:chat.inspect`
- `euclo:chat.implement`
- `euclo:debug.investigate`
- `euclo:archaeology.explore`
- `euclo:archaeology.compile-plan`
- `euclo:archaeology.implement-plan`
- `euclo:design.alternatives`
- `euclo:trace.analyze`
- `gap_analysis` as a cross-cutting capability baseline

These tests should focus on:

- correct primary capability selection
- correct supporting capability composition
- correct state transitions
- correct artifacts
- correct tool usage
- correct recovery behavior
- correct context lifecycle effects

### Layer 2: User Journey / Trigger-Flow Tests

Purpose: prove that the Euclo interaction contract works as a user would
experience it.

Includes:

- mode selection
- submode selection
- context proposal confirmation
- file add/remove flows
- trigger resolution
- transition acceptance
- HITL-style approval paths
- plan promotion
- workflow resume
- exploration -> plan -> implement
- debug reproduce -> localize -> patch
- chat ask -> inspect -> implement

These tests should be expressed as ordered scripts, not just prompts.

They need to cover:

- trigger resolution from freetext
- phase machine jumps
- carry-over artifacts
- context updates
- workflow state persistence
- cross-mode transitions

### Layer 3: Benchmark / Score Runs

Purpose: measure quality, stability, and performance under live conditions.

Includes:

- scorecards over multiple runs
- model/provider comparison runs
- artifact quality scoring
- token and latency measurements
- context pressure measurements
- repair success rates
- journey completion rates
- capability coverage rates

These runs should:

- be reproducible by input set
- be promotion-friendly
- remain separate from correctness gating
- preserve baselines over time

---

## Capability Baseline Taxonomy

Capability baselines should be subdivided so they can be run independently.

### A. Tool capability baselines

Validate:

- file read/write/list/search
- shell execution
- graph/context retrieval
- memory operations
- service interactions
- permission and policy enforcement

### B. Relurpic capability baselines

Validate:

- each primary Euclo capability
- each reusable local capability
- each specialized flow wrapper
- each mode-triggered behavior family

### C. Context-management baselines

Validate:

- context proposal
- context merge
- context compaction
- restore
- session pin persistence
- workflow knowledge carry-over
- BKC-aware seeding where applicable

### D. Cross-cutting capability baselines

Validate:

- `gap_analysis`
- verification repair
- review-guided implementation
- provenance-sensitive investigation
- transition between capability families

---

## Journey Taxonomy

Journeys should be defined by intent, not by one-off prompts.

### Chat journeys

- ask a question from code context
- inspect a structure
- implement a small change
- transition from answer to implementation

### Debug journeys

- diagnose a failure from evidence
- localize root cause
- propose a fix
- apply a fix
- verify the repair

### Planning journeys

- scope a problem
- clarify when needed
- generate a plan
- compare candidates
- refine a plan
- commit execution

### Archaeology journeys

- explore a codebase
- promote findings into plan state
- compile a plan from knowledge chunks
- implement from plan
- resume from prior exploration state

These should be explicitly distinct from benchmarks.

---

## Determinism Strategy

The live models will not be deterministic in the strict mathematical sense.
The harness must make the surrounding system deterministic enough for useful
testing.

### Deterministic inputs

- fixed prompts
- fixed context seeds
- fixed workflow seeds
- fixed interaction scripts
- fixed capability selection
- fixed workspace template profiles

### Deterministic sequencing

- ordered trigger firing
- ordered phase execution
- ordered transition acceptance
- explicit retry/reset policy
- explicit backend reset between cases

### Deterministic capture

- tape recording
- interaction tape recording
- state snapshots
- context snapshots
- model provenance
- artifact hashes where useful

### Deterministic evaluation

- exact artifact expectations for baselines
- bounded scoring for benchmarks
- comparison against committed baseline files
- explicit failure-class reporting

### Deterministic matrix control

- pinned model/provider sets
- pinned case groups
- pinned trigger scripts
- explicit random seeds where any harness randomness remains

---

## Multi-Phase Rework Plan

### Phase 1: CLI contract extraction

Objective: define the Euclo interaction contract in the CLI before expanding
coverage.

Work:

- add a dedicated `newEucloCmd()` tree under `app/dev-agent-cli`
  - `euclo capabilities`
  - `euclo triggers`
  - `euclo journey`
  - `euclo benchmark`
- define CLI-local types for:
  - `CapabilityCatalogEntry`
  - `TriggerCatalogEntry`
  - `EucloJourneyStep`
  - `EucloJourneyScript`
  - `EucloBenchmarkMatrix`
- add a small `EucloCommandRunner` abstraction so command handlers can be unit
  tested without booting the full runtime
- wire all Euclo commands through the existing workspace bootstrap path instead
  of introducing a second runtime startup flow
- extend `agenttest` with layer-aware flags and metadata:
  - `--layer`
  - `--capability`
  - `--trigger`
  - `--journey-script`
  - `--matrix`
  - `--score-only`
  - `--record-only`
  - `--replay-only`
- keep `start` semantics unchanged in this phase
- add JSON output for every catalog, resolve, and list command

Exit criteria:

- the CLI can list, resolve, and fire Euclo triggers
- the CLI can enumerate Euclo capabilities
- the CLI can execute a simple semantic script without depending on relurpish
- the CLI can label runs as capability, journey, or benchmark
- command handlers are testable through direct unit tests
- no relurpish-specific types leak into the CLI API

### Phase 2: Capability catalog and registry surfaces

Objective: make capability discovery and execution explicit and machine-readable.

Work:

- build a canonical catalog source from:
  - `named/euclo/relurpicabilities`
  - `named/euclo/interaction/modes`
  - `named/euclo/interaction/registry.go`
  - `named/euclo/core/relurpic.go`
- define runtime metadata for each capability:
  - capability ID
  - primary owner
  - mode family
  - supporting routines
  - expected artifact kinds
  - supported transition targets
  - baseline-safe or journey-only classification
- expose trigger metadata per mode:
  - phrases
  - phase jump target
  - capability ID
  - `RequiresMode`
  - description
- add catalog renderers:
  - human-readable table output
  - machine-readable JSON output
  - single-item `show` output
- add selector helpers for:
  - exact capability IDs
  - prefix matching
  - mode-scoped capability families
  - trigger phrase lookup
- add a capability-to-test-layer mapping table so one capability can route to
  baseline, journey, or benchmark execution depending on metadata

Exit criteria:

- capability inventory is queryable from the CLI
- tests can select capabilities by exact ID or selector
- baselines can assert on catalog shape without executing a journey
- trigger bindings are visible separately from capability ownership
- the catalog is stable enough to snapshot-test

### Phase 3: Trigger-flow execution engine

Objective: encode user-visible Euclo interaction flows as reusable scripts.

Work:

- define a versioned journey script schema separate from raw agenttest case
  YAML:
  - `script_version`
  - `initial_mode`
  - `initial_context`
  - `steps[]`
  - `expected_terminal_state`
  - `recording_mode`
- support step kinds such as:
  - `mode.select`
  - `submode.select`
  - `trigger.fire`
  - `context.add`
  - `context.remove`
  - `frame.respond`
  - `transition.accept`
  - `transition.reject`
  - `hitl.approve`
  - `hitl.deny`
  - `workflow.resume`
  - `plan.promote`
  - `artifact.expect`
- add a script executor that maps each step to a runtime entrypoint:
  - Euclo trigger resolution
  - phase-machine state mutation
  - workspace/service bootstrap
  - task context seeding
  - explicit response injection through the test emitter
- support ordered replay and deterministic step verification:
  - expected current phase
  - expected selected capability
  - expected emitted frame kind
  - expected artifact kind
  - expected state key mutations
- support both live-run and replay-run execution:
  - live-run executes the trigger or journey against a provider
  - replay-run reads a recorded tape plus interaction tape and verifies the
    journey sequence
- add journey artifacts for:
  - step transcript
  - step timing
  - emitted frames
  - response sequence
  - transition sequence
  - terminal state snapshot

Exit criteria:

- a journey can be replayed from a script file
- the same journey can be run live or replayed from tape
- trigger execution is recorded in a way that can be promoted into baselines
- the executor validates ordered user intent rather than only final output
- replay failures identify the exact failed step

### Phase 4: Capability baselines

Objective: create deterministic tests for every relurpic capability and core
tool capability.

Work:

- split capability tests out of the current mode suites into dedicated
  capability-focused suites
- add tool capability baseline suites that validate the minimum admitted tool
  set for each mode
- add relurpic capability baseline suites that target a single capability ID
  or small capability family at a time
- add context-management baselines for:
  - context proposal
  - context merge
  - compaction
  - restoration
  - workflow pin persistence
  - BKC-aware seeding
- add cross-cutting baselines for:
  - `gap_analysis`
  - verification repair
  - review-guided implementation
  - transition carry-over semantics
- expose a stable `baseline` mode in the CLI that forces exact expectations and
  disables benchmark aggregation
- add one baseline harness fixture per major capability family so failure
  attribution is unambiguous

Exit criteria:

- each major capability family has at least one baseline case
- capability failures are distinguishable from journey failures
- the suite can run baseline-only subsets
- baseline cases can be selected by capability ID
- tool regression failures do not get conflated with relurpic behavior failures

### Phase 5: Journey suite decomposition

Objective: turn the current mode-oriented suites into intent-oriented flow suites.

Work:

- split suite definitions by execution intent rather than by mode name alone
- refactor archaeology into:
  - capability baselines for `explore`, `compile-plan`, and `implement-plan`
  - end-to-end user journeys for `explore -> plan -> implement`
  - resume/restore journeys
  - benchmark scenarios
- refactor chat into:
  - `ask` baselines
  - `inspect` baselines
  - `implement` journeys
  - chat-to-code transition journeys
- refactor debug into:
  - investigation baselines
  - reproduce/localize/patch journeys
  - evidence-driven skip paths
- preserve planning as a distinct journey family rather than merging it into
  either debug or archaeology
- add a top-level suite classification field or metadata tag for:
  - `capability`
  - `journey`
  - `benchmark`
- keep `gap_analysis` as a reusable capability that can be invoked from
  chat/debug/planning where the prompt warrants it

Exit criteria:

- every current mode has a clear baseline layer and journey layer
- archaeology is no longer the only home of plan compilation and gap analysis
- tests can target a single journey intent end-to-end
- suite naming makes the execution layer obvious before a test is run

### Phase 6: Benchmark and scoring system

Objective: separate scoring runs from validation runs.

Work:

- define benchmark-specific suite metadata:
  - score family
  - score dimensions
  - comparison window
  - acceptable variance threshold
- define scoring adapters per layer:
  - capability score
  - journey completion score
  - artifact quality score
  - recovery score
  - context-pressure score
  - provider stability score
- emit benchmark reports separately from `SuiteReport` pass/fail results
- preserve benchmark artifacts as first-class outputs:
  - raw tape
  - interaction tape
  - score JSON
  - baseline comparison JSON
  - provider/model provenance JSON
- support aggregate scores over:
  - multiple runs of the same suite
  - multiple providers
  - multiple models
  - multiple trigger scripts
- make score computation deterministic from recorded artifacts so a benchmark
  can be recomputed offline

Exit criteria:

- benchmark runs can be invoked independently
- benchmark output is stable enough for trend tracking
- benchmark failures do not conflate with correctness failures
- benchmark output includes both per-case and aggregate summaries
- benchmark scoring can be rerun from stored artifacts

### Phase 7: Provider/model combinatorics

Objective: run the same semantic coverage across multiple live providers and
models.

Work:

- define explicit provider-set and model-set inputs, likely as files or config
  blocks rather than freeform flags
- expand a suite into a run matrix with deterministic ordering:
  - provider first or model first
  - capability layer grouping
  - journey grouping
  - benchmark grouping
- define backend reset policy per matrix axis:
  - unload between cases
  - restart backend service
  - no reset
  - reset on matched failure class
- record provider provenance in every case report:
  - provider name
  - endpoint
  - model name
  - model digest if available
  - backend reset strategy used
- add provider-adaptive expectations where needed:
  - same capability path, different artifact timing
  - same trigger flow, different token counts
  - same journey, different recovery shape
- preserve strict comparability by separating unsupported-provider skips from
  actual failures

Exit criteria:

- one logical Euclo suite can run across many provider/model combinations
- the harness can compare capability behavior across providers
- matrix runs can be summarized by layer and by provider
- provider-specific skips are reported distinctly from functional failures

### Phase 8: Promotion, replay, and regression workflow

Objective: turn live runs into durable regressions and maintainable coverage.

Work:

- add promotion paths for each artifact class:
  - capability tape promotion
  - journey tape promotion
  - benchmark baseline promotion
  - catalog snapshot promotion
- support replay-only mode for:
  - individual capability baselines
  - trigger-flow journeys
  - benchmark reproductions
- add drift detection for:
  - changed trigger bindings
  - changed capability ownership
  - changed phase ordering
  - changed artifact kinds
  - changed provider/model metadata
- add stale-baseline reporting that distinguishes:
  - baseline missing
  - baseline outdated
  - baseline incompatible with current catalog
- add a promotion review step so only cases that match the selected layer and
  output class can be promoted
- preserve artifact lineage so promoted files can be traced back to the exact
  matrix input and model/provider combination

Exit criteria:

- successful live runs can be promoted without manual tape surgery
- replay mode can isolate regressions from provider noise
- stale coverage is visible before the suite silently drifts
- promotion is layer-aware and refuses cross-layer artifact promotion
- drift reports tell you what changed, not just that something changed

---

## Implementation Boundary

### `dev-agent-cli`

Owns:

- CLI command surface
- run orchestration
- workspace and service bootstrap
- trigger/flow/benchmark dispatch
- provider/model matrix expansion
- reporting and promotion hooks

Should not own:

- Euclo behavior semantics
- relurpic capability logic
- phase-machine logic

### `named/euclo`

Owns:

- Euclo mode semantics
- relurpic capability semantics
- trigger resolution
- context lifecycle
- BKC integration where Euclo owns the wrapper
- workflow and memory semantics

Should not own:

- CLI presentation
- suite orchestration policy
- benchmark score presentation

### `testsuite`

Owns:

- suite definitions
- scripted case definitions
- fixtures
- baselines
- artifact expectations
- report generation

Should not own:

- CLI semantics
- runtime semantics

---

## Proposed Suite Layout

The current mode-based suite names can remain, but they should be layered.

### Capability baseline suites

- `euclo.capabilities.tools.testsuite.yaml`
- `euclo.capabilities.chat.testsuite.yaml`
- `euclo.capabilities.debug.testsuite.yaml`
- `euclo.capabilities.archaeology.testsuite.yaml`
- `euclo.capabilities.context.testsuite.yaml`
- `euclo.capabilities.crosscutting.testsuite.yaml`

### Journey suites

- `euclo.journey.chat.testsuite.yaml`
- `euclo.journey.debug.testsuite.yaml`
- `euclo.journey.planning.testsuite.yaml`
- `euclo.journey.archaeology.testsuite.yaml`
- `euclo.journey.transitions.testsuite.yaml`

### Benchmark suites

- `euclo.benchmark.chat.testsuite.yaml`
- `euclo.benchmark.debug.testsuite.yaml`
- `euclo.benchmark.archaeology.testsuite.yaml`
- `euclo.benchmark.provider-matrix.testsuite.yaml`
- `euclo.benchmark.context-pressure.testsuite.yaml`

The existing files can be migrated gradually rather than renamed all at once.

---

## Acceptance Criteria

The rework is successful when:

1. Euclo capabilities are discoverable and executable directly from the CLI.
2. Semantic triggers can be fired without relurpish.
3. User journeys can be replayed as ordered scripts.
4. Capability baselines, journeys, and benchmarks are separate layers.
5. The same Euclo case can be expanded across multiple models/providers.
6. Live runs can be reset, recorded, replayed, and promoted deterministically.
7. Archaeology, chat, debug, and planning retain clear ownership boundaries.
8. `gap_analysis` is covered as a reusable capability, not only as an archaeology artifact.
9. BKC-backed behavior is testable at the Euclo wrapper layer where Euclo owns
   the wrapper, and separately at the Archaeo primitive layer where Archaeo
   owns the primitive.

---

## Suggested Rollout Order

1. Add the CLI contract and catalog surfaces.
2. Add journey script execution.
3. Split capability baselines from journey tests.
4. Add benchmark output and score separation.
5. Add provider/model matrix expansion.
6. Convert the existing Euclo suites incrementally.
7. Promote stable capability tapes and journey tapes.

This order keeps the harness useful while the suite taxonomy is being reshaped.
