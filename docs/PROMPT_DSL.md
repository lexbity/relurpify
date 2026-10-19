# The Prompt DSL

> **Audience**: prompt authors and recipe authors who bind prompts.
> **Purpose**: the `.prompt` file format, the registry, and how the runtime resolves prompts.
> **Prerequisites**: [Configuration](CONFIGURATION.md) for where prompts live; [RECIPE_DSL](RECIPE_DSL.md) for `import prompt`.

## What a prompt file is

A `.prompt` file is a unit of model-facing text owned by the prompt layer, not
by agent code: front matter (identity, tags, variable declarations) plus a
markdown body.
System and user prompts live in prompt files; Go code must not embed them.

- Extension: `.prompt`.
- Identity: the `id` statement in the front matter; the file path does not
  define identity.
- Encoding: UTF-8, front matter first.

## Front matter

Line-oriented statements between `---` fences:

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
```

| Statement | Form | Notes |
|---|---|---|
| `schema` | `schema framework.prompt/v2` | Exactly this value; anything else is a load error |
| `id` | `id <dotted.name>` | Required; duplicate IDs across the registry are a hard load error naming both paths |
| `tag` | `tag "word"` | Repeatable, deduplicated; used for filtering |
| `var` | `var name = "default"` | Repeatable; the default must be a quoted string; an empty default records a warning |

Unknown statements, a missing `schema`, or a missing `id` are load errors.

## Body: markdown with variable substitution

The body is CommonMark, rendered block-by-block and trimmed.

- Variable references are **single-brace**: `{name}`.
  A `{{name}}` reference parses as an invalid variable and fails validation.
- Resolution order: caller-supplied runtime variables first, then the
  declared default.
- An unknown reference (no override, no default) is a resolve error.
- Escape braces with `\{` and `\}`.
- A body line that is only a variable reference substitutes multi-line values
  fine; the substituted text is not re-parsed as markdown.

**Code spans are verbatim.**
Inline code spans render without substitution, so protect literal JSON with
them:

```text
Return ONLY JSON with fields:
`{"thought":"short reasoning","action":"tool|complete"}`
```

Unprotected braces with a non-identifier inside are invalid variable
references and fail resolution; always code-span JSON schemas.

## Placement and loading order

The registry loads, in order:

1. `<workspace>/templates/prompts/framework/`
2. `<workspace>/templates/prompts/agents/`
3. `<workspace>/templates/prompts/named/`
4. `<workspace>/relurpify_cfg/prompts/`

Missing directories are skipped.
For a normal user workspace only `relurpify_cfg/prompts/` exists: doctor
materializes the shipped corpus there (`agents/`, `framework/`, `named/`,
29 files), and materialization **never overwrites an existing file**, so your
edits win.
Re-declaring an ID that already exists in another loaded layer is a hard
error naming both paths; pick a new dotted ID instead.

## Who resolves what

| Seam | Prompt ID | Variables injected |
|---|---|---|
| React system prompt | `agent.react.default` | `tools`, `guidance` |
| React user prompt | task override, else `agent.react.default.user` | `tools`, `instruction` |
| Recipe `goal prompt <alias>` | the imported ID | step context including `instruction` |
| Chainer links | the link's prompt ID | the link's `from` keys |

Known issue: a recipe `goal prompt` currently renders the file twice and the
user's request never reaches the model; prefer
`goal """...{{ index . "task.instruction" }}..."""` until it is repaired
(see [RECIPE_DSL](RECIPE_DSL.md#run)).

Resolution is deterministic: the same inputs render the same text.
Each resolution emits telemetry (`prompt.resolved`, `prompt.resolve_failed`),
which makes a silent prompt failure visible.

## The shipped corpus

| ID | Role |
|---|---|
| `agent.react.default` | React system prompt (`{tools}`, `{guidance}`) |
| `agent.react.default.user` | React user prompt (`{tools}`, `{instruction}`; the JSON decision schema is code-span-protected) |
| `agent.{chainer,euclo,generic,pipeline}.default` | Paradigm and agent defaults |
| `rewoo.plan.v1`, `rewoo.synth.v1` | ReWOO planner and synthesizer prompts |
| `framework/*` | Framework blocks: base system, capability catalog/execution/selection, error recovery, tool formatting, validation |
| `named/euclo/*` | Euclo prompts importable from recipes |

The runtime resolves some IDs by fixed name (`agent.react.default`).
Deleting or re-id-ing such a prompt degrades the react loop to its inline
fallback and emits `prompt.resolve_failed` in telemetry; treat that line as a
regression signal.

## Validation

- Load time: malformed front matter, unknown statements, invalid identifiers,
  and duplicate IDs are hard errors; empty `var` defaults and declared-but-
  unused variables are warnings.
- Resolve time: unknown variable references and invalid reference syntax are
  errors.
- `relurplint --check prompts --workspace .` runs the file-level checks
  without a runtime.

## Authoring workflow

1. Author `relurpify_cfg/prompts/<agents|framework|named>/.../<name>.prompt`.
2. Lint: `relurplint --check prompts --workspace .`
3. If a recipe will import it, bind it there:
   `import prompt <id> as <alias>` plus `goal prompt <alias>`
   (mind the double-render caveat above).
4. Run a turn and confirm `prompt.resolved` (not `prompt.resolve_failed`) in
   `.relurpify_state/telemetry/otlp/*.jsonl` (OTLP-JSON log records; see
   [RELURPISH](RELURPISH.md#inspecting-state)).

## Changing the shipped defaults

Editing the shipped defaults is framework work: change the embedded bundle at
`userconfig/templates/embedfs/prompts/`, run `make generate-config`, and let
the drift gate verify the materialized tree.
Editing a shipped prompt in your workspace is simpler: change the
`relurpify_cfg/prompts/` copy; materialization will not overwrite it.
