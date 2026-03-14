# Framework

## Synopsis

The `framework/` layer is the infrastructure that all agents, applications, and platform integrations build on. It provides foundational types, runtime services, and protocol implementations with no dependencies on specific agent logic or application surfaces.

The intended rule is strict dependency direction:

- `framework/` may define the canonical enforcement contract
- `agents/` may consume that contract
- `framework/` must not import `agents`

This boundary is enforced by `scripts/check-framework-boundaries.sh`.

---

## Package Map

```
framework/
├── core/           Foundational types shared across every layer
├── capability/     Central capability registry
├── capabilityplan/ Explicit capability admission planning
├── contract/       Effective runtime contract resolution
├── contextmgr/     LLM context window management
├── graph/          Deterministic state-machine workflow runtime
├── pipeline/       Staged LLM execution with typed contracts
├── manifest/       Agent security manifest parsing (relurpify/v1alpha1)
├── memory/         Hybrid storage (checkpoints, messages, vectors, workflows)
├── authorization/  Permission enforcement and HITL approval
├── policybundle/   Compiled policy bundle built from effective contract
├── search/         File glob and content search
├── identity/       Identity resolution and storage
├── telemetry/      Structured audit logging and execution tracing
├── event/          Shared event log
├── ast/            AST parsing and code indexing
├── sandbox/        Command execution (local and gVisor-sandboxed)
├── templates/      Prompt template resolution
├── config/         Workspace configuration path resolution
└── middleware/     Transport and protocol layers
    ├── channel/    Concurrent communication channel manager
    ├── gateway/    HTTP server and replay recording
    ├── node/       WebSocket connections to remote nodes
    ├── session/    Session routing and event-stream isolation
    └── mcp/        Full MCP (Model Context Protocol) implementation
        ├── protocol/   Wire-format types (2025-06-18, 2025-11-25)
        ├── client/     MCP client
        ├── server/     MCP server
        ├── session/    MCP session management
        ├── schema/     JSON schema validation and conversion
        ├── mapping/    Import/export capability mapping
        └── versioning/ Protocol version negotiation
```

---

## core

`framework/core` defines every type shared between agents, tools, providers, and the runtime. No other framework package defines domain types — they all import from core.

**Agent & task** — `Agent`, `AgentRuntimeSpec`, `Task`, `Plan`. The spec merge/overlay system composes manifest defaults, skill contributions, agent-definition overlays, and runtime overrides into the effective runtime contract.

**Context** — `Context` is the mutable state bag threaded through every graph node and tool call. It holds messages, tool observations, budget signals, and per-scope key/value pairs. `SharedContext` merges results from parallel graph branches.

**Memory classes and state data classes** — graph state is typed to control what each node may read and write.

`MemoryClass` categorises data by lifecycle:

| Class | Meaning |
|-------|---------|
| `working` | Transient per-run coordination state — cleared at graph completion |
| `declarative` | Durable facts, decisions, constraints, summaries — persisted across runs |
| `procedural` | Reusable routines and capability compositions — persisted and versioned |

`StateDataClass` categorises individual state entries by semantic role: `task-metadata`, `step-metadata`, `routing-flag`, `artifact-ref`, `memory-ref`, `structured-state`, `transcript`, `raw-payload`, `retrieval-dump`, `subagent-history`.

`StateBoundaryPolicy` declares what a node may read and write: allowed key patterns, allowed memory and data classes, inline size limits, and a `PreferArtifactReferences` flag that lint-flags raw payload stored inline instead of by reference. `LintStateMap` runs lint checks against a live state snapshot without blocking execution.

`ArtifactReference` is the preferred graph-state shape for any large output. State carries the reference; the payload lives in the workflow store. `MemoryReference` is the analogous pointer for declarative and procedural memory records.

**Capabilities** — `CapabilityDescriptor`, `CapabilityKind` (Tool/Prompt/Resource), `TrustClass`, `EffectClass`, `RiskClass`, and `InsertionAction` model where a capability came from and how its output may be used. `CapabilityResultEnvelope` wraps every result with provenance and an `InsertionDecision`.

**Providers** — `Provider` is the common interface for all capability sources: builtin, plugin, MCP client/server, agent-runtime, LSP, node-device. `ProviderPolicy`, `CapabilityPolicy`, and `GlobalPolicy` form the declarative authorization layer.

**LLM** — `LanguageModel`, `LLMOptions`, `LLMResponse`, `Message`, `ToolCall`, `Tool`. The interface implemented by `platform/llm`.

