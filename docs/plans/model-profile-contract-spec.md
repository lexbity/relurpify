# Model Profile Contract Spec

## Purpose

Define the contract for model-specific behavior templates in Relurpify so that
tool-calling quirks, repair behavior, and schema limitations are loaded from a
dedicated model-profile layer instead of being scattered through agent manifests
or framework policy.

This document is both:
- an architectural analysis of where the loader should live
- a technical specification for the shape and precedence of the resulting config

---

## Problem Statement

Relurpify currently has three distinct configuration domains that can look
similar from the outside but serve different purposes:

- agent manifests in `relurpify_cfg/agents/*.yaml`
- backend/model profiles in `relurpify_cfg/model_profiles/*.yaml`
- runtime enforcement and policy in `framework/`

The system already enforces permissions, security policy, and capability policy
through manifests and framework code. That is correct, but it creates a risk of
config sprawl if model-compatibility settings are mixed into the same manifest
surface.

The specific concern here is tool calling:

- some models support native tool calling
- some models need prompt-based fallback
- some models need repair heuristics for malformed tool arguments
- some models are limited to a single tool call per response

These are model traits, not agent permissions.

---

## Analysis

### What should be in `framework`

`framework` should own:

- the abstractions used by the rest of the system
- the policy and enforcement decisions
- the neutral types that other packages can consume without importing
  `platform/llm`

That means `framework` can own:

- a neutral backend capability type
- path helpers for locating config directories
- agent/runtime config that references a model by name or provider

It should not own:

- provider-specific model quirk parsing
- transport-specific tool-call repair logic
- backend-specific YAML profile schemas

### What should be in `platform/llm`

`platform/llm` should own the implementation details of loading and applying
model profiles because the profiles describe LLM behavior, not framework policy.

That means `platform/llm` should own:

- the model-profile schema
- the registry/loader for profile files
- matching logic from model name to profile
- adapter behavior that interprets profile fields

### What should remain in manifests

Agent manifests should stay focused on:

- agent identity
- allowed capabilities
- sandbox and permission policy
- security posture
- resource limits
- high-level runtime intent such as whether native tool calling is desired

Manifest files should not become a catch-all for backend quirks. If a field is
model-specific rather than agent-specific, it belongs in a model profile.

---

## Current State

The codebase already has a partial model-profile system:

- `platform/llm/model_profile.go`
- `platform/llm/profile_registry.go`
- `relurpify_cfg/model_profiles/default.yaml`
- `relurpify_cfg/model_profiles/qwen2.5-coder.yaml`

The current consumers use a `core.ProfiledModel` interface to read profile
behavior at runtime.

This is already enough to support different tool-calling behavior in principle,
but it is not yet a fully centralized loader path. The missing piece is a clear
bootstrap path that loads profiles from `relurpify_cfg/model_profiles/` and
applies them consistently to the active backend and agent runtime.

---

## Design Goals

1. Keep manifests free of backend quirks.
2. Keep model profiles free of permissions and security policy.
3. Keep framework enforcement neutral and provider-agnostic.
4. Make model profile selection deterministic.
5. Preserve backward compatibility for current manifests.
6. Make tool-calling behavior explainable and inspectable.

---

## Proposed Boundary

### `framework`

Responsibilities:

- expose neutral types for backend capabilities and runtime config
- resolve config directory paths
- validate and enforce policy

### `platform/llm`

Responsibilities:

- load model profiles
- match profiles by model name
- apply profile behavior to backend adapters
- expose `ProfiledModel` behavior through model wrappers

### `relurpify_cfg/model_profiles`

Responsibilities:

- store reusable model compatibility templates
- define tool-calling repair and schema quirks per model family

### `relurpify_cfg/agents`

Responsibilities:

- define agent policy and runtime intent
- refer to a model by name/provider
- request native tool calling at a high level when appropriate

---

## Configuration Domains

### Agent manifest

Example fields:

- `spec.agent.model.provider`
- `spec.agent.model.name`
- `spec.agent.native_tool_calling`
- permission and policy fields

Semantics:

- what agent is this
- what model does it want
- what is it allowed to do
- should native tool calling be preferred in the agent contract

### Model profile

Example fields:

- `pattern`
- `tool_calling.native_api`
- `tool_calling.double_encoded_args`
- `tool_calling.multiline_string_literals`
- `tool_calling.max_tools_per_call`
- `repair.strategy`
- `repair.max_attempts`
- `schema.flatten_nested`
- `schema.max_description_len`

Semantics:

- how does this model behave
- what compatibility workarounds are required
- what tool-calling constraints should be applied

