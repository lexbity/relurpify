# MCP Architecture

## Purpose

Relurpify uses MCP as a protocol integration and interoperability layer, not as
its core framework model. The framework remains capability-first, policy-first,
and runtime-managed. MCP client and server support are built on top of
Relurpify-owned abstractions for capabilities, providers, sessions, policy, and
inspection.

This document consolidates the MCP-related architecture and feature model from
the earlier engineering specs. It focuses on the durable system shape rather
than implementation sequencing.

## Core Principles

- Relurpify remains framework-first; MCP is subordinate to Relurpify's
  capability and policy model.
- Framework abstractions are Relurpify-owned, even when protocol mechanics are
  derived from or inspired by external MCP SDKs.
- Security enforcement happens below model-visible capability wrappers and
  applies equally to local and remote capabilities.
- Provider-managed sessions are runtime-owned, auditable, and recoverable where
  supported.
- Imported MCP capabilities are not callable by default; admission, exposure,
  invocation, and context insertion are separate decisions.
- MCP server export is projection-based and default-deny.
- The TUI and HTTP API should expose the same runtime model that execution and
  policy use, rather than inventing separate presentation-only structures.

## Capability Model

Relurpify's MCP support depends on a framework-native capability model that can
represent more than local tools.

Every capability has a Relurpify-owned identity with at least:

- capability ID
- kind
- stable public name
- version
- source provider ID
- source scope
- optional session ID

Primary capability kinds are:

- tool
- prompt
- resource
- session-backed capability
- subscription or watch capability

Relurpify also distinguishes runtime family from capability kind. A callable
capability is not necessarily a tool. The important runtime families are:

- local-native tools
- provider-backed capabilities
- Relurpify-native orchestrated capabilities

This distinction matters because trust, latency, sandboxing, session behavior,
audit handling, and approval expectations differ by runtime family.

Each capability descriptor carries the metadata needed for registry filtering,
policy evaluation, audit, TUI inspection, and MCP mapping. At minimum that
includes identity, kind, source, trust class, risk classes, schemas,
availability, and annotations.

## Security Model

MCP support must not bypass Relurpify's security model. Capability discovery
does not imply capability exposure, and exposure does not imply execution or
context insertion.

### Trust and risk

Capabilities are classified by both source trust and operational risk.

Trust classes include:

- builtin trusted
- workspace-owned trusted
- provider-managed local untrusted
- remote declared untrusted
- remote approved or trusted by policy

Risk classes include:

- read-only
- destructive
- execute
- network
- credentialed
- exfiltration-sensitive
- sessioned

Capabilities also declare effect classes such as filesystem mutation, process
spawn, network egress, credential use, external state change, long-lived
session creation, and model-context insertion.

### Enforcement points

Security decisions occur at five separate gates:

1. capability registration
2. capability exposure to the model
3. capability execution
4. result insertion into context
5. provider and session lifecycle operations

Remote MCP capabilities are untrusted by default. Imported protocol metadata is
advisory until Relurpify normalizes it into framework-owned descriptors and
policy metadata. Remote code never executes directly through imported protocol
logic; all calls must route through Relurpify invocation paths.

Output inserted into model-visible context must retain provenance, trust, and
transformation metadata so untrusted content cannot become trusted-looking
solely through summarization or reformatting.

## Provider and Runtime Architecture

MCP clients, MCP servers, browser sessions, language services, and future
background runtimes all fit into one provider lifecycle model.

Providers are runtime services, not just capability factories. A provider has a
descriptor with:

- provider ID
- provider kind
- configured source
- activation scope
- trust baseline
- recoverability mode
- diagnostics and health support

Providers are responsible for:

- initialization against the runtime
- capability registration through framework APIs
- session creation and lookup
- recovery hooks
- shutdown
- health snapshots

Activation is explicit and driven by workspace-owned manifests, skill
configuration, or runtime defaults for builtins. If a provider requires config
and none is supplied, it stays inactive.

### Session model

Provider-managed sessions are first-class runtime objects with:

- stable runtime ID
- provider ownership
- optional workflow or task scope
- trust and policy context
- creation and last-activity timestamps
- health and recoverability state
- close semantics

This same session model covers MCP client connections, MCP server peers,
browser sessions, and long-running delegated background services.

### Persistence and recovery

Providers declare whether they are ephemeral, recoverable in process, or
recoverable from persisted state. The runtime manages provider snapshots,
session snapshots, restoration, and orphan cleanup as separate concerns from
capability discovery.