**Permissions & HITL** — `ToolPermissions`, `PermissionSet`, `ApprovalBinding`, `HITLRequest`. `HITLRequest` carries a `Timeout`, `TimeoutBehavior`, and `RunID` for background task flows.

**Sessions & nodes** — `SessionInfo`, `NodeInfo`, `NodeDescriptor` support the Nexus distributed model.

**Policy** — `GlobalPolicy`, `CapabilityPolicy`, `ProviderPolicy`, `PolicyDuration`. These are the declarative types; enforcement logic lives in `framework/authorization`.

---

## capability

`framework/capability` is the runtime container for already admitted capabilities. It owns descriptor lookup, wrapper/runtime-policy bindings, and common invocation gating across tools, prompt capabilities, and resource capabilities.

**CapabilityRegistry** is the authoritative source for what an agent may call. It distinguishes:
- `KindTool` — local-native tools, subject to gVisor sandboxing.
- `KindPrompt` — LLM prompt templates injected into context.
- `KindResource` — structured data resources attached to context.

Dispatch is gated by the compiled policy engine plus concrete permission checks. Every result is wrapped in a `CapabilityResultEnvelope` carrying provenance and an `InsertionDecision`.

`tool_formatting.go` converts descriptors to Ollama's JSON schema tool format. `node_support.go` wires node-device providers for Nexus-backed capabilities.

## capabilityplan

`framework/capabilityplan` evaluates capability candidates against the final allowed selector set before they are admitted to the registry.

Today it is used primarily for skill-backed prompt/resource capabilities so startup can record a deterministic admitted/rejected result set instead of relying on incremental register-then-prune behavior.

**AdmissionResult** records:

- capability ID and public name
- capability kind
- admitted / rejected state
- rejection reason when filtered

This admission output is surfaced into runtime inspection so the TUI and debug tooling can explain why a capability is missing.

## contract

`framework/contract` resolves one canonical `EffectiveAgentContract` for a runtime.

The contract is built from:

1. manifest defaults
2. manifest `spec.agent`
3. skill contributions
4. agent-definition overlays
5. runtime overlays

The resulting contract includes:

- effective `AgentRuntimeSpec`
- effective permission set and resources
- resolved skills and skill application results
- a source summary for inspection/debugging

Downstream runtime code should consume this contract instead of recomputing manifest, skill, and overlay state independently.

---

## contextmgr

`framework/contextmgr` keeps agents within LLM token limits across long sessions.

**Strategies** — four compression strategies selectable via `ContextPolicy`:
- `Conservative` — retains as much as possible, drops only stale items.
- `Adaptive` — adjusts aggressiveness based on remaining budget.
- `Pruning` — removes least-recently-accessed items first.
- `Aggressive` — maximum compression; reduces non-essential items to metadata.

**ProgressiveLoader** defers loading file contents until the agent needs them, reducing upfront token cost. Files start as path-only stubs and are promoted to full content on first access.

---

## graph

`framework/graph` provides a deterministic state-machine workflow runtime with contract-aware preflight, safe checkpoint resumption, explicit system nodes, and pattern builder helpers.

### Node types

| Node | Responsibility |
|------|---------------|
| `LLMNode` | Calls the language model and routes its response |
| `ToolNode` | Invokes a capability through `CapabilityInvoker` and captures the observation |
| `ConditionalNode` | Branches on a predicate over the current context |
| `HumanNode` | Pauses for HITL input |
| `TerminalNode` | Signals completion or failure |
| `ObservationNode` | Records a tool observation into context |
| `CheckpointNode` | Persists a resumable checkpoint explicitly as a graph step |
| `SummarizeContextNode` | Summarises selected state keys and history into a durable artifact |
| `RetrieveDeclarativeMemoryNode` | Fetches bounded declarative memory records into state |
| `RetrieveProceduralMemoryNode` | Fetches bounded procedural routines into state |
| `HydrateContextNode` | Restores artifact or memory references into active working state |
| `PersistenceWriterNode` | Writes declarative records, procedural routines, and artifacts with an audit trail |

### Node contracts

Every node may implement `ContractNode` to declare a `NodeContract`:

| Field | Meaning |
|-------|---------|
| `RequiredCapabilities` | Capability selectors the node needs to function |
| `SideEffectClass` | None / Context / Local / External / Human |
| `Idempotency` | Unknown / ReplaySafe / SingleShot |
| `Placement` | Any / Local / Remote / StickySession |
| `CheckpointPolicy` | None / Preferred / Required |
| `Recoverability` | None / InProcess / Persisted |
| `ContextPolicy` | `StateBoundaryPolicy` — allowed keys, classes, size limits |

