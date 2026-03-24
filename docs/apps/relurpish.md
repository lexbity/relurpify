# TUI (`relurpish`)

## Synopsis

`relurpish` is the main interactive shell for Relurpify. It is a Bubble Tea
terminal UI that combines live chat, queued task tracking, session context
management, workspace settings, and local tool-permission editing in one
workspace-scoped interface.

The TUI is not just a chat window. It is also the operator surface for:

- switching agents and models
- reviewing and restoring saved sessions
- inspecting persisted workflows and live provider state
- responding to HITL approvals
- adjusting local tool policy and saving overrides back to the manifest

## Launch

```bash
cd /your/project
relurpish doctor
relurpish chat
```

`relurpish` expects to run inside a workspace with `relurpify_cfg/`. Runtime
state such as saved sessions, logs, telemetry, and workflow storage is kept
under that workspace.

Available commands:

| Command | Purpose |
|---------|---------|
| `relurpish chat` | Start the main TUI |
| `relurpish doctor` | Check dependencies and initialize or refresh workspace starter files |
| `relurpish status` | Start the same TUI with the current runtime diagnostics path |
| `relurpish serve` | Run only the HTTP API server, without the TUI |

## Layout

The interface is composed of four stacked regions:

1. optional title bar with workspace, model, token, and runtime summary
2. one active pane
3. a notification bar when there is a HITL prompt, restore prompt, or task toast
4. the input bar and bottom tab bar

The tab strip always exposes five panes:

```text
[1 Chat]  [2 Tasks]  [3 Session]  [4 Settings]  [5 Tools]
```

The title bar can be hidden if you want more vertical space.

## Panes

### 1. Chat

The Chat pane is the primary interaction surface. Submitted prompts start live
agent runs, and streamed output is appended to the feed as tokens, structured
results, plans, and file changes arrive.

Current Chat behavior:

- the chat feed keeps listening for stream and HITL events even when another
  tab is active
- only one run is allowed at a time unless `/parallel on` is enabled
- completed runs trigger autosave of the current session record
- `/stop` cancels the most recent active run
- `/retry` restarts the last prompt

File changes surfaced by the agent can be approved or rejected either through
slash commands or through the Session pane.

### 2. Tasks

The Tasks pane does two things:

- shows queued and completed foreground tasks created from submitted task items
- acts as the main inspector for runtime objects

The inspector surface is richer than the old docs implied. From the Tasks pane
you can open inspectors for:

- `w`: persisted workflows
- `c`: capabilities
- `m`: prompts
- `r`: resources
- `p`: live providers
- `s`: live provider sessions
- `a`: approvals

Inside an inspector, `up` and `down` move through the list and `esc` or `q`
closes the inspector. When viewing a workflow detail, `r` opens the resource
inspector scoped to that workflow's linked resources.

### 3. Session

The Session pane has two subviews:

- `Workspace Files`
- `Session Changes`

The files view is backed by an indexed workspace file list. When the Session
tab is active, submitting text in the input bar updates the file filter instead
of sending a prompt to the agent.

Session pane behavior:

- `enter` on a file adds it to the current agent context
- `e` opens the selected file in `$EDITOR`
- `tab` toggles between files and changes
- `y` or `n` in the changes view marks the selected change approved or rejected

This pane is the main place to manage context files and review the session's
change list outside the chat transcript.

### 4. Settings

The Settings pane combines three kinds of controls:

- available agents
- available Ollama models
- recording mode: `off`, `capture`, `replay`

It also lists saved sessions from the workspace session store.

Settings pane behavior:

- `enter` selects the highlighted row
- selecting an agent switches the live runtime agent
- selecting a model saves the preferred model and emits a restart-required
  system message
- selecting a recording mode updates the runtime recording mode
- selecting a saved session triggers a restore prompt
- `x` deletes the highlighted saved session entry

### 5. Tools

The Tools pane is specifically about local tools and permission policy. It is
not the general capability inspector anymore; that moved into the Tasks pane.

The pane groups local tools by risk or class labels such as:

- `read-only`
- `execute`
- `destructive`
- `network`
- `other`

Both group headers and individual tools can have live permission levels cycled
through the UI.

Tools pane behavior:

- `up` and `down` move selection
- `space` or `enter` cycles the selected permission level
- `r` resets the selected override to inherited/default
- `s` persists a selected per-tool override back to the manifest

This makes the Tools pane the operator surface for adjusting live tool policy
without leaving the TUI.

