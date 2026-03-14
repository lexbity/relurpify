# Blackboard Runtime Architecture

## Synopsis

This document defines the target execution model for `agents/blackboard`.

The goal is not to preserve the current prototype loop. The goal is to make
BlackboardAgent a first-class Relurpify runtime that expresses the blackboard
paradigm through framework-owned execution surfaces:

- `framework/graph` for execution and checkpoint boundaries
- `framework/core.Context` for working state and derived knowledge
- `framework/capability.Registry` for all tool execution
- `framework/memory` for durable workflow, checkpoint, and memory persistence
- framework telemetry, audit, and policy enforcement surfaces

The blackboard paradigm remains agent-owned. The execution substrate remains
framework-owned.

## Current Status

The graph-native runtime described here is now implemented in `agents/blackboard`.

Current implementation status:

- controller execution runs as a graph-native load → evaluate → dispatch loop
- namespaced `core.Context` keys are the canonical shared-state surface
- capability-routed tool and delegated-agent execution flow through the registry
- resumable callback checkpoints are implemented, with explicit checkpoint nodes
  available as persistence boundaries
- declarative and procedural retrieval hydrate prior memory before dispatch
- structured persistence writes blackboard summary, decision, and routine data
- telemetry, audit trail state, and execution summary state are emitted
- controller concurrency semantics are explicit: single-fire serial with
  reject-conflicts branch-merge rules defined for future parallel KS dispatch

Remaining work is therefore extension work, not a separate reimplementation.

## Scope

This document defines:

- the target control model
- state ownership and data placement
- controller-cycle semantics
- knowledge-source scheduling semantics
- retry and failure semantics
- durability and recovery requirements

This document does not define:

- the exact production prompt text for any knowledge source
- the final UX for TUI visualization
- the full production library of built-in knowledge sources

## Architectural Position

BlackboardAgent is a concrete agent runtime in `agents/`.

It must not introduce an execution substrate parallel to `framework/graph` or
`framework/pipeline`. A blackboard controller may make dynamic choices, but
those choices must still execute inside framework-native runtime boundaries so
that the agent inherits:

- capability preflight
- node contracts
- policy-enforced capability invocation
- resumable checkpoints
- state-boundary validation
- telemetry and audit trails

The graph runtime is therefore the authoritative execution engine. Blackboard
logic is expressed as graph-native controller and knowledge-source steps.

## Target Execution Model

The target blackboard runtime is a graph-driven control loop with one logical
controller cycle per dispatch pass.

Each cycle has these conceptual stages:

1. load or hydrate blackboard state
2. evaluate all registered knowledge sources against current state
3. rank eligible knowledge sources
4. select the next knowledge source to run
5. execute that knowledge source through a framework-native step
6. validate and persist the resulting state delta
7. test termination conditions
8. either checkpoint and continue, or checkpoint and terminate

The controller may remain single-dispatch per cycle in the first production
implementation. Multi-dispatch or parallel dispatch is a later extension, not a
phase-1 requirement.

## State Ownership

`core.Context` is the canonical in-memory blackboard.

The current package-local `Blackboard` struct is a prototype compatibility
surface, not the target long-term owner of runtime state.

The target state layout is namespaced under shared context keys such as:

- `blackboard.goals`
- `blackboard.facts`
- `blackboard.hypotheses`
- `blackboard.issues`
- `blackboard.pending_actions`
- `blackboard.completed_actions`
- `blackboard.artifacts`
- `blackboard.controller`
- `blackboard.metrics`

State placement rules:

- durable workflow-visible records belong in `Context` shared state
- derived summaries and controller hints may live in `Context` knowledge
- transient per-step scratch values belong in `Context` variables

This preserves compatibility with framework cloning, dirty-delta tracking, and
merge semantics.

## Knowledge Source Model

A knowledge source is a specialized runtime unit with four responsibilities:

1. declare when it is eligible to run
2. declare priority and optional scheduling metadata
3. produce a validated state delta
4. declare what capabilities or side effects it requires

