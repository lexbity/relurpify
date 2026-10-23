# Chapter 12 — Meet a Real Model

Every proof so far rode a metronome: the fake model, the scripted
conformance queues, the offline backend. The metronome taught you the
framework's *shape* — but a paradigm is only honest about real models
after it has met one. This chapter hands the clock over. You will run the
critic against the live `gemma4:e4b`, watch it refuse to rubber-stamp a
weak draft, watch the recipe path degrade a verdict format and recover
honestly, and classify what you saw in the framework's own flake
vocabulary. One chapter from now, the book hands you the PR.

## Where you are

All gates green (Chapter 11); the paradigm complete. This chapter's
overlay adds a build-tagged live test and a one-line harness fix, and its
captures are all live: Ollama at `localhost:11434`, `gemma4:e4b`, the
model book 1 trained you on.

## The live test, and why it is tagged

Live tests in this repository hide behind a build tag — the harness's own
live driver carries `//go:build live`, and so does yours:

```text
$ go test ./cognitionzoo/critic/ -tags live -count=1 -run TestLiveCritique -v
```

Without the tag, the file does not exist; CI stays offline; nobody's
pipeline breaks because a model server was down. The test skips — not
fails — when the backend is unreachable, because a lab machine without
Ollama lacks a *model*, not a *framework*. And one wiring detail the
build surfaced: the ollama provider kind registers from
`platform/llm/ollama`'s package init. Production links it through the
composition root; your test imports it directly. Miss that import and
`llm.New` reports `unknown provider kind "ollama"` — registration is a
link-time fact, not a config one.

## The direct critique: the contract holds

The live test drives the critic's directive loop with a recording
delegate — the surface under study is the **critique phase's verdict**,
so the drafter is a double and the model is real. One criterion, five
runs:

```text
live critique: verdict="fail" in 3.427s
live critique: verdict="fail" in 3.411s
live critique: verdict="fail" in 3.749s
live critique: verdict="fail" in 4.034s
live critique: verdict="fail" in 3.413s
verdict tally over 5 runs: fail=5 (decode failures: 0)
classification: stable (all 5 verdicts decoded)
```

Sit with what the model did. The delegate's work product was a bare
`{"summary": "draft done"}` — and the criterion demanded correctness,
error handling, and naming. A sycophant would say *pass*. A self-reviewer
would say *pass* — it wrote the summary. An independent critic read the
material against the standard and said **fail**, five times, in about
three and a half seconds each, identically at temperature zero. The
determinism probe confirms it: same input, same verdict, no drift. The
thesis of this book — separation is the mechanism — just held under a
real model.

The test's assertions are deliberately asymmetric, and the asymmetry is
the lesson: it *hard-asserts* the framework's honesty (every run closes;
at least one verdict decodes; no impossible verdict escapes strict
decode) and *logs* the distribution. A live test that hard-asserts a
model's opinion is a flake generator; a live test that hard-asserts the
framework's handling and records the model's behavior is evidence.

## The recipe path: degradation, honestly

Now the same model through the whole machine — the agenttest suite, a
fresh workspace, the real dispatcher. Two things happen, and both are the
chapter's core material.

**First, the route.** A fresh workspace dispatches the critique-shaped
instruction to `euclo.thoughtrecipe.critic_review` — deterministically.
This corrects Chapter 10's record: the earlier `explain`-recipe outcomes
were warm-state artifacts of a shared scratch tree; the fresh-tree route
matches the Chapter 9 TUI run. Route selection reads workspace state, so
suite outcomes inherit that state's temperature. (Chapter 10's
`[NEEDS CONFIRMATION]` on dispatcher scoring now has its mechanism named,
and the correction is in this chapter's transcript — the book keeps its
own record honest.)

**Second, the refusal — and then the run.** Before this chapter, the
harness's `Deps` carried no `PermissionChecker`, so a critic-recipe-routed
case died before the first model call: the Chapter 8 fail-closed bar, the
same refusal the Chapter 9 golden records. The overlay's one-line fix
mirrors the conformance harness's own stance (a permissive checker inside
the isolated workspace and its own registry — real denials remain the
live runtime's business) and is a PR candidate for the framework proper.
With it, the recipe runs — and under the live model, the ninth call
brought the degradation this chapter promised:

```text
step operational failure
  node.id=run.16.5.reviewer.0.execute  action_taken=abort
  message=critic critique output is not a valid verdict: invalid critique verdict ""
```

The critique response through the instrumented, profile-wrapped recipe
path decoded to an empty verdict. What followed is the framework's
decision-repair discipline end to end: the strict decode refused, the D6
retry fired exactly once, the failure protocol recorded an operational
failure with node, step, paradigm, and action — and aborted. No garbage
verdict routed the loop. No silent pass. The receipt is readable in one
telemetry line. `[NEEDS CONFIRMATION: the empty-content response's root
cause through the recipe path (thinking-channel capture vs. profile
wrapping) — the direct-path Chat decodes five for five.]`

## Three runs, three outcomes: the flake tiers earn their names

The suite's three captured runs:

```text
warm scratch:   2/2 passed   (routed: explain)
fresh, run 1:   0/2 passed   (routed: critic_review; verdict decode aborted)
fresh, run 2:   1/2 passed   (routed: critic_review; case A green in 41s)
```

Now say the honest sentence out loud: **the security-envelope assertions
are stable; the recipe-layer outcomes are live-flaky.** That is not a
confession of bad tests — it is exactly what the tier vocabulary exists
to encode. `smoke` for the envelope (fast, high-signal, deterministic);
`live-flaky` for behavior that a real model's mood and a workspace's
route state genuinely decide; `quarantined` for the broken, with
`--include-quarantined` the only door back in. Misclassifying a
live-flaky case as stable is how CI gets ignored; misclassifying a
stable one as flaky is how regressions walk through. The chapter's tally
is the classification exercise done on real data: three outcomes, one
mechanism (route state + verdict-decode variance), one tier.

## The knobs, and their vocabulary

When a decision format degrades, the remediation vocabulary is the model
profile — and every word of it is closed-set, source-verified:

- **`tool_calling.intent`** — `native` / `prompt_based` / `auto`. The
  gemma4 profile pins `native` (its template speaks tool calls); the
  materialized default profile ships `auto`, and Chapter 1 captured what
  `auto` + a tool-less template does to a *react* decision (the leak) and
  the one-line fix (`prompt_based`). The critic's *verdict* channel is a
  plain Chat, not tool calling — but the drafter underneath the recipe
  rides the same intent knobs, which is why the profile is the first
  place to look when a recipe run's decisions go textual.
- **`repair.strategy`** — `heuristic-only` or `llm`, with bounded
  attempts: whether a malformed decision is repaired locally or
  re-asked. You watched the paradigm-level twin of this all chapter:
  strict decode, one retry, honest abort.
- **The doctor names the remediation** — Chapter 1's
  `model_tool_template` row diagnosed the artifact and prescribed the
  profile edit in plain language. The loop this book keeps running —
  *the doctor names the problem, the fix lives in a file you own* — is
  the profile-tuning workflow.

Benchmarks stayed advisory through all of it: the token counts and call
counts are recorded per case (four to nine calls, ~600 tokens per phase)
and nothing failed on them. The receipts stayed readable: every failure
in this chapter is one telemetry line or one CLI line with coordinates.

## Replay refuses (and that closes open item 5)

The plan promised a promoted tape and a replay proof. Chapter 10 found
the recording path absent from the tree; this chapter demonstrates the
other half — a replay-declared suite with no tape refuses to run:

```text
replay tape unavailable at …/tape.jsonl: no such file or directory
```

That refusal is the correct behavior of a seam whose recording tooling
does not exist yet: fail-visible, never silent-live. The vocabulary
(fingerprints, promotion, lineage, 30-day staleness) remains real and
taught; the tooling is a framework follow-up this book's PR names in its
gaps. And one CLI semantics captured on the way: `--profile` **filters**
suites by their declared profile (`MatchesProfile`) rather than
overriding execution as its help text implies — a `record`-declared
suite is simply not selectable under `--profile ci-live`. Small, real,
and now written down.

## Gotchas

- **Live tests skip, they don't fail, on an unreachable backend.** And
  they carry a build tag so offline CI never sees them.
- **Import the provider package or the kind doesn't exist.** `llm.New`
  errors with `unknown provider kind` until `platform/llm/ollama`'s init
  registers it.
