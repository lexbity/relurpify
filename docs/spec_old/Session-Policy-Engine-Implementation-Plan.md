# Session Policy Engine Implementation Plan

**Prepared:** 2026-03-10  
**Status:** Proposed implementation plan  
**Scope:** Manifest-associated session policy, normalized policy rules, and end-to-end session authorization enforcement

## 1. Objective

Add a first-class `session_policies` manifest surface and route all session authorization through a single policy engine that consumes manifest-derived policy. This closes the current gap where:

- session access is not policy-evaluated
- `DefaultRouter.Authorize()` does not call the policy engine
- the Nexus gateway path does not call `Authorize()` at all
- `core.PolicyRule` exists but is not used for runtime decisions

The target state is one canonical manifest-to-policy pipeline:

1. Manifest loads.
2. Existing policy surfaces plus `session_policies` compile into normalized `PolicyRule` values.
3. One policy engine evaluates requests for capabilities, provider activation, and session operations.
4. Every session-sensitive path calls the engine with structured request data.

## 2. Current State

### 2.1 Manifest surfaces

Current manifest policy surfaces live primarily in [`framework/core/agent_spec.go`](/home/lex/Public/Relurpify/framework/core/agent_spec.go):

- `ToolExecutionPolicy`
- `CapabilityPolicies`
- `ExposurePolicies`
- `InsertionPolicies`
- `GlobalPolicies`
- `ProviderPolicies`

The top-level agent manifest in [`framework/manifest/manifest.go`](/home/lex/Public/Relurpify/framework/manifest/manifest.go) also includes `Spec.Policies`, which overlaps conceptually with agent-level global policy.

### 2.2 Runtime policy

The current policy implementation in [`framework/authorization/policy_engine.go`](/home/lex/Public/Relurpify/framework/authorization/policy_engine.go) is a compatibility adapter over `PermissionManager`. It does not load or evaluate `PolicyRule` conditions.

### 2.3 Session enforcement gap

The current session router in [`framework/middleware/session/session.go`](/home/lex/Public/Relurpify/framework/middleware/session/session.go):

- exposes `Policy PolicyEngine`
- does not use it in `Authorize()`
- only checks actor ID string equality

The Nexus gateway path in [`app/nexus/main.go`](/home/lex/Public/Relurpify/app/nexus/main.go) and [`app/nexus/gateway/server.go`](/home/lex/Public/Relurpify/app/nexus/gateway/server.go) uses `sessionStore.GetBoundary()` directly for outbound message routing and does not call `Authorize()`.

## 3. Design Goals

- Make session policy manifest-defined and framework-enforced.
- Avoid a second, drifting policy system beside existing manifest fields.
- Compile all policy surfaces into one normalized rule set.
- Keep backward compatibility with existing manifests that only use allow/ask/deny policy.
- Make session authorization rich enough to express actor, ownership, channel, trust, and operation.
- Ensure the Nexus gateway cannot bypass policy by directly looking up session boundaries.

## 4. Proposed Data Model

### 4.1 Add session policy to `AgentRuntimeSpec`

Extend [`framework/core/agent_spec.go`](/home/lex/Public/Relurpify/framework/core/agent_spec.go):

```go
type AgentRuntimeSpec struct {
    ...
    SessionPolicies []SessionPolicy `yaml:"session_policies,omitempty" json:"session_policies,omitempty"`
}
```

This keeps session policy alongside the other runtime policy surfaces that are already manifest-driven.

### 4.2 Add session policy types to `framework/core`

Add new types in a new file such as `framework/core/session_policy_types.go`.

Recommended types:

```go
type SessionOperation string

const (
    SessionOperationAttach  SessionOperation = "attach"
    SessionOperationSend    SessionOperation = "send"
    SessionOperationResume  SessionOperation = "resume"
    SessionOperationInspect SessionOperation = "inspect"
    SessionOperationClose   SessionOperation = "close"
)

type SessionSelector struct {
    Partitions        []string        `yaml:"partitions,omitempty" json:"partitions,omitempty"`
    ChannelIDs        []string        `yaml:"channel_ids,omitempty" json:"channel_ids,omitempty"`
    Scopes            []SessionScope  `yaml:"scopes,omitempty" json:"scopes,omitempty"`
    TrustClasses      []TrustClass    `yaml:"trust_classes,omitempty" json:"trust_classes,omitempty"`
    Operations        []SessionOperation `yaml:"operations,omitempty" json:"operations,omitempty"`
    ActorKinds        []string        `yaml:"actor_kinds,omitempty" json:"actor_kinds,omitempty"`
    ActorIDs          []string        `yaml:"actor_ids,omitempty" json:"actor_ids,omitempty"`
    RequireOwnership  *bool           `yaml:"require_ownership,omitempty" json:"require_ownership,omitempty"`
    AuthenticatedOnly *bool           `yaml:"authenticated_only,omitempty" json:"authenticated_only,omitempty"`
}

type SessionPolicy struct {
    ID          string               `yaml:"id" json:"id"`
    Name        string               `yaml:"name" json:"name"`
    Priority    int                  `yaml:"priority,omitempty" json:"priority,omitempty"`
    Enabled     bool                 `yaml:"enabled" json:"enabled"`
    Selector    SessionSelector      `yaml:"selector" json:"selector"`
    Effect      AgentPermissionLevel `yaml:"effect" json:"effect"`
    Approvers   []string             `yaml:"approvers,omitempty" json:"approvers,omitempty"`
    ApprovalTTL string               `yaml:"approval_ttl,omitempty" json:"approval_ttl,omitempty"`
    Reason      string               `yaml:"reason,omitempty" json:"reason,omitempty"`
}
```

If the team wants to avoid a second effect vocabulary, `SessionPolicy` can compile directly into `PolicyRule` and use `PolicyEffect` instead of `AgentPermissionLevel`.

### 4.3 Extend policy request modeling

Current [`core.PolicyRequest`](/home/lex/Public/Relurpify/framework/core/policy_types.go) is capability-oriented. It should be expanded or split so session authorization has first-class fields.

Preferred approach: extend `PolicyRequest` with a target category plus optional session context.

```go
type PolicyTarget string

const (
    PolicyTargetCapability PolicyTarget = "capability"
    PolicyTargetProvider   PolicyTarget = "provider"
    PolicyTargetSession    PolicyTarget = "session"
)

type SessionPolicyContext struct {
    Operation      SessionOperation
    Partition      string
    ChannelID      string
    Scope          SessionScope
    SessionOwnerID string
    IsOwner        bool
}

type PolicyRequest struct {
    Target         PolicyTarget
    Actor          EventActor
    ActorAuthn     PolicyActorAuthn
    CapabilityID   string
    CapabilityName string
    ProviderKind   ProviderKind
    TrustClass     TrustClass
    RiskClasses    []RiskClass
    ChannelID      string
    SessionID      string
    Session        *SessionPolicyContext
    Timestamp      time.Time
}
```

This avoids forcing the session router to fake a capability-like request.

## 5. Policy Engine Architecture

### 5.1 Canonical rule pipeline

The engine should operate on normalized `PolicyRule` values, regardless of how policy entered the system.

Compilation pipeline:

1. Read manifest.
2. Validate raw manifest fields.
3. Compile:
   - `ToolExecutionPolicy`
   - `CapabilityPolicies`
   - `ProviderPolicies`
   - `GlobalPolicies`
   - new `SessionPolicies`
4. Produce a canonical rule set in deterministic priority order.
5. Load the compiled rules into the engine.

This preserves one evaluator and one audit surface.

### 5.2 Keep compatibility fallback

The existing `ManifestPolicyEngine` can evolve into a real rule engine with compatibility fallback:

1. Evaluate explicit normalized rules first.
2. If no rule matches, fall back to current default policy behavior.
3. Preserve current builtin/workspace-trusted shortcuts only if that remains the intended product rule.

