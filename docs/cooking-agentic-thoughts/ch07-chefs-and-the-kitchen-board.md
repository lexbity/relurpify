# Chapter 7 — Chefs and the Kitchen Board: Planning Paradigms

A line cook executes. A chef decides *what* to execute — and sometimes the
decision is the deliverable: the plan itself, checked and signed, before
any pan is hot.

The remaining four paradigms are the planning family. Where the linear
set (Chapter 6) draws lines and loops, these draw decompositions:
`planner` produces a plan and checks it, `rewoo` plans every tool call up
front so the model reads the world once, `htn` decomposes tasks into
methods, and `blackboard` lets specialists contribute to a shared board
in whatever order the board dictates. Same method as last chapter —
contract first, recipe second, receipts always — plus the craft question
this family exists for: **when does planning beat doing?**

## Where we left off

The linear drawer is stocked: `add_truncate`, `summarize_funcs`,
`polish_docstring` — linted, registered, with a three-layer validation
story and a lock-error scar to show for it. Today finishes the drawer.

## Technical requirements

- The end-of-Chapter-6 state: binaries, green workspace, Ollama serving
  `gemma4:e4b`.
- About thirty-five minutes; the one dispatch run is time-boxed and its
  outcome — a governance catch — is captured below.
- Captured on the verification machine as always.

## The four contracts

Each paradigm is a contract — data declaring its directives, their
grammar, their order. Read straight from `cognitionzoo/<paradigm>/contract.go`
as of the pin:

| Paradigm | Shape | Directives (order enforced) | Composes |
|---|---|---|---|
| `planner` | PlanExecute | `plan "…"` → `step "…":` → `verify "…"` → `summarize "…"` | — |
| `rewoo` | StagedPlan | `plan "…"` (required) → `step "…":` → `synthesize "…"` | — |
| `htn` | Decomposition | `method "…":` → `task "…":` (requires method) | react |
| `blackboard` | Blackboard | `source <name>:` (repeatable) | — |

Facts the contracts add beyond the grammar:

- **A `step` block's body is `do` only** — in planner, rewoo, and htn.
  The step legs invoke *capabilities* (`do relurpic:…`), not model tools.
  And here the honest boundary from Chapter 5 returns: there is no
  load-time check that a `do` target's input schema is satisfied, and no
  DSL form produces literal map arguments — so capabilities that need
  literal inputs (the docs name `test_run`) are not authorable today.
  This is why the shipped `plan.erpe` carries no steps: its plan
  document *is* the product.
- **htn composes react** — tasks execute "in-envelope over a react
  primitive executor." Decomposition on the outside, the familiar
  bounded loop on the inside.
- **Blackboard authored sources replace the built-in specialist set.**
  From `authored_source.go`: sources run as a controller loop in
  declaration order, each with an optional `when` eligibility predicate,
  `read` state keys, a required `write` state target, and an optional
  `capability` pin. And the sentence that explains a whole class of
  receipts: *authored structure is authoritative — with no model calls
  beyond what a source's own execution performs.* A no-`do` source is a
  data-flow skeleton, not a conversation.

## When planning beats doing

A choosing table you can argue with — that is the point:

| Reach for | When | Why |
|---|---|---|
| `planner` | The deliverable is the plan, or execution is a separate, reviewed act | plan → verify → summarize is a checkpoint machine; a human can grade the plan before any step runs |
| `rewoo` | The task needs several tool calls whose *shape* is knowable up front | one planning pass, steps that depend only on earlier results, one synthesis — the model reads the world once instead of thinking-acting-observing its way into context bloat |
| `htn` | The work decomposes into named subtasks with known structure | methods and tasks are the recipe's own work breakdown structure, executed by react underneath |
| `blackboard` | The shape of the work is not knowable up front and specialists should react to what the board holds | data-driven order: each source runs when its `when` predicate says the board is ready for it |

And the negative space: planning beats doing when the plan is cheap
relative to the mistakes it prevents. For a one-function fix in a
two-file repo, react *is* the right tool — planning it first is
ceremony. The shipped corpus agrees: `plan.erpe` serves the `planning`
family and never writes; `debug_tdd_repair` serves the fixers.

## Hands-on

Three recipes, lint-first. All three passed the linter on the first
attempt — Chapter 6's scars at work — including the blackboard clause
forms (`read [state.x]`, `write state.x`), which the linter confirms at
grammar level.

### 1. planner — plan_repair

Modeled directly on the shipped `plan.erpe` (read it first —
`relurpify_cfg/euclo/plan.erpe` — it is the reference implementation of
the form). `relurpify_cfg/euclo/plan_repair.erpe`:

```erpe
thoughtrecipe euclo.thoughtrecipe.plan_repair
"Produce a verified repair plan for a failing test in stringkit: plan, verify, summarize. Planning never writes."

trigger as capability:
  may read workspace
  may invoke ["file_read", "file_list", "file_search"]
  family ["planning"]
  keyword ["plan repair", "repair plan"]

input workspace: "**/*"
input prompt: user.request

agent planner uses planner

run planner:
  may invoke ["file_read", "file_list", "file_search"]
  from input.prompt
  plan """Produce a concrete, ordered repair plan for the failing test described by the user. Ground every step in the real files and functions of this workspace. The user request is: {{ index . "task.instruction" }}"""
  verify "Check that every step names a real file or function in stringkit and that the plan ends with running the test suite to confirm the repair."
  summarize "Produce the final ordered repair plan, leading with the first step the user should take."
  capture result -> state.plan
```

No `step` blocks — deliberately. The plan is the product; Chapter 10's
plan/build pair will consume it. The `verify` line is where the planner
earns its keep: the model checks its own plan against a stated standard
before summarizing.

### 2. rewoo — rewoo_outline

`relurpify_cfg/euclo/rewoo_outline.erpe`:

```erpe
thoughtrecipe euclo.thoughtrecipe.rewoo_outline
"Plan the investigation steps up front, execute nothing, synthesize the outline. ReWOO separates planning from observation."

trigger as capability:
  may read workspace
  may invoke ["file_read", "file_list"]
  family ["investigation"]
  keyword ["rewoo", "outline"]

input workspace: "**/*"
input prompt: user.request

agent rewooer uses rewoo

run rewooer:
  may invoke ["file_read", "file_list"]
  from input.prompt
  plan """For the user's request, list the tool calls an investigator should make, in order, with each step depending only on earlier results. The user request is: {{ index . "task.instruction" }}"""
  synthesize "Turn the plan and any step results into a concise investigation outline for the user."
  capture result -> state.outline
```

`plan` is the one required directive; steps are optional (and `do`-only
when present); `synthesize` closes the shape. Without steps, rewoo is a
plan-and-synthesize instrument — the form you want when the *outline* is
the artifact.

### 3. blackboard — board_survey

`relurpify_cfg/euclo/board_survey.erpe`:

```erpe
thoughtrecipe euclo.thoughtrecipe.board_survey
"Two authored blackboard sources: explorer surveys stringkit, analyst reviews the findings. Authored sources are the data-flow skeleton; a no-do source performs no model calls."

trigger as capability:
  may read workspace
  may invoke ["file_read", "file_list"]
  family ["investigation"]
  keyword ["board"]

input workspace: "**/*"
input prompt: user.request

agent board uses blackboard

run board:
  may invoke ["file_read", "file_list"]
  from input.prompt
  source explorer:
    read [input.prompt]
    write state.survey
  source analyst:
    read [state.survey]
    write state.review
```

Read it as the architecture, not a conversation: explorer's output lands
in `state.survey`; analyst reads the board and writes `state.review`.
With no `capability` pinned and no `do` leg, neither source performs
model calls — this recipe is the skeleton, and it lints, registers, and
dispatches as exactly that. The built-in specialist lifecycle
(Explorer → Analyzer → Planner → Review → Executor → FailureTriage →
Verifier → Summarizer, per the package doc) shows what a fully-staffed
board looks like; wiring your own specialists to capabilities is builder's
book territory.

### 4. Dispatch plan_repair — and watch governance make the catch

Restart, pin, dispatch:

```text
/recipe euclo.thoughtrecipe.plan_repair
```

```text
Pinned recipe for this session: euclo.thoughtrecipe.plan_repair (dispatch runs exactly this recipe; /recipe clear to unpin)
```

```text
Plan the repair for a failing slugify test: the function keeps punctuation.
Produce an ordered, verified plan.
```

The captured session's receipt chain, in order:

```json
{"decided_by": "pinned", "node.id": "euclo.dispatch"}
{"node.id": "run.15.5.planner.0.execute"}
{"message": "node planner_execute execution failed: tool file_read blocked: input schema invalid: $.path required"}
{"outcome": "failure", "llm_calls": "0",
 "outcome_kind": "execution failed: node planner_execute execution failed: tool file_read blocked: input schema invalid: $.path required"}
```

Read that middle line slowly, because it is the governance system doing
its job in front of you: the planning model generated a `file_read` tool
call, the tool manifest's schema requires a `path` argument, the model
omitted it — and the **manifest schema validation blocked the call**,
naming the exact JSON path (`$.path required`). The default step-error
policy aborted the route, and the failure surfaced with its full cause.
No silent garbage in, no confident garbage out.

Two things to take from the capture:

- **The fix is a retry or a better model.** `/retry` re-rolls; a model
  that emits complete tool arguments passes. Schema-blocked tool calls
  are the system working, not the system failing.
