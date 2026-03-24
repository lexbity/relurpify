# Federated Mesh Protocol (FMP) Engineering Specification

> **Status:** Implemented but not yet fully end-to-end tested. The specification
> below describes the target design; the current implementation covers the
> protocol and framework layers but has not been validated across a multi-node
> production deployment.

## 1. Purpose

This document specifies a production-grade federated mesh protocol and the required node runtime assumptions for a system in which every node is both a client and a server, workloads run in sandboxed runtimes, and task execution may be resumed on a different node using portable context or state. The protocol is designed for independent meshes that selectively federate identity, discovery, routing, and resumable work transfer without collapsing into one shared control plane.

This is not an MVP document. It defines concrete protocol roles, objects, state machines, encryption model, federation boundaries, operational requirements, compatibility rules, and failure semantics.

## 2. Scope

In scope:
- node-to-node mesh protocol
- cross-mesh federation model
- node and runtime identities
- discovery and export model
- resumable task and context transfer
- end-to-end payload encryption between node runtimes
- trust bundles and authorization model
- leases, fencing, ownership transfer
- required runtime assumptions
- observability, failure handling, rollout, compatibility, and policy requirements

Out of scope:
- detailed language-level task checkpointing internals
- exact memory/context implementation inside application runtimes
- user-facing APIs and SDK ergonomics
- billing and settlement implementation details

## 3. Design goals

The system SHALL satisfy the following goals:

1. Preserve operational independence of each mesh.
2. Allow selective export of resumable services/task classes across federation boundaries.
3. Treat every node as both a service provider and consumer.
4. Run all untrusted workloads inside a sandboxed runtime.
5. Support task continuation via portable context/state rather than relying on live process migration.
6. Encrypt all transport links and support end-to-end encryption of resumable payloads between node runtimes.
7. Maintain a single active owner for any execution attempt.
8. Support explicit policy at federation boundaries.
9. Survive version skew, partitions, retries, and partial failures without duplicate ownership.
10. Keep routing aware of trust, policy, compatibility, and context-transfer constraints.

## 4. Non-goals

The system SHALL NOT assume:
- one shared scheduler across all meshes
- one global certificate authority
- unrestricted full-mesh trust
- process-level live migration with open sockets and host handles preserved
- transparent transfer of arbitrary in-memory language runtime state across incompatible versions
- inspection access to sealed task payloads by intermediate relays by default

## 5. Normative language

The keywords MUST, MUST NOT, REQUIRED, SHALL, SHALL NOT, SHOULD, SHOULD NOT, and MAY are to be interpreted as normative requirements.

## 6. System model

### 6.1 Core model

A deployment consists of one or more meshes. Each mesh is an independent operational domain with its own:
- scheduler and placement logic
- node membership
- rollout policy
- failure domain
- secrets issuance
- local policy engine

Meshes federate selectively by sharing:
- trust bundles
- exported service/task metadata
- boundary routing information
- federation authorization policy
- optional health summaries for exported endpoints

### 6.2 Node model

Every node runs:
- a trusted host-resident node agent
- trusted mesh control components as host processes
- one or more sandboxed workloads/tasks
- a mesh runtime endpoint responsible for node-to-node protocol handling

Each node acts as:
- a service endpoint for remote calls and handoffs
- a client for calling and resuming work on other nodes
- a control participant for membership, discovery, and state dissemination

### 6.3 Work model

Work is represented as a lineage of attempts.

- A **lineage** is the durable logical identity of a task or agent across resumes.
- An **attempt** is a concrete execution instance bound to a node runtime.
- A **context package** is a portable representation of resumable state.

The system does not require live process migration. Instead, a receiving node creates a new attempt from a validated context package under local policy.

## 7. Trust and security model

### 7.1 Trust rings

The system SHALL use three trust rings:

1. **Host/node trust ring**
   - node agent
   - local scheduler agent
   - federation gateway
   - secrets bootstrap
   - trusted host networking and storage control components

2. **Workload identity ring**
   - workload or service identities
   - task class identities
   - local authorization decisions for service-to-service traffic

3. **Sandboxed execution ring**
   - all untrusted workloads and tasks
   - no ambient host authority
   - mediated access to network, storage, secrets, and context import/export

### 7.2 Trust domains

Each mesh SHALL have its own trust domain.
Cross-mesh federation SHALL be based on explicit trust bundle exchange and explicit authorization policy. Trust SHALL NOT be transitive by default.

