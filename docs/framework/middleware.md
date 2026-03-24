# Middleware

## Synopsis

`framework/middleware` provides the transport and protocol layers that connect
Relurpify agents to each other and to external systems. The middleware sits
between the agent runtime and the network, handling connection management,
session isolation, event routing, and the full MCP protocol stack.

---

## Package Map

```
framework/middleware/
├── channel/    Concurrent communication channel manager
├── fmp/        Federated mesh protocol orchestration, context transfer, discovery, and federation policy
├── gateway/    HTTP server and replay recording for Nexus
├── node/       WebSocket connections to remote agent nodes
├── session/    Session routing and event-stream isolation
└── mcp/        Full MCP (Model Context Protocol) implementation
    ├── protocol/   Wire-format types (versions 2025-06-18, 2025-11-25)
    ├── client/     MCP client — connects to external MCP servers
    ├── server/     MCP server — exposes capabilities to MCP clients
    ├── session/    MCP session lifecycle management
    ├── schema/     JSON schema validation and conversion
    ├── mapping/    Capability import/export translation
    └── versioning/ Protocol version negotiation
```

---

## channel

The channel package provides the adapter layer for inbound and outbound agent
communication pipelines. Adapters normalize messages from external services
(chat relays, messaging platforms) into framework events and deliver outbound
replies.

**Manager** supervises registered adapters: starts/stops them together, routes
outbound messages by channel name, and supports individual adapter restarts.
**InboundMessage** carries a normalized sender Identity and MessageContent.
**OutboundMessage** carries channel name and conversation ID for reply routing.

---

## gateway

`framework/middleware/gateway` provides two services for the Nexus gateway:

**HTTP server** — routes incoming requests (node connections, capability
requests, admin API calls, event subscriptions) to their handlers.

**Replay recording** — in capture mode, all requests and responses are written
to a tape file. In replay mode, the tape plays back verbatim. This enables
integration tests to run against a recorded Nexus session without live node
connections, keeping CI hermetic.

The gateway is also the current admission point for FMP-aware node transport.
`server.go` accepts node connect metadata that now includes:

- `transport_profile`
- `session_nonce`
- `session_issued_at`
- `session_expires_at`
- `peer_key_id`
- FMP runtime metadata such as `runtime_id`, `runtime_version`,
  `compatibility_class`, and `supported_context_classes`

`fmp_transport.go` defines `FMPTransportPolicy`, which validates:

- whether an FMP transport profile is allowed
- whether a nonce is present and non-replayed
- whether the negotiated session lifetime is within policy
- whether insecure transport is allowed only for loopback development

This is the current bridge between generic gateway connections and
mesh-specific transport hardening.

---

## fmp

`framework/middleware/fmp` is the middleware owner for federated mesh protocol
mechanics.

It currently covers:

- protocol objects and validation
- lineage, attempt, lease, and fencing orchestration
- context packaging and sealed context transport metadata
- discovery advertisements for nodes, runtimes, and exports
- route selection and capability projection
- chunked and external transfer session management
- trust bundle and boundary policy evaluation
- federated gateway forwarding policy
- Nexus adapter hooks for tenant, session, export, and federation policy

The package does not own tenant administration or operator UX. Those surfaces
live in `app/nexus`.

### Runtime and export registration

FMP now distinguishes between simple discovery records and authoritative
runtime registration. Connected nodes can register runtimes and advertise
exports through the framed mesh channel using messages such as:

- `fmp.runtime.register`
- `fmp.runtime.registered`
- `fmp.export.advertise`
- `fmp.export.advertised`

These flows are routed from `app/nexus/server/node_runtime.go` into the FMP
service so that runtime and export discovery can be tied back to enrolled node
identity and attestation metadata.

### Chunk transfer control path

Chunked transfer no longer exists only as a packaging concept. The current
framed node transport carries explicit FMP chunk session messages:

- `fmp.chunk.open`
- `fmp.chunk.opened`
- `fmp.chunk.read`
- `fmp.chunk.data`
- `fmp.chunk.ack`
- `fmp.chunk.acked`
- `fmp.chunk.cancel`
- `fmp.chunk.cancelled`
- `fmp.chunk.error`

These messages bridge the node transport in `framework/middleware/node` to the
chunk transfer manager in `framework/middleware/fmp`.

---

## node

`framework/middleware/node` manages the lifecycle of WebSocket connections
to remote agent nodes registered with the Nexus gateway.

**NodeManager** tracks all connected nodes: pairing, authentication, capability
advertisement, and graceful disconnect.

**ws_connection.go** owns the per-node WebSocket connection, framing messages,
handling ping/pong, and surfacing structured events to the session router.

