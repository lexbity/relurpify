# Chapter 12 — live-model transcripts (captured)

Real output from the verification machine: live Ollama 0.40.8 serving
`gemma4:e4b`, Linux, the full ch02–ch12 overlay in a fresh scratch clone.
The Chapter 12 overlay (`ch12-live`) adds the build-tagged live test
(`cognitionzoo/critic/live_critique_test.go`, `//go:build live`) and the
agenttest harness fix (`testsuite/agenttest/execution.go` fork: a
permissive permission checker in the harness `Deps`, so governed-paradigm
recipe runs are not refused before they start).

## 1. The direct live critique — the contract holds

```text
$ go test ./cognitionzoo/critic/ -tags live -count=1 -run TestLiveCritique -v
=== RUN   TestLiveCritiqueUnderGemma4
    live_critique_test.go:109: live critique: verdict="fail" in 3.427s
    live_critique_test.go:109: live critique: verdict="fail" in 3.411s
    live_critique_test.go:109: live critique: verdict="fail" in 3.749s
    live_critique_test.go:109: live critique: verdict="fail" in 4.034s
    live_critique_test.go:109: live critique: verdict="fail" in 3.413s
    live_critique_test.go:126: verdict tally over 5 runs: fail=5 (decode failures: 0)
    live_critique_test.go:136: classification: stable (all 5 verdicts decoded)
--- PASS: TestLiveCritiqueUnderGemma4 (24.27s)
=== RUN   TestLiveCritiqueDeterminismProbe
    live_critique_test.go:159: probe: identical verdicts ("fail") —
        deterministic under zero temperature
--- PASS: TestLiveCritiqueDeterminismProbe (10.67s)
```

Read the verdict: `fail`, five for five — the live critic **refused to
rubber-stamp** the delegate's bare `{"summary": "draft done"}` against a
criterion demanding correctness, error handling, and naming. The
independent critic judged the draft honestly, strictly, and identically at
temperature 0. (Note the ollama provider kind registers from
`platform/llm/ollama`'s init — the live test imports it directly, the
composition root in production.)

## 2. The recipe path — degradation, captured with provenance

Fresh workspace, the agenttest suite, live model. The dispatcher now
selects the critic recipe deterministically:

```text
route: euclo.thoughtrecipe.critic_review   llm: 9   success: false
```

**This corrects Chapter 10's record**: the earlier `explain`-recipe
outcomes were warm-state artifacts (the ch10 scratch shared accumulated
`.relurpify_state`; fresh trees route to `critic_review`, matching the
Chapter 9 TUI run). The run executed — 8–9 LLM calls, drafter and
critique phases — and closed in the failure protocol with structured
telemetry:

```text
step operational failure
  node.id=run.16.5.reviewer.0.execute  action_taken=abort  kind=unknown
  message=critic critique output is not a valid verdict: invalid critique verdict ""
  on_error=abort  paradigm=critic  step_id=run.16.5.reviewer.0
```

The live model's critique response, through the instrumented/profiled
recipe path, decoded to an empty verdict; the D6 retry fired exactly
once; the failure protocol aborted honestly. `[NEEDS CONFIRMATION: root
cause of the empty-content response through the recipe path (thinking-
channel capture vs. profile wrapping) — the direct-path Chat decodes
5/5.]`

## 3. Three runs, three outcomes — the flake-tier evidence

```text
ch10 scratch (warm):  euclo.critic.testsuite.yaml [record]: 2/2 passed   (routed: explain)
ch12 run 1 (fresh):   euclo.critic.testsuite.yaml [record]: 0/2 passed   (critic_review; decode abort)
ch12 run 2 (fresh):   euclo.critic.testsuite.yaml [record]: 1/2 passed   (critic_review; case A green in 41s,
                                                                      case B assertion failure)
```

The honest classification: the suite's security-envelope assertions are
stable; the recipe-layer outcomes are **live-flaky** — routing state and
verdict-decode variance decide them. That is what `tier: live-flaky`
exists to say, and saying it is the tier system working.

## 4. The harness fix that made the recipe path runnable

Before this chapter's overlay, any critic-recipe-routed case was refused
before the first model call — the harness's `Deps` carried no
`PermissionChecker`, so the Chapter 8 fail-closed bar fired (the same
refusal the Chapter 9 golden records, by design). The overlay's one-line
fix mirrors the conformance harness's own stance:

```go
// The harness runs in an isolated workspace with its own registry — a
// permissive checker matches the conformance harness's stance.
PermissionChecker: harnessPermissiveChecker{},
```

With it, behavior under live models is the model's honesty, not a
wiring refusal. (This fork is a PR candidate for the framework proper.)

## 5. Profile selection is a filter, not an override

```text
$ ./dev-agent agenttest run --suite … --profile ci-live
no testsuites matched the requested filters
```

`--profile` selects suites whose **declared** profile matches
(`MatchesProfile` compares against the suite header) — despite the CLI
help text saying "override". A suite meant for the CI lane declares
`profile: ci-live` in its header. The `record`-declared critic suite is
therefore not selectable under `--profile ci-live` — a CLI-doc quirk,
captured as found.

## 6. Replay refuses, fail-visibly (open item 5, resolved-as-finding)

A replay-declared variant of the suite, no committed tape:

```text
$ ./dev-agent agenttest run --suite critic_replay_variant… --case critique_readonly_route
replay tape unavailable at …/tape.jsonl: stat …: no such file or directory
```

With the Chapter 10 finding (the ollama provider ignores `TapePath`;
the registered `tape` kind is replay-only; no recording path exists in
the tree), the replay tier's behavior is the honest close of the loop:
**no tape can be produced, so replay refuses rather than silently going
live.** The tape seam's vocabulary (record/replay, SHA-256 fingerprints,
promotion, lineage, 30-day staleness) remains real and taught; its
recording tooling is a framework follow-up.

## 7. Gates

```text
$ ./verify.sh gates ch12        # build, vet, tests, lint-arch, goldens, conformance, coverage
[PASS] gates: ch12 holds the framework's law
```
