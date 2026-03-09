# MCP Core Integration Engineering Specification

## Status

Implemented foundation with remaining transport hardening and follow-on cleanup

## Goal

Introduce first-class MCP client and server support into Relurpify using a Relurpify-owned architecture and a forked protocol baseline, without adopting the external `go-sdk` as a dependency or allowing MCP implementation details to define framework boundaries.

## Scope

This specification covers:

- fork boundaries from `MCP-take-apart`
- internal package layout
- protocol/session/transport support
- client and server roles
- protocol-version handling
- conformance strategy

This specification does not define:

- the primary framework capability model
- framework security policy semantics
- TUI rendering behavior

## Current State

Relurpify now has a real MCP subsystem foundation, built on top of earlier framework/runtime work:

- runtime-managed providers
- capability registry and policy resolution
- workflow persistence
- context and telemetry systems
- an HTTP API surface

The codebase already includes:

- protocol and revision boundary code under [`framework/mcp/`](/home/lex/Public/Relurpify/framework/mcp)
- runtime-managed MCP client and server providers in
  [`app/relurpish/runtime/mcp_provider.go`](/home/lex/Public/Relurpify/app/relurpish/runtime/mcp_provider.go)
- provider-backed import of remote tools, prompts, and resources
- selector-driven, default-deny MCP export
- stdio and basic HTTP lifecycle support
- session tracking, capability mapping, advanced feature handling, and tests

Remaining work is now about hardening, resumability, and follow-on refinement,
not about creating the first real MCP implementation.

The reviewed fork candidate under [`MCP-take-apart/go-sdk-main`](/home/lex/Public/Relurpify/MCP-take-apart/go-sdk-main) provides:

- protocol data types
- session and transport mechanics
- streamable HTTP and stdio support
- resources, prompts, and tool content models
- protocol version handling

## Design Principles

- MCP protocol mechanics may be forked; framework contracts remain Relurpify-owned.
- Relurpify must not expose imported SDK public APIs as stable framework APIs.
- Protocol-version awareness must be explicit at the boundary.
- Client and server support should both use provider/runtime lifecycle patterns.
- MCP client import must remain constrained by Relurpify security and exposure policy before anything becomes callable.
- MCP server export must remain constrained by Relurpify security and selector-driven default-deny projection.
- Conformance should be treated as a first-class engineering requirement.
- The roadmap should plan toward full MCP feature support, using the reviewed
  Go SDK as a protocol/reference source, while still sequencing delivery in
  stable phases.
- MCP integration should plug into one capability model and one provider/runtime model, not introduce duplicate MCP-specific execution paths.

## Fork Boundary

### Candidate for fork

- protocol type definitions
- JSON-RPC/session plumbing
- stdio and streamable transport mechanics
- content block handling
- schema caching and schema helper logic
- conformance fixtures and reference servers/clients

### Do not adopt directly

- top-level SDK client/server public APIs
- SDK capability inference/defaulting
- SDK authorization model as framework security
- raw SDK tool registration model

These areas should be re-expressed in Relurpify-native APIs.

The reviewed fork's own documented rough edges reinforce this boundary,
especially around default capability inference and content-shape/API choices.

## Proposed Internal Package Layout

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

### Responsibilities

- `protocol/`: MCP message and feature types
- `versioning/`: negotiated revision handling and compatibility
- `transport/`: stdio, streamable HTTP, and future transport implementations
- `session/`: session state and lifecycle
- `content/`: content block encoding/decoding
- `schema/`: framework-owned schema mapping to/from MCP
- `client/`: Relurpify-owned MCP client facade
- `server/`: Relurpify-owned MCP server facade
- `mapping/`: mapping between framework capabilities and MCP concepts
- `conformance/`: tests and fixtures

Package ownership should be strict:

- `framework/mcp/*` owns protocol and transport mechanics
- capability normalization lives at the mapping boundary
- security and exposure decisions remain outside the imported/forked protocol layer

Suggested internal subpackages and responsibilities:

- `framework/mcp/protocol`: forked wire structs, protocol constants, request/response envelopes, and error codes
- `framework/mcp/versioning`: supported revision matrix, feature gates, and downgrade helpers
- `framework/mcp/transport/stdio`: stdio subprocess transport, sandbox integration, and process shutdown rules
- `framework/mcp/transport/http`: streamable HTTP client/server transport, session binding, and resumability hooks
- `framework/mcp/session`: session state machine, request tracking, cancellation, and heartbeat/health logic
- `framework/mcp/content`: MCP content <-> framework `ContentBlock` conversion
- `framework/mcp/schema`: MCP JSON Schema adaptation to framework-owned `core.Schema`
- `framework/mcp/client`: Relurpify-owned client facade used by MCP client providers
- `framework/mcp/server`: Relurpify-owned server facade used by MCP server runtime services
- `framework/mcp/mapping`: capability/resource/prompt/session/subscription mapping between MCP and framework descriptors
- `framework/mcp/conformance`: imported fixtures plus Relurpify-native conformance harnesses

The following must not leak out of `framework/mcp/*` as public framework
contracts:

- forked SDK client/server types
- forked transport option structs
- forked capability inference behavior
- forked schema cache abstractions unless re-expressed in framework-owned terms

## MCP Session Model

MCP needs an explicit runtime/session model rather than being treated as an
opaque provider connection.

Each live MCP connection should have a framework-owned session record with at
least:

- provider ID
- runtime session ID
- transport kind
- remote endpoint or process identity
- negotiated protocol version
- state (`connecting`, `initializing`, `initialized`, `degraded`, `closing`, `closed`, `failed`)
- remote peer metadata (`serverInfo` / `clientInfo` equivalent)
- advertised local capabilities
- discovered remote capabilities
- active request count
- recoverability metadata

Required state transitions:

1. provider activation allocates a runtime session record
2. transport is established
3. initialization handshake executes
4. negotiated version and peer capabilities are persisted
5. registry synchronization runs only after session reaches `initialized`
6. transport failure moves the session to `degraded` or `failed`
7. shutdown revokes session-affined capabilities before transport teardown completes

For MCP servers, session state must be per connected peer, not just per
listener. A single MCP server provider may therefore own:

- one listener/service record
- many peer sessions beneath it

## Protocol Lifecycle Requirements

The implementation must codify MCP lifecycle behavior explicitly instead of
relying on implicit SDK semantics.

Client-side minimum requirements:

- send initialize request with Relurpify-owned client metadata
- capture peer protocol version and server capabilities from initialize result
- send initialized notification only after local registration is ready
- reject capability import until handshake completes
- propagate cancellation to the transport/session layer

Server-side minimum requirements:

- reject peer requests before initialization is complete where the protocol requires it
- track server session readiness independently for each peer
- expose initialized hooks so framework export synchronization can happen after handshake
- preserve request ordering guarantees required by the chosen transport implementation

Shutdown requirements:

- close transport idempotently
- retire in-flight requests with MCP-shaped cancellation semantics
- revoke session-affined capabilities before provider/session cleanup completes
- record terminal health and failure reason in provider/session snapshots

## Client Model

The MCP client should be implemented as a runtime provider that:

- negotiates protocol version
- discovers remote capabilities
- normalizes them into framework capabilities
- registers them through the capability registry
- maintains session state under runtime ownership

The import path should be:

1. transport/session handshake
2. negotiated version capture
3. remote feature discovery
4. normalization into framework capability descriptors
5. capability admission/exposure policy
6. model/API visibility

The MCP client should be treated as a special class of external skill/provider
integration, not as the same configuration concern as server export.

Imported MCP capabilities should be `inspectable` by default. They should not
become `callable` unless Relurpify-owned manifest/policy decisions explicitly
promote them.

### Client Capability Import Rules

Imported MCP features should map into framework descriptors as follows:

- MCP tool -> `CapabilityKindTool`, `RuntimeFamilyProvider`
- MCP prompt -> `CapabilityKindPrompt`, `RuntimeFamilyProvider`
- MCP resource -> `CapabilityKindResource`, `RuntimeFamilyProvider`
- MCP resource subscription/update flow -> `CapabilityKindSubscription` where modeled explicitly
- MCP session metadata -> `CapabilityKindSession`

Imported capability identity must be stable and framework-owned. The spec should
define a deterministic ID shape such as:

- `mcp:<provider-id>:tool:<remote-name>`
- `mcp:<provider-id>:prompt:<remote-name>`
- `mcp:<provider-id>:resource:<normalized-uri-or-template>`
- `mcp:<provider-id>:session:<session-id>`

Required import metadata:

- remote MCP name/URI/template
- negotiated protocol version
- provider ID and session ID
- source scope
- trust baseline
- remote annotations that remain advisory until admitted by framework policy

Catalog synchronization rules:

- initial import must support paginated list flows where MCP requires them
- list-changed notifications must trigger an incremental re-sync
- removed remote features must revoke the corresponding framework capabilities
- sync failures must not leave partially updated registry state visible as callable
- newly imported remote capabilities must remain inspectable-by-default unless manifest/policy explicitly exposes them as callable

Imported callable capabilities must invoke through a provider-backed execution
path, never through legacy local-tool wrappers.

## Server Model

The MCP server should be implemented as a runtime provider or service that:

- exports framework capabilities
- exposes prompts and resources from framework-owned registries
- surfaces workflow and context resources where policy allows
- enforces Relurpify policy before every exported action

The export path should be:

1. choose exportable framework capabilities/resources/prompts
2. map to MCP protocol features
3. enforce policy per request
4. normalize results and errors back through framework-owned content handling

The MCP server should be configured through an explicit manifest-owned service
definition, consistent with existing Relurpify configuration patterns for runtime
services and exported behavior.

### Server Export Rules

Framework export must be projection-based, not registry mirroring.

Only capabilities/resources/prompts explicitly selected by manifest and policy
should be exportable. Export projection must support:

- export allowlist by capability selector
- export denylist by capability selector
- runtime-family filters so local-native tools, provider-backed capabilities, and Relurpic capabilities can be exported differently
- prompt/resource export selectors independent of callable capability selectors
- per-export auth and visibility defaults

Export should be default-deny. Nothing is exported unless a manifest-owned
selector or equivalent explicit config includes it.

Required export behavior:

- exported tool schemas must be derived from framework-owned schemas, not raw internal structs
- output must be normalized from `CapabilityResultEnvelope` into MCP content/result structures
- framework policy must run before executing any exported action
- export handlers must preserve provenance and trust metadata for audit, even when the remote peer only sees MCP-shaped output
- list-changed notifications must be emitted when the export catalog changes for a connected peer

The server provider must be able to export at least:

- callable capabilities as MCP tools
- prompt capabilities as MCP prompts
- readable resource capabilities or registries as MCP resources
- selected workflow/context resources where policy allows

For agent-coordination-related exports, initial allowed shapes should be narrow
task services only. The first implementation should support exporting typed
planning, review, verification, workflow-inspection, or similarly constrained
task services rather than a generic remote "agent shell."

This keeps exported coordination surfaces:

- selector-friendly
- auditable
- easier to permission
- easier to version and recover

Broader generic delegated-agent exports may be revisited later, but they are
not part of the initial MCP server coordination surface.

The server provider must not expose internal-only framework artifacts by default:

- raw policy snapshots
- hidden capabilities
- synthetic provider health/state objects unless explicitly mapped as resources

The initial manifest/config surface for server export may be a stub, but this
spec should record that the surface is expected to be re-engineered after the
first end-to-end implementation validates the final selector and policy shape.

## Protocol Versioning

Relurpify should treat protocol revision as negotiated session metadata.

Required rules:

- every MCP session stores negotiated protocol version
- transport and feature behavior may branch on protocol revision
- framework capability contracts remain protocol-version agnostic where possible
- downgrade behavior is explicit

Relurpify should keep an internal support matrix such as:

- framework capability contract version
- supported MCP protocol revisions
- enabled transports by revision
- feature flags by revision

The support matrix should be a concrete source-controlled table, not prose. For
each supported revision, define:

- handshake version string
- enabled feature families
- content shape quirks or compatibility branches
- transport restrictions
- deprecated or unsupported methods
- test fixture coverage status

Initial implementation target:

- one primary MCP revision is implemented end-to-end first
- unsupported revisions should fail explicitly and diagnostically
- boundary design must still assume future multi-revision support as a separate concern
- the framework capability/provider model must remain stable when additional revisions are added

Version branching should happen only at:

- protocol encoding/decoding
- feature availability checks
- request/response normalization

Version branching should not leak into the generic framework capability model.

## Transport Support

Initial support:

- stdio
- streamable HTTP

Deferred:

- legacy SSE only if required by protocol support goals
- additional custom transports
- resumability/redelivery beyond the basic streamable HTTP lifecycle

### Stdio Transport Requirements

- subprocess launch must integrate with existing runtime sandbox/command runner infrastructure
- process identity, argv, cwd, and environment must be auditable
- stderr handling must be defined explicitly
- dead process detection and restart policy must be explicit
- transport close must terminate child processes deterministically

### Streamable HTTP Requirements

- endpoint, auth, headers, and timeout config must be manifest-owned
- session binding must distinguish listener lifecycle from peer-session lifecycle
- resumability/redelivery support must be optional and tied to persistence capability
- stateless mode, if supported at all, must be explicitly marked as degraded functionality because it limits server-initiated client calls

Initial streamable HTTP delivery should cover only the basic lifecycle:

- connect
- initialize
- request/response handling
- shutdown

Resumability/redelivery should be added only after stdio and basic streamable
HTTP lifecycle behavior is stable and well-tested.

### Shared Transport Requirements

- tracing/logging wrapper support
- cancellation propagation
- bounded read/write buffering
- backpressure handling
- redaction-aware diagnostics

## Manifest And Config Surface

The MCP spec needs a concrete config surface instead of only naming provider
kinds.

### MCP Client Config

Minimum fields:

- provider ID
- enabled flag
- target endpoint or stdio command target
- transport kind (`stdio`, `streamable-http`)
- protocol revision preferences
- headers/env/working directory references
- roots configuration if supported
- local client capabilities to advertise (`sampling`, `elicitation`, `roots`, etc.)
- recoverability mode
- sync policy (`eager`, `lazy`, `on-demand`)
- default exposure behavior for imported capabilities, with the initial default set to inspectable-only

### MCP Server Config

Minimum fields:

- provider/service ID
- enabled flag
- transport kind and bind/listen configuration
- auth mode
- exported capability selectors
- exported prompt selectors
- exported resource selectors
- protocol revision preferences
- resumability/event-store policy
- logging/audit policy
- explicit export-default setting, with the initial default set to deny

This manifest/config shape should be stubbed in early even if later phases
rework the exact schema.

### Deferred-Feature Config Stubs

The standard manifest surface should also reserve explicit configuration for:

- client sampling support
- client elicitation support
- any transport/auth settings those features depend on

These may remain non-functional in early phases, but they should exist as
first-class config so later implementation does not need to invent a parallel
configuration surface.

Credential-bearing values should never be stored raw in provider snapshots.
They must be referenced indirectly through existing runtime credential surfaces
or redacted before persistence.

## Mapping And Normalization Requirements

`framework/mcp/mapping` should define one canonical mapping layer. At minimum it
must cover:

- framework `CapabilityDescriptor` -> MCP tool/prompt/resource descriptors
- MCP tool/prompt/resource descriptors -> framework `CapabilityDescriptor`
- framework `core.Schema` <-> MCP JSON Schema payloads
- framework `ContentBlock` <-> MCP content blocks
- framework errors <-> MCP protocol errors and tool-error payloads

