# Nexus Identity, Tenancy, and Session Security Specification

**Prepared:** 2026-03-10  
**Status:** Proposed, Phase 1 in progress  
**Scope:** Nexus gateway authentication, node pairing, external provider identity resolution, session ownership, and multi-tenant isolation

## 1. Objective

Define and implement a security architecture for Nexus that closes the currently identified high-severity gaps:

- unauthenticated websocket access
- self-asserted node identity and trust
- spoofable session ownership
- collapsed trust boundaries between external channels and local actors

The target state is a tenant-aware system where every gateway action is attributable to an authenticated principal and every session, node, and external identity is bound to an explicit tenant-scoped owner.

## 2. Current Problems

The current Nexus implementation has four structural issues:

1. Gateway websocket clients are not required to authenticate.
2. Node identity is accepted from client-declared connect metadata instead of server-verified enrollment.
3. Session authorization is driven by user-supplied actor strings and deterministic session keys.
4. External provider traffic is treated as workspace-trusted during session routing.

These are not isolated bugs. They come from a missing identity model.

## 3. Design Goals

- Introduce a first-class multi-tenant identity model.
- Separate authentication from authorization.
- Make node trust server-assigned rather than node-declared.
- Resolve external provider users into internal tenant-scoped subjects before session routing.
- Replace predictable session handles with opaque internal ownership.
- Keep the first implementation phases additive where possible to minimize churn.

## 4. Non-Goals

- Full public SaaS account management in the first rollout.
- Billing, tenant quotas, or commercial organization management.
- Cross-tenant collaboration in the initial design.
- Full OAuth provider implementation for every channel in Phase 1.

## 5. Core Concepts

### 5.1 Tenant

The tenant is the hard isolation boundary.

All durable records must be scoped by `tenant_id`:

- users
- service accounts
- external identities
- node enrollments
- sessions
- approvals
- event partitions or partition metadata

### 5.2 Subject

A subject is the canonical internal identity that can own sessions or perform actions.

Initial subject kinds:

- `user`
- `service_account`
- `node`
- `external_identity`
- `system`

### 5.3 Principal

A principal is an authenticated runtime actor:

- an operator logged into Nexus
- an agent runtime with a bearer token
- a paired node proving possession of its private key
- a verified external-provider webhook event resolved to an internal subject

### 5.4 External Identity

An external identity is a verified mapping from a provider-specific identity into an internal tenant-scoped subject.

Examples:

- Discord user ID in a guild
- Telegram user ID
- Webchat anonymous conversation identity

External identities are not trusted by default. They become actionable only after policy or binding resolution.

### 5.5 Node Enrollment

Node enrollment is the durable pairing record for a device. It includes:

- tenant ownership
- node ID
- bound subject or service account
- public key material
- trust class assigned by the gateway
- verification timestamps and rotation state

### 5.6 Session Ownership

Sessions must be owned by a tenant-scoped subject, not by raw channel IDs.

External routing metadata remains attached to a session as binding metadata, but authorization decisions use the canonical owner subject.

## 6. Data Model

### 6.1 New framework/core concepts

Add core types for:

- `SubjectKind`
- `AuthMethod`
- `ExternalProvider`
- `SubjectRef`
- `AuthenticatedPrincipal`
- `ExternalIdentity`
- `ExternalSessionBinding`
- `NodeEnrollment`

### 6.2 Extend existing security envelopes

Extend:

- `core.EventActor` with optional tenant and subject metadata
- `core.SessionBoundary` with `TenantID`, canonical `Owner`, and external binding metadata
- `core.NodeDescriptor` and `core.NodeCredential` with optional tenant binding data
- `core.PolicyRequest` with tenant and principal metadata required for tenant-aware authorization

These additions are additive in early phases.

### 6.3 Persistence surfaces

New or migrated stores will eventually require:

- `tenants`
- `subjects`
- `external_identities`
- `node_enrollments`
- `session_bindings`
- `auth_tokens` or delegated credentials metadata

