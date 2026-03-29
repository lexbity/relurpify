# Euclo Rework Plan

Status

Proposed full-product implementation plan.

Scope

This document converts the research/specification work in:

- [4.md](/home/lex/Public/Relurpify/docs/research/4.md)
- [5.md](/home/lex/Public/Relurpify/docs/research/5.md)
- [6.md](/home/lex/Public/Relurpify/docs/research/6.md)
- [7.md](/home/lex/Public/Relurpify/docs/research/7.md)
- [8.md](/home/lex/Public/Relurpify/docs/research/8.md)

into a detailed multi-phase implementation plan for Euclo.

This is not a minimum viable product plan.

It assumes the target is the full-product Euclo runtime:

- UX agnostic
- coding-focused
- toolkit-like
- able to collect context to execute a unit of work
- able to enter a heavy plan-backed path for long-running implementation
- integrated directly with Archaeo as a stable semantic substrate
- using relurpic capabilities as behavior-oriented reasoning units
- remaining effective on lower-end LLMs

## 1. Architecture Summary

### 1.1 Stable Boundaries

The intended architecture is:

- Archaeo requests and evaluates
- Euclo executes
- relurpic capabilities reason

The fixed constraint is:

- `archaeo` should not need modification

That means this plan must be implemented primarily in:

- `/named/euclo`
- `/agents/relurpic`
- `/framework` only where generic substrate support is needed

`/app/archaeo-graphql-server` remains an optional transport surface and is not the default in-process integration dependency.

### 1.2 Runtime Identity

Euclo should continue to behave like a coding toolkit that gathers context in order to execute a unit of work.

Euclo is not only a long-running plan executor.

Instead:

- all Euclo work begins as a unit of work
- some work units are light and direct
- some work units are investigative
- some work units are heavy plan-backed, long-running execution assemblies

The runtime identity should be:

- toolkit first
- explicit execution assembly second
- plan-backed long-horizon execution as one important path, not the only path

### 1.3 Core Runtime Objects

The target runtime model should revolve around the following Euclo-owned objects:

- `TaskEnvelope`
- `TaskClassification`
- `UnitOfWork`
- `CompiledExecution`
- `DeferredExecutionIssue`
- `ExecutionStatus`
- `ExecutionResultClass`

The intended flow is:

`Task -> TaskEnvelope -> TaskClassification -> Mode/Profile Presets -> UnitOfWork -> Execute -> CompiledExecution/Artifacts`

### 1.4 Single-Plan Execution Contract

For plan-backed long-running execution:

- one run binds to one fully formed plan
- Euclo does not create successor plans during execution
- execution-time unresolved concerns become deferrals
- conservative plan reconsideration belongs to a future archaeology re-entry, not the current run

### 1.5 Deferral Contract

Deferred execution issues are:

- Euclo-owned runtime artifacts
- both step-scoped and run-scoped
- persisted as workflow artifacts
- emitted as markdown plus YAML workspace artifacts
- reflected in the final result class as `completed_with_deferrals` when appropriate

## 2. Engineering Specification

### 2.1 Required Runtime Concepts

The implementation must support these concepts as first-class runtime state:

- `UnitOfWork`
- `CompiledExecution`
- `DeferredExecutionIssue`
- `ExecutionStatus`
- `ExecutionResultClass`
- `ContextLifecycleState`

### 2.2 UnitOfWork

`UnitOfWork` is the active execution assembly object.

It must include at minimum:

- identity
- mode and objective kind
- behavior family
- context strategy
- verification and deferral policy
- optional plan binding
- routine bindings
- skill bindings
- tool/capability bindings
- status
- result class
- deferred issue ids

`UnitOfWork` is Euclo-owned and should live in `named/euclo/runtime` or a dedicated Euclo runtime package.

### 2.3 CompiledExecution

`CompiledExecution` is the durable continuity object.

It must persist enough state for:

- resume
- restore after compaction
- final reporting
- run-level deferral indexing

It should be derived from `UnitOfWork` plus runtime progress state.

### 2.4 DeferredExecutionIssue