### Framework/runtime config

Example fields:

- workspace path
- manifest path
- inference endpoint
- inference model
- debug toggles

Semantics:

- where is the workspace
- which agent manifest should be used
- which backend should be contacted
- which model should be selected

---

## Precedence Rules

These precedence rules should be used everywhere the model template is applied.

### 1. Model profile matches by model name

Load all profiles from `relurpify_cfg/model_profiles/`.

Match order:

- exact model name match
- longest prefix/glob match
- fallback `default.yaml`

### 2. Manifest intent is higher-level than profile quirks

If the manifest says native tool calling is disabled, that is the agent-level
intent. The model profile may still describe what the backend can do, but the
agent runtime should not force native tool calling on top of an explicit
manifest disable.

### 3. Backend capability gates execution

Even if the manifest wants native tool calling, the backend must report that it
actually supports it before the runtime uses it.

### 4. Framework fallback is mandatory

If the backend does not support native tool calling, or the profile indicates a
repair/fallback strategy, the framework fallback path must remain available.

### 5. Profile settings never override permissions or security

Model profiles can affect response formatting and tool-call repair behavior.
They cannot grant permissions, relax sandboxing, or bypass policy.

---

## Loader Contract

The loader should live in `platform/llm` and use path resolution from
`framework/config` or `ayenitd`.

### Required behavior

- load all YAML profiles from a config directory
- ignore non-YAML files
- allow a missing directory
- support deterministic matching
- produce a profile object that adapters can consume directly

### Suggested API

```go
type ProfileRegistry struct {
    // loaded profiles
}

func NewProfileRegistry(configDir string) (*ProfileRegistry, error)
func (r *ProfileRegistry) Match(modelName string) *ModelProfile
```

### Startup wiring

The composition root should:

- resolve `relurpify_cfg/model_profiles`
- build a registry
- select the profile for the chosen model
- attach the profile to the active backend or model wrapper

This should happen in bootstrap, not in agent business logic.

---

## Manifest Impact

### What stays in manifests

Keep the manifest fields that describe runtime intent and policy:

- agent identity
- permissions
- security
- resource limits
- model provider/name
- canonical native tool-calling intent

### What should not move into manifests

Do not move the following into agent manifests:

- double-encoded argument workarounds
- multiline JSON literal workarounds
- tool-call limit heuristics
- repair strategy
- schema flattening rules

Those belong in model profiles.

### Why this avoids configuration soup

The config model is only a soup if multiple layers own the same semantics.
This design keeps each layer accountable for a distinct question:

- manifest: what is allowed?
- model profile: how does this model behave?
- framework: what is enforced?

That separation is the main guardrail against the “everything configurable in
one YAML blob” problem.

---

## Proposed File Layout

### `framework`

- `framework/config/paths.go`
- `framework/core/backend_capabilities.go`

### `platform/llm`

- `platform/llm/model_profile.go`
- `platform/llm/profile_registry.go`
- `platform/llm/backend.go`
- `platform/llm/backendext.go`
- provider-specific wrappers and adapters

### `relurpify_cfg`

- `relurpify_cfg/model_profiles/default.yaml`
- `relurpify_cfg/model_profiles/*.yaml`
- `relurpify_cfg/agents/*.yaml`

---

## Migration Rules

1. Existing agent manifests remain valid.
2. Existing `native_tool_calling` and legacy `ollama_tool_calling` values are
   resolved through the canonical manifest accessor layer.
3. Existing model profile files remain valid.
4. If a profile is missing, the system falls back to the built-in safe default.
5. Profile loader failures should surface as runtime configuration errors, not
   silent fallback unless the directory is absent.

---

## Acceptance Criteria

This work is complete when all of the following are true:

- model profiles are loaded from `relurpify_cfg/model_profiles/`
- backend/model selection applies a profile deterministically
- agent manifests do not contain model-specific repair quirks
- backend capability and manifest intent are both consulted before native tool
  calling is enabled
- the fallback path still works when a model lacks native tool calling
- docs explain the separation clearly
- tests cover matching, precedence, and fallback behavior

---

## Open Questions

1. Should a manifest be allowed to explicitly opt out of native tool calling even
   when the model profile enables it?
2. Should `native_tool_calling` remain on the manifest at all, or should it become
   purely a runtime resolution detail after the model profile is loaded?
3. Should model profiles be selected only by model name, or by provider plus name?
4. Should profile overrides be layered by provider, family, and exact model?

These questions should be resolved before any cleanup removes the last remaining
compatibility fields from the manifest surface.

