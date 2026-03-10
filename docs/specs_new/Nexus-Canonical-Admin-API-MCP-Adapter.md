# Nexus Canonical Admin API and Versioned MCP Adapter

**Version:** 0.1.0-draft
**Status:** Engineering Specification
**Date:** 2026-03-10
**Scope:** Nexus admin surface, MCP export boundary, and `nexusish` integration contract

---

## 1. Decision

MCP is not the Nexus admin API.

MCP is a transport adapter for the Nexus admin API. The canonical admin protocol
is defined in Go as typed request and response structs plus a typed service
interface. Versioning lives at that canonical layer. MCP tool names remain
stable and map onto versioned canonical operations.

This keeps Nexus free to support:

- local in-process callers during transition
- stdio MCP transport
- HTTP MCP transport
- multiple admin API versions at once

without treating MCP tool names as the source of truth.

---

## 2. Why This Is Needed

The current repo state shows a temporary direct coupling:

- `app/nexus/admin/admin.go` exposes ad hoc helper functions such as
  `ApprovePairing` and `RejectPairing`
- `app/nexusish/runtime/runtime.go` imports `app/nexus/admin` directly
- `app/nexus/status/status.go` returns a local snapshot struct rather than a
  versioned admin protocol result

That shape is acceptable for early local iteration, but it is not a stable
remote admin contract. It makes `nexusish` depend on Nexus internals and leaves
no clean place to define API versioning, scope filtering, or MCP resource
exports.

There is a more immediate correctness problem underneath the architecture issue:

- the current offline admin helpers open fresh SQLite connections per call
- `NexusApp.Handler()` creates the live in-process `nodeManager`,
  `channel.Manager`, and gateway materializer used by the running server
- `nexusish` direct-calling the offline package only works safely when Nexus is
  not running

When Nexus is already running, admin operations must be served by that running
process. The admin API cannot be implemented as a second file-level writer.

---

## 3. Two Admin Modes

Nexus has two distinct admin execution modes and they must stay separate.

### 3.1 Offline Admin Helpers

The existing file-based helpers remain useful for local CLI repair and setup:

- `nexus node approve`
- `nexus node reject`
- config resolution and diagnostics when the server is not running

These helpers:

- open stores directly
- may create short-lived DB connections
- are not the live admin API

They should remain in the existing package, but be treated as offline commands.
The recommended file split is:

- `app/nexus/admin/offline.go`
- `app/nexus/admin/service.go`

### 3.2 Live Admin Service

The live admin API exists only inside the running Nexus process and must be
backed by already-wired in-process components.

This service is what MCP exports.

---

## 4. Canonical Admin Service

The canonical contract should live in the Nexus application layer because it
depends on live Nexus runtime components. The recommended package is:

- `app/nexus/admin`

The package must define a single source-of-truth interface for live admin
operations.

```go
// Version: v1alpha1
type AdminService interface {
    // Nodes
    ListNodes(ctx context.Context, req ListNodesRequest) (ListNodesResult, error)
    GetNode(ctx context.Context, req GetNodeRequest) (GetNodeResult, error)
    RevokeNode(ctx context.Context, req RevokeNodeRequest) (RevokeNodeResult, error)
    ListPendingPairings(ctx context.Context, req ListPendingPairingsRequest) (ListPendingPairingsResult, error)
    ApprovePairing(ctx context.Context, req ApprovePairingRequest) (ApprovePairingResult, error)
    RejectPairing(ctx context.Context, req RejectPairingRequest) (RejectPairingResult, error)

    // Sessions
    ListSessions(ctx context.Context, req ListSessionsRequest) (ListSessionsResult, error)
    GetSession(ctx context.Context, req GetSessionRequest) (GetSessionResult, error)
    CloseSession(ctx context.Context, req CloseSessionRequest) (CloseSessionResult, error)

    // Identity
    ListSubjects(ctx context.Context, req ListSubjectsRequest) (ListSubjectsResult, error)
    ListExternalIdentities(ctx context.Context, req ListExternalIdentitiesRequest) (ListExternalIdentitiesResult, error)
    ListTokens(ctx context.Context, req ListTokensRequest) (ListTokensResult, error)
    IssueToken(ctx context.Context, req IssueTokenRequest) (IssueTokenResult, error)
    RevokeToken(ctx context.Context, req RevokeTokenRequest) (RevokeTokenResult, error)

    // Channels
    ListChannels(ctx context.Context, req ListChannelsRequest) (ListChannelsResult, error)
    RestartChannel(ctx context.Context, req RestartChannelRequest) (RestartChannelResult, error)

    // Policy
    ListPolicyRules(ctx context.Context, req ListPolicyRulesRequest) (ListPolicyRulesResult, error)
    SetPolicyRuleEnabled(ctx context.Context, req SetPolicyRuleEnabledRequest) (SetPolicyRuleEnabledResult, error)

    // Gateway
    Health(ctx context.Context, req HealthRequest) (HealthResult, error)
    ListEvents(ctx context.Context, req ListEventsRequest) (ListEventsResult, error)

    // Tenant management
    ListTenants(ctx context.Context, req ListTenantsRequest) (ListTenantsResult, error)
}
```

