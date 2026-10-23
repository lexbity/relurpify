# Chapter 6 — Parallel Boards and Retrieved Context

Two critics are better than one — *if* they are independent, *if* their
verdicts reach you without overwriting each other, and *if* you can say
exactly which critic said what. That is a concurrency problem with a
governance flavor, and the graph runtime has an opinion about how it
should go: branches run against immutable snapshots of the graph and
clones of the envelope, and their changes merge in edge-declaration order,
with conflicts recorded rather than rejected. This chapter you build the
ensemble, wire the retrieval and streaming seams, and — the milestone —
run the whole thing as a program. Part II ends with a working,
hand-composed critic pipeline in Go that you can execute from a shell.

## Where you are

Twenty tests green in `cognitionzoo/critic/`, all of them `_test.go`
files. This chapter changes that: you add the package's **first
production code** — `ensemble.go` and a small `cmd/critic-runner` command
— and the consumer gate that rejected your Chapter 2 layout finally gets
what it asked for: a package somebody imports.

## The parallel section, read from the source

When a node has parallel edges, `nextNodes` (`execution/agentgraph/
graph.go`) splits the outgoing edges into two lists. The comments on the
code state the contract precisely, and three sentences from it are worth
keeping verbatim in your head:

> Condition evaluation above already ran against the pre-branch parent
> state (predicates route, branches compute). Each branch executes on a
> private structural snapshot of the sealed graph and its own cloned
> envelope; results land in a pre-sized slice by edge index so collection
> order is never goroutine completion order.

And from the merge:

> Scan in edge-declaration order: the lowest-index error aborts before
> any merge, so failure handling is deterministic too.

The machinery: `snapshotGraph` freezes the sealed structure; each branch
gets `contextdata.CloneEnvelope`; branches run concurrently, each ending
at a terminal or a no-transition node; `ComputeBranchDelta` reduces each
branch to its change set relative to the fork-time parent; and
`ApplyBranchMerges` installs the deltas **in edge-declaration order**. On
a conflicting key, the branch later in declaration order wins; deletions
propagate; conflicts are recorded — in the `graph.branch_merged`
telemetry event, which carries `branches`, `keys_written`, `keys_skipped`,
`conflicted_keys`, and `winner_index` — not rejected.

One structural subtlety the source taught *me* while building this
chapter, and it will bite you too if you skip it: **a node whose outgoing
edges are all parallel ends the run.** The transition reason is literally
`"parallel-complete"` — the traversal returns after the merge. To continue
after a parallel section, the fork node needs one serial edge as well
(reason `"parallel-serial"`): the branches are the width, the serial edge
is the road onward. And a failed *intermediate* result does not stop the
run — `Success: false` without an error still traverses. Failures stop
runs by being errors; nodes that fail visibly (the retrieval node with no
retriever, which you will meet below) must be positioned where their
failure is the answer.

## Retrieval and streaming: the seams, read honestly

Two shipped node types connect a graph to the knowledge runtime.

**`RetrievalNode`** (`retrieval_node.go`) reads a query from a
working-memory key, runs it through a `*retrieval.Retriever` — the
scatter-gather ranker-fusion pipeline over the knowledge store — writes
the results and the ranked chunk IDs to working memory, and adds a
`RetrievalReference` to the envelope so provenance survives. Its contract
is `SideEffectNone` / `replay-safe` (retrieval reads; it does not write
the world), with a declared context boundary over `retrieval.*` keys.
Wiring a real retriever means wiring the knowledge runtime — that arrives
with `paradigm.Deps` in Chapter 8 (`SearchEngine`, `IndexManager`). What
you can prove today is the node's honest failure mode: no retriever, no
panic — a failed result naming the cause:

```text
retriever not configured
```

**`StreamTriggerNode`** (constructed via `NewContextStreamNode`) requests
a compiler-triggered context update: it pulls a `contextstream.Trigger`
from the execution context, submits a budgeted query, and applies the
compiled slice back onto the envelope under the epoch guard you read
about in Chapter 3. Its contract is `SideEffectContext` /
`replay-safe`. Like the retriever, its wiring (the trigger in context)
belongs to the runtime composition; the constructor and contract are
readable now, and the node refuses loudly when the trigger is absent —
`no compiler trigger in context` — fail-visible, same taste.

The same is true of the **grounder**: `agentgraph.Grounder` is a
one-method interface (`Ground(ctx, items, …)` — `*knowledge.
GroundingService` satisfies it), wired with `SetGrounder`; a graph
without one runs without the epoch coordinator, and a recipe that
declares captures fails graph build instead of reaching execution. The
capability catalog you met in Chapter 4 completes the quartet of
graph-level wiring: telemetry, grounder, drain, catalog.

