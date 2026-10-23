# Chapter 6 — Line Cooks: The Linear Paradigms

Every recipe you have written binds its agent to one paradigm — `react` —
and every turn you have watched ran the same shape: think, act, observe,
repeat, bounded. It works. It is also one knife in a drawer of eight.

The recipe language declares the full set, and each paradigm is a
different shape of thinking: an iteration loop, a staged line, a chain of
isolated prompts, a review cycle, planners of three kinds, a shared
board. This chapter teaches the four *linear* ones — the paradigms whose
control flow you can draw as a line — with their mechanics verified in
the framework's own source, three real recipes you will write, and the
captured truth about what each costs on a small local model.

## Where we left off

`triage` and `gate_demo` are registered; the full toolkit — types, routes,
gates, annotations — is in your hands. Today adds the paradigm directive
grammar and a choosing table you will use for the rest of the book.

## Technical requirements

- The end-of-Chapter-5 state: binaries, green workspace, Ollama serving
  `gemma4:e4b`.
- About forty minutes. Model-dependent steps say what they cost.
- Captured on the verification machine as always — including one runtime
  failure with a named cause, one infrastructure lock error, and one
  receipt this book refuses to pretend it understands.

## What a paradigm is

A **paradigm** is a reusable reasoning strategy: the control flow an agent
runs inside a step, independent of what the step is *for*. The recipe
declares `agent inspector uses react`; the loader lowers the step into an
execution graph whose nodes think, act, and observe; the model fills the
thoughts. Eight paradigms ship: `react`, `rewoo`, `planner`, `htn`,
`reflection`, `chainer`, `blackboard`, `pipeline` — and the set is closed.
A misspelled one is a load error listing the valid set.

Two facts make this more than vocabulary. First, each paradigm is a
**contract** — data in `cognitionzoo/<paradigm>/contract.go` declaring its
directives, their grammar, their order, and the runtime behaviors it
guarantees. The DSL directive table is generated from those contracts, so
what you can write is exactly what the paradigm promises to execute.
Second, some paradigms **compose** others: reflection's contract says
`Composes: ["react"]` — its review loop runs over a react delegate. You
are not choosing eight rivals; you are choosing how they snap together.

## react, deeply (you have been running it all along)

The bounded think-act-observe loop, now with its engine exposed. The
package doc splits it across eight files with self-explanatory names:
`react_think_node.go` generates the decision, `react_act_node.go` invokes
the tool, `react_observe_node.go` captures the observation,
`react_decision.go` routes between them, `react_recovery.go` handles tool
errors, `react_completion.go` detects done.

**The decision.** Each think step asks the model for a JSON decision —
`thought`, `action`, `tool`, `arguments`, `complete` — the same JSON you
watched stream in Chapter 2's feed. Parsing that decision is where the
runtime earns its honesty.

**Decision repair.** When a decision arrives unparseable, the runtime
does not guess and does not spin silently. The captured flow, from
`react_think_node.go` as of the pin:

1. Parse the response text. Parsed with a tool or a completion? Done.
2. Thinking-model support: if the response spent everything in the
   reasoning channel, parse the decision *out of the thinking* — the
   reasoning is first-class material, never discarded.
3. If the model profile declares `repair.strategy: llm` (the shipped
   `gemma4` profile does), make one bounded repair call that asks the
   model to fix its own malformed output — and the repair must produce
   actual JSON. The source is emphatic: prose that looks like an
   apology is not blessed as a completed decision.
4. Still nothing? The step fails with the error you met in Chapter 2:
   `react decision unparseable (finish_reason=…, strategy=…)`.
5. And one comment from the source worth taping to your monitor: *an
   unparseable decision is never a completion.* A truncated think-model
   response once ended a step "successfully" with no work done; the
   guard exists so it cannot again.

**The bound.** `until <n>` caps the loop — a react-only directive, a
hard stop, a positive integer. Without it the runtime's default budget
applies. Chapter 2's "bounded" finally has its mechanism.

## Hands-on

Three recipes, one per remaining linear paradigm. Each is small by
design — Chapter 5 taught what ambition costs on a CPU.

### 1. pipeline — a plan stage and an implement stage

`relurpify_cfg/euclo/add_truncate.erpe`:

