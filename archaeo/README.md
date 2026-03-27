# Archaeo

`archaeo` is the archaeology and living-plan runtime for Relurpify.

It exists to make archaeology, intent grounding, learning interactions, tension
tracking, and versioned living-plan state durable, inspectable, and reusable
across multiple agent and UX surfaces.

The core idea is simple:

- LLMs are useful because they can surface patterns, tensions, and possible
  structures that a developer may not see from their current vantage point.
- That inference should not live only inside a transient model context.
- The system should turn those inferences into durable artifacts:
  exploration sessions, learning interactions, tensions, plan versions,
  convergence records, projections, and events.

`archaeo` is the package namespace that owns that durable domain runtime.

## Why This Exists

Relurpify has two different kinds of intelligence in play:

- execution intelligence
- archaeology intelligence

Execution intelligence is what a coding agent like `named/euclo` uses to carry
out an accepted plan, route capabilities, recover from failures, and produce
artifacts.

Archaeology intelligence is different. It is concerned with:

- discovering patterns already present in a codebase
- surfacing tensions, contradictions, and drift
- grounding inferred intent in evidence
- involving the developer in semantic confirmation and refinement
- evolving a living plan over time as the codebase and the understanding of it
  change

Those concerns are too important to leave embedded only inside a single agent
implementation, and too domain-specific to treat as generic middleware.

That is why `archaeo` is a first-class namespace:

- it depends on `framework/*` primitives
- it is consumed by `named/euclo`
- it is surfaced by apps such as `app/relurpish`
- it is informed by archaeology-specialist capability providers such as
  `agents/relurpic`

## What Archaeo Is

`archaeo` is:

- a domain runtime
- a durable state owner
- an event and projection producer
- a plan-lifecycle manager
- a semantic interaction system

`archaeo` is not:

- the generic agent substrate
- the GraphQL transport itself
- the TUI/UX layer
- the code-execution agent

Those concerns integrate with it, but they are not the same thing.

## Design Intent

The long-term design goal is to let the system do this:

1. explore a workspace and surface candidate patterns, anchors, tensions, and
   semantic possibilities
2. ask the developer to confirm, reject, refine, or defer those findings
3. persist that learning as durable domain artifacts
4. produce and evolve versioned living plans grounded in those artifacts
5. hand accepted plans off to execution-oriented systems such as `euclo`
6. feed execution and verification results back into archaeology and plan
   evolution

This is the main reason `archaeo` is separate from `named/euclo`: execution is
only one part of the loop.

## Package Structure

Current subpackages:

- [`archaeo/domain`](/home/lex/Public/Relurpify/archaeo/domain)
  - core domain records and enums
  - phases, exploration sessions, snapshots, tensions, convergence state,
    versioned plans, handoff records, timeline events

- [`archaeo/phases`](/home/lex/Public/Relurpify/archaeo/phases)
  - durable phase-state transitions
  - phase driver logic
  - blocked/deferred/surfacing/completion semantics

- [`archaeo/archaeology`](/home/lex/Public/Relurpify/archaeo/archaeology)
  - exploration session and snapshot lifecycle
  - archaeology-side preparation and refresh logic
  - current integration point between exploration state and living-plan prep

- [`archaeo/learning`](/home/lex/Public/Relurpify/archaeo/learning)
  - learning interaction model
  - blocking vs non-blocking learning
  - proposal sync from patterns, drifts, and tensions
  - resolution side effects into patterns, anchors, tensions, and plan state
  - broker for live learning interaction delivery

- [`archaeo/tensions`](/home/lex/Public/Relurpify/archaeo/tensions)
  - tension artifact lifecycle
  - summary and status management
  - accepted vs unresolved tension semantics

- [`archaeo/plans`](/home/lex/Public/Relurpify/archaeo/plans)
  - active living-plan loading
  - versioned plan lifecycle
  - successor draft generation
  - version comparison
  - active-version alignment with exploration drift

- [`archaeo/execution`](/home/lex/Public/Relurpify/archaeo/execution)
  - execution-preflight helpers
  - execution handoff records
  - step finalization and execution-session support
  - intentionally execution-adjacent because accepted plans must be handed off
    cleanly

- [`archaeo/verification`](/home/lex/Public/Relurpify/archaeo/verification)
  - convergence finalization
  - convergence failure surfacing
  - unresolved-tension-aware verification state

- [`archaeo/events`](/home/lex/Public/Relurpify/archaeo/events)
  - durable archaeology event vocabulary
  - workflow event log adapter
  - projection snapshot storage helpers

