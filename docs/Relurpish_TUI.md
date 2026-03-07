# TUI (relurpish)

## Synopsis

`relurpish` is the primary interface to Relurpify. It is a terminal application built with Bubble Tea that gives you a live view of agent reasoning, a queue for multi-step tasks, a file and change browser for the current session, and direct control over agent configuration and tool permissions — all in one window.

---

## Why a TUI

A terminal interface fits the local-first, keyboard-driven workflow that Relurpify is designed for. It keeps everything in one place: you can watch the agent reason in real time, respond to permission prompts without switching context, review what files changed, and adjust configuration — without leaving your terminal.

---

## How It Works

### Launch

Run `relurpish` from your project directory:

```bash
cd /your/project
relurpish doctor
relurpish chat
```

Relurpish discovers `relurpify_cfg/` in the current directory. All state (sessions, logs, telemetry, memory) is scoped to that workspace.

Available subcommands:

| Command | Purpose |
|---------|---------|
| `relurpish chat` | Start the chat TUI (default) |
| `relurpish doctor` | Initialize or repair `relurpify_cfg/`, and check Docker/runsc/Ollama/Chromium |
| `relurpish status` | Show runtime diagnostics |
| `relurpish serve` | Run the HTTP API without the TUI |

### Layout

```
┌─────────────────────────────────────────────────────┐
├─────────────────────────────────────────────────────┤
│                                                     │
│              Active Pane                            │
│                                                     │
├─────────────────────────────────────────────────────┤
│  NotificationBar  (HITL prompts · toasts)           │
├─────────────────────────────────────────────────────┤
│  >>> InputBar  (prompt · /commands)                     │
├─────────────────────────────────────────────────────┤
|   TabBar  [ 1 Chat | 2 Tasks | 3 Session |          │
│            4 Settings | 5 Tools ]                   |
└─────────────────────────────────────────────────────┘
```

### Session Persistence

Every conversation is automatically saved to `relurpify_cfg/sessions/{id}/session.json`. When you restart relurpish, a notification appears offering to restore the previous session. Sessions can also be managed from the Settings pane.

---

## Panes

### 1 — Chat

The main conversation pane. Agent reasoning tokens stream here as they arrive — you see the model's thought process in real time.

Submit a prompt by typing in the InputBar and pressing `enter`. While a run is active, the agent's tool calls and observations appear inline with the response.

**Search the feed**: Press `ctrl+f`, type a query, press `enter`. Press `esc` to clear.

**HITL prompts** appear in the NotificationBar when the agent requests permission for an undeclared action:

```
[HITL] bash_execute: go test ./...
 [y] once  [s] session  [a] always  [n] deny  [d] dismiss
```

| Key | Effect |
|-----|--------|
| `y` | Approve once |
| `s` | Approve for this session |
| `a` | Always approve (saves to manifest) |
| `n` | Deny |

### 2 — Tasks

A queue for multi-step work. Add tasks with `/task <description>` in the InputBar. Tasks execute sequentially — when one run finishes the next starts automatically.

Each task row shows its status (pending / in progress / complete) and the run ID it is linked to.

### 3 — Session

Two-section view of the current session:

**Files section** — a fuzzy-searchable index of workspace files. Type to filter. Press `enter` to open a file in `$EDITOR`. Press `tab` to switch to the Changes section.

**Changes section** — files modified during the session. Review each change before accepting it:

| Key | Action |
|-----|--------|
| `y` | Accept change |
| `n` | Reject change |
| `e` | Open file in `$EDITOR` |

### 4 — Settings

Configure the active agent and model without restarting.

| Section | Keys | Action |
|---------|------|--------|
| Agent | `↑↓` + `space` | Select from discovered manifests |
| Model | `↑↓` + `space` | Select from models available in Ollama |
| Recording | `space` | Toggle off / capture / replay |
| Sessions | `enter` | Restore a saved session |
| Sessions | `x` | Delete a saved session |

Changing the model writes to `config.yaml` and shows a "restart required" notice.

### 5 — Tools

View and edit tool execution policies without manually editing the manifest.

Tools are grouped by tag: `read-only`, `execute`, `destructive`, `network`.

| Key | Action |
|-----|--------|
| `↑↓` | Navigate |
| `space` / `enter` | Cycle policy: default → allow → ask → deny |
| `r` | Reset to default |
| `s` | Save to manifest |

Tag header rows cycle the policy for all tools in that group. A `custom` label marks per-tool overrides; `tag` shows tag-inherited policies.

---

## Global Keybindings

| Key | Action |
|-----|--------|
| `1` – `5` | Switch to pane |
| `tab` | Next pane |
| `shift+tab` | Previous pane |
| `?` | Toggle help overlay |
| `ctrl+f` | Search feed (Chat pane) |
| `esc` | Close overlay / clear search |
| `ctrl+c` | Quit |

---

## Command Palette

Type `/` in the InputBar to enter command mode:

| Command | Action |
|---------|--------|
| `/task <description>` | Add task to queue |
| `/clear` | Clear the message feed |
| `/export` | Export current session |
| `/help` | Show help overlay |

---

## Recording Mode

The Settings pane exposes three recording modes, used primarily for testing:

| Mode | Behaviour |
|------|-----------|
| `off` | Normal live Ollama calls |
| `capture` | Records all LLM interactions to a tape file |
| `replay` | Replays from tape; no Ollama calls made |

See [Testing](testing.md) for how recording mode is used with `agenttest`.

---

## Flags Reference

```bash
relurpish chat [flags]

--workspace <path>          Workspace root (default: current directory)
--manifest <path>           Agent manifest path
--agent <name>              Agent preset (coding, planner, react, ...)
--ollama-endpoint <url>     Ollama base URL
--ollama-model <name>       Model override
--runsc <path>              runsc binary path
--container-runtime <name>  docker or containerd
--sandbox-platform <name>   kvm or ptrace
--serve                     Also launch HTTP API on --addr
--addr <addr>               HTTP API listen address (default :8080)
```

---

## See Also

- [Architecture](architecture.md) — how the TUI connects to the agent runtime
- [Permission Model](permission-model.md) — HITL approval flow
- [Agents](agents.md) — switching agents from the Settings pane
- [Testing](testing.md) — recording mode
