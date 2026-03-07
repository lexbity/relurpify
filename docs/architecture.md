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

Think of Relurpify in four layers, each building on the one below:

```
┌──────────────────────────────────────────┐
│  Interface Layer                         │
│  relurpish TUI · HTTP API · dev-agent   │
└────────────────┬─────────────────────────┘
                 │
┌────────────────▼─────────────────────────┐
│  Agent Layer                             │
│  CodingAgent · PlannerAgent · ReActAgent │
│  ReflectionAgent · (more on the way)     │
└────────────────┬─────────────────────────┘
                 │
┌────────────────▼─────────────────────────┐
│  Framework Layer                         │
│  Graph runtime · PermissionManager       │
│  ContextManager · ToolRegistry           │
│  Memory · Search · local logging         │
└────────────────┬─────────────────────────┘
                 │
┌────────────────▼─────────────────────────┐
│  Execution Layer                         │
│  Ollama (LLM) · gVisor container (tools) │
└──────────────────────────────────────────┘
```

**Interface layer** — how you talk to the system. The `relurpish` TUI is the primary end-user interface. `relurpish doctor` handles workspace initialization and local dependency checks. The `dev-agent` CLI is for development and scripted use. The HTTP API (`relurpish serve`) exposes the same runtime for editor integrations.

**Agent layer** — the reasoning layer. An agent receives an instruction, builds a plan or enters a reasoning loop, decides which tools to call, and produces a result. Different agent types implement different reasoning patterns (see [agents.md](agents.md)).

**Framework layer** — the infrastructure agents sit on top of. The graph runtime executes the agent's workflow as a deterministic state machine. The permission manager enforces the file scopes, tooling execution rules, and other aspects of the security contract. The context manager compresses token usage extending the limits of local models. These components are invisible to end-users but define the system's behaviour.

**Execution layer** — where work actually happens. LLM reasoning happens via Ollama on the host. Tool execution (running tests, editing files, calling git) happens inside a gVisor-sandboxed container, isolated from the rest of your system.

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

Before any of the above happens, the agent's manifest is loaded and validated. The manifest is a YAML file that declares:

- Which filesystem paths the agent may read, write, or execute
- Which binaries it may run (go, git, bash, etc.)
- Which network endpoints it may reach
- Which container image to run tools inside
- What to do with actions not explicitly declared (ask / allow / deny)

The manifest is checked at startup. If it requires `runtime: gvisor` (mandatory) and gVisor isn't installed, the system refuses to start. This is intentional — a degraded mode without sandbox isolation defeats the purpose.

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
- [Permission Model](permission-model.md) — how the security contract works
- [TUI](Relurpish_TUI.md) — using the relurpish interface
