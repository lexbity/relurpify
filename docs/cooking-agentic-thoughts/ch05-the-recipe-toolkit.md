# Chapter 5 — The Recipe Toolkit

Chapter 4's recipe had one idea: read a thing, explain it. Real kitchen
work is not one idea. It is *inspect, decide, confirm with the boss, act,
prove* — and when the boss says no, that answer has to go somewhere.

This chapter adds the rest of the authoring language: types that make
findings checkable, routes that make behavior conditional, an `ask user`
gate that puts a decision in your hands mid-recipe, the delegation and
pipeline constructs, and the verification vocabulary that lets a recipe
prove its own work. You will build `triage` — a recipe with all of it —
and meet, honestly, the point where a small local model and an ambitious
recipe disagree about what fits in a turn.

## Where we left off

`explain_test` is registered, pinned, and proven (`decided_by: "pinned"`).
The recipe language you know: header, trigger, inputs, one agent, one run,
one capture. Today the toolkit doubles.

## Technical requirements

- The end-of-Chapter-4 state: workspace green, binaries built, Ollama
  serving `gemma4:e4b`.
- About thirty-five minutes. The recipe-writing is fast; the runtime
  experiments vary with your hardware.
- Captured on the verification machine as before — including two linter
  errors the author triggered and fixed on the way to green, because
  those errors are the documentation.

## The rest of the language

### Types: findings you can check

```erpe
type Finding:
  title: Text
  severity: low | medium | high
  evidence: list<Text>
```

A **type** is a recipe-local shape declaration. Primitives: `Text`,
`Markdown`, `Json`, `Bool`, `Number`, `Percent`, `File`, `Path`,
`Workspace`, `Diff`. Compounds: `list<T>`, `map<K, V>`, `optional<T>`,
and inline unions like `low | medium | high`. Types are checked on typed
captures — the moment a step claims to produce a `Finding`, the runtime
holds the result to that shape. A type is a promise with a referee.

### Captures: the only state transition

State changes only through `capture`. Three forms:

```erpe
capture result -> state.explanation            # inline, untyped

capture:                                       # block, typed
  finding: Finding -> state.finding
  severity: low | medium | high -> state.severity

capture:                                       # forwarding
  state.finding -> state.plan
```

Five namespaces, each with one job:

| Namespace | Purpose |
|---|---|
| `input.*` | External values declared by `input` |
| `state.*` | Durable recipe-level state |
| `scratch.*` | Temporary local state |
| `user.*` | Answers from `ask user` interactions |
| `output.*` | Final exported recipe outputs |

And one annotation the language offers on any binding:

```erpe
capture:
  summary: Text as claimed -> state.repair_summary
```

`as claimed` / `as given` annotates the binding's provenance strength.
What it *does* lives one layer deeper than recipes: when captures ground
into the knowledge graph, the annotation feeds the trust computation —
`claimed` values carry an llm-generated floor, `given` values claim
verbatim user provenance (`context/knowledge/grounding.go:55-57`, as of
the pin). You will meet that machinery properly in the internals book;
for now, read it as the recipe marking how much a value can be trusted.

> **Warning — annotation position.** The annotation follows the *source*,
> before the arrow: `summary: Text as claimed -> state.repair_summary`.
> Captured linter error for writing it after the destination:
>
> ```text
> relurpify_cfg/euclo/triage.erpe: [error recipes.parse] ...:42:50: expected "->"
> ```
>
> The linter's message is terse; the fix is moving `as claimed` left of
> the arrow.

### Routes: behavior with conditions

```erpe
route:
  when state.severity is high:
    run fixer:
      ...
  when missing state.findings:
    ...
  otherwise:
    capture:
      state.finding -> state.plan
```

Branches evaluate in document order; first match wins; `otherwise` is the
fallback; a route with no matching branch and no `otherwise` fails the
turn at runtime. The complete predicate grammar:

```text
when <path> is <value>
when <path> contains <value>
when <path> confidence below <number>
when missing <path>
when present <path>
```

There are no boolean combinations — no `and`, no `or`. Complex conditions
get expressed by capturing a derived value and routing on *it*. That
limitation is a feature: routes stay readable, and readability is
auditable.

`route` is a first-class execution construct — a recipe whose only body
is a route is valid (the gate experiment below proves it).

### The ask user gate

```erpe
ask user:
  question "Proceed with the recommended repair?"
  choices ["yes", "no"]
  capture answer -> state.decision
```

`ask user` pauses the turn on an interaction frame carrying the question
and choices; your answer lands in `user.*` and — via the capture — in
recipe state. Requirements and semantics, from the project's reference:
it needs `may ask user` in the trigger; `choices` is a static list or
`choices from state.<name>`; recipes are atomic except at these pauses.
This is the DSL's human gate — the in-recipe sibling of the approval
banner.

