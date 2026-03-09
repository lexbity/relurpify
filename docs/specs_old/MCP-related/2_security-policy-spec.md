# Capability Security And Policy Engineering Specification

## Status

Draft

## Goal

Define the security model for local and remote capabilities so that MCP and MCP-like integrations cannot bypass Relurpify's manifest permissions, HITL controls, audit, telemetry, or workspace trust boundaries.

## Scope

This specification covers:

- capability trust classes
- policy mapping and enforcement
- HITL and approval rules
- output trust handling
- remote capability controls
- provider security requirements

This specification does not define:

- protocol transport details
- capability schema design
- TUI approval presentation

## Current State

Relurpify already has a strong execution-time permission model in [`permissions.go`](/home/lex/Public/Relurpify/framework/runtime/permissions.go), capability policy handling in [`capability_registry.go`](/home/lex/Public/Relurpify/framework/capability/capability_registry.go), and local tool helpers in [`tools.go`](/home/lex/Public/Relurpify/framework/tools/tools.go).

Current strengths:

- file, executable, and network checks
- tool and tag policy support as the precursor to capability policy
- HITL integration
- audit and telemetry hooks

Current gaps for MCP-like capabilities:

- no trust model for capability origin
- no policy classification for prompts/resources
- no explicit handling for remote structured outputs
- no unified security contract for runtime providers

## Design Principles

- No capability may execute outside framework-owned policy enforcement.
- Capability discovery does not imply capability exposure.
- Remote capability sources are untrusted by default.
- Security decisions must be made by Relurpify, not by imported MCP code.
- Output inserted into model context must carry trust and provenance metadata.
- Default behavior must fail closed for remote and provider-imported capabilities.
- Remote MCP client-side capabilities should be enforced through the same core
  security pipeline used for existing tool and skill execution, rather than
  through a parallel or weaker policy path.
- The replacement architecture should converge on one policy pipeline, not a
  legacy tool-policy path plus a new capability-policy path.
- That single policy pipeline should operate over the capability registry,
  which is now the framework's primary registry surface.
- Tools should be treated as one capability kind within that registry, not as a
  separate policy domain.

## Sandbox Boundary

Relurpify's existing gVisor-based sandbox remains an important part of the
security model, but it is only one layer.

The sandbox boundary should be treated as:

- local execution isolation for subprocesses and runtime-managed local services
- enforcement of low-level filesystem, executable, and network restrictions
- defense in depth beneath framework policy decisions

gVisor and manifest enforcement are responsible for constraining what local
code can do once execution is allowed. They do not decide:

- whether a remote capability should be admitted
- whether a capability should be exposed to the model
- whether a result should be inserted into context
- whether content is trusted
- whether delegation across trust boundaries is allowed
- whether provider sessions should inherit or retain rights

For MCP-related features this distinction is especially important:

- remote MCP servers are not sandboxed by Relurpify's local gVisor boundary
- MCP client/server providers may still use gVisor for local subprocesses,
  transports, browser backends, language servers, or helper daemons
- all local side effects triggered through MCP-related capabilities must still
  pass both framework policy and sandbox enforcement

The intended layering is:

- manifests and capability policy decide what is allowed
- the capability registry and provider/session model decide what is admitted,
  exposed, invocable, and insertable
- gVisor constrains how approved local execution is contained
- audit and telemetry record what happened

## Capability Trust Classes

Relurpify should classify capability sources into at least these trust classes:

1. Builtin trusted
2. Workspace-owned trusted
3. Provider-managed local untrusted
4. Remote declared untrusted
5. Remote approved/trusted-by-policy

Trust class affects:

- default visibility
- required approvals
- allowed content insertion
- audit verbosity
- whether the capability may be promoted into skill or agent policy selectors

## Capability Risk Classes

Capabilities should be classified independently from trust source.

Initial risk classes:

- read-only
- destructive
- execute
- network
- credentialed
- exfiltration-sensitive
- sessioned

These may replace or extend current tag-only policy semantics.

Risk classes should be additive, not exclusive. For example, one capability may
be both `network` and `exfiltration-sensitive`.

## Capability Effect Model

Relurpify should distinguish "can invoke" from "can affect the world".

