# Chapter 9 — Prompt Engineering, with a Metronome

Here is the uncomfortable truth about the last three chapters: everything
you authored — recipes, prompts, postures — was verified the expensive
way. You ran a turn, watched the feed, read the receipts, and *you* were
the regression suite. A recipe change that silently breaks behavior is
found the same way a chef finds a missing ingredient: by tasting
everything, every service, forever.

This chapter builds the metronome. Two movements: **craft** — the
context-discipline and goal-writing habits that make agents behave on
small local models — and **evidence** — the agent-test harness that turns
your agentic layer into something a machine can re-verify, offline, on
demand. By the end, the Chapter 2 slugify turn exists as a test case with
your name on it, and a model wobble has been caught by that test in front
of you.

## Where we left off

Your house style carries one standing order you wrote, proven loaded via
the resolution receipt. The full recipe drawer is authored. Everything
now depends on model time — which is exactly the dependency this chapter
removes.

## Technical requirements

- The end-of-Chapter-8 state, plus the relurpify repository checkout
  (the harness runs from it).
- `dev-agent` built from the repository: `go build -o dev-agent
  ./tooling/dev-agent-cli` (note the path — the testsuite README's
  quick-start says `./app/dev-agent-cli`, which does not exist in this
  tree; flag it when you see it).
- Ollama serving `gemma4:e4b` for the record run; **no model needed for
  replay**.
- About forty minutes. The record run is a real agent turn on CPU.

## Craft: the reduce half

Chapter 2 quoted the framing from the wider agentic-engineering
community: every context technique is **reduce** (shrink what the model
sees) or **delegate** (move work to a different window). Relurpify's
architecture is that discipline compiled: the context compiler assembles
what the model sees each turn under a token budget, with provenance —
"compiled, not stuffed" is not a slogan; it is a pipeline stage with a
budget. Your levers as an operator:

- **Scope is reduce.** Chapter 5's lesson compounding: a step's `may
  invoke` list is what the model gets told exists. Fewer tools, fewer
  decisions, fewer ways to spin.
- **Goals are reduce.** A goal states the task, the verification, and
  nothing else. Compare Chapter 2's two turns: the repair that succeeded
  named the defect and the test path; the one that spun asked the model
  to "find" things it could not see.
- **Stages are delegate.** The pipeline in Chapter 6 exists so planning
  context never contaminates implementation context.
- **The profile is reduce.** The `gemma4` profile (Chapter 3) sets
  `context.max_tokens: 8192` and arms bounded decision repair — the
  budget and the safety net in one file.

### Goal craft, distilled from this book's own turns

Three patterns, each earned by a captured turn:

1. **Verb-first, verification-carrying.** "Repair the defect, then run
   python_pytest on test_stringkit.py and report the real result." The
   verification is in the goal — Chapter 2's failed verification hid in
   its absence.
2. **Always interpolate the request.** `{{ index . "task.instruction" }}`.
   Non-negotiable since Chapter 4.
3. **Name paths and facts you hold.** The six-second repair of Chapter 2
   worked because the task said *which file*, *which defect*, *which
   test*, *where the test lives*. Precision in, precision out.

## Evidence: the agenttest harness

**dev-agent** is the repository's agent test runner: YAML-driven suites
where each case runs the real agent in a derived temporary workspace
against a real or replayed model, then asserts. It is development
tooling with an unstable surface — the API surfaces doc grades it
"unstable" — which is precisely why learning it now pays: you will use
it the way the maintainers do, and its rough edges are visible.

The assertion model is **OSB** — three tiers with different strength:

| Tier | Strength | Asserts |
|---|---|---|
| Outcome | hard | What the run achieved: success, files changed, output content |
| Security | hard | The sandbox contract — including *expected* violations |
| Benchmark | advisory | Telemetry hints: LLM calls, tokens, phases |

Outcome and Security failures fail the case; Benchmark results are
advisory. Execution profiles select where model traffic comes from:
`live` (real model), `record` (live, recorded into tapes), `replay`
(tapes only, no network — deterministic), plus CI-sized variants. A
**tape** is a fingerprinted record/replay fixture: replay matches
requests by SHA-256 fingerprint, and a mismatch is a loud re-record
error, never a silent pass. A passing live run can be **promoted** to a
golden fixture with lineage records; tapes older than thirty days are
stale and the tooling proposes re-record commands.

## Hands-on

### 1. Write the stringkit suite

Create `euclo.stringkit.testsuite.yaml` — the Chapter 2 turn as a case.
Modeled on the shipped smoke suite (`testsuite/agenttests/
euclo.gemma4.smoke.testsuite.yaml`), which is the reference for the form:

```yaml
apiVersion: relurpify/v1alpha1
kind: AgentTestSuite
metadata:
  name: euclo.stringkit
  description: "Stringkit regression suite: the slugify repair, pinned as a replayable case"
  owner: stringkit-maintainer
  tier: stable
  quarantined: false
spec:
  agent_name: euclo
  manifest: relurpify_cfg/agents/euclo.yaml
  execution:
    profile: record
    strict: false
    timeout: 300s
  workspace:
    strategy: derived
    exclude:
    - .book-scratch/**
    - devdocs/**
    - .relurpify_state/**
    - coverage
    - dev-agent-cli
    - relurpish
    - relurplint
  models:
  - name: gemma4:e4b
    endpoint: http://localhost:11434
  cases:
  - name: slugify_repair
    description: "The Chapter 2 turn: fix the punctuated slugify, verify with the test suite"
    task_type: code_modification
    tags:
    - stable
    - level:1
    timeout: 300s
    setup:
      files:
      - path: stringkit.py
        content: |
          """stringkit — small string utilities, kept tiny on purpose."""

          import re


          def slugify(text: str) -> str:
              """Turn a title into a URL-safe slug."""
              slug = text.strip().lower()
              slug = re.sub(r"\s+", "-", slug)
              return slug
      - path: test_stringkit.py
        content: |
          """Tests for stringkit."""

          from stringkit import slugify


          def test_slugify_strips_punctuation():
              assert slugify("Hello, World!") == "hello-world"


          def test_slugify_collapses_whitespace():
              assert slugify("  many   spaces  ") == "many-spaces"
    prompt: >
      The slugify function is broken: it keeps punctuation.
      test_slugify_strips_punctuation fails. Fix slugify and run the tests to verify.
    expect:
      outcome:
        must_succeed: true
        files_changed:
        - stringkit.py
        files_contain:
        - path: stringkit.py
          contains:
          - "re.sub"
```

The `exclude` list is load-bearing and earned: the harness derives a
temporary workspace by copying the repository root, and the first
captured run died twice in setup — once recursing into the author's own
output directory (`.book-scratch/**` must be excluded or the copy
copies itself until paths explode), once choking on a documentation
symlink (`devdocs/**`). Derive-copy is greedy; exclude what you do not
need.

### 2. Run it — and meet the metronome

From the repository root:

```bash
dev-agent agenttest run \
  --suite /path/to/euclo.stringkit.testsuite.yaml \
  --workspace /path/to/relurpify \
  --out /path/to/output-dir
```

The captured run on the verification machine:

```text
euclo.stringkit.testsuite.yaml [record]: 0/1 passed (0 infra, 1 assertion)
```

**The case failed — and that is the harness working.** The report shows
the whole story, every field a receipt:

```json
{
  "FailureKind": "assertion",
  "DurationMS": 87936,
  "TokenUsage": {"PromptTokens": 132345, "CompletionTokens": 1150,
                 "TotalTokens": 133495, "LLMCalls": 3},
  "route_selection": {"chosen_route": "euclo.thoughtrecipe.debug_tdd_repair",
                      "decided_by": "lattice:score"},
  "phase_metrics": [{"phase": "react", "duration_ms": 87281,
                     "llm_calls": 3, "tokens_used": 133495}],
  "knowledge": {"grounded_chunks": 3}
}
```

The runtime classified and selected exactly as Chapters 2 and 6 taught;
the react phase burned three calls and 133,495 tokens in 88 seconds —
and produced **no file change**. The workspace's `stringkit.py` after the
run: untouched, still buggy, tests still red. The suite caught it:
`must_succeed` and `files_changed` failed, the case went red, and the
paper trail — setup and execution reports, logs, OTLP telemetry — landed
in the output directory.

This is the sentence the chapter exists for: **on Tuesday, that wobble
would have been you re-reading a diff that never happened. Under the
metronome, it is a red case with a named cause, before it ever reaches
your afternoon.**

### 3. Grade your assertions too

Honesty compounds: one of the chapter's own assertions was too weak.
`files_contain: "re.sub"` passed on the *unfixed* file — the buggy
version already contains `re.sub`. The fix is to assert on the
*behavior*, not the tooling: assert the punctuation-stripping
substitution itself.

```yaml
        files_contain:
        - path: stringkit.py
          contains:
          - '[^\w\s]'
```

Assertions are code; review them like code. A test that passes on a
non-fix is worse than no test — it launders false confidence.

### 4. The record → promote → replay cycle

The full deterministic loop, from the project's testing reference:

```bash
# record: live calls captured into tapes (profile from the suite or flag)
dev-agent agenttest run --suite … --profile record

# promote a passing run into a golden fixture with lineage
dev-agent agenttest promote …

# replay: deterministic, offline, no network
dev-agent agenttest run --suite … --profile replay
```

Two captured honesty notes ride with this cycle. First, the chapter's
record run executed the case fully (real model, real receipts) but left
`RecordingMode: "off"` and an empty `TapePath` in its report — the tape
did not record from the suite-level profile alone on this build.
`[NEEDS CONFIRMATION: record-profile tape capture at your pin]` — the
promote/replay loop above is the documented workflow, and the repository
carries a committed tape with a lineage file
(`testsuite/agenttests/tapes/euclo.gemma4.smoke/`) as the reference
shape. Second, replay is strict about freshness: a fingerprint mismatch
is a loud re-record error, and tapes older than thirty days are stale —
the tooling proposes `agenttest rerecord` commands.

