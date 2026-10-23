# Chapter 2 — One Turn, Watched Closely

An order comes into the kitchen: *"The slugify function is broken. Fix it
and prove the tests pass."*

In most tools, that order disappears into a box marked "AI" and comes out
some time later with edits attached. You grade the result and wonder, in
the permanent low hum of agent-assisted development: *what did it actually
do in there?*

In this kitchen, the order travels a lit corridor. It gets sorted into a
category. A route is chosen, and the reason for the choice is written down.
Before anything is modified, a request comes back with your name on it.
Every action runs in a locked room. At the end, the corridor keeps the
tapes. This chapter walks you down that corridor once, slowly, holding a
flashlight.

## Where we left off

stringkit sits red — `test_slugify_strips_punctuation` fails on the
punctuated slug — with a clean git baseline under it. Your workspace is
materialized (`relurpify_cfg/` in place, Ollama serving `gemma4:e4b`), the
doctor reported `Result: ready`, and one open question from Chapter 1 (the
TUI startup gate's behavior on some machines) rides along with us.

## Technical requirements

- The end-of-Chapter-1 state: built binaries, stringkit with its failing
  test and baseline commit, a materialized workspace, `doctor` green.
- Ollama serving `gemma4:e4b`.
- `jq` installed — the receipts are JSON-lines, and this is the chapter you
  start reading them.
- About thirty minutes, plus turn time: on a CPU, a small thinking model
  works at a thoughtful pace. Expect minutes, not seconds, for the turn.

> **Warning — carry-over from Chapter 1**
> `[NEEDS CONFIRMATION: exact TUI unlock behavior at BETA-RC1-FALL2026]`
> On the verification machine, the TUI's startup gate held chat locked on
> its Doctor pane across five backend configurations even with the CLI
> doctor reporting `Result: ready` — while the project's own guide (verified
> October 2026 by the maintainer) describes boots that arrive at the
> Postures banner. Chapter 1's Warning box has the details and the code
> receipts. If your boot locks too, try the Doctor pane's `r` (refresh) and
> `h` (heal), and if the gate stays shut, you are one bug report away from
> helping this book and this project. The rest of this chapter is written
> so that every concept stands on the project's own verified documentation,
> with the hands-on marked accordingly.

## The anatomy of a turn

When you press Enter on an instruction, five stations process it. Learn
these five and you can narrate any turn you ever watch.

**1. Classification.** A deterministic classifier scores your instruction
into **intent families** — nine built-in keywords groups: `debug`,
`repair`, `planning`, `review`, `implementation`, `refactor`, `migration`,
`investigation`, `architecture`. Your slugify order ("broken", "fails",
"fix") lands squarely in `debug`. No model is consulted for this: it is
keyword classification, fast and repeatable, the same words in, the same
family out.

**2. Route selection.** The runtime picks what will serve the turn: a
**thoughtrecipe** (a declared workflow — Chapter 4 makes you an author of
these) or a capability. Selection is deterministic, and — this is the part
worth underlining — the *reason* is recorded. Telemetry will carry a
`decided_by` value for every selection:

| `decided_by` | Meaning |
|---|---|
| `pinned` | You pinned a recipe with `/recipe <id>`; selection was overridden |
| `explicit` | The instruction matched a recipe's trigger directly |
| `lattice:score` | The deterministic scoring lattice chose the best match |
| `default_recipe` | Nothing matched strongly; the fallback recipe took it |
| `clarification` | You answered a clarification frame and the answer chose |

**3. Clarification, only on genuine ambiguity.** In the default
`deterministic` posture, an instruction that scores confidently into one
family never interrupts you. When two families tie with no breaker, a
clarification frame appears: answer with a slot number, press Enter for the
default, or `d` to defer. The point of the design: questions go to the
human, and confident instructions are not nagged. "Fix slugify, run the
tests" is confident; expect no frame.

**4. Governed execution.** The chosen recipe's steps run — typically a
**bounded ReAct loop**: the model reasons, calls a tool, observes the
result, repeats, with a hard cap on iterations. Two words do heavy lifting
here. *Governed*: every tool call passes admission, policy, and — for
writes and risky actions — the approval gate, then executes inside the
sandbox, and the result lands with a trust classification. *Bounded*: the
loop cannot think forever at your electricity's expense.

**5. Outcome.** The turn ends with a classified outcome, and on failure the
feed names the real cause — for example `react decision unparseable`, which
means the model's last decision wasn't parsable (small local models do
this sometimes; the runtime's bounded decision repair tries to fix it
first). `/retry` re-runs the same prompt. An honest failure you can read
beats a mysterious one you cannot.

### The approval gate

Sooner or later in that loop, Euclo wants to *write* — and in `governed`
mode, writes pause. A banner appears:

```text
[y] once  [s] session  [a] always  [n] deny  [d] dismiss
```