`DeferredExecutionIssue` is Euclo-owned and should capture:

- taxonomy:
  - `ambiguity`
  - `stale_assumption`
  - `pattern_tension`
  - `nonfatal_failure`
  - `verification_concern`
  - `provider_constraint`
- step scope
- run scope
- evidence payload
- Archaeo references
- workspace artifact path
- recommended archaeology re-entry

### 2.5 ExecutionResultClass

Execution results should become a top-level Euclo classification with at minimum:

- `completed`
- `completed_with_deferrals`
- `blocked`
- `failed`
- `canceled`
- `restore_failed`

This should coexist with older capability-oriented statuses during migration.

### 2.6 Context Lifecycle

Long-running plan-backed execution must tolerate context clearing and compaction.

The runtime must track:

- current context lifecycle state
- restore requirements
- restore summaries
- continuity artifacts sufficient to continue without rebuilding the run from prose context alone

### 2.7 Relurpic Behavior Families

Relurpic capability families should be treated as behavior-oriented execution routines.

The initial core families are:

- `gap_analysis`
- `verification_repair`
- `scope_expansion_assessment`
- `stale_assumption_detection`
- `tension_assessment`

These should be selectable from mode, step, runtime state, and `UnitOfWork`.

### 2.8 Archaeo Integration Contract

Euclo and relurpic capabilities should integrate with Archaeo directly through existing:

- services
- bindings
- projections
- request lifecycle flows

This work must not require:

- new Archaeo domain types
- new Archaeo persistence schema
- GraphQL-first local integration

## 3. Current State Assessment

### 3.1 What Already Exists

The current codebase already has useful precursors:

- `TaskEnvelope` and `TaskClassification` in `named/euclo/runtime`
- execution/session/finalizer bindings into Archaeo
- artifact plumbing in `named/euclo/euclotypes`
- profile-driven orchestration
- interaction state and resumability
- existing relurpic capability implementations

### 3.2 Main Gaps

The main gaps this plan addresses are:

- Euclo still lacks an explicit `UnitOfWork` object
- runtime semantics are spread across mode/profile/orchestrate/interaction/session layers
- compiled execution is not yet formalized as a strong continuity object
- deferrals are not yet formalized as Euclo-owned runtime artifacts with workspace persistence
- final result classes are still capability-oriented rather than runtime-oriented
- relurpic behavior families are not yet cleanly expressed as execution-family abstractions

## 4. Package-Level Implementation Targets

### 4.1 Named Euclo

Primary targets:

- `named/euclo/agent.go`
- `named/euclo/runtime/*`
- `named/euclo/euclotypes/*`
- `named/euclo/orchestrate/*`
- optionally new packages:
  - `named/euclo/workunit`
  - `named/euclo/executionstate`

### 4.2 Agents Relurpic

Primary targets:

- `agents/relurpic/*`

Focus:

- behavior-family organization
- Archaeo-first direct integration
- reducing reliance on raw handler-plus-store composition where stronger existing bindings already exist

### 4.3 Framework

Possible targets only if generic support is necessary:

- artifact persistence helpers
- generic runtime state helpers
- context lifecycle helpers

### 4.4 Archaeo

No required modification.

## 5. Multi-Phase Plan

## Phase 1: Runtime Type Foundation

Goal

Introduce the Euclo-owned runtime type surface required for the rework without yet changing the full execution path.

Deliverables

- define `UnitOfWork`
- define `UnitOfWorkPlanBinding`
- define `UnitOfWorkContextBundle`
- define `UnitOfWorkRoutineBinding`
- define `DeferredExecutionIssue`
- define `ExecutionResultClass`
- define richer `ExecutionStatus` for runtime state
- preserve compatibility with existing capability-level `ExecutionStatus` where necessary

Target packages

- `named/euclo/runtime`
- `named/euclo/euclotypes`

Dependencies

- prior work: none
- external packages:
  - `framework/*` current runtime/state primitives
  - existing `euclotypes.CapabilitySnapshot`
  - existing Archaeo reference ids only as field shapes, not as new dependencies