Beyond trust and risk classes, capabilities should declare effect classes that
describe what they can change or influence.

Initial effect classes should include:

- filesystem mutation
- process spawn
- network egress
- credential use
- external state change
- long-lived session creation
- model-context insertion

Effect classes improve:

- policy specificity
- approval specificity
- audit semantics
- review of delegated or chained capability behavior

They should be tracked independently from trust source and risk class. For
example, a capability may be low-trust, read-mostly, but still have the effect
class `model-context insertion` or `credential use`.

## Enforcement Points

Security enforcement must occur at these levels:

1. Capability registration
2. Capability exposure to the model
3. Capability execution
4. Result insertion into context
5. Provider/session lifecycle

These should be treated as separate gates with separate telemetry and audit
events. A capability passing one gate must not imply it passes the others.

### Registration

When a provider discovers or imports a capability, Relurpify must decide:

- whether it is admitted at all
- what trust class it receives
- what risk classes it receives
- what policy defaults apply
- whether the capability is hidden, inspectable-only, or callable

Registration should occur through the capability registry rather than a
separate tool-only registry path.

### Exposure

A discovered capability may exist in runtime state but still be hidden from the agent if policy does not allow it.

### Execution

Execution must route through:

- manifest-derived permissions
- per-capability policy
- risk-class policy
- provider-specific restrictions
- HITL where required

Execution policy should evaluate in this order:

1. source trust defaults
2. capability-specific policy
3. risk-class policy
4. provider/session restrictions
5. manifest/runtime permission checks
6. HITL if still required

This execution path should apply uniformly to all capability kinds, including
tool capabilities.

### Context insertion

Capability output must be evaluated before insertion into model-visible context.

Required metadata:

- source capability
- source provider
- trust class
- content type
- truncation/summarization status
- whether insertion is raw, transformed, or metadata-only

Provenance metadata must survive transformation steps. If untrusted content is
summarized, transformed, merged, or re-emitted, the resulting content should
retain enough provenance to prevent untrusted data from becoming
"trusted-looking" solely through processing.

## Remote Capability Rules

Remote capabilities imported through MCP-like clients must be treated as untrusted by default.

Required rules:

- no direct execution by imported protocol code
- all remote capability calls become Relurpify capability invocations
- remote capabilities must not self-declare trust level
- remote schemas and metadata are advisory until normalized by Relurpify
- remote prompts/resources must not be inserted into model context without insertion-policy evaluation

Remote client-side capabilities should be normalized into the same framework
policy model used for current tools and skill-contributed behavior:

- manifest and skill policy remain authoritative
- capability execution still flows through framework admission, exposure,
  invocation, and insertion checks
- remote origin adds trust/risk constraints, but does not create a second policy stack

Remote capability normalization should strip or recompute:

- trust labels
- risk labels
- execution affordances
- user-facing descriptions if policy requires redaction

Remote capabilities must also not be able to trigger more privileged local
effects indirectly through output shaping alone. The framework should defend
against confused-deputy flows where one capability's output causes another,
more privileged capability to run without an explicit policy decision.

## Provider Security Rules

All providers must:

- register capabilities through a framework-owned API
- bind sessions to runtime lifecycle
- emit audit and telemetry events
- expose enough metadata for policy evaluation
- never bypass permission checks with direct subprocess or network execution

Provider implementations should also declare:

- whether they originate local or remote capabilities
- whether they hold credentials
- whether they emit content that is safe for direct insertion

Providers that hold credentials should declare credential domains explicitly.
Credential domains may include:

- Git hosting credentials
- browser cookies or browser-authenticated sessions
- API tokens
- cloud credentials
- database credentials

The runtime should prevent credential sharing across unrelated providers,
sessions, workflows, or capability invocations unless policy explicitly allows
it.

Provider sessions should also be created with the narrowest applicable policy
snapshot. A session opened for one task or provider purpose should not silently
inherit broader rights later.

## Manifest Policy Surface

The security model should be reflected directly in workspace-owned manifests.

The manifest surface should become more explicit around:

- provider activation
- capability policy
- delegation policy
- exposure policy
- insertion policy

This is a shift away from a primarily tool-centric manifest model toward a
capability-centric one.

### Agent manifests

Agent manifests should move from an emphasis on:

- allowed tools
- tool execution policy
- ad hoc service-specific fields

toward a more explicit security and runtime contract built around capabilities.

Agent manifests should gain or formalize:

- provider declarations:
  explicit runtime services the agent requires, such as browser, LSP, MCP
  client imports, or MCP server export
- capability selectors:
  policy keyed by capability ID, kind, source, trust class, or risk class
  rather than only tool name or tags
- delegation policy:
  which coordinated agent capabilities may be invoked, including trust and
  recursion rules
- exposure policy:
  which admitted capabilities are visible to the model versus inspectable-only
- insertion policy:
  rules controlling how remote prompt/resource/tool output may enter model
  context

Existing manifest concepts such as:

- `allowed_capabilities`
- `tool_execution_policy`
- service-specific tool knobs such as `browser`
- invocation settings

should be re-expressed in capability/provider/delegation terms rather than
retained as the primary long-term policy surface.

### Skill manifests

Skill manifests should also shift from prompt/tool overlays toward capability
bundles and provider activation.

Skill manifests should be able to declare:

- provider requirements:
  for example, activation of an MCP client provider targeting a specific remote
  source
- capability bundles:
  prompts, resources, delegated-agent targets, and narrow tool capabilities
- policy narrowing:
  skills may reduce visibility or allowed invocation, but must not bypass
  manifest or runtime policy
- configuration blobs:
  especially for MCP client imports and other provider-backed integrations

Skill configuration should remain subordinate to workspace/agent policy:

- skills may contribute or request capabilities
- skills may narrow policy
- skills must not grant trust, broaden permissions, or bypass admission,
  exposure, execution, or insertion checks

The security model should treat MCP client imports as especially important here:

- MCP client configuration is primarily skill-driven
- MCP server export is primarily manifest-declared
- both still flow through the same capability and policy pipeline

### Policy evaluation across manifests

Manifest-driven policy should remain authoritative across both agent and skill
surfaces.

The effective policy model should support:

- provider activation defaults
- capability admission defaults
- capability exposure rules
- invocation rules
- insertion rules
- delegation rules

Skill manifests may narrow effective policy, but should not widen it beyond what
the workspace-owned agent manifest and runtime allow.

## Output Trust Handling

Results from capabilities should support trust-aware handling policies:

- allow direct insertion
- allow summarized insertion
- allow metadata only
- require approval before insertion
- deny insertion

Relurpify should define at least these insertion actions:

1. direct
2. summarized
3. metadata-only
4. HITL-required
5. denied

The insertion policy should operate on content blocks, not only whole results.
One result may contain blocks with different insertion outcomes.

Output handling should also include structural controls:

- size limits for text and structured payloads
- limits on binary/blob references
- recursion or fan-out limits for resource graphs
- truncation and summarization rules that preserve provenance metadata

These controls are part of the security model, not only the UX model, because
large or adversarial payloads can create denial-of-service or prompt-flooding
conditions.

Capability inputs and outputs should also be structurally validated at runtime.
Schema validation is not only for UI or model convenience; it is part of safely
handling malformed or hostile capability payloads.

## HITL Approval Semantics

Approvals should bind to the exact operation being approved.

Approval records should bind to at least:

- capability ID
- provider ID
- session scope
- effect classes
- target resource or target capability where applicable
- workflow or task scope

Broad approvals should be avoided because they create policy drift and make
audit harder.

## Audit And Telemetry Requirements

Every capability interaction should emit structured events for:

- admission
- exposure decision
- execution attempt
- execution result
- insertion decision
- provider/session lifecycle events

Audit fields should include:

- capability ID and name
- provider ID
- trust class
- risk classes
- workflow/task context
- approval/grant context
- insertion outcome

The framework should persist effective policy snapshots for runs, sessions, and
workflows so later audits can determine which exact policy state was in force
when an action or insertion decision occurred.

Audit, telemetry, and persisted artifacts should also follow a framework-level
redaction policy for:

- secrets and tokens
- cookies and browser session material
- sensitive file excerpts
- provider configuration containing credentials

Redaction should be consistent across runtime logs, telemetry, workflow state,
and exported artifacts.

## Runtime Safety Controls