`validateNodeContract` enforces that active fields are coherent (e.g. a node that declares `CheckpointRequired` without `Persisted` recoverability is invalid). `ResolveNodeContract` returns default contracts for built-in node types when the node does not implement `ContractNode` itself.

### Capability routing at ToolNode

`ToolNode` holds a `Registry CapabilityInvoker` field. When the registry is set, every tool call is routed through `InvokeCapability`, which applies `CapabilityPolicy` evaluation, `EffectClass` checks, trust-class enforcement, and wraps the result in a `CapabilityResultEnvelope`. This is the primary path. Direct `tool.Execute()` is not called at the graph level.

```go
type CapabilityInvoker interface {
    InvokeCapability(ctx, state, idOrName string, args map[string]any) (*core.ToolResult, error)
}
```

### Preflight and placement

Before execution begins, `Graph.Preflight(catalog)` validates all nodes against the available capability catalog:

1. For each node with a `NodeContract`, required capabilities are resolved against the catalog.
2. Missing required capabilities produce blocking `PreflightIssue` entries.
3. `PlacementDecision` records are produced for capabilities with a placement preference, scored by trust rank and risk rank.
4. `PreflightReport.HasBlockingIssues()` determines whether it is safe to execute.

Agents set a catalog via `g.SetCapabilityCatalog(registry)` before calling `Execute`.

### Checkpoint semantics

`GraphCheckpoint` captures execution state at a transition boundary — not at a node entry point:

```
CompletedNodeID   the node that just finished
NextNodeID        the node that should run next on resume
LastTransition    NodeTransitionRecord — includes transition reason and timestamp
Context           snapshot of the full context at that boundary
```

`ResumeFromCheckpoint` starts execution from `NextNodeID`, not from `CompletedNodeID`. This means a completed `SingleShot` node (e.g. a tool with an external side effect) is never replayed. The graph skips directly to whatever follows it.

`CreateCheckpoint` and `CreateCompressedCheckpoint` both accept a `NodeTransitionRecord` so the caller controls what reason is recorded.

The callback-based `WithCheckpointing(every N, saveFn)` is available for incremental checkpointing during execution without inserting explicit `CheckpointNode` steps.

### System node interfaces

The system nodes in `system_nodes.go` and `persistence_node.go` depend on narrow interfaces, not concrete store types:

| Interface | Role |
|-----------|------|
| `ArtifactSink` | Receives artifact records for durable storage |
| `CheckpointPersister` | Persists `GraphCheckpoint` values |
| `MemoryRetriever` | Returns bounded `[]MemoryRecordEnvelope` for a query |
| `StateHydrator` | Restores references into active state |
| `RuntimePersistenceStore` | `PutDeclarative / SearchDeclarative / PutProcedural / SearchProcedural` |
| `PersistenceAuditSink` | Receives `PersistenceAuditRecord` entries |

`framework/memory` provides bridge functions (`AdaptWorkflowStateStoreArtifactSink`, `AdaptWorkflowStateStoreAuditSink`, `AdaptRuntimeStoreForGraph`) to wire concrete stores to these interfaces without the graph importing memory directly.

### Pattern builder helpers

`patterns.go` provides composable graph constructors and wrappers:

| Function | Topology produced |
|----------|------------------|
| `BuildPlanExecuteVerifyGraph` | plan → execute → verify → done |
| `BuildPlanExecuteSummarizeVerifyGraph` | plan → execute → summarize → verify → done |
| `BuildThinkActObserveGraph` | think → act → observe ⟲ (loop until done condition) |
| `BuildReviewIterateGraph` | execute → review ⟲ (loop until done condition) |
| `WrapWithCheckpointing` | inserts a `CheckpointNode` before the terminal node of an existing graph |
| `WrapWithPeriodicSummaries` | inserts a `SummarizeContextNode` before the terminal node |
| `WrapWithDeclarativeRetrieval` | prepends a `RetrieveDeclarativeMemoryNode` before the start node |
| `WrapWithProceduralRetrieval` | prepends a `RetrieveProceduralMemoryNode` before the start node |

### Agent feature flags

Agents that use system nodes expose opt-in `*bool` fields on `core.Config`:

| Flag | Effect |
|------|--------|
| `UseExplicitCheckpointNodes` | Inserts `CheckpointNode` steps into the graph; disables callback-based checkpointing |
| `UseDeclarativeRetrieval` | Prepends `RetrieveDeclarativeMemoryNode` before the agent's first node |
| `UseStructuredPersistence` | Includes `PersistenceWriterNode` at graph completion |