### Delegate and pipeline

```erpe
delegate to repairer:
  from state.finding
  goal "Plan the revision."
  capture:
    summary: Text -> state.repair_summary
```

`delegate` creates a child execution for a declared agent; the child
receives `from` inputs and writes back only through `capture`; nesting is
supported. `pipeline` stages runs in document order:

```erpe
pipeline:
  stage explore:
    run explorer:
      ...
  stage plan:
    run planner:
      ...
```

> **Warning — delegate blocks cannot declare vocabulary.** Captured
> linter error, verbatim:
>
> ```text
> [error recipes.parse] ...:40:11: may invoke is only allowed in trigger and run blocks
> ```
>
> A delegated child's tools come from the trigger-level declaration, and
> the trigger-level list unions into *every* step's scope (the scope rule:
> `trigger may invoke ∪ run may invoke`). If you need tight per-step
> vocabulary, use a `run` block inside a `route` branch — it takes its own
> `may invoke` — and that is exactly what the chapter's recipe does.

### The verification vocabulary

Chapter 3 counted five test-runner manifests under `tools/test/`:
`python_pytest`, `python_unittest`, `go_test`, `node_npm_test`,
`rust_cargo_test`. They are model-callable, they take typed constrained
inputs (a working directory, an optional path filter), and they run a
*fixed* command — the model chooses neither the command nor the binary.
To make a recipe prove its work: put the matching runner in the step's
`may invoke` and say what "done" means in the goal. Chapter 2's repair
turn ran inside this same arrangement.

### families.yaml: the handoff table

The shipped `relurpify_cfg/euclo/families.yaml` maps classified intent
families to handoff recipes — `debug` to `debug_tdd_repair`, `review` to
`code_review`, and so on for seven of the nine built-in families. It is
workspace data: added once by materialization, never overwritten, and
yours to edit. Two rules give it teeth: a mapping whose target recipe is
not registered is a **boot error**; a family with no mapping degrades to
a named runtime failure if a handoff ever targets it. Custom families are
reachable only by pinning — and the linter warns you at authoring time.

## Hands-on

### 1. Write triage

Create `relurpify_cfg/euclo/triage.erpe`:

```erpe
thoughtrecipe euclo.thoughtrecipe.triage
"Inspect a failing test, classify the defect, confirm the repair with the user, then repair and verify."

trigger as capability:
  may read workspace
  may write workspace
  may ask user
  may invoke ["file_read", "file_list"]
  family ["investigation"]
  keyword ["triage"]

input workspace: "**/*"
input prompt: user.request

type Finding:
  title: Text
  severity: low | medium | high
  evidence: list<Text>

agent inspector uses react
agent repairer uses react

run inspector:
  may invoke ["file_read", "file_list"]
  from input.prompt
  goal """Read the workspace's test file, identify the failing test, and classify the defect it reveals. The user request is: {{ index . "task.instruction" }}"""
  capture:
    finding: Finding -> state.finding
    severity: low | medium | high -> state.severity

ask user:
  question "Proceed with the recommended repair?"
  choices ["yes", "no"]
  capture answer -> state.decision

route:
  when state.decision is yes:
    run repairer:
      may invoke ["file_read", "file_write", "python_pytest"]
      from state.finding
      goal """Repair the defect described by the finding, then run python_pytest on test_stringkit.py and report the real result. The finding is: {{ index . "state.finding" }}"""
      capture:
        summary: Text as claimed -> state.repair_summary
  otherwise:
    capture:
      state.finding -> state.plan
```

Read the design decisions back:

- The **inspector** is deliberately read-only: two read tools, and the
  test runner is *not* in its scope. Classification does not need
  execution, and a step that cannot write cannot surprise you.
- The **route branch** is a `run`, not a `delegate` — that is the
  captured lesson from the Warning above: the repairer gets tight
  vocabulary (`file_write` + `python_pytest`) exactly where it acts.
- The **otherwise branch** forwards the finding to `state.plan` — the
  user said no, and the work item is preserved instead of dropped.
- The **annotation** `as claimed` marks the repair summary for what it
  is: a model's claim about its own verification. Chapter 2 taught you
  what those are worth; the recipe now carries the caution in its types.

### 2. Lint, register, pin, dispatch

```bash
~/relurpify/relurplint --check recipes --workspace .
```

```text
relurplint: all checks passed (0 diagnostics)
```

Restart relurpish, pin, and dispatch:

```text
/recipe euclo.thoughtrecipe.triage
```

