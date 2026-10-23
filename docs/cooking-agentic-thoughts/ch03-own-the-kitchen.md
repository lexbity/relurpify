# Chapter 3 — Own the Kitchen

For two chapters you have cooked in a kitchen someone else stocked. The
model was a default. The posture dials sat wherever the template put them.
Ninety-nine tools, ten recipes, twenty-nine prompts, four security policies
— all of it materialized into your project by a command you ran once, and
all of it *yours now*.

This is the chapter where you read every shelf, flip every dial on
purpose, and put your name on the standing orders. Nothing here requires
the agent to think; every claim verifies from the command line in under a
minute. It is the calmest chapter in the book, and the one that makes
every later chapter safe.

## Where we left off

stringkit is green — `2 passed`, the slugify fixed across two audited
turns. The workspace still runs on materialized defaults, and you have
read exactly one config file region (`model:` and `selection:`) and one
model profile line. Time to open the filing cabinet.

## Technical requirements

- The end-of-Chapter-2 state: a materialized `relurpify_cfg/`, working
  binaries, Ollama serving `gemma4:e4b`.
- No new tools. About twenty-five minutes; the only model involvement is
  two thirty-second TUI boots.
- Every output below is captured from the verification machine, exactly as
  in Chapters 1–2.

## The tree, shelf by shelf

Run this and you have the whole cabinet in one screen:

```bash
find relurpify_cfg -maxdepth 1 | sort
```

```text
relurpify_cfg
relurpify_cfg/euclo
relurpify_cfg/model
relurpify_cfg/prompts
relurpify_cfg/security
relurpify_cfg/tools
relurpify_cfg/workspace.yaml
```

Six shelves plus a ledger. The captured inventory, verified by counting:

| Shelf | Contents (counted on this machine) | Governs |
|---|---|---|
| `workspace.yaml` | one file | provider, model, sandbox backend, posture dials, logging, audit, telemetry, runner |
| `model/provider/` | 4 manifests — `ollama`, `lmstudio`, `openai_compatible`, `offline` | which backends exist and how to reach them |
| `model/profiles/` | 3 profiles — `default`, `gemma4`, `qwen2.5-coder` | per-model behavior: tool-calling intent, context budget, repair |
| `security/` | 4 policies — sandbox, shell, localtool, ingestion | the fences: what may never happen |
| `tools/` | 99 manifests — `file` 6, `git` 5, `lsp` 7, `search` 3, `test` 5, `shell` 73 (in 8 category subdirectories) | everything the model may call |
| `prompts/` | 29 files — `agents/`, `framework/`, `named/` | what the model is told (Chapter 8) |
| `euclo/` | 10 recipes + `families.yaml` | what the agent does (Chapter 4) |

Count them yourself — `find relurpify_cfg/tools -name "*.yaml" | wc -l`
prints `99` here, and `ls relurpify_cfg/tools/shell/` shows the eight
shell categories (build, text, network, and friends) whose manifests sum
to 73. The numbers matter because they are the *allowlist*: if it is not
in `tools/`, the model cannot call it.

> **Note** One `tool_manifest` deserves a meet-and-greet. Peek at
> `relurpify_cfg/model/provider/ollama.provider.yaml`:
>
> ```yaml
> schema: relurpify/model/provider/v1
> name: ollama
> endpoint: http://localhost:11434
> kind: ollama
> request_timeout_seconds: 120
> available_models:
>   - gemma4:e4b
>   - gemma4:12b
>   - codellama:13b
> native_tool_calling: true
> max_concurrent: 2
> ```
>
> Every provider on your machine is a file like this. Adding one — an
> LM Studio box, a lab server with an OpenAI-compatible endpoint — is
> adding a file, not a ceremony.

### The rules every file obeys

Four validation rules, enforced at load, worth tattooing somewhere:

1. **The first line declares the schema** — `schema: relurpify/workspace/v1`
   and friends. The tag is the versioning mechanism.
2. **Unknown fields are errors.** A typo is a load failure that names the
   key, not a silently ignored line.
3. **YAML anchors and aliases are rejected outright** — the loader wants
   files a human can read linearly.
4. **Secret-like key names are banned** — `apikey`, `token`, `password`,
   `secret`, `privatekey`, `credential` fail anywhere in any config file.
   The one secret this framework has, `RELURPIFY_LLM_API_KEY`, lives in
   the environment and nowhere else.

## workspace.yaml, the steering wheel

The captured file, with the shipped comments trimmed to the lines that
steer:

```yaml
schema: relurpify/workspace/v1

paths:
  state_dir: .relurpify_state

model:
  provider: ollama
  name: gemma4:e4b

sandbox:
  backend: gvisor

selection:
  posture: deterministic

logging:
  level: info
  format: json

audit:
  retention_days: 7
  enforcement: strict

telemetry:
  enabled: true

runner:
  enabled: true
  workers: 2
  queues:
    - knowledge
```

Four regions deserve your attention now:

- **`model:`** — the turn model. `provider` must name a provider manifest;
  `name` must be served by it. This is what governs agent turns; a CLI
  `--inference-model` flag, by contrast, only sets a backend default.
- **`selection.posture:`** — dial two, below.
- **`audit.enforcement: strict`** — the shipped file's own comment says it
  best: governed actions that cannot be durably recorded *fail closed*.
  `best_effort` is the availability escape hatch; strict is the default,
  and this book never touches it.
- **`telemetry.enabled: true`** — the receipts you read in Chapter 2 are
  on by default in the materialized tree. The receipts exist because the
  switch ships on.

### The two posture dials

You met both in Chapter 1 as banner text. Here is what each actually
decides:

| Dial | Values | What it decides |
|---|---|---|
| Approvals (`--hitl-mode` flag) | `governed` (default) / `permissive` | Whether write and risky-capability requests block on a human or auto-approve with an audit record |
| Intent selection (`selection.posture`) | `deterministic` (default) / `model_assisted` | When your instruction is genuinely ambiguous: always ask you, or let a tier-2 model break the tie |

The governed/deterministic pair is the work permit from Chapter 1, stated
as configuration: *questions come to you; confident work proceeds.* This
book runs governed-deterministic everywhere except Chapter 11, where you
will shift the dials knowing the price.

## Hands-on

### 1. Get a green baseline

```bash
~/relurpify/relurplint --check all --workspace .
```

```text
relurplint: all checks passed (0 diagnostics)
```

**relurplint** is the workspace linter: config, tools, recipes, prompts,
agents — validated without booting anything, fast enough for a git hook.
From today, this is your pre-flight check whenever you touch the tree.
Exit code `0`, zero diagnostics: that is "the kitchen passes inspection."

### 2. Flip a dial, watch it move

Set the selection posture to its other value — for real, in the file you
own:

```bash
sed -i 's/posture: deterministic/posture: model_assisted/' \
  relurpify_cfg/workspace.yaml
```

Ask the runtime what posture it sees — `status` is the script-friendly
diagnostic surface, and `--format json` makes it greppable:

```bash
~/relurpify/relurpish status --format json | grep -o "model_assisted\|deterministic"
```

```text
model_assisted
```

And the banner, booted for real with the dial turned:

```text
Postures — approvals: governed; intent selection: model_assisted
```

Turn it back and capture the contrast:

```bash
sed -i 's/posture: model_assisted/posture: deterministic/' \
  relurpify_cfg/workspace.yaml
```

```text
Postures — approvals: governed; intent selection: deterministic
```

That is the whole loop of owning this kitchen: edit a file you own, probe
with `status` or `relurplint`, see the change land in the running system.
No restarts for config; the banner is read at boot, so TUI sessions pick
up posture changes on their next start.

> **Warning** The doctor's `selection_posture` row has a static streak:
> on this build it prints its `deterministic: ... always ask a human`
> explanation even while the workspace says `model_assisted`. The dial
> itself works — `status --format json` and the boot banner both tell the
> truth. Trust those two, and consider the doctor's wording there a known
> rough edge (`[NEEDS CONFIRMATION: fixed at your pin?]`).

### 3. Learn what `--fix` asks before it takes

Plain `doctor` never overwrites (Chapter 1). `--fix` *re*-materializes —
with a gate:

```bash
~/relurpify/relurpish doctor --fix --workspace . < /dev/null
```

```text
Result: ready
Overwrite current starter config and manifest from templates? [y/N]:
```

With no terminal attached, the prompt took its default — `N` — and
exited cleanly with the edits intact. That is the contract: `--fix` offers,
you confirm, and `--yes` is the unattended form. The `your-edits-win` rule
protects you from every materialization except the one you explicitly
confirm.

### 4. Know the precedence ladder — and its honest edges

The documented precedence is: **CLI flag → environment → workspace config
→ built-in default.** Parts of that ladder are demonstrated in this book:
the `--offline` CLI flag flipped the doctor's whole inference section in
Chapter 1 (provider, state, selected model), and `workspace.yaml` governs
agent turns — Chapter 2's turns ran on `gemma4:e4b` no matter what flags
trailed the command.

