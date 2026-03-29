# Plan 08: Archaeo Runtime Boundaries and Execution-Time Mutation Model

## Status

Proposed engineering specification and phased implementation plan.

This document defines the next architectural improvements around:

- `archaeo` as a first-class runtime namespace
- the separation between `archaeo`, `named/euclo`, relurpic capabilities, and
  UX/app transport layers
- execution-time archaeology mutations and their effect on active execution
- direct bindings versus GraphQL transport

This plan assumes the namespace migration to root-level `archaeo/` has already
started and that Phases 1-5 of the earlier archaeology-runtime work are
substantially implemented, while GraphQL and relurpish-server integration are
not yet complete.

## Goal

The system should be able to use LLMs to surface possibilities, patterns,
contradictions, and structural interpretations that are not yet explicit in the
developer's local mental model, while keeping the resulting archaeology and
living-plan state durable, inspectable, and challengeable.

The core property we want is:

The system must be able to continue reasoning about a codebase as the codebase
changes, while execution remains anchored to explicit handoff/version state
rather than to a transient and implicit LLM context.

That implies three architectural requirements:

1. `archaeo` must own durable archaeology-domain state and lifecycle.
2. `named/euclo` must remain execution-focused and should not permanently own
   archaeology lifecycle coordination just because the implementation started
   there.
3. execution-time archaeology changes must be modeled explicitly rather than
   handled only by ad hoc phase transitions or scattered invalidation logic.

## Why This Plan Exists

`archaeo` inherited part of its current shape from euclo's execution path. That
was the correct bootstrapping path, but it leaves a few unresolved tensions:

- `named/euclo/agent.go` still acts as a composition root for too much
  archaeology logic
- execution-time semantic change handling exists, but it is distributed across
  learning, tensions, plan versioning, and preflight logic rather than modeled
  explicitly
- relurpic behavior is already essential to archaeology, but it is still
  expressed mostly as capability handlers rather than through runtime-facing
  archaeology provider interfaces
- the GraphQL server should be transport/app level, not the archaeology runtime
  itself

This plan addresses those tensions directly.

## Architectural Position

The intended layering is:

1. `framework/*`
2. `archaeo/*`
3. `agents/*`
4. `named/*`
5. `app/*`

This is not a strict inheritance hierarchy. It is a dependency-direction and
responsibility model.

### Framework

`framework/*` remains the reusable substrate:

- graph
- retrieval
- patterns/comments
- guidance broker
- memory/event/persistence primitives
- capability/sandbox infrastructure
- plan primitives

### Archaeo

`archaeo/*` is the durable archaeology runtime.

It should own:

- archaeology lifecycle
- exploration sessions and snapshots
- learning interactions
- tensions/coherence
- versioned living-plan lifecycle
- execution handoff artifacts
- archaeology events and projections
- archaeology requests
- direct in-process bindings

It should not own:

- generic agent paradigms
- code-edit execution coordination
- LLM execution loops for archaeology reasoning
- the TUI or any specific UX
- GraphQL transport logic embedded into core domain packages

`archaeo/requests` should model durable requested work, dispatch state, and
results, not hidden worker execution. If a request requires LLM-backed
reasoning, fulfillment should happen through relurpic capability families,
typically dispatched through relurpic capabilities under euclo execution
coordination.

### Agents and Relurpic

`agents/*` remains the substrate for agent paradigms, capability composition,
subagents, and skills.

`agents/relurpic/*` should be treated as the primary archaeology capability
provider. In this context, the right abstraction is capability families with
targeted workflow behavior, not just generic worker classes:

- subagent behavior
- skill-guided behavior
- tool/capability composition
- targeted workflow behavior

Relurpic should continue to provide:

- pattern surfacing
- gap/tension analysis
- prospective analysis
- convergence review

but it should do so behind runtime-facing `archaeo` service interfaces rather
than by being permanently coupled to capability-handler wiring.

### Euclo

`named/euclo/*` should remain execution-focused.

Euclo should own:

- mode/profile selection
- capability routing
- execution-session coordination
- recovery/escalation during execution
- production of execution artifacts
- honoring execution-time archaeology signals produced by `archaeo`

Euclo should consume archaeology state, not be the permanent owner of it.

### Apps and Transport

The GraphQL server should be treated as an application/runtime that depends on
`archaeo`, not as the `archaeo` runtime itself.

That implies a likely shape such as:

- `archaeo/*`
- `app/archaeo-graphql-server/*`
- `app/relurpish/*`

This keeps:

- runtime/domain logic in `archaeo`
- transport in an app/server package
- direct in-process bindings available for euclo and relurpish

## Direct Runtime vs GraphQL vs Direct Bindings

There are three distinct surfaces to keep separate.

