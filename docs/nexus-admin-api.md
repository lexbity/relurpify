# Nexus Admin API

## Synopsis

Nexus exposes a canonical typed admin API, and MCP is one transport adapter for
that API. MCP is not the source of truth for Nexus administration.

The canonical admin contract is defined as typed request and response structs
plus a typed service interface in the Nexus application layer. Versioning lives
at that canonical layer. Stable MCP tool names map onto versioned canonical
operations.

This keeps Nexus free to support local in-process callers, stdio MCP, HTTP MCP,
and multiple admin API versions at once without making MCP tool names the
primary contract.

---

## Why This Split Exists

Nexus has to support both local administrative behavior and a stable remote
administration boundary.

Ad hoc helper functions and direct package imports are acceptable for early
local iteration, but they are not sufficient as a long-term remote admin
contract. They make external callers depend on Nexus internals and leave no
clean place to define:

- API versioning
- authorization scope filtering
- MCP tool export
- MCP resource export

There is also a correctness issue underneath the architecture. Offline helpers
may open new store handles directly, while the running Nexus process already
owns live in-memory managers and materializers. When Nexus is running, admin
operations must be served by that live process rather than by a second
file-level writer.

---

## Two Admin Modes

Nexus has two distinct admin execution modes, and they should remain separate.

### Offline admin helpers

Offline helpers are for local CLI repair and setup when the server is not
running. They may:

- open stores directly
- use short-lived database connections
- resolve config and perform repair actions

These helpers are useful, but they are not the live admin API.

### Live admin service

The live admin API exists inside the running Nexus process and is backed by the
already-wired in-process components created by the application runtime.

This live service is the boundary that MCP exports.

---

## Canonical Admin Service

The canonical admin contract belongs in the Nexus application layer because it
depends on live Nexus runtime components.

The service is a typed interface that becomes the single source of truth for
live admin operations across domains such as:

- nodes
- sessions
- identity
- channels
- policy
- gateway health and events
- tenant management

The key rule is that business logic lives behind this interface, not in the MCP
adapter layer.

The canonical interface supports operations such as:

- listing, getting, revoking, and pairing nodes
- listing, getting, and closing sessions
- listing subjects, identities, and tokens
- issuing and revoking tokens
- listing and restarting channels
- listing and updating policy rules
- health and event inspection
- tenant listing

---

## Live Service Dependencies

The live admin implementation must hold references to the same runtime
components created inside the running Nexus application, such as:

- the node manager
- the channel manager
- the gateway state materializer
- stores for nodes, sessions, and identities
- the event log
- the policy engine

This is a correctness requirement, not just a cleanup preference. If admin
operations bypass the live runtime components and reopen state independently,
the running process can drift from persisted state and expose stale or
inconsistent behavior.

---

## Request and Response Envelope

All canonical admin operations use a shared request and response envelope.

The request envelope carries:

- `APIVersion`
- `Principal`
- `TenantID`
- `RequestID`

The result envelope carries:

- `APIVersion`
- `RequestID`

The important rules are:

- `APIVersion` identifies the canonical Nexus admin API version, such as
  `v1alpha1`
- `Principal` is injected by authentication and authorization middleware, not
  accepted from untrusted caller input
- `TenantID` is injected or validated by authn and authorization layers
- `RequestID` is a correlation ID echoed in results

For generated schemas and MCP tool arguments, runtime-injected fields such as
`Principal` should be omitted because they are not caller-supplied input.

---

## Error Model

Canonical admin operations return typed admin errors rather than raw
transport-facing strings.

The error taxonomy includes:

- `unauthorized`
- `not_found`
- `conflict`
- `policy_denied`
- `invalid_argument`
- `internal`

The canonical layer deals in typed admin errors. The MCP adapter translates
those errors into MCP-facing error text and structured result content. Audit
and logs should record the error code and safe detail fields.

---

## Pagination Contract

List operations share one pagination model with:

- an opaque cursor
- a limit
- a next cursor in results
- an optional total count

The important rules are:

- `Limit == 0` means server default
- an empty `NextCursor` means end of results
- `Total == -1` means unknown
- cursor contents are opaque to clients

This keeps list-style admin operations consistent across domains.

---

## MCP Adapter Boundary

The MCP adapter is the only layer that knows about MCP protocol types.

The layering is:

```text
AdminService (canonical, typed)
    -> Admin MCP adapter/exporter
    -> MCP server middleware
    -> stdio or HTTP transport
```

The adapter's responsibilities are:

- expose stable MCP tool names
- translate MCP calls into typed canonical requests
- inject runtime auth context
- dispatch by canonical API version
- translate canonical results and errors into MCP results
- emit audit events

MCP should not contain the business logic for admin operations.

### Tool registry

Admin operations are declared once in a registry table with:

- stable tool name
- description
- generated schema
- minimum required scope
- handler

Tool names are stable capability names such as:

- `nexus.nodes.list`
- `nexus.nodes.get`
- `nexus.nodes.approve_pairing`
- `nexus.sessions.list`
- `nexus.identity.issue_token`
- `nexus.gateway.health`

Version does not appear in the tool name. The canonical API version is carried
in the request payload instead.

### Tool visibility and invocation

`ListTools` should:

1. resolve the session principal
2. filter tools by required scope
3. return only tools visible to that caller

`CallTool` should:

1. look up the tool in the registry
2. extract principal from request context
3. verify caller scope
4. read the requested admin `api_version`
5. unmarshal into the version-specific typed request
6. inject `Principal`, `TenantID`, and `RequestID`
7. evaluate policy for the operation
8. call the canonical service
9. marshal the typed result into MCP structured content
10. emit an audit event

Authentication and policy are separate decisions. A caller may be
authenticated and still be denied by admin policy.

### MCP resources

MCP resources expose read models and snapshots, not alternate business logic.

Resource mapping rules are:

- resource URIs map onto canonical list or get operations
- URI query parameters map to typed request filters
- resource content serializes canonical result structs as JSON
- subscriptions must be declared explicitly; not every resource is watchable

The resource URI scheme should use:

- `nexus://<domain>/<collection>?<filters>`

Example:

- `nexus://nodes/enrolled?tenant=default&limit=50&cursor=abc`

---

## HTTP Auth Middleware

For HTTP-mounted MCP endpoints, admin principals must be resolved before the
request reaches the exporter.

The auth middleware should:

- read bearer tokens from `Authorization`
- resolve principals through the same resolver already used by the gateway
- reject callers without sufficient admin or operator authority
- store the resolved principal in request context

This keeps the auth path consistent and avoids introducing a separate token
validation stack just for the MCP admin surface.

---

## Versioning Model

Two version domains exist and must remain separate:

- the MCP protocol version negotiated by MCP
- the canonical Nexus admin API version carried in request payloads

Canonical versioning rules are:

- breaking admin API changes bump `APIVersion`
- MCP tool names remain stable across admin API versions
- the adapter dispatches by `APIVersion`
- multiple canonical versions may be supported at once during migration

This separation keeps the wire protocol revision from becoming the semantic
version of the admin API itself.

---

## Naming and Scope Conventions

Operation names should use:

- `nexus.<domain>.<verb>`

Initial domains are:

- `nodes`
- `sessions`
- `identity`
- `channels`
- `policy`
- `gateway`
- `tenants`

The initial scope hierarchy is:

- `nexus:admin`
- `nexus:operator`
- `nexus:observer`

Scope ordering is hierarchical:

- admin includes operator and observer rights
- operator includes observer rights
- observer is read-only

`ListTenants` remains outside normal tenant-operator scope and should require
gateway-level admin authority.

---

## Summary

The Nexus admin surface is built around one core rule: the canonical typed
admin API is primary, and MCP is an adapter over it.

That structure gives Nexus:

- one source of truth for live admin behavior
- correct access to running in-process state
- stable MCP tool names with versioned canonical semantics
- a clean boundary for auth, policy, resources, and audit

It also keeps offline repair helpers useful without letting them define the
remote admin architecture.