Notes:

- This interface replaces ad hoc admin helper functions as the long-term admin
  boundary.
- Concrete business logic belongs behind the interface, not in the MCP layer.
- Early phases may implement only a subset of methods, but the request and
  response conventions in this document must still be used.

---

## 5. Live Service Dependencies

The live implementation must hold references to Nexus components created inside
`NexusApp.Handler()`. It must not reopen the world on every request.

Recommended `NexusApp` additions:

```go
type NexusApp struct {
    // existing fields

    nodeManager       *fwnode.Manager
    channelManager    *channel.Manager
    stateMaterializer *gateway.StateMaterializer
}
```

Recommended live service shape:

```go
type adminServiceImpl struct {
    nodes        fwnode.NodeStore
    nodeManager  *fwnode.Manager
    sessions     session.Store
    identities   identity.Store
    events       event.Log
    materializer *gateway.StateMaterializer
    channels     *channel.Manager
    partition    string
    policyEngine authorization.PolicyEngine
}
```

This matters for correctness, not just cleanliness.

Example: pending node pairings are coordinated through the live node manager.
If the admin path bypasses that manager and talks directly to a separate store
handle, the running process can keep stale in-memory state and capability
registration can drift from persisted state.

---

## 6. Request and Response Envelope

All operations must share a consistent envelope before any MCP exporter or
client runtime is written.

```go
type AdminRequest struct {
    APIVersion string
    Principal  core.AuthenticatedPrincipal
    TenantID   string
    RequestID  string
}

type AdminResult struct {
    APIVersion string
    RequestID  string
}
```

Rules:

- `APIVersion` is the canonical admin protocol version such as `v1alpha1`
- `Principal` is injected by authn or policy middleware and is never accepted
  from an untrusted caller payload
- `TenantID` is injected or validated by authn and authorization layers
- `RequestID` is the correlation ID echoed back in the result

Example:

```go
type ApprovePairingRequest struct {
    AdminRequest
    Code string
}

type ApprovePairingResult struct {
    AdminResult
    NodeID   string
    PairedAt time.Time
}
```

`AdminRequest.Principal` should be omitted from generated JSON schema and MCP
tool args because it is runtime-injected state, not caller input.

---

## 7. Error Taxonomy

Canonical admin operations must return typed admin errors rather than raw
transport-facing strings.

```go
type AdminErrorCode string

const (
    AdminErrorUnauthorized   AdminErrorCode = "unauthorized"
    AdminErrorNotFound       AdminErrorCode = "not_found"
    AdminErrorConflict       AdminErrorCode = "conflict"
    AdminErrorPolicyDenied   AdminErrorCode = "policy_denied"
    AdminErrorInvalidArgument AdminErrorCode = "invalid_argument"
    AdminErrorInternal       AdminErrorCode = "internal"
)

type AdminError struct {
    Code    AdminErrorCode
    Message string
    Detail  map[string]any
}
```

Rules:

- the canonical layer deals in `AdminError`
- the MCP adapter translates `AdminError` into MCP error text and structured
  result content
- logs and audit events should record `Code` and selected safe `Detail` fields

---

## 8. Pagination Contract

Every list operation uses the same opaque cursor envelope.

```go
type PageRequest struct {
    Cursor string
    Limit  int
}

type PageResult struct {
    NextCursor string
    Total      int
}
```

Rules:

- `Limit == 0` means server default
- `NextCursor == ""` means end of result set
- `Total == -1` means unknown
- cursor format is opaque to clients and should be base64-encoded state

Example:

```go
type ListNodesRequest struct {
    AdminRequest
    Page PageRequest
}

type ListNodesResult struct {
    AdminResult
    PageResult
    Nodes []core.NodeDescriptor
}
```

---

## 9. MCP Adapter Boundary

The MCP adapter is the only layer that knows about MCP protocol types.

```text
AdminService (canonical, typed)
    -> AdminMCPExporter
    -> framework/middleware/mcp/server
    -> stdio or HTTP transport
```

The adapter should live alongside the live admin service, for example:

- `app/nexus/admin/mcp_exporter.go`

It implements the exporter interface already used by
`framework/middleware/mcp/server`.

### 7.1 Tool Registry

Admin operations are declared once in a registry table.