The framework should include budget and revocation controls in addition to
allow/ask/deny policy.

Required runtime safety controls should include:

- rate limits per capability
- rate limits per provider
- per-session ceilings for calls, bytes, tokens, subprocesses, and network requests
- capability revocation
- provider revocation or quarantine
- session revocation or forced shutdown

Capability admission should not be permanent. Capabilities, providers, or
sessions that become unhealthy, drift from expected behavior, or violate policy
should be revocable at runtime.

Trust should also not self-elevate. A remote capability must not become trusted
merely because:

- it has been used repeatedly
- another remote capability refers to it as trusted
- its outputs were previously accepted

Trust elevation should occur only through explicit workspace-owned policy.

## Default Policy Baseline

The first implementation should default to:

- builtin and workspace capabilities: hidden or callable according to manifest/capability policy
- provider-local capabilities: inspectable but not automatically callable unless allowed by policy
- remote capabilities: admitted as untrusted and governed by the same manifest/skill
  policy semantics as framework capabilities, with remote-origin trust affecting defaults,
  approvals, and insertion handling
- remote prompt/resource content: summarized or metadata-only by default

For MCP server export, the default security posture should also be conservative:

- minimal metadata exposure
- narrow exported scopes
- no generic task-execution surface unless explicitly declared
- explicit auth, trust, and audit policy

This is especially important for:

- remote resources
- remote prompts
- remote tool outputs
- browser and network-derived content

## Replacement Phases

### Phase 1

- introduce capability trust and risk classification
- introduce capability effect classification
- replace current tag-centric policy semantics with explicit risk classes
- add provider security contract

### Phase 2

- add result trust metadata and insertion policy
- add provenance propagation, approval binding, and policy snapshot persistence
- add remote capability admission rules

### Phase 3

- add revocation, redaction, and runtime budget controls
- remove remaining legacy tool-policy terminology from UI, manifests, and runtime code paths

### Phase 4

- add explicit capability exposure policy
- distinguish admitted, inspectable-only, hidden, and callable capability states
- enforce exposure filtering in the capability registry and agent/runtime discovery paths
- add explicit exposure defaults for builtin, workspace, provider-local, and remote capabilities

### Phase 5

- add provider/session lifecycle enforcement beyond invocation blocking
- support provider quarantine and forced shutdown for revoked or unhealthy providers
- support forced shutdown and revocation of live sessions
- add per-session subprocess and network-request ceilings in addition to existing call/byte/token ceilings
- persist and expose provider/session runtime safety state as part of policy and audit surfaces

### Phase 6

- add runtime structural validation for capability inputs and outputs
- add output-size, binary-reference, and resource-recursion/fan-out controls
- add structured audit and telemetry events for admission, exposure, insertion, and provider/session lifecycle
- apply framework redaction consistently to workflow state, persisted artifacts, and exports in addition to runtime audit/telemetry
- remove remaining tool-centric naming from manifests and runtime APIs where capability-native replacements exist

## Remaining Work Summary

The current implementation materially covers phases 1 through 3, but the
specification is not complete yet.

The major remaining work is:

- exposure policy and enforcement
- live provider/session quarantine and forced shutdown
- subprocess and network-request runtime ceilings
- runtime schema and structural validation for capability inputs and outputs
- admission/exposure/insertion/provider-lifecycle audit and telemetry events
- redaction for persisted workflow state and exported artifacts

These items are intentionally split across phases 4 through 6 so the remaining
work can proceed in framework-safe order:

1. exposure semantics first
2. lifecycle enforcement and stronger runtime controls second
3. full structural validation and observability completion last

## Acceptance

This specification is complete when:

- capability origin and risk are treated separately
- capability effects are explicit enough for policy, approval, and audit
- remote capability import is clearly constrained
- provider security requirements are explicit
- output trust handling is defined as a framework concern
- gVisor and manifest enforcement are clearly bounded as the local execution layer rather than the whole security model
- provenance survives transformation strongly enough to avoid accidental trust elevation
- approvals bind to specific capability/provider/session/effect scope
- credential domains, policy snapshots, revocation, and redaction are framework concerns
- admission, exposure, execution, and insertion are independently enforceable
- the runtime no longer depends on a separate legacy policy path for pre-capability tools