All flags default to false. Agents remain backward-compatible when flags are unset.

---

## pipeline

`framework/pipeline` is a staged LLM execution model for workflows that need typed, contract-declared steps.

Each **Stage** implements four methods:
- `BuildPrompt` — constructs the LLM prompt.
- `Decode` — parses the raw response into a typed result.
- `Validate` — checks the result against a schema contract.
- `Apply` — writes the result into the shared context map.

**ContractDescriptor** declares a stage's input key, output key, schema version, and retry policy (`RetryOnDecodeError`, `RetryOnValidationError`, `MaxAttempts`).

**Runner** executes stages sequentially. Key `RunnerOptions` fields:

| Field | Purpose |
|-------|---------|
| `CapabilityInvoker` | Routes tool calls through the capability registry; falls back to direct `tool.Execute()` with a deprecation warning when nil |
| `CheckpointStore` | Persists a `Checkpoint` after each stage when `CheckpointAfterStage` is true |
| `ResumeCheckpoint` | If set, skips all stages up to and including the checkpointed stage on the next run |
| `EnableToolCalling` | Enables `ChatWithTools` and tool-execution paths per stage |

**CheckpointStore** is the pipeline-level checkpoint interface (`Save / Load`). It is separate from the graph-level `CheckpointSnapshotStore` — pipeline checkpoints capture stage-level snapshots, not graph-node transitions.

**`BuildGraph`** on `PipelineAgent` returns a visualization graph of the stage sequence for inspection. The nodes are stubs — the graph is not executable. Stage execution always goes through the `Runner` directly.

---

## manifest

`framework/manifest` parses and validates agent security contracts (relurpify/v1alpha1).

**AgentManifest** declares everything an agent is permitted to do: filesystem paths, allowed executables, network endpoints, container image, default policy, and skill references.

**SkillManifest** defines reusable skill packages composed into agent manifests.

**Composition** — permission/resource defaults are resolved before contract compilation; skill manifests are validated and resolved into pure skill data first, then admitted later against the final selector set. Skill resource paths are containment-checked against the workspace and later read through permission-aware handlers rather than direct filesystem reads.

---

## memory

`framework/memory` owns durable runtime persistence. The package separates transient scratch memory from authoritative durable workflow, checkpoint, and structured runtime records.

### Stores

| Store | Purpose |
|-------|---------|
| `MemoryStore` | Generic compatibility surface for scratch and legacy memory callers |
| `RuntimeMemoryStore` | Structured declarative/procedural runtime memory (`DeclarativeMemoryStore` + `ProceduralMemoryStore`) |
| `CheckpointSnapshotStore` | Resumable graph execution checkpoints (`Save / Load / List`) |
| `WorkflowStateStore` | Authoritative durable workflow records: runs, steps, artifacts, events, provider snapshots, delegations |
| `CompositeRuntimeStore` | Unified runtime surface: `WorkflowStateStore` + `RuntimeMemoryStore` + `CheckpointSnapshotStore` |
| `MessageStore` | Conversation history |
| `VectorStore` | Embeddings for semantic recall |
| `WorkflowStore` | Legacy top-level workflow compatibility layer (file-backed JSON) |
| `CodeIndexStore` | Workspace symbol index |

### Structured runtime memory

`RuntimeMemoryStore` distinguishes two lanes:

**Declarative** (`DeclarativeMemoryRecord`) — facts, decisions, constraints, preferences, and project knowledge. Fields include `Kind`, `Title`, `Content`, `Summary`, `ArtifactRef`, `Tags`, `Verified`, and scope/workflow/task identifiers. Searchable by query, scope, kind, tags, and workflow.

**Procedural** (`ProceduralMemoryRecord`) — reusable routines and capability compositions. Fields include `Kind`, `Name`, `Description`, `InlineBody` or `BodyRef`, `CapabilityDependencies`, `VerifiedField`, and `Version`. Procedural records require `Verified: true` before they are persisted by `PersistenceWriterNode`.

`SQLiteRuntimeMemoryStore` implements both lanes in a single SQLite file with separate tables. It also satisfies the legacy `MemoryStore` interface by routing `Remember` / `Recall` through the declarative lane.

### Checkpoints

`SQLiteCheckpointStore` persists `GraphCheckpoint` values to a `graph_checkpoints` table keyed by checkpoint ID, task ID, workflow ID, and run ID. It implements `CheckpointSnapshotStore` and optionally emits events to a `WorkflowStateStore` for cross-store audit visibility.

The file-backed `CheckpointStore` (`memory.NewCheckpointStore`) remains available as a lightweight fallback for agent configurations that do not configure a workflow database.

