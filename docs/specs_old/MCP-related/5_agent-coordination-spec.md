# Agent Coordination And Deployment Engineering Specification

## Status

Draft with Phase 5 Slices 1-5 implemented in the current runtime baseline

## Goal

Evolve Relurpify's partial subagent support into a first-class coordination
system built on the same capability, provider, session, policy, and persistence
architecture used by MCP and other runtime-managed services.

This specification defines how agents delegate work, exchange structured state,
run as local or remote services, and expose or consume agent-like behavior
without reintroducing a separate "subagent special case" runtime.

## Scope

This specification covers:

- delegation contracts
- local and remote agent runtime roles
- workflow-scoped shared resources
- coordination permissions and trust boundaries
- agent-capability registration and discovery
- MCP-imported and MCP-exported agent surfaces
- deployment and recovery expectations for long-lived agent services

This specification does not define:

- low-level MCP transport details
- TUI implementation details
- final task UX
- generic capability schema mechanics already covered elsewhere

## Relationship To The Other Specifications

This document is downstream of:

- [`1_capability-model-spec.md`](/home/lex/Public/Relurpify/docs/MCP-related/1_capability-model-spec.md)
- [`2_security-policy-spec.md`](/home/lex/Public/Relurpify/docs/MCP-related/2_security-policy-spec.md)
- [`3_provider-runtime-spec.md`](/home/lex/Public/Relurpify/docs/MCP-related/3_provider-runtime-spec.md)
- [`4_mcp-core-integration-spec.md`](/home/lex/Public/Relurpify/docs/MCP-related/4_mcp-core-integration-spec.md)

The coordination model must therefore assume:

- capabilities are the primary runtime abstraction
- providers own long-lived services and sessions
- policy gates admission, exposure, execution, and insertion separately
- MCP client/server behavior is already capability- and provider-based

This spec should not create a second registry, a second policy pipeline, or a
second session model just because the target happens to be another agent.

## Current State Review

Relurpify already has pieces of an agent-coordination system, but they are not
yet unified into a framework-native runtime model.

Current strengths:

- graph-based agents with workflow persistence
- multiple local agent implementations and mode-specific delegation in
  [`agents/coding_agent.go`](/home/lex/Public/Relurpify/agents/coding_agent.go)
- manifest/runtime invocation controls in
  [`framework/core/agent_spec.go`](/home/lex/Public/Relurpify/framework/core/agent_spec.go)
- provider lifecycle, provider sessions, and recoverability in
  [`app/relurpish/runtime/providers.go`](/home/lex/Public/Relurpify/app/relurpish/runtime/providers.go)
- capability-native registry and policy enforcement in
  [`framework/capability/capability_registry.go`](/home/lex/Public/Relurpify/framework/capability/capability_registry.go)
- framework-native orchestrated capabilities in
  [`agents/relurpic_capabilities.go`](/home/lex/Public/Relurpify/agents/relurpic_capabilities.go)
- MCP client/server infrastructure and provider integration in
  [`framework/mcp/`](/home/lex/Public/Relurpify/framework/mcp) and
  [`app/relurpish/runtime/mcp_provider.go`](/home/lex/Public/Relurpify/app/relurpish/runtime/mcp_provider.go)

Current gaps:

- no explicit runtime delegation record or delegation result model
- no first-class "agent target" descriptor kind or coordination trait
- no provider-owned local subagent runtime service
- no shared workflow resource model for structured handoff
- no unified treatment of local delegates, runtime services, and MCP-imported
  agent-like capabilities
- no explicit approval, insertion, or recovery rules for delegation results
- no deployment model for long-lived background or remote agent services

## Design Principles

- Agent coordination is a capability/runtime concern, not a prompt hack.
- Agents are specialized coordinated capabilities, not a separate unmanaged subsystem.
- Delegation must use the same provider/capability/policy surfaces as all other execution.
- Shared state should move through explicit structured resources, not transcript replay.
- Local and remote agent targets should share one framework contract.
- The runtime should prefer typed task and workflow services over generic "do anything" agents.
- Coordination must be auditable, persistable, and recoverable.
- Delegation outputs must remain provenance-bearing and trust-aware.
- Exported remote agent services should be narrow and typed, not generic shells around the primary interactive agent.

