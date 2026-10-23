# Chapter 10 — Rewriting the House Style

Every kitchen has house style. Not the recipes — the *habits*: taste
before seasoning, clean as you go, the way everyone plates a dish without
being told. For seven chapters you worked under this kitchen's shipped
habits, encoded in sentences the model reads before it reads you. Chapter
8 let you add one line. This chapter asks the bigger question: *which
sentences actually reach the model, and who gets to write them?*

The answers — verified from telemetry and source, not assumed — will
surprise you. Then you will write the house style at both ends of the
power spectrum: a line in the prompt that resolves every turn, and a
recipe that drafts its own prompt for your review — which the runtime
will never install itself, because that is the whole point.

## Where we left off

The metronome runs: your agentic layer fails loudly under a
deterministic harness, and the stringkit suite caught a real wobble with
a full receipt chain. Craft and evidence are both yours. This chapter is
the craft chapter's summit — and Part IV begins by turning the evidence
discipline on the house style itself.

## Technical requirements

- The end-of-Chapter-9 state: binaries, green workspace, Ollama serving
  `gemma4:e4b`.
- About thirty-five minutes; one short dispatch for the resolution
  receipt.
- Captured on the verification machine as always.

## What actually reaches the model

Before you rewrite a house style, verify which sentences are load-bearing.
The method is Chapter 8's resolution telemetry, aggregated across every
run this book has executed:

| Prompt ID | Resolutions across all captured runs |
|---|---|
| `agent.react.default` | 21 |
| `agent.react.default.user` | 21 |
| `framework.*` (all eight blocks) | **0** |
| `agent.{chainer,euclo,generic,pipeline}.default` | **0** |
| `rewoo.plan.v1`, `rewoo.synth.v1` | **0** |
| `named/euclo/*` | **0** |

Twenty-one resolutions each for the two react prompts — every turn,
system and user, in that order. Everything else: zero.

> **Warning — the framework blocks are unwired.** The eight
> `framework/*` blocks are shipped, materialized, and lintable — and as
> of this pin, **nothing resolves them**. No code references their IDs,
> and no turn in this book's captured telemetry ever resolved one. Their
> content reads as scaffolding ("You are the framework error recovery
> assistant"). Editing one is harmless and future-proofing — but it
> changes no behavior today. Do not mistake a lint-green edit to an
> unwired file for a change of house style.
> `[NEEDS CONFIRMATION: framework block wiring at your pin]`

So the live house style — the sentences that reach the model every turn —
is exactly two files you have already met:

- `agent.react.default` — the system voice (Chapter 8 added its line).
- `agent.react.default.user` — the per-turn frame, with the task
  injected through `{instruction}` and the JSON decision schema
  protected in code spans.

Both are fixed-name IDs. Both resolve by name every turn. This is the
whole throne room; the rest is corridor.

## Hands-on

### 1. Read the per-turn frame, then edit it

You saw `agent.react.default.user` in Chapter 8. Read it again as what it
is — the sentence pair that opens every turn:

```text
You are a ReAct agent optimized for a small-context local model.
Work step-by-step. Prefer the smallest useful action. Do not restate the task.
```

Add one line of *your* house style — the book's thesis, made standing
order:

```text
When a task names a test, run it and report the real result.
```

(Insert after the "smallest useful action" line, exactly as in the lab's
copy.) Lint, restart relurpish, and dispatch a turn that exercises it:

```text
Run the tests in this workspace and tell me whether they pass.
```

The captured receipt chain:

```text
euclo.route.selected   (decided_by: "lattice:score")
prompt.resolved        agent.react.default        cache_hit: false
prompt.resolved        agent.react.default.user   cache_hit: false
prompt.resolved        agent.react.default        cache_hit: false
prompt.resolved        agent.react.default.user   cache_hit: false
euclo.execution.complete   outcome: "unverified"
```

Three things to read in that chain:

- **Your line is live.** The user prompt resolved twice (the react loop
  re-renders it per iteration), `cache_hit: false` — fresh renders of
  the file containing your sentence.
- **Two prompts, four resolutions.** System and user resolve as a pair
  each iteration; the loop's rhythm is visible in the telemetry.
- **A new outcome word.** Not `success` this time — `unverified`. The
  agent ran the tests and reported; it changed no files, so the runtime
  classified the turn as completed-without-verification. The outcome
  vocabulary has more depths than Chapter 2's first taste, and it is
  honest in both directions: `success` means the loop completed
  (Chapter 2's lesson), and `unverified` means the loop completed
  *without anything to show for it*. Your receipts keep getting more
  articulate.

### 2. The framework tour, with the wiring diagram

Walk all eight framework blocks and the paradigm defaults with the
reader's eye (`relurpify_cfg/prompts/framework/`,
`relurpify_cfg/prompts/agents/`). Then check wiring the way this book
does — resolution telemetry plus a source grep. The captured result is
the section above's zero column. Understanding *why* nothing resolves is
builder's book territory (the prompt assembly lives in the react
prompt-context code), but the operator's conclusion is actionable now:

- The **live seams** are the two react prompts. Edit those; verify with
  resolution receipts.
- The **corpus** is real estate: lint-clean, registered, harmless. If a
  future pin wires `framework/*` in, your early edits inherit the
  wiring — but never count on unwired files.

### 3. Posture tuning is house style too

Not every standing order lives in a prompt file. Chapter 3's two dials
are conversation design in configuration form, and at this point in the
book you can articulate what they *mean* rather than recite them:

- `approvals: governed` — every write surfaces as a moment where your
  judgment outranks the model's. The house style of *authority*.
- `selection.posture: deterministic` — ambiguity resolves to a question
  to you, never to a model's guess. The house style of *honesty about
  uncertainty*.

Both captured banners (Chapter 3) showed the values reading back. The
design decision this chapter adds: these interact with your prompt
edits. A house line like "run the tests and report the real result" is
*safe* under governed approvals precisely because verification reads are
cheap and writes still pause. House style is a system; edit it as one.

### 4. The levels ladder — every rung, one mechanism you own

The wider agentic-engineering community describes a ladder of prompt
techniques, from static text to prompts that improve themselves
(framing credited to that discourse; the mappings below are Relurpify's
own, and every rung is something you have already built):

| Level | Technique | Your mechanism | Built in |
|---|---|---|---|
| 1. Static | Fixed, reusable text | An edited prompt file (`agent.react.default`) | Ch 8 |
| 2. Workflow | Inputs, ordered steps, defined output | A pipeline recipe with stages | Ch 6 |
| 3. Control flow | Conditionals in the workflow | `route:` with predicates | Ch 5 |
| 4. Delegation | The prompt spawns sub-work | `delegate` / `run` in branches | Ch 5 |
| 5. Higher-order | A prompt that consumes another's output | `plan_repair`'s plan consumed by a build step (Ch 7 → Ch 11 pairs it) | Ch 7 |
| 6. Template meta | A prompt that generates new prompts | The drafting recipe, below | Ch 10 |
| 7. Self-improving | A prompt that rewrites itself | **Not automated — by design.** The draft lands in your hands; the gate is you | Ch 10 |

Two honest footnotes. Level 5's full plan/build pair awaits one more
recipe (Chapter 11 pairs them for queued work); the planning half is
already proven. Level 7 is where this framework parts company with the
self-improving-prompt zeitgeist — deliberately, and the stretch below is
the demonstration.

### 5. The drafting recipe — and the gate that makes it safe

Create `relurpify_cfg/euclo/draft_prompt.erpe`:

```erpe
thoughtrecipe euclo.thoughtrecipe.draft_prompt
"Draft one house-style line for the react system prompt into drafts/ for human review. Never touches relurpify_cfg — the human installs."

trigger as capability:
  may read workspace
  may write workspace
  may invoke ["file_read", "file_write"]
  family ["implementation"]
  keyword ["draft prompt"]

input workspace: "**/*"
input prompt: user.request

agent drafter uses react

run drafter:
  may invoke ["file_read", "file_write"]
  from input.prompt
  goal """Read stringkit.py and test_stringkit.py. Draft exactly one new house-style line for a ReAct coding agent working in this repository — a short imperative sentence that would prevent a mistake visible in the code or tests. Write only that line to drafts/system-prompt-line.txt. Do not edit any other file. The user request is: {{ index . "task.instruction" }}"""
  capture result -> state.draft
```

Lint it green, restart, and — when you want the full loop — pin,
dispatch, and open `drafts/system-prompt-line.txt`. Notice the shape of
the authority:

- The recipe **can write** — to a drafts folder in your workspace, under
  governed approval like any write.