The node transport now has a framed mode for mesh traffic. `FramedRPCConn`
separates message channels such as:

- `node.control`
- `node.capability`
- `fmp.control`
- `fmp.data`

`WSConnection` also supports a frame handler for non-capability messages, which
is how Nexus routes FMP runtime registration, export advertisement, chunk
transfer, and tenant-bound resume execution over the same node connection.

**credential.go** stores and rotates node authentication credentials used
during the pairing handshake.

---

## session

`framework/middleware/session` provides session routing and event-sink
integration for agent conversation isolation.

**Router** routes InboundMessages to existing or newly created SessionBoundary
objects using a composite key (scope, partition, channel ID, peer ID, thread
ID). DefaultRouter enforces ownership and tenant boundaries via PolicyEngine.

**SessionSink** implements channel.EventSink: on each inbound message it
appends the raw event to the event log, resolves sender identity, routes the
message to a SessionBoundary, and appends a normalized session.message event.

**Store** persists SessionBoundary records with upsert, lookup, list, delete,
and TTL-based expiry sweep operations.

---

## MCP (Model Context Protocol)

Relurpify implements the full Model Context Protocol in `framework/middleware/mcp`,
supporting protocol versions **2025-06-18** and **2025-11-25**.

MCP defines how AI models discover and invoke capabilities (tools, prompts,
resources) provided by external servers, and how those servers expose their
capabilities. Relurpify acts as both an MCP client (connecting to external MCP
servers) and an MCP server (exposing its own capabilities to external clients).

### protocol

`mcp/protocol` declares every wire-format type: JSON-RPC request, response,
notification, and error messages for both MCP versions. No transport or session
logic lives here — only the protocol surface.

### client

`mcp/client` implements the MCP client that connects to external MCP servers
and imports their capabilities into the Relurpify capability registry.

On connect, the client:
1. Sends `initialize` and negotiates a protocol version (via `mcp/versioning`).
2. Calls `tools/list`, `prompts/list`, `resources/list`.
3. Passes listings through `mcp/mapping` to produce `CapabilityDescriptor` objects.
4. Registers those descriptors with the capability registry.

Subsequent `tools/call`, `prompts/get`, and `resources/read` requests are
routed to the external server transparently.

### server

`mcp/server` exposes Relurpify capabilities to external MCP clients over
HTTP (Server-Sent Events) and stdio transports.

The server handles the full MCP lifecycle:
- `initialize` / `initialized`
- `tools/list`, `tools/call`
- `prompts/list`, `prompts/get`
- `resources/list`, `resources/read`

Incoming `tools/call` requests are dispatched through the internal capability
registry so policy enforcement and sandboxing apply equally to MCP-triggered
calls.

### session

`mcp/session` tracks active MCP sessions from initialization through
termination. Each session holds its negotiated protocol version, active
subscriptions, and in-flight request correlations.

### schema

`mcp/schema` validates tool input schemas against the MCP specification and
converts between the MCP JSON schema format and the internal schema
representation used by `CapabilityDescriptor`.

### mapping

`mcp/mapping` is the translation layer between MCP wire format and internal
types:

- **importing.go** — converts `ListTools`/`ListPrompts`/`ListResources`
  responses from an external MCP server into `CapabilityDescriptor` objects.
- **exporting.go** — converts internal `CapabilityDescriptor` objects into the
  MCP response format served to external clients.

### versioning

`mcp/versioning` implements version negotiation during the MCP `initialize`
handshake. The client proposes its supported versions; the server selects the
highest mutually supported one. Both the MCP client and server use this package
so the selection logic is not duplicated.

---

## Integration with the Nexus Gateway

The middleware packages are assembled by `app/nexus` at startup:

```
                      ┌──────────────────────────────┐
                      │  Nexus Gateway               │
                      │                              │
  Remote node ──ws──▶ │  middleware/node             │
                      │      │                       │
                      │  middleware/fmp              │
                      │      │                       │
                      │  middleware/session          │
                      │      │                       │
                      │  middleware/channel          │
                      │      │                       │
                      │  middleware/gateway (HTTP)   │
                      │                              │
  MCP client ──http──▶│  middleware/mcp/server       │
                      └──────────────────────────────┘
```

`app/nexus/bootstrap` wires these components together at startup, injecting
the shared event log and identity store.

In current Nexus wiring, the application also attaches optional FMP stores and
adapters such as:

- tenant export enablement store
- tenant federation policy store
- identity/session stores as the FMP Nexus authority adapter
- the default FMP transport policy for node connections

That makes the middleware stack FMP-aware without moving tenant control-plane
ownership out of `app/nexus`.

---