```go
type adminToolDef struct {
    Name        string
    Description string
    Schema      json.RawMessage
    MinScope    string
    Handler     adminToolHandler
}
```

Example tool names:

- `nexus.nodes.list`
- `nexus.nodes.get`
- `nexus.nodes.approve_pairing`
- `nexus.sessions.list`
- `nexus.identity.issue_token`
- `nexus.gateway.health`

Rules:

- tool names are stable capability names, not version names
- version does not appear in the MCP tool name
- schema is generated from caller-supplied argument structs, not from
  server-injected fields like `Principal`

### 7.2 `ListTools`

`ListTools`:

1. resolves the session principal
2. filters the registry by required scope
3. returns only the tools visible to that caller

This prevents observers from discovering admin-only tools.

### 7.3 `CallTool`

`CallTool` flow:

1. look up the tool in the registry
2. extract the principal from request context
3. verify caller scope satisfies `MinScope`
4. read requested `api_version` from the args payload
5. unmarshal caller args into the version-specific typed request
6. inject `Principal`, `TenantID`, and `RequestID`
7. evaluate policy for the operation
8. call the canonical `AdminService`
9. marshal the typed result into MCP structured content
10. emit an audit event

The policy decision here is distinct from transport authentication. A caller
may be authenticated but still denied by admin policy.

### 7.4 Resources

MCP resources expose read models and snapshots, not alternate business logic.

Example URI shape:

```text
nexus://nodes/enrolled?tenant=default&limit=50&cursor=abc
```

Resource mapping rules:

- resource URIs map onto canonical list or get operations
- URI query params become typed request filter fields
- resource content serializes canonical result structs as JSON
- subscribable resources must be explicitly declared; do not assume every
  resource is watchable

---

## 10. HTTP Auth Middleware

The exporter interface only receives `context.Context`, so admin principals must
be injected before the MCP service is reached.

The HTTP mount should use dedicated auth middleware:

```go
func adminAuthMiddleware(
    resolver PrincipalResolver,
    next http.Handler,
) http.Handler
```

Responsibilities:

- read bearer token from `Authorization`
- resolve principal using the same resolver already used by the gateway
- reject callers without admin or operator authority
- store the resolved principal in request context for the MCP exporter

This should reuse the existing gateway principal resolution path rather than
introduce a second auth table or separate token validation stack.

---

## 11. Versioning

Two version domains exist and must stay separate.

- MCP protocol version: negotiated by MCP handshake, for example `2025-06-18`
- Nexus admin API version: carried in the request payload, for example
  `v1alpha1`

Canonical versioning rules:

- breaking admin API changes bump `APIVersion`
- MCP tool names stay stable across admin API versions
- the adapter dispatches by `APIVersion`
- multiple canonical versions may be supported at once during migration

Example dispatcher shape:

```go
func handleApprovePairing(
    ctx context.Context,
    svc AdminService,
    apiVersion string,
    args map[string]any,
) (*protocol.CallToolResult, error) {
    switch apiVersion {
    case "v1alpha1":
        // decode v1alpha1 args
    case "v1":
        // decode v1 args
    default:
        return nil, AdminError{
            Code:    AdminErrorInvalidArgument,
            Message: "unsupported API version",
        }
    }
}
```

---

## 12. Naming, Scope, and Resource Conventions

The following must be standardized before handler work begins.

### 9.1 Operation Names

Use:

- `nexus.<domain>.<verb>`

Initial domains:

- `nodes`
- `sessions`
- `identity`
- `channels`
- `policy`
- `gateway`
- `tenants`

### 9.2 Scope Hierarchy

Initial hierarchy:

- `nexus:admin`
- `nexus:operator`
- `nexus:observer`

Ordering:

- `nexus:admin` includes operator and observer rights
- `nexus:operator` includes observer rights
- `nexus:observer` is read-only

`ListTenants` is `gateway-admin` only and should remain outside normal tenant
operator scope.

### 9.3 Resource URI Scheme

Use:

- `nexus://<domain>/<collection>?<filters>`

Examples:

- `nexus://nodes/enrolled?...`
- `nexus://nodes/pending?...`
- `nexus://sessions/active?...`
- `nexus://gateway/events?...`

### 9.4 Audit Events

Admin mutations must emit dedicated framework event types such as:

- `admin.pairing.approved.v1`
- `admin.pairing.rejected.v1`
- `admin.node.revoked.v1`
- `admin.token.issued.v1`
- `admin.token.revoked.v1`
- `admin.policy.rule.updated.v1`

These are separate from lower-level runtime events like
`node.pairing.approved.v1`.

---

## 13. NexusApp Mounting

The live admin service should be mounted from `NexusApp.Handler()` beside the
existing gateway handler, not as a standalone file tool.

Recommended wiring:

```go
adminSvc := newAdminService(adminServiceImpl{
    nodes:        a.NodeStore,
    nodeManager:  nodeManager,
    sessions:     a.SessionStore,
    identities:   a.IdentityStore,
    events:       a.EventLog,
    materializer: stateMaterializer,
    channels:     manager,
    partition:    a.partition(),
    policyEngine: policyEngine,
})

adminExporter := newAdminMCPExporter(adminSvc)
adminMCPSvc := mcpserver.New(
    protocol.PeerInfo{Name: "nexus-admin", Version: "v1alpha1"},
    adminExporter,
    mcpserver.Hooks{},
)

adminHandler := adminAuthMiddleware(
    staticGatewayPrincipalResolver(a.Config.Gateway.Auth),
    adminMCPSvc,
)

mux := http.NewServeMux()
mux.Handle(a.gatewayPath(), srv.Handler())
mux.Handle("/admin/mcp", adminHandler)
```

This keeps:

- `/gateway` for gateway WebSocket traffic
- `/admin/mcp` for admin MCP over HTTP

with one auth resolver and one running Nexus process.

---

## 14. Stdio Transport for Local `nexusish`

The first local client transport should be stdio, exposed as:

- `nexus admin mcp`

This subcommand:

- opens stores and live dependencies for a local admin session
- constructs the same `AdminService`
- serves MCP over stdin/stdout using `mcp.Service.ServeConn`

Stdio is the correct bootstrap path for local `nexusish` because:

- Nexus may not yet be listening on HTTP
- same-user local operation does not need the remote HTTP auth path first
- it cleanly reuses the existing MCP client transport stack

---

## 15. `nexusish` Runtime Direction

`app/nexusish/runtime/runtime.go` should stop importing the offline admin
helpers and become an MCP client.

Recommended startup policy:

1. try configured Nexus HTTP admin endpoint
2. if unavailable, spawn `nexus admin mcp` and connect over stdio
3. if neither path works, surface first-run or installation guidance

This gives one client code path across:

- local stdio
- remote HTTP

while keeping the canonical admin protocol identical.

---

## 16. Gateway API Remaining Work

The gateway WebSocket path is already the live API surface for session and node
traffic. It is mostly built and should be treated as a separate track from the
admin MCP build.

Remaining protocol work:

- `ping`
- `pong`
- `error`
- `session.close`
- `admin.snapshot`

Remaining wiring work:

- confirm node challenge-response completion flows into live node registration
- confirm registered nodes appear in `ListCapabilitiesForPrincipal`
- confirm capability invocation path is end-to-end after registration

In the current code, `NexusApp.Handler()` already wires:

- `ListCapabilitiesForPrincipal`
- `InvokeCapability`
- `HandleNodeConnection`

through the gateway server and node runtime. The gap to close is validating the
registration path end-to-end, not replacing the whole gateway design.

---

## 17. Transition Plan for Existing Code

The current direct coupling must be removed in phases rather than by a single
rewrite.

### Phase 1: Canonical Contract

- split offline helpers from live admin service
- define `AdminService` in `app/nexus/admin/service.go`
- move current pairing approval and rejection logic behind that service
- adapt status snapshot logic into `Health` and read-model list operations

### Phase 2: MCP Export

- add `AdminMCPExporter`
- expose it through `framework/middleware/mcp/server`
- mount HTTP transport at `/admin/mcp`
- add a `nexus admin mcp` stdio subcommand

### Phase 3: `nexusish` Cut-Over

- stop importing `app/nexus/admin` from `app/nexusish/runtime/runtime.go`
- replace direct calls with an MCP client over stdio or HTTP
- keep local-stdio as the default fast path when `nexus` is on the same host

### Phase 4: Compatibility Window

- support at least one overlapping canonical API version during migration
- keep MCP tool names stable
- remove old canonical versions only after all first-party clients are moved

---

## 18. Implementation Order

Build in this order:

1. Finish the remaining gateway frame and node-registration work
2. Split offline helpers from live admin service
3. Canonical request and response types
4. `AdminService` interface
5. Concrete `AdminService` implementation over live Nexus components
6. Policy evaluation hook for admin operations
7. `AdminMCPExporter`
8. HTTP transport at `/admin/mcp`
9. stdio transport via `nexus admin mcp`
10. `nexusish` MCP runtime adapter

This order matters because the MCP layer should export a stable canonical
contract, not become the contract.

---

## 19. Lock Before Coding

The following are blocking decisions and should be treated as protocol locks for
`v1alpha1`:

1. operation naming convention
2. request and response envelope
3. admin error code taxonomy

The following can evolve during implementation, but should be stubbed before the
MCP exporter is wired:

1. resource URI scheme
2. cursor encoding format
3. admin audit event taxonomy
4. exact scope-to-operation matrix