| Key | Grants | Lasts |
|---|---|---|
| `y` | this one request | once |
| `s` | this request, and this kind for the session | until you quit |
| `a` | this request, and this kind | permanently — recorded |
| `n` | no | the agent is told and adapts |
| `d` | dismiss | parked; visible under `/hitl` |

The banner has key priority: you can press the key while your cursor sits
in the input bar. During active work, `s` is the usual answer — you are
saying "yes, this session may write files as it repairs things," and the
grant is scoped to the session and written to the audit trail either way.

Compare the other posture, for one paragraph only: `--hitl-mode permissive`
auto-approves every request and writes each decision to the audit trail.
Nothing about the sandbox or capability enforcement changes — but the
doctor marks the posture degraded, and the book stays in governed mode
until Chapter 11, where you will choose with open eyes.

### The receipts

Two records survive every turn, and both live under `.relurpify_state/` —
output, never source, per the two-directory rule.

**The audit chain** at `.relurpify_state/audit/` records the approval
decisions: what was requested, what you answered, when.

**The telemetry trail** at `.relurpify_state/telemetry/otlp/*.jsonl`
records the runtime's decisions as OpenTelemetry log records — local
OTLP-JSON files, readable with `jq`, retained for seven days by default.
Two events run this book:

```bash
# why did that route run?
jq -r '.. | .logRecords[]? | select(.eventName=="euclo.route.selected")' \
    .relurpify_state/telemetry/otlp/*.jsonl | tail -3

# how did the turn end — really?
jq -r '.. | .logRecords[]? | select(.eventName=="euclo.execution.complete")' \
    .relurpify_state/telemetry/otlp/*.jsonl | tail -1
```

The first carries `decided_by`. The second carries the honest counters —
real LLM call counts and token usage — and the true cause on failure. One
boundary makes these receipts trustworthy: telemetry records *decisions*
(which route, which tool, what outcome), not raw payloads. Your source
code and the model's verbatim chatter stay out of the trail by design.

## Hands-on

> **A word on what follows.** Everything through step 3 is captured from a
> real machine, as in Chapter 1. The turn itself (steps 4–6) is documented
> procedure from the project's own verified guides, not a captured
> transcript — the verification machine's TUI gate issue (flagged above)
> stands between this book and a capture today. Where the guides specify
> exact banner text, event names, and counters, that text is quoted; where
> your machine's wording may differ, it says so. Chapter 9 replaces this
> honesty with determinism: once we meet the agent-test harness, these very
> steps get captured and pinned. For now: run it on your machine, and read
> the receipts it leaves.

### 1. Boot

```bash
~/relurpify/relurpish chat --workspace ~/code/stringkit
```

On a healthy boot you land on the chat tab: the **Postures banner** across
the top, three tabs — `[1] chat  [2] diff  [3] recipe` — and the input bar
at the bottom. Read the banner like a pilot reads the walk-around: *who
answers approvals* (governed — you), *who breaks intent ties*
(deterministic — you again).

### 2. Give the order

Type this and press Enter:

```text
The slugify function is broken: it keeps punctuation.
test_slugify_strips_punctuation fails. Fix slugify and run the tests to verify.
```

Watch what does *not* happen: no clarification frame. The instruction
scores confidently into `debug`, and in deterministic posture a confident
instruction proceeds. (If your phrasing straddles two families — say, you
ask for a fix *and* a refactor in one breath — you will see the frame; slot
number answers it, Enter takes the default, `d` defers.)

### 3. Grant the session

Somewhere in the loop, Euclo will want to write `stringkit.py`. The banner
appears; press `s`. That single keystroke says: this session may perform
this kind of write, and the grant goes into the audit chain. From here the
loop runs at the model's pace: list files, read the function, edit through
the governed `file_write`, run the governed test runner inside the sandbox,
observe green.

### 4. Grade the work

- The **diff tab** (`2`) shows the pending change — with your Chapter 1
  git baseline in place, this is exactly what Euclo touched. Expect a
  small edit to `slugify` that strips punctuation before hyphenating.
- Confirm from the shell:

```bash
python3 -m pytest test_stringkit.py
```

Expect: `2 passed`. The bug this book was born with is dead.

### 5. Read the receipts

Still from the shell, in the stringkit directory:

```bash
jq -r '.. | .logRecords[]? | select(.eventName=="euclo.route.selected")' \
    .relurpify_state/telemetry/otlp/*.jsonl | tail -3
```

Expect a record whose `decided_by` is `lattice:score` or `explicit` — the
classifier scored, the lattice chose, and the reason survived on disk.
Then the closing record:

```bash
jq -r '.. | .logRecords[]? | select(.eventName=="euclo.execution.complete")' \
    .relurpify_state/telemetry/otlp/*.jsonl | tail -1
```

Expect an outcome of success with the turn's real LLM call count and token
usage. And the audit:

```bash
ls .relurpify_state/audit/
```

Expect an entry for your `s` grant. This is the property the whole
framework exists to deliver: *you can reconstruct the turn from records
you did not have to trust — only read.*

### 6. Keep a transcript

Before you quit (`ctrl+c`):

```text
/export md ~/code/stringkit/turn1-transcript.md
```

The session transcript lands in the project. Between the transcript, the
diff, the audit entry, and the telemetry, you hold four independent
accounts of one turn.

## Gotchas

- **Approval keys go where the focus is — except when the banner is up.**
  The banner takes key priority, so `s` lands on the approval even mid-
  typing. No banner and an `s` keystroke becomes an `s` in your input.
  If a stray character appears in the input bar, the banner was not up.
- **`a` (always) outlives the session on purpose.** It is the grant you
  will forget you made. Chapter 3 shows where persisted grants live; treat
  `a` as a commitment, `s` as a habit.
- **A failed turn tells you why — read it.** `react decision unparseable`
  means the model's decision wasn't parsable and bounded repair could not
  recover it. `/retry` re-rolls the same prompt. With small local models,
  an occasional retry is the weather, not a flaw.
- **CPU pace is real.** A thinking model on a CPU takes minutes per turn.
  The feed narrates the loop the whole time — that narration is the
  runtime being inspectable, which is the point of the book.
- **Never feed the state directory.** If a recipe or tool misbehaves, the
  fix belongs in `relurpify_cfg/` (or in a bug report). Hand-editing
  `.relurpify_state/` is doctoring the receipts.

## Summary

- A turn is five stations: classify, select, clarify-if-ambiguous, execute
  governed, report an honest outcome.
- Selection is deterministic and its *reason* is recorded (`decided_by`).
- In governed mode, writes pause at a banner whose grants are scoped —
  once, session, or always — and audited regardless.
- The turn leaves receipts you can read without trusting anyone: the audit
  chain and two telemetry events, `euclo.route.selected` and
  `euclo.execution.complete`.
- Telemetry records decisions, not payloads — the privacy boundary that
  makes the receipts safe to keep and share.

## Check yourself

1. Your instruction scores into two families with no tie-breaker. What
   happens next in the default posture, and what are your three possible
   answers?
2. What is the difference between pressing `s` and `a` at the banner —
   in scope, in duration, and in what either choice writes to disk?
3. `euclo.execution.complete` shows a failure. Why is the named cause more
   trustworthy than a generic "something went wrong," and what single
   command re-runs the turn?
4. A teammate proposes editing a telemetry JSONL line to "clean up" a
   failed turn before sharing it. What are you actually doing when you do
   that, and what does the two-directory rule say about it?
5. Why does the runtime record decisions but not raw payloads in
   telemetry — what does that boundary buy you when you share receipts?

*Answers.* 1. A clarification frame appears; answer with a slot number,
press Enter for the default slot, or `d` to defer. 2. `s` grants this kind
of request for the session only; `a` persists across sessions — both write
to the audit chain, but `a` is a standing grant you must remember you
made. 3. The cause names the actual failing mechanism (e.g. an
unparseable model decision), so you can act on it rather than guess;
`/retry` re-runs the prompt. 4. You are doctoring the receipts — the state
directory is output, never source, and edited receipts are worse than
failed ones. 5. The boundary makes receipts safe to keep and share: they
prove what the runtime decided without exposing your source or the model's
raw output.

## Try it yourself

1. **Deny, and watch the refusal travel.** Reset stringkit (`git checkout
   -- stringkit.py`, keeping the test red if you like), boot, re-give the
   order, and press `n` at the write banner. Watch how the agent responds
   in the feed, then read the audit directory: the denial is in there.
   Re-run with `s` to actually fix it.
2. **The recipe registry, from the driver's seat.** In the TUI, run
   `/recipe`. You will see the shipped recipes Chapter 1's tree walk
   promised (`default`, `debug_tdd_repair`, `code_review`, ...), each with
   its families. Pin one with `/recipe euclo.thoughtrecipe.explain_test`,
   give an order, then check `euclo.route.selected` in telemetry: your
   `decided_by` should read `pinned`. Unpin with `/recipe clear`.

## Where next

One turn down, receipts in hand. But notice what you did *not* control:
the model, the posture values, the sandbox image, the runner — all of it
came from materialized defaults you have never read end to end. Chapter 3
opens the filing cabinet: every directory in `relurpify_cfg/`, the two
posture dials, environment-variable precedence, and the first `relurplint`
run. By its end, the kitchen's standing orders will be ones you chose on
purpose.

The lab for this chapter lives in
`docs/cooking-agentic-thoughts/labs/` — `verify.sh ch01` still guards the
red seed, and this chapter's receipts commands are collected there as a
command card (see `labs/ch02-first-turn/`).
