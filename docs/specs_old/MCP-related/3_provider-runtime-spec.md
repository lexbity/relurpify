# Provider And Runtime Lifecycle Engineering Specification

## Status

Implemented foundation with follow-on cleanup and hardening still ongoing

## Goal

Generalize Relurpify's runtime-managed provider pattern so long-lived services such as browser sessions, MCP clients, MCP servers, language services, and future agent deployments all share a consistent lifecycle, registration, persistence, and recovery model.

## Scope

This specification covers:

- provider contracts
- provider activation
- capability registration from providers
- session ownership
- persistence and recovery
- workflow and context integration

This specification does not define:

- detailed capability schemas
- exact security policy rules
- protocol transport internals

## Current State

Relurpify already has a minimal provider lifecycle in [`providers.go`](/home/lex/Public/Relurpify/app/relurpish/runtime/providers.go) and a concrete example in [`browser_provider.go`](/home/lex/Public/Relurpify/app/relurpish/runtime/browser_provider.go).

Current strengths:

- runtime-owned initialization
- deterministic shutdown
- provider-managed sessions
- provider-added tools as an early form of provider-added capabilities

Current limitations:

- some manifest/config surfaces are still stub-shaped and expected to evolve
- provider health/diagnostics are still lighter than the long-term target
- transport-specific hardening and resumability are still incomplete in MCP-related paths

## Design Principles

- Providers are runtime services, not merely tool factories.
- Provider-managed sessions must be runtime-owned and auditable.
- Provider activation must be explicit, not based on hard-coded skill name heuristics.
- Providers may register capabilities, resources, prompts, and sessions.
- Provider shutdown and recovery must be deterministic.
- Provider/runtime replacement should collapse ad hoc provider hooks into one framework-owned lifecycle system.
- The framework should not preserve duplicate activation paths once a provider has been moved to the common runtime lifecycle.
- Provider-added functionality should register into the capability registry,
  which is now the framework's primary registry.
- Tools should remain supported as one capability kind, not as a separate registry model.

## Provider Descriptor

Providers should be registered with framework-owned metadata, not only as Go objects.

Minimum descriptor fields:

- provider ID
- provider kind (`builtin`, `plugin`, `mcp-client`, `mcp-server`, `agent-runtime`, `lsp`)
- configured source
- activation scope
- trust baseline
- recoverability mode
- diagnostics/health support

## Provider Contract

Providers should support:

- initialization against a runtime
- capability registration
- session creation and lookup
- recovery hooks
- close/shutdown

A provider contract should also support:

- descriptor/identity lookup
- capability admission through a framework API
- session listing
- health snapshot

Optional later extensions:

- health reporting
- state snapshotting
- resumable subscriptions
- provider-specific diagnostics

The runtime should expose a provider-facing service surface rather than handing
providers unrestricted access to internals.

## Activation Model

Provider activation should be driven by workspace-owned manifests and skills.

Possible activation sources:

- agent manifest provider declarations
- skill manifest provider declarations
- runtime defaults for builtin services

The MCP client and MCP server should be modeled differently:

- MCP server: a manifest-declared runtime service with explicit provider
  configuration, because it exports Relurpify-owned services
- MCP client: a skill-driven external capability source, treated as a special
  form of external agent skill/provider import

Activation should no longer depend on name conventions such as `web-*`.

Activation resolution order should be explicit:

1. workspace/agent manifest declarations
2. skill manifest declarations
3. runtime defaults for builtins
4. disabled unless selected above

If a provider requires configuration and none is supplied, it should remain
inactive rather than silently guessing behavior.

## Session Model

Providers should manage first-class session handles.

Session requirements:

- stable runtime identifier
- provider ownership
- optional workflow/task scope
- recoverability metadata
- trust and policy context
- close semantics

Session handles should additionally expose:

- creation time
- last activity time
- health state
- recoverability state
- optional workflow/run/task binding

This model should cover:

- browser sessions
- MCP client connections
- MCP server listeners
- long-lived background agent loops

For agent-coordination use cases, long-running delegated work should be offloaded
only through provider-owned, session-backed background services. This is not
just an implementation preference; it is the mechanism that makes long-running
work auditable, recoverable where promised, and isolated from timeout
constraints imposed by interactive applications or transports.

Short-lived delegated tasks may still be executed as normal callable
capabilities, but once work is expected to exceed normal interactive timeout
budgets or requires independent lifecycle management, it should move into a
provider/session-backed runtime service.

## Persistence And Recovery

Providers should declare whether they are:

- ephemeral
- recoverable in-process
- recoverable from persisted state

