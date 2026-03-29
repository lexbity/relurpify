# Euclo Relurpic Behavior Completion Plan

## Synopsis

This plan closes the remaining gap between:

- the relurpic capability architecture described in
  [docs/research/4.md](/home/lex/Public/Relurpify/docs/research/4.md),
  [docs/research/6.md](/home/lex/Public/Relurpify/docs/research/6.md), and
  [docs/research/9.md](/home/lex/Public/Relurpify/docs/research/9.md)
- the current implementation in `named/euclo/relurpicabilities`
- the execution paradigms already available in `/agents`

This is not a minimum viable product plan.

It assumes:

- `framework/` owns relurpic capability as a primitive via
  `CapabilityRuntimeFamilyRelurpic`
- `named/euclo/` owns Euclo-specific relurpic capability families
- `/agents` remains the generic execution-paradigm substrate
- `archaeo/` remains the owner of memory, provenance, living-plan state, and
  codebase-knowledge relationships

The main problem this plan addresses is:

- relurpic capability ownership is present
- relurpic capability routing is present
- relurpic capability runtime reporting is present
- but most primary capabilities are still thin dispatchers instead of concrete
  behavior owners

This plan therefore focuses on behavior depth, paradigm composition, and
supporting-routine execution.

---

## Current Gap Summary

### Implemented Well Enough Today

- Euclo-owned relurpic capability IDs and descriptors
- primary owner selection by mode and task shape
- supporting capability assembly metadata
- runtime reporting and capability contract surfaces
- some reusable local/shared capabilities in
  `named/euclo/relurpicabilities/local`
- some `/agents` usage through Euclo execution adapters

### Not Yet Implemented To Spec

- `euclo:chat.ask` as a real ask-oriented behavior owner
- `euclo:chat.inspect` as a real inspect-first behavior owner
- `euclo:chat.implement` as a true composition root over local/shared support
- `euclo:debug.investigate` as a mixed investigation workspace and routine
  coordinator
- `euclo:archaeology.explore` as a real archaeology exploration behavior
- `euclo:archaeology.compile-plan` as a structured compile behavior rather than
  planner-plus-fallback
- `euclo:archaeology.implement-plan` as a real long-horizon plan execution
  behavior

### Structural Weaknesses

- supporting routines are still often represented by seeded state in
  `execution.EnsureRoutineArtifacts(...)`
- execution adapters are mostly thin constructors instead of recipe-level
  runners
- `/agents` paradigms like Architect, Pipeline, Chainer, and ReWOO are not yet
  used deeply inside relurpic capability behavior composition
- archaeology capabilities are still much thinner than the research documents
  intend

---

## Capability Target Matrix

### Chat

Primary capabilities:

- `euclo:chat.ask`
- `euclo:chat.inspect`
- `euclo:chat.implement`

Supporting capabilities to operationalize:

- `euclo:chat.direct-edit-execution`
- `euclo:chat.local-review`
- `euclo:chat.targeted-verification-repair`

Expected paradigm substrate:

- `react`
- `reflection`
- `htn`
- `architect`

### Debug

Primary capability:

- `euclo:debug.investigate`

Supporting capabilities to operationalize:

- `euclo:debug.root-cause`
- `euclo:debug.hypothesis-refine`
- `euclo:debug.localization`
- `euclo:debug.flaw-surface`
- `euclo:debug.verification-repair`

Expected paradigm substrate:

- `blackboard`
- `react`
- `htn`
- `reflection`
- optional `pipeline`

### Archaeology / Plan

Primary capabilities:

- `euclo:archaeology.explore`
- `euclo:archaeology.compile-plan`
- `euclo:archaeology.implement-plan`

Supporting capabilities to operationalize:

- `euclo:archaeology.pattern-surface`
- `euclo:archaeology.prospective-assess`
- `euclo:archaeology.convergence-guard`
- `euclo:archaeology.coherence-assess`
- `euclo:archaeology.scope-expansion-assess`

Expected paradigm substrate:

- `planner`
- `reflection`
- `blackboard`
- `rewoo`
- `architect`
- `chainer` or `pipeline` where deterministic staged shaping is needed

---

## Architectural Rules

### Rule 1: Primary Capability Owns Execution

Primary relurpic capabilities must become the concrete behavior roots for their
mode invocation.

They should not merely:

- seed supporting artifact placeholders
- append diagnostics
- dispatch through generic workflow execution unchanged

They must own:

- supporting routine orchestration
- paradigm composition
- execution posture
- evidence collection strategy
- escalation and deferral behavior

### Rule 2: Supporting Capabilities Execute As Routines

Supporting relurpic capabilities must execute as concrete routines, not only as
descriptor metadata or seeded state markers.

They may be:

- callable subordinate routines
- stage-like units in a composed behavior
- blackboard specialists
- planner/reflection subpasses

But they must be real execution units.

### Rule 3: `/agents` Paradigms Are Substrate, Not Owner

`/agents` paradigms should implement the execution machinery for relurpic
behavior, not own Euclo’s coding behavior identity.

Euclo’s relurpic capability layer should compose:

- `react`
- `planner`
- `htn`
- `reflection`
- `architect`
- `rewoo`
- `blackboard`
- `pipeline`
- `chainer`

as needed per capability.

### Rule 4: Archaeology Capability Depth Must Match The Research Direction

Archaeology capabilities must move toward:

- pattern surfacing
- prospective possibility exploration
- convergence and coherence assessment
- living plan compilation
- executable plan handoff

rather than remaining planner wrappers.

---

## Phase 1: Convert Supporting Capability Metadata Into Real Routine Contracts

### Goal

Replace seeded supporting-routine placeholders with explicit executable routine
contracts.

### Changes

- introduce a supporting routine interface inside `named/euclo/relurpicabilities`
- define concrete routine implementations for:
  - `chat.local-review`
  - `chat.targeted-verification-repair`
  - `debug.root-cause`
  - `debug.hypothesis-refine`
  - `debug.localization`
  - `debug.flaw-surface`
  - `debug.verification-repair`
  - `archaeology.pattern-surface`
  - `archaeology.prospective-assess`
  - `archaeology.convergence-guard`
  - `archaeology.coherence-assess`
  - `archaeology.scope-expansion-assess`
- remove the behavior significance from `execution.EnsureRoutineArtifacts(...)`
- keep artifact seeding only as a compatibility convenience where still needed

### Package Targets

- `named/euclo/relurpicabilities/chat`
- `named/euclo/relurpicabilities/debug`
- `named/euclo/relurpicabilities/archaeology`
- `named/euclo/execution`

### Dependencies

- current relurpic descriptor catalog
- existing execution adapters

### Tests

- unit tests that each supporting capability has a concrete implementation
- unit tests that primary capabilities invoke expected supporting routines
- integration tests that runtime state reflects actual routine execution, not
  only seeded metadata

---

## Phase 2: Rebuild `chat.ask` And `chat.inspect` As Real Behaviors

### Goal

Turn `chat.ask` and `chat.inspect` into real behavior owners rather than thin
workflow dispatchers.

### Changes

- `chat.ask`
  - explicit question/explanation evidence collection
  - optional read-only codebase inspection
  - optional reflection review for answer quality
  - explicit non-mutation contract enforcement
- `chat.inspect`
  - inspect-first evidence collection
  - subordinate local review
  - compatibility assessment when the request implies API or surface analysis
  - selective mutation only when policy and tools allow it, without treating
    it as the default path

### Paradigm Composition

- `react` for collection and tool use
- `reflection` for answer/review shaping
- optional `planner` only for explicit comparison/option requests

### Package Targets

- `named/euclo/relurpicabilities/chat`
- `named/euclo/execution/react`
- `named/euclo/execution/reflection`

### Dependencies

- Phase 1 supporting routines
- current chat runtime reporting surfaces

### Tests

- `chat.ask` produces answer-oriented artifacts and remains non-mutating
- `chat.inspect` activates inspection and local-review routines
- policy-constrained tool use is surfaced correctly

---

## Phase 3: Rebuild `chat.implement` As A True Composition Root

### Goal

Make `chat.implement` the real owner of direct coding behavior rather than a
React-heavy shell with sidecar helpers.

### Changes

- move direct edit execution under explicit support routines
- branch by task shape:
  - bounded direct edit
  - multi-step implementation
  - migration
  - API-compatible refactor
  - review-guided repair
- integrate reusable local capabilities as subordinate routines rather than
  standalone parallel capability paths
- formalize lazy archaeology exploration handoff

### Paradigm Composition

- `react` for bounded direct edits
- `htn` for decomposition-heavy edits
- `architect` for plan-then-execute multi-step coding
- `reflection` for repair/review loops
- optional `pipeline` for deterministic verify-or-repair paths

### Package Targets

- `named/euclo/relurpicabilities/chat`
- `named/euclo/relurpicabilities/local`
- `named/euclo/execution/react`
- `named/euclo/execution/htn`
- `named/euclo/execution/reflection`
- `named/euclo/execution/architect` when implemented

### Dependencies

- Phase 1 and Phase 2
- current local shared capability implementations

### Tests

- task-shape selection tests for direct edit vs migration vs refactor
- integration tests showing local capabilities are invoked under
  `chat.implement`
- runtime tests showing lazy archaeology acquisition is triggered and persisted

---

## Phase 4: Rebuild `debug.investigate` Around Mixed Investigation Semantics

### Goal

