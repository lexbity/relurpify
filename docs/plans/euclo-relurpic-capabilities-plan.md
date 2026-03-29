# Euclo Relurpic Capabilities Plan

## Synopsis

This plan implements Euclo-owned relurpic capabilities as the primary behavior
layer for Euclo.

It assumes:

- `framework/` owns relurpic capability as a primitive through
  `CapabilityRuntimeFamilyRelurpic`
- `named/euclo/` owns Euclo's coding-specific relurpic capability catalog
- `agents/` remains the generic paradigm substrate
- `archaeo/` remains the owner of memory, provenance, living-plan state, and
  knowledge relationships

This is a full-product plan, not a minimum viable product.

Related documents:

- `docs/research/9.md`
- `docs/research/6.md`
- `docs/research/8.md`
- `docs/plans/euclo-rework.md`
- `docs/plans/euclo-runtime-opportunities-plan.md`

---

## Engineering Specification

### Goals

Implement a Euclo runtime where:

- mode selection leads to Euclo-owned relurpic capability selection
- `UnitOfWork` binds a primary relurpic capability owner
- supporting relurpic capabilities are explicit runtime bindings
- executor composition becomes subordinate to behavior selection
- Archaeo-associated capabilities remain distinct from Euclo-local ones
- security/policy remains capability-first even for composed behaviors

### Canonical Capability IDs

Chat:

- `euclo:chat.ask`
- `euclo:chat.implement`
- `euclo:chat.inspect`

Archaeology:

- `euclo:archaeology.explore`
- `euclo:archaeology.compile-plan`
- `euclo:archaeology.implement-plan`

Debug:

- `euclo:debug.investigate`

### Runtime Guarantees

- `euclo:chat.ask` is non-mutating.
- `euclo:chat.inspect` is inspect-first but not guaranteed non-mutating because
  tool capabilities may not guarantee that.
- `euclo:chat.implement` may lazily acquire Archaeo-backed semantic input.
- `euclo:archaeology.compile-plan` must emit a full executable plan or a
  deferred artifact.
- `euclo:archaeology.implement-plan` requires a compiled/finalized living plan.
- `euclo:debug.investigate` may use tool exposition internally but must not
  bypass capability security; repair beyond its bounded investigation contract
  transitions to `euclo:chat.implement`.

### Type And Registry Changes

Euclo should gain:

- a Euclo-owned relurpic capability descriptor type
- a capability registry/catalog in `named/euclo`
- primary/supporting capability bindings on `UnitOfWork`
- per-capability metadata:
  - mode eligibility
  - mutability contract
  - Archaeo association
  - transition compatibility
  - supporting executor/paradigm recipe
  - required skill/policy facets
  - security notes

### Transition Model

- preserve `UnitOfWork` where the semantic objective is continuous
- permit a new `UnitOfWork` when moving from Archaeo-backed context into
  local-only execution if that creates a clearer runtime contract
- always preserve artifact linkage between predecessor and successor work units

### Security Model

- relurpic capability execution remains subject to manifest admission and
  capability policy
- no relurpic capability may bypass tool/capability restrictions
- tool exposition for debug remains a facet of the owning relurpic capability,
  not a backdoor tool path

### Deferred Design Note

Cross-mode reuse should stay implicit for now.

Implementation should leave room for future capability tags for:

- interoperability
- transition compatibility
- reuse hints

but tags are not required in this phase plan.

---

## Phase 1: Euclo Relurpic Capability Contract

### Goal

Create the Euclo-owned relurpic capability contract and registry surface inside
`named/euclo`.

### Changes

- add Euclo-owned relurpic capability descriptor types
- add canonical capability IDs and stable metadata
- add capability classification fields:
  - primary-capable endpoint
  - supporting-only capability
  - mutability posture
  - Archaeo association
  - transition compatibility summary
- add a Euclo relurpic capability catalog/registry package
- update `UnitOfWork` to bind:
  - `PrimaryRelurpicCapabilityID`
  - `SupportingRelurpicCapabilityIDs`

### Package Targets

- `named/euclo/runtime`
- `named/euclo/euclotypes`
- `named/euclo/relurpic` or equivalent new package

### Dependencies

- existing `UnitOfWork` type work
- framework `CapabilityRuntimeFamilyRelurpic`

### Tests

- unit tests for descriptor validation
- unit tests for registry lookup by mode and ID
- unit tests for primary/supporting capability binding in `UnitOfWork`
- regression tests that existing runtime behavior remains intact

---

## Phase 2: Mode-To-Capability Selection

### Goal

Make Euclo select primary relurpic capability owners by mode and intent before
executor details are chosen.

### Changes

- update mode/intake classification to produce capability-owner selection
- add intent-to-capability selection rules for `chat`:
  - ask
  - implement
  - inspect
- add archaeology capability selection rules for:
  - explore
  - compile-plan
  - implement-plan
- add debug selection rules for:
  - investigate
- preserve mode-level behavior while changing the owning runtime selection logic

### Package Targets

- `named/euclo/runtime`
- `named/euclo/agent.go`