Engineering notes

- `TaskEnvelope` and `TaskClassification` remain intake-stage types
- `UnitOfWork` is added after classification and profile selection, not as a replacement
- avoid polluting `euclotypes/types.go` with all runtime-heavy structures if a dedicated runtime package is clearer

Tests

- unit tests for type conversions and helper constructors
- serialization round-trip tests for:
  - `UnitOfWork`
  - `DeferredExecutionIssue`
  - `ExecutionResultClass`
- compatibility tests proving older capability results still map into the new runtime types

Exit criteria

- new runtime types exist
- old type usage still compiles cleanly
- no Archaeo changes are required

## Phase 2: UnitOfWork Assembly Pipeline

Goal

Make Euclo explicitly assemble a `UnitOfWork` after intake/classification instead of relying only on mode/profile and scattered runtime keys.

Deliverables

- add `BuildUnitOfWork(...)` or equivalent assembly flow
- derive `UnitOfWork` from:
  - `TaskEnvelope`
  - `TaskClassification`
  - mode resolution
  - profile/preset selection
  - runtime state
  - optional living-plan context
- add explicit context strategy and behavior-family selection
- attach routine/skill/tool/capability bindings to the assembled work unit

Target packages

- `named/euclo/runtime`
- `named/euclo/agent.go`
- `named/euclo/orchestrate`

Dependencies

- Phase 1 runtime types
- existing:
  - `NormalizeTaskEnvelope`
  - `ClassifyTask`
  - `ResolveMode`
  - `SelectExecutionProfile`

Engineering notes

- profiles should become presets/hints into `UnitOfWork` assembly rather than the deepest execution abstraction
- the initial migration may keep profile selection intact while layering `UnitOfWork` on top

Tests

- unit tests for `BuildUnitOfWork(...)`
- classification-to-work-unit mapping tests for:
  - direct execution
  - debug/investigation
  - plan-backed execution
- tests proving plan-backed `UnitOfWork` binds a single plan version
- regression tests preserving current classification/profile behavior where still intended

Exit criteria

- Euclo can assemble a `UnitOfWork` for all supported high-level paths
- runtime code can inspect a `UnitOfWork` instead of inferring everything from scattered state

## Phase 3: CompiledExecution And Runtime Persistence

Goal

Introduce a Euclo-owned durable continuity object derived from the active `UnitOfWork`.

Deliverables

- define `CompiledExecution`
- derive/persist `CompiledExecution` from `UnitOfWork`
- persist execution status separately from capability-local status
- add workflow artifact persistence for:
  - compiled execution
  - execution status
  - restore summary

Target packages

- `named/euclo/runtime`
- `named/euclo/euclotypes`
- `named/euclo/agent.go`

Dependencies

- Phase 1 and 2
- existing workflow artifact persistence mechanisms
- existing runtime workflow store access

Engineering notes

- this must be Euclo-owned, not Archaeo-owned
- compiled execution remains bound to exactly one plan version for a run
- it should be persistence-facing, while `UnitOfWork` is runtime-facing

Tests

- round-trip persistence tests for `CompiledExecution`
- restore seed tests from persisted artifacts
- tests proving compiled execution remains bound to one plan version for the life of a run
- tests for execution status artifact updates over run transitions

Exit criteria

- Euclo can persist and reload compiled execution state without Archaeo changes

## Phase 4: Deferred Execution Issue Model

Goal

Implement Euclo-owned deferral semantics as first-class runtime artifacts.

Deliverables

- create `DeferredExecutionIssue` generation helpers
- implement issue taxonomy
- implement step-scoped and run-scoped issue association
- persist workflow artifacts for deferred issues
- emit markdown plus YAML workspace artifacts
- maintain run-level issue indexes

Target packages

- `named/euclo/runtime`
- `named/euclo/agent.go`
- optionally new:
  - `named/euclo/executionstate`

Dependencies

- Phase 1 and 3
- existing workflow artifact persistence
- existing filesystem/write capabilities in Euclo execution path

Engineering notes