Target properties of a knowledge source:

- stable name
- activation predicate over blackboard state
- priority
- optional cooldown/backoff metadata
- explicit required capability selectors
- explicit node contract metadata where side effects exist

Knowledge sources should not bypass framework capability routing. If a knowledge
source needs a tool, it must invoke it through the admitted capability surface.
If it delegates to another agent runtime, that delegation must also be explicit
and auditable.

## Scheduling Semantics

The scheduler is data-driven, not structurally hardcoded, but it must still be
deterministic given the same blackboard state and capability environment.

Phase-1 target scheduling rules:

- all registered knowledge sources are evaluated every cycle
- only eligible sources participate in ranking
- the scheduler selects exactly one source per cycle
- highest priority wins
- ties are broken by stable source name
- after execution, the controller re-evaluates from fresh blackboard state

Fairness rules for later phases may add cooldowns, starvation prevention, or
quota-based dispatch. The initial production runtime should optimize first for
determinism, inspectability, and replay safety.

## Termination Semantics

Termination must not be inferred from incidental state shape alone.

The target runtime terminates only when one of these explicit conditions holds:

- a success policy reports the task satisfied
- a terminal failure policy reports the task cannot proceed
- a configured cycle limit is reached
- cancellation or timeout is received from the parent context

The success policy should be configurable and able to consider:

- verified artifacts
- completed required action classes
- explicit goal-satisfaction markers
- verification outputs

The controller must also produce an explicit termination reason for telemetry,
debugging, and resumable recovery.

## Retry and Failure Semantics

Retry semantics are controller-owned policy, not ad hoc per-source behavior.

Phase-1 target rules:

- knowledge-source execution failures are recorded in blackboard state
- the controller classifies the failure as retryable or terminal
- retryable failures may requeue the same knowledge source with bounded attempts
- terminal failures stop execution with a structured reason
- failed actions become first-class blackboard records, not silent drops

External side effects must be treated as replay-sensitive. Recovery behavior
must align with graph checkpoint semantics so completed single-shot actions are
not replayed after resume.

## Durability and Recovery

BlackboardAgent must support real crash-safe recovery.

That means:

- checkpoints are persisted through framework checkpoint stores
- checkpoints are taken at graph transition boundaries or explicit
  `CheckpointNode` boundaries
- blackboard state needed for resume is serializable and versioned
- resumed execution continues from the next controller step rather than
  replaying already-completed side-effecting work

In-process storage of raw pointers inside `Context` is not sufficient as the
primary recovery mechanism.

## Persistence and Memory Requirements

Blackboard execution should integrate with durable memory lanes rather than
keeping all useful output in transient working state.

The target design supports:

- declarative persistence for durable facts, decisions, and findings
- procedural persistence for reusable routines or successful control patterns
- artifact persistence for reports, summaries, patches, or verification output
- audit persistence for why records were written

Retrieval is also part of the target runtime. Blackboard runs should be able to
hydrate relevant project memory before the controller begins dispatching.

## Observability Requirements

The blackboard runtime must expose enough structure to debug and review a run.

Required observability surfaces:

- telemetry event per controller cycle
- telemetry for eligibility evaluation and selected source
- telemetry for dispatched actions and completion status
- explicit termination reason
- graph structure that reflects the actual runtime flow
- state summaries that the TUI and tests can inspect

## Non-Goals

The target design does not attempt to:

- replace the framework graph runtime
- add a second, agent-owned checkpoint subsystem
- permit direct tool execution outside capability policy
- treat blackboard as an unstructured shared map with no state conventions

## Extension Guidance

This document should now be read as the architectural contract for the shipped
runtime plus the guide for follow-on extensions.

Near-term extension areas include:

- richer production knowledge-source libraries for domain-specific workflows
- optional multi-dispatch or parallel KS execution on top of the defined
  branch-merge semantics
- deeper TUI visualization of controller contenders, dispatch outcomes, and
  persisted audit history
- additional policy controls for fairness, cooldown, and starvation prevention
