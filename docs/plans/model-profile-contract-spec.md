# Model Profile and Manifest Contract Spec

## Summary

Relurpify currently mixes three different configuration concerns:

1. agent policy and permissions
2. model compatibility behavior
3. runtime/backend enforcement

That overlap is acceptable only as long as the boundaries stay explicit. In the
current codebase, the biggest concrete problem is not theoretical config soup.
It is a security loophole: an agent can be granted file access that includes
its own manifest and other governance files.

This spec defines the technical path to fix that loophole and then cleanly
separate manifest policy from model-profile behavior.

The implementation should be staged:

- protect governance paths first
- make manifest reads immutable within a session
- split manifest policy from agent behavior
- move model quirks into `relurpify_cfg/model_profiles`
- expose the resolved state in inspection tooling

---

## Existing Code Shape

The current code already has useful building blocks:

- `framework/manifest/manifest.go` parses agent manifests
- `framework/core/agent.go` and `framework/core/agent_spec.go` carry runtime
  policy and tool-calling intent
- `framework/config/paths.go` defines the canonical `relurpify_cfg` layout
- `framework/authorization/runtime.go` builds enforcement primitives
- `platform/llm/model_profile.go` defines model compatibility data
- `platform/llm/profile_registry.go` loads profile files
- `relurpify_cfg/model_profiles/*.yaml` already exist
- `core.ProfiledModel` is already consumed by runtime and agent code

The missing part is a complete contract that says:

- what is protected and cannot be mutated
- what is immutable after bootstrap
- what belongs in a manifest versus a model profile
- how profile resolution is performed
- where the runtime should consume the resolved result

---

## Goals

1. Agents cannot mutate their own manifests or governance config.
2. Manifest state is stable for the lifetime of a session unless explicitly
   reloaded.
3. Model-specific tool-calling quirks live in model profiles, not manifests.
4. The manifest schema is separated into policy versus agent intent.
5. Model profiles are loaded from `relurpify_cfg/model_profiles`.
6. Loader ownership follows package boundaries:
   - `framework` owns enforcement and path resolution
   - `platform/llm` owns profile loading and matching
7. The resolved state is visible through doctor/probe/TUI surfaces.

---

## Non-Goals

This spec does not replace the existing authorization or sandbox architecture.
It tightens it.

It also does not remove compatibility fields immediately. Migration should be
safe and observable.

---

## Technical Boundaries

### `framework`

Owns:

- enforcement
- permission evaluation
- sandbox file-scope checks
- runtime path helpers
- manifest validation semantics

Should not own:

- YAML profile schemas for model quirks
- provider-specific repair logic

### `platform/llm`

Owns:

- model profile schema
- profile loading
- provider/model matching
- profile-aware backend wrapper behavior

Should not own:

- permissions
- sandbox policy
- filesystem path governance

### `relurpify_cfg/agents`

Should describe:

- agent identity
- allowed capabilities
- sandbox/security posture
- resource limits
- high-level model choice
- high-level tool-calling intent

Should not describe:

- transport-specific decoding quirks
- per-model repair workarounds
- tool-call schema flattening rules

### `relurpify_cfg/model_profiles`

Should describe:

- tool-calling compatibility quirks
- repair strategy
- max tool call handling
- schema flattening behavior
- provider-scoped or model-scoped matching

Should not describe:

- permissions
- audit policy
- sandbox policy
- agent-specific allowed capabilities

---

## Phase 1: Protected Config Paths

### Objective

Prevent agents from modifying protected governance files even if their manifest
appears to grant broad filesystem access.

### Why this is required

Today a manifest can grant `fs:write /home/lex/Public/Relurpify/**`, which
includes:

- its own manifest
- `relurpify_cfg/config.yaml`
- `relurpify_cfg/nexus.yaml`
- `relurpify_cfg/policy_rules.yaml`
- `relurpify_cfg/model_profiles/*.yaml`

That is a policy self-write channel and must be closed.

### Required code changes

#### `framework/authorization/permissions.go`

Do not duplicate sandbox file-scope enforcement here.

Manifest policy, HITL, and ordinary filesystem permission checks remain in the
permission manager, but protected governance roots are enforced by the sandbox
file-scope policy before host I/O occurs.

#### `framework/config/paths.go`

Use the canonical path helpers already present:

- `ConfigFile()`
- `NexusConfigFile()`
- `PolicyRulesFile()`
- `AgentsDir()`
- `ModelProfilesDir()`

If needed, add a small helper that returns the governance roots as a list.

#### `framework/authorization/runtime.go`

Keep the sandbox file-scope policy in sync with the runtime workspace roots so
governance paths remain outside the agent-visible filesystem boundary.

#### `framework/sandbox`

Use file-scope or mount-scope policy to exclude governance roots from the
agent-visible filesystem tree. This is the enforcement boundary for protected
paths.

### Tests

- writing to a protected governance file is blocked by the sandbox file-scope policy
- deleting or renaming a protected file is blocked by the sandbox file-scope policy
- writing outside protected roots still honors manifest permissions
- symlink traversal does not bypass the protected check
- the sandbox file-scope error is distinguishable from ordinary permission denial

### Exit criteria

- an agent cannot self-modify its own manifest or governance config
- governance path protection happens before manifest permission evaluation

---

## Phase 2: Manifest Snapshot Immutability

### Objective

Ensure bootstrap reads the manifest once and subsequent session logic uses a
snapshot, not a fresh file read.

### Why this matters

Even if the sandbox file-scope layer is correct, a long-running TUI or daemon can
still observe a changed manifest if it re-reads from disk later in the session.

### Required code changes

#### `framework/manifest`

Add a snapshot type that carries:

- parsed manifest
- source path
- file fingerprint
- load timestamp

Suggested shape:

```go
type AgentManifestSnapshot struct {
    Manifest *AgentManifest
    Fingerprint [32]byte
    LoadedAt time.Time
    SourcePath string
}
```

Provide a helper that loads bytes, hashes them, and returns the parsed manifest
with its fingerprint.

#### `ayenitd/open.go`

Bootstrap should capture and retain the snapshot once.

No regular post-bootstrap code path should call `LoadAgentManifest` again for
the active session.

#### `framework/authorization/runtime.go`

Authorization bootstrap should accept a manifest snapshot or at least a parsed
manifest plus fingerprint so later checks can compare against the original
state.

#### `app/relurpish/runtime`

Long-running modes may optionally rehash the manifest and emit an audit event if
the file changes.

Reload should be explicit and operator-driven, not automatic.

### Tests

- snapshot fingerprint is stable for unchanged input
- changed file bytes produce a different fingerprint
- the normal startup path does not re-read the manifest after bootstrap
- an explicit reload path returns a fresh snapshot

### Exit criteria

- manifest state is immutable for a session unless explicitly reloaded

---

## Phase 3: Manifest Schema Cleanup

### Objective

Make the manifest layout reflect the actual boundary between security policy
and agent behavior.

### Problem in the current schema

`framework/core.AgentRuntimeSpec` currently mixes:

- policy-adjacent data
- agent identity and behavioral intent
- model reference
- provider policies
- logging/debug intent
- tool-calling intent

That structure works, but it is too flat. It makes it easy to place unrelated
concerns in the same YAML layer.

### Required schema split

Introduce a logical split into:

- `spec.policy`
- `spec.agent`

Keep policy defaults under the policy side:

- `spec.policy.defaults`

### Suggested new meaning

#### `spec.policy`

Security and enforcement contract:

- permissions
- security
- audit
- resources
- policy defaults

#### `spec.agent`

Runtime intent and behavior:

- implementation
- mode
- version
- prompt
- model
- logging intent
- skills
- tool-calling intent

### Required code changes

#### `framework/manifest/manifest.go`

Add loader compatibility so both the old flat shape and the new split shape can
be parsed.

The new shape should win when both are present.

#### `framework/core/agent_spec.go`

Replace the current boolean tool-calling field with an explicit intent enum.

Suggested shape:

```go
type ToolCallingIntent string

const (
    ToolCallingIntentAuto         ToolCallingIntent = "auto"
    ToolCallingIntentPreferNative ToolCallingIntent = "prefer_native"
    ToolCallingIntentPreferPrompt ToolCallingIntent = "prefer_prompt"
)
```

`AgentRuntimeSpec` should carry that intent explicitly rather than a legacy
boolean compatibility flag.

#### `framework/core/agent_spec_overlay.go`

Update merge logic so the new schema fields do not double-apply and the
compatibility loader can reconcile old and new representations deterministically.