### 7.3 Endpoint definition for end-to-end encryption

For this system, the cryptographic ends are the node runtimes that terminate resumable task/context traffic.
Gateways, relays, mirrors, and discovery components SHALL NOT be considered end recipients for payload confidentiality unless explicit mediation mode is configured.

## 8. Runtime assumptions

### 8.1 Runtime placement assumptions

The trusted node agent and control components SHALL run outside the workload sandbox.
All untrusted tasks SHALL run inside a sandbox runtime.

### 8.2 Sandbox assumptions

The runtime SHALL support:
- per-task isolation
- bounded CPU and memory limits
- mediated filesystem access
- mediated network access
- explicit secrets injection
- stable task identity inside the runtime
- verifiable runtime version and compatibility class

### 8.3 Context assumptions

The runtime is assumed to support:
- export of resumable context as a typed, versioned object
- import of context into a new attempt
- validation of context class and schema version
- idempotent resume handling at the runtime API layer

This specification does not mandate how the application runtime internally models memory/context.
It only requires that portable context be emitted and consumed through the standard protocol objects defined here.

### 8.4 Host integration assumptions

A production implementation SHOULD use a host runtime stack with a trusted node agent and a sandboxed workload engine. A containerd-plus-sandbox approach is assumed to be a valid deployment model.

## 9. Architecture overview

### 9.1 Protocol layers

The system consists of two tightly coupled protocol layers:

1. **Federated mesh routing layer**
   - identity federation
   - service/task export
   - discovery
   - topology and boundary routing
   - transport security
   - federation policy
   - coarse admission signaling

2. **Continuation transfer layer**
   - lineage and attempt objects
   - context manifests and sealed payloads
   - leases and ownership transfer
   - handoff, accept, resume, commit, and fence semantics
   - provenance and receipts
   - payload-level encryption metadata

### 9.2 Control/data separation

The implementation SHALL separate:
- control traffic
- application service traffic
- resumable context/object transfer traffic
- observability traffic

These MAY share a transport substrate but SHALL be independently classified, rate-limited, and observable.

## 10. Node roles

A given binary stack MAY perform multiple roles on one node. The protocol defines the following logical roles:

### 10.1 Node Agent
Trusted host component. Responsible for:
- launching sandboxed tasks
- enforcing local resource limits
- exposing runtime capabilities
- health reporting
- log and metric streaming
- receiving assignment and termination orders

### 10.2 Runtime Endpoint
Trusted host component that terminates FMP traffic for resumable work.
Responsible for:
- receiving handoff offers
- verifying peer identities
- obtaining or unwrapping payload keys
- validating context manifests
- creating new attempts
- issuing resume receipts
- applying fences to prior attempts

### 10.3 Federation Gateway
Boundary component for cross-mesh traffic.
Responsible for:
- validating foreign trust bundles
- enforcing export/import policy
- exposing federated discovery entries
- forwarding sealed payload traffic
- optionally mediating payload inspection if explicit mediation mode is enabled

### 10.4 Scheduler / Placement Authority
Local mesh component responsible for:
- desired state
- placement
- resource accounting
- compatibility matching
- retry policy for stateless work
- ownership transfer authorization for resumable work

The scheduler SHALL remain local to a mesh. Federation SHALL expose interfaces between schedulers, not a merged global scheduler.

## 11. Naming and identity

### 11.1 Identity classes

The protocol SHALL define distinct identities for:
- Mesh
- Node
- Runtime Endpoint
- Gateway
- Workload/Service
- Lineage
- Attempt
- Context Object
- Exported Task Class

### 11.2 Stable naming

The implementation SHALL use globally qualified names to prevent collisions across meshes.
Recommended forms:
- `mesh://<trust-domain>/<export-class>`
- `node://<trust-domain>/<node-id>`
- `runtime://<trust-domain>/<node-id>/<runtime-id>`
- `lineage://<trust-domain>/<tenant>/<lineage-id>`
- `context://<trust-domain>/<hash>`

### 11.3 Lineage identity

Lineage identity SHALL remain stable across resumes and across meshes.
Attempt identity SHALL be unique per concrete execution instance.
No two active attempts SHALL share the same active ownership token.

## 12. Capability model

Every resumable task class SHALL declare a capability envelope that constrains what a resumed attempt MAY do.
The capability envelope SHALL include at least:
- network egress class
- storage access class
- secret scopes
- CPU/memory/time budgets
- child-task spawning permission
- onward context export permission
- permitted task classes for resume
- observability export level

