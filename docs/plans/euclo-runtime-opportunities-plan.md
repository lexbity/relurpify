# Euclo Runtime Opportunities Plan

Status

Proposed full-product implementation plan.

Scope

This document converts the current review of Euclo, `/agents`, `/archaeo`, and `/framework` into a detailed multi-phase engineering plan.

It is not a minimum viable product plan.

It focuses on the remaining engineering opportunities needed to move Euclo closer to the intended vision:

- Euclo as the UX-agnostic coding runtime
- Archaeo as the semantic memory / provenance / living-plan substrate
- relurpic capabilities as behavior-oriented reasoning units
- `/agents` as reusable execution paradigms rather than Euclo’s implicit identity
- `/framework` as the substrate for context, capability, provider, policy, and restore mechanics

It also resolves the current architectural feedback:

- executor families are still mostly compatibility shells
- semantic inputs are still mostly projection/ref level
- provider restore is persisted but not fully replayed
- relurpic providers still expose handler-shaped composition

## 1. Current Assessment

### 1.1 What Is Real Today

Euclo now has a credible runtime shell:

- `TaskEnvelope`
- `TaskClassification`
- `UnitOfWork`
- `CompiledExecution`
- `DeferredExecutionIssue`
- runtime `ExecutionStatus`
- runtime `ExecutionResultClass`
- context lifecycle state
- provider snapshot persistence metadata
- semantic input bundles for `planning` and `debug`
- direct framework skill-policy resolution into runtime state
- executor descriptors and explicit executor family selection

This is meaningful progress.

### 1.2 Main Remaining Gaps

The main remaining gaps are:

- executor family selection exists, but most executor families still route through compatibility execution
- Euclo consumes semantic refs and projections, but not enough evaluated semantic outputs
- provider snapshot/session persistence exists, but actual provider restore hooks are not executed
- framework context manager support is summarized, not actively orchestrated
- relurpic capability composition is still too handler-shaped
- mode-specific Euclo reasoning families are only partially implemented
- pattern / tension / coherence / prospective reasoning remains below the research vision

## 2. Architecture Targets

### 2.1 Stable Layering

The implementation target remains:

- `/framework`: context, capabilities, provider/runtime substrate, policy, restore, security
- `/agents`: reusable execution paradigms and generic orchestration patterns
- `/archaeo`: requests, evaluation, learning, tensions, plan memory, provenance, projections
- `/named/euclo`: coding-runtime orchestration and execution ownership
- `/agents/relurpic`: behavior-oriented reasoning families and provider-side composition

### 2.2 Required Runtime Outcome

The target state after this plan is:

- Euclo owns execution semantics, not only execution descriptors
- semantic inputs are richer and more execution-relevant
- restore means runtime restoration, not just artifact restoration
- relurpic reasoning families are explicit runtime contracts
- executor families remain layered over `/agents`, but Euclo selects and drives them deliberately

## 3. Engineering Specification

### 3.1 Executor Semantics

Executor families must become first-class execution behaviors.

At minimum the runtime must support these families:

- `react_executor`
- `planner_executor`
- `htn_executor`
- `rewoo_executor`
- `reflection_executor`

The contract is:

- executor selection is driven by `UnitOfWork`
- each executor family may share common Euclo runtime scaffolding
- each executor family must be able to diverge in execution semantics where appropriate
- compatibility routing remains only as a migration mechanism, not the end state

### 3.2 Semantic Input Deepening

`SemanticInputBundle` must evolve beyond projection references.

It should support:

- request refs
- completed request refs
- evaluated pattern refs
- evaluated tension refs
- evaluated prospective refs
- convergence review refs
- learning interaction refs
- provenance refs
- optional typed summaries of evaluated outputs where safe

Euclo must remain Archaeo-first:

- use existing Archaeo services, requests, learning, tensions, projections, and bindings
- do not require Archaeo schema changes

### 3.3 Provider Restore Contract

Provider restore must become a runtime action, not just a persisted record.

Euclo should:

- persist provider snapshots and session snapshots
- reload them during continuity restore
- invoke framework restore interfaces where providers support them
- surface restore failures as runtime-classified failures

Required framework interfaces:

- `ProviderSnapshotter`
- `ProviderSessionSnapshotter`
- `ProviderRestorer`
- `ProviderSessionRestorer`

### 3.4 Context Lifecycle Contract

Euclo must actively orchestrate context management for long-running work.

This includes:

- selecting a `contextmgr` strategy from skill/runtime policy
- managing compaction thresholds
- preserving semantic and plan identity during compaction
- progressive loading for high-context execution
- linking derivation loss and compaction state back into runtime status

### 3.5 Relurpic Runtime Contract

relurpic capabilities should be expressed as explicit behavior families.

