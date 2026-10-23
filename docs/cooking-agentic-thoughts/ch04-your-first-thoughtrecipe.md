# Chapter 4 — Your First Thoughtrecipe

Until today, every choice the agent made on your behalf came from recipes
someone else wrote. You watched the lattice score twelve candidates, and
you took its word that the winner was right.

The shelf marked `euclo/` is yours now, and it takes text files. This
chapter you write one from an empty file — a trigger, a goal, a capture —
and then you make the runtime prove a sentence that has never been true in
this kitchen before: *it ran because you said so.* The receipt will read
`decided_by: "pinned"`, and by the end of the chapter you will know
exactly what that line cost and what it bought.

## Where we left off

The workspace is known-green: `relurplint` passes, both posture dials sit
where you set them, the fences are read. stringkit is green, and two of
your turns live in the audit chain. The ten shipped recipes in
`relurpify_cfg/euclo/` are still cargo. Fixing that today.

## Technical requirements

- The end-of-Chapter-3 state: materialized workspace, green lint baseline,
  Ollama serving `gemma4:e4b`.
- About thirty minutes, including one boot and one or two short turns.
- Everything below was executed and captured on the verification machine,
  as in Chapters 1–3.

## What a recipe is

A **thoughtrecipe** (a **recipe** from here on) is a single declarative
file describing one agentic workflow: how it is triggered, which tool
vocabulary its steps may use, which agent paradigm executes, and how state
flows between steps. The file extension is `.erpe`, and the files live in
`relurpify_cfg/euclo/` — top level only.

Two boundaries from the project's own reference are worth framing before
you write a line, because they prevent the two classic first-recipe
mistakes:

- **Recipes are not programs.** No loops, no recursion, no arbitrary
  expressions. A recipe is a declaration the loader lowers into an
  execution graph; the loops live inside the paradigms it names.
- **Recipes are not grants.** Every `may ...` line declares *vocabulary
  and intent*. The permission system, the sandbox, and the policies decide
  what actually happens. Chapter 3's fences outrank anything a recipe
  says.

Think of a recipe as a *station card* in a professional kitchen: what the
station is for, which knives it may reach, and what it hands to the next
station. The chef who wrote the card does not thereby own the knives.

### Anatomy, by reading one

Here is the recipe you will write, whole. Read it once before the
line-by-line:

```erpe
thoughtrecipe euclo.thoughtrecipe.explain_test
"Read a named test file and explain in plain language what behavior it covers and what a bug in the code under test would break."

trigger as capability:
  may read workspace
  may invoke ["file_read", "file_list"]
  family ["review"]
  keyword ["explain_test", "test coverage"]

input workspace: "**/*"
input prompt: user.request

agent explainer uses react

run explainer:
  may invoke ["file_read", "file_list"]
  from input.prompt
  goal """Read the test file the user names, then explain in plain language what behavior it covers and what a bug in the code under test would break. The user request is: {{ index . "task.instruction" }}"""
  capture result -> state.explanation
```

Eleven lines of structure, six parts:

| Part | What it does |
|---|---|
| `thoughtrecipe euclo.thoughtrecipe.explain_test` | The identity. This dotted ID is the recipe's name everywhere — registry, pins, handoff tables. It comes from the header, not the filename. |
| `"…"` | The description. This is what `/recipe` shows. Write it for the reader of the list — future you. |
| `trigger as capability:` | The route policy: effect declarations (`may read workspace`), recipe-level tool vocabulary (`may invoke […]`), the intent families it serves, and keywords that help classification. |
| `input … :` | Read-only inputs. `input prompt: user.request` binds the user's instruction; `input workspace` scopes the file universe to `"**/*"`. |
| `agent explainer uses react` | Declares an agent bound to a paradigm. `react` is the bounded think-act-observe loop your turns have been running all along. |
| `run explainer:` | The execution body: step-level vocabulary, the input source, the **goal** (the step's instruction to the model), and a **capture** that writes the result into state. |

Two details carry more weight than their size:

**The interpolation.** `{{ index . "task.instruction" }}` inside the goal
injects the user's actual request. A goal that drops it leaves the agent
blind — it would follow the static text and never know what was asked.
This is the recipe language's most important habit, and the project's
reference states it as a rule: *always interpolate the request.*

**The two `may invoke` lists.** Recipe-level (in the trigger) and
step-level (in the `run` block) unite into the step's effective scope,
and every name must resolve against the tool registry **at boot** —
recipe vocabulary is load-checked, not decorative. `file_read` and
`file_list` are manifest tools from Chapter 3's `tools/` shelf; the
model-callable set is the vocabulary the model sees.

Note what this recipe deliberately omits: any write. It reads the test
file and explains it. Hold that thought for the hands-on — it explains
what did *not* happen at the approval gate.

## Hands-on

### 1. Write the recipe

Create `relurpify_cfg/euclo/explain_test.erpe` in your stringkit
workspace with exactly the content shown above. (The shipped corpus has a
cousin, `explain.erpe` — same shape, different job: it explains any code;
yours explains test files. The distinct ID keeps both registered: the
first recipe with a given ID wins, and later duplicates become load
warnings.)

### 2. Lint

```bash
~/relurpify/relurplint --check recipes --workspace .
```

```text
relurplint: all checks passed (0 diagnostics)
```

> **Warning — the lint's honest edge.** The recipes check parses, checks
> types, resolves symbols, and lowers — with no runtime. What it cannot
> do is resolve your tool vocabulary or prompt imports against a live
> registry: `file_read` and `file_list` exist in this workspace, so this
> recipe lints clean and boots clean, but a typo'd tool name can pass the
> lint and still fail at boot. The project reference says so in as many
> words. The lint is the cheap gate; the boot is the truth.

### 3. Restart, and find yourself in the registry

Recipes load once, at boot. Start relurpish (or restart it if a session
was open — Chapter 3's banner rule applies to posture; this rule applies
to recipes), then run:

```text
/recipe
```

The captured listing from the verification machine's registry, trimmed:

```text
  euclo.thoughtrecipe.code_review
  euclo.thoughtrecipe.debug_tdd_repair
  euclo.thoughtrecipe.default
  euclo.thoughtrecipe.dep_upgrade
  euclo.thoughtrecipe.explain
  euclo.thoughtrecipe.explain_test
  ...
```

Eleven recipes where yesterday there were ten. Yours is between `explain`
and `extract_func`, alphabetical, indistinguishable from the shipped set
— which is the point. It is not a guest; it is part of the corpus.

> **Warning — the restart rule has teeth.** Pinning validates against the
> loaded registry, and the validator's captured error for an unknown ID
> says exactly what to do:
>
> ```text
> Cannot pin /recipe: not registered in this workspace.
> Recipes load once at boot — restart relurpish to pick up new recipe files.
> Registered:
>   euclo.thoughtrecipe.code_review
>   ...
> ```
>
> (That capture came from a keystroke race in the author's test harness —
> the argument arrived mangled — and it earns its place here anyway,
> because it shows the validator refusing an unknown pin *with the full
> registered list attached*. A pin that would fail the whole turn at
> dispatch is rejected at pin time instead.)

### 4. Pin it

```text
/recipe euclo.thoughtrecipe.explain_test
```

The captured acknowledgment:

```text
Pinned recipe for this session: euclo.thoughtrecipe.explain_test (dispatch runs exactly this recipe; /recipe clear to unpin)
```

**Pinning** overrides selection entirely: every turn this session runs
exactly this recipe, regardless of what the classifier or lattice would
choose. It is the sharpest selection tool you own, and Chapter 2's
`decided_by` table already told you the receipt it produces.

### 5. Order an explanation

```text
Explain what test_stringkit.py covers: read the test file and describe
the behavior it specifies and what a bug in stringkit would break.
```

What happened in the captured session, in order:

- **No approval banner.** The recipe is read-only — `may read workspace`,
  two read tools — so there is nothing to approve. Governed mode pauses
  writes and risky capabilities; it does not nag about reading. The turn
  ran unimpeded.
- The turn completed: the feed's result line read
  `node=euclo.done | status=ok` in about four seconds (yours may take
  longer on a cold model; the counters below are the honest ones).

### 6. Read the receipt that proves the sentence

```bash
jq -c 'select(.resourceLogs) | .resourceLogs[]?.scopeLogs[]?.logRecords[]?
       | select(.eventName=="euclo.route.selected")' \
    .relurpify_state/telemetry/otlp/*.jsonl | tail -1
```

The captured attributes:

```json
{
  "node.id": "euclo.dispatch",
  "candidate_count": "1",
  "decided_by": "pinned"
}
```

Compare with Chapter 2's selection record for the same kitchen:
`decided_by: "lattice:score"`, `candidate_count: 12`. Your recipe's
registration even changed the unpinned math — a later capture showed
`candidate_count: 14` (two new candidates from two new files), the
lattice scoring your recipe alongside the shipped ten. Pinning collapsed
fourteen candidates to one and changed the reason code from *the machine
chose* to *you chose*. That single attribute is the difference between
operating a recommendation engine and commanding one.

The completion record closes the loop:

```json
{
  "outcome": "success",
  "llm_calls": "1"
}
```

When you are done admiring it:

```text
/recipe clear
```

```text
Recipe pin cleared; normal selection applies.
```

And selection is the lattice's again — with your recipe in its candidate
set, reachable by trigger, keyword, or score, exactly like the shipped
ten.

## Gotchas

- **Recipes load once, at boot.** Edit the file, restart relurpish. The
  pin validator will refuse unknown IDs at pin time with the registered
  list attached — but the deeper trap is subtler: `/recipe` shows
  skipped files *with reasons*, so read that list after any edit.
- **The lint cannot see your vocabulary.** `relurplint --check recipes`
  passes recipes that name tools which do not exist; boot resolves the
  registry and boot fails loudly, naming recipe, step, and tool. Lint
  early, boot to confirm.
- **Always interpolate the request.** `{{ index . "task.instruction" }}`
  in every goal. A goal that drops it produces an agent that executes
  your static text with no idea what was asked.
- **Vocabulary is not permission.** `may invoke` says what the step may
  *reach for*; the sandbox, policies, and approvals decide what happens.
  A recipe declaring `file_write` would still pause at the banner in
  governed mode.
- **Duplicates lose quietly the first time.** First recipe with an ID
  wins; later duplicates become load warnings that `/recipe` reports.
  Copy an existing file to remix it? Change the header ID first.
- **Pins are per-session.** `/recipe <id>` rides until
  `/recipe clear` or quit. Nothing about a pin persists to disk — a
  deliberate boundary; Chapter 11 shows what persistence looks like when
  the project does mean it.

## Summary

- A recipe is a declarative station card: trigger (effects, vocabulary,
  families, keywords), inputs, an agent bound to a paradigm, and a run
  body whose goal interpolates the user's request and whose capture
  writes state.
- Recipes are not programs and not grants; the loader lowers them to an
  execution graph, and the fences outrank them.
- The workflow is write → lint → restart → `/recipe` → pin → run → read
  the receipt. You performed every step on your own file.
- `decided_by: "pinned"` with `candidate_count: 1` is the receipt that
  says the runtime ran *your* choice, not its best guess.
- Your recipe is now a first-class citizen of the registry — scored
  alongside the shipped ten whenever selection runs unpinned.

## Check yourself

1. Your recipe lints clean but fails at boot, naming a tool it may
   invoke. What did the lint not check, and which two fixes are open to
   you?
2. In the goal, what does `{{ index . "task.instruction" }}` do, and what
   observable failure follows from leaving it out?
3. A teammate edits a recipe file while relurpish runs, then pins it by
   ID and gets "Cannot pin …: not registered." What actually happened,
   and what are the two steps to fix it?
4. After pinning, `candidate_count` read `1`; unpinned, `14`. What are
   the two numbers counting in each case, and which of the two states
   would you want for a turn where the classifier might plausibly know
   better than you?
5. Your recipe declares `may invoke ["file_write"]` in a `run` block, and
   the model obeys it during a governed-mode turn. Does the write happen
   silently? Trace the exact sequence from tool call to disk.

*Answers.* 1. The lint checks grammar, types, symbols, and lowering —
without a live registry — so it cannot resolve tool vocabulary; fix the
name (typo) or add a manifest to `tools/` (deliberate new tool), then
boot to confirm. 2. It injects the user's request into the step's goal;
without it the agent executes the static instruction blind — Chapter 2's
repair turn succeeded *because* the task text reached the model, and a
goal without interpolation cuts that channel for every turn of this
recipe. 3. Recipes load once at boot, and their file was written after
this boot; restart relurpish (registration), then pin again — the
validator listed the registered IDs to prove which one was missing. 4.
Unpinned, 14 counts the registered candidates the lattice scored; pinned,
1 counts the single recipe the pin forced. Choose the lattice when the
classifier's evidence is at least as good as yours — routine requests in
familiar territory; pin when you want determinism over convenience —
demos, debugging selection itself, or a recipe whose behavior must be
exactly reproducible. 5. No. The call passes admission and policy
evaluation, then pauses at the approval banner; only your grant (`y`, `s`,
or `a`) lets the governed `file_write` execute in the sandbox — and the
decision lands in the audit chain either way.

## Try it yourself

1. **The restart rule, experienced.** While relurpish runs, copy
   `explain_test.erpe` to `explain_test2.erpe` and change its header ID
   to `euclo.thoughtrecipe.explain_test2`. Run `/recipe` — your new file
   is absent (loads happen at boot). Restart, `/recipe` again, and find
   twelve. Pin it, confirm `decided_by: "pinned"` in telemetry, then
   delete the file and restart once more: `/recipe` should now warn about
   a missing file or show a skipped entry — read what the registry says
   about a file that vanished.
2. **Vocabulary that fails at boot, safely.** Add `"file_delete"` to the
   run block's `may invoke` list. Lint: green (the lint cannot see the
   registry). Boot: read the failure — it names the recipe, the step, and
   the unresolved tool. Revert. You have now seen every layer of the
   recipe validation story with your own file.

## Where next

You own one recipe — written, registered, pinned, and proven. But it is
the simplest shape in the language: one agent, one run, one capture.
Chapter 5 opens the full toolkit — types, routes, `ask user` gates,
`delegate`, the verification vocabulary that lets a recipe *prove* its
own work — and builds the triage recipe that turns stringkit's test
failures into decisions. The station cards get longer; the kitchen gets
smarter.

The lab for this chapter lives in
`docs/cooking-agentic-thoughts/labs/ch04-explain-test/` — the recipe file
exactly as captured, the pin acknowledgment, and both route-selection
receipts (pinned and lattice) side by side.
