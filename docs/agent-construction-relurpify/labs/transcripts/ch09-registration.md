# Chapter 9 — registration & DSL transcripts (captured)

Real output from the verification machine (same environment as the earlier
transcripts). The Chapter 9 overlay (`ch09-registration`) carries the
registration wiring — modified copies of four repository files at their
true paths, plus the recipe and golden fixtures — applied into a scratch
clone; the main repository is never modified.

## 1. The wiring, as a diff summary

Four files change, each by the smallest honest edit, each following the
shipped pattern for the eight existing paradigms:

- `lexer.go` — `"critique": {}` joins the reserved keyword set (between
  `review` and `revise`, its declaration-order neighbors).
- `parser.go` — `critique` joins the directive-item case list (the
  generic `parseDirectiveItem` handles the grammar).
- `paradigm_options.go` — `criticOptions` lowers the directive payloads
  (mirroring `reflectionOptions` line for line) and `newCriticReviseBody`
  runs authored body steps in-process via `ExecuteDelegateCore`.
- `step_core.go` — `case "critic":` in `buildAgent`'s switch (mirroring
  the reflection case; the default branch's `ErrContractViolation` guard
  is untouched).

## 2. Goldens: untouched, then one deliberate baseline

With the wiring applied, every existing golden passes byte-identical, and
only the new recipe lacks its triple:

```text
--- FAIL: TestGoldenTraces/11_critic
    golden_trace_test.go:49: read golden testdata/golden/11_critic.trace.json:
    no such file or directory (set UPDATE_GOLDEN=1 to create)
```

Deliberate re-baseline, then the byte-identical proof:

```text
$ UPDATE_GOLDEN=1 go test ./named/euclo/thoughtrecipes/ -run TestGolden
ok  	codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes	0.270s
$ go test ./named/euclo/thoughtrecipes/ -run TestGolden -v
--- PASS: TestGoldenPlans (0.00s)
--- PASS: TestGoldenGraphs (…)      [all existing triples byte-identical]
--- PASS: TestGoldenTraces (0.13s)
ok  	codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes	0.273s
```

The new `11_critic` triple records the plan (recipe lowered: agents
`fixer`/`reviewer`, the critic step), the graph (valid, 2 nodes), and the
trace — whose exec error is the governance bar reaching the DSL harness:

```json
{
  "graph_valid": true,
  "start_node": "run.10.5.reviewer.0.execute",
  "exec_success": false,
  "exec_error": "critic agent requires a permission checker (fail closed)"
}
```

The golden harness's minimal deps carry no permission checker (the same
honesty that records `react agent missing language model` in the shipped
traces) — and the critic's fail-closed refusal lands in the same
protocol. The bar survives the DSL path; that refusal is the wiring
proof.

## 3. Lint parity, both directions

A workspace carrying the critic recipe lints clean:

```text
$ relurplint --check recipes --workspace ~/code/stringkit
relurplint: all checks passed (0 diagnostics)
```

And a D1 violation — a `revise` block with no `critique` before it — is
rejected by the linter with the loader's own error, at coordinates:

```text
relurpify_cfg/euclo/critic_broken.erpe: [error recipes.contract]
  …/critic_broken.erpe:11:3: directive "revise" requires directive
  "critique" in the critic paradigm contract
```

## 4. The pinned offline run

`relurpish chat --offline --workspace ~/code/stringkit`, with
`critic_review.erpe` materialized into the workspace, instruction:
*"explain and critique the slugify function in stringkit.py"*. The
deterministic route selects the critic recipe (telemetry route record):

```text
family_id:investigation  recipe_id:euclo.thoughtrecipe.critic_review
route_kind:capability    name:euclo.thoughtrecipe.critic_review
```

One HITL approval banner (`kind=execution | risk=medium`), approved for
the session (`[s]`), then the graph runs. The run completes with the
loop-close honesty guard firing — the same flag Chapter 1 met — because
the offline backend's scripted responses are not valid verdict JSON: the
critic's strict decode fails, the D6 retry fires exactly once, and the
failure protocol records the outcome. Telemetry carries the receipts
(`node euclo.execute_thoughtrecipe`, the route record naming
`critic_review`, `critic` as the executing paradigm).

The honest reading: **addressability is proven end to end** — load, lint,
route, approve, execute, record. Critique *quality* under the offline
backend is not; a scripted model that cannot emit verdict JSON cannot
pass strict decoding, and the receipts say so. Real-model behavior is
Chapter 12's subject, and the runner (`critic-runner`) remains the
deterministic behavioral proof.

`[NEEDS CONFIRMATION: recipe dispatch selection rules — the first
instruction ("critique …" with no investigation-family keyword) routed to
the default recipe; the dispatcher's exact keyword-vs-family mechanics
get pinned during Chapter 10 research.`