Mapping rules that must be explicit in this spec:

- which framework annotations are never exported over MCP
- which remote MCP metadata remains advisory only
- how invalid remote schemas are handled
- how unsupported MCP content types degrade
- how binary/resource references are represented in framework results
- how tool error vs protocol error is distinguished

Remote capability data must never bypass:

- capability admission
- exposure policy
- insertion policy
- runtime safety budgets

## Registry Synchronization

The client and server integration work should define how MCP state projects into
the capability registry over time.

Required behaviors:

- atomic sync of a discovered remote catalog into the registry
- deterministic capability revocation on disconnect or feature removal
- per-session capability affinity
- provider-level quarantine revokes all MCP-imported capabilities for that provider
- session revoke closes the underlying MCP session and removes only session-bound capabilities

The registry should distinguish:

- admitted but hidden imported capabilities
- inspectable imported capabilities
- callable imported capabilities

This distinction matters because discovery, operator inspection, and model
callability are separate concerns.

## Persistence And Recovery

MCP integration should build directly on the provider snapshot model from
[`3_provider-runtime-spec.md`](/home/lex/Public/Relurpify/docs/MCP-related/3_provider-runtime-spec.md).

Provider snapshots should capture:

- configured transport and target metadata
- negotiated protocol version
- export/import sync generation
- last known peer metadata
- degraded/failed status and reason

Session snapshots should capture:

- session ID
- peer metadata
- negotiated revision
- active subscriptions if recoverable
- capability IDs projected from that session
- last sync timestamp

Recovery rules must be explicit:

- stdio sessions may require full reconnect and re-discovery rather than raw transport resumption
- HTTP resumability can only be restored if the configured event store guarantees it
- recovered sessions must re-run admission and exposure decisions before capabilities become visible again
- stale capability IDs from unrecoverable sessions must be revoked during restore

## Feature Coverage Plan

The engineering plan should target full MCP feature coverage over time rather
than treating prompt/resource/tool support as the terminal scope.

Planning targets should include:

- tools
- prompts
- resources
- subscriptions/watch flows where applicable
- sampling
- elicitation
- richer content block support
- protocol revision compatibility work
- auth-related protocol support where required

Implementation may still be phased, but each phase should extend the same final
architecture rather than preserve legacy fallbacks or introduce temporary
parallel paths.

Feature priorities should follow this order:

1. stdio lifecycle and one primary protocol revision
2. basic streamable HTTP lifecycle
3. client import and server export on the stable provider/capability model
4. advanced features such as sampling, elicitation, subscriptions, and resumability

## Conformance Strategy

Relurpify should keep a conformance baseline from the forked source and add framework-specific tests for:

- protocol negotiation
- transport correctness
- capability export/import correctness
- policy enforcement at capability boundaries
- cancellation and shutdown behavior

Relurpify-specific tests should also cover:

- capability normalization
- hidden-but-admitted capability behavior
- insertion-policy enforcement for remote content
- protocol downgrade behavior
- provider recovery behavior for MCP sessions

Conformance should be split into three layers:

1. Fork preservation tests
2. Relurpify protocol/transport integration tests
3. Framework mapping and policy tests

Required test classes:

- unit tests for protocol version negotiation helpers
- unit tests for schema/content mapping
- unit tests for capability ID generation and catalog diffing
- unit tests for session state transitions
- unit tests for provider snapshot redaction and recovery metadata
- integration tests using in-memory or fixture transports for handshake, discovery, and call flows
- integration tests for stdio subprocess lifecycle
- integration tests for streamable HTTP session lifecycle
- conformance replay tests against imported fixture clients/servers
- failure-path tests for disconnects, invalid schemas, partial sync, cancellation, and list-changed churn

For each implementation phase, tests are part of the deliverable. A phase is
not complete if it adds runtime behavior without:

- unit coverage for new mapping/state helpers
- integration coverage for provider/session lifecycle
- negative tests for policy and failure paths

