# Applications

## Synopsis

Relurpify ships four application binaries that operate different parts of the
same system:

| Binary | Package | Audience |
|--------|---------|---------|
| `relurpish` | `app/relurpish` | End users — primary coding TUI |
| `nexus` | `app/nexus` | Infrastructure — Nexus gateway server |
| `nexusish` | `app/nexusish` | Administrators — Nexus dashboard TUI |
| `dev-agent` | `app/dev-agent-cli` | Developers — scripted CLI for testing |

## App Guides

- relurpish.md
- dev-agent.md
- ../nexus/nexus.md
- nexusish.md

---

## relurpish

`app/relurpish` is the primary end-user interface to Relurpify. It is a
terminal TUI (Bubble Tea) backed by a local Ollama LLM and an extensible
capability provider system.

### TUI panes

| Pane | Key | Purpose |
|------|-----|---------|
| Chat | `1` | Conversational agent interaction with streaming output |
| Planner | `2` | Euclo plan exploration, analysis, and finalize views |
| Debug | `3` | Tests, benchmarks, traces, and plan diff inspection |
| Config | `4` | Agent-specific policies, capabilities, prompts, tools, and contract views |
| Session | `5` | Workspace files, queued tasks, changes, live workflows, providers, approvals |

### HITL approval

When an agent requests a capability not explicitly permitted by the manifest,
relurpish shows an approval overlay in the active shell. The operator responds:

| Key | Effect |
|-----|--------|
| `y` | Approve this invocation once |
| `s` | Approve for the rest of the session |
| `a` | Always allow (saves to manifest policy) |
| `n` | Deny |

### Runtime architecture (`app/relurpish/runtime`)

The runtime registers multiple capability providers before starting the agent:

- **Builtin tools** — filesystem, shell, git, language tools, AST tools, search,
  LSP, browser (from `platform/`).
- **MCP client** — connects to external MCP servers declared in workspace config.
- **Nexus node provider** — connects to the Nexus gateway and exposes
  capabilities from registered remote nodes.
- **Background delegation provider** — routes tasks explicitly marked for
  background execution to Nexus-managed agent instances.
- **Browser capability** — exposed by `ayenitd/service/browser` through the
  shared registry.

### Starting relurpish

```bash
# Interactive TUI
relurpish

# With a specific agent
relurpish --agent coding

# Diagnostics (checks Ollama, gVisor, manifest)
relurpish doctor
```

---

## nexus

`app/nexus` is the Relurpify gateway server that coordinates distributed agent
nodes in a mesh topology.

### Responsibilities

- **Node pairing** — authenticates remote relurpish instances as trusted nodes.
- **Capability routing** — forwards capability requests to the node advertising
  the requested capability.
- **Event streaming** — aggregates execution events from all nodes into a unified
  observability stream.
- **Admin API** — management surface exposed over MCP (see `app/nexus/admin`).

### Subdirectory structure

| Package | Role |
|---------|------|
| `admin` | Admin domain logic and MCP handler surface |
| `adminapi` | Request/response type contracts for the admin API |
| `bootstrap` | Startup dependency wiring and config resolution |
| `config` | Nexus-specific configuration types |
| `gateway` | Event materializer (events → OTel spans / audit log) |
| `server` | HTTP handler composition and node connection helpers |
| `status` | Health and status monitoring |

### Starting nexus

```bash
nexus --config relurpify_cfg/nexus.yaml
```

Remote relurpish instances connect to nexus by adding a `nexus:` provider
block to their workspace configuration.

### Node pairing

A new node initiates pairing by sending a registration request with its public
key. Nexus displays the pairing code in `nexusish`; the administrator confirms.
Once confirmed, the node's capabilities are advertised to all sessions connected
to that Nexus instance.

---

## nexusish

`app/nexusish` is a terminal TUI dashboard for managing and monitoring a
running Nexus gateway. It is intended for Nexus administrators, not end users.

### Panes

| Pane | Content |
|------|---------|
| Dashboard | Overview: node count, active sessions, event rate |
| Nodes | Connected nodes with status, capabilities, and health |
| Sessions | Active and recent sessions |
| Channels | Communication channel status |
| Events | Live event stream from all nodes |
| Registry | Capability registry — all advertised capabilities |
| Security | Active policies and pending pairing requests |
| Identity | Identity records and role assignments |

### Runtime (`app/nexusish/runtime`)

nexusish communicates exclusively through the Nexus admin HTTP API. The
`Runtime` type provides typed methods for each admin endpoint. `ClientState`
holds the last-fetched snapshot updated on each poll cycle by the TUI's update
loop.

### Starting nexusish

```bash
nexusish --nexus http://localhost:8080
```

---

## dev-agent (CLI)

`app/dev-agent-cli` is a Cobra CLI for development, testing, and scripted
automation. End-user coding is done via relurpish; this tool is for developers
and CI pipelines.

### Commands

```
dev-agent start        Run an agent session with a given instruction
dev-agent agenttest    Run integration test suites
dev-agent agents       List registered agent types
dev-agent skill        Inspect and manage skill packages
dev-agent session      List and inspect past sessions
dev-agent config       Display resolved workspace configuration
```

### Agent test suites

`dev-agent agenttest run` executes YAML test suites from `testsuite/agenttests/`.
Each suite specifies an instruction, expected file edits or tool calls, and a
pass/fail predicate. Tape recording makes suites deterministic in CI without a
live Ollama instance.

```bash
# Run all suites
dev-agent agenttest run

# Run a specific suite
dev-agent agenttest run --suite testsuite/agenttests/coding.go.testsuite.yaml

# Re-record a suite live
dev-agent agenttest refresh --suite testsuite/agenttests/coding.go.testsuite.yaml
```

### Key flags

| Flag | Effect |
|------|--------|
| `--agent` | Agent type (default: `coding`) |
| `--instruction` | Task instruction (required for `start`) |
| `--yes` | Auto-approve all HITL prompts |
| `--no-sandbox` | Disable gVisor (development only) |

---
