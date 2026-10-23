# Chapter 1 — A Line Cook with a Work Permit

You want help in your kitchen. Two candidates walk in.

The first one is brilliant, tireless, and wants root on your laptop. You hand
over the keys, turn your back, and hope. When the sauce catches fire, you
will not know who turned up the flame, when, or why.

The second one is equally brilliant, but arrives with a work permit. The
permit says which knives they may touch, which burners they may light, and
that every order they fill gets written down and signed. You watch the
permit, not the cook. When something goes wrong, the paperwork tells you
what happened — and you can revoke the permit at any time.

Relurpify is the second candidate. This book teaches you to be the kind of
kitchen owner who hires them.

## Where you are

Nothing to recall — this is the first page. You start with a terminal, a
machine, and about twenty minutes. By the end of the chapter you will have a
small, deliberately broken Python project, a materialized agent workspace,
and a doctor's report you can read line by line.

## Technical requirements

| Requirement | Version / notes |
|---|---|
| OS | Linux x86-64 or arm64 (this book verified on Linux; macOS readers: see the box in Gotchas) |
| Go | 1.26 or newer to build — the repository pins toolchain 1.27.2 |
| Git | Any recent version |
| Python | 3.10+ with pytest 9.x, for the demo project |
| Ollama | 0.40.x, serving `gemma4:e4b` |
| Sandbox | Docker, plus gVisor `runsc` on Linux for the default backend |
| Disk | ~3 GB free, plus room for the model (`gemma4:e4b` is a few GB) |

You need no GPU — `gemma4:e4b` is a small model, and a CPU is fine for this
book's tasks. Expect slower turns on an old laptop; the runtime will tell
you what it is doing the whole time.

> **Note** The `offline` backend — a deterministic scripted model that needs
> no setup — exists for CI and plumbing checks. This book runs against a
> real local model, because that is the job you are training for. We touch
> the offline backend once, in Chapter 9, where its determinism becomes a
> feature.

Everything in this chapter was captured on a real machine: Linux 6.18, Go
1.27.3, Ollama 0.40.8 with `gemma4:e4b` served, Docker with `runsc`
installed, Python 3.14.8, pytest 9.1.1. Where your output will differ, the
text says so.

## What you are hiring, exactly

Four names, one kitchen. Keep them straight and the rest of the book reads
easily:

- **Relurpify** is the framework: the governed agent runtime this whole
  repository builds. It is the permit system — policy, approvals, sandbox,
  audit, receipts.
- **Euclo** is the built-in coding agent, a worker who runs on that
  framework. When you type a task, Euclo is who thinks about it.
- **relurpish** is the program you will actually run: a terminal UI
  (the "TUI" — a full-screen program inside your terminal). The same binary
  also carries `doctor` and `status`, two non-interactive commands you will
  use constantly.
- **relurplint** is the workspace linter, and **relurpify-runner** is a
  background helper the app spawns when it needs one. They wait in the
  wings for now; Chapters 3 and 4 give them their entrances.

And one word you will see everywhere: **workspace** — the project directory
the agent works in. The agent acts only inside it, and every command it
runs executes inside a **sandbox**: container-isolated command execution,
through gVisor (`runsc`) or Docker. The sandbox is a locked door between
Euclo and the rest of your machine. We will lean on that door all book.

## The two-directory rule

After this chapter, your project will contain two new directories, and the
difference between them is the most important rule in the book:

| Directory | What it is | Who writes it |
|---|---|---|
| `relurpify_cfg/` | The agent's configuration: which model, which tools, which recipes, which policies | The `doctor` command creates it once, on first run. After that, **you** own it. Your edits win. |
| `.relurpify_state/` | Everything the runtime *produces*: logs, telemetry, the audit trail, memory, session records | The runtime. Always output, never source. You read it; you never feed it. |

A kitchen analogy, and then we drop it for a while: `relurpify_cfg/` is the
rulebook on the wall — the recipes, the standards, the allergy list.
`.relurpify_state/` is the tape of last night's service. You rewrite the
rulebook whenever you like. You do not cook from the tape.

Why the hard split? Because a system you are learning to trust owes you a
clean answer to one question: *what did you do, and why?* If configuration
and output mix, that question loses its answer. Governed behavior starts
with this directory line.

## Hands-on

### 1. Get the code and build the binaries

Clone the repository and check out the edition this book is pinned to:

```bash
git clone https://github.com/lexbity/relurpify.git
cd relurpify
git checkout BETA-RC1-FALL2026
```

Build the two binaries this book uses (run from the repository root):

```bash
go build -o relurpish ./app/relurpish
go build -o relurplint ./app/relurplint
```

