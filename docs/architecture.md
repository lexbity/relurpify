# Architecture

## Synopsis

Relurpify is a local-first AI agent framework. It gives a language model the ability to read, write, and execute code inside a sandboxed environment — with every action governed by a security contract you define. No cloud services, no telemetry leaving your machine, no ambiguity about what the agent is allowed to do.

---

## Why It Exists

Most AI coding tools are opaque: you don't know what the model is about to do, you can't constrain it, and it runs with your full filesystem permissions. Relurpify is built around the opposite philosophy:

- **Declared permissions** — every filesystem path, executable, and network endpoint an agent can touch is declared upfront in a manifest
- **Sandbox isolation** — all agent-executed commands run inside a gVisor container, not on your host
- **Human-in-the-loop** — anything not explicitly permitted pauses and asks you before proceeding
- **Local inference** — all LLM calls go to a local Ollama instance; nothing leaves your machine

The result is an agent you can trust to run on a real codebase.

---

## Mental Model

Think of Relurpify in five layers, each building on the one below:

```
┌──────────────────────────────────────────────────────┐
│  Application Layer                                   │
│  relurpish TUI · Nexus gateway · nexusish · dev-agent│
└──────────────────────┬───────────────────────────────┘
                       │
┌──────────────────────▼───────────────────────────────┐
│  Agent Layer                                         │
│  CodingAgent · ArchitectAgent · PipelineAgent        │
│  PlannerAgent · ReActAgent · ReflectionAgent         │
│  EternalAgent                                        │
└──────────────────────┬───────────────────────────────┘
                       │
┌──────────────────────▼───────────────────────────────┐
│  Framework Layer                                     │
│  Graph runtime · Pipeline runner · ContextManager    │
│  CapabilityRegistry · AuthorizationManager · Memory  │
│  Event log · Telemetry · AST index                   │
└──────────────────────┬───────────────────────────────┘
                       │
┌──────────────────────▼───────────────────────────────┐
│  Middleware Layer                                    │
│  MCP client/server · Nexus transport (WebSocket)     │
│  Session routing · Channel manager · Replay recorder │
└──────────────────────┬───────────────────────────────┘
                       │
┌──────────────────────▼───────────────────────────────┐
│  Platform / Execution Layer                          │
│  Ollama (LLM) · gVisor (sandboxed tools)             │
│  Shell · Git · Filesystem · Language tools (Go/Rust/ │
│  Python/JS) · LSP · Browser · SQLite · AST tools     │
└──────────────────────────────────────────────────────┘
```

**Application layer** — how you interact with the system. `relurpish` is the primary end-user TUI. `nexus` is the gateway server that coordinates distributed agent nodes. `nexusish` is the admin TUI for nexus. `dev-agent` is the CLI for development and scripted testing.

**Agent layer** — the reasoning layer. Agents receive an instruction, build a plan or enter a reasoning loop, decide which tools to call, and produce a result. Seven agent types implement different strategies: CodingAgent (multi-mode), ArchitectAgent (plan-then-execute), PipelineAgent (typed stages), PlannerAgent, ReActAgent, ReflectionAgent, EternalAgent. See [agents.md](agents.md).

**Framework layer** — the infrastructure agents sit on top of. The graph runtime executes workflows as deterministic state machines; every node declares a contract (side-effect class, idempotency, placement, checkpoint policy, state boundaries) and the runtime validates those contracts before execution. Tool calls at node boundaries route through the capability registry with policy evaluation. Checkpoints capture transition-boundary state so interrupted graphs resume without replaying completed work. System nodes (CheckpointNode, SummarizeContextNode, RetrieveDeclarativeMemoryNode, RetrieveProceduralMemoryNode, HydrateContextNode, PersistenceWriterNode) are first-class graph steps for structured memory, summarisation, and persistence. The pipeline runner executes typed stage sequences with declared contracts. Runtime startup first resolves an effective contract from the manifest, skills, and overlays, then compiles one policy bundle from that contract, then admits capabilities into the registry. The authorization manager enforces the three-level policy (Allow/Ask/Deny) against that final resolved state. The context manager compresses token usage for small local models. Memory is separated into working, declarative (facts, decisions), and procedural (routines) lanes backed by SQLite. See [framework.md](framework.md).

**Middleware layer** — transport and protocol. The MCP client/server implementation (versions 2025-06-18 and 2025-11-25) allows Relurpify to both consume capabilities from external MCP servers and expose its own capabilities to MCP clients. The Nexus transport layer (WebSocket) connects remote relurpish instances to the gateway. Session routing and channel management keep concurrent sessions isolated. See [middleware.md](middleware.md).

