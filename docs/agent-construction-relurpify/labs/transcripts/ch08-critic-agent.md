# Chapter 8 — critic agent transcripts (captured)

Real output from the verification machine (same environment as the earlier
transcripts). The Chapter 8 overlay (`ch08-critic-agent`) adds
`cognitionzoo/critic/critic.go` (the agent), the workspace prompt file
`relurpify_cfg/prompts/agents/agent.critic.default.prompt`, and
`cognitionzoo/critic/critic_agent_test.go`.

## Package tests (ch02–ch08 overlays)

```text
$ go test ./cognitionzoo/critic/... -count=1 -v        (32 tests)
--- PASS: TestCriticFailsClosedWithoutPermissionChecker (0.00s)
--- PASS: TestCritiqueWritesScratch (0.00s)
--- PASS: TestReviseFiresOnPredicateAndRecritiques (0.00s)
--- PASS: TestRevisionCapEndsLoopNormally (0.00s)
--- PASS: TestLibraryModeReportsWithoutRevision (0.00s)
--- PASS: TestCriticPromptFileLoadsAndValidates (0.00s)
--- PASS: (…the twenty-six Chapter 2–7 tests…)
PASS
ok  	codeburg.org/lexbit/relurpify/cognitionzoo/critic	0.061s
```

The six new tests, and what each pins:

- `TestCriticFailsClosedWithoutPermissionChecker` — deps without a
  permission checker: `Execute` returns `critic.ErrNoPermissionChecker`
  (asserted with `errors.Is`). The ReWOO bar, stated as an error rather
  than a silent downgrade.
- `TestCritiqueWritesScratch` — the draft phase runs through the delegate
  once, the critique phase issues one `Chat` with the authored criteria in
  the prompt (asserted via the fake model's `LastInput`), the verdict
  lands in `scratch.critic.verdict` / `scratch.critic.issues`, and the
  result fields carry `critique`.
- `TestReviseFiresOnPredicateAndRecritiques` — the revise body runs
  exactly once when the verdict reads issues, then the loop re-critiques
  to a pass: two critique calls total.
- `TestRevisionCapEndsLoopNormally` — persistent issues run at most
  `critic.MaxRevisionCycles` (2) bodies, the telemetry sink records
  `critic.revision_capped`, and the run ends *successfully* with the last
  verdict standing — a cap is a bound, not a malfunction.
- `TestLibraryModeReportsWithoutRevision` — the zero value (no authored
  options) runs the library-mode graph: draft → critique → done, no
  revision.
- `TestCriticPromptFileLoadsAndValidates` — the workspace prompt file
  (shipped in the overlay at
  `relurpify_cfg/prompts/agents/agent.critic.default.prompt`, following
  the `agent.<agent-name>.default` convention) loads through the prompt
  registry from a `fstest.MapFS` and validates clean.

## Notes from the build

- The critic's telemetry event strings follow the shipped
  `<paradigm>.<event>` vocabulary (`critic.iteration`,
  `critic.reviewed`, `critic.revision_capped`, `critic.completed`) as
  package-local constants; upstreaming named constants into the telemetry
  package's vocabulary file is part of the Chapter 13 PR checklist.
- The prompt validator flags declared-but-unused variables (the shipped
  `agent.react.default.prompt` carries two as well); the critic's file
  declares none — its criteria arrive through the agent's options.
- The runner binary now links the full paradigm: contract, agent, prompt
  file, ensemble builder — and still runs its ensemble transcript
  unchanged.

## Gates

```text
$ ./verify.sh gates ch08
== tier 2 (gates): ch08 ==
… (build, vet, tests, make lint-arch — all PASS)
[PASS] gates: ch08 holds the framework's law
```
