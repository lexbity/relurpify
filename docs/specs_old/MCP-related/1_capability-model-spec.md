# Framework Capability Model Engineering Specification

## Status

Draft

## Goal

Evolve Relurpify from a tool-only runtime contract into a framework-native capability model that can represent local native tools, Relurpify-native orchestrated capabilities, provider-backed capabilities, prompts, resources, sessions, and structured results in a way that cleanly supports MCP and non-MCP features.

## Scope

This specification covers:

- local-native `Tool` replacement
- framework-native callable capability distinctions
- capability kinds beyond tools
- schema and content models
- result and error structures
- capability discovery and registration
- replace `core.Tool` as the generic callable abstraction

This specification does not define:

- MCP transport mechanics
- concrete security policy rules
- provider persistence behavior
- TUI rendering details

## Current State

Relurpify currently exposes a flat `core.Tool` interface in [`tool_types.go`](/home/lex/Public/Relurpify/framework/core/tool_types.go).

Current strengths:

- simple model-facing metadata
- easy builtin tool implementation
- registry-based discovery
- permission-aware execution wiring

Current limitations:

- flat parameter model
- weak result typing
- no first-class prompts or resources
- no session-aware capability abstraction
- tags carry too much semantic weight

## Design Principles

- Framework abstractions must be Relurpify-owned.
- Capabilities must be serializable to MCP, but not designed as raw MCP wire types.
- Capability schemas must support nested structured data.
- This rework is a replacement, not a compatibility exercise.
- `core.Tool` / `Tool v1` should be removed rather than preserved behind adapters.
- Builtin and workspace tools should be upgraded directly to the new local-native tool contract.
- The framework should avoid duplicate registries, duplicate invocation pipelines, and long-lived legacy shims.
- Capability metadata should be rich enough for policy, TUI, and agents without relying on free-form conventions.
- `Tool` should no longer be used as shorthand for every callable capability.
- Skills remain composition, policy, and guidance surfaces rather than runtime capability types.

## Canonical Capability Identity

Every capability should have a framework-owned identity independent of display name
or wire protocol naming.

Required identity fields:

- capability ID
- kind
- stable public name
- version
- source provider ID
- source scope (`builtin`, `workspace`, `provider`, `remote`)
- optional session ID

Names should be unique within a capability namespace, but policy and persistence
should key off capability ID rather than name.

## Capability Kinds

Relurpify should support these first-class capability kinds:

1. Tool
2. Prompt
3. Resource
4. Session-backed capability
5. Subscription or watch capability

Not every capability kind must be model-callable. Some are discoverable/readable but not executable.

## Capability Family Distinctions

The framework should explicitly distinguish between capability identity,
capability kind, and capability runtime family.

At the framework level:

- `Capability` is the umbrella abstraction
- `Tool` is a local-native capability implementation
- `Skill` is a composition/guidance surface, not a capability runtime type

This means a capability may be callable without being a tool.

The key family distinctions should be:

- `LocalToolCapability`
  - local native implementation
  - executed inside Relurpify's own runtime and sandbox boundary
  - low-level framework-owned integration such as shell, file, git, or browser-action primitives
- `ProviderCapability`
  - provider-backed capability
  - may be local or remote
  - may be session-backed
  - admitted into the same capability registry, but not treated as a tool
- `RelurpicCapability`
  - framework-native higher-level capability implemented through Relurpify orchestration
  - may use workflows, planners, sub-agents, or other framework patterns internally
  - local to Relurpify, but not a low-level native tool
- `PromptCapability`
- `ResourceCapability`
- `SessionCapability`
- `SubscriptionCapability`

This distinction matters because local native tools, provider-backed
capabilities, and Relurpic capabilities have different:

- trust assumptions
- latency and failure modes
- sandbox expectations
- session behavior
- audit semantics
- approval expectations
- UI presentation needs

## Callable Trait And Runtime Families

The framework should stop treating `tool` and `callable` as synonyms.

Instead:

- callable is a capability trait
- tool is one specific local-native callable family

The framework should support callable capabilities across multiple runtime families:

1. local-native tool runtime
2. provider-backed runtime
3. Relurpic orchestration/runtime

This allows:

- a local shell integration to remain a tool
- an imported MCP callable capability to remain a provider capability
- a local sub-agent or workflow-backed callable unit to become a Relurpic capability

without collapsing them into the same runtime identity.

## Core Descriptor Shape

The capability model should converge on a shape equivalent to:

```go
type CapabilityDescriptor struct {
    ID              string
    Kind            CapabilityKind
    Name            string
    Version         string
    Description     string
    Category        string
    Source          CapabilitySource
    TrustClass      TrustClass
    RiskClasses     []RiskClass
    SessionAffinity string
    InputSchema     *Schema
    OutputSchema    *Schema
    Availability    AvailabilitySpec
    Annotations     map[string]any
}
```

This is not a wire contract. It is the minimum metadata shape needed for
registry filtering, policy evaluation, audit, TUI inspection, and MCP mapping.

## Tool V2 As Local-Native Capability

### Required properties

- stable name
- human description
- category
- capability annotations
- input schema
- output schema
- result content model
- availability rules
- execution entrypoint

### Key changes from current `core.Tool`

- replace `[]ToolParameter` with a structured input schema
- replace `map[string]interface{}` output convention with structured result types
- promote execution characteristics from tags into explicit metadata
- support session-aware and possibly long-running tools
- replace `core.Tool` as the framework contract for local-native tools rather than using it as the generic callable abstraction

### Replacement policy