Make `debug.investigate` match the intended mixed behavior:

- chat-style interaction
- data exposition
- tool output
- archaeology-backed semantic grounding
- bounded repair

### Changes

- build a real debug behavior pipeline around explicit subordinate routines:
  - reproduce
  - root cause
  - hypothesis refinement
  - localization
  - flaw surfacing
  - verification repair
- migrate regression investigation and trace analysis into the debug owner path
- make tool exposition a required facet of the owner
- make escalation to `chat.implement` explicit when mutation moves beyond the
  bounded debug contract

### Paradigm Composition

- `blackboard` as the main investigation workspace
- `react` for tool-driven reproduction and bounded execution
- `htn` for drilldown decomposition
- `reflection` for patch and verification review
- optional `pipeline` for deterministic reproduction/localization/verify rails

### Package Targets

- `named/euclo/relurpicabilities/debug`
- `named/euclo/execution/blackboard`
- `named/euclo/execution/react`
- `named/euclo/execution/htn`
- `named/euclo/execution/reflection`

### Dependencies

- Phase 1 supporting routines
- current blackboard execution bridge
- current debug runtime reporting

### Tests

- debug investigation invokes root-cause and localization routines explicitly
- regression investigation is subordinate to the debug owner rather than a
  parallel capability path
- escalation to `chat.implement` is correctly reported and bounded

---

## Phase 5: Rebuild `archaeology.explore` As A Real Exploration Environment

### Goal

Implement archaeology exploration as a first-class relurpic behavior instead of
metadata plus generic dispatch.

### Changes

- make pattern surfacing a concrete behavior
- implement prospective pairing and structural-option exploration
- implement convergence and coherence analysis as real exploration passes
- integrate Archaeo-backed semantic evidence more directly into exploration
  flow
- produce artifacts suitable for later learning/HITL-style confirmation, even
  if the full UX is not yet implemented

### Paradigm Composition

- `blackboard` for pattern/tension/prospective accumulation
- `planner` for structured exploration hypotheses
- `reflection` for coherence and convergence review
- optional `chainer` for repeatable exploration passes

### Package Targets

- `named/euclo/relurpicabilities/archaeology`
- `named/euclo/execution/blackboard`
- `named/euclo/execution/planner`
- `named/euclo/execution/reflection`

### Dependencies

- Phase 1 supporting routines
- current semantic input bundle and archaeology runtime surfaces

### Tests

- pattern-surface and prospective-assess produce real artifacts
- exploration consumes archaeology-backed semantic inputs materially
- convergence/coherence routines affect exploration outputs

---

## Phase 6: Rebuild `archaeology.compile-plan` As A Structured Compile Behavior

### Goal

Turn `archaeology.compile-plan` into a multi-pass compile behavior that matches
the research direction and the capability spec.

### Changes

- separate compile-plan into explicit passes:
  - evidence synthesis
  - pattern/prospective reconciliation
  - plan shaping
  - plan review
  - compile-or-defer decision
- remove the “planner plus fallback” shape as the primary behavior
- ensure full-plan-or-deferral semantics are enforced by behavior structure
- produce stronger compiled plan artifacts suitable for later execution

### Paradigm Composition

- `planner` as core plan synthesizer
- `reflection` as plan reviewer
- `pipeline` or `chainer` for deterministic compile passes
- optional `blackboard` if evidence reconciliation remains iterative

### Package Targets

- `named/euclo/relurpicabilities/archaeology`
- `named/euclo/execution/planner`
- `named/euclo/execution/reflection`
- `named/euclo/execution/pipeline`
- `named/euclo/execution/chainer`

### Dependencies

- Phase 5 archaeology exploration outputs
- current plan runtime and deferral surfaces

### Tests

- compile-plan succeeds only with a full executable plan artifact
- partial compile emits deferred issues, not success
- compile-plan consumes exploration-produced supporting artifacts explicitly

---

## Phase 7: Rebuild `archaeology.implement-plan` As A Long-Horizon Execution Owner

### Goal

Make `archaeology.implement-plan` a genuine plan-bound execution behavior.

### Changes

- execute explicit compiled plan steps
- checkpoint and verify at meaningful plan boundaries
- preserve single-plan execution semantics through the behavior itself
- surface deferred issues and step outcomes as part of long-horizon execution
- keep runtime continuity and restore integrated with plan step history

### Paradigm Composition

- `rewoo` or `architect` as the primary long-horizon execution substrate
- `planner` for step context and plan access
- `reflection` for checkpoint review
- optional `pipeline` for deterministic verification/checkpoint stages

### Package Targets

- `named/euclo/relurpicabilities/archaeology`
- `named/euclo/execution/rewoo`
- `named/euclo/execution/architect`
- `named/euclo/execution/reflection`

### Dependencies