```text
Pinned recipe for this session: euclo.thoughtrecipe.triage (dispatch runs exactly this recipe; /recipe clear to unpin)
```

Then give it the triage task. In the captured session, the dispatch
receipt landed exactly as Chapter 4's did:

```json
{
  "decided_by": "pinned",
  "candidate_count": "1"
}
```

The recipe is registered, selected, and running under your command.

### 3. What the turn did — including where it struggled

Honest accounting from the captured session, so you know what to expect
on small-model hardware:

- **The approval gate sat on reads.** The audit chain shows what the
  early banner was really approving — not a write:
  `action: "tool:file_read"`, `type: "hitl"`, `result:
  "tool_allowed"`. Under this build's policy posture, even reads
  surfaced the banner on their first use; your `s` grant covered the
  session. The audit chain is where mysteries like this get solved —
  that is why it exists.
- **The inspector struggled.** With the test runner in scope (an earlier
  draft of the recipe put `python_pytest` in the inspector's step), the
  model spun: the feed showed the same thought — *"run python_pytest on
  the tests to find the failing test case"* — repeated as the bounded
  loop retried. The recipe's final draft removed the runner from the
  inspector's scope (classification by reading; the repairer runs the
  tests), and the spin went with it. Vocabulary scoping is not only a
  security boundary; it is a focus boundary for the model too.
- **When a step fails, the route aborts.** A cancelled step's telemetry
  record, captured from a killed run, shows the machinery that governs
  this:

  ```json
  {
    "node.id": "run.15.5.repairer.0.execute",
    "action_taken": "abort",
    "kind": "cancelled",
    "message": "node react_think execution failed: context canceled",
    "on_error": "abort"
  }
  ```

  `on_error: "abort"` is the default step-error policy: there is no
  `on_error` syntax in the DSL; recovery is expressed with
  `route: otherwise:` branches. The event names the node, the paradigm,
  and the action taken — a failed turn is never a mystery.
- **Two-phase recipes are heavy for a small model.** On the
  verification machine's CPU, the full inspect → confirm → repair →
  verify arc did not complete inside a reasonable capture window; the
  decision-repair loop's retries were visible as thoughts growing one
  fragment longer each round. The operator's move is `/stop` — stop the
  run, read the feed, simplify the task or the recipe, retry. This is
  the honest boundary of local-first agent work on modest hardware, and
  Chapter 9 gives you the deterministic tooling to develop recipes
  without paying model time for every loop.

### 4. The gate experiment: `ask user` under a microscope

