# Framework

## Synopsis

The `framework/` layer is the infrastructure that all agents, applications, and platform integrations build on. It provides foundational types, runtime services, and protocol implementations with no dependencies on specific agent logic or application surfaces.

---

## Package Map

```
framework/
├── core/           Foundational types shared across every layer
├── capability/     Central capability registry
├── contextmgr/     LLM context window management
├── graph/          Deterministic state-machine workflow runtime
├── pipeline/       Staged LLM execution with typed contracts
├── manifest/       Agent security manifest parsing (relurpify/v1alpha1)
├── memory/         Hybrid storage (checkpoints, messages, vectors, workflows)
├── authorization/  Permission enforcement and HITL approval
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

**Agent & task** — `Agent`, `AgentRuntimeSpec`, `Task`, `Plan`. The spec merge/overlay system composes manifest-declared skill configurations at startup.

**Context** — `Context` is the mutable state bag threaded through every graph node and tool call. It holds messages, tool observations, budget signals, and per-scope key/value pairs. `SharedContext` merges results from parallel graph branches.

**Capabilities** — `CapabilityDescriptor`, `CapabilityKind` (Tool/Prompt/Resource), `TrustClass`, `EffectClass`, `RiskClass`, and `InsertionAction` model where a capability came from and how its output may be used. `CapabilityResultEnvelope` wraps every result with provenance and an `InsertionDecision`.

**Providers** — `Provider` is the common interface for all capability sources: builtin, plugin, MCP client/server, agent-runtime, LSP, node-device. `ProviderPolicy`, `CapabilityPolicy`, and `GlobalPolicy` form the declarative authorization layer.

**LLM** — `LanguageModel`, `LLMOptions`, `LLMResponse`, `Message`, `ToolCall`, `Tool`. The interface implemented by `platform/llm`.

**Permissions & HITL** — `ToolPermissions`, `PermissionSet`, `ApprovalBinding`, `HITLRequest`. `HITLRequest` carries a `Timeout`, `TimeoutBehavior`, and `RunID` for background task flows.

**Sessions & nodes** — `SessionInfo`, `NodeInfo`, `NodeDescriptor` support the Nexus distributed model.

**Policy** — `GlobalPolicy`, `CapabilityPolicy`, `ProviderPolicy`, `PolicyDuration`. These are the declarative types; enforcement logic lives in `framework/authorization`.

---

## capability

`framework/capability` maps descriptors to implementations and enforces provider policies at dispatch time.

**CapabilityRegistry** is the authoritative source for what an agent may call. It distinguishes:
- `KindTool` — local-native tools, subject to gVisor sandboxing.
- `KindPrompt` — LLM prompt templates injected into context.
- `KindResource` — structured data resources attached to context.

Dispatch is gated by `CapabilityPolicy` and `ProviderPolicy`. Every result is wrapped in a `CapabilityResultEnvelope` carrying provenance and an `InsertionDecision`.

`tool_formatting.go` converts descriptors to Ollama's JSON schema tool format. `node_support.go` wires node-device providers for Nexus-backed capabilities.

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

`framework/graph` provides a deterministic state-machine workflow runtime.

**Node types:**

| Node | Responsibility |
|------|---------------|
| `LLMNode` | Calls the language model and routes its response |
| `ToolNode` | Invokes a capability and captures the observation |
| `ConditionalNode` | Branches on a predicate over the current context |
| `HumanNode` | Pauses for HITL input |
| `SystemNode` | Injects a system message or state transformation |
| `ObservationNode` | Records an observation |
| `TerminalNode` | Signals completion or failure |

**Checkpointing** — `graph_checkpoint.go` captures full execution state (completed nodes, pending branches, context snapshot) for pause-and-resume without replaying from the start.

**Plan executor** — `plan_executor.go` compiles a `Plan` into a linear graph, enabling structured plans to execute through the graph runtime without manual wiring.

---

## pipeline

`framework/pipeline` is a staged LLM execution model for workflows that need typed, contract-declared steps.

Each **Stage** implements four methods:
- `BuildPrompt` — constructs the LLM prompt.
- `Decode` — parses the raw response into a typed result.
- `Validate` — checks the result against a schema contract.
- `Apply` — writes the result into the shared context map.

**ContractDescriptor** declares a stage's input key, output key, schema version, and retry policy. The **Runner** enforces contracts, persists stage results to `SQLiteWorkflowStateStore`, and resumes interrupted pipelines from the last completed stage.

---

## manifest

`framework/manifest` parses and validates agent security contracts (relurpify/v1alpha1).

**AgentManifest** declares everything an agent is permitted to do: filesystem paths, allowed executables, network endpoints, container image, default policy, and skill references.

**SkillManifest** defines reusable skill packages composed into agent manifests.

**Composition** — `merge.go` overlays workspace-local overrides onto the base template; `resolve.go` resolves relative paths; `skills_resolver.go` expands skill references into `CapabilityDescriptor` sets at startup.

---

## memory

`framework/memory` provides hybrid in-memory and disk-backed storage, scoped to session, project, or global level.

| Store | Purpose |
|-------|---------|
| `MemoryStore` | Primary interface: Remember/Recall/Search/Forget/Summarise |
| `CheckpointStore` | Graph execution checkpoints |
| `MessageStore` | Conversation history |
| `VectorStore` | Embeddings for semantic recall |
| `WorkflowStateStore` | Pipeline stage results by workflow ID |
| `WorkflowStore` | Top-level workflow metadata and status |
| `CodeIndexStore` | Workspace symbol index |

---

## authorization

`framework/authorization` enforces the three-level policy model: **Allow**, **Ask**, **Deny**.

**PermissionManager** gates every tool invocation, checking required file paths, executables, and network endpoints against the compiled policy.

**PolicyEngine** compiles declarative `PolicyRule` objects into a fast match structure. `policy_compile.go` builds it; `policy_match.go` evaluates incoming requests.

**HITL** — when a request falls into the Ask path, `hitl.go` surfaces a `HITLRequest` to the operator. Responses: `[y]` once, `[s]` session, `[a]` always, `[n]` deny.

**Command authorization** — `command_authorization.go` checks binary names and argument patterns against the manifest's allowed executables.

**Delegations** — `delegations.go` manages bounded grants of capability authority issued by one agent to another.

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