## Implementation Boundary From `MCP-take-apart`

Preferred import/fork order:

1. protocol message and content primitives
2. JSON-RPC/session machinery
3. stdio transport
4. streamable HTTP transport
5. conformance fixtures

Only after those are stable should Relurpify add:

- client provider facade
- server provider facade
- capability mapping layer
- policy-aware resource/prompt exports

Fork review notes that should be codified here:

- do not import SDK default capability inference as-is; Relurpify must declare capabilities explicitly
- do not preserve SDK public API naming where it conflicts with framework naming
- do not let forked types dictate framework result/content shapes
- forked rough edges around defaults and content shape should be corrected at the Relurpify boundary instead of propagated inward

## Replacement Phases

### Phase 1

- establish forked protocol/session/transport baseline
- define internal package layout
- implement version-aware session negotiation

Phase 1 specifically should include:

- `framework/mcp/protocol`, `versioning`, `session`, and one concrete transport package
- framework-owned session state machine
- explicit supported-version matrix
- protocol handshake tests across supported and unsupported versions
- transport close/cancellation tests

Phase 1 acceptance:

- a real MCP session can be created without using SDK public client/server APIs
- negotiated protocol version is stored in runtime session state
- shutdown is deterministic under cancellation and disconnect
- unit tests cover version helpers, session transitions, and transport teardown

### Phase 2

- implement MCP client provider
- implement capability import mapping on the final capability model

Phase 2 specifically should include:

- manifest/config parsing for MCP client providers
- remote catalog discovery and pagination support
- stable framework capability ID generation for imported tools/prompts/resources
- incremental re-sync on list-changed notifications
- provider/session snapshot support for imported sessions
- policy-aware invocation path for imported callable capabilities
- inspectable-by-default exposure for imported capabilities

Phase 2 acceptance:

- a configured MCP client provider performs real handshake, discovery, import, and invocation
- imported capabilities are provider runtime-family capabilities, not local tools
- imported capabilities are inspectable by default and require explicit exposure to become callable
- disconnect/reconnect revokes and re-admits capabilities deterministically
- unit tests cover mapping, diffing, and sync
- integration tests cover discovery, invocation, disconnect, and recovery

### Phase 3

- implement MCP server provider
- implement capability export mapping on the final capability model

Phase 3 specifically should include:

- manifest/config parsing for MCP server services
- export projection by selector/policy
- tool/prompt/resource export handlers
- request authorization and audit binding for exported actions
- per-peer server session tracking
- list-changed/update notification support
- stubbed manifest-owned export config surface with default-deny behavior

Phase 3 acceptance:

- a configured MCP server exports selected framework capabilities/resources/prompts over real MCP transports
- export remains default-deny unless explicit selectors/config enable exposure
- exported actions enforce framework policy before execution
- per-peer session state is visible through provider/session snapshots
- unit tests cover export mapping and policy gates
- integration tests cover peer connection, listing, invocation/read/get flows, and revocation

### Phase 4

- complete advanced feature support on the same architecture
- harden recovery, subscriptions, and auth-related protocol support

Phase 4 specifically should include:

- subscriptions/resource update flows
- sampling and elicitation support where enabled by policy/config
- richer content block and binary/resource handling
- resumability support where backed by durable storage
- auth-related protocol support needed for selected transports/deployments
- conformance replay against imported fixtures for all supported revisions

Phase 4 acceptance:

- advanced MCP features do not introduce parallel registry or policy paths
- supported advanced features are version-gated and recoverable where promised
- integration and conformance suites cover success and failure paths for each enabled advanced feature

## Acceptance

This specification is complete when:

- fork boundaries are explicit
- no external SDK dependency is required
- MCP client/server roles are defined as Relurpify-owned subsystems
- protocol-version handling is an explicit architectural concern
- the long-term path to full MCP feature coverage is explicit
- MCP client/server support does not rely on duplicate legacy registry or invocation code paths
