# Chapter 11 — Out of the Loop, Not Out of Mind

The driving lesson from the wider agentic-engineering world goes: in-loop
(hands on the wheel) → out-loop (cruise control, you in the seat) → the
aspiration some call zero-touch engineering (the robotaxi). The same
world's honest critique answers itself: a loop running unattended is also
a loop failing unattended, and the failures scale with the autonomy.

This chapter is both halves, in one kitchen. You will hand stringkit's
repair to the queue, answer one approval from the other side of the
loop, and let the runtime work while you watch receipts instead of the
feed. The machinery will do everything Chapter 2 through 10 built toward.
The model will wobble anyway. And you will close this book knowing
exactly why that is not a failure of the design — it is the design
working.

## Where we left off

The house style is yours: two live prompts edited with receipts, the
levels ladder mapped rung-for-rung onto mechanisms you own, and the
manual gate drawn at the top — machine-drafted, human-installed. The
metronome from Chapter 9 stands ready. One question remains, and it is
the book's oldest one: how far can the kitchen run alone while the
permit is still yours?

## Technical requirements

- The end-of-Chapter-10 state: binaries, green lint, Ollama serving
  `gemma4:e4b`, and stringkit **red** (the seed has been restored — the
  final exam needs a real defect).
- About thirty minutes for the exam, plus reading time for the close.
- Captured on the verification machine, as every chapter before it.

## The ladder, and what governance adds

The in-loop → out-loop ladder is a ladder of attention. In-loop, you are
the loop: prompt, wait, read, prompt. Out-loop, you hand over a task and
the agent works while you are elsewhere; you come back to a result. The
community framing is crisp about the price: your output is capped by your
attention in-loop, and one agent costs one person — but out-loop only
works if the loop can be *trusted*, and trust is earned in receipts, not
declared in prompts.

What Relurpify adds is the part the ladder usually skips: **governance is
what makes letting go survivable.** The community's own critique lists
the ways unattended loops fail, and each named failure has a counter you
have already operated:

| Named risk (the critique) | Your counter (this book) |
|---|---|
| Same-model review — correlated errors | Governed approvals + the audit chain (Ch 2); *your* tests as the independent check (Ch 9) |
| The ticket is an attack surface — untrusted text driving a privileged agent | Deterministic classification and routing, sandbox, schema-blocked tool calls (Ch 3, 7) |
| Self-improvement compounds errors — memory poisoning by accident | The manual gate: machine-drafted, human-installed (Ch 10) |
| Nobody decides what to build — ambiguous tickets ship fast and wrong | Clarification frames: ambiguity resolves to a question to *you* (Ch 2) |
| Local fixes, global mess — architecture rots quietly | `relurplint`, the fences, and receipts that make drift visible (Ch 3) |

The critique's conclusion — autonomy scales whatever your checks catch,
so invest in verification before autonomy — is this book's Chapter 9.
The out-of-loop ladder is not a place; it is a *ratio*, and the
denominator is how good your checks are.

## Hands-on — the final exam

### 1. Queue the repair

stringkit is red (`1 failed, 1 passed`). From inside relurpish, instead
of typing the task as a turn, queue it:

```text
/queue The slugify function is broken: it keeps punctuation. test_slugify_strips_punctuation fails. Fix slugify and run the tests to verify.
```

Queued tasks execute sequentially — and because this one must *write*,
the session needs a mode with edit rights. The captured session surfaced
a **mode picker** at exactly this point:

```text
[1] local-read-only    [2] local-edit-on    [3] online-read-on ...
```

Answer `2` — a repair writes files. Then the governed loop engaged, as
it has since Chapter 2: the approval banner appeared, and `s` granted
the session.

### 2. Let it work — and watch the receipts, not the feed

Now do the out-of-loop thing: leave the feed alone. Poll the tests from
another terminal, the way Chapter 9 taught:

```bash
cd ~/code/stringkit && python3 -m pytest test_stringkit.py -q
```

The captured session's arc, minute by minute:

- The approval banner rose within seconds of the queue; `s` granted.
- The loop ran for roughly nine minutes on the verification CPU: reads,
  a governed write, test runs.
- The diff landed — and it is worth reading in full, because it is the
  book's whole argument in four hunks:

```diff
-"""stringkit — small string utilities, kept tiny on purpose.
+stringkit — small string utilities, kept tiny on purpose.
 ...
-    slug = re.sub(r"\s+", "-", slug)
+    # Remove punctuation (keep only alphanumeric and spaces)
+    slug = re.sub(r'[^\w\s]', '', slug)
+    # Replace whitespace with a hyphen
+    slug = re.sub(r'\s+', '-', slug)
```

The fix is *right* — punctuation stripped, whitespace hyphenated, the
exact repair Chapter 2's model eventually landed. And the rewrite
*mangled the module docstring again*, exactly as Chapter 5's did. Same
model, same signature, out-of-loop this time:

```text
1 error in 0.12s
```

SyntaxError. The queued repair completed; stringkit is still red.

### 3. Read the closing receipt — and let it close the book

```bash
jq -c 'select(.resourceLogs) | .resourceLogs[]?.scopeLogs[]?.logRecords[]?
       | select(.eventName=="euclo.execution.complete")' \
    .relurpify_state/telemetry/otlp/*.jsonl | tail -1
```

The captured attributes for the queued run: `outcome: "success"`.

There it is — the book's first lesson, returned at out-of-loop scale.
The telemetry is not lying: the run *completed*, approvals honored, diff
landed, loop closed. The tests are still red because the model's rewrite
broke the file. Complete is a claim. Your tests are the fact. The
out-of-loop version of that sentence is the supervisor's burden: **the
further you stand from the loop, the more your checks — not your eyes —
are the kitchen's quality.** The audit chain now holds twenty records;
the session's every decision is reconstructible without having watched
any of it.

Then inspect what persistence kept:

```text
/workflows
```

The persisted workflow list is the resume point — `/workflow <id>`
inspects one, `/resume <id|latest>` re-enters it, `/rerun <workflow>
<step>` re-executes a single step, `/cancelwf` stops one. This is the
machinery of supervision: state that outlives the session, and commands
that re-enter it.

### 4. Permissive mode, priced