That avoids breaking current manifests while making `PolicyRule` meaningful.

### 5.3 Rule evaluation semantics

The initial evaluator should support only the subset needed for current manifest surfaces and session policy.

Phase 1 rule matching support:

- actor kind / ID
- capability ID / name
- provider kind
- trust class
- risk class threshold
- session operation
- session partition / channel / scope
- session ownership

Deferred unless needed immediately:

- time windows
- rate limits
- external approver routing

If unsupported rule fields remain in the schema, the loader should reject them or mark them unsupported explicitly. Silent ignore is not acceptable.

## 6. Manifest Semantics and Precedence

### 6.1 Source of truth

Manifest policy should be authoritative. Runtime code should not make independent policy decisions after the engine is introduced.

### 6.2 Precedence order

Recommended precedence:

1. Explicit session/capability/provider rules, highest priority first
2. Compiled legacy per-surface manifest rules
3. Compatibility default policy fallback

### 6.3 Validation rules

Add validation at manifest load time for:

- duplicate session policy IDs
- invalid or empty selectors
- unsupported operations
- conflicting equal-priority rules with incompatible effects
- malformed ownership/authentication requirements
- invalid mixes of session-only selectors with capability-only selectors if a shared `PolicyRule` surface is used

The goal is to fail manifest loading, not discover invalid policy during live execution.

## 7. Session Authorization API Changes

### 7.1 Replace string-only authorization input

Current API:

```go
Authorize(ctx context.Context, actorID string, boundary *core.SessionBoundary) error
```

This is too narrow for policy-based evaluation.

Recommended replacement:

```go
type AuthorizationRequest struct {
    Actor         core.EventActor
    Authenticated bool
    Operation     core.SessionOperation
    Boundary      *core.SessionBoundary
}

Authorize(ctx context.Context, req AuthorizationRequest) error
```

### 7.2 Router behavior

`DefaultRouter.Authorize()` should:

1. Validate boundary presence and partition.
2. Derive ownership from `boundary.ActorID` and `req.Actor.ID`.
3. Build `core.PolicyRequest{Target: PolicyTargetSession, ...}`.
4. Call `Policy.Evaluate(...)` when configured.
5. Map decisions:
   - `allow` -> return nil
   - `deny` -> return `ErrSessionBoundaryViolation`
   - `require_approval` -> either invoke HITL if supported here, or return a structured approval-required error
6. Optionally preserve the existing actor-ID equality check as a default fallback when no policy engine is set.

## 8. Nexus Gateway Wiring Changes

### 8.1 Remove direct boundary lookup as authorization

Current flow in [`app/nexus/main.go`](/home/lex/Public/Relurpify/app/nexus/main.go):

- accept `session_key`
- call `sessionStore.GetBoundary(sessionKey)`
- send outbound message

This must change. Boundary lookup is data retrieval, not authorization.

### 8.2 Add an authorization-aware outbound path

Introduce a service boundary in the gateway layer, for example:

```go
type SessionAuthorizer interface {
    ResolveAndAuthorize(ctx context.Context, req OutboundSessionRequest) (*core.SessionBoundary, error)
}
```

Where `OutboundSessionRequest` includes:

- session key
- actor identity from websocket connection
- authentication result
- requested operation (`send`)

`HandleOutboundMessage` should receive the connection actor and authorize against the router before sending.

### 8.3 Bind actor identity to the connection

The websocket server currently uses `connectFrame.Role` and optional bearer token validation, but it does not bind a durable actor identity to outbound requests.

Implementation steps:

1. Define a connection principal model.
2. Populate it from:
   - bearer token validation result
   - node identity
   - channel/operator identity
3. Store the principal in connection state.
4. Pass the principal into outbound message and future session operations.

Without this, session policy will still be evaluating weak identity data.

## 9. Manifest Loader and Compiler Changes

### 9.1 Extend manifest structs