```erpe
thoughtrecipe euclo.thoughtrecipe.add_truncate
"Add a truncate function to stringkit, planned first, implemented and verified second."

trigger as capability:
  may read workspace
  may write workspace
  may invoke ["file_read", "file_list"]
  family ["implementation"]
  keyword ["truncate"]

input workspace: "**/*"
input prompt: user.request

agent planner uses pipeline
agent implementer uses pipeline

pipeline:
  stage plan:
    run planner:
      may invoke ["file_read", "file_list"]
      from input.prompt
      goal """Read stringkit.py and test_stringkit.py. State where a truncate(text, limit) function fits and which behaviors a test should assert. Do not write files. The user request is: {{ index . "task.instruction" }}"""
      capture result -> state.plan
  stage implement:
    run implementer:
      may invoke ["file_read", "file_write", "python_pytest"]
      from state.plan
      goal """Implement truncate(text, limit) in stringkit.py per the plan, add a test to test_stringkit.py, then run python_pytest on test_stringkit.py and report the real result. The plan is: {{ index . "state.plan" }}"""
      capture result -> state.implementation
```

Read the shape: stages run in document order — the contract's one
guarantee is *"stages execute in declared order"* — and the second stage
reads the first stage's capture. This is Chapter 2's R&D discipline
(reduce, delegate) rendered as DSL: planning explores in one context;
implementation starts from the clean plan, not the exploration debris.

### 2. chainer — three links, isolated prompts

`relurpify_cfg/euclo/summarize_funcs.erpe`:

```erpe
thoughtrecipe euclo.thoughtrecipe.summarize_funcs
"Summarize stringkit in three ordered links: inventory, per-function behavior, test coverage."

trigger as capability:
  may read workspace
  may invoke ["file_read", "file_list"]
  family ["investigation"]
  keyword ["summarize"]

input workspace: "**/*"
input prompt: user.request

agent surveyor uses chainer

run surveyor:
  may invoke ["file_read", "file_list"]
  from input.prompt
  link:
    from input.prompt
    prompt "You are a precise code surveyor. List every public function in the Python module named in the user content, each with a one-line purpose. Output a plain list."
    capture result -> state.inventory
  link:
    from state.inventory
    prompt "You are a precise code analyst. For each function in the inventory given in the user content, describe its behavior in one sentence and name the input class it handles (plain text, whitespace-heavy, punctuated). Output the list."
    capture result -> state.behaviors
  link:
    from state.behaviors
    prompt "You are a test-coverage reviewer. The user content lists behaviors of a string module. For each behavior, state whether it is likely covered by a test suite that focuses on slugify punctuation and whitespace handling, and name the gap if not. Output the list."
    capture result -> state.coverage
```

The chainer is a chain of isolated prompts: each `link:` block carries a
`prompt "…"` (the link's system prompt), `from` keys (fed into the
prompt context), and a `capture` (the output key). **No `goal`** — links
do not take one; the prompt clause is the instruction. Write the version
above, not the goal-shaped one the grammar tempts you toward — the
captured story of why is the next section's best lesson.

### 3. reflection — review, then revise when it says so

`relurpify_cfg/euclo/polish_docstring.erpe`:

```erpe
thoughtrecipe euclo.thoughtrecipe.polish_docstring
"Review stringkit's docstrings against the code, then revise the weakest one."

trigger as capability:
  may read workspace
  may write workspace
  may invoke ["file_read", "file_list"]
  family ["refactor"]
  keyword ["docstring"]

input workspace: "**/*"
input prompt: user.request

agent reviewer uses reflection
agent fixer uses react

run reviewer:
  may invoke ["file_read", "file_write", "python_pytest"]
  from input.prompt
  review """Read stringkit.py and test_stringkit.py. Judge whether every public function's docstring matches its actual behavior. Write a verdict listing any docstring that is wrong, stale, or missing. The user request is: {{ index . "task.instruction" }}"""
  revise when scratch.review contains issues:
    run fixer:
      may invoke ["file_read", "file_write", "python_pytest"]
      goal """Fix the docstring issues named in the review verdict, changing only docstrings and comments. Then run python_pytest on test_stringkit.py and report the real result. The verdict is: {{ index . "scratch.review" }}"""
      capture result -> state.revision
```