> **Warning — the honest edges.** Two documented environment knobs did
> not visibly move anything on this build: `RELURPIFY_OLLAMA_HOST`
> (set to a dead port, the ollama provider row still probed
> `localhost:11434` and reported ready) and `RELURPIFY_MODEL_PROVIDER=offline`
> (the doctor's inference section stayed on the workspace's `ollama`).
> The architecture comment that explains it: provider/model env and CLI
> values set a *backend default*, while the workspace configuration
> governs turns — and the doctor reports the turn-facing view. Verify any
> env override against your pin before you depend on it
> (`[NEEDS CONFIRMATION: env override behavior at your pin]`). The
> reliable overrides in this book are files and CLI flags.

Two more environment variables earn their keep without moving endpoints:
`RELURPIFY_STRICT` turns the five defaultable `workspace.yaml` keys
(`paths.state_dir`, `logging.level`, `logging.format`,
`audit.retention_days`, `telemetry.enabled`) from "filled and logged"
into "required" — a missing one becomes a load error that names it. And a
trap with a friendly name: `RELURPIFY_LOG_LEVEL` does not change log
verbosity despite looking like it should — it selects a runtime recording
mode.

### 5. Read the fences

The `security/` shelf is four small files, and each is worth reading in
full — they are short. The fence posts:

```yaml
# security/sandbox.policy.yaml (trimmed to the load-bearing lines)
protected_paths:
  - relurpify_cfg
no_new_privileges: true
image_digest: ""
```

```yaml
# security/shell.policy.yaml
rules:
  - id: deny-git-reset-hard
    pattern: '(^|\s)git\s+reset\s+--hard(\s|$)'
    reason: Destructive git reset is blocked
    action: block
```

```yaml
# security/localtool.policy.yaml
tools:
  cli_git:
    execute: ask
  bash:
    execute: ask
```

Four design rules sit under all of them, from the project's configuration
reference: the default tool posture is `ask`, and a dedicated lint gate
*rejects permissive defaults* if you try to ship one; egress to private or
loopback hosts is denied regardless of what any policy says; `.git` and
configuration roots are protected paths; and missing policy pieces fail
closed — they never open up. Notice what shipped, concretely: the shell
blacklist has exactly one rule (`git reset --hard`, blocked), and the two
sharpest tools, `bash` and `cli_git`, start at `ask`. The fence is narrow
on purpose. Chapter 7's recipes will live inside it comfortably.

### 6. The sandbox image, unpinned

The second informational warning from Chapter 1 deserves its explanation
now. `runtime_image: degraded` means the sandbox container image is
referenced by tag, resolved from your local daemon at each boot — so a
re-tagged image upstream could change what runs under your agent. The
policy file's own comment states the contract:

```yaml
# Digest-pin the sandbox runtime image (SBH-1 D-13). Leave empty to resolve
# the ref digest from the local daemon at boot; set sha256:… to state it.
image_digest: ""
```

Pinning is setting that field to your image's `sha256:…` digest. On the
verification machine the local sandbox image was built locally and carries
no registry digest to pin, so this remains a reader exercise — `docker
images --digests` shows you what your daemon holds. Leave it empty and the
warning stays informational; pin it and the warning clears. Nothing in
this book depends on either choice.

> **Note — the offline backend, where it belongs.** A fifth provider
> ships in `model/provider/offline.provider.yaml`: a deterministic
> scripted model that needs no daemon, no Docker, and no network
> (`relurpish doctor --offline`, `relurpish chat --offline`). It exists
> for plumbing checks and CI. This book uses real models everywhere
> except Chapter 9, where its determinism becomes the whole point.

## Gotchas

- **The doctor's `selection_posture` row text is static** on this build —
  it describes `deterministic` even when the dial says otherwise. Verify
  posture with `status --format json` or the boot banner.
- **`--fix` + `y` overwrites your edits, by design.** The
  your-edits-win rule guards automatic materializations, not confirmed
  ones. If you keep precious recipes (you will, from Chapter 4), keep
  them in git too.
- **A typo is a load error, not a silent default.** Unknown fields fail
  loudly, schema tags are mandatory, and that is a gift: the tree cannot
  rot quietly.
- **You cannot configure your way to permissive.** Default tool posture
  `ask` is lint-enforced; egress to private and loopback hosts is denied
  above the policy layer; `.git` and `relurpify_cfg` are protected paths.
  The fences are not configuration — they are law.