The build is offline-friendly. First build takes a minute or two.

Check what you built:

```bash
./relurpish --version
```

```text
relurpish dev (commit none, built unknown)
```

A source build reports `dev` for the version and `none`/`unknown` for the
commit and build date. Release binaries report real values, like
`relurpish 0.1.0 (commit 594da458..., built 2026-10-08T...)` — the
difference is cosmetic, and it tells you which kind of binary you are
holding.

> **Tip** If you work in a sandboxed or disk-quotad machine, the
> repository's own contributing guide suggests keeping the Go caches inside
> the checkout: `export GOMODCACHE=$PWD/.gomodcache GOCACHE=$PWD/.gocache`.

### 2. Seed the stringkit project

Every chapter from here works on **stringkit**, a tiny Python string
library that starts life with one bug on purpose. The bug is the book's
pedal: the agent will spend Chapter 2 fixing it while you watch every step.

Create the project, initialize git, and add two files:

```bash
mkdir -p ~/code/stringkit && cd ~/code/stringkit
git init
```

`stringkit.py`:

```python
"""stringkit — small string utilities, kept tiny on purpose."""

import re


def slugify(text: str) -> str:
    """Turn a title into a URL-safe slug."""
    slug = text.strip().lower()
    slug = re.sub(r"\s+", "-", slug)
    return slug
```

`test_stringkit.py`:

```python
"""Tests for stringkit."""

from stringkit import slugify


def test_slugify_strips_punctuation():
    assert slugify("Hello, World!") == "hello-world"


def test_slugify_collapses_whitespace():
    assert slugify("  many   spaces  ") == "many-spaces"
```

Read `slugify` the way a reviewer would. It lowercases, it turns runs of
whitespace into single hyphens — and it never touches punctuation. The
first test describes behavior the code does not have. Commit the broken
state (a clean git baseline makes the agent's changes visible as a diff,
which you will want in Chapter 2):

```bash
git add -A
git commit -m "seed stringkit"
python3 -m pytest test_stringkit.py
```

```text
collected 2 items

test_stringkit.py .F                                                     [100%]

=================================== FAILURES ===================================
_______________________ test_slugify_strips_punctuation ________________________

    def test_slugify_strips_punctuation():
>       assert slugify("Hello, World!") == "hello-world"
E       AssertionError: assert 'hello,-world!' == 'hello-world'
E
E         - hello-world
E         + hello,-world!
E         ?      +      +

test_stringkit.py:11: AssertionError
=========================== short test summary info ============================
FAILED test_stringkit.py::test_slugify_strips_punctuation - AssertionError: a...
========================= 1 failed, 1 passed in 0.03s ==========================
```

One red, one green, and pytest's assert diff tells the whole story:
`hello,-world!` — the comma and the bang rode along. That red test is
stringkit's state at the end of this chapter. Leave it broken. Fixing it is
the agent's job, not yours.

> **Note** The exact pytest banner above your failure list varies with your
> pytest version and installed plugins. What matters is the bottom:
> `1 failed, 1 passed`, and the failure named
> `test_slugify_strips_punctuation`.

### 3. Run the doctor

**doctor** is relurpish's readiness check. It inspects your machine and
workspace, tells you what is missing, and — on a workspace that has none —
creates the starter configuration. Run it from inside stringkit, pointing
at the binary you built:

```bash
~/relurpify/relurpish doctor --workspace .
```

Substitute your own path to the repository. Here is the captured output on
the verification machine, with the log lines at the top trimmed and one
long section elided where marked:

```text
Workspace: ~/code/stringkit
Config root: ~/code/stringkit/relurpify_cfg
Workspace present: yes
Config file: yes
Manifest fingerprint: e3985117c09367091d0a4cc6f7a5b28f11519a3a9f411114410f75552a30badc
Inference backend:
  provider: ollama
  endpoint: http://localhost:11434
  state: ready
  tool_calling_mode: fallback
  models: parable/qwen3-fable:8b, rafw007/qwen35-claude-coder:9b, gemma4:12b, gemma4:e4b
  selected: parable/qwen3-fable:8b
  error: -
  profile: gemma4.llm.yaml
Providers:
    lmstudio [unhealthy]
    endpoint: http://localhost:1234
    hint: Start LM Studio, enable the local HTTP server in Settings, then verify the port
  * ollama [ready]
    endpoint: http://localhost:11434
    models: parable/qwen3-fable:8b, rafw007/qwen35-claude-coder:9b, gemma4:12b, gemma4:e4b
    hint: Start it: `ollama serve` then `ollama pull gemma4:e4b`
    ... [offline and openai_compatible rows elided]
Dependencies:
  - workspace_directory: ok [required] (workspace directory exists and is readable)
  - disk_space: ok [warning] (sufficient disk space available (317725 MB))
  - runner: missing [warning] (no runner status (not spawned yet))
  - hitl_mode: ok [warning] (governed: human-approval requests block on a decision)
  - selection_posture: ok [warning] (deterministic: ambiguous intents always ask a human via a clarification frame (workspace selection.posture: model_assisted to opt into model disambiguation))
  - starter-templates: ok [required] (workspace template archive available)
  - model-profile: ok [required] (gemma4.llm.yaml)
  - sandbox_backend: ok [required] (runsc version release-20260406.0
spec: 1.1.0-rc.1)
  - runsc: ok [warning] (runsc version release-20260406.0
spec: 1.1.0-rc.1)
  - docker: degraded [warning] (degraded: runsc runtime not registered)
  - audit_chain: ok [required] (no audit chain yet (created on first agent run))
  - runtime_image: degraded [required] (unpinned (tag-based; resolved from the local daemon at boot — set security.sandbox.image_digest to pin))
Result: ready
```

`Result: ready` is the verdict, and this report deserves a slow read,
because it is your kitchen inventory:

- **The config tree now exists.** `Config root: .../relurpify_cfg` — doctor
  materialized it on this first run, silently, from templates shipped
  inside the binary. That is *materialization*: the framework writes its
  starter configuration into your project once, and from that moment the
  copy in your workspace outranks the templates. Step 5 walks the tree.
- **The model is real.** `provider: ollama ... state: ready`, with every
  model your Ollama serves listed. The `*` marks the provider the doctor
  would use.
- **Two informational warnings are normal.** `runner: missing` (the
  background helper has not been spawned yet — it appears on first boot)
  and `runtime_image: degraded` (the sandbox's container image is not
  pinned to an immutable digest — Chapter 3 shows how, and why you would).
  Both say `warning`, neither says `blocking`, and the verdict is still
  ready.
- **Each row names its severity class.** `required` rows must pass for a
  green verdict; `warning` rows inform. Learning to sort those visually is
  most of what "reading the doctor" means.
- **The two posture rows are the work permit's headline.** `hitl_mode:
  governed` — approval requests block on a human, which is you. 
  `selection_posture: deterministic` — when your instruction is ambiguous,
  a clarifying question comes to *you*, not to a model making a guess.
  Chapter 3 makes both dials adjustable; for now, read them as the default
  contract.

> **Warning** Two lines in that transcript deserve a second look. First,
> `selected: parable/qwen3-fable:8b` — that is the *probe's* pick from the
> backend's model list, not the model your agent turns will use. The
> workspace configuration (`model.name` in `relurpify_cfg/workspace.yaml`)
> governs turns; Chapter 3 untangles this properly. Second, `docker:
> degraded (runsc runtime not registered)` means your Docker daemon does
> not know about gVisor as a runtime. The default `gvisor` backend works
> through `runsc` directly (`sandbox_backend: ok`), so this degrades only
> the *docker* backend choice — you can register `runsc` with Docker later
> if you want that option.

### 4. Walk the tree the doctor left

```bash
find relurpify_cfg -maxdepth 1 | sort
ls relurpify_cfg/euclo/
ls -a .relurpify_state/
```

```text
relurpify_cfg
relurpify_cfg/euclo
relurpify_cfg/model
relurpify_cfg/prompts
relurpify_cfg/security
relurpify_cfg/tools
relurpify_cfg/workspace.yaml
code_review.erpe
debug_tdd_repair.erpe
default.erpe
dep_upgrade.erpe
explain.erpe
extract_func.erpe
families.yaml
investigation.erpe
plan.erpe
security_review.erpe
test_synthesis.erpe
audit  artifacts  ast  jobs  logs  memory  sessions  telemetry  test_run
```

Three readings, three chapters:

- `euclo/` holds **thoughtrecipes** (`.erpe` files) — the workflows Euclo
  can run, plus `families.yaml`, the routing table that maps kinds of
  intent to recipes. Chapter 4 teaches the language; these shipped files
  are your reference corpus.
- `model/`, `security/`, `tools/`, `prompts/` are exactly what their names
  say: the provider catalog and per-model behavior profiles, the security
  policies, the ~99 tool manifests, and the prompt corpus. Chapter 3 tours
  them.
- `.relurpify_state/` already has subdirectories (`audit`, `telemetry`,
  `logs`, ...), and nothing in it is yours to edit. In Chapter 2 you will
  read two entries from it — the audit chain and the telemetry line — and
  they will become your receipts.

Look inside the file that steers it all:

```bash
cat relurpify_cfg/workspace.yaml
```

```yaml
schema: relurpify/workspace/v1

