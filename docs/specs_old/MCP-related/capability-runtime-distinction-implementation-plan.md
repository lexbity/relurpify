# Capability Runtime Distinction Implementation Plan

## Status

In progress with Phase 5 coordination slices implemented through inspection/audit surfaces

## Goal

Implement the framework-level distinction between:

- `Tool` as a local-native capability executed inside Relurpify's own runtime and sandbox boundary
- `ProviderCapability` as a provider-backed capability that may be local, remote, or session-backed
- `RelurpicCapability` as a framework-native orchestrated capability implemented through Relurpify workflows, patterns, or sub-agent execution

This plan is downstream of the updated capability-model specification in
[`1_capability-model-spec.md`](/home/lex/Public/Relurpify/docs/MCP-related/1_capability-model-spec.md).

## Principles

- `Capability` remains the umbrella abstraction.
- `callable` is a trait, not a synonym for `tool`.
- skills remain composition/guidance/policy surfaces, not runtime capability types.
- capability policy continues to operate at the capability level, not the tool level.
- gVisor guarantees should be associated with local-native tools and local provider subprocesses, not assumed for all callable capabilities.

## Target End State

The framework should expose one capability registry and one policy pipeline, but
distinguish callable runtime families explicitly:

1. `LocalToolCapability`
2. `ProviderCapability`
3. `RelurpicCapability`

All three may be callable, but they should not share the same runtime identity.

## Phase 1: Core Type Split

- narrow the meaning of `Tool` in core/runtime code to local-native integrations only
- define first-class runtime family markers or descriptors for:
  - local tool capability
  - provider capability
  - Relurpic capability
- stop using tool-specific names for generic invocation interfaces
- introduce a capability result type name that is not tool-specific

Acceptance:

- framework code no longer uses `tool` as the generic term for any callable capability
- local-native vs provider vs Relurpic runtime family can be identified from framework metadata

## Phase 2: Registry And Invocation Refactor

- make capability handlers the only canonical invocation contract
- remove remaining assumptions that callable capabilities are tools
- keep local tool wrappers only for actual local-native tools
- allow provider and Relurpic callable capabilities to register without passing through tool-shaped execution paths

Acceptance:

- registry invocation is capability-native
- provider and Relurpic capabilities are not stored or exposed as tool wrappers
- local tools still remain discoverable as a specific callable family

## Phase 3: Result And Telemetry Cleanup

- replace tool-specific result naming in execution, telemetry, and persistence paths
- emit runtime-family metadata in audit and telemetry events
- distinguish local-native execution, provider execution, and Relurpic orchestration in event records

Acceptance:

- execution telemetry does not imply every callable capability is a tool
- audit records can distinguish local tool calls from provider and Relurpic invocations

## Phase 4: Policy And Exposure Refinement

- keep policy selectors capability-based
- add optional policy selectors or metadata filters for runtime family
- ensure tool-specific approval behavior only applies to local-native tools
- ensure provider-backed and Relurpic capabilities can have distinct defaults without creating parallel policy systems

Acceptance:

- policy remains unified at capability level
- runtime-family-specific policy behavior exists without reintroducing separate registries

## Phase 5: Skill And Agent Coordination Alignment

- ensure skills only select/configure capabilities and never act as runtime capability types
- model local sub-agent/workflow-backed callable units as `RelurpicCapability`
- make planner/reviewer/coder/delegation-style callable units explicit Relurpic capabilities where reusable

Acceptance:

- reusable local orchestrated features are modeled as capabilities, not skills
- skill manifests remain composition surfaces only

### Pre-Phase-5 Prerequisites

Before Phase 5 implementation starts in earnest, the following coordination
primitives should be landed or stubbed in framework-owned form:

1. delegation request/result types in `framework/core`
2. structured coordination metadata for delegation targets
3. selector-based coordination manifest/config surface
4. workflow resource projection layer
5. runtime delegation lifecycle APIs
6. persistence support for delegated work and promoted artifacts

Phase 5 should build on those primitives rather than introducing
agent-coordination-specific side paths inside agents or applications.

### Suggested Pre-Phase-5 Execution Order

#### Step 1: Core Delegation Types

- add `DelegationRequest`, `DelegationResult`, and delegation status/state types
- add validation and provenance/insertion integration
- keep the shapes framework-owned and transport-agnostic

Acceptance:

- delegated work can be described and persisted without using app-local structs

Status:

- implemented

#### Step 2: Coordination Metadata

- define structured metadata for delegation targets
- map planner/reviewer/verifier/background-agent roles into that shape
- keep targets capability-based rather than introducing a separate agent-only registry