- deferrals are Euclo-owned even when they contain Archaeo references
- deferrals should not trigger in-run plan mutation or successor plan creation

Tests

- issue creation tests for each taxonomy value
- artifact emission tests for markdown plus YAML structure
- run-level aggregation tests
- step-scoped attribution tests
- tests proving evidence payloads are captured
- tests proving Archaeo refs are preserved but no Archaeo changes are required

Exit criteria

- Euclo can create, persist, and surface deferred issues as durable runtime artifacts

## Phase 5: Result Class And Finalization Rework

Goal

Promote runtime result classes to first-class Euclo outputs and fold deferral state into final run classification.

Deliverables

- add `ExecutionResultClass`
- compute:
  - `completed`
  - `completed_with_deferrals`
  - `blocked`
  - `failed`
  - `canceled`
  - `restore_failed`
- update finalization and final report assembly to include result class
- preserve compatibility with existing capability execution result shapes where needed

Target packages

- `named/euclo/agent.go`
- `named/euclo/runtime`
- `named/euclo/euclotypes`
- `named/euclo/orchestrate`

Dependencies

- Phase 3 and 4
- existing verification and success-gate flows

Engineering notes

- separate capability-level status from runtime-level result class
- final reporting should explicitly distinguish normal completion from completion with unresolved deferrals

Tests

- finalization tests for each result class
- regression tests for existing successful and failed runs
- tests proving `completed_with_deferrals` is surfaced when appropriate
- tests ensuring final reports include deferred issue references

Exit criteria

- result classes are first-class runtime outputs
- capability-level status no longer has to carry all runtime meaning alone

## Phase 6: Context Lifecycle, Compaction, And Restore

Goal

Implement the hard guarantee that long-running plan-backed execution can survive context clearing/compaction and continue safely.

Deliverables

- explicit context lifecycle state tracking
- restore summary artifact
- restore orchestration from:
  - compiled execution
  - workflow artifact state
  - existing Archaeo projections/refs
- work-unit reconstruction after compaction

Target packages

- `named/euclo/runtime`
- `named/euclo/agent.go`
- possibly `/framework` for generic helpers only

Dependencies

- Phase 2 and 3
- existing Archaeo projections and workflow state
- existing persisted artifact and runtime store surfaces

Engineering notes

- restore orchestration remains Euclo-owned
- Archaeo remains only the semantic substrate
- no new Archaeo restore semantics should be required

Tests

- restore-after-compaction integration tests
- tests proving plan-backed execution resumes with the same bound plan
- tests ensuring deferred issue visibility survives restore
- tests for `restore_failed` classification
- stress tests for long-running run continuity with repeated compaction cycles

Exit criteria

- context clearing/compaction no longer implies long-running execution loss

## Phase 7: Relurpic Behavior Family Rework

Goal

Refactor relurpic execution behavior around explicit behavior families and direct Archaeo-first reasoning integration.

Deliverables

- organize core behavior families:
  - `gap_analysis`
  - `verification_repair`
  - `scope_expansion_assessment`
  - `stale_assumption_detection`
  - `tension_assessment`
- bind these families explicitly into `UnitOfWork`
- clean up provider/routine wiring to reduce raw handler-plus-store coupling where stronger existing bindings exist

Target packages

- `agents/relurpic`
- `named/euclo/runtime`
- `named/euclo/orchestrate`

Dependencies

- Phase 2
- existing relurpic implementations
- existing Archaeo services/bindings

Engineering notes

- this phase should not be GraphQL-first
- use existing direct Archaeo services/bindings as the primary integration path
- the goal is behavior-oriented runtime composition, not user-facing capability labeling

Tests

- unit tests for each behavior family selection path
- integration tests proving routine families can read existing Archaeo state without new Archaeo APIs
- regression tests for current relurpic provider behavior
- tests for work-unit family binding and execution selection

Exit criteria

- relurpic behaviors are legible execution families rather than only ad hoc capability entry points

## Phase 8: Euclo Execution Ownership Cleanup

Goal

Make Euclo a more self-owning runtime with one clearer execution owner and less architectural ambiguity across delegate/orchestrate/interaction/session layers.

