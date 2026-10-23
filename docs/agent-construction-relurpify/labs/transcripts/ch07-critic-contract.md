# Chapter 7 — critic contract transcripts (captured)

Real output from the verification machine (same environment as the earlier
transcripts). The Chapter 7 overlay (`ch07-critic-contract`) adds the
critic's paradigm contract as production code
(`cognitionzoo/critic/contract.go`, self-registering from `init`) plus its
tests (`contract_test.go`).

## Package tests (ch02–ch07 overlays)

```text
$ go test ./cognitionzoo/critic/... -count=1 -v        (30 tests)
--- PASS: TestCriticContractRegistered (0.00s)
--- PASS: TestContractCompleteness (0.00s)
--- PASS: TestRegistrationValidatesIntrinsics
    --- PASS: …/unknown_shape (0.00s)
    --- PASS: …/duplicate_directive (0.00s)
    --- PASS: …/requires_undeclared_directive (0.00s)
    --- PASS: …/order_rule_references_undeclared_directive (0.00s)
    --- PASS: …/case_pins_undeclared_directive (0.00s)
    --- PASS: …/declared_directive_without_a_case (0.00s)
    --- PASS: …/duplicate_registration (0.00s)
--- PASS: (…the twenty-three Chapter 2–6 tests…)
PASS
ok  	codeburg.org/lexbit/relurpify/cognitionzoo/critic	0.061s
```

- `TestCriticContractRegistered` — the process-wide registry holds a
  `critic` contract: `ShapeReviewLoop`, `Composes [react]`, directives
  exactly `[critique revise]` in declaration order, `revise` a predicate
  block requiring `critique`, an order rule present, no required
  directives (goal-only recipes preserved).
- `TestContractCompleteness` — the mirror of every shipped paradigm's own
  completeness test: every declared directive pinned by a conformance
  case, no case pinning an undeclared directive, and `react` (the
  Composes entry) resolved in the fully-populated registry.
- `TestRegistrationValidatesIntrinsics` — a scoped registry driven through
  the seven intrinsic refusals, each captured verbatim from the validator:
  `unknown execution shape "Whirlpool"`; `duplicate directive "critique"`;
  `directive "revise" requires undeclared directive "approve"`;
  `order rule references undeclared directive "polish"`;
  `conformance case "lab/ghost" pins undeclared directive "polish"`;
  `directive "revise" has no conformance case`; and
  `duplicate paradigm contract registration for "critic-lab"`.

## The registry now carries nine paradigms

The critic package's `init` registers the contract in any binary that
links it — including the runner:

```text
$ go run ./cognitionzoo/critic/cmd/critic-runner
result:     success=true node=done
ensemble:   size=3 verdict=clean
revisions:  1
final draft: a revised draft with error handling
preflight:  0 issues, 0 placements
```

The runner's process now carries the `critic` contract alongside the eight
shipped ones — visible to any recipe validation in that process. What it
cannot do yet is load a recipe: the runtime composition, lint parity, and
goldens are Chapter 9's work.

## Gates

```text
$ ./verify.sh gates ch07
== tier 2 (gates): ch07 ==
… (build, vet, tests, make lint-arch — all PASS)
[PASS] gates: ch07 holds the framework's law
```