- **Route state is suite temperature.** Warm and fresh workspaces
  dispatch differently; clean `.relurpify_state` between captured runs
  or your transcript mixes states.
- **Hard-assert the framework, log the model.** A live test that asserts
  a model's verdict is flake-bait; assert the decode, the retry, the
  close — record the opinion.
- **`--profile` is a selector.** Declare the profile you mean in the
  suite header; the flag chooses among declared suites.

## Summary

- The direct live critique held the contract: five independent runs,
  five honest `fail` verdicts against a weak draft, zero decode
  failures, deterministic at temperature zero — separation working under
  a real model.
- The recipe path met real degradation: strict decode refused an empty
  verdict, the D6 retry fired once, the failure protocol aborted with
  full provenance — and the harness fix (a permissive checker in its
  isolated `Deps`) that made the run possible is a PR candidate.
- Three suite outcomes across routing state and decode variance earned
  the tier names honestly: stable envelope, live-flaky recipe layer.
  Classification is the deliverable.
- The profile is the remediation vocabulary (intent, repair strategy),
  the doctor names the fix, benchmarks stayed advisory, and replay
  refuses fail-visibly — open item 5 closed as a finding with the
  follow-up named.

## Check yourself

1. Your live test hard-asserts some things and logs others. Which is
   which, and why is the line drawn exactly there?
2. The five live verdicts were all `fail`. What does that tell you about
   the critic that a hundred *pass* verdicts would not?
3. Name the two independent sources of the suite's live-flakiness this
   chapter captured, and which tier each layer of assertion belongs to.
4. The recipe run aborted with `invalid critique verdict ""`. Walk the
   handling chain from decode to abort, naming each mechanism and where
   a profile knob could intervene.
5. Why is "replay tape unavailable" the *right* error for this tree, and
   what would have been wrong with silently running live instead?

*Answers.* 1. Hard: every run closes, at least one verdict decodes, no
invalid verdict escapes strict decode — the framework's handling. Log:
the verdict distribution and timing — the model's opinion. 2. That the
critic applies the criterion against the material rather than deferring
to the draft's self-assessment — a sycophantic or self-reviewing loop
produces passes on weak work; five independent fails are the separation
working. 3. Route state (warm vs. fresh dispatches differently — the
envelope assertions stay stable across it) and verdict-decode variance
under the live model — recipe-layer outcomes are live-flaky; a
`tier: smoke` suite with live-flaky recipe dependencies documented, or a
split suite, is the honest structure. 4. Strict decode (`ErrInvalidCritique`)
→ the run-node's D6 model-invalid-output retry (exactly once) → the
failure protocol records the operational failure (node, step, paradigm,
action=abort). A profile with `repair.strategy: llm` and bounded attempts
re-asks before the abort; `tool_calling.intent` governs the *drafter's*
decision format underneath. 5. Because no recording path exists in the
tree, no tape can be produced; refusing keeps the replay tier's promise
(pinned, reproducible execution) instead of quietly substituting an
unpinned live run — a silent live run would certify reproducibility it
did not have.

## Try it yourself

1. **Stress the classification.** Raise the live test's runs to 20 and
   record your machine's tally. If decode failures appear, re-classify
   honestly and write the tier line you would put in the PR. You are
   doing the chapter's last exercise with your own machine's opinions.
2. **Meet the profile knob.** Copy the suite, declare
   `execution.profile: ci-live` in its header, and re-run with
   `--profile ci-live`. Note what changes (strict mode) and what does
   not (the model). Then read `relurpify_cfg/model/profiles/gemma4.llm.yaml`
   end to end — every key you have met in this book is in that file.

## Where next

The paradigm has met a real model and its receipts are readable. Chapter
13 shapes everything into the contribution: the clean-checkout gauntlet
from nothing, DCO and commit style, the PR checklist with every gap
named — the `[NEEDS CONFIRMATION]`s this book carried, the harness-fix
candidate, the tape follow-up — and the review rubric a maintainer reads
first. The book closes by handing your work to someone else.

The lab lives at `devdocs/agent-construction-relurpify/labs/` —
`verify.sh gates ch12` re-runs the tier, and `transcripts/ch12-live.md`
holds every live capture in this chapter, verbatim.