model:
  provider: ollama
  name: gemma4:e4b

sandbox:
  backend: gvisor

selection:
  posture: deterministic
```

(Trimmed to the lines that matter this chapter — the shipped file also
carries logging, audit-retention, telemetry, and runner settings, each with
a comment explaining itself.) Notice `schema:` on the first line: every
config file kind carries a schema tag, the versioning mechanism. And
notice what you did *not* have to do: pick defaults. The materialized tree
is a working kitchen the moment it lands.

### 5. Read what the doctor tells you to fix

On the verification machine, one more row appeared the moment a real model
was reachable — this is the report earning its keep:

```text
model_tool_template MISSING  served template for gemma4:e4b has no tool support
(no .Tools block); profile intent is native, so tool decisions may arrive as
text instead of structured tool calls. Use a model artifact with a
tool-capable template, or set tool_calling.intent: prompt_based in the model
profile
```

Plain-language translation: Euclo drives models through *tool calls* —
structured requests, not prose. This particular `gemma4:e4b` artifact's
prompt template has no tool section, so with the profile's native intent,
the model's decisions may arrive as text the runtime cannot parse. The row
tells you the fix, and the fix is one line in a file you own:

```bash
sed -i 's/  intent: native/  intent: prompt_based/' \
  relurpify_cfg/model/profiles/gemma4.llm.yaml
