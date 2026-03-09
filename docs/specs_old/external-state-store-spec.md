# External State Store Rework Specification

## Synopsis

This document specifies the External State Store rework for Relurpify. The goal is to move long-running workflow state out of in-memory LLM context and into a first-class durable store, while preserving compact prompting, resumability, auditability, and predictable step execution.

This is a workflow runtime redesign centered on `architect` mode first. It also introduces new workflow features that depend on durable structured state:

- reliable pause and resume
- step-level audit trail
- selective replay and rerun
- replanning after repeated step failure
- dependency-aware invalidation
- store-backed workflow inspection APIs
- persisted `facts`, `issues`, and `decisions` for prompt projection
- cross-session workflow collaboration

Current date of this specification: March 6, 2026.

---

## Problem Statement

Relurpify currently relies on `core.Context` as both:

- the live scratchpad used by an executing agent
- the de facto carrier of multi-step workflow state

That design creates several problems for long-running tasks:

- workflow state grows inside prompt context and must be compressed aggressively
- inter-step state is implicit and difficult to inspect
- resume behavior depends on checkpoint snapshots rather than workflow-native state
- server and session boundaries risk leaking workflow-local state
- replay, rerun, invalidation, and cross-session inspection are difficult to model cleanly

The existing persistence layer is useful infrastructure, but it is currently checkpoint-oriented. The missing piece is a store-backed execution pattern where step boundaries are first-class and persisted state is authoritative.

---

## Goals

- Make durable workflow state the source of truth for multi-step execution.
- Keep LLM prompt context small by projecting only the state slice needed for the current step.
- Execute each plan step with a fresh ephemeral `core.Context`.
- Support workflow-native pause, resume, inspection, and replay.
- Persist structured workflow records for telemetry, debugging, and future tuning.
- Preserve history compression as a supported secondary mechanism.

---

## Non-Goals

- No `EternalAgent` changes in this rework.
- No mid-run plan mutation inside a workflow.
- No broad ReAct-wide migration in the first phase; `architect` mode is the first target.

If a user wants a materially different plan, the workflow should be canceled and restarted rather than edited in place.

---

## Fixed Design Decisions

The following decisions are part of the approved scope for this rework.

### Workflow identity model

Use `workflow_id` as the durable identity of an atomic composition of work.

Supporting identifiers:

- `task_id`: caller-facing task identifier
- `run_id`: a specific execution attempt for a workflow

Rules:

- a workflow may have multiple runs
- persisted records are keyed primarily by `workflow_id`
- reruns and resumes create or attach to `run_id` records without changing `workflow_id`

### Artifact model

Artifacts support both:

- inline blob storage
- file or external reference storage

Selection depends on payload size, content type, and compression policy.

### Tool output storage model

Tool outputs must not be stored only as compressed summaries.

Each stored artifact must support a dual representation:

- `prompt_view`: compressed summary optimized for prompt projection and default reads
- `raw_view`: full raw output when small enough, or a file/blob reference when large

Compression is derivational, not destructive. The compressed form is always derived from a raw payload or a raw-backed artifact reference.

### Prompting model

Prompt projection should prefer structured persisted state:

- current step
- dependency outputs
- relevant artifacts
- unresolved issues
- accepted decisions
- extracted facts

History compression remains supported as fallback and supplemental context, not the primary carrier of workflow knowledge.

### Plan mutation policy

No plan changes mid-run.

If repeated failure requires a changed approach, the workflow enters a replanning state. The UI or caller may cancel the workflow and start a new one, or the runtime may produce a versioned downstream replacement plan record as a new execution branch. Completed steps remain immutable.

---

## Architecture Overview

The rework introduces three major concepts:

1. `WorkflowStateStore`
2. `StateProjector`
3. step-scoped ephemeral runtime context

### WorkflowStateStore

The durable persistence layer stores workflow structure, execution state, artifacts, and audit records.

### StateProjector

The projection layer reads workflow state and builds a targeted step slice for prompting and execution.

### Ephemeral step runtime

Each step executes with a fresh `core.Context` seeded from the projected state slice. That context is disposable after the step finishes. Durable state is written back to the store explicitly.

---

## Execution Model

### Planner phase

1. Create workflow and run records.
2. Generate a plan.
3. Persist:
   - workflow metadata
   - immutable plan record
   - step definitions
   - dependency graph
   - planner facts, issues, and decisions
   - workflow events

### Step execution phase

For each ready step:

1. Load the step slice from the store.
2. Build a fresh ephemeral `core.Context`.
3. Run ReAct for that step only.
4. Persist:
   - step attempt result
   - artifacts
   - verification result
   - extracted facts, issues, and decisions
   - telemetry and events
5. Mark the step complete, failed, invalidated, or needs-replan.

### Resume phase

Resume is workflow-native:

1. load workflow state by `workflow_id`
2. inspect current cursor and step statuses
3. project only the next required step slice
4. continue execution

Raw graph checkpoints may remain as low-level recovery/debug support, but they are not the primary workflow resume mechanism for architect workflows.

---

## Data Model

The store should be backed by SQLite in v1.

Recommended persisted entities:

- `workflows`
  - workflow identity, task metadata, status, current cursor, timestamps
- `workflow_runs`
  - execution attempts, runtime version, agent version, status, timestamps
- `workflow_plans`
  - immutable plan records
- `workflow_steps`
  - step definitions, dependencies, ordering metadata
- `step_runs`
  - step attempts, retry count, status, summary, verification outcome
- `step_artifacts`
  - prompt summary plus raw payload or raw reference
- `step_facts`
  - persisted extracted facts used for prompt projection
- `step_issues`
  - unresolved or resolved issues with provenance
- `step_decisions`
  - decisions and rationale with provenance
- `workflow_events`
  - append-only audit and telemetry event stream
- `workflow_invalidation`
  - downstream invalidation records caused by reruns

### Artifact fields

Each artifact record should support at least:

- `artifact_id`
- `workflow_id`
- `step_run_id`
- `kind`
- `content_type`
- `summary_text`
- `summary_metadata`
- `inline_raw_text` nullable
- `raw_ref` nullable
- `raw_size_bytes`
- `compression_method`
- `created_at`

---

## Store Interface Requirements

The concrete Go interfaces may evolve, but v1 needs capabilities equivalent to:

- create workflow
- load workflow
- list workflows
- create run
- persist plan and steps
- query ready steps
- load step slice by selectors
- commit step result with version checking
- append workflow events
- mark step complete, failed, invalidated, canceled, or needs-replan
- record rerun and invalidation relationships
- list artifacts, facts, issues, and decisions
- advance or clear workflow cursor

Concurrency control should use optimistic versioning where practical.

---

## Step Slice Projection

The `StateProjector` is responsible for building the minimal step input for the LLM and executor.

For a given step, it should load:

- workflow goal
- current step definition
- dependency outputs
- selected relevant artifacts
- unresolved issues
- accepted decisions
- high-relevance facts
- bounded recent step-local history, if needed
- compressed historical context only as fallback

Projection must exclude:

- invalidated downstream outputs
- stale superseded step attempts
- unrelated workflow history

---

## Replanning Support

Repeated step failure should become an explicit workflow state transition.

Rules:

- each step has a persisted retry count
- after a configured threshold, the step becomes `needs_replan`
- completed steps remain immutable
- replanning may replace only the remaining suffix of the workflow or create a new run/branch
- all replanning decisions are persisted in events and decision records

The initial UI policy may still cancel the workflow instead of editing the active plan, but the store must represent the `needs_replan` state so the runtime and UI can respond cleanly.

---

## Dependency-Aware Invalidation

When an upstream completed step is rerun, all downstream dependent steps must be marked invalidated until rerun or replaced.

Rules:

- prior outputs remain stored for audit
- invalidated outputs are excluded from prompt projection
- rerun APIs must support:
  - rerun one failed step
  - rerun from a chosen step forward
  - rerun all invalidated dependents

Invalidation must be explicit and queryable.

---

## API Surface

The HTTP API should become workflow-aware.

Recommended endpoints:

- `GET /api/workflows`
- `GET /api/workflows/{workflow_id}`
- `GET /api/workflows/{workflow_id}/steps`
- `GET /api/workflows/{workflow_id}/events`
- `GET /api/workflows/{workflow_id}/facts`
- `GET /api/workflows/{workflow_id}/issues`
- `GET /api/workflows/{workflow_id}/decisions`
- `POST /api/workflows/{workflow_id}/resume`
- `POST /api/workflows/{workflow_id}/cancel`
- `POST /api/workflows/{workflow_id}/rerun-step`
- `POST /api/workflows/{workflow_id}/rerun-invalidated`

These endpoints are intended to support:

- inspection
- pause and resume
- replay and rerun
- cross-session collaboration
- TUI workflow navigation

---

## Telemetry and Audit Trail

The external state store should become the durable companion to framework telemetry.

