# Chapter 8 — The Prompt DSL

Every turn you have run had a voice the model heard before it heard you: a
system prompt telling it what it is, which tools exist, how to answer.
Through seven chapters that voice was the shipped corpus's. You have
edited recipes, postures, policies — but the *house style*, the sentences
that shape every decision, stayed factory-set.

Not anymore. The prompt layer is files, and the files are yours. This
chapter you read the two prompts behind every react turn you have ever
run, learn the small grammar that governs them, make one deliberate edit
— and prove the runtime loaded *your* sentence, not the template's.

## Where we left off

All eight paradigms are authored, the choosing table is filled, and
`llm_calls` has taught you healthy skepticism about counters. The prompt
corpus — 29 files counted in Chapter 3 — is about to become literature
you edit.

## Technical requirements

- The end-of-Chapter-7 state: binaries, green workspace, Ollama serving
  `gemma4:e4b`.
- About twenty-five minutes. The hands-on is file-and-lint work plus one
  short dispatch; one verification note in the lab is honest about a
  capture that stayed pending.
- Captured on the verification machine as always.

## What a prompt file is

A **prompt file** is a unit of model-facing text owned by the prompt
layer — front matter (identity, tags, variables) plus a markdown body.
Extension: `.prompt`. Identity is the `id` statement inside the file —
*the path is not the identity*, which means copying a file to remix it
requires changing the `id`, exactly like a recipe header. System and user
prompts live in these files; Go code must not embed them (the project's
own rule — the prompt layer is the single source of the model's voice).

### The front matter grammar

Between `---` fences, four statements:

| Statement | Form | Notes |
|---|---|---|
| `schema` | `schema framework.prompt/v2` | Exactly this value; anything else is a load error |
| `id` | `id <dotted.name>` | Required; duplicate IDs across the registry are a hard load error naming both paths |
| `tag` | `tag "word"` | Repeatable, deduplicated; for filtering |
| `var` | `var name = "default"` | Repeatable; the default must be a quoted string; an empty default records a warning |

Unknown statements, a missing `schema`, a missing `id` — all load errors.
The grammar is small on purpose: prompt files are config-grade artifacts,
validated like config.

### The body: one brace, not two

Here is the inversion that trips every recipe author: recipe goals use
Go template braces — `{{ index . "task.instruction" }}` — but prompt
bodies use **single-brace** references: `{tools}`, `{guidance}`,
`{instruction}`. A double-brace `{{name}}` in a prompt body parses as an
invalid variable and fails validation. After two chapters of writing
`{{ index . … }}`, your fingers will fight you. Let them lose.

The rules, from the project reference and verified against the shipped
files:

- Resolution order: caller-supplied runtime variables first, then the
  declared `var` default.
- An unknown reference (no override, no default) is documented as a
  resolve error — with a captured caveat you will see below.
- Escape literal braces with `\{` and `\}`.
- Substituted text is not re-parsed as markdown.
- **Code spans are verbatim.** Inline code spans render without
  substitution — which is how the shipped user prompt protects its JSON
  schema from the brace parser. From
  `relurpify_cfg/prompts/agents/agent.react.default.user.prompt`, the
  last line verbatim:

  ```text
  Return ONLY JSON with fields:
  `{"thought":"short reasoning","action":"tool|complete","tool":"tool name or empty","arguments":{},"complete":true|false,"summary":"final answer when complete"}`
  ```

  That backticked JSON is the decision schema every react turn has
  parsed since Chapter 2. Without the code spans, the brace parser would
  eat it.

### The registry: four layers, your copy wins

Prompts load at boot, in this order:

1. `<workspace>/templates/prompts/framework/`
2. `<workspace>/templates/prompts/agents/`
3. `<workspace>/templates/prompts/named/`
4. `<workspace>/relurpify_cfg/prompts/`

Missing directories are skipped. A normal workspace has only the last:
doctor materialized the shipped corpus there (29 files — `agents/` 8,
`framework/` 8, `named/euclo/` 13, counted in Chapter 3), and
materialization never overwrites. Edit the copy in your workspace; your
edit wins. Re-declaring an ID that already exists in another layer is a
hard error naming both paths — new voice, new dotted ID.

Some IDs the runtime resolves **by fixed name** — `agent.react.default`
above all. Deleting or re-id-ing one does not crash anything: the react
loop degrades to its inline fallback and telemetry emits
`prompt.resolve_failed`. Treat that line as a regression signal, and
treat the fallback as what it is — a safety net, not a feature.

### Who resolves what

| Seam | Prompt ID | Variables injected |
|---|---|---|
| React system prompt | `agent.react.default` | `tools`, `guidance` |
| React user prompt | task override, else `agent.react.default.user` | `tools`, `instruction` |
| Recipe `goal prompt <alias>` | the imported ID | step context incl. `instruction` |
| Chainer links | the link's prompt ID | the link's `from` keys |

One known issue, taught head-on because the docs state it plainly: the
recipe form `goal prompt <alias>` **currently renders the prompt file
twice and the user's request never reaches the model**. Until it is
repaired, the working form is the one every recipe in this book already
uses — `goal """…{{ index . "task.instruction" }}…"""` with plain
interpolation. When you meet a recipe in the wild with `goal prompt`,
you now know its goals never arrived.

## Hands-on

### 1. Tour the corpus, read the voice

```bash
find relurpify_cfg/prompts -name "*.prompt" | sort
```

```text
relurpify_cfg/prompts/agents/agent.chainer.default.prompt
relurpify_cfg/prompts/agents/agent.euclo.default.prompt
relurpify_cfg/prompts/agents/agent.generic.default.prompt
relurpify_cfg/prompts/agents/agent.pipeline.default.prompt
relurpify_cfg/prompts/agents/agent.react.default.prompt
relurpify_cfg/prompts/agents/agent.react.default.user.prompt
relurpify_cfg/prompts/agents/rewoo.plan.v1.prompt
relurpify_cfg/prompts/agents/rewoo.synth.v1.prompt
relurpify_cfg/prompts/framework/base.system.prompt
relurpify_cfg/prompts/framework/capability.catalog.prompt
relurpify_cfg/prompts/framework/capability.execution.prompt
relurpify_cfg/prompts/framework/capability.selection.prompt
relurpify_cfg/prompts/framework/error.recovery.prompt
relurpify_cfg/prompts/framework/tool.formatting.prompt
relurpify_cfg/prompts/framework/validation.input.prompt
relurpify_cfg/prompts/framework/validation.safety.prompt
relurpify_cfg/prompts/named/euclo/...
```

Then read the two that run your turns:

```bash
cat relurpify_cfg/prompts/agents/agent.react.default.prompt
```

```text
---
schema framework.prompt/v2
id agent.react.default
tag "system"
tag "agent"
tag "react"
var tools = ""
var guidance = ""
---

You are a ReAct agent optimized for small local models.
Think carefully, but keep reasoning short.
Available tools:
{tools}
{guidance}
IMPORTANT: Only call tools listed above. Never invent or use tool names that are not in this list.
When information is missing, read/search before editing.
Return ONLY structured JSON. No prose outside the JSON object.
```

That is the whole system prompt — and now every feed line you have ever
watched reads differently. `{tools}` is where the runtime injects the
model-callable tool list; `{guidance}` is a runtime-injected variable;
both default to empty strings in the front matter. The strict tone ("ONLY
structured JSON") is the front line of decision parseability — Chapter
6's repair machinery exists for when even this fails.

### 2. Baseline lint

```bash
~/relurpify/relurplint --check prompts --workspace .
```

```text
relurplint: all checks passed (0 diagnostics)
```

The prompts check validates front matter and statements without a
runtime — file-level truth only.

### 3. Make the edit — your first house-style change

Edit the workspace copy of `agent.react.default.prompt` and add one
guidance line after the read/search line:

```text
When information is missing, read/search before editing.
Before editing any file, read it in full.
Return ONLY structured JSON. No prose outside the JSON object.
```

Keep it this small. A system prompt is a standing order, and standing
orders compete for the model's finite attention (the reduce half of the
context discipline — Chapter 9 formalizes it). Lint:

```bash
~/relurpify/relurplint --check prompts --workspace .
```

```text
relurplint: all checks passed (0 diagnostics)
```

Restart relurpish — prompts, like recipes, load once at boot.

### 4. Prove the runtime loaded your file

The receipts live in telemetry. Each resolution emits
`prompt.resolved`, and — a quirk worth knowing before it confuses you —
the prompt's ID rides in an attribute named `task.id`:

```json
{"task.id": "agent.react.default", "cache_hit": "false",
 "blocks_included": "0", "duration_ms": "0", "paradigm": "react"}
```

Read the fields: `cache_hit: false` on the first resolution after a
fresh boot (the render cache warms as turns run), `paradigm: react`,
and the ID you edited. The captured proof that the workspace copy is
the live one: with an intentionally broken line appended to this very
file (see the next section), a fresh boot still resolved
`agent.react.default` — from your workspace, bogus line and all. The
file you edit is the file the model gets.

### 5. The honest edges — where the docs and the build disagree

This chapter's verification surfaced two deltas between the documented
prompt behavior and the captured build, and both stay in the book as
flags rather than being smoothed over:

- **The unknown-reference resolve error did not reproduce.** The
  reference documents that a `{name}` with no override and no default is
  a resolve error. Captured reality: a body line referencing
  `{nonexistent_demo_var}` — an undeclared variable — *resolved
  successfully* (`prompt.resolved`, turn completed). The linter passed
  it too (file-level checks, resolution is runtime work).
  `[NEEDS CONFIRMATION: unknown-reference handling at your pin]` —
  until resolved, treat undeclared references as undefined behavior and
  lint your own braces.
- **`prompt.resolve_failed` never fired** — not once across the entire
  verification day's telemetry, including the broken-reference run.
  The documented regression signal may require the heavier failure
  (missing file, deleted fixed-name ID) to trigger.
  `[NEEDS CONFIRMATION: resolve_failed trigger conditions at your pin]`

And one operational warning earned the hard way during capture: after a
session of force-killed relurpish instances, boots unlocked but
dispatches stalled with *zero* telemetry. The recovery that worked: kill
every relurpish instance, clear `.relurpify_state/sessions/`, fresh
boot. Prefer graceful `ctrl+c` exits; if a dispatch goes silent, hunt
zombies first (Chapter 6's lock gotcha, subtler costume).

## Gotchas

- **Single brace in prompts, double brace in goals.** `{{name}}` in a
  prompt body is an invalid variable and fails validation; `{name}` in a
  goal is plain text. The two grammars are neighbors, not twins.
- **The lint cannot see resolution.** Undeclared references and bad
  substitutions are runtime concerns; a green `--check prompts` is
  necessary, not sufficient.
- **Identity is the `id`, not the path.** Copy-paste a prompt file to
  remix it without changing the `id`, and the registry rejects the
  duplicate at load, naming both paths.
- **`goal prompt` double-renders.** Interpolate the request into `goal`
  instead — every recipe in this book does.
- **Fixed-name IDs are load-bearing.** `agent.react.default` is resolved
  by name; deleting it silently downgrades every react turn to the
  inline fallback. If turns suddenly read oddly, check the registry
  before the model.
- **Restart after edits.** Prompts load at boot. The verification lab
  notes one more boot-related trap: force-killed instances can leave a
  workspace that boots but silently stalls dispatches — clean restart
  recovers.

## Summary

- Prompt files are config-grade artifacts: a strict front-matter grammar,
  a single-brace body language, code spans for verbatim JSON.
- The registry loads four layers and your `relurpify_cfg/prompts/` copy
  wins; fixed-name IDs are load-bearing; identity is the `id` statement.
- Resolution is deterministic and receipted — `prompt.resolved` per
  resolution, ID in `task.id`, cache behavior visible.
- Two documented behaviors (the unknown-reference error, the
  `resolve_failed` signal) did not reproduce on this build and are
  flagged, not glossed.
- Your house style now carries one standing order you wrote — and the
  runtime proved it loaded yours.

## Check yourself

1. Your prompt body needs to show the model a literal JSON object with
   braces. What protects it from the variable parser, and where in the
   shipped corpus does this appear?
2. A teammate copies `agent.react.default.prompt` to a new file,
   changes only the body text, restarts, and gets a load error naming
   two paths. What did they forget, and why is the error an error
   rather than a warning?
3. Where does the model's instruction arrive from in a react turn —
   which file, which variable — and which recipe-side line feeds it?
4. `prompt.resolved` shows `cache_hit: "true"` for a prompt that you
   edited and restarted to load. What does the cache_hit value actually
   tell you, and what would you check next to confirm your edit is in
   the rendered text?
5. Why does the book call `prompt.resolve_failed` a *regression signal*
   when this chapter could not make it fire at all?

*Answers.* 1. Inline code spans render verbatim; the shipped
`agent.react.default.user` prompt protects the react decision schema
exactly this way. 2. They forgot the `id` — identity is the statement,
not the path, so two files now claim one ID; the registry refuses at
load (naming both paths) because a duplicate would make resolution
ambiguous for every seam that resolves that ID by name. 3. From
`agent.react.default.user`, variable `{instruction}` — fed by the
runtime from the turn's task; the recipe side feeds it through the
goal's `{{ index . "task.instruction" }}` interpolation when a recipe
runs. 4. `cache_hit` says whether this resolution reused a rendered
render — it does not tell you *which version* rendered. Check that the
boot happened after the edit (prompts load at boot) and, for hard
proof, that the first resolution after restart shows `cache_hit:
"false"`. 5. Because the mechanism is documented as the prompt layer's
visible failure mode — and a signal you have never seen fire is one you
cannot yet rely on; the chapter's failure to trigger it is itself a
finding to carry until your pin resolves it.

## Try it yourself

1. **The inline fallback, safely.** Move (do not delete)
   `agent.react.default.prompt` out of `relurpify_cfg/prompts/agents/`
   to a scratch location, restart, run a read-only turn, and look for
   `prompt.resolve_failed` in telemetry and oddities in the feed (the
   inline fallback changes the model's instructions). Then move the file
   back, restart, and confirm `prompt.resolved` for the ID. You will
   have executed the documented degradation and recovery with your own
   hands.
2. **A variable of your own.** Add `var tone = "terse"` to a *new*
   prompt file with a new dotted ID, reference `{tone}` in its body,
   lint it, and reason through: who could supply a runtime override for
   `tone`, and what does the default do in the meantime? (The
   who-resolves-what table is the map.)

## Where next

Your prompts and recipes are now authored and receipted — but every
receipt so far cost model minutes, and a recipe change that silently
breaks behavior is found the expensive way: by watching. Chapter 9
closes Part III with the evidence discipline: record a turn as a tape,
write an agenttest suite that pins your recipes' behavior offline, and
re-run the whole agentic layer deterministically — the metronome that
makes prompt engineering an engineering discipline instead of a vibe.

The lab for this chapter lives in
`docs/cooking-agentic-thoughts/labs/ch08-prompts/` — the edited prompt,
the diff, the captured resolution receipt, and the two flagged deltas.