- `Tool v2` should become the framework-supported abstraction for local-native tools
- all builtin tools should be ported directly to native `Tool v2`
- workspace-contributed local tools should register through the capability model only
- provider-backed callable capabilities should not be modeled as tools merely because they are invocable
- Relurpic capabilities should not be modeled as tools merely because they are callable
- Relurpify should not keep a permanent `Tool v1` adapter, dual registry, or dual execution path

## Skills Are Not Runtime Capability Types

Skills should remain:

- composition surfaces
- policy selectors
- prompt and resource packaging
- guidance for when and why to use capabilities

Skills should not become a parallel runtime abstraction for execution.

If a reusable local framework feature:

- uses LLM pattern recognition
- uses framework orchestration
- uses workflow or session state
- behaves like a callable reasoning unit

then it should be modeled as a `RelurpicCapability`, not as a skill.

The skill may still:

- select it
- prefer it in a given phase
- constrain its use by policy
- package prompts/resources that improve it

but the skill is not the capability itself.

## Prompt Model

Prompts become first-class capabilities rather than only skill prompt snippets.

Prompt capabilities should support:

- named prompt definitions
- argument schema
- prompt body or generator
- provenance and version metadata
- usage hints for agents and UI

Skill prompt snippets should be migrated into prompt capabilities rather than
maintained as a separate long-term prompt mechanism.

Prompt capabilities should explicitly distinguish:

- static prompt definitions
- generated prompts
- prompts intended for model use
- prompts intended for human/operator reuse

## Resource Model

Resources become first-class readable capabilities.

Resource capabilities should support:

- stable identifiers
- URI-like addressing where useful
- direct and templated resources
- metadata such as MIME type, size, provenance, freshness
- structured and unstructured content
- subscription/update hooks where supported

This model should cover:

- workspace-owned skill resources
- workflow knowledge
- audit/event streams
- documentation bundles
- remote MCP resources

Resource capabilities should explicitly support:

- direct read
- list
- template expansion
- optional subscribe/watch

Those operations should be modeled separately from generic tool execution so
policy can reason about read-like access differently from execute-like access.

## Content Model

The framework should support typed content blocks rather than a single loosely typed output map.

Required initial content block types:

- text
- structured JSON object
- resource link
- embedded resource
- binary reference
- error content

Optional later content types:

- image
- audio
- progress update
- tool-use / tool-result conversational blocks

Every content block should carry provenance metadata:

- source capability ID
- source provider ID
- trust class
- whether the block is raw, summarized, or transformed

## Schema Model

Capability inputs and outputs should use a framework-owned schema representation that can:

- map cleanly to JSON Schema
- support defaults and enums
- support nested objects and arrays
- support validation before execution
- carry UI and agent-facing annotations

The schema model does not need to reproduce every JSON Schema feature on day one, but it must support lossless transport of the subset Relurpify needs to expose.

Initial required schema features:

- object, array, string, number, integer, boolean
- required fields
- enum
- defaults
- nullable
- nested objects/arrays
- descriptive annotations for UI and agent prompts

Deferred unless proven necessary:

- arbitrary combinators
- recursive schemas
- custom schema dialect extensions

## Capability Registry

Relurpify has evolved from a pure tool registry to a broader capability registry.

Responsibilities:

- register capabilities by kind
- filter visible capabilities by policy
- resolve capabilities by name/id
- expose metadata for agent/tool-calling surfaces
- support provider-owned capability registration

The capability registry should support four distinct operations:

1. admission: register in runtime state
2. exposure: decide model/API visibility
3. resolution: find by ID/name
4. invocation/read: execute through policy

Those must not be collapsed into one "registered means callable" behavior.

This registry replaces the old tool-only registry rather than sitting beside it
as a parallel long-lived system.

## Availability And Session Affinity

Availability should be explicit metadata, not an implicit `IsAvailable()` call only.

Required availability states:

- available
- unavailable
- degraded
- requires-session
- requires-approval

Capabilities with session affinity should declare whether they are:

- stateless
- session-optional
- session-required
- session-creating

## Result Model

Native capability execution should return a structured result, not a loose map.

Minimum result fields:

- success/failure
- content blocks
- machine-readable metadata
- source/provenance metadata
- optional recovery or retry hints
- optional partial/progress state

This result model should apply across local tools, provider capabilities, and
Relurpic capabilities rather than being tool-specific.

## Versioning

The framework capability model should be versioned independently from MCP protocol versions.

Required properties:

- explicit internal version for capability contracts
- version-aware serialization for MCP export/import

## Replacement Phases

### Phase 1

- define framework capability types as the replacement for `core.Tool` as the generic callable abstraction
- formalize `Tool` as local-native only
- formalize callable capability families including `ProviderCapability` and `RelurpicCapability`
- add schema and content primitives
- implement the new capability registry

### Phase 2

- port builtin local tools to native `Tool v2`
- port browser and LSP-style session tools to native capability implementations
- introduce first-class runtime interfaces for provider and Relurpic callable capabilities
- remove `core.Tool` from framework internals as the generic invocation path while retaining it as the local-tool runtime contract

### Phase 3

- add prompt and resource capability types on the same registry and invocation model
- expose capability metadata to agents and UI
- ensure skills operate over capabilities and never act as runtime capability types

## Acceptance

This specification is complete when:

- `Tool v2` is clearly defined as the local-native framework tool concept
- the framework distinguishes between local tool, provider, and Relurpic callable capabilities
- prompts and resources are first-class capabilities
- `Tool v1` / `core.Tool` is no longer required by framework internals
- the framework uses one capability registry and one invocation path
- the model is suitable for MCP mapping without becoming MCP-owned
- capability identity, availability, and provenance are explicit enough for policy and persistence