## Architectural Review Of The Codebase Changes Thus Far

The MCP and capability work already shifts the engineering baseline for this spec.

What now exists in the codebase:

- provider-backed tool, prompt, resource, session, and subscription capabilities
- inspectable-vs-callable exposure policy for provider capabilities
- provider snapshots and session snapshots with negotiated MCP metadata
- runtime-backed prompt and resource handlers, not just descriptors
- MCP server export with default-deny selectors and per-peer session tracking
- MCP advanced flows for resource subscriptions, sampling, and elicitation hooks

This means the coordination spec should now assume:

- agent-like things can be represented as provider-backed or Relurpic capabilities
- background agent services should be provider-owned
- workflow/shared-state handoff should use resource capabilities or provider-owned state
- remote agent import/export should be an MCP mapping problem layered on the coordination model
- coordination should scale by adding more capability targets and provider-owned runtime services, not by introducing a second agent-only runtime layer

This also means the spec should explicitly avoid:

- a legacy "subagent call" path that bypasses the capability registry
- ad hoc delegate instantiation with no provider/session identity
- hidden transcript copying as the long-term state transfer mechanism

## Coordination, Orchestration, And Deployment Scalability

One architectural question matters more than it first appears:

- should "agent target" remain metadata on normal capabilities, or
- should the framework introduce a separate first-class agent-only runtime type

The intended direction of this specification is:

- agent targets remain normal capabilities
- coordination metadata declares that a capability is a valid delegation target
- provider/session lifecycle handles placement, health, concurrency, and recovery

This matters because it directly affects orchestration and scalability.

If agent targets remain capability-based:

- orchestration can select targets with the same selector/policy model used elsewhere
- local and remote targets can share one admission, exposure, execution, and insertion path
- scaling deployments becomes a provider/session concern rather than a new agent-runtime concern
- horizontal growth can happen by adding more provider-owned background services or remote task services without rewriting the coordination model

If agent targets become a separate runtime type too early:

- the framework tends to duplicate registry logic
- policy and approval semantics drift from the capability model
- scheduling and deployment become agent-specific special cases
- MCP-imported and MCP-exported agent surfaces stop fitting cleanly into the same architecture

The framework may later introduce a structured coordination metadata block for
delegation targets, but it should still layer on top of capability descriptors
rather than replace them.

## Canonical Coordination Model

Coordination should be modeled as four cooperating concepts:

1. delegation target
2. delegation contract
3. delegation session/runtime owner
4. delegation result and shared resources

The target may be:

- a local agent capability
- a provider-owned agent runtime service
- a remote imported agent-like capability
- an exported agent task service consumed over MCP

The contract and lifecycle rules should be identical regardless of which target
family is chosen.

## Coordination Roles

Relurpify should support these roles:

1. Primary interactive agent
2. Local delegated agent capability
3. Provider-owned background/runtime agent
4. Remote imported delegated capability
5. Exported remote task/agent service
6. Review/verifier/planner specialist capability

These roles should differ by:

- runtime family
- provider ownership
- trust source
- policy defaults
- lifecycle and recoverability

They should not differ by hidden one-off runtime rules.

## Coordination Capability Families

Coordination should reuse existing runtime families rather than invent a new
parallel one.

Expected mapping:

- local low-level execution remains `CapabilityRuntimeFamilyLocalTool`
- local orchestrated planner/reviewer/delegator behavior becomes `CapabilityRuntimeFamilyRelurpic`
- long-lived or imported agent services become `CapabilityRuntimeFamilyProvider`

The coordination system should introduce a semantic distinction such as
"delegation target" or "agent task capability" through descriptor annotations,
category, or explicit coordination metadata rather than a separate registry.

This specification therefore prefers:

- capability-based coordination terms
- delegation target selectors
- provider/session-backed deployment rules

and explicitly moves away from:

- subagent-name allowlists as the long-term model
- ad hoc delegate wiring in individual agent implementations
- a separate agent-only registry or scheduler

## Delegation Target Descriptor Requirements

Any capability that is eligible as a delegated target should declare explicit
coordination metadata.

Minimum target metadata:

- target capability ID
- stable public name
- target role (`planner`, `reviewer`, `executor`, `verifier`, `domain-pack`, `background-agent`)
- accepted task types
- expected input contract
- expected output contract
- maximum recommended runtime/depth
- trust class and source
- whether direct insertion is ever allowed
- whether the target is synchronous, session-backed, or long-running

Suggested descriptor annotation shape:

```go
Annotations: map[string]any{
    "coordination.target": true,
    "coordination.role": "reviewer",
    "coordination.task_types": []string{"review"},
    "coordination.long_running": false,
}
```

This annotation shape is advisory metadata for coordination discovery. The real
policy still comes from the capability descriptor plus manifest policy.

Long-running delegates should not be represented merely by setting a metadata
flag on an otherwise synchronous capability. If a target is long-running, it
should be provider-owned and session-backed, with the coordination metadata
declaring that fact explicitly.

## Delegation Contract

Delegation should be structured and persistable.

Minimum contract fields:

- delegation ID
- workflow ID
- task/run ID if applicable
- caller agent ID
- caller capability or node ID
- target capability/provider/session ID
- delegation task type
- instruction
- referenced resources
- expected output contract
- recursion depth
- trust snapshot
- policy snapshot ID
- approval context
- created timestamp

Equivalent framework-owned shape:

```go
type DelegationRequest struct {
    ID                 string
    WorkflowID         string
    TaskID             string
    CallerAgentID      string
    CallerCapabilityID string
    TargetCapabilityID string
    TargetProviderID   string
    TargetSessionID    string
    TaskType           string
    Instruction        string
    ResourceRefs       []string
    ExpectedResult     *core.Schema
    Depth              int
    PolicySnapshotID   string
    ApprovalRequired   bool
    Metadata           map[string]any
}
```

The contract must be persisted independently from the raw transcript so:

- review and audit can reconstruct who delegated what
- workflows can resume or retry delegated work
- delegation can be surfaced in TUI/API inspection

## Delegation Result Model

Delegation results should be structured, provenance-bearing, and separable from
immediate caller insertion.

Minimum result fields:

- delegation ID
- target capability/provider/session identity
- completion state
- structured result payload
- generated resources or artifacts
- diagnostics
- trust and provenance metadata
- insertion decision
- timestamps

Equivalent shape:

```go
type DelegationResult struct {
    DelegationID      string
    TargetCapabilityID string
    ProviderID        string
    SessionID         string
    Success           bool
    Data              map[string]any
    ResourceRefs      []string
    Diagnostics       []string
    Insertion         core.InsertionDecision
    Metadata          map[string]any
}
```

Important rule:

- a successful delegated result is not automatically inserted into caller context

It must still pass the same insertion-policy and trust checks used for other
capability results.

## Shared State And Workflow Resources

Coordinated agents should exchange structured state via workflow-owned
resources, not by copying arbitrary conversational history.

Required shared resource classes:

- workflow facts
- issue lists
- implementation plans
- decisions and rationale
- code references and symbol references
- verification artifacts and test results
- provider/session handles
- delegated task results

These should be modeled as first-class resource capabilities or persistence
records with resource-like identities. The preferred long-term direction is:

- workflow state store owns durable records
- capability/resource layer projects them as readable resources
- delegation contracts reference them by ID/URI

This avoids transcript replay as the default state transfer mechanism.

## Context Management Model

Context management is materially affected by structured coordination.

Different agent roles should not receive the same context shape by default. For
example:

- a planner needs goals, constraints, prior decisions, and relevant summaries
- an architect needs plan state, design decisions, execution progress, and verification feedback
- a reviewer/verifier needs diffs, artifacts, policy metadata, and acceptance criteria
- a background/session-backed delegate may need private working state that should not be copied back into caller context automatically

The coordination system should therefore split context into three layers:

1. shared workflow context
2. role-specific projected execution context
3. session-private working context

Shared workflow context itself should be tiered according to:

- memory usage
- projection/retrieval latency
- persistence/storage cost
- available local or distributed compute/LLM capacity

### Shared workflow context

This is the durable, structured, cross-agent state:

- workflow facts
- plans
- decisions
- code and artifact references
- verification outputs
- delegation request/result records