The reflection contract pins the mechanics: `review` writes its verdict
into the ephemeral scratch namespace — `scratch.review` and
`scratch.review_issues` — and `revise when <predicate>:` (whose body may
hold `run` or `delegate`) executes when the predicate reads true, then
re-reviews, bounded. Order is enforced: `review` before `revise`, and a
`revise` without a `review` is a load error. This is self-critique as
control flow — and note what it is *not*: not a second model
double-checking the first (that is the critic pattern, built in the
companion builder's book), but one model's structured look-before-you-
leap.

### 4. Lint — and let it catch you

```bash
~/relurpify/relurplint --check recipes --workspace .
```

On the road to these three passing, the linter caught the author once,
and the catch is permanent documentation:

```text
# polish_docstring.erpe — a nested run's agent must exist:
[error recipes.lower] ...:21:9: unknown agent "fixer"
```

`run fixer:` inside a `revise` block still needs `agent fixer uses react`
declared at the top — nesting does not inherit declarations. The other
near-miss never reached the linter because the contract system caught it
at authoring time: the first draft carried `until 2` inside the
reflection block, and reflection's contract (checked against the
DirectiveSpec table before writing) declares `review` and `revise` only —
`until` is react's directive. Reflection bounds its own loop; a foreign
directive in a paradigm block is a load error named by paradigm and
directive. Your lint should end:

```text
relurplint: all checks passed (0 diagnostics)
```

### 5. Register, pin, dispatch — and read the receipts

Restart relurpish so all three register. Pin and dispatch the chainer
(read-only, no approval expected):

```text
/recipe euclo.thoughtrecipe.summarize_funcs
```

```text
Pinned recipe for this session: euclo.thoughtrecipe.summarize_funcs (dispatch runs exactly this recipe; /recipe clear to unpin)
```

Then the task:

```text
Summarize stringkit: list its public functions, describe their behavior, and check test coverage.
```

And now the honest part — the captured session log from this chapter's
verification runs, told straight:

- **The first chainer draft failed in three seconds, with a name.** The
  draft used `goal`-shaped links (grammar-legal — the linter passed it —
  and the boot loaded it). The runtime failed the step immediately:

  ```json
  {"outcome": "failure", "llm_calls": "0",
   "outcome_kind": "execution failed: chainer link \"link\" requires a `prompt` clause"}
  ```

  This is the recipe system's three-layer validation story, now complete:
  the **lint** checks grammar, types, and declarations; the **boot**
  resolves vocabulary and prompt imports; the **runtime** enforces what
  only execution can know — here, that every link carries its prompt
  clause (`named/euclo/thoughtrecipes/chain_builder.go` names the exact
  requirement). The failure cost one second and named the fix. Compare
  that with any framework where the equivalent mistake is a silent empty
  response.
- **The corrected chainer was then rejected at the door — by
  infrastructure.** The re-run's boot reported `boot.degraded` and the
  task was rejected:

  ```text
  Cannot acquire directory lock on ".../.relurpify_state/memory/ast_index".
  Another process is using this Badger database. err: resource temporarily unavailable
  ```

  The cause: an earlier relurpish instance was still alive, holding the
  state directory's single-writer lock. **One workspace, one relurpish
  at a time.** When a task is rejected with a lock error, look for the
  zombie session before you suspect your recipe.
- **With a clean instance, the corrected chain dispatched pinned and
  completed** — and the completion receipt read
  `outcome: "success"` with `llm_calls: "0"`, which is not what three
  model-backed links should produce. This book does not print
  explanations it has not verified: the zero-call success is flagged
  `[NEEDS CONFIRMATION: chainer link execution and counters at your pin]`,
  and it is exactly the kind of anomaly your receipts now make *you*
  able to catch. Chapter 9's deterministic harness exists partly for
  this.
- **Cost expectations, stated plainly:** each react turn on the
  verification CPU ran one to five minutes per model call; a
  two-stage pipeline or a review-then-revise reflection multiplies that.
  The recipes are written and linted regardless; run them on your
  hardware and read the receipts — that practice, not this chapter's
  capture log, is the transferable skill.

## Gotchas

- **Directives are paradigm-typed.** `until` is react-only (its contract
  declares it; reflection's declares `review`/`revise`); putting a
  foreign directive in a paradigm block is a load error named by
  paradigm and directive. Check the contract before you reach for a
  directive.
- **Nested runs still declare their agents.** `run fixer:` inside a
  revise body needs `agent fixer uses react` at recipe top level.
- **Chainer links take `prompt`, never `goal`.** Grammar-legal `goal`
  links pass the lint and the boot, then fail at execution naming the
  missing clause. When the runtime names a requirement, fix the recipe
  it names — the error message is the manual.
- **One workspace, one relurpish.** The state directory is
  single-writer; a second instance degrades boot and rejects tasks with
  a Badger lock error naming the path. Kill the zombie first.
- **A paradigm is a contract, not a suggestion.** What a paradigm
  guarantees is data (`contract.go`), checked by its own conformance
  tests — when you wonder "what does reflection actually promise," the
  contract answers, and Chapter 2's `composes` insight (reflection runs
  react underneath) comes from reading it.
