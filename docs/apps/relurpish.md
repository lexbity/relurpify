# TUI (`relurpish`)

## Synopsis

`relurpish` is the primary interactive shell for Relurpify. It is a Bubble Tea
terminal UI for running agents, reviewing plans, debugging workflows, managing
workspace state, and handling approvals from a single workspace-scoped session.

The current shell is organized around five top-level panes:

- `chat`
- `planner`
- `debug`
- `config`
- `session`

## Launch

```bash
cd /your/project
relurpish doctor
relurpish chat
```

`relurpish` expects to run inside a workspace with `relurpify_cfg/`. Session
records, workflow state, logs, telemetry, and runtime storage are kept under
that workspace.

Available commands:

| Command | Purpose |
|---------|---------|
| `relurpish chat` | Start the main TUI |
| `relurpish doctor` | Check dependencies and initialize or refresh workspace starter files |
| `relurpish status` | Start the TUI focused on current runtime diagnostics |
| `relurpish serve` | Run the HTTP API server without the TUI |

## Layout

The shell is composed of:

1. an optional title bar
2. one active pane
3. overlays for command palette, guidance, HITL, and notifications
4. the input bar
5. the bottom tab bar

The tab strip exposes the current shell:

```text
[1 chat]  [2 planner]  [3 debug]  [4 config]  [5 session]
```

## Panes

### Chat

Chat is the primary execution surface. Submitted prompts start live agent runs,
and streamed output appears in the transcript as text, structured results,
plans, and file changes.

Current behavior:

- the feed continues to receive streamed output and approvals even when another pane is active
- completed runs autosave the current session record
- `/stop` cancels the most recent active run
- `/retry` restarts the last prompt
- `ctrl+f` enters transcript search

### Planner

Planner is the euclo-oriented reasoning surface. It has three subtabs:

- `explore`: scope, candidates, and symbol/pattern detail
- `analyze`: summaries, tensions, drifts, and supporting signals
- `finalize`: living-plan view, step detail, and evidence

Planner data is runtime-backed. Pattern proposals, tensions, plan state, and
notes are loaded from the active workspace runtime rather than from local-only
placeholder state.

### Debug

Debug is the runtime inspection and execution pane. It has four subtabs:

- `tests`
- `benchmarks`
- `trace`
- `plan diff`

Current behavior:

- tests run through the runtime adapter using `go test -json`
- benchmarks run through the runtime adapter using `go test -bench`
- trace and plan diff views load runtime-backed diagnostic data
- debug commands are available through slash commands such as `/test`, `/bench`, `/trace-refresh`, and `/plan-diff`

### Config

Config is the agent-specific operator surface. It contains:

- `policies`
- `capabilities`
- `prompts`
- `tools`
- `contract`

This pane replaces the older split between workspace settings and tool
inspection. Capability details, prompt details, live tool policy, and contract
data are all loaded from the runtime.

### Session

Session is the global workspace/session surface. It contains:

- `tasks`: workspace files, pending changes, and queued tasks
- `live`: workflows, providers, approvals, and runtime diagnostics
- `settings`: session-level settings and persisted session behavior

Current behavior:

- submitting text in `session -> tasks` updates the file filter instead of sending an agent prompt
- `enter` on a file adds it to the current agent context
- `e` opens the selected file in `$EDITOR`
- changes can be approved or rejected from the tasks view
- `/queue <instruction>` adds queued work to the session task runner
- the live subtab shows workflows, providers, and approvals with interactive detail panels

## Guidance and HITL

`relurpish` surfaces both classic HITL approvals and higher-level guidance
requests. These appear through the shared overlay and notification path.

Current approval responses:

| Key | Effect |
|-----|--------|
| `y` | Approve once |
| `s` | Approve for the current session |
| `a` | Persist policy when applicable |
| `n` | Deny |

Guidance requests and deferred observations can also be inspected with:

- `/guidance`
- `/deferred`

## Input Bar Behavior

The input bar changes role based on the active pane:

- `chat`: submit a prompt to the active agent
- `planner`: submit planner-specific actions such as notes
- `debug`: submit test and benchmark targets
- `config`: mostly navigation plus slash commands
- `session`: update filters or queued work depending on subtab

Prompt prefixes are context-sensitive:

- `>` in chat
- `@` in session/tasks
- `?` in config

The shell also provides:

- slash commands
- command palette/autocomplete for `/...`
- command history on `up` and `down`
- file picker mode

## Common Slash Commands

| Command | Purpose |
|---------|---------|
| `/help [command]` | Show available commands |
| `/add <path>` | Add a file to context |
| `/remove <path>` | Remove a file from context |
| `/context` | Show current context files and token usage |
| `/clear` | Clear chat history |
| `/queue <instruction>` | Enqueue background/foreground task work in the session runner |
| `/guidance` | List pending guidance requests |
| `/deferred` | List deferred guidance observations |
| `/test [package]` | Run tests in the debug pane |
| `/bench [package]` | Run benchmarks in the debug pane |
| `/trace-refresh` | Reload latest runtime trace |
| `/plan-diff` | Reload plan-diff data |
| `/export [md|json] [path]` | Export the current session |
| `/mode <mode>` | Set or inspect the session mode hint |

## Runtime Notes

`app/relurpish/runtime` is responsible for:

- workspace bootstrap and doctor flows
- runtime capability registration
- agent wiring
- guidance and HITL routing
- workflow, provider, and approval state
- planner/debug/session data loaders used by the TUI

The euclo integration extends the shell with planner and debug behavior while
continuing to stream interaction frames into the shared relurpish UI.
