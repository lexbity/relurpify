# Chapter 6 — ensemble & runner transcripts (captured)

Real output from the verification machine (same environment as the earlier
transcripts). The Chapter 6 overlay (`ch06-ensemble-runner`) adds the
package's first production code — `cognitionzoo/critic/ensemble.go` and
`cognitionzoo/critic/cmd/critic-runner/main.go` — plus `ensemble_test.go`.

## The critic-runner, offline end to end

```text
$ go run ./cognitionzoo/critic/cmd/critic-runner
telemetry graph_start          node=
telemetry node_start           node=drafter
telemetry node_finish          node=drafter success=true
telemetry graph_start          node=
telemetry node_start           node=critic-2
telemetry graph_start          node=
telemetry graph_start          node=
telemetry node_finish          node=critic-2 success=true
telemetry node_start           node=branch-done-2
telemetry node_finish          node=branch-done-2 success=true
telemetry graph_finish         node= status=success
telemetry node_start           node=critic-0
telemetry node_finish           node=critic-0 success=true
telemetry node_start           node=branch-done-0
telemetry node_finish          node=branch-done-0 success=true
telemetry graph_finish         node= status=success
telemetry node_start           node=critic-1
telemetry node_finish          node=critic-1 success=true
telemetry node_start           node=branch-done-1
telemetry node_finish          node=branch-done-1 success=true
telemetry graph_finish         node= status=success
telemetry graph.branch_merged  node= branches=[critic-0 critic-1 critic-2] conflicted_keys=[] keys_deleted=0 keys_skipped=0 keys_written=6 refs_retrieval=0 refs_streamed=0 units_applied=3 winner_index=map[]
telemetry node_start           node=merge
telemetry node_finish          node=merge success=true
telemetry node_start           node=gate
telemetry node_finish          node=gate success=true
telemetry node_start           node=revise
telemetry node_finish          node=revise success=true
telemetry graph_start          node=
telemetry node_start           node=critic-2
…  [second re-review round: the three critics' second scripted turns,
    another graph.branch_merged, merge, gate — then done; elided]
telemetry graph_finish         node= status=success

result:     success=true node=done
ensemble:   size=3 verdict=clean
revisions:  1
final draft: a revised draft with error handling
preflight:  0 issues, 0 placements
```

Read the transcript: the nested `graph_start` events are the three
parallel branches (each branch runs against its own structural snapshot
and cloned envelope); `graph.branch_merged` reports the merge in edge
declaration order (`branches=[critic-0 critic-1 critic-2]`,
`units_applied=3`, no conflicts, six keys written — three verdicts per
round); the revise node fired once and the re-review passed clean.

## Package tests (ch02–ch06 overlays)

```text
$ go test ./cognitionzoo/critic/... -count=1 -v        (23 tests)
--- PASS: TestEnsembleGraphRunsAndMerges (0.00s)
--- PASS: TestParallelMergeResolvesConflictsByDeclarationOrder (0.00s)
--- PASS: TestRetrievalNodeFailsVisibly (0.00s)
--- PASS: (…the twenty Chapter 2–5 tests…)
PASS
ok  	codeburg.org/lexbit/relurpify/cognitionzoo/critic	0.060s
```

- `TestEnsembleGraphRunsAndMerges` — the exported builder, three critics,
  one flagging issues: the merge unions all three verdict keys, the
  ensemble is conservative, exactly one revision fires, the re-review
  passes clean, the checkpoint is requested. The final verdict keys hold
  the *re-review* values — the graph never merges stale verdicts, because
  the revise node fans back out to the same parallel branches.
- `TestParallelMergeResolvesConflictsByDeclarationOrder` — two branches
  write the same key with different values; the later-declared branch
  wins; the conflict is recorded (in the `graph.branch_merged` telemetry),
  not rejected.
- `TestRetrievalNodeFailsVisibly` — `NewRetrievalNode` with no retriever
  yields a failed result with `retriever not configured`. The node stands
  alone in the graph, because a failed *intermediate* result does not
  stop a run — the failure is the final result only when nothing follows.

## The consumer gate, satisfied honestly

This is the chapter where `cognitionzoo/critic` gains production files and
its first importer: `cmd/critic-runner` (a `main` package) imports
`critic.BuildEnsembleGraph`. `make lint-arch` in the scratch clone passes
with the production package present — no exemptions, no allowlist edits.

```text
$ ./verify.sh gates ch06
== tier 2 (gates): ch06 ==
… (build, vet, tests, make lint-arch — all PASS)
[PASS] gates: ch06 holds the framework's law
```
