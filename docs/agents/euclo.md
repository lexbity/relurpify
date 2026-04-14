# Euclo

## Synopsis

Euclo is Relurpify's named coding runtime in `named/euclo/`.

It is not just a generic coding wrapper. Euclo is a modal runtime for software
development and data-oriented code investigation that:

- assembles a `UnitOfWork`
- selects a Euclo-owned relurpic capability owner
- composes execution paradigms from `/agents`
- uses `/framework` for capability, skill, context, restore, and policy
- uses `/archaeo` for memory, provenance, living-plan state, and knowledge relationships

Euclo is UX-agnostic. The same runtime can be driven from a chat surface, a
research/planning UI, a CLI, or a programmatic integration.

---

## Runtime Model

Euclo executes around a `UnitOfWork`.

A `UnitOfWork` binds:

- mode
- execution objective
- context strategy
- semantic inputs
- resolved execution policy
- primary relurpic capability owner
- supporting relurpic capabilities
- executor recipe
- verification, checkpoint, and deferral policy

Euclo then runs that `UnitOfWork` under shared runtime guarantees:

- framework policy remains authoritative
- context and restore are tracked explicitly
- deferred issues are durable artifacts
- compiled execution continuity is persisted
- plan-backed execution stays bound to one compiled plan per run

---

## Modes

Euclo currently treats modes as software-development execution paradigms rather
than UX labels.

### `chat`

Broad collect-context plus toolbox execution.

This is the general coding surface for:

- asking engineering questions
- implementing changes
- inspecting code and current behavior

The quality of `chat` depends heavily on model quality, so Euclo constrains and
structures behavior rather than assuming a strong model.

### `planning`

Archaeo-backed living-plan exploration and compilation.

This is the data-oriented mode for:

- codebase exploration
- pattern surfacing
- prospective architectural assessment
- plan compilation
- plan-backed implementation

It uses Archaeo as the memory/provenance substrate and is designed for wider
scope exploration and longer context.

### `debug`

Mixed investigation mode between `chat` and `planning`.

This mode combines:

- drill-down debugging
- tool-output exposition
- root-cause and localization behavior
- Archaeo-backed semantic memory when useful
- controlled escalation into implementation when repair is required

Context, memory, and session continuity may be shared across modes. Some mode
transitions preserve the same `UnitOfWork`; others rebind to a successor work
unit when the runtime contract changes materially.

---

## Euclo-Owned Relurpic Capabilities

Relurpic capability is a framework primitive through
`CapabilityRuntimeFamilyRelurpic`.

Euclo owns the coding-specific relurpic capability catalog in
`named/euclo/relurpic`.

### Primary-capable endpoints

Chat:

- `euclo:chat.ask`
- `euclo:chat.implement`
- `euclo:chat.inspect`

Planning:

- `euclo:archaeology.explore`
- `euclo:archaeology.compile-plan`
- `euclo:archaeology.implement-plan`

Debug:

- `euclo:debug.investigate-repair`

### Supporting capabilities

Supporting capabilities are explicit runtime bindings under a primary owner.
Examples include:

- `euclo:chat.direct-edit-execution`
- `euclo:chat.local-review`
- `euclo:chat.targeted-verification-repair`
- `euclo:archaeology.pattern-surface`
- `euclo:archaeology.prospective-assess`
- `euclo:archaeology.convergence-guard`
- `euclo:debug.root-cause`
- `euclo:debug.localization`
- `euclo:debug.verification-repair`

Some relurpic capabilities are Archaeo-associated by design. Others are
Euclo-local and rely only on framework facilities.

---

## Execution Ownership

Euclo owns orchestration.

Relurpic capabilities own coding-specific behavior assemblies.

`/agents` provides reusable execution paradigms such as:

- ReAct-style iterative tool use
- planner-style structured decomposition
- HTN-style drill-down execution
- Rewoo-style long-running execution
- reflection-style review/correction

Euclo composes those paradigms under relurpic capability ownership instead of
treating one paradigm as its identity.

This means executor choice is subordinate to behavior choice:

`mode -> UnitOfWork -> relurpic capability owner -> executor recipe`

---

## Relationship To Framework And Archaeo

Euclo is layered on top of `/framework`, `/agents`, and `/archaeo`.

### Framework

Framework owns:

- capabilities and relurpic capabilities as primitives
- skill policy
- capability and tool admission
- sandbox and permission enforcement
- context management
- provider snapshot and restore interfaces

Euclo consumes framework policy and reports compatibility against the
framework-admitted execution catalog. Euclo does not act as the permission
authority.

### Archaeo