Tasks SHALL NOT inherit ambient node privilege.
Transferred work SHALL execute only with the capability envelope attached to the lineage or the stricter local projection of it.

## 13. Runtime compatibility classes

Each runtime endpoint SHALL advertise a compatibility descriptor including:
- runtime version
- sandbox version
- ABI/schema compatibility set
- supported context classes
- supported encryption suite set
- supported transport set
- network policy class
- filesystem mode class
- maximum supported context size
- attestation profile

Routing and placement MUST consider compatibility before handoff acceptance.

## 14. Export model

### 14.1 Exported classes

Meshes SHALL export only selected task or service classes.
An export descriptor SHALL include:
- export name
- accepted caller trust domains
- accepted source identities
- accepted context classes
- maximum context size
- required runtime compatibility classes
- sensitivity classes accepted
- allowed transport paths (direct or gateway)
- health/admission summaries

### 14.2 Discovery modes

The protocol SHALL support:
- mirrored discovery entries
- gateway-routed discovery entries

A mesh MAY expose remote exports as locally addressable discovery objects, but MUST preserve namespace boundaries.

## 15. Routing model

### 15.1 Routing constraints

Routing MUST account for:
- reachability
- trust-domain compatibility
- export policy
- destination runtime compatibility
- context class acceptance
- sensitivity rules
- current admission state
- size and transfer mode
- locality and latency

### 15.2 Route decision

A route is valid only if all of the following hold:
- destination export exists
- source identity is authorized
- destination accepts the context class and size
- destination runtime compatibility is satisfied
- destination path is reachable
- destination admission allows new handoff
- encryption recipient binding can be satisfied

### 15.3 Federation defaults

Traffic SHALL remain local by default unless a remote export is explicitly chosen or local policy explicitly allows remote failover.
Automatic cross-mesh failover SHALL be opt-in.

## 16. Transport requirements

### 16.1 Transport security

All node-to-node and gateway-to-gateway transport channels MUST be authenticated and encrypted.
The transport layer MUST provide:
- peer authentication
- confidentiality
- integrity
- replay protection
- session expiration and rotation

### 16.2 Transport substrate

The protocol MAY be implemented over any reliable framed transport that supports bidirectional streams and authenticated peer sessions.
The implementation MUST support:
- request/response control messages
- streaming chunk transfer for large context payloads
- cancellation
- per-stream flow control
- backpressure signaling

## 17. Encryption model

### 17.1 Encryption scopes

The protocol SHALL implement three encryption scopes:

1. **Link encryption**
   Protects every peer session.

2. **Object encryption**
   Protects context and checkpoint objects at rest and in motion.

3. **End-to-end payload encryption**
   Protects resumable payloads from the source runtime endpoint to the authorized destination runtime endpoint.

### 17.2 Visible vs sealed fields

The protocol SHALL separate messages into:
- **routable envelope fields**, visible to the mesh
- **sealed payload fields**, visible only to the destination runtime endpoint

Visible envelope fields SHOULD include only what is needed for routing and policy:
- source identity
- destination identity or export target
- message type
- object reference or ciphertext reference
- size and chunking metadata
- policy labels
- handoff token
- expiry
- anti-replay nonce
- crypto suite identifiers

Sealed payload fields SHALL include sensitive execution material:
- serialized context
- task inputs
- capability envelope details beyond routable labels
- private provenance
- checkpoint data

### 17.3 Recipient binding

A sealed payload MUST be encrypted to the destination runtime endpoint identity or to a key wrapping scheme bound to that identity.
It MUST be possible to verify that the intended recipient identity matches the accepted handoff target.

### 17.4 Handoff-aware keying

Because resumed work may later move again, the implementation MUST support re-targeting on ownership change by at least one of:
- decrypt and re-encrypt by the current owner
- cryptographic key rewrap for the new runtime endpoint
- fresh grant from a lineage key authority

### 17.5 Mediation mode

Cross-mesh gateways SHALL forward sealed payloads without decryption by default.
A deployment MAY enable explicit mediation mode for selected traffic classes, in which case the gateway becomes a trusted inspection and re-encryption endpoint. This mode SHALL be disabled by default.

### 17.6 Stored object encryption

Context and checkpoint objects stored in blob/object stores MUST be encrypted independently from transport sessions.
Content-addressed references MAY be used, but object confidentiality MUST NOT rely solely on opaque object names.