- **The `llm_calls` counter reads `0` again** — as it did for the
  chainer in Chapter 6, while react turns counted their calls (Chapter
  2's five). Two paradigms, same symptom: the counter appears
  react-bound as of this pin. `[NEEDS CONFIRMATION: llm_calls counting
  for non-react paradigms at your pin]` — treat benchmark-style counters
  for planner/chainer/htn/blackboard runs as unverified until that is
  resolved.

## Gotchas

- **`step` legs are `do`-only** in planner, rewoo, and htn — and
  capabilities needing literal inputs are not authorable today (no DSL
  form produces a map argument). Plan-and-report recipes like the
  shipped `plan.erpe` are the working form; execution legs belong to
  `run` blocks in routes or pipelines.
- **htn's `task` requires `method`**, and both are blocks, not lines —
  the D1 mixing rule from Chapter 5's contract reading applies here too:
  authored structure without its governing objective is a load error.
- **Blackboard `write` is mandatory per source.** A source that writes
  nothing is not a specialist; the linter and the lowering both insist.
- **Authored sources replace the built-ins wholesale.** Your two-source
  board does not run *alongside* the Explorer→…→Summarizer lifecycle —
  it replaces it for that execution.
- **Counter skepticism.** `llm_calls` reads 0 for planner and chainer
  runs as of this pin; do not benchmark on it across paradigms.
- **Planning never writes by convention.** The shipped planning recipe's
  description says so; your planning recipes should keep the discipline
  — read-only vocabulary in the trigger is the enforceable form.

## Summary

- Four contracts, four shapes: plan-execute, staged-plan,
  decomposition-over-react, and a specialist board.
- The `do`-only step body and the literal-input limitation explain the
  shipped corpus's design: planning recipes produce plans; execution
  lives in runs, routes, and pipelines.
- Blackboard authored sources are a data-flow skeleton — declaration
  order, eligibility, read/write lanes — with model calls only where a
  source's own execution puts them.
- The captured plan_repair dispatch shows governance catching a
  malformed tool call at the schema layer, naming `$.path`, and failing
  honestly — the system working.
- The choosing table is a starting position: plan when the plan is the
  product or the checkpoint is cheap; do when the task is small; board
  when the work's shape is data-driven.

## Check yourself

1. Why does the shipped `plan.erpe` contain no `step` blocks, and what
   would a step block require that the DSL cannot currently give it?
2. In `board_survey`, what happens during dispatch if you swap the two
   sources — `analyst` before `explorer` — and why does the contract
   make that legal rather than a load error?
3. Your rewoo recipe's `plan` directive is missing entirely, but `step`
   and `synthesize` are present. What happens, and which contract field
   decides it?
4. The planner's model emitted a `file_read` without `path`. Name the
   layer that blocked it, the exact diagnostic it produced, and the two
   operator responses the chapter offers.
5. `llm_calls` read 5 for a react turn and 0 for planner and chainer
   turns. What should you conclude — about the turns, and about the
   counter?

*Answers.* 1. Its plan document is the product (read-only planning
family), and a step block's body is `do`-only — capabilities whose input
schemas need literal arguments (like `test_run`'s command) are not
authorable today. 2. Sources run in declaration order with per-source
eligibility; analyst would read `state.survey` before explorer wrote it
— an empty read context, legally. The contract enforces order of
*eligibility*, not correctness of *dataflow*; the recipe author owns the
lane wiring. 3. It is a load error: rewoo's contract marks `plan`
`Required: true` — authored structure without its governing objective is
rejected, the same D1 principle Chapter 5 applied to reflection. 4. The
tool-manifest schema validation layer, at execution; the diagnostic is
`tool file_read blocked: input schema invalid: $.path required`; the
responses are `/retry` (re-roll the model's call) or use/switch to a
model that emits complete arguments. 5. About the turns: nothing — the
counter is the liar here. About the counter: as of this pin it appears
react-bound (5 on react turns, 0 on planner and chainer turns); flag it,
do not benchmark across paradigms with it.

## Try it yourself

1. **The board with teeth.** Add a `when` eligibility predicate to
   `analyst` (the route predicate grammar applies: `when state.survey is
   …` or `when missing …`), lint, and reason through which cycles each
   source is eligible in. Then pin and dispatch, and read the step
   events for the cycle count.
2. **Method, then tasks.** Write an `htn` recipe with one `method` and
   two `task` blocks whose bodies you leave empty. Lint it, read the
   error (both are blocks that require bodies), and then decide: given
   the `do`-only body rule, what *could* a legal htn task body invoke
   today? Write the answer in one sentence next to the recipe.

## Where next

The drawer is full: eight paradigms, every directive, a choosing table
earned from contracts and receipts. But a recipe is still a single card
pinned for one turn. Chapter 8 opens the prompt DSL — the files that
shape what every one of these paradigms says to the model — and your
first edited system prompt goes live with a `prompt.resolved` receipt to
prove it.

The lab for this chapter lives in
`docs/cooking-agentic-thoughts/labs/ch07-planning-paradigms/` — the three
recipes, the captured governance catch, and the contract pointers.