Add `SessionPolicies` to:

- [`framework/core/agent_spec.go`](/home/lex/Public/Relurpify/framework/core/agent_spec.go)
- overlay/merge helpers in [`framework/core/agent_spec_overlay.go`](/home/lex/Public/Relurpify/framework/core/agent_spec_overlay.go)
- skill manifest equivalents in [`framework/manifest/skill_manifest.go`](/home/lex/Public/Relurpify/framework/manifest/skill_manifest.go) if skills are allowed to contribute session policy

### 9.2 Compiler package

Add a dedicated compiler package or file set, for example:

- `framework/authorization/policy_compile.go`
- `framework/authorization/policy_match.go`

Responsibilities:

- compile manifest policy surfaces into normalized rules
- sort rules by priority
- validate unsupported constructs
- annotate compiled rules with origin metadata for debugging

### 9.3 Rule provenance

Each compiled rule should retain provenance, for example:

- manifest path
- source surface (`session_policies`, `provider_policies`, `default_tool_policy`)
- source key/index

This is important for audits and for explaining why a rule matched.

## 10. Five-Phase Migration Strategy

### Phase 1: Schema and Type Foundation

**Goal:** introduce the session policy surface without changing runtime behavior.

**Implementation tasks**

- add `SessionOperation`, `SessionSelector`, and `SessionPolicy` to `framework/core`
- add validation helpers for session selectors and policies
- extend `AgentRuntimeSpec` with `SessionPolicies`
- extend `AgentSpecOverlay` merge/clone logic for `SessionPolicies`
- extend skill manifest parsing and validation if skills are allowed to narrow session behavior
- add initial unit coverage for validation and merge semantics

**Expected repository touchpoints**

- `framework/core/session_policy_types.go`
- `framework/core/agent_spec.go`
- `framework/core/agent_spec_overlay.go`
- `framework/manifest/skill_manifest.go`
- corresponding tests

**Exit criteria**

- manifests and skills can express `session_policies`
- invalid session policy declarations fail validation
- no runtime authorization behavior changes yet

**Status**

- In progress: initial schema and validation wiring is the first implementation slice to land

### Phase 2: Rule Compilation and Canonical Policy Loading

**Goal:** make manifest policy compile into one normalized rule set.

**Implementation tasks**

- add a manifest-to-rule compiler in `framework/authorization`
- compile existing policy surfaces plus `session_policies` into canonical `PolicyRule` values
- define deterministic precedence and priority sorting
- attach provenance metadata to compiled rules for audit/debug output
- reject unsupported rule constructs explicitly at load time

**Expected repository touchpoints**

- `framework/authorization/policy_compile.go`
- `framework/authorization/policy_engine.go`
- manifest-loading/runtime bootstrap paths

**Exit criteria**

- compiled rules exist as the canonical in-memory representation
- `ManifestPolicyEngine` loads normalized rules before fallback behavior
- compatibility with current manifests is preserved

### Phase 3: Unified Engine Evaluation for Capability and Provider Authorization

**Goal:** make non-session enforcement paths use the same compiled-rule evaluator.

**Implementation tasks**

- extend `PolicyRequest` to model target category and richer context
- update capability invocation to use normalized rule matches
- update provider activation to use the same evaluator and decision mapping
- preserve current HITL and deny behavior under legacy manifests
- emit policy-evaluation audit events with matched rule provenance

**Expected repository touchpoints**

- `framework/core/policy_types.go`
- `framework/capability/capability_registry.go`
- `app/relurpish/runtime/providers.go`
- authorization tests and audit/event code

**Exit criteria**

- capability and provider authorization share one policy-evaluation path
- matched-rule reasoning is inspectable in tests and logs

### Phase 4: Session Authorization API and Nexus Enforcement

**Goal:** eliminate session-key-based bypasses and enforce policy on real session operations.

**Implementation tasks**