### Adapter layer

Three bridge functions connect stores to the graph's narrow interfaces without creating import cycles:

| Function | Bridges |
|----------|---------|
| `AdaptWorkflowStateStoreArtifactSink` | `WorkflowStateStore` → `graph.ArtifactSink` |
| `AdaptWorkflowStateStoreAuditSink` | `WorkflowStateStore` → `graph.PersistenceAuditSink` |
| `AdaptRuntimeStoreForGraph` | `RuntimeMemoryStore` → `graph.RuntimePersistenceStore` |

### Database layout

All stores open their own SQLite files. Path constants live in `framework/config/paths.go`:

| File | Owner |
|------|-------|
| `workflow_state.db` | Framework/agents — workflow state, runtime memory, checkpoints |
| `index.db` | Framework/agents — code symbol index |
| `checkpoints/` | Framework/agents — file-backed checkpoint fallback |
| `nodes.db` | Nexus only — registered agent nodes |
| `sessions.db` | Nexus only — session boundaries and delegations |
| `identities.db` | Nexus only — tenants, subjects, external identities |
| `admin_tokens.db` | Nexus only — admin API tokens |
| `events.db` | Nexus only — gateway event log |

Nexus-specific stores (`sqlite_identity_store`, `sqlite_session_store`, `sqlite_node_store`, `sqlite_admin_token_store`, `sqlite_event_log`) live in `app/nexus/db/`. Framework/agent stores (`sqlite_workflow_state_store`, `sqlite_runtime_memory_store`, `sqlite_checkpoint_store`) live in `framework/memory/db/`. The two packages do not share code.

---

## authorization

`framework/authorization` enforces the three-level policy model: **Allow**, **Ask**, **Deny**.

**PermissionManager** gates every tool invocation, checking required file paths, executables, and network endpoints against declared permissions. It now also serves permission-aware capability/resource handlers, including skill-backed resource capabilities.

**PolicyEngine** compiles declarative `PolicyRule` objects into a fast match structure. It is built from the effective contract rather than the raw manifest so provider/session/capability enforcement all evaluate the final resolved runtime spec.

**HITL** — when a request falls into the Ask path, `hitl.go` surfaces a `HITLRequest` to the operator. Responses: `[y]` once, `[s]` session, `[a]` always, `[n]` deny.

**Command authorization** — `command_authorization.go` checks binary names and argument patterns against the manifest's allowed executables.

**Delegations** — `delegations.go` manages bounded grants of capability authority issued by one agent to another.

## policybundle

`framework/policybundle` compiles an immutable runtime policy bundle from an effective agent spec.

The compiled bundle carries:

- effective `PolicyRule` set
- executable `PolicyEngine`
- agent ID and effective spec metadata

Runtime startup, preset switching, and live reload compile through `BuildFromSpec(agentID, spec, ...)`, with `BuildFromContract(...)` retained only as a compatibility wrapper.

---

## middleware

`framework/middleware` provides transport and protocol layers connecting agents to each other and to external systems.

### channel

Multiplexes logical communication streams between agents and the Nexus gateway over a shared transport. Provides ordered delivery and back-pressure per channel.

### gateway

HTTP server and replay recording for the Nexus gateway. Capture mode writes all requests/responses to a tape file; replay mode plays it back deterministically for integration tests.

### node

Manages WebSocket connections to remote agent nodes: pairing, authentication, capability advertisement, and disconnect. `ws_connection.go` owns per-node framing; `credential.go` stores authentication material.

### session

Session routing and event-stream isolation. `SessionManager` maps requests to sessions; `SessionSink` gives each session its own delivery queue so a slow consumer cannot block others.

### mcp

Full implementation of the Model Context Protocol (MCP), versions 2025-06-18 and 2025-11-25.

| Subpackage | Role |
|------------|------|
| `protocol` | Wire-format types (all JSON-RPC messages) |
| `client` | Connects to external MCP servers; imports capabilities |
| `server` | Exposes Relurpify capabilities to external MCP clients |
| `session` | Tracks active MCP sessions and their lifecycle |
| `schema` | JSON schema validation and format conversion |
| `mapping` | Import/export translation between MCP wire format and `CapabilityDescriptor` |
| `versioning` | Protocol version negotiation during initialize handshake |

---

## Further Reading

- [Agents](agents.md) — how the framework is used by agent types
- [Middleware](middleware.md) — deeper dive into MCP and the Nexus transport layer
- [Permission Model](permission-model.md) — authorization policy details
- [Configuration](configuration.md) — manifest schema reference