```

Whether you need this depends on your model artifact — a tool-capable
build of `gemma4:e4b` will not show the row at all. The lesson is not the
specific edit; it is the shape of the exchange: *the doctor names the
problem, the fix lives in `relurpify_cfg/`, and your edit wins.* You will
run that loop for the rest of the book.

### 6. Boot the TUI

```bash
~/relurpify/relurpish chat --workspace .
```

What you should see, per the project's own guide: a **Postures banner**
(`Postures — approvals: governed; intent selection: deterministic`), three
tabs (`[1] chat [2] diff [3] recipe`), and an input bar at the bottom. You
are not chatting yet — Chapter 2's first turn is this book's real opening
night — but you can look around: the input bar accepts `/` for the command
registry (try `/help`), and `ctrl+c` quits.

Here is what the verification machine's first boot rendered, captured from
a live run (the `relurpify_cfg` panels redraw at your terminal's width; the
transcript is trimmed to one panel):

```text
╭─────────────────────────────────────────────────────────╮
│ Doctor / Checks                                         │
│ workspace  ~/code/stringkit                             │
│ config  ~/code/stringkit/relurpify_cfg                  │
│ manifest  missing                                       │
│ fingerprint  e3985117c09367091d0a4cc6f7a5b28f11519a3a9f411114410f75552a30badc
│ workspace_directory OK  workspace directory exists and is readable
│ disk_space   OK  sufficient disk space available (317725 MB)
│ runner       MISSING  no runner status (not spawned yet)
│ hitl_mode    OK  governed: human-approval requests block on a decision
│ selection_posture OK  deterministic: ambiguous intents always ask a human
│                       via a clarification frame
│ starter-templates OK  workspace template archive available
│ model-profile OK  gemma4.llm.yaml
│ sandbox_backend OK  runsc version release-20260406.0
│ runsc        OK  runsc version release-20260406.0
│ docker       MISSING  degraded: runsc runtime not registered
│ audit_chain  OK  no audit chain yet (created on first agent run)
│ runtime_image OK  unpinned (tag-based; resolved from the local daemon at
│                   boot — set security.sandbox.image_digest to pin)
╰─────────────────────────────────────────────────────────╯
startup checks failed; resolve Doctor issues to unlock guest chat
tab checks/templates/logs  r refresh  h heal  p models
```

The TUI's first screen is the same report the doctor prints, rendered live
— the same checks, the same severities, now with `r` (refresh), `h` (heal),
and `p` (models) in the footer.

> **Warning — flag to the author** On the verification machine, this gate
> held chat locked on the Doctor pane even though the CLI doctor reported
> `Result: ready`: the runner row stayed `MISSING` and the lock persisted
> across refresh and heal attempts in captured runs. Reading the source,
> the gate unlocks when a *refreshed* doctor report arrives healthy — and
> two spots look unimplemented rather than intentional (a `manifest`
> status field that is declared and read but never assigned; an empty
> report returned when the runtime handle is not yet wired). If your boot
> unlocks after a refresh or a heal, tell the author which key cleared it —
> that line goes in this chapter; if it stays locked, Chapter 2's receipts
> will be captured the moment this resolves.
> `[NEEDS CONFIRMATION: exact TUI unlock behavior at BETA-RC1-FALL2026]`

Meanwhile you have lost nothing: the CLI doctor is this chapter's oracle,
it says ready, and everything Chapter 2 needs — a materialized, governed,
model-backed workspace — is standing.

## Gotchas

- **`doctor --fix` is not the same as `doctor`.** On a fresh workspace,
  plain `doctor` materializes quietly. `--fix` *re*-materializes and asks
  `Overwrite current starter config and manifest from templates? [y/N]:` —
  answer carefully, because `y` overwrites your edited copies with shipped
  templates. The `your-edits-win` rule protects you from *future*
  materializations, not from a confirmed overwrite.
- **`selected:` is the probe's pick, not your model.** Turns are governed
  by `model.name` in `relurpify_cfg/workspace.yaml`. Chapter 3 separates
  the two for good.
- **A MISSING row is not always a blocker.** Read the severity word and
  the verdict line, in that order. `runner: missing` and
  `runtime_image: degraded` are informational; `Result: ready` is the
  verdict.
- **The tool-template row is model-artifact-specific.** If
  `model_tool_template` appears, your model build's template lacks a tool
  section — switch to an artifact with tool support, or set
  `tool_calling.intent: prompt_based` in the model profile, exactly as the
  row's hint says.
- **Terminal too narrow.** The TUI needs at least 60 columns and wants
  more; on a cramped terminal it says so and renders badly. Widen the
  window and continue.

## Summary

- You hired the second candidate: Relurpify is the framework, Euclo the
  agent, relurpish the program you drive, and the workspace is the only
  room they work in — through a sandbox.
- `doctor` materializes `relurpify_cfg/` once and then reports readiness
  forever after; `Result: ready` plus two informational warnings is a
  healthy machine.
- The two-directory rule is the book's constitution in miniature: you edit
  `relurpify_cfg/`, you read `.relurpify_state/`.
- Postures are the permit's headline: governed approvals, deterministic
  selection. You will meet the approval banner in person next chapter.
- When the doctor names a problem, the fix lives in a file you own, and
  your edit wins.

## Check yourself

1. What does `doctor` do on a workspace that has no `relurpify_cfg/`
   directory, and what does it do on one that does?
2. Which of the two directories would you edit to change which model the
   agent uses — and which one holds the audit trail?
3. The doctor's output says `runner: missing [warning]` and
   `Result: ready`. Is your workspace ready to boot? Why does the warning
   not block?
4. Your teammate runs `doctor --fix` on their edited workspace and answers
   `y` at the overwrite prompt. What happens to their edits, and what does
   that teach about the difference between `--fix` and plain `doctor`?
5. The probe's output says `selected: parable/qwen3-fable:8b` but
   `workspace.yaml` says `name: gemma4:e4b`. Which model runs your next
   turn, and why does the probe's line exist at all?

*Answers.* 1. On a bare workspace it materializes the starter config
silently; on an initialized one it re-checks and reports. 2. Edit
`model.name` in `relurpify_cfg/workspace.yaml`; the audit trail lives in
`.relurpify_state/audit/`. 3. Yes — the row is severity `warning`, not
blocking, and the verdict line is the authority; the runner spawns on
first boot. 4. Their edits are overwritten by the shipped templates; `--fix`
is a deliberate re-materialization with a confirmation gate, while plain
doctor never overwrites. 5. `gemma4:e4b` — the workspace configuration
governs turns; the `selected:` line shows what the probe picked while
listing backend health.

## Try it yourself

1. **Fingerprint stability.** Note the `Manifest fingerprint:` line from
   your doctor run. Run `doctor --fix`, answer `y`, and run plain `doctor`
   again. Is the fingerprint the same? (It should be — `--fix` restores the
   shipped templates; it does not invent new ones. If it differs, stop and
   read the diff before proceeding.)
2. **Model swap, observed.** Change `model.name` in
   `relurpify_cfg/workspace.yaml` to another model your Ollama serves
   (`ollama list` shows them), re-run plain `doctor`, and find where the
   new name shows up in the report. Swap it back when you are done.

## Where next

You have a kitchen, a work permit, and a report that says ready. Next
chapter you hand Euclo its first order — the broken `slugify` — and watch
one complete governed turn: classification, selection, an approval banner
with your name on it, a sandboxed edit, a test run, and the two receipts
the run leaves behind. Bring the red test; it finally turns green.

The lab for this chapter lives in
`devdocs/cooking-agentic-thoughts/labs/ch01-stringkit/` — the exact
stringkit files, the captured failure, and `verify.sh ch01` to check your
seed matches the book's.