This state should be persisted and addressable by resource ID/URI.

### Tiered workflow state

Shared workflow state should not be treated as one flat pool. The framework
should explicitly support at least three tiers:

1. hot state
2. warm state
3. cold state

#### Hot state

Hot state is:

- in-memory
- latency-sensitive
- scoped to the currently executing role/session
- heavily constrained by token and memory budgets

Examples:

- current delegated task contract
- current step or active sub-plan fragment
- recent role-local summaries
- explicitly pinned artifacts needed for immediate execution

Hot state optimizes for performance, not durability.

#### Warm state

Warm state is:

- structured and readily projectable
- available across agents/workers without replaying raw transcript history
- durable enough for reuse during a workflow
- the primary source for role-specific context projection

Examples:

- workflow facts
- approved decisions
- plan state
- issue lists
- code references
- verification summaries
- promoted artifacts from long-running delegates

Warm state is the default handoff layer for coordination between planner,
architect, reviewer, verifier, and similar roles.

#### Cold state

Cold state is:

- persisted for durability and recovery
- not loaded into active execution context by default
- fetched on demand by selector, query, or recovery need

Examples:

- older workflow history
- large logs
- bulky artifacts
- full transcripts
- archived delegation results
- superseded summaries retained for audit/recovery

Cold state optimizes for storage and resumability, not execution latency.

### Role-specific projected execution context

Each delegated task should receive a projection of shared workflow state that is
appropriate for that role and task contract rather than a replay of the full
conversation transcript.

Projection rules should support:

- task-type-specific resource filtering
- summarization/compression by role
- trust-aware inclusion/exclusion
- bounded token budgets
- explicit inclusion of only the resources referenced by the delegation contract
- projection from hot, warm, and cold tiers according to configured resource budgets

### Session-private working context

Long-running background delegates may maintain private session state such as:

- intermediate reasoning state
- private caches
- pending work queues
- local summaries not yet promoted to workflow resources

This private state should remain provider/session-owned. Only explicit promoted
artifacts should re-enter shared workflow context.

## Coordination Context Rules

The runtime should enforce the following:

- delegation passes resource references and a task contract, not raw transcript dumps
- each target role gets a role-specific context projection
- insertion policy applies when delegated results are projected back into caller-visible context
- background/session-backed delegates may keep private state, but published artifacts must become explicit workflow resources or delegation results
- remote delegated services should receive the minimum projected context needed for their contract

This context model improves:

- orchestration quality, because planner/architect/reviewer roles stop competing over one generic transcript
- safety, because untrusted delegated outputs remain insertion-gated
- scalability, because remote or background services do not require the full live caller transcript to contribute useful work
- token efficiency, because the framework can project compact role-specific context instead of replaying everything

## Manifest-Configurable Projection Policy

Context projection should be partially configurable through manifest-owned
policy, because different deployments have different resource constraints.

The framework must support constrained local environments now while leaving room
for later horizontal scaling across multiple agents, workers, and LLM instances.

That means projection policy should depend on:

- target role
- task type
- trust/policy rules
- available memory
- available token budget
- available persistence/storage
- available local or remote model/runtime capacity

Suggested manifest/config controls:

- per-role hot-context budget
- per-role maximum projected tokens
- preferred warm resource classes for each role
- cold-state retention and retrieval policy
- summarize-vs-fetch-vs-recompute preferences
- whether projection prefers local execution or scale-out when capacity exists
- preferred model class or instance pool for each role

The framework should not require one universal projection strategy.

## Resource-Aware Projection And Horizontal Scaling

The projection model should be designed to work in both:

- limited local environments
- horizontally scaled multi-agent or multi-LLM deployments

### Limited local environments

When resources are constrained, the framework should prefer:

- smaller hot state
- aggressive summarization into warm state
- selective retrieval from cold state
- local execution when remote/offloaded execution would cost more than it saves

### Horizontally scaled environments

When additional workers, sessions, or LLM instances are available, the framework
should be able to:

- offload long-running work to provider-owned background agent services
- project warm-state resources across workers without replaying the full caller context
- keep hot state local to the active executing session
- treat cold state as durable shared backing storage for recovery and on-demand retrieval