SQLite remains the default implementation.

## 7. Authentication Model

### 7.1 Human and operator authentication

Nexus must support a first-class auth provider for operators and agent users.

Recommended initial approach:

- local bootstrap admin credential or CLI-issued bootstrap token
- optional OIDC integration for interactive operator login
- signed access tokens carrying:
  - `tenant_id`
  - `subject_id`
  - `subject_kind`
  - scopes
  - expiry

### 7.2 Agent authentication

Agent runtimes connect using signed bearer tokens issued to:

- a user subject
- a service account
- or a dedicated tenant-scoped agent identity

Agents must never self-assign `actor_id` or tenant membership through the websocket connect frame.

### 7.3 Node authentication

Node pairing becomes a two-step enrollment flow:

1. Pairing request creates a pending enrollment.
2. Approved enrollment stores the node public key and assigned trust class.
3. Every live node websocket connect performs challenge-response.
4. The gateway mints a short-lived node session principal on success.

Trust class is assigned by server policy, not by node input.

### 7.4 External provider verification

For Discord, Telegram, and future providers:

- inbound events are accepted only from the verified provider transport
- provider-specific user IDs are resolved to `ExternalIdentity`
- if resolved, the system derives a tenant-scoped subject
- if unresolved, the message is tagged as unbound and routed into a restricted policy path

## 8. Authorization Model

Every sensitive action must authorize against:

- tenant
- principal subject
- subject role or scopes
- resource ownership
- trust class
- external provider binding

Sensitive actions include:

- websocket subscription to event streams
- session attach/resume/send
- node registration
- capability invocation
- admin inspection APIs

## 9. Session Model

### 9.1 Opaque session identifiers

Internal sessions must use opaque IDs. Deterministic session routing keys may still exist internally for deduplication or lookup, but they are not exposed as authority-bearing handles.

### 9.2 Conversation binding

A session stores both:

- canonical owner subject
- external binding metadata such as provider, conversation ID, thread ID, and external user ID

### 9.3 Authorization rules

Outbound session actions require:

- authenticated principal
- same tenant
- permission to act on the owning subject or explicit delegated rights
- provider binding consistency

## 10. Eventing and Observability

The event log remains the source of truth, but event access must become scoped.

Required changes:

- event frames carry tenant metadata
- clients subscribe to authorized projections, not the raw global stream
- admin feeds are separate from runtime feeds
- session-bound clients see only session-authorized events

## 11. External Provider Binding Model

### 11.1 Discord

Recommended identity tuple:

- provider = `discord`
- external account/guild or application context
- user ID

Optional future dimensions:

- guild membership
- channel role mapping
- bot installation scope

### 11.2 Telegram

Recommended identity tuple:

- provider = `telegram`
- bot account or tenant binding
- user ID

### 11.3 Webchat

Webchat starts as untrusted unless promoted by an explicit auth flow:

- email link
- OAuth
- operator acceptance
- guest-only restricted mode

## 12. Multi-Tenant Architecture

### 12.1 Isolation rules

- No record may be loaded without tenant filtering.
- No websocket session may exist without a resolved tenant.
- No provider capability may be visible across tenants by default.
- No node enrollment may move across tenants without reprovisioning.

### 12.2 Roles

Initial recommended roles:

- `tenant_admin`
- `operator`
- `member`
- `service_account`
- `paired_node`

### 12.3 Delegation

Cross-subject actions should use explicit delegation records instead of implicit operator privilege.

Examples:

- operator sending on behalf of a user-owned conversation
- service account managing a tenant-specific bot session
- node acting for a device-owned capability scope

## 13. Proposed Package Boundaries

Recommended new or expanded packages:

- `framework/core`
  - identity and tenancy types
- `framework/authn`
  - token validation, node challenge verification, principal resolution
- `framework/authz`
  - tenant-aware authorization over policy rules
- `framework/identity`
  - tenant, subject, and external identity stores/services