### 1. Archaeo runtime

This is the authoritative state and lifecycle layer.

It should provide domain operations such as:

- ensure exploration session
- create/update exploration snapshot
- create/resolve learning interaction
- create/update tension
- sync active plan version with exploration
- produce projections
- emit archaeology events

### 2. GraphQL transport

This is the UX-agnostic transport surface over the runtime.

It should expose:

- queries against projections/read models
- mutations for archaeology-domain state transitions
- subscriptions over domain events and projections

It should not reimplement domain logic.

### 3. Direct bindings

These are in-process adapters for runtimes that should not be forced through
GraphQL.

Examples:

- `archaeo/bindings/euclo`
- `archaeo/bindings/relurpish`

These bindings should allow direct use of `archaeo` services without the
overhead or coupling of transport.

## Execution-Time Mutation Model

Execution-time archaeology changes must be modeled explicitly.

Phases are not enough.

Phases answer:

- what lifecycle state is this workflow in?

Mutation modeling answers:

- what semantic change just occurred?
- how widely does it spread?
- what is its effect on the current execution handoff?
- what should euclo do now?

### Why phase state is not sufficient

A workflow can remain in `execution` phase while receiving very different kinds
of archaeology changes:

- a harmless observational note
- a non-blocking confidence drop
- a new unresolved tension touching future steps
- a current-step anchor drift that invalidates the handoff
- a semantics change that requires successor draft generation

All of these happening during `execution` does not mean they have the same
runtime implications. That is why mutation modeling is required in addition to
phases.

### Required concepts

The execution-time mutation model should introduce four related but distinct
concepts.

#### Mutation category

What type of semantic change occurred?

Candidate categories:

- `observation`
- `confidence_change`
- `step_invalidation`
- `plan_staleness`
- `blocking_semantic`

#### Blast radius

How broadly does the change spread through the current model?

Blast radius should describe structural spread, not policy.

It should include:

- affected step IDs
- affected symbols or graph nodes
- affected patterns
- affected anchors
- rough scope: local / step / plan / workflow / workspace

#### Mutation impact

What is the execution significance of the change?

This should be strongly informed by blast radius, but should not be identical
to blast radius.

Why not identical:

- a small local change can be execution-critical if it touches a required anchor
  for the active step
- a broad change can be non-blocking if it does not threaten the current
  handoff

Candidate impacts:

- `informational`
- `advisory`
- `caution`
- `local_blocking`
- `handoff_invalidating`
- `plan_recompute_required`

#### Execution disposition

What should the execution runtime do in response?

Candidate dispositions:

- `continue`
- `continue_on_stale_plan`
- `pause_for_learning`
- `pause_for_guidance`
- `invalidate_step`
- `block_execution`
- `require_replan`

This lets `archaeo` publish a clear execution-facing signal while preserving the
ability for euclo to apply policy.

### Relationship to existing constructs

The system already has partial implementations of these ideas:

- blocking learning in `archaeo/learning`
- tension status and related plan step IDs in `archaeo/tensions`
- plan staleness and successor drafts in `archaeo/plans`
- preflight invalidation and blocked-step handling in `archaeo/execution`
- phase state in `archaeo/phases`
- execution handoff in `archaeo/execution/handoff.go`

The improvement is not inventing these signals from zero. The improvement is
making them explicit and consolidated.

## Package Impact

This plan affects the following packages most directly.

### `archaeo/domain`

Add:

- `MutationCategory`
- `BlastRadius`
- `MutationImpact`
- `ExecutionDisposition`
- `MutationEvent`

Potentially also:

- helper enums for blast-radius scope
- helper structs for impact evaluation results

### `archaeo/events`

Add:

- archaeology-domain mutation event types
- request lifecycle event types
- append helpers for mutation events
- event payload shapes for mutation records

### `archaeo/projections`

Add:

- mutation/history projection
- request history projection
- execution-impact projection or mutation queue view if needed
- possibly coherence-oriented read models that combine tensions, confidence, and
  execution dispositions

### `archaeo/requests`

Add:

- durable requested-work records
- request lifecycle status and dispatch metadata
- request-to-result linkage
- request query helpers and supporting events

This package should record requested work, not execute LLM-backed reasoning
itself.

### `archaeo/learning`

Extend to emit mutation events when:

- learning resolutions confirm/reject/refine semantic artifacts
- confidence changes
- blocking learning appears or is cleared

### `archaeo/tensions`

Extend to emit mutation events when:

- tensions become unresolved/accepted/resolved
- tensions touch active plan steps
- tensions should create execution-facing blocking signals

### `archaeo/plans`

Extend to emit mutation events when:

- active versions become stale
- successor drafts are created
- version alignment with exploration changes
- plan comparison identifies current-handoff-threatening drift