## 18. Core protocol objects

### 18.1 NodeDescriptor
Fields:
- node_id
- trust_domain
- runtime_endpoints[]
- capacity_summary
- locality
- compatibility_classes[]
- admission_state
- exported_classes[]
- attestation_summary
- expiration
- signature

### 18.2 RuntimeDescriptor
Fields:
- runtime_id
- node_id
- runtime_version
- sandbox_version
- supported_context_classes[]
- supported_encryption_suites[]
- compatibility_class
- max_context_size
- max_concurrent_resumes
- attestation_claims
- signature

### 18.3 ExportDescriptor
Fields:
- export_name
- trust_domain
- accepted_source_domains[]
- accepted_identities[]
- accepted_context_classes[]
- max_context_size
- sensitivity_limit
- required_compatibility_classes[]
- route_mode
- admission_summary
- signature

### 18.4 LineageRecord
Fields:
- lineage_id
- tenant_id
- parent_lineage_id (optional)
- task_class
- context_class
- current_owner_attempt
- current_owner_runtime
- capability_envelope_ref
- sensitivity_class
- allowed_federation_targets[]
- created_at
- updated_at
- lineage_version

### 18.5 AttemptRecord
Fields:
- attempt_id
- lineage_id
- runtime_id
- state
- lease_id
- lease_expiry
- start_time
- last_progress_time
- fenced (bool)
- previous_attempt_id (optional)

### 18.6 ContextManifest
Fields:
- context_id
- lineage_id
- attempt_id
- context_class
- schema_version
- size_bytes
- chunk_count
- content_hash
- sensitivity_class
- ttl
- object_refs[]
- encryption_mode
- recipient_set
- creation_time
- signature

### 18.7 SealedContext
Fields:
- envelope_version
- context_manifest_ref
- cipher_suite
- recipient_bindings[]
- ciphertext_chunks[] or external_object_refs[]
- integrity_tag
- replay_protection_fields

### 18.8 LeaseToken
Fields:
- lease_id
- lineage_id
- attempt_id
- issuer
- issued_at
- expiry
- fencing_epoch
- signature

### 18.9 HandoffOffer
Fields:
- offer_id
- lineage_id
- source_attempt_id
- source_runtime_id
- destination_export
- context_manifest_ref
- context_class
- sensitivity_class
- requested_capability_projection
- lease_token
- expiry
- trace_context
- signature

### 18.10 HandoffAccept
Fields:
- offer_id
- destination_runtime_id
- accepted_context_class
- accepted_capability_projection
- rewrap_request
- provisional_attempt_id
- expiry
- signature

### 18.11 ResumeCommit
Fields:
- lineage_id
- old_attempt_id
- new_attempt_id
- destination_runtime_id
- receipt_ref
- commit_time
- signature

### 18.12 FenceNotice
Fields:
- lineage_id
- attempt_id
- fencing_epoch
- reason
- issuer
- signature

### 18.13 ResumeReceipt
Fields:
- lineage_id
- attempt_id
- runtime_id
- imported_context_id
- start_time
- compatibility_verified
- capability_projection_applied
- status
- signature

## 19. Attempt lifecycle and state machine

### 19.1 Attempt states

A runtime attempt SHALL be in exactly one of the following states:
- CREATED
- ADMITTED
- RUNNING
- CHECKPOINTING
- HANDOFF_OFFERED
- HANDOFF_ACCEPTED
- RESUME_PENDING
- COMMITTED_REMOTE
- FENCED
- COMPLETED
- FAILED
- ORPHANED

### 19.2 Ownership rule

At any moment, at most one attempt MAY be the active owner for a lineage execution token.
Ownership transfer MUST use an explicit lease and fencing model.

### 19.3 Handoff state transitions

1. Source attempt enters CHECKPOINTING.
2. Source emits ContextManifest and SealedContext.
3. Source sends HandoffOffer.
4. Destination validates policy and compatibility.
5. Destination replies with HandoffAccept.
6. Destination imports or fetches sealed context.
7. Destination creates a new attempt in RESUME_PENDING.
8. Destination decrypts and validates context.
9. Destination starts new attempt and emits ResumeReceipt.
10. Control plane or source emits ResumeCommit.
11. Prior attempt is fenced.
12. Prior attempt transitions to FENCED or COMPLETED.
13. New attempt transitions to RUNNING.

## 20. Federation handshake

### 20.1 Establishment