### 5. What the layers cannot catch

The harness closes Part III's loop, and it is worth being precise about
the division of labor your toolbox now enforces:

- **The linter** catches grammar, types, declarations (Chapters 4–6).
- **The boot** catches vocabulary and imports.
- **The runtime** catches schema-blocked tool calls, step failures,
  governance denials (Chapters 5–7).
- **The harness** catches behavioral drift — including the silent
  non-fix that no linter can see.
- **You** catch what none of these can: the fix that lands, passes, and
  is still wrong for reasons no assertion encodes. Read the diffs
  sometimes even when everything is green.

## Gotchas

- **The suite's exclude list is load-bearing.** Derived workspaces copy
  greedily; the captured failures were a self-copying output directory
  and a documentation symlink. Exclude output dirs, `devdocs/`, and
  state.
- **Weak assertions pass on non-fixes.** Assert the behavior
  (`[^\w\s]`), not the tooling (`re.sub`).
- **dev-agent is development tooling.** Unstable surface, per the
  project's own grading; pin expectations to your tag and re-verify on
  re-pin.
- **The README's build path is stale** (`./app/dev-agent-cli`); the
  binary builds from `./tooling/dev-agent-cli`.
- **Model wobble is a test result, not a flake to ignore.** The captured
  failure was a real non-fix; `/retry`-style re-rolls live in the
  harness too (`--max-retries`), but a case that fails often should
  change the task, the model, or the recipe — not the assertion.
- **Replay needs tapes; tapes need recording.** If `TapePath` reads
  empty after a record-profile run, the tape did not record — check the
  flag and the recording configuration before assuming the loop is
  broken.

## Summary

- The context discipline reduces to two moves — reduce and delegate —
  and Relurpify compiles both: budgets and provenance in the compiler,
  scope in the recipe, stages in the pipeline.
- Goal craft is three rules: verb-first with verification carried,
  always interpolate, name what you know.
- The agenttest harness runs your agentic layer as tests: OSB tiers
  (outcome/security hard, benchmark advisory), profiles from live to
  deterministic replay, tapes with fingerprints, promotion with lineage.
- The captured run proves the value in the negative: a silent non-fix
  became a red case with a full receipt chain — 3 LLM calls, 133,495
  tokens, a chosen route, and no changed file.
- Your agentic layer now fails loudly. Tuesday is safe.

## Check yourself

1. Your recipe's goal reads "Fix the bug in the workspace." Which two
   goal-craft rules does it violate, and what captured turn in this book
   demonstrates the cost?
2. Why are Benchmark-tier failures advisory while Outcome failures fail
   the case? Give one example of a signal that belongs in each tier.
3. The suite run printed `0 infra, 1 assertion` — what does that
   division tell you about where the failure lived, and which receipt
   would you read first?
4. Why does replay match requests by fingerprint and fail loudly on a
   mismatch, instead of fuzzy-matching "close enough" responses?
5. The chapter's first `files_contain` assertion passed on an unfixed
   file. What is the general principle, and what is the fixed
   assertion?

*Answers.* 1. No verification carried and nothing specific named; the
Chapter 2 spin (and its repair) showed unnamed tasks cost minutes and
fail. 2. Outcome/Security assertions define correct behavior — a
violation is a defect. Benchmark signals (call counts, tokens, phases)
vary with model and hardware; failing the case on them would make the
suite flaky, so they advise instead. Outcome: the file changed; 
Benchmark: LLM calls within a usual range. 3. Infra failures mean setup
or environment broke before judging (fix the harness path); assertion
failures mean the agent ran and misbehaved (read the case report first —
FailureKind, TokenUsage, route_selection). 4. Because "close enough"
responses are how false confidence enters: a drifted response that
roughly matches would silently pass and re-record would never trigger;
the fingerprint makes drift loud. 5. Assert observable behavior of the
*fix*, not presence of pre-existing tooling; the fixed assertion checks
the punctuation-stripping character class `[^\w\s]`.

## Try it yourself

1. **A second case.** Add `explain_coverage` to the suite: setup seeds
   the *fixed* stringkit, the prompt asks what `test_stringkit.py`
   covers, and the assertions are `must_succeed: true` +
   `no_file_changes: true` (a read-only ask — Chapter 4's explain_test
   as a case). Record, and if it passes, promote it.
2. **The staleness drill.** Note a tape's date, then consult the
   tooling's re-record proposal (`agenttest rerecord --help`). You are
   rehearsing the maintenance loop you will run every month.

## Where next

Craft and evidence are both yours: the prompts and recipes are authored,
and the layer they compose now fails loudly under a deterministic
harness. Part IV opens with the chapter the whole book has been circling:
Chapter 10, *Rewriting the House Style* — system-prompt engineering with
the DSL, the prompt-levels ladder, and the stretch where your agent
drafts a prompt and *you* decide whether it installs.

The lab for this chapter lives in
`docs/cooking-agentic-thoughts/labs/ch09-agent-tests/` — the suite, the
red capture with its full OSB receipt chain, the assertion-grade lesson,
and the three infra gotchas.
