# The Recipe DSL (thoughtrecipes)

> **Audience**: authors of agent workflows.
> **Purpose**: the complete syntax and semantics of `.erpe` recipe files.
> **Prerequisites**: a workspace with `relurpify_cfg/euclo/` ([Getting started](GETTING_STARTED.md)); tool-name resolution is described in [Configuration](CONFIGURATION.md#tool-manifests).

## What a recipe is

A recipe is a single declarative file describing one agentic workflow: how it
is triggered, what tool vocabulary its steps may use, which paradigms execute,
which capabilities are invoked, and how state flows between steps via
`capture`.

Two boundaries matter:

- Recipes are **not programs**: no loops, recursion, functions, or arbitrary
  expressions.
- Recipes are **not grants**: `may ...` lines declare vocabulary and intent.
  The permission system, sandbox, and policies decide what actually happens.

## Files and loading

- Location: `<workspace>/relurpify_cfg/euclo/`, top-level files only
  (subdirectories are warned and skipped).
- Extensions: `.erpe`, `.euclo`, `.thoughtrecipe` (cosmetic; identity is the
  `thoughtrecipe <name>` header). Only `.erpe` files ship.
- Load pipeline: parse -> type check -> symbol resolution -> lowering ->
  tool-vocabulary validation -> registration.
- Any failure fails the boot and names the recipe, step, and construct.
- The first recipe with a given name wins; later duplicates become load
  warnings.
- Recipes load once at boot; restart relurpish after editing, and check
  `/recipe` for skipped files.
- A missing recipe directory is a boot error; doctor materializes it.

Validate without booting the runtime:

```bash
relurplint --check recipes --workspace .
```

Caveat: the lint does not resolve tool vocabulary or prompt imports (no
registry is wired), so a lint-clean recipe can still fail at boot on those.

## Anatomy

```erpe
thoughtrecipe euclo.thoughtrecipe.explain_test
"Read a named test file and explain in plain language what behavior it covers."

trigger as capability:
  may read workspace
  may invoke ["file_read", "file_list"]
  family ["review"]

input workspace: "**/*"
input prompt: user.request

agent explainer uses react

run explainer:
  may invoke ["file_read", "file_list"]
  from input.prompt
  goal """Read the test file the user names, then explain in plain language
  what behavior it covers and what a bug in the code under test would break.
  The user request is: {{ index . "task.instruction" }}"""
  capture result -> state.explanation
```

Required: exactly one `thoughtrecipe` header, exactly one `trigger as` block,
and at least one execution construct (`run`, `pipeline`, or `route`).
Recommended order: header, description, trigger, inputs, types, agents,
execution body.

Lexical rules: `#` starts a comment; strings are `"short"` or
`"""multi-line"""`; inline lists are `["a", "b"]`; indent two spaces per
level; identifiers match `[a-zA-Z_][a-zA-Z0-9_]*`.

## The trigger block

```erpe
trigger as capability:
  may read workspace
  may write workspace
  may ask user
  may invoke ["file_read", "file_list", "grep", "file_write"]
  family ["debug"]
  keyword ["fix", "repair", "failing", "test"]
  handoff ["onboarding"]
```

- `trigger as <policy>` sets the route kind; `capability` is the shipped
  convention.
- `may <effect> <resource>` lines (read/write workspace, ask user) are
  trigger-only effect declarations.
  `may ask user` is **required** if the recipe contains any `ask user` block.
- `may invoke ["tool", ...]` declares recipe-level tool vocabulary.
  It is the only `may` policy allowed inside `run` blocks.
- `family`, `keyword`, and `handoff` lists feed selection and handoff
  resolution.

### The families handoff table

`relurpify_cfg/euclo/families.yaml` maps classified intent families to
handoff recipes; it is workspace data, added once by materialization and
never overwritten:

```yaml
schema: relurpify/euclo/families/v1
handoffs:
  debug: euclo.thoughtrecipe.debug_tdd_repair
  review: euclo.thoughtrecipe.code_review
```

- Keys are lowercased families; values are registered recipe IDs.
- A mapped family whose target is not registered is a **boot error**.
- An unmapped family has no boot constraint; a handoff for it degrades to a
  named runtime failure.
- The classifier emits nine built-in families: `debug`, `repair`, `planning`,
  `review`, `implementation`, `refactor`, `migration`, `investigation`,
  `architecture`.
  The shipped table maps seven of them.
  Custom families are reachable only by pinning the recipe; the linter warns
  at authoring time.

## Inputs and types

```erpe
input workspace: "**/*"
input prompt: user.request
```

Inputs are read-only values referenced as `input.<name>`;
`input prompt: user.request` binds the user's request.

```erpe
type Finding:
  title: Text
  severity: low | medium | high
  evidence: list<Text>
```

Primitives: `Text`, `Markdown`, `Json`, `Bool`, `Number`, `Percent`, `File`,
`Path`, `Workspace`, `Diff`.
Compound: `list<T>`, `map<K, V>`, `optional<T>`; inline unions like
`low | medium | high`.
Types are checked on typed captures; type and input names are recipe-local.

## Agents

```erpe
agent repairer uses react
```

Paradigms: `react`, `rewoo`, `planner`, `htn`, `reflection`, `chainer`,
`blackboard`, `pipeline`.
An unknown paradigm is a load error listing the valid set; multiple agents may
share a paradigm.

## Execution constructs

### run

```erpe
run repairer:
  may invoke ["file_read", "file_write"]
  from input.workspace
  stream "symbols relevant to the request" max 256 mode blocking
  goal """Repair the module. The user request is: {{ index . "task.instruction" }}"""
  capture result -> state.repair
```

- `may invoke [...]`: the step vocabulary.
  The effective step scope is the union of the trigger-level and step-level
  declarations; every name must resolve in the registry at load, or the boot
  fails naming recipe, step, and tool.
- `from <expr>`: input sources; repeatable.
- `goal "<text>"`: the step instruction, rendered as a Go `text/template`.
- `stream "<query>" [max <n>] [mode <blocking|background>]`: retrieves
  knowledge chunks into the step's working set; omit it for runtime defaults.
- `until <n>`: react-only iteration cap (a positive integer).

**Always interpolate the request** with `{{ index . "task.instruction" }}`;
a goal that drops it leaves the agent blind.
Prefer `goal` over `goal prompt <alias>`: the prompt-bound form currently
renders twice and the user request does not reach the model.

### capture — the only state transition

```erpe
capture result -> state.summary          # inline, untyped

capture:                                  # block, typed
  finding: Finding -> state.finding
  severity: low | medium | high -> state.severity

capture:                                  # forwarding
  state.findings -> state.plan
```

Suffix `as claimed | as given` annotates the provenance strength of a binding.
State changes only through `capture`; predicates and `from` lines read it.

| Namespace | Purpose |
|---|---|
| `input.*` | External values declared by `input` |
| `state.*` | Durable recipe-level state |
| `scratch.*` | Temporary local state |
| `user.*` | Answers from `ask user` interactions |
| `output.*` | Final exported recipe outputs |

### route

```erpe
route:
  when state.severity is high:
    delegate to planner:
      ...
  when state.confidence confidence below 40:
    run fixer:
      ...
  when missing state.findings:
    ...
  otherwise:
    capture:
      state.finding -> state.plan
```

Branches evaluate in document order; first match wins; `otherwise` is the
fallback; no match without `otherwise` is a runtime failure.
The complete predicate grammar:

```text
when <path> is <value>
when <path> contains <value>
when <path> confidence below <number>
when missing <path>
when present <path>
```

`is ambiguous` is not a reserved form; it parses as an ordinary value
comparison.
Boolean combinations are not supported.

### delegate, ask user, pipeline, import, do

```erpe
delegate to planner:
  from state.finding
  goal "Plan the revision."
  capture:
    plan: Text -> state.plan

ask user:
  question "Proceed with the recommended change?"
  choices ["yes", "no", "revise"]
  capture answer -> state.decision

pipeline:
  stage explore:
    run explorer:
      ...
  stage plan:
    run planner:
      ...

import prompt named.demo.tdd_repair as tdd_goal

do relurpic:test_run on state.repair
```

- `delegate` creates a child execution for a declared agent; the child writes
  back only through `capture`; nesting is supported.
- `ask user` requires `may ask user` in the trigger; `choices` is a static
  list or `choices from state.<name>`.
  The turn pauses on an interaction frame and resumes with your answer.
- `pipeline` stages execute in document order.
- `import prompt` binds a prompt ID for `goal prompt <alias>`; `import recipe`
  binds a recipe anchor.
  See [PROMPT_DSL](PROMPT_DSL.md) for prompt files.
- `do <capability>` invokes a governed `relurpic` capability:
  `on <expr>` supplies input, `with <expr>` additional arguments.
  Namespace is `relurpic`; bare names resolve (`test_run` ->
  `euclo:cap.test_run`).
  A `relurpic` capability is dispatch-only: it never appears in the model's
  tool list.

Known limitation: there is no load-time check that a `do` target satisfies the
capability's input schema, and no DSL form produces a literal map argument, so
capabilities requiring literal inputs are not yet authorable.
`test_run` is one of these — it needs a literal command — so a recipe cannot
yet express it through `do`. For verification, use the model-callable
test-runner manifest tools instead (see
[Tool vocabulary](#tool-vocabulary)); the shipped recipes do.

## Paradigm directives

Directives are valid only inside `run`/`delegate` blocks of an agent whose
paradigm declares them; violations are load errors named by paradigm and
directive.

| Paradigm | Directives | Constraints |
|---|---|---|
| `react` | `until <n>` | Bounded think-act-observe loop; the cap is a hard stop |
| `planner` | `plan "..."`, `step "...":` (body: `do`), `verify "..."`, `summarize "..."` | Order: plan -> step -> verify -> summarize; `step` requires `plan` |
| `rewoo` | `plan "..."` (required), `step "...":` (body: `do`), `synthesize "..."` | Order: plan -> step -> synthesize |
| `htn` | `method "...":`, `task "...":` (body: `do`) | `task` requires `method` |
| `reflection` | `review "..."`, `revise when <predicate>:` (body: `run`/`delegate`) | `revise` requires `review` |
| `chainer` | `link:` (repeatable, required; no name argument) | Links run in declared order; capability/step items inside a link are errors |
| `blackboard` | `source <name>:` (repeatable) | Shared-workspace sources |
| `pipeline` | none in run blocks | Use the top-level `pipeline:` construct |

Block directives need an indented body; a line form where a block is declared
is a load error.

## Tool vocabulary

Two deliberately different vocabularies:

- **Manifest tools** (`file_read`, `file_write`, `grep`, `cli_*`, `lsp_*`,
  ...) are model-callable: when in a step's scope they appear in the model's
  tool list and run through governed paths.
- **`relurpic` capabilities** are dispatch-only: they run via `do` steps and
  are never shown to the model.
  Declaring one in `may invoke` resolves but the model never sees it.

Resolution order for a declared name: exact capability ID, then the registry
name index (manifest names, bare invocable-capability names, curated aliases
such as `grep` -> `file_search`), else a load error.

### Verification vocabulary

The shipped `test/` manifests are model-callable runners:
`python_pytest`, `python_unittest`, `go_test`, `node_npm_test`,
`rust_cargo_test`. They take typed, constrained inputs (a working directory and
an optional path or name filter) and run a fixed command — the model chooses
neither the command nor the binary.

To verify an edit, declare the runner that matches the project in the step's
`may invoke` and tell the agent to run it before reporting complete:

```erpe
run repairer:
  may invoke ["file_read", "file_write", "python_pytest"]
  goal """Repair the defect, then run python_pytest to prove the suite passes
  before reporting complete. The user request is: {{ index . "task.instruction" }}"""
```

Unlike the dispatch-only `test_run` capability, these runners appear in the
model's tool list, so a react step can actually invoke them.

## Execution semantics

- The loader lowers a recipe into an execution graph: linear sections in
  document order, route groups as first-match conditional branches, one
  terminal node.
- Step scope is `(trigger may invoke union step may invoke)`, resolved at
  load, then intersected with callable and available tools.
- Steps fail operationally: the failure is named in telemetry and the result,
  and the default step-error policy aborts the route.
  There is no `on_error` syntax; express recovery with
  `route: otherwise:` branches.
- Recipes are atomic except at `ask user` pauses.

## Authoring workflow

1. Create `relurpify_cfg/euclo/<name>.erpe`; start from a shipped recipe.
2. Lint: `relurplint --check recipes --workspace .`
3. Restart relurpish; boot resolves vocabulary and prompt imports against the
   live registries, and failures name the artifact.
4. Confirm selection with telemetry (OTLP-JSON log records under
   `.relurpify_state/telemetry/otlp/`):
   `jq -r '.. | .logRecords[]? | select(.eventName=="euclo.route.selected")' .relurpify_state/telemetry/otlp/*.jsonl`.
5. If a family should hand off cleanly, map it in `families.yaml`.

For a turn you control entirely, pin the recipe with `/recipe <id>`; see
[RELURPISH](RELURPISH.md#working-with-recipes).
