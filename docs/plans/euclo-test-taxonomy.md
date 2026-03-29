# Euclo Test Taxonomy

Status

Proposed engineering taxonomy for Euclo-focused testing.

Scope

This document defines how Euclo should be tested as:

- a modal coding runtime
- a layered runtime built on `/agents`, `/archaeo`, and `/framework`
- a capability-centered orchestration system
- a runtime that supports multiple execution paradigms without collapsing into one
- a runtime that must remain effective without assuming live LLM access in local engineering tests

Canonical primary relurpic capability owners for local testing:

- `euclo:chat.ask`
- `euclo:chat.implement`
- `euclo:chat.inspect`
- `euclo:archaeology.explore`
- `euclo:archaeology.compile-plan`
- `euclo:archaeology.implement-plan`
- `euclo:debug.investigate`

This document is not the full end-to-end LLM testsuite plan.

The full live end-to-end suite remains in `/testsuite` and is out of scope here.

## 1. Testing Position

Euclo should not be tested only as:

- a single-agent coding wrapper
- a single execution paradigm
- a prompt-in / text-out system

Euclo should be tested as:

- a modal runtime
- a capability orchestrator
- a context-collecting coding toolkit
- a plan-backed long-running executor when required
- a manifest-constrained runtime under the framework security model

Euclo is more than a multi-style runtime.

Its behavior is shaped by:

- mode
- unit of work
- relurpic behavior family
- capability/tool/skill policy
- archaeology-backed semantic state when relevant

So the taxonomy must validate both:

- paradigm selection
- modal runtime behavior

## 2. Test Layers

The Euclo-local suite should be split into six layers.

### 2.1 Runtime Contract Tests

Purpose:
Validate stable Euclo-owned runtime contracts.

Primary subjects:

- `TaskEnvelope`
- `TaskClassification`
- `UnitOfWork`
- `CompiledExecution`
- `DeferredExecutionIssue`
- `ExecutionStatus`
- `ExecutionResultClass`
- `ContextLifecycleState`
- `SemanticInputBundle`
- `ResolvedExecutionPolicy`
- `WorkUnitExecutorDescriptor`

These tests should prefer:

- deterministic local state
- no live model calls
- typed assertions over textual output assertions

These are the highest-stability tests and should define the baseline runtime contract.

### 2.2 Modal Behavior Tests

Purpose:
Validate that Euclo’s modes produce the correct runtime behavior and execution posture.

Current focus:

- direct collect-context plus toolbox mode
- heavy plan-backed mode

Later expansion:

- deeper review mode behavior
- richer debug/investigation mode behavior
- additional modes as they become real runtime distinctions

These tests should assert:

- mode resolution behavior
- primary relurpic capability owner selection
- supporting relurpic capability assembly
- context strategy
- semantic input requirements
- executor family selection
- mutation/verification/deferral posture

These tests should avoid asserting:

- exact preset names unless the preset name itself is the contract

### 2.3 Orchestration Tests

Purpose:
Validate that Euclo can orchestrate capabilities, sub-agents, and shared context correctly.

Primary subjects:

- relurpic capability routing
- multi-capability execution within one work unit
- sub-agent orchestration through `/agents`
- shared context/memory across orchestrated agents
- capability-family and executor-family cooperation

These tests should validate:

- multiple execution components can participate in one unit of work
- shared runtime state is preserved
- artifact production and continuity remain coherent
- orchestration respects policy and mode
- relurpic capability ownership remains explicit even when several paradigms participate

These tests should not hardcode:

- one exact internal chain such as `HTN -> pipeline -> ReAct`

Instead they should assert:

- the allowed executor/capability family set
- required artifacts
- shared-context preservation
- correct final runtime result

### 2.4 Security And Policy Tests

Purpose:
Validate that Euclo remains constrained by framework and manifest policy.

Primary subjects:

- capability admission and exposure
- tool execution policy
- skill policy narrowing
- provider policy
- write/edit/mutation constraints
- verification policy
- recovery policy

These tests should assert:

- manifest/runtime policy changes execution behavior
- Euclo does not bypass capability policy through sub-agent orchestration
- collect-context modes remain subject to the same permissions model
- plan-backed execution remains subject to the same permissions model
- Euclo runtime reporting reflects framework-admitted execution catalog state

These tests are part of Euclo’s core quality bar and should not be treated as optional edge cases.

### 2.5 Continuity And Persistence Tests

Purpose:
Validate resumability and durable runtime continuity.

Primary subjects:

- compiled execution persistence
- final report persistence
- deferred issue persistence
- context compaction and restore
- provider snapshot and session snapshot persistence
- restored semantic inputs
- restored executor identity
- restored policy snapshot