This lets coordination scale out without requiring:

- full transcript transfer
- shared in-memory state across all agents
- a separate orchestration model for local vs distributed execution

## Projection Selection Rules

The runtime should select context projection using a combination of:

1. delegation contract requirements
2. role-specific defaults
3. manifest-configured projection policy
4. current resource availability
5. trust and insertion constraints

This means the same workflow may project differently depending on deployment:

- a constrained laptop may use compact summaries and shallow warm-state hydration
- a larger deployment may hydrate richer warm-state resources and assign different LLM instances by role

The coordination contract remains stable even when the projection strategy
changes with available resources.

## Coordination Permission Model

Delegation should obey:

- manifest invocation permissions
- capability exposure rules
- target allowlists/selectors
- recursion depth limits
- provider/session trust restrictions
- insertion policy for returned content
- optional HITL when crossing trust boundaries or high-risk scopes

Delegation policy should explicitly separate:

- who may delegate
- which targets are eligible
- what task types are allowed
- what resources may be handed off
- whether remote targets are permitted
- whether long-running background targets are permitted
- whether the result may be inserted directly, summarized, metadata-only, or denied

## Agent Manifest Surface To Add

The existing invocation fields in
[`framework/core/agent_spec.go`](/home/lex/Public/Relurpify/framework/core/agent_spec.go)
are a starting point, but not sufficient.

The manifest surface should expand from:

- `can_invoke_subagents`
- `allowed_subagents`
- `max_depth`

to a coordination-oriented shape that can express:

- explicit delegation target selectors
- allowed runtime families for delegated targets
- remote-target policy
- background/runtime-agent policy
- resource handoff policy
- insertion policy overrides for delegated outputs
- HITL requirement for cross-trust delegation

Suggested additions:

- `delegation_targets`
- `delegation_target_selectors`
- `allow_remote_delegation`
- `allow_background_agents`
- `delegation_require_hitl_cross_trust`
- `delegation_result_insertion`
- `delegation_resource_selectors`

The old subagent fields may remain as compatibility inputs for one migration
phase, but the long-term model should be selector- and capability-based.

The specification should use delegation-target-selector language as the primary
vocabulary going forward rather than preserving "subagent" as the dominant term.

## Coordination Runtime APIs To Add

The runtime should eventually expose framework-owned operations equivalent to:

- register delegation target
- list delegation targets
- validate delegation request
- start delegation
- cancel delegation
- snapshot delegation state
- list active delegations by workflow
- emit delegation result
- materialize workflow handoff resources

These APIs should be built on:

- the capability registry
- provider/session lifecycle
- workflow persistence

They should not create a side registry for "subagents."

## Pre-Implementation Checklist

Before full Phase 5 implementation begins, the following prerequisites should
be completed or explicitly stubbed in framework-owned form.

### Required prerequisites

1. Delegation request/result primitives in `framework/core`
2. Structured coordination metadata for delegation targets
3. Selector-based manifest/config surface for coordination policy
4. Workflow resource projection layer for shared handoff state
5. Runtime delegation lifecycle APIs
6. Persistence shape for delegated work and projected artifacts

Current status:

- complete: delegation request/result primitives in `framework/core`
- complete: structured coordination metadata on capability descriptors
- complete: selector-based coordination manifest/config surface
- complete: workflow resource projection layer with `workflow://` URIs and tiered views
- complete: runtime delegation lifecycle manager and runtime wrappers
- complete: persistence for delegation records, transitions, promoted artifacts, and recovery metadata

### Checklist Details

#### 1. Delegation request/result primitives

The framework still needs first-class:

- `DelegationRequest`
- `DelegationResult`
- delegation status/state enums
- validation helpers
- provenance and insertion metadata integration

Phase 5 should not start by inventing these ad hoc inside one agent or provider.

#### 2. Structured coordination metadata

Capabilities that act as delegation targets need a framework-owned metadata
shape for:

- target role
- accepted task types
- long-running/session-backed status
- expected input/output contract
- direct-insertion policy hints

Annotations may still carry the serialized data, but the framework should define
the shape explicitly before large-scale coordination logic is added.

#### 3. Selector-based manifest/config surface

The current manifest vocabulary still centers on:

- `can_invoke_subagents`
- `allowed_subagents`
- `max_depth`

Before Phase 5, the manifest surface should move toward:

- delegation target selectors
- resource handoff selectors
- projection-policy fields
- cross-trust approval settings
- background/remote delegation controls

Compatibility mapping may exist temporarily, but implementation should target
the selector-based model directly.

#### 4. Workflow resource projection layer

The spec now assumes shared workflow state can be handed off as structured
resources with hot/warm/cold semantics. That means the framework needs:

- workflow-owned resource identity/URI rules
- projection from persistence into readable resources
- role-aware resource selection/projection rules
- promotion of delegate-produced artifacts into shared workflow resources

This is the largest functional prerequisite for clean coordination.

#### 5. Runtime delegation lifecycle APIs

The runtime already has provider/session lifecycle, but not delegation
lifecycle. Before Phase 5, it should gain framework-owned operations for:

- validating a delegation request
- starting a delegation
- listing active delegations
- cancelling delegation
- snapshotting delegation state
- linking delegations to workflow/task/provider/session IDs

#### 6. Persistence shape for delegated work

Workflow persistence should be able to store:

- delegation request records
- delegation state transitions
- delegation results
- projected or promoted shared artifacts
- recovery metadata for long-running delegated services

Without this, long-running or recoverable coordination will drift into
application-local state.

## Current Foundation

The framework now has the minimum coordination substrate needed to begin full
Phase 5 implementation:

- `framework/core` has delegation request/result/snapshot types plus structured coordination metadata
- `framework/core/agent_spec.go` exposes selector-based coordination policy and tiered projection settings
- `framework/persistence` exposes workflow projection resources and durable delegation records
- `framework/runtime` owns delegation lifecycle and persistence bridging
- `app/relurpish/runtime` exposes delegation lifecycle and persistence through the runtime surface

The remaining work is no longer prerequisite plumbing. It is the actual
coordination implementation on top of those primitives.

The current implementation baseline now also includes:

- admitted local coordination targets for planner, architect, reviewer, verifier, and executor roles
- runtime-owned delegation execution with workflow resource handoff
- provider/session-backed background delegation for long-running local work
- MCP-imported narrow remote coordination targets under explicit config and policy
- TUI/runtime inspection of delegations, transitions, promoted artifacts, linked resources, and provider/session linkage
- delegation-oriented telemetry and audit records

### Non-blocking follow-on work

The following should not block initial Phase 5 implementation:

- broader generic remote-agent export shapes
- more MCP transport variants beyond the current baseline
- full horizontal scheduler/placement logic
- full TUI/API coordination UX

These are important later, but they should build on the core delegation,
resource, manifest, runtime, and persistence primitives above.

## Local Coordination Model

Local delegation should support two families:

### Relurpic capability delegates

These are local framework-owned orchestrated capabilities such as:

- planner
- verifier
- reviewer
- code-generation helper
- summarizer

These are usually:

- synchronous or bounded
- `RuntimeFamilyRelurpic`
- admitted through the capability registry

### Provider-owned local runtime agents

These are long-lived or session-backed local services such as:

- background code index workers with agent logic
- persistent reviewer loops
- daemonized domain specialists

These should be:

- `ProviderKindAgentRuntime`
- provider-owned
- session-visible
- recoverable when promised

This distinction matters because a local planner capability and a long-lived
background verifier should not share the same lifecycle assumptions.

## Remote Coordination Model

Remote coordination should only happen through provider-backed capability
surfaces, primarily MCP.

Remote agent-like things should be imported as one of:

- tool-like delegated task capability
- typed prompt/resource/task bundle
- provider-owned workflow/task service

The coordination layer should classify them explicitly because that affects:

- default visibility
- approval requirements
- insertion policy
- retry/recovery behavior

Remote generic "agent shell" imports should be discouraged in the first
implementation. Typed review/planning/task services are safer and more auditable.

## MCP Client As Imported Coordination Surface

MCP-imported agent-like surfaces should be modeled through the same MCP client
provider pipeline defined in
[`4_mcp-core-integration-spec.md`](/home/lex/Public/Relurpify/docs/MCP-related/4_mcp-core-integration-spec.md).