The runtime should support:

- provider session snapshots
- workflow-linked restoration
- cleanup of orphaned sessions
- visibility into failed recovery attempts

The runtime should distinguish:

- provider snapshot state
- session snapshot state
- capability re-discovery state

These have different durability and failure semantics.

## Capability Registration

Provider-added capabilities must be registered through the framework-owned
capability registry.

The registration API should support:

- capability kind
- provider source
- trust metadata
- session affinity
- policy metadata

Provider-added capabilities should not be visible to agents until the registry
finishes both admission and exposure decisions.

There should not be a parallel provider-to-tool-registry path once the
replacement architecture is in place.

## Workflow And Context Integration

Provider state should integrate with:

- `core.Context`
- workflow persistence
- audit logs
- telemetry

Provider-owned state should not leak into generic context blobs without provenance metadata.

## Runtime APIs To Add

The provider/runtime rework should introduce framework-owned operations equivalent to:

- register provider descriptor
- activate provider
- deactivate provider
- list providers
- list sessions by provider
- snapshot provider/session state
- restore provider/session state
- admit provider capability
- revoke provider capability

These operations should target capability-registry state, not a separate
tool-registry subsystem.

## Manifest Surface

Provider activation should be configurable from workspace-owned manifests.

Minimum manifest needs:

- enabled/disabled state
- provider kind and target
- provider-specific config blob
- trust policy defaults
- exposure defaults
- recoverability preference

For MCP server specifically, the manifest/config surface should additionally cover:

- transports
- bind/listen settings
- exported capability/resource/prompt scopes
- auth and exposure policy
- audit/telemetry policy

For MCP client specifically, the primary configuration surface should live with
skill configuration and skill policy, with runtime provider activation derived
from that skill declaration.

## Replacement Phases

### Phase 0

Before provider-runtime work proceeds, Relurpify must finish the capability-native
runtime refactor that replaces the remaining tool-first execution spine.

This is a prerequisite, not optional cleanup.

The current framework already has capability descriptors, capability policy,
provider-backed capability registration, and capability exposure/insertion
controls. However, the dominant runtime contract is still tool-first in key
areas, which would make provider-runtime engineering converge on the wrong
abstraction if left in place.

The required refactor track is:

1. Define capability-native runtime interfaces in `framework/core`
2. Treat `CapabilityRegistry` as the canonical registry and remove any remaining tool-first assumptions
3. Represent tools as one capability kind rather than the center of the runtime model
4. Move tool execution behind a legacy adapter layer during migration
5. Rename remaining skill/manifest surfaces such as `SkillCapabilitySelector` and `phase_capabilities`
6. Migrate agent execution paths to invoke capabilities rather than raw tools
7. Remove legacy tool-first registry/execution internals once internal callers have moved

This prerequisite refactor should produce:

- one canonical registry model
- one canonical capability invocation path
- one canonical admission/exposure/execution/insertion lifecycle
- no parallel provider-to-tool-registry path
- no new framework feature added directly to `core.Tool`

### Phase 1

- define capability-native runtime interfaces and entry types
- keep `CapabilityRegistry` as the framework-owned registry
- introduce a legacy tool adapter instead of preserving tool-first registry semantics
- rename skill/runtime surfaces to capability-native names such as `SkillCapabilitySelector` and `phase_capabilities`

### Phase 2

- formalize provider activation and registration APIs on top of `CapabilityRegistry`
- remove skill-name-based activation heuristics
- define provider-owned session handle abstraction

### Phase 3

- add recoverability metadata and state snapshot contract
- integrate provider state into workflow persistence

### Phase 4

- migrate browser provider to new contracts and remove the app-scoped legacy provider path
- add MCP client/server providers

## Capability Runtime Refactor Acceptance

The provider-runtime work must not advance past the early phases unless the
capability-runtime refactor has achieved the following:

- `CapabilityRegistry` is the canonical framework registry surface
- tools are represented as `CapabilityKindTool`, not as a separate registry model
- capability invocation is framework-owned and capability-native
- internal framework execution paths do not depend on `core.Tool` as their primary abstraction
- compatibility shims, if any remain, live at the edges rather than in the registry core
- skill and manifest capability-selection surfaces are capability-native rather than tool-native

## Acceptance

This specification is complete when:

- provider activation, session ownership, and capability registration are framework-owned runtime concepts
- app-scoped or heuristic provider activation paths are no longer required
- browser, MCP client, and MCP server providers fit the same lifecycle model
- provider identity, health, and recovery are inspectable runtime concepts
- provider-added tools/resources/prompts/agent surfaces all enter the same capability registry