- The recipe **cannot install** — `relurpify_cfg/` is a protected path,
  so the sandbox refuses writes there even if the recipe asked (Chapter
  3's fence).
- And the book's rule is stricter than the fence: **the human reads the
  draft, decides, and performs the install** — edit the live prompt, or
  place a new named prompt — then restarts. Level 7's "self-improving"
  becomes level 6.5: *machine-drafted, human-installed.* Chapter 2's
  notes called the unprompted version memory poisoning; this kitchen
  calls the alternative Tuesday.

The manual step is not friction. It is the last place in the loop where
taste is non-negotiable — and now it is the *only* place, because
everything else receipts itself.

## Gotchas

- **Unwired files are not behavior.** The `framework/*` blocks resolve
  zero times; editing them changes nothing today. Verify wiring with
  resolution telemetry before claiming an edit "took."
- **The brace inversion never sleeps.** Prompt bodies: `{name}`.
  Recipe goals: `{{ index . … }}`. Mixing them is the Chapter 8 trap
  with the Chapter 4 accent.
- **`unverified` is a third outcome, not a synonym for success.** A
  turn that completes without producing or verifying anything reads
  `outcome: "unverified"` — receipt vocabulary for "nothing to show."
  Count it separately from success in anything you track.
- **Two live prompts, one throne room.** Everything else in the corpus
  is corridor. Spend your house-style attention on the files that
  resolve.
- **The gate is the feature.** If your drafting recipe ever drafts into
  `relurpify_cfg/`, that is not cleverness — the sandbox will block it,
  and the book's convention is that it should never ask.

## Summary

- The live house style is two files — the react system and user prompts
  — proven by resolution telemetry: 21 resolutions each across this
  book's runs, zero for everything else.
- The framework blocks are unwired corpus as of the pin: lint-clean,
  harmless, inert.
- The user prompt's edit resolved live and the turn receipt taught new
  vocabulary: `outcome: "unverified"`.
- The levels ladder maps rung-for-rung onto mechanisms you own — and
  its top rung is deliberately human-gated: machine-drafted,
  human-installed.
- House style is a system: prompt edits, posture dials, and the fences
  compose, and you now hold all three.

## Check yourself

1. Which two prompt files resolve on every react turn, and how would you
   prove it without reading any source code?
2. You edit `framework/error.recovery.prompt` and restart. What changes
   in the next turn's behavior, and what would you check to confirm
   your reading?
3. What does `outcome: "unverified"` measure that `success` does not —
   and which turn in this book's history would have carried it?
4. Place the shipped `plan.erpe` recipe on the levels ladder, and name
   the recipe pattern that would complete its rung.
5. Your drafting recipe wrote to `relurpify_cfg/prompts/` directly.
   Name the two things that should have stopped it — one mechanical,
   one conventional.

*Answers.* 1. `agent.react.default` and `agent.react.default.user` —
prove it with resolution telemetry: `prompt.resolved` events whose
`task.id` attribute carries those IDs, on every turn. 2. Nothing — the
block is unwired as of the pin (zero resolutions, no referencing code);
confirm by checking resolution telemetry for the edited ID after a
turn. 3. `unverified` means the turn completed without producing or
verifying anything — Chapter 2's failed repair turn, had the runtime
classified it that way, would have read `unverified` instead of
`success`. 4. `plan.erpe` is level 5-in-waiting: a higher-order
*producer* whose plan document is the input a build recipe consumes; the
completing pattern is the plan/build pair — plan's captured
`state.plan` feeding a build recipe's goal (Chapter 11 pairs them for
queued work). 5. Mechanically: the sandbox's protected-path rule —
`relurpify_cfg` is configured protected, so the write is denied at the
sandbox layer. Conventionally: the drafting recipe's own goal scopes it
to `drafts/`, and the house rule is that the runtime never proposes to
install itself — the human installs.

## Try it yourself

1. **The full loop.** Pin and dispatch `draft_prompt` (task: "draft a
   house-style line that would have prevented the slugify bug"). Read
   `drafts/system-prompt-line.txt`, judge the line, and either merge it
   into `agent.react.default` (your edit, your restart, your receipt)
   or reject it with a one-line reason written next to the draft.
   Either way, you have run level 6.5 end to end.
2. **The wiring watch.** After your next relurpify re-pin, re-run the
   resolution-telemetry query from this chapter. If the `framework.*`
   zero-column moves, the wiring changed — and your early edits inherit
   the wiring on that day.

## Where next

The house style is yours, the ladder is mapped, and the gate is held.
One chapter remains: Chapter 11 goes out of the loop — queued work,
workflow persistence, what permissive mode really costs, and the honest
answer to the question this book opened with: how far can you let the
kitchen run alone while the permit is still yours? The stringkit layer
you built gets its final exam: a queued task, completed while you watch
the receipts instead of the feed.

The lab for this chapter lives in
`docs/cooking-agentic-thoughts/labs/ch10-house-style/` — both live
prompts with the edits, the resolution receipt chain, the unwired-blocks
finding, and the drafting recipe.