The coordination-specific requirements are:

- imported remote task services must become provider-backed capabilities
- imported remote planning/review resources must remain inspectable unless promoted by policy
- imported delegation targets must carry remote trust defaults
- imported results must still pass insertion policy
- imported long-running or subscription-based targets must remain session-affined

The client side should support importing:

- remote task execution surfaces
- remote review/verifier services
- remote prompt/resource bundles that support delegation
- remote workflow coordination helpers

## MCP Server As Exported Coordination Surface

Relurpify may export narrow agent services over MCP, but not a generic
"do arbitrary work" endpoint as the default.

Initial export classes should be narrow and typed:

- plan generation
- review of provided artifacts
- verification against provided artifacts
- workflow inspection
- delegated task execution over a specific schema

Initial allowed export shapes should remain limited to these narrow task
services. Broader generic remote-agent surfaces may be revisited later, but are
out of scope for the first coordination architecture.

These exports must be:

- selector- and policy-gated
- default-deny
- auditable
- version-aware
- visible through provider/session snapshots

The MCP server provider should remain the transport and session owner; the
coordination layer defines which agent/task capabilities are eligible for export.

## Deployment Model

The coordination system should support three deployment forms:

1. in-process delegated capability
2. runtime-managed local agent service
3. remote imported or exported agent service

For each form, the runtime must know:

- owner provider
- session identity
- recoverability mode
- trust baseline
- health state
- active workflow bindings

This prevents "background agent" from becoming an untracked goroutine concept.

## Persistence And Recovery

Delegation and deployment need explicit persistence rules.

Persist:

- delegation request records
- delegation result records
- workflow handoff resources
- active runtime agent sessions when recoverable
- remote provider/session metadata for imported delegated targets

Recovery rules:

- in-process delegated capability calls are not resumed; they are retried or failed at workflow level
- provider-owned background agent sessions may be restored only if their provider recoverability mode permits it
- remote MCP-imported delegated targets must re-run admission and exposure on reconnect
- unresolved delegations from unrecoverable sessions must fail deterministically, not silently disappear

## Approval And Trust Rules

At minimum, HITL should be considered for:

- delegation from trusted local caller to remote untrusted target
- delegation that hands off sensitive resources
- delegation to long-running background runtime agents
- delegation that would insert untrusted results directly into caller context
- delegation that crosses provider/session trust boundaries

The approval prompt should include:

- caller identity
- target identity
- target provider/session
- trust class
- handed-off resource references
- requested task type
- expected insertion behavior

## Coordination Observability

Delegation must be inspectable in TUI/API and in telemetry.

Minimum observability:

- active delegation list
- completed/failed delegation history
- workflow-to-delegation linkage
- target provider/session linkage
- trust class and insertion decision
- approval/audit events

This is required so users can understand:

- what agent delegated work
- to whom
- using what resources
- with what result and trust level

## Replacement Phases

## Phase 5 Implementation Plan

The implementation should proceed in ordered slices that reuse the new
framework-owned primitives rather than adding agent-local side paths.

### Slice 1: Coordination Registry And Target Admission

- register local planner, architect, reviewer, verifier, and narrow executor targets as explicit coordinated capabilities
- remove remaining name-special-cased local delegation assumptions
- add target discovery helpers built on coordination metadata and delegation target selectors
- define which existing Relurpic capabilities are reusable coordination targets versus internal-only helpers

Acceptance:

- runtime can discover valid delegation targets through capability metadata and selector policy alone
- local coordination targets are admitted through the normal capability registry
- no new local-only subagent registry is introduced

### Slice 2: Delegation Execution And Shared Context Handoff

- implement runtime-owned delegation execution that selects a target, validates the delegation contract, and invokes the target through the registry or provider runtime
- use workflow projection resources as the default handoff mechanism instead of transcript copying
- add role-specific projection defaults for planner, architect, reviewer, and verifier
- define when delegation results are inserted directly, summarized, metadata-only, or require HITL based on trust/insertion policy

Acceptance:

- a caller can delegate work using resource references plus a typed task contract
- hot/warm/cold workflow projections are used as shared context inputs
- delegation result handling stays trust-aware and policy-gated