To verify the ask-user machinery in isolation, the author wrote a recipe
with **no model calls at all** — a route as the only execution construct
alongside the gate (`relurpify_cfg/euclo/gate_demo.erpe`, included in
this chapter's lab). It lints, registers, pins, and dispatches. The
telemetry for its turn shows the machinery working:

```text
euclo.interaction.frame.emitted   (frame-1792781054761170596)
euclo.step.started                (node.id: ask.10.1.0.execute)
```

The input bar switched to `running — > blocked`, which is the paused
state the reference describes.

> **Warning — a finding to carry.** In this build, the emitted frame did
> not surface as an answerable row in the TUI's chat feed, and `/hitl`
> reported `No pending approvals` while the turn sat paused on the frame.
> The runtime emitted it (`euclo.interaction.frame.emitted` is on disk);
> the TUI did not render an answer path in the captured sessions.
> `[NEEDS CONFIRMATION: ask-user frame rendering in TUI at your pin]` —
> if your machine shows the `● question [1] yes [2] no [esc] dismiss`
> row, the book wants to know; if it does not, you have reproduced a real
> gap, and `/stop` is the way out of a paused turn.

The authoring lesson stands on verified ground regardless: the gate,
its `may ask user` requirement, the capture of the answer, and the
routing on `state.decision` are all real DSL machinery — the shipped
corpus and the project reference use them as documented, and the
`gate_demo` recipe proves the grammar end to end at the registry and
telemetry layers.

### 5. Keep the artifacts

`triage.erpe` and `gate_demo.erpe` now live in your workspace's recipe
directory. Commit them — the config tree is yours, and git makes every
future recipe change reviewable:

```bash
git add relurpify_cfg/euclo/
git commit -m "add triage and gate_demo recipes"
```

## Gotchas

- **`may invoke` is trigger-and-run only.** Delegate blocks cannot
  declare vocabulary (captured linter error above). Tight per-step scope
  means run-in-route, not delegate.
- **Trigger vocabulary unions into every step.** Declare `file_write` at
  the trigger and every step sees it — including the ones that should
  never write. Keep the trigger list to what is shared; push specials
  into the step.
- **`as claimed` sits before the arrow.** `Text as claimed ->
  state.repair_summary`, not after the destination.
- **There is no `on_error` in the DSL.** Step failure aborts the route by
  default; recovery is `route: otherwise:`. The failure event names the
  node, paradigm, and action — read it before rewriting the recipe.
- **Scope is a focus tool, not only a fence.** A model with `python_pytest`
  in scope will sometimes run it in a loop instead of reading; take the
  runner out of scopes that do not need it.
- **Custom families are pin-only.** A family outside the nine built-ins
  never wins a dispatch by classification; the linter warns. If you need
  it selectable, map it in `families.yaml` — and remember a mapping to an
  unregistered recipe is a boot error.
- **A paused turn blocks the input bar** (`running — > blocked`). If
  nothing answerable appears, `/stop` ends it; the feed and telemetry
  tell you what the pause was.

## Summary

- Types make step outputs checkable; typed captures hold results to the
  declared shape; `as claimed`/`as given` annotates provenance into the
  knowledge layer's trust computation.
- `route` gives recipes conditional behavior with a five-form predicate
  grammar, first-match-wins order, and an `otherwise` fallback that also
  makes route-only recipes valid.
- `ask user` is the DSL's human gate; `delegate` and `pipeline` compose
  agents; the verification vocabulary lets a recipe prove its work with
  fixed-command test runners.
- The trigger-unions-into-steps scope rule shaped the chapter's recipe:
  read-only inspector, tightly-scoped repairer in a route branch.
- Triage is registered, pinned, and dispatched under your command — and
  the captured session taught the honest boundary: small models spin
  under ambitious two-phase recipes, the default error policy aborts
  cleanly with a named cause, and `/stop` is the operator's exit.

## Check yourself

1. Your inspector step must read files but never write. Where does each
   tool name go — trigger or run block — and what is the observable
   consequence of getting it wrong in the permissive direction?
2. In `when state.decision is yes`, where does `state.decision` get its
   value, and which trigger line is mandatory for the construct that
   sets it?
3. A teammate's recipe uses `delegate to fixer:` with `may invoke
   ["file_write"]` inside the block. What happens, exactly, and what are
   the two documented ways to give a delegated child its tools?
4. The repairer's summary is captured `as claimed`. What does that
   annotation do downstream, and why is it the honest choice for a
   model's self-report?
5. A step fails at runtime. What does the default step-error policy do,
   what does the failure event name, and what DSL construct expresses
   recovery instead?

*Answers.* 1. Reads shared by many steps may sit at the trigger; anything
special (and all writes here) go in the run block. Wrong in the
permissive direction: the write tool unions into every step's scope — the
model sees it and may use it; the approval banner still guards writes in
governed mode, but the focus damage (and the extra decisions) are real. 2.
The `ask user` block captures `answer -> state.decision`;
`may ask user` in the trigger is mandatory for any recipe containing an
`ask user` block. 3. The linter rejects it — captured error: "may invoke
is only allowed in trigger and run blocks." The two ways: declare the
tools at trigger level (they union into every step), or use a `run` block
inside a route branch with its own `may invoke`. 4. When captures ground
into the knowledge graph, the annotation feeds the trust computation —
`claimed` applies an llm-generated floor rather than verbatim-user
trust. It is honest because a model's summary of its own verification is
exactly that: a claim. 5. The default policy aborts the route
(`action_taken: "abort"`, `on_error: "abort"`); the event names the step
node, the paradigm, and the failure message; recovery is expressed with
`route: otherwise:` branches — there is no `on_error` syntax in the DSL.

## Try it yourself

1. **The forwarding branch, with intent.** Modify triage's `otherwise`
   branch to forward `state.finding -> state.plan` *and* add a comment
   you would leave a teammate. Then answer "no" at the gate (or observe
   the paused turn) and trace where the finding landed — in the recipe's
   state, and in nothing else.
2. **A severity route.** Add a second route *above* the decision route:
   `when state.severity is low:` that forwards the finding straight to
   `state.plan` without asking. Lint, restart, pin, and think through
   the ordering question: what would happen if this branch came *after*
   the decision route? (First match wins — order is behavior.)

## Where next

You now hold the whole authoring language — but every recipe so far has
run one paradigm: react, the bounded loop. The language declares eight,
and each is a different shape of thinking. Chapter 6 walks the linear
four — react, pipeline, chainer, reflection — with a mental model, a real
run, and a verdict for each. The station cards are yours; next you learn
the knife skills.

The lab for this chapter lives in
`docs/cooking-agentic-thoughts/labs/ch05-triage/` — both recipes, the two
captured linter errors, the dispatch receipts, and the frame/`/hitl`
finding with its telemetry.