### Dependencies

- Phase 1 capability catalog
- existing `TaskEnvelope` / `TaskClassification`

### Tests

- unit tests for selection rules by mode and task intent
- integration tests showing `UnitOfWork` carries the expected primary capability
- regression tests preserving current profile selection where still contractual

---

## Phase 3: Supporting Capability Assembly

### Goal

Add explicit supporting relurpic capability assembly under a primary owner.

### Changes

- define supporting capability sets for each primary capability
- add assembly rules for:
  - `euclo:chat.ask`
  - `euclo:chat.implement`
  - `euclo:chat.inspect`
  - `euclo:archaeology.explore`
  - `euclo:archaeology.compile-plan`
  - `euclo:archaeology.implement-plan`
  - `euclo:debug.investigate`
- let supporting capabilities advertise:
  - local-only operation
  - Archaeo-associated operation
  - lazy semantic acquisition

### Package Targets

- `named/euclo/runtime`
- `named/euclo/relurpic`

### Dependencies

- Phase 1 and Phase 2
- current semantic-input bundle support

### Tests

- unit tests for supporting capability expansion
- unit tests for lazy Archaeo acquisition eligibility
- integration tests for mode-specific capability bundles

---

## Phase 4: Executor Recipes Under Relurpic Ownership

### Goal

Move executor/paradigm composition under relurpic capability ownership instead
of treating executor family as the top-level decision.

### Changes

- define executor recipes per primary capability
- map recipes to `/agents` paradigms and Euclo-managed managed-flow execution
- make executor selection derive from relurpic capability assembly
- keep current executor families available:
  - react
  - planner
  - htn
  - rewoo
  - reflection
- formalize multi-paradigm composition inside capability recipes where needed

### Package Targets

- `named/euclo/executors.go`
- `named/euclo/runtime`
- `agents/*` only where interface adjustments are required

### Dependencies

- Phase 3
- current executor work from `euclo-rework.md`

### Tests

- unit tests for capability-to-executor-recipe mapping
- integration tests confirming selected capability drives executor choice
- regression tests that no mode loses current execution support
- benchmark comparisons by capability owner

---

## Phase 5: Capability Contracts Per Endpoint

### Goal

Implement and enforce per-capability runtime guarantees.

### Changes

- enforce non-mutating contract for `euclo:chat.ask`
- define inspect-first policy surface for `euclo:chat.inspect`
- add lazy Archaeo acquisition rules for `euclo:chat.implement`
- enforce full-plan-or-deferral result for `euclo:archaeology.compile-plan`
- enforce compiled-plan prerequisite for `euclo:archaeology.implement-plan`
- enforce debug-to-implement transition rule for repair escalation

### Package Targets

- `named/euclo/runtime`
- `named/euclo/agent.go`
- `named/euclo/gate`
- `named/euclo/interaction` where needed

### Dependencies

- Phase 2 through Phase 4
- current deferral/result-class model

### Tests

- unit tests for contract enforcement per endpoint
- integration tests for:
  - `chat.ask` non-mutation
  - `chat.implement` lazy Archaeo acquisition
  - compile-plan deferral on unresolved issue
  - implement-plan requiring compiled plan
  - debug repair escalation transition

---

## Phase 6: Archaeo-Associated Capability Deepening

### Goal

Complete the Archaeo-associated Euclo relurpic capability set for planning and
data-oriented work.

### Changes

- deepen `euclo:archaeology.explore` composition for:
  - pattern recognition routines
  - provenance analysis
  - prospective structure analysis
  - convergence/coherence routines
- deepen `euclo:archaeology.compile-plan`
- deepen `euclo:archaeology.implement-plan`
- make LLM-dependent archaeology behavior explicit in capability descriptors and
  runtime reporting

### Package Targets

- `named/euclo/relurpic`
- `named/euclo/runtime`
- `archaeo/adapters/relurpic`
- `agents/relurpic` where generic support already exists

### Dependencies

- Phase 3 through Phase 5
- current Archaeo semantic input surfaces

### Tests

- integration tests for archaeology capability chains
- persistence tests for archaeology capability artifacts and semantic bundles
- benchmark tests for explore/compile/implement plan paths without live LLM

---

## Phase 7: Debug Capability Deepening

### Goal

Complete `euclo:debug.investigate` as a mixed behavior owner.

### Changes

- formalize internal debug supporting behaviors:
  - root-cause investigation
  - defect hypothesis refinement
  - verification-driven localization
  - flaw / smell / vulnerability surfacing
  - targeted verification repair
  - tool output exposition facet
- integrate platform/tool output explicitly into the debug runtime payload and
  reporting surfaces
- add escalation rules from investigation into implementation

### Package Targets

- `named/euclo/relurpic`
- `named/euclo/runtime`
- `named/euclo/interaction`

### Dependencies

- Phase 3 through Phase 5
- existing debug semantic-input and observability work

### Tests

- integration tests for mixed debug behavior
- tests for tool output remaining capability-security constrained
- tests for escalation into `euclo:chat.implement`
- benchmark tests for localization and verification-heavy debug workloads