- **Cost scales with shape.** Stages, links, and review cycles are
  multiplicative on model time. Small local models make two-phase
  recipes a minutes-per-turn affair; that is arithmetic, not a defect.

## Summary

- Eight paradigms ship; the four linear ones — react, pipeline, chainer,
  reflection — cover loop, line, chain, and review shapes.
- react's mechanics are now yours: the JSON decision, the thinking-
  channel parse, the one bounded LLM repair, and the guard that keeps an
  unparseable decision from ever reading as completion.
- pipeline's guarantee is order; chainer's links are isolated
  prompt+from+capture units; reflection reviews into `scratch.review`
  and revises on a predicate, bounded.
- The three-layer validation story — lint, boot, runtime — catches
  different classes of mistakes, and each layer's failure names its fix.
- The receipts catch what the layers cannot: a lock rejection, and a
  success receipt that looks wrong. You are now equipped to notice both.

## Check yourself

1. Your react step ends with
   `react decision unparseable (finish_reason="stop", strategy="llm")`.
   Walk the four things the runtime tried before failing, and name the
   profile key that armed step three.
2. Why does a `revise when scratch.review contains issues:` predicate
   read `scratch.review` rather than `state.review` — and what does the
   reflection contract say lives in that namespace?
3. A teammate's pipeline's second stage does not see the first stage's
   result. What two recipe lines are probably wrong, and what does the
   pipeline contract guarantee about stages that *do* work?
4. The chainer v1 recipe passed the lint and loaded at boot, then failed
   at execution. What does that tell you about which layer checks which
   class of requirement — and why is a three-second named failure still
   better than a silently empty response?
5. A task is rejected with a Badger directory-lock error on
   `memory/ast_index`. What is the actual problem, what is the fix, and
   why does the lock exist at all?

*Answers.* 1. Parse the response text; parse the decision out of the
thinking channel (thinking models put intent there); one bounded LLM
repair call (armed by `repair.strategy: llm` in the model profile — the
shipped gemma4 profile); then the named failure. 2. The contract
guarantees authored review verdicts live in the ephemeral scratch
namespace — `scratch.review` and `scratch.review_issues` — because the
verdict is step-local material, not durable recipe state. 3. The first
stage must `capture result -> state.plan` (or any state name) and the
second must `from state.plan` (its own `from` line); the contract
guarantees stages execute in declared order — the data flow is the
recipe's job. 4. The lint checks grammar, types, and declarations
without a runtime; boot resolves vocabulary and imports against live
registries; only execution can enforce requirements that are about
execution itself (every link needs its prompt clause). A named failure
beats a silent one because it is actionable — the error *is* the fix. 5.
Another relurpish instance is still running against this workspace,
holding the state directory's single-writer lock; find and close the
zombie (or restart cleanly). The lock protects the integrity of the
embedded stores (the AST index, the event log) — two writers to a Badger
database would corrupt it.

## Try it yourself

1. **The choosing table, filled by you.** For each of the four linear
   paradigms, write one sentence naming a stringkit task it fits — and
   one it does not. Compare with the chapter's verdicts; disagreement
   with a reason is the point.
2. **The lock error, deliberately.** With a turn running in one
   terminal, start a second `relurpish chat --workspace .` in another.
   Read the doctor/boot output for the lock message, then exit the
   second instance. You have now seen the single-writer rule from the
   safe side.

## Where next

Four line cooks trained. But some work is not a line: it decomposes, or
it plans before it moves, or it needs a shared board where specialists
contribute. Chapter 7 takes the planning family — `planner`, `rewoo`,
`htn`, `blackboard` — with the same method: contract first, recipe
second, receipts always. And it answers the craft question the linear
set leaves open: when does planning beat doing?

The lab for this chapter lives in
`docs/cooking-agentic-thoughts/labs/ch06-linear-paradigms/` — all three
recipes, the captured failures and receipts, and the paradigm contract
pointers.