Before traffic is exchanged across meshes, the following MUST occur:
- trust bundle exchange
- gateway or runtime endpoint authentication
- route/export synchronization
- policy agreement on permitted source identities and export classes

### 20.2 Boundary policy

Boundary policy SHALL answer at least:
- which source trust domains are accepted
- which identities may call which exports
- whether direct runtime-to-runtime paths are allowed
- whether traffic must traverse gateways
- whether mediation mode is allowed
- what retry and timeout policies apply at the boundary

## 21. Discovery and advertisement

### 21.1 Advertisement frequency

Nodes and gateways SHALL advertise descriptors with bounded TTLs and signatures.
Receivers MUST treat expired advertisements as invalid for new placement or handoff.

### 21.2 Health summaries

Export health summaries MAY be shared across federation boundaries.
Detailed internal node membership SHOULD remain local.

### 21.3 Namespace isolation

Discovery import MUST preserve source trust-domain qualification to avoid collisions.
No imported export may silently shadow a local export of the same unqualified name.

## 22. Admission control and backpressure

### 22.1 Required controls

Every runtime endpoint and gateway SHALL implement:
- per-peer connection limits
- per-export concurrency limits
- max context size limits
- transfer bandwidth limits
- queue depth limits
- refusal signaling
- deadline enforcement

### 22.2 Refusal reasons

The protocol SHALL support structured refusal reasons including:
- unauthorized
- incompatible runtime
- unsupported context class
- context too large
- admission closed
- sensitivity not allowed
- destination overloaded
- invalid handoff token
- expired offer

## 23. Scheduling and placement assumptions

### 23.1 Local scheduler authority

Placement remains a local mesh concern.
A remote mesh MAY expose coarse admission and compatibility hints, but SHALL NOT expose its internal scheduler internals as part of federation.

### 23.2 Placement inputs

A production scheduler MUST consider:
- CPU and memory capacity
- locality and latency
- runtime compatibility class
- network intensity
- filesystem intensity
- sensitivity class
- context size and restore cost
- export policy
- destination admission state

## 24. Failure handling

### 24.1 Crash of source during handoff

If the source crashes after emitting a HandoffOffer but before commit, the control plane SHALL determine ownership by lease validity and ResumeReceipt presence.
Destination attempts SHALL remain provisional until committed or lease-expiry rules promote them.

### 24.2 Crash of destination during resume

If the destination crashes before emitting ResumeReceipt, ownership SHALL remain with the source until lease rules and control-plane reconciliation say otherwise.

### 24.3 Partition handling

The system SHALL distinguish:
- eventually consistent discovery and health caches
- strongly coordinated ownership, leases, and durable lineage state

Partition behavior SHALL be explicit. A partitioned node MUST NOT assume ownership if it cannot verify its lease continuity.

### 24.4 Duplicate handoff protection

The protocol MUST detect and suppress duplicate resume attempts using:
- lineage_id
- offer_id
- lease_id
- fencing_epoch
- destination provisional_attempt_id

## 25. Consistency requirements

### 25.1 Strong coordination required for
- active attempt ownership
- lease issuance and fencing epochs
- durable lineage state
- commit of resumed ownership
- revocation of prior ownership

### 25.2 Eventual consistency allowed for
- endpoint caches
- coarse health summaries
- topology hints
- non-critical metric dissemination

## 26. Versioning and compatibility

### 26.1 Protocol versions

Every message SHALL include a protocol version.
Implementations MUST support version negotiation for minor versions and MAY reject unsupported major versions.

### 26.2 Context schema versions

Context classes SHALL be independently versioned.
A destination MUST verify support for the declared context schema version before accepting resume.

### 26.3 Runtime skew

Deployments MUST define a compatibility window for runtime versions and context schema versions.
Old runtimes MAY continue to serve stateless traffic while being ineligible for resumable context import.

## 27. Observability and audit

### 27.1 Required identifiers

All logs, traces, metrics, and audit events related to resumable work SHALL include:
- trust_domain
- lineage_id
- attempt_id
- runtime_id
- node_id
- offer_id when applicable
- lease_id when applicable

### 27.2 Audit events

The system SHALL emit signed or tamper-evident audit events for at least:
- export registration
- federation trust changes
- handoff offer
- handoff accept
- key rewrap or decryption grant
- resume commit
- fence issuance
- unauthorized attempt
- policy denial

### 27.3 Payload logging prohibition