The initial runtime behavior families should include:

- `pattern_surface_and_confirm`
- `prospective_structure_assessment`
- `convergence_guard`
- `gap_analysis`
- `verification_repair`
- `stale_assumption_detection`
- `tension_assessment`
- `coherence_assessment`
- `scope_expansion_assessment`

These are runtime-oriented reasoning contracts.

They are not the same thing as user-facing modes or profile names.

### 3.6 Modal Runtime Requirement

Euclo is modal, not only multi-paradigm.

The implementation must preserve:

- direct collect-context plus toolbox execution
- plan-backed long-context execution
- debug/investigative execution
- review/correction execution

Modes remain meaningful runtime distinctions even when multiple executor families are available.

### 3.7 Security And Manifest Constraint

All new behavior must remain subject to:

- capability policy
- manifest/runtime policy
- tool permission constraints
- provider trust/recoverability constraints
- sub-agent orchestration under the same security model

No executor or relurpic routine may bypass framework policy just because it is nested.

## 4. Phase Plan

## Phase 1: Executor Runtime Split

### Goal

Make executor families affect real runtime behavior rather than only graph identity and descriptors.

### Changes

- Introduce a richer Euclo-owned executor contract in `named/euclo`.
- Split execution behavior into:
  - shared Euclo runtime scaffolding
  - executor-family-specific preparation/execution/finalization hooks
- Keep plan/deferral/result/report scaffolding centralized in Euclo.
- Move compatibility routing into temporary adapters.
- Implement first real native executor paths for:
  - `reflection_executor`
  - `planner_executor`
  - `rewoo_executor`
- Keep `react_executor` as the valid collect-context/toolbox path.
- Keep HTN as a structured decomposition executor, not only a graph-builder choice.

### Package Targets

- `named/euclo/executors.go`
- `named/euclo/agent.go`
- new `named/euclo/executor_runtime.go`
- optional new `named/euclo/executor_*` files by family
- selective updates to `/agents/planner`, `/agents/rewoo`, `/agents/htn`, `/agents/reflection` integration paths

### Dependencies

- existing `UnitOfWork`
- existing `ResolvedExecutionPolicy`
- existing executor descriptors
- existing compiled execution and result class model

### Tests

- unit tests for executor selection by `UnitOfWork`
- unit tests for executor-family-specific preparation hooks
- integration tests proving:
  - `react_executor` keeps collect-context/toolbox behavior
  - `planner_executor` uses plan-first preparation semantics
  - `rewoo_executor` uses long-running context-aware execution semantics
  - `reflection_executor` preserves review/correction behavior
- regression tests proving current profile execution still works during migration

## Phase 2: Semantic Input Enrichment

### Goal

Deepen Euclo’s semantic inputs from “projection refs” into richer evaluated execution inputs.

### Changes

- Extend `SemanticInputBundle` with richer typed fields.
- Add a Euclo semantic prepass that:
  - for `planning` mode loads pattern/prospective/convergence inputs
  - for `debug` mode loads tension/pattern inputs
  - for `review` mode loads pattern/tension/coherence inputs where available
- Add Euclo-side adapters over existing Archaeo outputs so `UnitOfWork` can carry:
  - evaluated pattern summaries
  - tension summaries
  - prospective match summaries
  - convergence state summaries
- Persist semantic inputs into `CompiledExecution` and final reports.
- Ensure restore preserves semantic grounding.

### Package Targets

- `named/euclo/runtime/semantic_inputs.go`
- `named/euclo/agent.go`
- optional new `named/euclo/runtime/semantic_prepass.go`
- adapters over existing `archaeo/projections`, `archaeo/learning`, `archaeo/tensions`, `archaeo/verification`

### Dependencies

- existing Archaeo projections and request history
- existing semantic bundle support
- Phase 1 executor/runtime split

### Tests

- unit tests for richer semantic bundle assembly
- integration tests for:
  - `planning` mode with pattern/prospective/convergence inputs
  - `debug` mode with tension/pattern inputs
  - `review` mode with pattern/tension inputs
- restore tests proving semantic inputs survive compaction and resume
- final report tests proving semantic input provenance is surfaced

## Phase 3: Real Provider Snapshot And Session Restore

### Goal

Turn provider persistence into actual provider/runtime restoration.

### Changes

- Discover providers participating in the current Euclo runtime.
- When providers implement restore interfaces, invoke:
  - `SnapshotProvider`
  - `SnapshotSessions`
  - `RestoreProvider`
  - `RestoreSession`
- Add a restore coordinator in Euclo that:
  - restores provider state
  - restores provider sessions
  - records per-provider restore outcomes
  - classifies partial vs full restore outcomes