**Platform / Execution layer** — where work actually happens. LLM reasoning via Ollama on the host. Tool execution (tests, edits, git) inside a gVisor-sandboxed container. Language-aware tools (go_test, cargo_test, pytest, npm_test) are the primary verification surface. See [platform.md](platform.md).

---

## How It Works

### A Request from Start to Finish

```
You type an instruction in relurpish
        │
        ▼
Agent receives a Task (instruction + workspace context)
        │
        ▼
Agent builds a Graph — a sequence of reasoning and action nodes
        │
        ▼
Graph executes node by node:
  ┌─────────────────────────────────────────┐
  │  LLM node: call Ollama, get response    │
  │    → if response contains tool calls:   │
  │                                         │
  │  Tool node: check PermissionManager     │
  │    → Allow:  run tool in gVisor         │
  │    → Ask:    pause, notify you (HITL)   │
  │    → Deny:   return error to agent      │
  │                                         │
  │  Observation: result added to context   │
  │  Loop back to LLM node until done       │
  └─────────────────────────────────────────┘
        │
        ▼
Final response streamed to TUI
```

### The Manifest as Contract

Before any of the above happens, the runtime resolves an effective contract. The manifest is the starting point for that contract, but the final runtime state also includes skill contributions, agent-definition overlays, and runtime overrides.

The manifest declares:

- Which filesystem paths the agent may read, write, or execute
- Which binaries it may run (go, git, bash, etc.)
- Which network endpoints it may reach
- Which container image to run tools inside
- What to do with actions not explicitly declared (ask / allow / deny)

At startup the runtime:

1. loads and validates the manifest
2. resolves effective permissions/resources
3. resolves skills and overlays into one effective agent spec
4. compiles one policy bundle from that effective contract
5. builds and admits capabilities against the final selector set

If the manifest requires `runtime: gvisor` (mandatory) and gVisor isn't installed, the system refuses to start. This is intentional — a degraded mode without sandbox isolation defeats the purpose.

### Token Budget Management

Local models have finite context windows, so Relurpify treats context as a systems problem rather than a prompt-writing problem. The context manager tracks files, summaries, tool results, and conversation history against a token budget derived from the model's `max_tokens` setting. The live prompt is rebuilt from compact state each iteration instead of replaying the full transcript. When the budget tightens, file contents are downgraded to summaries, tool outputs are compressed, and only the most relevant working context is carried forward. Long-running plan execution can also persist checkpoints so interrupted work resumes without replaying the whole workflow.

---

## Key Files and Directories

```
relurpify_cfg/
├── config.yaml          # Default model and workspace settings
├── agent.manifest.yaml  # Default agent manifest
├── agents/              # Workspace-owned copied agent manifests
├── skills/              # Workspace-owned copied skill packages
├── telemetry/           # Structured telemetry output
├── sessions/            # Persisted TUI sessions (auto-created)
├── memory/              # Agent memory store (auto-created)
├── logs/                # Runtime logs (auto-created)
└── test_runs/           # Isolated testsuite runs and artifacts
```

Everything Relurpify creates or modifies at runtime is scoped to the project directory — either inside `relurpify_cfg/` or inside the workspace paths declared in the manifest.

Shared templates are not runtime state. They are copied into `relurpify_cfg/` and become workspace-owned from that point forward.

Skill resource paths are also contained to the workspace. Prompt/resource capabilities contributed by skills are admitted against the final resolved selector set and their resource reads still go through manifest filesystem enforcement.

---

## Entry Points

| Binary | Purpose |
|--------|---------|
| `relurpish` | Primary TUI — what end-users run |
| `relurpish doctor` | Workspace initialization and local dependency checks |
| `relurpish status` | Runtime diagnostics |
| `relurpish serve` | HTTP API only (no TUI) |
| `dev-agent` | CLI for scripted / development use |

---

## Further Reading

- [Installation](installation.md) — prerequisites and setup
- [Workspace Layout](workspace-layout.md) — canonical `relurpify_cfg/` contract and ownership rules
- [Configuration](configuration.md) — manifests, workspace config, skills
- [Agents](agents.md) — agent types and when to use each
- [Framework](framework.md) — per-package reference for all framework packages
- [Middleware](middleware.md) — MCP and Nexus transport layer
- [Platform](platform.md) — LLM client, language tools, and execution layer
- [Applications](applications.md) — relurpish, nexus, nexusish, dev-agent
- [Permission Model](permission-model.md) — how the security contract works
- [TUI](Relurpish_TUI.md) — using the relurpish interface