Persist at minimum:

- workflow lifecycle events
- planner output events
- step start and finish
- tool execution records
- retries and recoveries
- verification results
- invalidation events
- replanning events

This supports:

- explainability
- debugging
- audit trail
- agent tuning metrics

Examples of metrics enabled by structured persistence:

- retry rates by step and tool
- tool failure hotspots
- token usage by step
- average time to verification
- invalidation frequency
- replanning frequency

---

## Cross-Session Collaboration

Cross-session collaboration becomes a supported workflow capability once state is store-backed.

Requirements:

- multiple clients can inspect the same workflow safely
- workflow status and events are queryable without accessing in-memory runtime state
- read/write behavior must respect concurrency constraints
- the TUI and API should operate against the same workflow store model

This does not require concurrent editing of one active step in v1, but it must support concurrent inspection and controlled resume/rerun operations.

---

## Compatibility and Migration

### Existing checkpointing

Checkpointing remains useful for:

- low-level graph recovery
- debugging
- crash forensics

Checkpoint snapshots are not the primary architect workflow persistence mechanism after this rework.

### Existing context management

History compression and the context manager remain supported.

Changes:

- durable workflow state moves to the store
- prompt-building prefers structured projection over replaying full history
- `core.Context` becomes step-local scratch state for architect execution

### Existing tests

Several tests will need rewriting because they currently assume shared mutable context across steps or checkpoint-centric resume semantics.

---

## Rollout Plan

### Phase 1: Store foundation

- add SQLite-backed workflow schema and migrations
- add store interfaces
- add persistence unit tests
- add artifact dual-representation support

### Phase 2: Architect runtime migration

- refactor `ArchitectAgent` to use workflow store as source of truth
- execute each step with fresh ephemeral context
- implement workflow-native resume
- persist audit events and telemetry linkage

### Phase 3: Replay, invalidation, and replanning

- add rerun support
- add dependency-aware invalidation
- add repeated-failure `needs_replan` handling
- add projector support for facts, issues, and decisions

### Phase 4: API and TUI workflow integration

- add workflow inspection endpoints
- add resume, cancel, and rerun endpoints
- integrate TUI workflow navigation and inspection

### Phase 5: broader adoption

- extend store-backed stepwise execution patterns to other suitable long-running agents
- keep `EternalAgent` out of scope unless reapproved separately

---

## Testing Requirements

### Unit tests

Add or update unit tests for:

- SQLite schema creation and migration
- workflow CRUD and run creation
- plan and step persistence
- step slice projection correctness
- optimistic concurrency/version checks
- artifact storage with summary and raw representations
- facts/issues/decisions persistence and query behavior
- invalidation propagation
- rerun selection logic
- repeated-failure replanning trigger
- API handlers for listing, inspection, resume, rerun, and cancel

### Integration tests

Add integration tests for:

- full architect workflow execution using the store
- workflow resume after process restart
- rerunning an upstream step invalidates dependents
- repeated step failure enters `needs_replan`
- prompt projection excludes invalidated outputs
- cross-session workflow inspection through API or TUI-facing adapter paths
- telemetry records align with persisted workflow events
- history compression still works as fallback when structured state is sparse

### Regression tests

Update existing tests that currently assume:

- shared in-memory context is authoritative across steps
- checkpoint snapshot restore is the primary architect resume mechanism
- server request execution can safely merge workflow-local state into a shared global context

---

## Risks

Primary risks:

- mixing persisted authoritative state with legacy in-memory state
- storing only compressed outputs and losing replay/debug value
- allowing replanning to mutate completed work implicitly
- exposing workflow APIs before concurrency and version semantics are defined
- under-testing invalidation and rerun semantics

Mitigations:

- make durable store ownership explicit in interfaces
- preserve raw artifact view alongside prompt summaries
- keep completed steps immutable
- use optimistic concurrency/versioning
- ship architect-first with comprehensive unit and integration tests

---

## Definition of Done

This rework is complete for v1 when all of the following are true:

- `architect` mode runs from a durable workflow store as the source of truth
- each step executes with a fresh ephemeral `core.Context`
- workflow-native pause and resume work without depending on raw checkpoint snapshots
- selective rerun and dependency invalidation are implemented
- persisted `facts`, `issues`, and `decisions` drive prompt projection
- artifacts retain both compressed summaries and raw payload or raw reference
- workflow inspection APIs are available
- telemetry and audit records are durably queryable
- unit and integration tests cover store, projection, resume, invalidation, rerun, and replanning behavior