### Migration rule

Old manifests must remain loadable.

Compatibility behavior:

- old flat fields are promoted into the new internal representation
- new nested fields win if both are set
- deprecation reporting should flag old forms

### Tests

- old flat manifest still loads
- new split manifest loads
- old and new forms together resolve deterministically
- tool-calling intent resolves to the enum value expected by runtime code
- policy fields cannot be smuggled into agent behavior fields

### Exit criteria

- the manifest schema makes policy/behavior boundaries obvious
- the compatibility loader preserves existing manifests

---

## Phase 4: Model Profile Integration

### Objective

Move all model-compatibility behavior into `relurpify_cfg/model_profiles` and
make profile resolution part of the startup path.

### Required behavior

Model profiles should own:

- native tool-calling quirks
- tool-call repair strategy
- max tools per call
- argument decoding quirks
- multiline JSON handling
- schema flattening rules

### Required code changes

#### `platform/llm/model_profile.go`

Keep the model-profile schema in `platform/llm`, not in `framework`.

The schema should remain focused on model compatibility, not policy.

#### `platform/llm/profile_registry.go`

Load profiles from the directory pointed to by `framework/config.Paths.ModelProfilesDir()`.

Required matching precedence:

1. exact provider + model match
2. exact model match
3. longest prefix or glob match
4. `default.yaml`

`provider + model` matching should be supported explicitly because the same
model family may behave differently across providers or backends.

#### `platform/llm/backend.go` and provider adapters

Provider adapters should expose the resolved profile through `core.ProfiledModel`
and/or their managed backend wrappers.

The profile must drive:

- `UsesNativeToolCalling()`
- `ToolRepairStrategy()`
- `MaxToolsPerCall()`

#### `ayenitd/open.go`

Bootstrap should load the profile registry once and attach the resolved profile
to the active backend/model wrapper before agent execution begins.

#### `app/relurpish/runtime/bootstrap.go`

The TUI/runtime path should resolve the same profile data so the interactive and
CLI paths behave consistently.

### Required precedence

The final runtime decision for tool calling should be:

1. backend capability
2. manifest intent
3. model profile
4. framework fallback path

Interpretation:

- if the backend cannot do native tool calling, it is not used
- if the manifest explicitly prefers prompt fallback, that preference is honored
- if the profile says the model needs repair heuristics, the adapter applies them
- the framework fallback remains available when native calling is not viable

### Important design choice

The manifest should not be allowed to force native tool calling onto a backend
that cannot support it.

The explicit intent enum should be used to express the agent’s preference, not
as a guarantee that the backend must comply.

### Tests

- provider-scoped profile matches before model-only fallback
- exact match beats family match
- family match beats default
- backend capability blocks native calling when unsupported
- prompt fallback remains available when native calling is disabled

### Exit criteria

- tool-calling quirks are removed from manifests
- model profiles are the single source of model compatibility behavior

---

## Phase 5: Audit and Inspection Tooling

### Objective

Expose the resolved state so operators can inspect what the runtime decided.

### Required outputs

Inspection surfaces should report:

- sandbox file-scope governance roots
- manifest snapshot fingerprint
- selected model profile
- profile resolution reason
- manifest policy summary
- deprecation notices for old schema or legacy intent fields

### Surfaces

- `relurpish doctor`
- CLI workspace inspection
- TUI read-only permissions/profile view
- structured audit events

### Tests

- JSON output is machine-parseable
- profile resolution is deterministic
- manifest reload is emitted as a structured audit event
- sandbox file-scope denials are surfaced through tool/runtime errors and inspection views

---

## Implementation Order

Recommended order:

1. Phase 1: protected config paths
2. Phase 2: manifest snapshot immutability
3. Phase 3: manifest schema cleanup
4. Phase 4: model profile integration
5. Phase 5: audit and inspection tooling

Phase 1 is the security fix. The rest of the work is cleanup and normalization
around that fix.

---

## Acceptance Criteria

This work is complete when all of the following are true:

- agents cannot mutate governance files
- manifest state is immutable for a session unless explicitly reloaded
- the manifest schema separates policy from agent behavior
- model quirks live in `relurpify_cfg/model_profiles`
- profile resolution is deterministic and provider-aware
- runtime tool-calling decisions are explainable
- the inspection surfaces show what was resolved and why
