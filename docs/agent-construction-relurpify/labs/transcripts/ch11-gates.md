# Chapter 11 — gates transcripts (captured)

Real output from the verification machine (same environment as the earlier
transcripts). The Chapter 11 overlay (`ch11-gates`) adds: the critic's
conformance runners + fixtures (`testsuite/conformance/`), the regenerated
`paradigm_matrix.yaml`, the streamed-context matrix entry, the legacy
paradigm parity vectors (`named/euclo/surface/paradigm_test.go`), the
coverage override for the runner main (`tooling/coverage/overrides.yaml`),
and the two embedded-template sources (the recipe and the prompt — ADR-002).
It also carries the D-2 fix to the agent itself
(`cognitionzoo/critic/critic.go` — the critique phase now renders the
compiled slice, mirroring reflection).

## 1. The gauntlet, all green

```text
$ ./verify.sh gates ch11        # build, vet, 34 critic tests, then:
lint-arch:                  [PASS]
check-gates-honest:         [PASS]
domain-check:               [PASS]
grep-architecture-gates:    [PASS]
test-euclo-golden:          [PASS]
test-conformance:           [PASS]

$ make test-coverage
  [PASS] codeburg.org/lexbit/relurpify/cognitionzoo/critic           78.28% (floor  40; 209/267 stmts)
  [PASS] codeburg.org/lexbit/relurpify/cognitionzoo/critic/cmd/critic-runner   0.00% (floor   0; 0/54 stmts)
[PASS] coverage: all 151 packages at or above their floors (default floor 40%)
```

## 2. The failures the gauntlet taught (all real, all fixed honestly)

### Conformance: declared cases with no runners

The harness is registry-driven; the critic's four declared cases failed on
arrival:

```text
--- FAIL: TestParadigmContractConformance
    conformance case "critic/critique_writes_scratch" has no runner
    (contract declared, runtime drift)
```

Fixed by the runners file (`critic_conformance_test.go`, registering four
runners that mirror reflection's line for line) plus two fixtures
(`critic_critique.erpe`, `critic_revise.erpe`) and — after a real failure
— a third (`critic_pass.erpe`, no capture line): the pass-case first
failed because the shared fixture's `capture revisions` bound the result
field `revisions: 0` (present). Reflection ships separate pass/revise
fixtures for exactly this reason; the mirror now does too.

### The matrix, generated not hand-written

```text
--- FAIL: TestParadigmMatrixUpToDate
    paradigm matrix drifted from the contract registry:
    (set UPDATE_MATRIX=1 to regenerate)
--- FAIL: TestParadigmMatrixInvariants
    declared case critic/critic/critique_writes_scratch has no status annotation
```

`UPDATE_MATRIX=1` regenerates the committed matrix from the registry
(same discipline as the goldens); the status annotations
(`implemented`) seed the generator's table. The regenerated matrix's
critic rows:

```yaml
    - paradigm: critic
      directive: critique
      status: implemented
      cases:
        - critic/critique_writes_scratch
        - critic/critique_pass_ends_loop
    - paradigm: critic
      directive: revise
      status: implemented
      cases:
        - critic/revise_fires_on_predicate
        - critic/revision_cap_bounded
```

### The stream matrix caught a real integration gap

```text
--- FAIL: TestParadigmStreamedContextMatrix
    paradigm "critic" is registered but has no streamed-context matrix
    runner (paradigm_not_integrated)
```

Running the sentinel proved the deeper fact: the Chapter 8 critic never
rendered the compiled slice into its critique prompt — reflection does
(`paradigm.StreamedSection`, the D-2 seam). **The fix is in the agent, not
the test**: `runCritique` now renders the streamed section into its system
slot exactly as reflection's review does, and the matrix runner
(`streamMatrixCritic`) pins the sentinel. All 34 package tests still pass
with the integration.

### The surface parity vectors

```text
--- FAIL: TestAllParadigms_MatchesLegacyValidateList
    AllParadigms() returned 10 paradigms; want 9
--- FAIL: TestAgentParadigms_MatchesLegacyAgentList
    AgentParadigms() returned 9 paradigms; want 8
```

The surface vocabulary (Chapter 9's `ParadigmCritic`) grew the canonical
lists; the legacy parity vectors grow with them — `critic` joins both,
deliberately, in the forked test file.

### The config-tree drift gate (ADR-002)

```text
--- FAIL: TestDriftGatePassesWhenInSync
    Only in relurpify_cfg/euclo: critic_review.erpe
    Only in relurpify_cfg/prompts/agents: agent.critic.default.prompt
```

The checked-in `relurpify_cfg/` must be the materialization of the
embedded template bundle. Fix per ADR-002: add the recipe and prompt as
**template sources** (`userconfig/templates/embedfs/…`) and regenerate:

```text
$ make generate-config
$ make check-config-tree-drift
[PASS] relurpify_cfg/ matches embedded templates
```

`git status relurpify_cfg/` after regeneration shows exactly the two new
files — nothing else drifted.

### The coverage floor (D9, verbatim)

The runner main arrived at 0.0%:

```text
	codeburg.org/lexbit/relurpify/cognitionzoo/critic/cmd/critic-runner
		coverage: 0.0% of statements
```

The house pattern for mains is an explicit floor entry with a reason
(`floor: 0; reason: …; ratchets up as tests land`), so the overlay's
`overrides.yaml` fork carries one for `critic-runner`. The package itself
— written test-first since Chapter 2 — lands at **78.28%** against the
enforced floor of 40 (the Makefile's global default; `overrides.yaml` is
the per-package ratchet) and above the project's 70% aspiration.