---

## Phase 8: Chat Capability Completion

### Goal

Complete the direct coding and engineering support behaviors under `chat`.

### Changes

- complete `euclo:chat.ask`
- complete `euclo:chat.inspect`
- complete `euclo:chat.implement`
- add supporting local behaviors for:
  - direct coding/edit execution
  - local review/inspection
  - targeted verification repair
- ensure `chat` remains model-quality tolerant by leaning on context,
  capability, and verification structure rather than assuming strong models

### Package Targets

- `named/euclo/relurpic`
- `named/euclo/runtime`
- `named/euclo/capabilities`

### Dependencies

- Phase 3 through Phase 5

### Tests

- integration tests for ask/inspect/implement separation
- tests for shared context continuity across chat behavior switches
- benchmark tests for direct coding/edit execution on weak-model assumptions

---

## Phase 9: Transition And `UnitOfWork` Rebinding

### Goal

Make mode and capability transitions explicit runtime behavior.

### Changes

- define transition compatibility matrix
- preserve `UnitOfWork` where possible
- create successor `UnitOfWork` when moving from Archaeo-backed context into
  local-only execution
- persist predecessor/successor work-unit linkage
- surface transition reasons in runtime artifacts and final reports

### Package Targets

- `named/euclo/runtime`
- `named/euclo/agent.go`
- `named/euclo/euclotypes`

### Dependencies

- Phase 1 through Phase 8

### Tests

- unit tests for transition decision rules
- integration tests for:
  - chat -> archaeology
  - archaeology -> implement-plan
  - debug -> chat.implement
  - archaeology-backed -> local-only new work unit
- persistence tests for linked work-unit history

---

## Phase 10: Security, Policy, And Skill Hardening

### Goal

Ensure the framework-owned security model remains authoritative while Euclo
adds behavior-level compatibility diagnostics and skill-aware capability
composition.

### Changes

- keep sandbox, permission denial, tool execution policy, capability policy,
  session policy, and runtime safety enforcement in `framework/`
- treat Euclo relurpic security metadata as behavior-contract metadata and
  compatibility hints rather than as the enforcement authority
- ensure skill policy contributes capability/routine selection directly within
  Euclo runtime assembly
- add explicit Euclo-owned security diagnostics per primary capability owner:
  - desired tool posture
  - supporting capability compatibility
  - framework-admitted vs behavior-desired capability shape
  - provider trust / restore diagnostics
- use framework-owned admitted registry and policy surfaces as the source of
  truth when evaluating whether a relurpic behavior can execute as designed
- leave room for future capability tags without introducing them as a required
  feature now

### Package Targets

- `named/euclo/runtime`
- `named/euclo/relurpic`
- `framework/skills`
- `framework/capabilityplan`
- `framework/authorization`
- `framework/capability`

### Dependencies

- all prior phases

### Tests

- policy tests proving framework remains the source of hard deny/approval
- Euclo runtime tests for security compatibility diagnostics under each primary
  capability owner
- skill-policy tests altering supporting capability composition
- integration tests for framework-denied tool/capability paths surfacing as
  Euclo diagnostics rather than Euclo-authored policy ownership
- regression tests for shared-context security guarantees

---

## Phase 11: Documentation And Benchmarks

### Goal

Finish the public and engineering-facing documentation and benchmark coverage.

### Changes

- update `docs/agents/euclo.md` to reflect the new mode/capability model
- add package docs in `named/euclo` for the relurpic capability catalog
- add benchmark documentation for local capability workloads
- align test taxonomy with the new capability-owner model

### Package Targets

- `docs/agents/euclo.md`
- `named/euclo/*`
- `docs/plans/euclo-test-taxonomy.md`

### Dependencies

- all prior phases

### Tests

- doc link/reference checks where applicable
- benchmark runs for:
  - `chat.ask`
  - `chat.implement`
  - `chat.inspect`
  - `archaeology.explore`
  - `archaeology.compile-plan`
  - `archaeology.implement-plan`
  - `debug.investigate`

---

## Test Strategy

Local engineering tests and benchmarks should remain separate from the full live
LLM E2E suite in `/testsuite`.

Required local coverage:

- unit tests for capability descriptors and assembly
- unit tests for capability-owner selection
- unit tests for transition/rebinding rules
- unit tests for contract enforcement
- integration tests for each primary capability endpoint
- persistence tests for artifacts, deferrals, and work-unit linkage
- security/policy tests
- benchmark tests without live LLM assumptions

---

## Acceptance Criteria

This plan is complete when:

- Euclo owns a stable relurpic capability catalog in `named/euclo`
- every Euclo mode selects a primary relurpic capability owner
- executor composition is subordinate to relurpic capability ownership
- Archaeo-associated capability behavior is explicit and complete
- chat/debug/planning capability contracts are enforced
- transitions and `UnitOfWork` rebinding are explicit and tested
- local tests and benchmarks validate the capability-owner model without
  depending on live LLM access