## Hands-on

Three files land this chapter, and one of them is a milestone.

### 1. `ensemble.go` — the first production code

`BuildEnsembleGraph(cfg EnsembleConfig)` wires the ensemble
(`cognitionzoo/critic/ensemble.go`):

```text
drafter ──▶ critic-0 ──▶ branch-done-0 ┐
   │     ──▶ critic-1 ──▶ branch-done-1 ┤ parallel; deltas merged
   │     ──▶ critic-2 ──▶ branch-done-2 ┘ in declaration order
   └─serial─▶ merge ──▶ gate ──(revise)──▶ revise ──▶ critics (again)
                          └─(finish)──▶ done
```

Design decisions worth stealing:

- **Each critic writes its own key** (`critic.verdict.0/1/2`), so the
  parallel merge *unions* instead of conflicting. Where you want every
  voice kept, give every voice its own key; conflicts and
  declaration-order winners are for when branches genuinely disagree
  about one fact.
- **The merge is conservative**: any critic flagging issues flags the
  ensemble. That is the critic thesis as a reduce function — independent
  verification only helps if one raised hand is enough to stop the line.
- **The revise node fans back out to the same parallel branches.** My
  first draft merged stale verdicts after a revision — the gate revised
  twice. The fix is structural: after revising, re-run the critics, then
  merge, then gate. A merge is only as honest as the freshness of what it
  merges. (The test caught this; the test suite in this book is not
  decoration.)
- Every phase is a model call — data, not action — so the graph needs no
  capability registry; the governed-invocation story of Chapter 4 applies
  the moment a node invokes one.

### 2. `cmd/critic-runner` — the first importer

A small `main` that builds the ensemble from scripted offline models —
the data-plane pattern the framework itself ships (`offline` and `tape`
backends) — wires a printing telemetry sink (`telemetry.Telemetry` is one
method: `Emit`), and executes:

```bash
go run ./cognitionzoo/critic/cmd/critic-runner
```

```text
telemetry graph_start          node=
telemetry node_start           node=drafter
telemetry node_finish          node=drafter success=true
telemetry graph_start          node=
telemetry node_start           node=critic-2
telemetry graph_start          node=
telemetry graph_start          node=
…  [three branches run — the nested graph_start events are the branch
    subgraphs; then graph.branch_merged, merge, gate, revise]
telemetry graph.branch_merged  node= branches=[critic-0 critic-1 critic-2] conflicted_keys=[] keys_deleted=0 keys_skipped=0 keys_written=6 refs_retrieval=0 refs_streamed=0 units_applied=3 winner_index=map[]
telemetry node_start           node=merge
…  [second re-review round, another branch_merged, then done; elided]

result:     success=true node=done
ensemble:   size=3 verdict=clean
revisions:  1
final draft: a revised draft with error handling
preflight:  0 issues, 0 placements
```

The transcript is in `labs/transcripts/ch06-ensemble-runner.md`, full
width. Read the telemetry the way an operator would: three nested branch
runs, one merge event naming its branches in declaration order, one
revision, a re-review, a clean verdict. This is Part II's promised
"telemetry on the table" — the same event stream your Chapter 1 turn
wrote, now produced by your own graph.

And note what did *not* happen: no gate went red. The consumer gate that
called your package dead code in Chapter 2 is satisfied by an honest
importer — a command that exists because a program that runs beats a
library that waits.

### 3. `ensemble_test.go` — the merge, proven

Three tests pin the semantics the chapter claims:

- **`TestEnsembleGraphRunsAndMerges`** — the exported builder, three
  critics, one flagging issues: union of verdict keys, conservative
  ensemble verdict, exactly one revision, re-review clean, checkpoint
  requested.
- **`TestParallelMergeResolvesConflictsByDeclarationOrder`** — two
  branches write one key; the later-declared branch wins; the run
  completes. Conflicts are data, not errors.
- **`TestRetrievalNodeFailsVisibly`** — the retrieval node with no
  retriever, standing alone so its failure is the final result.

```text
$ go test ./cognitionzoo/critic/... -count=1
ok  	codeburg.org/lexbit/relurpify/cognitionzoo/critic	0.060s
```

Twenty-three tests. `verify.sh gates ch06` runs the full tier — build,
vet, tests, `make lint-arch` — in a scratch clone with the production
package and its importer in place, and it is green.

## Gotchas

- **Parallel-only fork nodes end the run.** If your graph stops after a
  merge, you forgot the serial continuation edge. The transition reason
  in the telemetry (`parallel-complete` vs `parallel-serial`) will tell
  you.
