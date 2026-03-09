# MCP-Related Roadmap

## Status

In progress. Phases 1 through 6 now have substantial implementation in the
codebase, and the roadmap should be read as a status-and-follow-on document
rather than a purely speculative design set.

## Goal

Adopt MCP and MCP-like capability patterns to improve Relurpify's framework model, while preserving strict security boundaries, workspace ownership, and runtime-managed execution.

This roadmap treats MCP as a forcing function for framework improvement, not merely as an external protocol integration.

## Why Multiple Specifications

This work spans several layers that should evolve independently:

- framework capability abstractions
- security and policy enforcement
- runtime provider lifecycle
- MCP protocol/session/transport support
- agent coordination and deployment
- user-facing TUI and API surfaces

Keeping these concerns in separate specifications prevents protocol details from driving core framework design too early.

## Specification Set

1. [`1_capability-model-spec.md`](/home/lex/Public/Relurpify/docs/MCP-related/1_capability-model-spec.md)
   Defines the framework-native capability model that MCP support must map onto.
2. [`2_security-policy-spec.md`](/home/lex/Public/Relurpify/docs/MCP-related/2_security-policy-spec.md)
   Defines capability trust, enforcement points, HITL, and output handling rules.
3. [`3_provider-runtime-spec.md`](/home/lex/Public/Relurpify/docs/MCP-related/3_provider-runtime-spec.md)
   Defines runtime-managed providers, session ownership, persistence, and lifecycle.
4. [`4_mcp-core-integration-spec.md`](/home/lex/Public/Relurpify/docs/MCP-related/4_mcp-core-integration-spec.md)
   Defines protocol ownership, fork boundaries, session/version handling, and conformance.
5. [`5_agent-coordination-spec.md`](/home/lex/Public/Relurpify/docs/MCP-related/5_agent-coordination-spec.md)
   Defines agent delegation, deployment, shared state, and MCP client/server agent surfaces.
6. [`6_tui-api-surface-spec.md`](/home/lex/Public/Relurpify/docs/MCP-related/6_tui-api-surface-spec.md)
   Defines how new capabilities, sessions, prompts, resources, and approvals are surfaced to users.

## Topic Order

The specifications are intentionally ordered by implementation dependency, not by
user-visible feature priority:

1. Capability model
2. Security and policy
3. Provider/runtime lifecycle
4. MCP core integration
5. Agent coordination and deployment
6. TUI/API surface

This is also the recommended reading order for engineering work.

## Expected Output From Each Spec

Each spec should produce an implementation-facing artifact, not only prose:

- capability model: core types, replacement boundaries, and registry responsibilities
- security/policy: admission, exposure, invocation, and insertion rules
- provider/runtime lifecycle: runtime/provider/session contracts and activation rules
- MCP core integration: package boundaries, fork boundary, supported feature baseline, and conformance plan
- agent coordination: delegation contract, shared-resource model, and trust rules
- TUI/API surface: minimum inspectability and operability endpoints

## Design Principles

- Relurpify remains framework-first; MCP is subordinate to Relurpify's abstractions and security rules.
- Security enforcement must happen below model-visible capability wrappers.
- Workspace-owned manifests and skills remain authoritative.
- Forked MCP code may supply protocol/session mechanics, but public framework contracts remain Relurpify-owned.
- Version-aware protocol handling must not force version-specific framework internals.
- Significant restructuring is acceptable when it simplifies the architecture.
- Prefer hard replacement over long-lived compatibility layers or duplicate code paths.
- Legacy framework contracts should be removed rather than carried forward behind adapters once replacement work starts.

## Sequencing

### Phase 1: Capability Foundation

Write and approve:

- capability model
- security and policy model
- provider/runtime lifecycle

Reason:

- these define the stable internal boundary
- MCP protocol work should not start until the framework shape is explicit
- agent export/import should not start until provider and security contracts exist

### Phase 2: MCP Core Baseline

Write and approve:

- MCP core integration

Work implied:

- decide fork boundary from `MCP-take-apart`
- define internal package layout
- define protocol-version handling strategy
- define conformance/test baseline
- define the minimum stable MCP feature baseline for initial implementation

### Phase 3: Agent and Deployment Expansion

Write and approve:

- agent coordination and deployment

Work implied:

- unify local and remote agent invocation
- define MCP client as a provider/skill surface
- define MCP server as a capability export surface
- define workflow-resource handoff contracts

### Phase 4: User Surface

Write and approve:

- TUI/API surface

Work implied:

- capability inspection
- structured result rendering
- session/provider management
- richer approval and audit views
- minimal external orchestration endpoints

## Immediate Implementation Order

1. Capability model
2. Security and policy model
3. Provider/runtime lifecycle
4. MCP core integration
5. Agent coordination and deployment
6. TUI/API surface

## Phase Gates

The implementation should treat these as hard gates:

- Do not import remote MCP capabilities into model-visible surfaces before capability admission and insertion policy exist.
- Do not export Relurpify workflow or task execution over MCP before provider/session ownership and audit are defined.
- Do not build user-facing capability management surfaces before capability identity and trust metadata are stable.

## Non-Goals

This roadmap does not require:

- protocol-complete MCP support in the first implementation
- preserving `go-sdk` public APIs
- adopting `go-sdk` as a runtime dependency
- exposing every MCP feature immediately
- finalizing all remote agent deployment semantics before `Tool v2`
- preserving a legacy generic tool registry or any duplicate invocation path once the capability-native architecture is implemented

## Acceptance

This roadmap is complete when:

- each linked specification exists and is internally coherent
- the sequence between specifications is explicit
- protocol work is clearly downstream of framework/security decisions