Acceptance:

- coordination targets can be discovered and filtered by structured metadata

Status:

- implemented

#### Step 3: Manifest And Projection Policy Surface

- add delegation-target-selector language
- add projection-policy fields for hot/warm/cold context handling
- add remote/background/cross-trust delegation controls
- keep old subagent terms only as temporary compatibility inputs if needed

Acceptance:

- coordination policy is selector-based and resource-aware

Status:

- implemented

#### Step 4: Workflow Resource Projection Layer

- define workflow resource IDs/URIs
- expose handoff artifacts as structured resources
- support projection across hot/warm/cold tiers
- enable role-specific resource projection for planner/architect/reviewer/etc.

Acceptance:

- delegated work can exchange structured workflow state without transcript replay

Status:

- implemented

#### Step 5: Runtime Delegation Lifecycle

- add start/cancel/list/snapshot delegation operations
- link delegations to workflow/task/provider/session IDs
- support local bounded delegations and long-running background/service-backed delegations

Acceptance:

- delegation has a runtime-owned lifecycle rather than an agent-local call chain

Status:

- implemented

#### Step 6: Persistence And Recovery Integration

- persist delegation records and promoted artifacts
- support recovery metadata for long-running provider/session-backed delegated services
- make failure and retry semantics explicit

Acceptance:

- delegated work is inspectable, auditable, and recoverable where promised

Status:

- implemented

### Next Execution Sequence For Phase 5

With Steps 1-6 complete, the next implementation work should shift from
framework prerequisites to actual coordination behavior:

1. register and admit local coordination targets through the capability registry
2. implement delegation execution and workflow-resource handoff
3. add provider/session-backed local background delegates
4. add narrow MCP-imported and MCP-exported coordination surfaces
5. add inspection, audit, and telemetry surfaces for delegation state

This sequence is now the active plan for `5_agent-coordination-spec.md`.

Current status:

- complete: register and admit local coordination targets through the capability registry
- complete: delegation execution and workflow-resource handoff
- complete: provider/session-backed local background delegates
- complete: narrow MCP-imported coordination surfaces plus policy-bound remote delegation
- complete: inspection, audit, and telemetry surfaces for delegation state

Remaining coordination work should now be tracked primarily under
[`5_agent-coordination-spec.md`](/home/lex/Public/Relurpify/docs/MCP-related/5_agent-coordination-spec.md)
and the TUI/API expansion work in
[`6_tui-api-surface-spec.md`](/home/lex/Public/Relurpify/docs/MCP-related/6_tui-api-surface-spec.md).

## Phase 6: Tool Runtime Narrowing

- move remaining local-native implementations into the narrowed `Tool` family
- remove legacy uses of `Tool` for generic callable behavior
- document the gVisor/runtime guarantees that apply specifically to local-native tools

Acceptance:

- `Tool` means local-native capability implementation everywhere in framework-facing terminology
- no provider-backed or Relurpic capability is described as a tool merely because it is callable

## Phase 7: UI And External Surface Alignment

- present runtime family in TUI/API inspection
- distinguish callable local tools from provider and Relurpic capabilities in user-facing inspection
- preserve one capability-oriented inspection model while exposing runtime-family differences clearly

Acceptance:

- operators can tell whether a callable capability is a local tool, provider capability, or Relurpic capability

## Suggested Implementation Order

1. Core type split
2. Registry/invocation refactor
3. Result/telemetry cleanup
4. Policy refinement
5. Skill and agent coordination alignment
6. Tool runtime narrowing
7. UI/API alignment

## Immediate Candidate Areas In Code

- [`framework/core/capability_runtime.go`](/home/lex/Public/Relurpify/framework/core/capability_runtime.go)
- [`framework/core/capability_types.go`](/home/lex/Public/Relurpify/framework/core/capability_types.go)
- [`framework/capability/capability_registry.go`](/home/lex/Public/Relurpify/framework/capability/capability_registry.go)
- [`agents/skills.go`](/home/lex/Public/Relurpify/agents/skills.go)
- [`agents/pattern/react.go`](/home/lex/Public/Relurpify/agents/pattern/react.go)
- [`app/relurpish/runtime`](/home/lex/Public/Relurpify/app/relurpish/runtime)

## Acceptance

This plan is complete when the framework can truthfully say:

- `Tool` means local-native capability implementation
- provider-backed callable capabilities are not tools
- Relurpify-native orchestrated callable capabilities are not tools
- skills are not runtime capability types
- the framework still uses one capability registry and one capability policy pipeline