- **Branches share the graph, not the envelope.** Writes to a *shared*
  key from multiple branches are conflicts resolved by declaration order;
  writes to branch-unique keys are unions. Design your keys before your
  branches.
- **Merges must be fresh.** If a loop revises work and then merges
  verdicts computed before the revision, the loop will thrash. Re-run the
  critics after the revise, as the ensemble does.
- **A failed intermediate result keeps the run going.** `Success: false`
  without an error traverses. If a phase's failure must stop the pipeline,
  return an error — or position the node so its failure is the final
  answer.
- **The runner's models are scripted, and that is the point.** An
  offline demo with a temperature knob is a coin flip with output
  formatting. Determinism is the demo.

## Summary

- Parallel branches run on immutable structural snapshots with cloned
  envelopes; deltas merge in edge-declaration order; the later branch
  wins conflicts; deletions propagate; conflicts land in the
  `graph.branch_merged` telemetry event. The lowest-index branch error
  aborts before any merge.
- A parallel section continues through a serial edge off the fork node —
  parallel-only edges end the traversal. Failed intermediate results
  traverse; errors stop runs.
- `RetrievalNode` and `StreamTriggerNode` are the knowledge-runtime
  seams, both fail-visible when unwired, both carrying typed contracts;
  their wiring arrives with `paradigm.Deps` in Chapter 8.
- The critic ensemble — N independent critiques, per-branch verdict keys,
  a conservative merge, a revision that re-runs the critics before
  re-merging — now exists as production code with its own runner
  command, and the consumer gate is satisfied honestly.
- **End of Part II:** a working, hand-composed critic pipeline in Go,
  runnable from a shell, telemetry on the table — not yet
  DSL-addressable. The gap between this and `agent fixer uses critic` is
  Part III's subject: contracts, registration, and goldens.

## Check yourself

1. Two parallel branches write different values to `critic.verdict`. What
   does the parent hold after the merge, where is the conflict recorded,
   and how would you have designed the keys so there was no conflict at
   all?
2. Your fork node has three parallel edges and no serial edge. What does
   the run do after the merge, and what one-line change continues it?
3. A branch's node returns `&Result{Success: false, Error: ""}`. Does the
   run stop? What would stop it?
4. Why does the ensemble's revise node fan back out to the critics
   instead of going straight to the gate?
5. `telemetry.Telemetry` is one method. Why does the graph's emit path
   work with such a narrow interface, and what did the same taste buy the
   `model` domain in Chapter 2?

*Answers.* 1. The later-declared branch's value; in the
`graph.branch_merged` event (`conflicted_keys`, `winner_index`); give
each branch its own key (`critic.verdict.<i>`) so the merge unions. 2.
It ends — transition reason `parallel-complete`; add one serial edge off
the fork node (`parallel-serial`). 3. No — `Success: false` without an
error traverses; a non-nil error stops the run (wrapped as
`node <id> execution failed: …`). 4. Because the merge would otherwise
work from stale verdicts — the gate would revise again against critiques
of the old draft; the re-fan-out makes the second merge as fresh as the
first. 5. The graph emits complete `telemetry.Event` values and stamps
correlation itself, so any sink — a printer, the OTel leg, a test sink —
plugs in without importing the telemetry domain's guts; the same
consumer-defined-port taste is why `model.EventSink` keeps the model
domain a DAG leaf.

## Try it yourself

1. **Widen the ensemble, watch the merge.** Add a fourth critic whose
   first turn flags a *different* issue and whose second passes. Predict
   the `graph.branch_merged` metadata (`units_applied`, `keys_written`)
   before running the runner, then check. Prediction is how you prove to
   yourself you understand declaration order.
2. **A deletion race.** Write a two-branch test where branch zero *adds*
   a key and branch one *deletes* it. Run it, read which value survives,
   and find the rule in the package doc
   (`execution/agentgraph/doc.go`). Deletion propagation is the merge's
   sharpest edge — meet it in a test before it meets you in production.

## Where next

Part III begins where Chapter 4 ended: the graph works, but no recipe can
say so. Chapter 7 reads a shipped paradigm contract end to end —
reflection, the review loop — then puts you in front of the contract
system that makes the DSL data-driven: `Contract`, `DirectiveSpec`,
`OrderRule`, and the closed set of execution shapes. You will design the
critic's grammar as data, and the honest conversation about which
`ExecutionShape` it deserves begins there.

The lab lives at `devdocs/agent-construction-relurpify/labs/` —
`verify.sh gates ch06` re-runs the full tier, and
`transcripts/ch06-ensemble-runner.md` holds the runner's telemetry
transcript in full.