- Add runtime report fields for:
  - restored providers
  - restored sessions
  - failed provider restores
  - skipped providers due to missing recoverability

### Package Targets

- `named/euclo/runtime/provider_restore.go`
- `named/euclo/agent.go`
- optional `named/euclo/runtime/provider_runtime.go`
- framework-facing provider registry/runtime access points

### Dependencies

- existing provider snapshot persistence
- framework provider interfaces
- Phase 1 executor split
- workflow store support already in place

### Tests

- unit tests for provider restore coordinator
- fake-provider tests implementing snapshot/restore interfaces
- integration tests proving:
  - provider restore is attempted when snapshots exist
  - session restore is attempted when session snapshots exist
  - restore failures become runtime-visible
  - `restore_failed` result class occurs only when restore is materially required and cannot be satisfied

## Phase 4: Active Context Manager Integration

### Goal

Make Euclo actively use `framework/contextmgr` instead of only mirroring policy metadata.

### Changes

- Introduce Euclo runtime context policy construction from:
  - mode
  - skill policy
  - executor family
  - plan-backed vs direct task execution
- Instantiate and manage a real context manager for long-running work.
- Use progressive loader for large workspace context.
- Tie compaction events to:
  - derivation loss
  - lifecycle state
  - semantic preservation guarantees
- Make Rewoo and long-context executor families explicitly consume the same context policy/manager.

### Package Targets

- `named/euclo/runtime/execution_policy.go`
- `named/euclo/runtime/lifecycle.go`
- new `named/euclo/runtime/context_runtime.go`
- selective `/agents/rewoo` and `/agents/react` integration points

### Dependencies

- framework `contextmgr`
- Phase 1 executor split
- Phase 3 provider restore

### Tests

- unit tests for context policy derivation from mode/skill/executor
- compaction tests with derivation-loss-aware summaries
- progressive loading tests for plan-backed work
- integration tests proving long-running execution survives context pressure without losing plan/semantic identity

## Phase 5: Relurpic Runtime Service Layer

### Goal

Replace handler-shaped relurpic composition with stable runtime services for Euclo-facing reasoning behavior.

### Changes

- Add a relurpic runtime service layer above handler internals.
- Refactor provider implementations in `/agents/relurpic` to depend on stable services rather than raw handler structs.
- Define clear runtime interfaces for:
  - pattern surfacing
  - tension assessment
  - prospective analysis
  - convergence assessment
  - gap analysis
  - verification repair
- Ensure Euclo depends on these behavior interfaces rather than handler details.

### Package Targets

- `agents/relurpic/providers.go`
- new `agents/relurpic/runtime/*`
- `archaeo/adapters/relurpic/service.go`
- optional additional relurpic tests and adapters

### Dependencies

- Phase 2 semantic enrichment
- existing archaeology provider bundle
- no Archaeo schema changes

### Tests

- unit tests for relurpic runtime services
- compatibility tests proving provider bundle behavior is unchanged externally
- integration tests proving Euclo can invoke relurpic behavior families without handler coupling

## Phase 6: Mode-Scoped Relurpic Capability Completion

### Goal

Complete the mode-specific relurpic reasoning families that Euclo still lacks.

### Changes

- Define per-mode required and optional relurpic behavior families.
- For direct collect-context mode:
  - preserve ReAct-like toolbox execution
  - add optional targeted gap/verification routines
- For `planning` mode:
  - complete pattern/prospective/convergence/gap/coherence families
- For `debug` mode:
  - complete tension/stale-assumption/verification-repair families
- For `review` mode:
  - complete tension/coherence/compatibility/approval families
- Add routine-selection logic driven by:
  - mode
  - step
  - semantic inputs
  - policy
  - executor family

### Package Targets

- `named/euclo/runtime/workunit.go`
- `named/euclo/runtime/execution_policy.go`
- `/agents/relurpic`
- relevant Euclo capability packages

### Dependencies

- Phase 2 semantic enrichment
- Phase 5 relurpic runtime services

### Tests

- mode behavior tests proving routine family selection
- capability/routine cooperation tests
- integration tests proving different modes surface different reasoning families under the same framework policy model

## Phase 7: Pattern And Tension Reasoning Upgrade

### Goal

Move closer to the research vision of pattern surfacing, tension/coherence analysis, and possibility-space support.

### Changes

- Introduce a Euclo-owned pattern/tension/coherence reasoning layer over existing Archaeo records.
- Extend semantic-input bundle and reports with:
  - pattern proposals
  - tension clusters
  - coherence suggestions
  - prospective pairings
- Add execution-time reasoning hooks for:
  - stale assumption detection at step boundaries
  - scope expansion assessment
  - coherence concerns tied to touched symbols
- Connect these outputs to deferral artifacts where inline resolution is not appropriate.