Also add first-class plan formation capabilities so draft formation is not
treated as only a side-effect of version alignment or execution preparation.

### `archaeo/execution`

Add:

- impact evaluation helpers
- handoff-state interpretation of mutation events
- explicit execution-facing mutation consumption contracts

### `archaeo/bindings/euclo`

Introduce:

- a focused euclo-facing adapter over archaeology runtime services
- conversion from archaeology-domain mutation signals into euclo execution
  behavior

This package does not exist yet, but it should become the main integration seam
instead of `named/euclo/agent.go` directly importing many `archaeo/*`
subpackages.

### `archaeo/bindings/relurpish`

Introduce:

- relurpish-facing in-process adapters for reading projections and resolving
  archaeology interactions

This package may remain light, but it should exist conceptually as a boundary.

### `archaeo/interfaces` or runtime interfaces within subpackages

Introduce runtime-facing provider interfaces for archaeology behavior supplied by
relurpic capability families, such as:

- pattern surfacing
- tension analysis
- prospective analysis
- convergence review

### `agents/relurpic`

Refactor progressively to implement archaeology-facing provider interfaces while
remaining a capability-backed provider family.

### `named/euclo`

Reduce direct archaeology lifecycle coordination ownership in:

- `named/euclo/agent.go`

and replace it progressively with:

- `archaeo/bindings/euclo`

Euclo should continue to own execution coordination, not archaeology state
ownership.

## Current Gaps This Plan Addresses

This plan is specifically intended to address the following current gaps:

### Runtime gaps inside `archaeo`

- `archaeo` still has no first-class `requests` package for durable requested
  work such as exploration refresh, provider-backed analysis, convergence
  review, or plan reformation.
- exploration sessions are still only partially workspace-owned because the
  current domain and persistence model still carry workflow ownership.
- plan formation is still weaker than the architecture implies: the runtime has
  versioning and alignment, but not a first-class draft-formation lifecycle
  grounded in exploration, learning, and tensions.
- provider interfaces exist, but the default archaeology lifecycle does not yet
  invoke provider-backed pattern surfacing, tension analysis, prospective
  analysis, and convergence review at explicit runtime seams.
- execution-time mutation handling exists, but as a narrow checkpoint model. It
  still needs a fuller checkpoint strategy and better read-model surfacing.
- provenance and mutation-history surfaces remain thinner than the runtime
  state they are meant to explain.

### Integration gaps outside `archaeo`

- euclo still carries too much archaeology-facing composition, even though the
  runtime boundary is now cleaner.
- GraphQL transport remains outside the scope of this plan and should continue
  to be handled separately in `plan-09`.

## Updated Runtime Scope

This plan is now focused on `archaeo` runtime completion before deeper euclo
integration or GraphQL transport work.

That means the priority order is:

1. durable runtime requests
2. workspace-owned exploration cleanup
3. first-class plan formation
4. provider-driven archaeology lifecycle flows
5. richer execution-time mutation checkpoints and read models
6. stronger provenance/history surfaces

## Multi-Phased Implementation Plan



## Test Strategy

Each phase should be validated at three layers where appropriate:

### 1. Domain unit tests

These should live under `archaeo/*` packages and verify:

- type semantics
- lifecycle rules
- event production
- projection behavior
- mutation mapping

### 2. Euclo and binding integration tests

These should verify:

- execution-time consumption of archaeology state and mutation signals
- stable handoff semantics
- no regression in euclo execution behavior

### 3. App/transport tests

These should verify:

- direct binding behavior for relurpish
- GraphQL transport behavior once introduced

The GraphQL layer should never become the only place where archaeology
semantics are tested.

## Risks and Mitigations

### Risk: Phases and mutation model overlap confusingly

Mitigation:

- keep phases for lifecycle
- keep mutation categories for semantic change type
- keep dispositions for runtime response

### Risk: Euclo loses too much execution orchestration authority

Mitigation:

- preserve euclo as the execution control plane
- move archaeology ownership, not execution ownership, into `archaeo`

### Risk: Relurpic becomes awkwardly split

Mitigation:

- treat relurpic as capability-backed provider families
- extract runtime-facing interfaces first, not all behavior at once

### Risk: GraphQL leaks back into runtime design

Mitigation:

- keep GraphQL in an app/server layer
- expose runtime services through direct bindings and transport separately

## Expected Outcomes

When this plan is complete, the system should have:

- a clearer and more stable `archaeo` runtime boundary
- a cleaner split between archaeology lifecycle ownership and execution
  coordination
- explicit execution-time mutation semantics
- relurpic integrated as archaeology capability-provider families
- direct bindings for in-process consumers
- a cleaner path to a GraphQL app runtime without making transport the owner of
  archaeology logic