## MCP Core Integration

Relurpify supports MCP through a Relurpify-owned integration boundary. Forked
or imported MCP protocol mechanics may supply message types, session plumbing,
transport handling, content models, and conformance fixtures, but public
framework contracts remain Relurpify-owned.

Suggested internal package layout:

```text
framework/mcp/
├── protocol/
├── versioning/
├── transport/
├── session/
├── content/
├── schema/
├── client/
├── server/
├── mapping/
└── conformance/
```

The important architectural rule is that protocol mechanics stay inside
`framework/mcp/*`, while capability normalization, security decisions, and
runtime exposure remain outside that protocol layer.

### MCP session lifecycle

Each live MCP connection has a framework-owned session record with at least:

- provider ID
- runtime session ID
- transport kind
- endpoint or process identity
- negotiated protocol version
- session state
- peer metadata
- advertised and discovered capabilities
- active request count
- recoverability metadata

The runtime tracks session state transitions such as connecting, initializing,
initialized, degraded, closing, closed, and failed. Capability synchronization
only occurs after initialization completes.

## MCP Client Model

The MCP client is a runtime provider that:

- negotiates protocol version
- discovers remote features
- normalizes them into framework capabilities
- registers them through the capability registry
- maintains session state under runtime ownership

Imported MCP concepts map into Relurpify capability kinds:

- MCP tool -> tool capability, provider-backed runtime family
- MCP prompt -> prompt capability, provider-backed runtime family
- MCP resource -> resource capability, provider-backed runtime family
- MCP update or subscription flows -> subscription capability where modeled
- MCP session metadata -> session capability

Imported identities are stable and framework-owned. They should be derived from
provider identity, remote feature type, and remote name or URI rather than
relying on transient display labels.

Catalog synchronization supports initial listing, incremental refresh, removal
handling, and revocation of no-longer-present capabilities. Newly imported
capabilities remain inspectable by default and only become callable if explicit
manifest or policy decisions allow it.

## MCP Server Model

The MCP server is a runtime-managed provider or service that exports selected
Relurpify capabilities, prompts, and resources through MCP.

Server export is projection-based, not a raw mirror of internal registries.
Only explicitly selected objects are exportable, and export remains default-deny.

Export policy supports:

- allowlists and denylists by capability selector
- runtime-family-aware export control
- separate prompt and resource selectors
- per-export auth and visibility defaults

For every exported action:

- schemas derive from framework-owned schemas
- execution still passes through Relurpify policy
- results and errors are normalized through framework-owned content handling

For servers, session state is tracked per connected peer rather than only per
listener process.

## Canonical APIs and MCP Adapters

When Relurpify exposes administrative or application-specific services over
MCP, MCP should be treated as an adapter over a canonical typed service
interface, not the source of truth for the API itself.

The canonical layer should define:

- typed request and response structs
- API versioning
- typed error taxonomy
- pagination contracts
- runtime-injected auth and tenant context

Stable MCP tool names then map onto versioned canonical operations. This keeps
local callers, stdio MCP, HTTP MCP, and future transports aligned without
making MCP tool names the primary contract.

## TUI and HTTP API Surface

The user-facing surface for MCP-backed behavior should expose ordinary runtime
state, not a separate MCP admin model.

The main inspectable objects are:

- capabilities
- providers
- sessions
- workflows and delegations
- prompts and resources
- approvals and HITL records

Capability inspection should show:

- identity and kind
- runtime family
- provider and session source
- trust class
- exposure mode
- risk classes
- schema summary
- availability
- session affinity and coordination metadata where relevant

Provider and session inspection should show:

- active providers and sessions
- health and last error state
- configured versus active state
- age and scope
- recoverability state
- negotiated MCP metadata where relevant

Structured result rendering should preserve typed content, provenance, linked
resources, structured errors, and insertion disposition instead of flattening
everything to logs.

Approval and HITL surfaces should use one unified approval model with typed
kinds such as:

- execution
- insertion
- admission
- provider operation

The HTTP API should expose capability, provider, session, workflow, prompt,
resource, and approval inspection without introducing unrestricted execution
paths or a separate MCP-specific inspection protocol.

## Summary

MCP in Relurpify is built on four stable architectural ideas:

- a capability-native framework model
- a policy-first security pipeline
- a runtime-owned provider and session lifecycle
- a projection-based import and export boundary

That combination allows Relurpify to support MCP clients and servers without
letting protocol details define framework contracts, security behavior, or user
inspection surfaces.