These tests should assert:

- no silent loss of plan intent
- no silent loss of deferrals
- no silent loss of semantic grounding
- restored runs preserve execution identity where expected
- restored runs preserve policy snapshot and admitted execution-shape reporting where applicable

### 2.6 Benchmark And Performance Tests

Purpose:
Measure local runtime behavior and regressions without live LLM dependency.

These are engineering benchmarks, not product E2E tests.

They should focus on:

- runtime overhead
- artifact generation cost
- continuity restore cost
- orchestration overhead
- archaeology projection/semantic input overhead
- benchmark fixtures that model lower-end LLM constraints without calling live models

Current workload benchmarks should include:

- failing test to fix
- multi-step living-plan execution
- compatibility-preserving refactor
- long-running migration
- restore after compaction

## 3. What The Euclo Suite Should Design For

The local Euclo suite should explicitly design for these runtime truths.

### 3.1 Modal Runtime

Euclo is modal.

The suite should prove:

- mode changes runtime behavior materially
- mode changes primary capability ownership materially
- mode affects context collection strategy
- mode affects semantic input requirements
- mode affects executor family selection
- mode affects verification and deferral posture

### 3.2 Capability-Centered Runtime

Capabilities are the central construct.

Tools are one special case of capability usage.

The suite should prove:

- capability routing is meaningful
- capability policy constrains execution
- tool access does not bypass capability policy
- sub-agent execution still participates in capability-centered policy
- primary and supporting relurpic capability ownership is explicit in runtime state

### 3.3 Shared-Context Orchestration

The suite should prove:

- Euclo can orchestrate sub-agent-like relurpic routines
- multiple agents can share context and memory
- execution continuity remains coherent across that orchestration

### 3.4 Collect-Context Plus Toolbox Execution

The suite should prove:

- ReAct-like behavior remains supported where appropriate
- Euclo can execute coding tasks in a context-collecting plus toolbox pattern
- this remains policy-constrained

### 3.5 Plan-Backed Long-Running Execution

The suite should prove:

- Euclo can bind to a living plan
- Euclo can continue through long-running implementation
- unresolved issues become deferrals
- context compaction does not terminate long-running execution

### 3.6 Layered Architecture

The suite should prove Euclo works correctly as a layered runtime built on:

- `/framework`
- `/agents`
- `/archaeo`

It should not pretend those layers do not exist.

## 4. What The Euclo Suite Should Not Overfit To

The local suite should avoid overfitting to these unless a specific case explicitly intends to pin them.

### 4.1 Exact Preset Or Profile Names

Do not make architectural tests depend primarily on:

- `plan_stage_execute`
- `edit_verify_repair`
- `review_suggest_implement`
- similar exact preset names

Prefer testing:

- executor family
- behavior family
- mutation posture
- verification posture
- context strategy
- semantic input expectations

### 4.2 Exact Internal Controller Keys

Avoid making end-to-end Euclo-local tests depend on:

- `euclo.profile_controller`
- similar controller/adapter breadcrumbs

Those are valid for controller-level tests, but not as the main acceptance contract.

### 4.3 Exact Internal Paradigm Chains

Avoid pinning:

- `HTN -> pipeline -> ReAct`
- `planner -> reflection -> react`
- any other exact chain

unless the test is explicitly a composition wiring test.

The real contract is:

- what behavior family was selected
- what executor family was selected
- what runtime invariants held

### 4.4 Transient State Noise

Avoid relying on transient state keys unless they are explicitly part of the runtime contract.

Examples:

- ephemeral controller traces
- incidental timestamps
- incidental ordering of diagnostic state

## 5. Required Test Categories

The Euclo-local suite should include the following concrete categories.

### 5.1 Runtime Contract Category

Representative tests:

- `UnitOfWork` assembly for direct execution
- `UnitOfWork` assembly for plan-backed execution
- `CompiledExecution` round-trip and reconstruction
- `DeferredExecutionIssue` taxonomy and evidence shape
- `ExecutionResultClass` derivation
- `ResolvedExecutionPolicy` from skill policy
- `SemanticInputBundle` construction from archaeology state

### 5.2 Modal Category

Representative tests:

- direct collect-context coding task resolves to ReAct-compatible execution posture
- planning task resolves to heavy context posture
- debug task loads tension/pattern semantic inputs
- review task resolves to review-oriented posture

### 5.3 Orchestration Category

Representative tests:

- one unit of work uses multiple capability families
- one unit of work shares state across orchestrated sub-agents
- relurpic reasoning can feed later execution stages
- executor family and capability family selection cooperate correctly

### 5.4 Security Category

Representative tests:

- manifest policy removes mutation capability from a coding task
- skill policy narrows phase capability selection
- sub-agent orchestration cannot bypass denied capabilities
- write execution remains blocked under read-only conditions

### 5.5 Continuity Category

Representative tests:

- compaction and restore preserve plan binding
- compaction and restore preserve semantic inputs
- compaction and restore preserve executor descriptor
- provider snapshots persist and restore
- final reports preserve deferred issue refs and result class

### 5.6 Observability Category

Representative tests:

- `euclo.action_log` is emitted
- `euclo.proof_surface` is emitted
- final report includes semantic inputs, executor descriptor, and result class
- telemetry emission can be driven without live model calls

Observability should remain a dedicated category, not the main end-to-end acceptance criterion.

### 5.7 Composition Category

Representative tests:

- planner-backed executor family can still share Euclo state
- HTN-backed execution can still share Euclo state
- reflection-backed review can preserve Euclo runtime result semantics
- ReAct-style collect-context mode remains available

This category is where exact composition details may be tested, but only explicitly.

## 6. Benchmark Taxonomy

Benchmarks should be part of the Euclo-local engineering suite.

They should not assume live LLM access.

### 6.1 Benchmark Rules

All Euclo-local benchmarks should:

- use deterministic stub/fake models
- use local stores
- use reproducible fixtures
- avoid network dependency
- avoid timing dependence on live providers

### 6.2 Benchmark Categories

#### A. Runtime Assembly Benchmarks

Measure:

- task normalization
- classification
- mode/profile/policy resolution
- `UnitOfWork` assembly
- semantic input bundle construction

#### B. Persistence Benchmarks

Measure:

- compiled execution persistence
- deferred artifact persistence
- final report assembly
- provider snapshot persistence

#### C. Restore Benchmarks

Measure:

- restore from compacted context
- reconstruct `UnitOfWork` from compiled execution
- restore semantic inputs from persisted state
- restore provider snapshot/session snapshot metadata

#### D. Orchestration Benchmarks

Measure:

- multi-capability execution overhead
- sub-agent shared-context overhead
- routing/orchestration overhead across executor families

#### E. Planning-Path Benchmarks

Measure:

- heavy-context path assembly
- archaeology projection reads used for semantic inputs
- plan-backed execution runtime overhead without live model inference

### 6.3 Benchmark Fixtures

Recommended fixtures:

- small direct code task
- debug localization task
- review task
- plan-backed long-running task
- restored long-running task after compaction
- multi-agent shared-context task

These fixtures should exist independently of the full live `/testsuite`.

## 7. Suggested File Taxonomy

Suggested reorganization target:

- `named/euclo/runtime/*_test.go`
  Runtime contract tests

- `named/euclo/euclotest/modal_*_test.go`
  Mode and unit-of-work behavior tests

- `named/euclo/euclotest/orchestration_*_test.go`
  Multi-capability and sub-agent orchestration tests

- `named/euclo/euclotest/security_*_test.go`
  Manifest/policy/security tests

- `named/euclo/euclotest/continuity_*_test.go`
  Persistence and restore tests

- `named/euclo/euclotest/observability_*_test.go`
  Action log, proof surface, telemetry tests

- `named/euclo/benchmark/*_bench_test.go`
  Local engineering benchmarks

## 8. Existing Tests To Rewrite First

The first refactor targets should be tests that overfit exact preset identity or exact internal composition details.

### Priority 1

- profile-selection tests that assert exact preset names for architectural scenarios
- full integration tests that require controller-specific state such as `euclo.profile_controller`
- capability tests that encode one exact paradigm chain as the contract

### Priority 2

- end-to-end Euclo-local tests that assert too many transient state keys
- tests that pin exact internal route ordering when a higher-level executor/capability family contract is enough

### Priority 3

- artifact tests that could be rewritten around final-report/runtime-contract outputs rather than intermediate state shape

## 9. New Tests To Add

The next missing tests to add are:

- `UnitOfWork` modal behavior tests using executor family and context strategy assertions
- shared-context multi-agent orchestration tests
- skill-policy-to-runtime-policy tests at the Euclo agent level
- manifest security tests across collect-context and plan-backed modes
- semantic-input restoration tests
- benchmark fixtures for restore and orchestration overhead

## 10. Acceptance Standard

The Euclo-local engineering suite is healthy when:

- runtime contract tests are stable
- modal behavior is explicit
- orchestration is tested as a first-class feature
- policy/security is tested as a first-class feature
- continuity is tested as a first-class feature
- benchmarks exist for key runtime paths without live LLM dependency
- only `/testsuite` is responsible for full live LLM end-to-end validation

That is the intended testing standard for Euclo as a layered, modal, capability-centered coding runtime.
