# The Relurpish Guide

> **Audience**: anyone driving the Euclo coding agent from the terminal.
> **Purpose**: daily use of the relurpish TUI — the screen, the commands, approvals, and recipes.
> **Prerequisites**: a working workspace; see [Getting started](GETTING_STARTED.md).

## Starting and stopping

```bash
relurpish chat --workspace .
```

| Flag | Meaning |
|---|---|
| `--workspace <dir>` | Workspace root (default: current directory) |
| `--hitl-mode governed\|permissive` | Who answers approval requests (default: `governed`) |
| `--sandbox-backend gvisor\|docker` | Sandbox execution backend (default: `gvisor`) |
| `--offline` | Shorthand for `--inference-provider offline` |
| `--inference-provider <name>` | `ollama`, `lmstudio`, `offline` |
| `--inference-model <name>` | Backend default model; the workspace config governs agent turns |
| `--inference-endpoint <url>` | Provider endpoint (Ollama default: `http://localhost:11434`) |

Quit with `ctrl+c`.

## The screen

At boot you see:

- A **Postures** banner, for example
  `Postures — approvals: governed; intent selection: deterministic`.
  It tells you who answers approvals and who breaks intent ties; see
  [Postures](#postures).
- Tabs: **[1] chat  [2] diff  [3] recipe**.
  The chat tab has sub-tabs for the agent mode (for example
  `local-read-only`, `local-edit-on`).
- An input bar at the bottom; while a run is active it reads
  `running — > blocked | : and / active | ctrl+c quit`.

Prefixes in the input bar:

| Prefix | Effect |
|---|---|
| `/` or `:` | Open the command registry (same commands either way) |
| `>` | Send the line as a prompt |
| `?` | Search |

Known quirk: the input-bar hint text mentions `:git status`, but there is no
`git` command in the registry; git operations are `/commit` and `/local-review`.

## A turn, as you experience it

1. You type a task in plain language and press Enter.
2. The classifier scores it into intent families (`debug`, `review`,
   `implementation`, ...).
3. A thoughtrecipe or capability is picked **deterministically**.
   The reason is recorded as `decided_by` in telemetry: `pinned`,
   `explicit`, `lattice:score`, `default_recipe`, or `clarification`.
4. If the intent is genuinely ambiguous, a **clarification frame** appears:
   answer with the slot number, press Enter on empty input for the default
   slot, or `d` to defer.
   A confident single-family instruction never asks.
5. Writes and risky capabilities pause with an approval banner; see
   [Approvals](#approvals).
6. The recipe's steps run, typically a bounded ReAct loop: the model reads,
   edits through governed `file_write`, and verifies through the governed test
   runner inside the sandbox.
7. The turn ends with a classified outcome; the feed reports the real cause on
   failure (for example `react decision unparseable`), and `/retry` re-runs it.

The **diff** tab shows pending file changes; the **recipe** tab shows the
recipe registry.

## Approvals

The default `governed` HITL mode pauses writes and risky capabilities with a
banner:

```text
[y] once  [s] session  [a] always  [n] deny  [d] dismiss
```

The banner has key priority: press the key even while the cursor is in the
input.
During active work, `s` (this session) is the usual answer.
`/hitl` lists pending approvals.

In `permissive` mode (`--hitl-mode permissive` or the doctor
`--set-provider`-style configuration), approval requests auto-approve and each
decision is written to the audit trail.
Sandbox and capability enforcement are unchanged; the doctor marks the posture
degraded.

## Postures

Two independent postures are decided at boot and shown in the banner:

| Posture | Values | Meaning |
|---|---|---|
| Approvals | `governed` (default), `permissive` | Whether approval requests block on you or auto-approve audited |
| Intent selection | `deterministic` (default), `model_assisted` | Whether ambiguous intents always ask you (`deterministic`) or the tier-2 model may break ties (`model_assisted`); set `selection.posture` in `relurpify_cfg/workspace.yaml` |

## Command reference

Commands are the same whether you start them with `/` or `:`.

### Universal commands

| Command | Purpose |
|---|---|
| `/help [command]` | Command list and per-command help |
| `/recipe` | List registered recipes (id, families, description) |
| `/recipe <id>` | Pin a recipe: every turn this session runs exactly it |
| `/recipe clear` | Unpin; normal selection applies |
| `/retry` | Re-run the last prompt |
| `/stop` | Stop the current run |
| `/hitl` | Show pending HITL approvals |
| `/model [name]` | Inspect or set the active model |
| `/model-provider` | Open model provider configuration |
| `/mode <mode>` | Set agent mode (`local-read-only`, `local-edit-on`, `online-read-on`, `online-edit-on`) |
| `/permissions` | Open sandbox permissions |
| `/filescopes` | Focus file scope controls |
| `/doctor` | Show the workspace doctor report |
| `/export [md\|json] [path]` | Export the session transcript |
| `/workflows [limit]` | List persisted workflow runs |
| `/workflow <id>` | Inspect one workflow run |
| `/queue <instruction>` | Queue a task for sequential execution |
| `/workspace [path]` | Switch workspace root |
| `/agent <name>` | Switch agent type |
| `/securityguard` | Open the security guard panel |
| `/keybindings` | Open keybinding configuration |
| `/rerun <workflow> <step>` | Re-run one step of a workflow |
| `/cancelwf <workflow>` | Cancel a workflow |
| `/resume <wf-id\|latest>` | Resume a workflow |

### Chat-tab commands

| Command | Purpose |
|---|---|
| `/add <path>` | Add a file to the conversation context |
| `/remove <path>` | Remove a file from context |
| `/context` | Show current context usage |
| `/clear` | Clear the conversation |
| `/approve` | Approve the pending request |
| `/reject` | Reject the pending request |
| `/diff [index\|path]` | Show a pending change |
| `/parallel on\|off` | Toggle parallel tool execution |
| `/commit [message]` | Commit pending changes |
| `/local-review` | Review pending changes |
| `/checkpoint [label]` | Mark a session checkpoint |
| `/compact` | Compact the conversation |

Palette entries `/test-latency`, `/test-rule`, `/reset-bindings`,
`/diff-apply-all`, `/diff-revert`, and `/diff-view` currently report
"not yet implemented as palette command".

## Working with recipes

`/recipe` lists what is registered; `/recipe <id>` pins one for the session;
`/recipe clear` unpins.
Pinning overrides selection entirely: every turn runs exactly that recipe.

Recipes load once at boot.
After adding or editing a file in `relurpify_cfg/euclo/`, restart relurpish;
`/recipe` tells you when a file was skipped and why.
A pin of an unregistered recipe is rejected at pin time with the registered
list.

Recipe authoring is documented in [RECIPE_DSL](RECIPE_DSL.md); validate with
`relurplint --check recipes --workspace .`.

## Inspecting state

Everything the runtime produces lives under `<workspace>/.relurpify_state/`:

| Path | Contents |
|---|---|
| `telemetry/telemetry.jsonl` | The full event trail: route selections, policy decisions, tool calls, outcomes |
| `logs/` | Main log, `runner.log`, runner JSONL |
| `audit/` | The audit chain: approvals and auto-approvals |
| `jobs/` | Runner spool, `status.json` heartbeat, job logs |
| `ast/`, `memory/` | AST index, knowledge chunks |
| `artifacts/` | Large tool outputs, referenced by `artifact://` URIs |
| `sessions/` | Persisted sessions |
| `events.db` | Framework event log (Badger) |

Two useful greps during or after a session:

```bash
grep 'euclo.route.selected' .relurpify_state/telemetry/telemetry.jsonl | tail -3
grep 'euclo.execution.complete' .relurpify_state/telemetry/telemetry.jsonl | tail -1
```

`euclo.route.selected` carries `decided_by` (why this route ran);
`euclo.execution.complete` carries the real LLM call count and token usage.

## Tips

- **Recipes load once at boot**; restart after editing `relurpify_cfg/euclo/`.
- **Pin deliberately**: natural-language selection is deterministic, and the
  clarify flow only stops you on genuine ambiguity.
- **A failed turn tells you why**: the error in the feed is the real cause,
  and `/retry` re-rolls it.
  With small local models an occasional retry is normal.
- **Approvals are scoped**: `y` answers once, `s` for the session, `a` for
  always; all three are auditable in `audit/`.
- **Export before you close**: `/export md transcript.md` keeps a shareable
  record of the session.