- **`tools/` is not flat.** Ninety-nine manifests: six categories in
  plain sight, and the 73 shell tools inside eight category
  subdirectories under `tools/shell/`.
- **`RELURPIFY_LOG_LEVEL` does not do what its name says**, and the env
  provider/model overrides did not visibly move the doctor on this build.
  Files and CLI flags are this book's reliable levers.
- **`RELURPIFY_STRICT` exists for the paranoid**, and the paranoid are
  right: with it set, a trimmed `workspace.yaml` produced
  `Result: blocking issues detected` in a captured run. Restore the keys,
  and it was `Result: ready` again.

## Summary

- The cabinet has six shelves and a ledger: workspace steering, provider
  manifests, model profiles, security policies, the 99-tool allowlist,
  the 29-file prompt corpus, and ten shipped recipes.
- Every file obeys four rules — schema first, unknown fields error,
  anchors rejected, secrets banned — and `relurplint` inspects it all
  without a boot.
- The two posture dials are yours: flip, probe with `status`, see the
  banner change on next boot.
- Precedence is CLI → env → workspace → default in the docs; this book
  proves the CLI and workspace rungs and flags the env rung as
  verify-first.
- The fences ship narrow and fail closed: one blacklisted shell pattern,
  two ask-postured tools, protected paths, and a lint gate that rejects
  permissiveness. You own the standing orders; the laws are not for sale.

## Check yourself

1. You want the agent to use a model called `coder:13b` that your Ollama
   serves. Which file and which two keys change — and what must be true
   about the provider manifest for the boot to succeed?
2. What makes `relurplint --check all` suitable as a git hook or a CI
   step, given that it never boots the runtime or touches a model?
3. A teammate proposes `audit.enforcement: best_effort` "for speed." What
   does strict mode currently guarantee that best_effort gives up, and
   what would Chapter 2's receipts look like under best_effort?
4. Which is this book's reliable way to run one doctor diagnosis against
   the offline backend — an environment variable or a CLI flag — and what
   did the captured evidence show for the other one?
5. Why does the framework ban secret-like key names in config files
   outright, and where does the one allowed secret live?

*Answers.* 1. `relurpify_cfg/workspace.yaml`, keys `model.provider` (must
stay/match `ollama`) and `model.name: coder:13b` — and `coder:13b` must
appear in the provider manifest's `available_models` (and be served by the
daemon). 2. It validates the whole tree — grammar, schemas, known keys, banned
   names — statically: no boot, no model, no sandbox, fast, with a CI-friendly
   exit code. 3. Strict fails closed — a governed action that cannot
be durably recorded blocks the action; best_effort counts the drop and
proceeds, so receipts can have silent holes on exactly the days
everything is on fire. 4. The CLI flag (`--offline`) — it demonstrably
flipped the doctor's inference section in Chapter 1, while the
`RELURPIFY_MODEL_PROVIDER` env knob showed no visible delta on this
build. 5. Because configuration files are the thing you copy, commit, and
share; a key named `token` invites a pasted secret. The one secret,
`RELURPIFY_LLM_API_KEY`, is read from the environment by the single
configuration boundary that may read environment variables at all.

## Try it yourself

1. **Strict mode, felt.** Delete the `logging:` block from a *copy* of
   your `workspace.yaml`, set `RELURPIFY_STRICT=1`, and run plain doctor.
   On the verification machine this produced
   `Result: blocking issues detected` and an error naming the problem;
   restore the block and confirm `Result: ready`. Then try it *without*
   `RELURPIFY_STRICT` and watch the missing keys fill in silently — that
   contrast is the whole feature.
2. **Provider tour.** Run
   `~/relurpify/relurpish doctor --set-provider offline --workspace .`
   and read what changes in the report (the workspace file changes too —
   inspect it). Switch back to ollama the same way. You have now driven
   every provider transition this book needs without editing a file.

## Where next

The kitchen is inspected, the dials are yours, and the fences are read.
Now for the part the book title promised: in Chapter 4 you write your
first thoughtrecipe — a `.erpe` file with a trigger, a goal, and a
capture — pin it, run it, and prove from telemetry that the runtime chose
it *because you said so*. The ten shipped recipes stop being cargo and
become your reference corpus.

The lab for this chapter lives in
`docs/cooking-agentic-thoughts/labs/ch03-workspace-baseline/` — the
annotated `workspace.yaml`, the captured inventory and lint run, and both
posture banners.