- replace `Authorize(ctx, actorID, boundary)` with structured authorization input
- add session-specific policy request context including operation, ownership, and authentication
- update `DefaultRouter.Authorize()` to invoke the policy engine
- bind gateway connections to a principal model rather than an unstructured role string
- require authorization before outbound send, resume, inspect, or similar session actions
- stop treating `sessionStore.GetBoundary()` as authorization

**Expected repository touchpoints**

- `framework/middleware/session/session.go`
- `app/nexus/gateway/server.go`
- `app/nexus/main.go`
- session and Nexus integration tests

**Exit criteria**

- session operations are denied or approved through the engine
- session keys alone no longer grant authority
- Nexus outbound routing cannot bypass policy

### Phase 5: Consolidation, Cleanup, and Follow-on Features

**Goal:** remove transitional duplication and prepare for advanced policy features.

**Implementation tasks**

- deprecate or collapse overlapping manifest policy fields where appropriate
- remove redundant inline policy checks once the engine is authoritative
- decide whether top-level manifest `Spec.Policies` should remain or be folded into agent-level policy
- add operator-facing diagnostics for matched rules and approval reasons
- evaluate follow-on support for time windows, rate limits, and policy reload

**Expected repository touchpoints**

- manifest package
- runtime bootstrap
- UI/inspection surfaces
- docs and migration notes

**Exit criteria**

- one policy model is clearly authoritative
- transitional checks are removed or explicitly retained with rationale
- advanced policy work has a stable base

## 11. Testing Plan

### 11.1 Core validation tests

Add tests for:

- `SessionPolicy.Validate()`
- selector validation
- operation enum validation
- conflict detection

### 11.2 Compiler tests

Add tests that prove:

- `session_policies` compile into normalized rules
- existing manifest policy fields compile correctly
- priority ordering is deterministic
- provenance metadata is retained

### 11.3 Engine tests

Add tests for:

- session owner allowed by rule
- non-owner denied
- approval-required result for cross-trust session access
- fallback behavior when no explicit rule matches

### 11.4 Router tests

Extend [`framework/middleware/session/router_test.go`](/home/lex/Public/Relurpify/framework/middleware/session/router_test.go) to cover:

- policy engine allow
- policy engine deny
- policy engine approval-required
- fallback actor match behavior with no engine

### 11.5 Nexus integration tests

Add tests covering:

- outbound websocket message denied when actor is not authorized for session
- authorized actor can send
- session key alone is insufficient without matching principal

## 12. Risks and Non-Goals

### Risks

- Keeping both top-level manifest `Policies` and agent-level `GlobalPolicies` may continue to confuse precedence unless normalized aggressively.
- Adding session policy without stronger actor identity in Nexus will create a false sense of security.
- Supporting too many `PolicyRule` features before real call sites exist will expand scope without improving security.

### Non-goals for the first iteration

- hot reload of policy files
- distributed or external policy backends
- advanced time-window and rate-limit enforcement unless a concrete caller needs them
- general ABAC/Rego-style policy language

## 13. Recommended Work Breakdown

1. Add `SessionOperation`, `SessionSelector`, and `SessionPolicy` core types.
2. Extend `AgentRuntimeSpec`, overlays, and manifest validation.
3. Implement manifest-to-rule compilation with provenance.
4. Upgrade `ManifestPolicyEngine` to evaluate compiled rules before fallback.
5. Replace `Authorize(actorID, boundary)` with structured session authorization input.
6. Wire router authorization into Nexus outbound session flows.
7. Add connection principal modeling for gateway clients.
8. Add unit and integration coverage proving session keys do not bypass policy.

## 14. Exit Criteria

This work is complete when all of the following are true:

- manifests can declare `session_policies`
- session policy is validated at manifest load time
- compiled rules are the canonical runtime policy representation
- capability, provider, and session authorization all flow through one engine
- Nexus outbound session operations call authorization before using session boundaries
- a session key by itself is no longer sufficient to act on a session
- `core.PolicyRule` is used in real enforcement paths rather than remaining inert