### Package Targets

- new `named/euclo/runtime/pattern_reasoning.go`
- new `named/euclo/runtime/tension_reasoning.go`
- relurpic runtime services
- Euclo final report assembly

### Dependencies

- Phase 2 semantic enrichment
- Phase 5 relurpic runtime services
- Phase 6 mode-scoped capability completion

### Tests

- reasoning-layer unit tests
- deferral tests proving pattern/tension issues become artifacted deferrals
- report tests proving surfaced reasoning survives into final output

## Phase 8: Security, Manifest, And Shared-Context Hardening

### Goal

Raise the runtime quality bar around policy enforcement and shared-context sub-agent orchestration.

### Changes

- Verify executor families and relurpic routines all honor capability/tool policy.
- Ensure nested sub-agent execution remains subject to the same manifest constraints.
- Add explicit shared-context orchestration support for cooperating executor/routine combinations.
- Ensure restored sessions/providers do not widen permissions implicitly.
- Add runtime diagnostics for:
  - denied capability usage
  - denied tool usage
  - downgraded provider trust
  - provider recoverability mismatch

### Package Targets

- `named/euclo/agent.go`
- `named/euclo/executors.go`
- runtime policy logic
- framework/provider integration points

### Dependencies

- all prior phases
- framework policy/security model

### Tests

- policy enforcement tests across executors
- nested sub-agent permission tests
- shared-context continuity tests
- provider trust/recoverability tests

## 5. Cross-Phase Test Plan

### 5.1 Contract Tests

Maintain and expand tests for:

- `UnitOfWork`
- `CompiledExecution`
- `DeferredExecutionIssue`
- `SemanticInputBundle`
- `ResolvedExecutionPolicy`
- `WorkUnitExecutorDescriptor`
- context lifecycle state
- provider restore state

### 5.2 Modal Runtime Tests

Maintain and expand tests for:

- collect-context plus toolbox execution
- plan-backed execution
- debug/investigation behavior
- review/correction behavior
- mode-driven semantic input requirements

### 5.3 Orchestration Tests

Maintain and expand tests for:

- multi-capability participation in one unit of work
- shared context across cooperating agents/routines
- executor family and relurpic routine cooperation
- deferral creation under orchestration

### 5.4 Security And Policy Tests

Maintain and expand tests for:

- manifest-driven capability narrowing
- tool permission enforcement
- sub-agent policy inheritance
- provider trust and restore policy enforcement

### 5.5 Continuity Tests

Maintain and expand tests for:

- compiled execution persistence
- semantic-input persistence
- provider snapshot/session persistence
- active restore behavior
- context compaction and restoration
- restored executor identity and result classification

### 5.6 Benchmarks

Local engineering benchmarks should cover:

- direct execution assembly cost
- semantic prepass cost
- executor selection overhead
- compiled execution persistence cost
- provider restore bookkeeping cost
- context compaction / restore overhead
- relurpic reasoning family invocation overhead

These remain local engineering benchmarks and do not assume live LLMs.

## 6. Package Dependencies Summary

### `/named/euclo`

Primary owner of:

- runtime orchestration
- work unit assembly
- executor selection and execution semantics
- continuity / result / reporting
- deferral ownership

### `/agents`

Used as:

- execution paradigm substrate
- reusable sub-agent behaviors
- context-aware execution implementations

### `/agents/relurpic`

Used as:

- behavior-oriented reasoning families
- archaeology-facing provider bundle implementations
- Euclo runtime reasoning support

### `/archaeo`

Used as:

- request lifecycle
- learning and tension storage
- projections and provenance
- plan memory and evaluation substrate

No Archaeo schema changes are assumed.

### `/framework`

Used as:

- capability and security model
- context manager and progressive loading
- provider snapshot/restore interfaces
- skill-policy resolution
- runtime/provider trust constraints

## 7. Definition Of Done

This plan is complete when:

- executor family selection corresponds to real runtime behavior
- semantic inputs are richer and materially affect execution
- provider restore executes through framework interfaces where supported
- Euclo actively manages context through framework context infrastructure
- relurpic reasoning families are explicit runtime contracts
- mode-specific relurpic capability coverage is substantially complete
- policy/security constraints hold across direct and nested execution
- Euclo remains effective on lower-end models with strong continuity and deferral behavior

## 8. Expected Outcome

After this plan, Euclo should feel like:

- a true coding runtime rather than a stronger orchestration shell
- a modal execution toolkit
- a runtime that can both collect context directly and execute against a heavier semantic substrate
- a system that compounds project knowledge over time without depending on ephemeral prompt state
- a runtime that uses `/agents`, `/archaeo`, and `/framework` deliberately rather than incidentally