- [`archaeo/projections`](/home/lex/Public/Relurpify/archaeo/projections)
  - read models for exploration, learning queue, active plan, and timeline
  - workflow-level materialized views
  - live projection subscription support

- [`archaeo/providers`](/home/lex/Public/Relurpify/archaeo/providers)
  - runtime-facing archaeology provider contracts
  - pattern surfacing, tension analysis, prospective analysis, and convergence
    review interfaces
  - intended to be implemented by archaeology-specialist capability families
    such as relurpic

- [`archaeo/bindings/euclo`](/home/lex/Public/Relurpify/archaeo/bindings/euclo)
  - direct in-process euclo adapter over archaeology runtime services
  - keeps euclo execution-focused while avoiding archaeology package-by-package
    wiring in the agent

- [`archaeo/bindings/relurpish`](/home/lex/Public/Relurpify/archaeo/bindings/relurpish)
  - direct in-process relurpish-facing archaeology adapter
  - exposes projection and interaction surfaces without requiring transport

- [`archaeo/benchmark`](/home/lex/Public/Relurpify/archaeo/benchmark)
  - deterministic benchmark fixtures and archaeology/runtime benchmarks
  - no live LLM dependency

## Dependency Direction

The intended dependency direction is:

```text
framework/*  -> foundational primitives
archaeo/*    -> archaeology domain runtime
named/euclo  -> coding/execution runtime using archaeo
app/*        -> UX layers using euclo and/or archaeo
agents/*     -> worker/capability implementations that feed or consume archaeo
```

In other words:

- `archaeo` depends on `framework/*`
- `archaeo` does not depend on `app/*`
- `named/euclo` depends on `archaeo`
- `named/euclo` integrates through [`archaeo/bindings/euclo`](/home/lex/Public/Relurpify/archaeo/bindings/euclo)
- `app/relurpish` depends on `archaeo` bindings through runtime surfaces
- `agents/relurpic` contributes archaeology-specialist behavior that integrates
  with `archaeo`

This is why `archaeo` is a root namespace rather than a subpackage under
`named/euclo`.

## Key Framework Dependencies

`archaeo` currently depends heavily on lower-level framework packages:

- `framework/memory`
  - workflow state, artifacts, event persistence, runtime surfaces

- `framework/plan`
  - living-plan structures, step state, convergence contracts

- `framework/patterns`
  - pattern records, comments, confirmations, proposals

- `framework/retrieval`
  - anchors, drift events, semantic evidence

- `framework/guidance`
  - shared interaction mechanics and guidance state used alongside learning

- `framework/graphdb`
  - structural graph and blast-radius support where execution and archaeology
    intersect

- `framework/event`
  - materializer and projection runner infrastructure

- `framework/capability`
  - execution-adjacent integration where archaeology and euclo handoff meet

These are not incidental imports. They are what let `archaeo` persist and
relate the semantic/runtime artifacts that the rest of the system needs.

## Who Depends On Archaeo Today

Current direct consumers include:

- [`named/euclo`](/home/lex/Public/Relurpify/named/euclo)
  - uses `archaeo` for phase state, learning, exploration, plan versioning,
    tensions, projections, and convergence support
  - now integrates through [`archaeo/bindings/euclo`](/home/lex/Public/Relurpify/archaeo/bindings/euclo)
  - this is the main in-process consumer today

- [`app/relurpish/runtime`](/home/lex/Public/Relurpify/app/relurpish/runtime)
  - exposes archaeology-facing runtime methods to the relurpish TUI
  - reads projections, learning queues, tensions, and exploration state
  - is the natural consumer of [`archaeo/bindings/relurpish`](/home/lex/Public/Relurpify/archaeo/bindings/relurpish)

- [`agents/relurpic`](/home/lex/Public/Relurpify/agents/relurpic)
  - feeds archaeology outputs, especially gap/tension-oriented findings, into
    archaeology-domain state
  - is the primary expected implementer of [`archaeo/providers`](/home/lex/Public/Relurpify/archaeo/providers)

- [`named/euclo/benchmark`](/home/lex/Public/Relurpify/named/euclo/benchmark)
  and [`archaeo/benchmark`](/home/lex/Public/Relurpify/archaeo/benchmark)
  - use the runtime as deterministic performance audit surfaces

## What Features The System Provides

Today `archaeo` provides the following substantive features.

### 1. Explicit phase machine

The runtime can persist and transition between archaeology-oriented phases such
as:

- archaeology
- intent elicitation
- plan formation
- execution
- verification
- surfacing
- blocked
- deferred
- completed

This phase state is durable rather than implicit in a single call stack.

### 2. Workspace exploration sessions and snapshots

`archaeo` can persist exploration sessions and exploration snapshots so that the
system can track:

- which workspace is being explored
- what revision or semantic snapshot it was based on
- candidate patterns
- candidate anchors
- open learning interactions
- tension references
- recompute and stale state

### 3. First-class learning interactions

Learning is not treated as the same thing as operational guidance.

`archaeo` models semantic interactions explicitly:

- confirm
- reject
- refine
- defer

These interactions can apply to:

- patterns
- anchors
- tensions
- exploration interpretations

and their resolutions can feed back into:

- pattern state
- anchor state
- tension state
- plan confidence
- plan version staleness and successor drafts

### 4. Tension and coherence tracking

Tensions are first-class artifacts, not just debug notes.

The runtime tracks:

- tension kind and severity
- pattern and anchor links
- symbol scope
- related plan steps
- status such as inferred, confirmed, accepted, unresolved, resolved
- summary-level coherence state per exploration/workflow

### 5. Versioned living plans

Living plans are versioned artifacts, not just mutable blobs.

The runtime supports:

- draft vs active vs superseded/archived versions
- parent-version linkage
- exploration/snapshot linkage
- successor draft generation on drift or semantic change
- plan-version comparison

### 6. Execution handoff boundary

`archaeo` explicitly records the handoff from archaeology/plan lifecycle into
execution:

- which exploration was involved
- which plan version is being executed
- which step is being handed off
- which revision/snapshot it was based on

This is important because `archaeo` should own the durable reasoning state even
when another system performs execution.

### 7. Durable events and projections

The runtime produces durable events and read models so that consumers do not
need to reconstruct state manually from raw store tables.

Today that includes projections for:

- workflow state
- exploration state
- learning queue
- active plan
- timeline/history

### 8. Deterministic benchmarking

Because archaeology behavior must be auditable and optimizable outside live LLM
variance, the package also owns a deterministic benchmark suite.

This is used to measure:

- archaeology preparation
- learning sync and resolution
- plan version comparisons and drift handling
- projection rebuilds and subscriptions

## Relationship To Euclo

`archaeo` is not separate from euclo in the product sense.

It is separate from euclo in the package and architecture sense.

`named/euclo` is still the default coding/runtime agent that:

- routes capabilities
- executes accepted plan steps
- manages profile and mode selection
- produces execution artifacts

But euclo should not be the sole owner of archaeology state or semantics.

That is why `archaeo` exists: it holds the durable reasoning layer that euclo
uses, rather than burying archaeology inside the euclo agent implementation.

## Relationship To Relurpic

`agents/relurpic` is currently the most important archaeology-specialist
behavior provider.

Relurpic contributes:

- pattern-oriented capabilities
- gap and contradiction analysis
- prospective matching
- convergence verification
- comment and semantic review flows

Architecturally, these are best thought of as archaeology behavior providers
that feed the `archaeo` runtime, rather than as the runtime itself.

## Relationship To UX And GraphQL

`archaeo` is intentionally not the TUI.

It should support two consumption modes:

- direct in-process bindings
  - used by `named/euclo`
  - used by `app/relurpish/runtime`

- transport-driven access
  - future GraphQL server and subscriptions
  - alternate UX clients

That is why the runtime packages should remain UX-agnostic and transport-agnostic.

## Current Scope vs Future Scope

What exists now:

- domain runtime packages
- durable events
- projections
- in-process euclo and relurpish integration
- deterministic benchmarks

What is still planned but not complete:

- GraphQL server package and transport boundary
- dedicated requests package for durable longer-running archaeology and
  verification work requests, fulfilled externally rather than by hidden worker
  execution inside `archaeo`
- richer plan-formation runtime separate from execution preparation
- broader provenance/query surfaces
- additional direct bindings and alternate client validation

## Why This Matters

Without `archaeo`, archaeology logic tends to collapse into one of two bad
shapes:

- transient LLM behavior that cannot be inspected or resumed
- ad hoc execution-agent state that is too tightly coupled to one runtime

`archaeo` prevents that by making the system's semantic reasoning artifacts
durable and reusable.

That is the real purpose of this namespace:

- not to replace `euclo`
- not to replace `relurpic`
- not to replace `framework`

but to provide the durable archaeology and living-plan runtime that those parts
of the system can all meet around.