Sealed payload contents MUST NOT be logged by transport, gateway, or routing components.
Debug tooling SHALL default to redacted views.

## 28. Policy model

### 28.1 Policy dimensions

Policy SHALL be enforceable on:
- identity
- trust domain
- export class
- context class
- sensitivity class
- capability envelope
- route type
- mediation permission
- onward transfer permission

### 28.2 Local projection

A destination mesh MAY project a stricter local capability envelope than the source requested.
A destination mesh SHALL NOT broaden capability during resume.

## 29. Object storage and transfer

### 29.1 Transfer modes

The protocol SHALL support:
- inline transfer for small payloads
- chunked streamed transfer for medium payloads
- content-addressed object references for large payloads

### 29.2 Integrity

Every context object MUST carry or reference a cryptographic integrity digest.
The receiver MUST verify integrity before resume.

### 29.3 TTL and cleanup

Ephemeral context objects MUST have TTLs.
Expired or superseded objects SHOULD be garbage-collected once no valid ownership path references them.

## 30. Security requirements

### 30.1 Minimum security requirements

A compliant implementation MUST provide:
- authenticated transport encryption on all mesh links
- payload-level encryption for resumable context across trust boundaries
- strong node and runtime identity
- attestation or equivalent runtime integrity evidence
- capability-based authorization
- lease and fencing semantics
- replay protection
- tamper-evident audit trail

### 30.2 Key management requirements

The implementation MUST support:
- key rotation
- recipient-bound wrapping
- revocation on ownership change when feasible
- expiration of grants
- separation of transport keys and object/payload keys

## 31. Operational requirements

### 31.1 Rollouts

Protocol and runtime rollouts SHALL support canary and staged deployment.
A deployment MUST be able to prevent new resumable imports to incompatible versions while continuing local execution of existing attempts.

### 31.2 Capacity planning

Operators SHOULD plan independently for:
- control-plane traffic
- service traffic
- context transfer bandwidth
- object storage growth
- encrypted payload overhead
- checkpoint restore latency

### 31.3 Blast-radius controls

Cross-mesh failover and resume SHOULD be bounded by explicit budgets and rate limits to prevent one mesh incident from cascading into another.

## 32. Baseline protocol flows

### 32.1 Export and discovery flow

1. Mesh B creates ExportDescriptor for resumable class X.
2. Mesh B publishes descriptor through its federation boundary.
3. Mesh A imports the descriptor into its federated discovery cache.
4. Local schedulers and runtimes in Mesh A may now target export X subject to policy.

### 32.2 Handoff flow

1. Source attempt checkpoints context.
2. Source builds ContextManifest.
3. Source seals payload for destination runtime class or specific runtime.
4. Source sends HandoffOffer.
5. Destination validates export, policy, compatibility, and admission.
6. Destination replies HandoffAccept.
7. Source or object store exposes SealedContext reference.
8. Destination fetches or receives payload.
9. Destination decrypts, validates, and creates new attempt.
10. Destination emits ResumeReceipt.
11. Ownership is committed.
12. Prior attempt is fenced.

### 32.3 Reject flow

1. Destination cannot accept.
2. Destination returns structured refusal reason.
3. Source remains owner.
4. Scheduler may retry alternate routes according to resumable work policy.

## 33. Conformance profile

A system is conformant with this specification only if it:
- implements distinct mesh and continuation layers
- preserves local scheduler authority within each mesh
- supports exported resumable task classes
- implements leases and fencing
- implements recipient-bound payload encryption between node runtimes
- separates routable metadata from sealed payloads
- supports re-targeting on legitimate ownership transfer
- enforces compatibility and policy before resume

## 34. Recommended defaults

The recommended default deployment profile is:
- independent meshes with distinct trust domains
- gateway-mediated federation for discovery and policy
- direct runtime-to-runtime data paths where reachable and allowed
- sealed payload pass-through at gateways
- local smart-client routing instead of mandatory sidecars everywhere
- sandboxed workloads with trusted host agents
- checkpoint/resume semantics instead of live migration
- explicit cross-mesh failover budgets

## 35. Summary

This specification defines a federated continuation mesh in which routing is federation-aware, resumable execution is represented as lineage plus portable context, and end-to-end confidentiality is enforced between node runtimes. The design keeps meshes operationally independent, treats payload confidentiality and routing visibility as separate concerns, and requires explicit ownership transfer using leases and fencing instead of informal retries or ambiguous failover behavior.