### Slice 3: Local Background And Session-Backed Delegates

- add provider-owned local background agent services for long-running delegates
- bind long-running delegation requests to provider/session ownership rather than in-process goroutines
- define retry, timeout, and cancellation semantics for bounded vs background delegations
- surface active background delegations and bound sessions through runtime/provider inspection

Acceptance:

- long-running delegates are provider/session-backed
- cancellation and failure semantics are explicit
- recoverable vs unrecoverable delegation behavior is testable

### Slice 4: Remote Coordination Over MCP

- classify imported MCP task services as coordination targets when metadata and policy permit
- export narrow typed coordination services over MCP server surfaces
- apply cross-trust approval, insertion, and recovery rules to remote delegation
- ensure remote coordination state remains visible through provider/session snapshots and durable delegation records

Acceptance:

- remote coordination uses the MCP/provider/capability model only
- imported and exported coordination services remain narrow and policy-bound
- disconnect/recovery behavior is explicit and tested

### Slice 5: Inspection, Audit, And UX Surfaces

- add TUI/API inspection for active delegations, history, transitions, promoted artifacts, and linked workflow resources
- add coordination-oriented telemetry and audit records
- expose workflow-to-delegation and provider/session-to-delegation linkage clearly

Acceptance:

- operators can inspect active and historical delegations without reading raw persistence rows
- audit and telemetry distinguish delegation lifecycle and trust/insertion outcomes
- the runtime no longer hides coordination state inside agent internals

Status:

- implemented in the current runtime/TUI baseline
- HTTP API parity remains partial and is expected to continue in `6_tui-api-surface-spec.md`

### Phase 1

- define coordination metadata and delegation contract
- add delegation result model
- connect manifest invocation policy to capability-based target selection

Phase 1 specifically should include:

- framework-owned delegation request/result types
- target metadata conventions for coordinated capabilities
- capability-selector-based delegation allowlist model
- unit tests for policy matching and contract validation

Phase 1 acceptance:

- the framework can describe and validate a delegation request without using ad hoc subagent state
- delegation target selection is capability-based rather than name-special-cased
- tests cover validation, selector matching, and trust-policy behavior

### Phase 2

- represent local delegates as coordinated capabilities and provider-owned runtime agents
- add workflow-shared resources for structured handoff
- remove legacy subagent-only runtime assumptions

Phase 2 specifically should include:

- local planner/reviewer/verifier targets with explicit coordination metadata
- workflow resource projection for plans, decisions, and artifacts
- runtime APIs for starting and tracking delegations
- provider-backed local background/runtime agent support where needed

Phase 2 acceptance:

- local coordination works without transcript-copy-as-contract
- workflow resources can be handed off by ID/URI
- no separate legacy subagent execution path is required
- tests cover local delegation, handoff resources, and cancellation/failure paths

### Phase 3

- add remote imported and exported coordination surfaces
- add trust-aware approval for remote delegation

Phase 3 specifically should include:

- MCP-imported remote delegation/task targets
- MCP-exported narrow agent/task services
- trust and HITL rules for remote delegation
- provider/session-visible coordination state for remote targets

Phase 3 acceptance:

- remote delegated targets use the provider/capability model
- exported remote agent services are narrow and policy-bound
- remote delegation obeys trust and insertion rules
- tests cover import/export, approvals, and disconnect/recovery behavior

### Phase 4

- complete deployment and long-running coordination support
- harden persistence, recovery, and observability

Phase 4 specifically should include:

- long-running background/runtime agent sessions
- recoverable delegation records and workflow resources
- API/TUI inspection for delegation state
- conformance and integration coverage for remote coordination services

Phase 4 acceptance:

- long-running agent services are provider/session-owned and auditable
- delegation state is persistable and inspectable
- recovery behavior is explicit and tested
- no parallel legacy coordination path remains

## Acceptance

This specification is complete when:

- agent delegation is a structured runtime concept
- local and remote coordination fit one capability/provider/security model
- workflow handoff uses explicit structured resources
- deployment of long-lived agent services is provider/session-owned
- remote coordination surfaces are clearly downstream of MCP/provider architecture
- results remain trust-aware and insertion-gated
- the runtime no longer depends on a separate legacy subagent path