Archaeo owns:

- memory
- provenance
- living-plan state
- request/evaluation history
- knowledge relationships

Euclo executes against that state but does not push Euclo-specific runtime
semantics down into Archaeo.

---

## Plans, Deferrals, And Continuity

For plan-backed execution:

- one run binds to one fully compiled plan
- Euclo does not create successor plans during execution
- unresolved issues become deferred execution artifacts
- the developer can later revisit those deferrals and re-enter archaeology

Euclo persists:

- `UnitOfWork`
- compiled execution
- runtime execution status
- deferred execution issues
- context lifecycle state
- final report output

Restore can rebuild continuity from persisted Euclo artifacts and framework
provider/session snapshot state.

---

## Runtime Reporting

Euclo publishes runtime surfaces for local testing, persistence, and UX
integration, including:

- `euclo.compiled_execution`
- `euclo.execution_status`
- `euclo.semantic_inputs`
- `euclo.context_runtime`
- `euclo.security_runtime`
- `euclo.chat_capability_runtime`
- `euclo.debug_capability_runtime`
- `euclo.archaeology_capability_runtime`
- `euclo.unit_of_work_transition`
- `euclo.unit_of_work_history`
- `euclo.shared_context_runtime`

Euclo also now publishes assurance- and proof-oriented runtime surfaces,
including:

- `euclo.success_gate`
- `euclo.proof_surface`
- `euclo.verification_plan`
- `euclo.tdd.lifecycle`
- `euclo.waiver`

Those surfaces distinguish execution outcome from proof quality. Fresh edits
must be backed by executed current-run verification evidence unless an explicit
operator waiver is present.

---

## Assurance Model

Euclo reports runtime outcome and assurance as separate dimensions.

`result_class` answers whether the run completed, blocked, failed, or completed
with deferrals.

`assurance_class` answers how trustworthy the completion claim is. Current
assurance classes include:

- `verified_success`
- `partially_verified`
- `unverified_success`
- `review_blocked`
- `repair_exhausted`
- `tdd_incomplete`
- `operator_deferred`

This split is intentional:

- lifecycle outcome remains stable for runtime and restore consumers
- proof quality remains explicit for operators and downstream tooling

Euclo does not allow fallback or reused verification evidence to prove fresh
mutating success.

---

## Verification, Review, And Repair

For mutating work, Euclo now enforces a hardened path:

- semantic review gates all automatic mutation flows
- verification scope is selected explicitly and persisted as
  `euclo.verification_plan`
- verification execution records command-level evidence tied to the current run
- failed verification enters bounded repair rather than soft success
- exhausted repair loops surface as `repair_exhausted`

Language- and tool-specific verification planning is delegated outward:

- `named/euclo` owns assurance, gating, and reporting
- `/framework` owns generic planning interfaces and policy overlays
- `/platform` owns backend-specific verification scope resolution

---

## TDD Contract

TDD is no longer treated as a generic edit-and-verify variation.

In `test_driven_generation`, Euclo requires:

- red-phase evidence
- green-phase evidence
- explicit lifecycle state in `euclo.tdd.lifecycle`
- refactor evidence when refactor was requested

Bugfix-shaped TDD and debug flows may synthesize a reproducer first through
`euclo:test.regression_synthesize`, but that synthesized reproducer is surfaced
explicitly in artifacts and state.

---

## Waivers And Deferrals

Waivers are explicit and auditable.

An operator waiver:

- is surfaced as `euclo.waiver`
- is reflected in the success gate and proof surface
- yields `assurance_class=operator_deferred`
- is projected into deferred issue state
- may be linked into Archaeo provenance when that substrate is configured

Automatic degraded operation remains distinct from an operator waiver and does
not silently authorize proof claims.

These runtime objects are the main contract for local engineering tests. The
full live-model end-to-end suite remains separate in `/testsuite`.

---

## Package Structure

```text
named/euclo/
├── agent.go
├── executors.go
├── relurpic/          # Euclo-owned relurpic capability catalog
├── runtime/           # UnitOfWork, semantic inputs, restore, runtime state
├── capabilities/      # capability implementations and registry support
├── orchestrate/       # profile/recovery/controller support
├── interaction/       # interaction machinery and mode-facing flow
├── gate/              # evidence and success gating
├── euclotypes/        # shared Euclo value types and artifacts
├── benchmark/         # deterministic local benchmarks
└── euclotest/         # local runtime and integration tests
```

---

## Selection

Euclo is selected via manifest:

```yaml
spec:
  agent:
    implementation: coding
```

`coding` and `euclo` resolve to the same named runtime.