One dial remains unexamined. `--hitl-mode permissive` auto-approves every
request and writes each decision to the audit trail; sandbox and
capability enforcement are unchanged, and the doctor marks the posture
degraded. What it buys: out-of-loop runs that never pause for you. What
it costs: the one moment where your judgment outranked the model's now
passes without you — audited, but unguarded. The book ran governed for
eleven chapters; if you flip this dial (Chapter 3's banner command), flip
it for tasks whose diffs you will read the same day. The robotaxi
critique's version: skipping straight to zero-touch with weak checks is
shipping bugs faster.

### 5. Daily operation, in one paragraph

You have been running the real model since the D5 amendment made Ollama
the spine — so "going real" needs only hygiene: `doctor` green before
long sessions; `ollama pull` when you change models and
`workspace.yaml` to match; `/retry` for parse wobbles (bounded repair
first); the metronome (Chapter 9) before trusting any recipe or prompt
change; and the receipts — audit chain, telemetry, diffs — as the daily
newspaper of the kitchen. That is the whole operating manual.

## The thesis, at the end

The repository's tagline is *to the day it rewrites itself*. After
eleven chapters you can read that line precisely. The kitchen will
rewrite itself the day its checks are better than its writers' — and
this book has taught you to build those checks first: receipts before
autonomy, verification before delegation, a human gate exactly where
taste is non-negotiable. Chapter 10's drafting recipe drafts; *you*
install. That gate is not the last workaround before the future. It is
the design — and the day the runtime could honestly hold it alone, the
audit chain will be the thing that proves it.

Until then: supervise the system you built. You know how. You have the
receipts.

## Gotchas

- **Queued writes need a write-capable mode.** The captured session
  surfaced the mode picker at queue time; `local-edit-on` is the answer
  for repairs. A queued task in a read-only mode stalls where a write
  would happen.
- **Governed approvals out-of-loop still mean you.** The queued run
  waited at the banner until you answered. If you are genuinely away,
  the run waits — bounded autonomy is the contract, and it is the
  permissive dial (with its price) that changes it.
- **`outcome: "success"` and red tests can coexist.** Third time this
  book has said it; out-of-loop is where it bites hardest, because you
  were not watching the feed. The metronome and your own pytest are the
  standing checks.
- **Small-model rewrites mangle containers.** Two of this book's three
  repairs broke the docstring while fixing the function. Read whole
  diffs; Chapter 2's fix-the-repair pattern is a permanent tool.
- **Workflow commands are the supervision surface.** `/workflows`,
  `/workflow <id>`, `/resume`, `/rerun <workflow> <step>`, `/cancelwf`
  — learn them before the day you need them.

## Summary

- The out-of-loop ladder is a ratio of trust to checks, and Relurpify's
  governance is the checked version of letting go: queued work, mode
  gates, approvals, audit, receipts.
- The final exam ran clean mechanically and wobbled substantively — the
  correct fix, the mangled container, the red tests — and every step of
  it was reconstructible from receipts you did not have to watch being
  written.
- `outcome: "success"` closed a red-testing run; the supervisor's burden
  is real and it is yours.
- Permissive mode is a priced dial, not a promotion.
- The thesis holds: build the checks first, hold the gate, and the day
  the kitchen rewrites itself will arrive with receipts.

## Check yourself

1. The queued repair needed two answers from you before work started.
   Which two, and which postures decided that those answers were yours
   to give?
2. The queued run's telemetry says `success`; your tests say
   `SyntaxError`. Reconcile — precisely, using the outcome vocabulary
   from Chapters 2 and 10.
3. A teammate proposes running all future queued work in permissive
   mode "because approvals block the queue for hours." What does
   permissive actually change, what does it preserve, and what is the
   book's condition for accepting the proposal?
4. Name three of the five out-of-loop risks from the community critique
   and the specific Relurpify counter you have operated for each.
5. `/resume`, `/rerun`, and `/cancelwf` — which one do you reach for
   when (a) a workflow failed at step three of five, (b) a queued task
   turns out to be wrong entirely, (c) you want to continue yesterday's
   half-finished session?

*Answers.* 1. The mode picker (queued writes need `local-edit-on`) and
the approval banner (governed posture) — both decided by the postures
you set in Chapter 3. 2. The runtime's outcome records that the
execution completed — approvals honored, loop closed; it does not grade
the rewrite. `unverified` (Ch 10) would have been closer if the runtime
detects no produced artifacts; either way the SyntaxError is your
check's finding, and the two documents measure different things. 3.
Permissive auto-approves each request (audited) and leaves sandbox and
capability enforcement intact; the doctor marks the posture degraded.
It preserves enforcement and receipts and gives up the human gate.
Accept it for task classes whose diffs are verified the same day by
checks stronger than the model that wrote them — and revisit the
posture's degraded marking. 4. Any three of: same-model review →
governed approvals + your independent tests; ticket injection →
deterministic classification, sandbox, schema-blocked calls; memory
poisoning → the manual install gate; ambiguous tickets → clarification
frames; architecture rot → relurplint + fences + receipts. 5. (a)
`/rerun <workflow> <step>` for the failed step, (b) `/cancelwf`, (c)
`/resume <id|latest>`.

## Try it yourself

1. **Graduation.** Get stringkit green — you hold every tool: the
   Chapter 2 pattern (name the defect, name the test path), `/retry`,
   the diff tab, and your own pytest. Then commit, and add the final
   line to the repo's history: *repaired under supervision, across
   eleven chapters.*
2. **Pin the layer.** Point your Chapter 9 suite at the repaired
   stringkit (a regression case: no_file_changes on a read-only ask;
   the repair case re-recorded), and re-run it green. The layer you
   built in Part II now fails loudly under the harness you built in
   Part III. That combination — authored, receipted, and
   machine-verified — is the book's definition of done.

## Where next

There is no next chapter. There is a shelf: the *builder's* book, where
the graph node library and the paradigm contract system you met in
Chapter 6 become things you extend; the *internals* book, where the
bi-directional knowledge compiler and the benchmark suite open up; and
the project itself — contributing, the gates, the receipts. The kitchen
is yours. Close the diff, run the tests one last time, and go cook
something of your own.

The lab for this chapter lives in
`docs/cooking-agentic-thoughts/labs/ch11-out-of-loop/` — the final exam
transcript, the diff, the closing receipt, and the supervision command
card.