Deliverables

- reduce or isolate oversized generic delegate identity inside Euclo
- make `UnitOfWork` and runtime execution state the central owner of the execution flow
- simplify boundaries between:
  - Euclo runtime
  - orchestrate
  - interaction
  - session/finalization

Target packages

- `named/euclo/agent.go`
- `named/euclo/orchestrate`
- `named/euclo/interaction`
- `agents/react` only where compatibility seams still require it

Dependencies

- Phase 2 through 7

Engineering notes

- this is not a full rewrite
- the goal is to make the ownership of execution flow more legible and Euclo-native
- compatibility layers may remain during migration, but the runtime should no longer feel like a composition of unrelated owners

Tests

- end-to-end Euclo runtime tests for:
  - direct execution
  - debug/investigation
  - plan-backed long execution
  - completion with deferrals
  - restore after compaction
- regression tests preserving interaction and orchestrate behavior where still intended

Exit criteria

- Euclo reads as the clear runtime owner of coding execution

## Phase 9: Full-System Reliability And Benchmark Program

Goal

Validate the full-product runtime against the benchmark scenarios and lower-end model goals.

Deliverables

- benchmark suite for:
  - failing test to fix
  - multi-step implementation against a living plan
  - compatibility-preserving refactor
  - long-running migration
  - restore after compaction
- weaker-model validation profiles
- documentation updates for runtime behavior and guarantees

Target packages

- `named/euclo/euclotest`
- `named/euclo/benchmark`
- relurpic integration tests
- optionally app/runtime integration tests where behavior is observable through current shell/runtime APIs

Dependencies

- all prior phases

Engineering notes

- this phase is where product claims are validated
- the benchmark program should measure correctness, continuity, and clarity of deferred issue handling, not just raw speed

Tests

- benchmark workloads
- end-to-end integration tests
- resilience tests under repeated context compaction
- provider degradation tests where safe continuation is expected

Exit criteria

- the runtime can be defended as reliable, plan-capable, and continuity-preserving even on lower-end models

## 6. Dependency Matrix Summary

### 6.1 Core sequencing

- Phase 1 -> required by all later phases
- Phase 2 -> required by 3, 6, 7, 8
- Phase 3 -> required by 4, 5, 6
- Phase 4 -> required by 5, 6, 9
- Phase 5 -> required by 9
- Phase 6 -> required by 9
- Phase 7 -> required by 8 and 9
- Phase 8 -> required by 9 for final architecture stabilization

### 6.2 Package dependency emphasis

- `named/euclo/runtime` is the primary implementation center
- `named/euclo/agent.go` is the primary orchestration migration center
- `agents/relurpic` is the primary behavior-family rework center
- `framework` should only be touched for generic substrate helpers
- `archaeo` should not need modification

## 7. Testing Strategy

### 7.1 Unit test priorities

- runtime type conversions
- `UnitOfWork` assembly
- compiled execution persistence
- deferred issue generation and serialization
- result class classification

### 7.2 Integration test priorities

- direct execution paths
- plan-backed long-running execution
- deferral persistence and final reporting
- restore after compaction
- relurpic behavior-family execution

### 7.3 Regression priorities

- current classification/profile behavior
- current interaction and orchestrate flow where still intended
- current relurpic capability behavior

### 7.4 Benchmark priorities

- weaker-model behavior
- long-running continuity
- plan-backed execution stability
- completion-with-deferrals correctness

## 8. Non-Goals

This plan does not include:

- redesigning Relurpish UX
- making GraphQL the local integration path
- changing Archaeo domain model or persistence
- rewriting Euclo from scratch

## 9. Expected Outcome

If completed, this plan should leave the codebase with:

- an explicit `UnitOfWork` runtime object
- Euclo-owned runtime artifacts and result classes
- durable deferrals as discovered execution knowledge
- context restore for long-running plan-backed runs
- relurpic behavior families as legible execution routines
- a more self-owning Euclo runtime with clearer architectural boundaries

That is the target full-product rework state.