## HITL Approvals

When the runtime emits a human-in-the-loop approval request, the notification
bar becomes active and captures the approval keys.

Current approval responses:

| Key | Effect |
|-----|--------|
| `y` | Approve once |
| `s` | Approve for the current session |
| `a` | Approve persistently and save policy when applicable |
| `n` | Deny |

Pending approvals can also be listed from the command line with `/hitl`, and
the Tasks pane has an approvals inspector for reviewing them.

## Input Bar Behaviour

The input bar changes role depending on what tab is active:

- Chat: submit a prompt to the active agent
- Tasks: add a queued task item
- Session: update the workspace-file filter
- Settings: no special submission flow beyond navigation
- Tools: no special submission flow beyond navigation

The prompt prefix also changes by tab:

- `>` in Chat
- `+` in Tasks
- `@` in Session
- `?` in Settings

The input bar also provides:

- slash-command execution
- command palette/autocomplete for `/...`
- command history on `up` and `down`
- chat-feed search mode triggered by `ctrl+f`

Search mode filters the chat feed, not the Tasks or Session panes.

## Slash Commands

Current slash commands:

| Command | Purpose |
|---------|---------|
| `/help [command]` | Show available commands |
| `/add <path>` | Add file to context |
| `/remove <path>` | Remove file from context |
| `/context` | Show current context files and token usage |
| `/clear` | Clear chat history |
| `/approve` | Approve pending changes |
| `/reject` | Reject pending changes |
| `/diff [index|path]` | Toggle diff expansion for recent changes |
| `/export [md\|json] [path]` | Export the current session |
| `/hitl` | Show pending HITL approvals |
| `/mode <mode>` | Set or inspect the session mode hint |
| `/agent <name>` | Switch or inspect the active agent |
| `/strategy <strategy>` | Set or inspect the execution strategy hint |
| `/parallel on\|off` | Enable or disable parallel runs |
| `/stop` | Stop the current run |
| `/retry` | Retry the last prompt |
| `/workflows [limit]` | List persisted workflows |
| `/workflow <workflow-id>` | Show workflow details |
| `/rerun <workflow-id> <step-id>` | Rerun a workflow from a step |
| `/cancelwf <workflow-id>` | Mark a workflow canceled |
| `/resume <workflow-id>\|latest` | Resume a persisted workflow, primarily for architect-style runs |

The slash-command palette opens automatically as you type `/` and supports
autocomplete with `tab`.

## Navigation

Global navigation:

| Key | Action |
|-----|--------|
| `1` to `5` | Switch tabs |
| `tab` / `shift+tab` | Move between tabs when the input is empty |
| `ctrl+t` | Toggle the title bar |
| `ctrl+f` | Enter or exit chat search mode |
| `?` | Toggle the help overlay |
| `esc` | Close help, exit search mode, or close inspector context |
| `ctrl+c` / `ctrl+d` | Quit |

Pane-local navigation:

- Session pane: `tab`, `enter`, `e`, `y`, `n`
- Settings pane: `up`, `down`, `enter`, `x`
- Tools pane: `up`, `down`, `space` or `enter`, `r`, `s`
- Tasks inspectors: `w`, `c`, `m`, `r`, `p`, `s`, `a`, plus `esc` or `q` to
  close

## Session Persistence And Restore

Session records are stored under the workspace session store. On startup, if a
saved session exists and contains messages, the TUI can prompt to restore it.

Restore behavior:

- the restore prompt appears in the notification bar
- accepting it reloads the saved chat transcript
- saved context files are restored into the current shared context
- completed runs trigger autosave of the current session state

Session export is separate from autosave. `/export` writes a Markdown or JSON
snapshot of the current session, and can include references to runtime
artifacts such as telemetry and logs.

## Recording Mode

The Settings pane exposes three recording modes:

| Mode | Behaviour |
|------|-----------|
| `off` | Normal live provider and model calls |
| `capture` | Capture interactions for later replay |
| `replay` | Replay from prior captured interactions |

This is mainly useful for testing and reproducibility flows. See
../dev/testing.md.

## Flags

```bash
relurpish chat [flags]

--workspace <path>
--manifest <path>
--agent <name>
--ollama-endpoint <url>
--ollama-model <name>
--runsc <path>
--container-runtime <name>
--sandbox-platform <name>
--serve
--addr <addr>
```

`status` and `chat` both launch the TUI. `serve` runs only the HTTP API server.

For the authoritative command list, run `relurpish --help`.