- Phase 6 compiled plan behavior
- current restore, transitions, and reporting runtime support

### Tests

- implement-plan requires a compiled plan
- step execution preserves one-plan-per-run semantics
- checkpoint and deferral behavior survives restore and resume

---

## Phase 8: Upgrade `execution/` From Thin Constructors To Recipe-Level Runners

### Goal

Make `named/euclo/execution` the real Euclo-side paradigm adapter layer rather
than a mostly thin constructor package.

### Changes

- add recipe-level runners for:
  - react inquiry
  - react inspect
  - chat implement direct edit
  - debug drilldown
  - archaeology compile passes
  - plan-bound long-horizon execution
- add adapter packages for still-underused paradigms:
  - `architect`
  - `pipeline`
  - `chainer`
  - `rewoo`
- reduce direct `/agents` instantiation inside relurpic behavior code

### Package Targets

- `named/euclo/execution/*`

### Dependencies

- current execution packages
- completed behavior phases above

### Tests

- recipe-level unit tests per adapter
- integration tests showing behavior modules call recipe runners rather than
  instantiating agents ad hoc

---

## Phase 9: Align Runtime Reporting With Real Behavior Execution

### Goal

Make runtime reporting describe actual behavior/routine execution instead of a
mix of real execution and seeded placeholders.

### Changes

- update chat/debug/archaeology runtime reporting to record actual routine
  executions
- include paradigm mix and recipe execution details in behavior traces
- reduce or remove diagnostics that exist only because routines are not yet
  real
- align final reports with the rebuilt relurpic execution model

### Package Targets

- `named/euclo/runtime/reporting`
- `named/euclo/runtime/policy`
- `named/euclo/execution`

### Dependencies

- Phases 1 through 8

### Tests

- reporting tests that distinguish actual routine execution from metadata-only
  activation
- final report tests for chat/debug/archaeology behavior traces

---

## Phase 10: Retire Transitional Relurpic Gaps

### Goal

Remove the remaining mismatch between relurpic capability intent and concrete
behavior implementation.

### Changes

- eliminate behavior paths that only delegate generically without meaningful
  capability-owned logic
- reduce remaining standalone local/shared capabilities that should now be
  subordinate routines
- simplify runtime and registry surfaces around the completed behavior model
- update `named/euclo/README.md` and related docs to describe the actual final
  architecture

### Package Targets

- `named/euclo/relurpicabilities/*`
- `named/euclo/execution/*`
- `named/euclo/runtime/*`
- `named/euclo/README.md`

### Dependencies

- All prior phases

### Tests

- full Euclo-local regression run over `go test ./named/euclo/...`
- benchmark comparison against pre-completion behavior paths
- behavior-coverage audit by primary and supporting capability

---

## Test Strategy

### 1. Capability Coverage Tests

For every primary capability:

- it has a concrete behavior implementation
- it invokes expected supporting routines
- it uses the intended paradigm recipe

### 2. Supporting Routine Tests

For every supporting capability:

- it executes concretely
- it emits expected artifacts or diagnostics
- it can be invoked under the expected primary owner

### 3. Runtime Contract Tests

Preserve:

- `UnitOfWork` ownership
- compiled execution semantics
- single-plan run behavior
- deferral creation and reporting
- restore and transition behavior

### 4. Integration Tests

Mode-based:

- chat ask
- chat inspect
- chat implement
- debug investigate
- archaeology explore
- archaeology compile-plan
- archaeology implement-plan

### 5. Benchmarks

Benchmark at the relurpic-owner level:

- `chat.ask`
- `chat.implement`
- `debug.investigate`
- `archaeology.explore`
- `archaeology.compile-plan`
- `archaeology.implement-plan`

The benchmark goal is not only speed.

It must also track:

- LLM call count
- number of paradigm invocations
- runtime artifact/report overhead
- restore/checkpoint overhead for long-horizon paths

---

## Recommended Implementation Order

Recommended sequence:

1. Phase 1
2. Phase 4
3. Phase 6
4. Phase 3
5. Phase 2
6. Phase 5
7. Phase 7
8. Phase 8
9. Phase 9
10. Phase 10

Reason:

- debug and archaeology are furthest from the intended behavior model
- `chat.implement` already has the strongest foundation and can be deepened
  after the routine and recipe layer is made real
- execution recipe depth should follow behavior-owner clarity, not lead it

---

## Exit Criteria

This plan is complete when:

- every primary relurpic capability is a real behavior owner
- supporting capabilities execute as real routines
- `/agents` paradigms are composed materially inside relurpic behavior
- archaeology capabilities match the intended exploration/compile/implement
  depth better than thin planner wrappers
- runtime reporting reflects actual behavior execution
- the gap between the relurpic capability spec and the code is materially
  closed