- `framework/middleware/session`
  - opaque session IDs, owner subjects, provider bindings
- `framework/middleware/node`
  - enrollment and verification, not just pairing code generation

## 14. Implementation Phases

### Phase 1: Core Identity and Tenancy Types

Target:

- define additive core types for tenant, subject, principal, external identity, node enrollment, and external session binding
- extend current security envelopes with optional tenant-aware metadata
- add validation tests

Deliverables:

- new `framework/core` type file
- unit tests for validation and matching semantics
- no storage migration yet

### Phase 2: Tenant-Scoped Persistence Schema

Target:

- add tenant-aware tables and schema migrations for identities, node enrollments, and session bindings

Deliverables:

- SQLite schema changes
- storage interfaces for tenants, subjects, external identities, node enrollments
- migration tests

### Phase 3: Gateway Principal Authentication

Target:

- require authenticated principals for gateway websocket roles

Deliverables:

- token validator wiring in Nexus startup
- connect-frame validation stripped of authority-bearing identity fields
- authenticated principal resolution on connect

### Phase 4: Node Enrollment and Challenge-Response

Target:

- replace self-asserted node registration with verified enrollment

Deliverables:

- pending enrollment flow
- public key persistence
- challenge-response handshake
- trust class assignment on server

### Phase 5: External Provider Identity Resolution

Target:

- resolve inbound provider identities to internal tenant-scoped subjects before session routing

Deliverables:

- external identity resolver interfaces
- Discord/Telegram/Webchat binding model
- restricted handling for unbound identities

### Phase 6: Session Ownership and Opaque Handles

Target:

- make sessions subject-owned and stop using exposed deterministic session keys as authority

Deliverables:

- session boundary schema changes
- opaque session handle generation
- internal routing lookup table

### Phase 7: Tenant-Aware Authorization Enforcement

Target:

- enforce tenant, ownership, and provider-binding checks on session send/attach/resume and capability invocation

Deliverables:

- updated `PolicyRequest` and authorization paths
- gateway enforcement on outbound and capability invoke
- authz tests for impersonation and cross-tenant denial

### Phase 8: Scoped Event Subscriptions and Admin Feeds

Target:

- replace raw broadcast/replay with authorized projections

Deliverables:

- session-scoped and admin-scoped subscriptions
- tenant filtering in event streaming
- audit coverage for subscription decisions

### Phase 9: Operator UX and Bootstrap Flows

Target:

- provide usable provisioning for tenants, users, nodes, and provider bindings

Deliverables:

- bootstrap admin flow
- node approval CLI updates
- status and inspection surfaces for identities, tenants, and bindings

### Phase 10: Hardening and Migration Cleanup

Target:

- remove compatibility fallbacks that preserve insecure behavior

Deliverables:

- delete self-declared actor identity paths
- require auth on websocket connections
- remove trust defaults for external provider sessions

## 15. Phase 1 Implementation Plan

Phase 1 is intentionally additive and low-risk.

Work items:

1. Add new identity/tenancy types to `framework/core`.
2. Extend `EventActor`, `SessionBoundary`, `NodeDescriptor`, and `NodeCredential` with optional metadata fields.
3. Add validation and ownership helper methods.
4. Add unit tests.
5. Do not change runtime behavior yet except for new helper availability.

## 16. Acceptance Criteria

Phase 1 is complete when:

- the repository contains a committed engineering spec for this work
- additive identity and tenancy types exist in `framework/core`
- tests validate the new type contracts
- no existing runtime callers break

## 17. Risks

- Adding tenant fields without immediate enforcement can create false confidence if the rollout stalls.
- Persisted stores will need coordinated migration before enforcement phases.
- External identity linking UX can become complex if introduced before a clear role model.

## 18. Recommended Next Step After Phase 1

Begin Phase 2 by making session and node persistence tenant-aware first. That creates the durable substrate needed before authentication and authorization are tightened in the live Nexus gateway.
