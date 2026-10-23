# Chapter 5 — critic graph transcripts (captured)

Real output from the verification machine (same environment as the earlier
transcripts). The Chapter 5 overlay (`ch05-critic-graph`) adds
`cognitionzoo/critic/critic_graph_test.go` — the critic pipeline as a
hand-composed graph, plus contract-default and contract-validation tests —
on top of the Chapter 2–4 package.

## Full critic package test run (ch02–ch05 overlays)

```text
$ go test ./cognitionzoo/critic/... -count=1 -v        (20 tests)
--- PASS: TestCriticPipelineRevisesWhenFlagged (0.00s)
--- PASS: TestCriticPipelineDenialSkipsRevision (0.00s)
--- PASS: TestContractDefaultsAreTyped (0.00s)
--- PASS: TestContractValidationRefusesGarbage
    --- PASS: …/invalid_side_effect_class (0.00s)
    --- PASS: …/tool_node_must_declare_capabilities (0.00s)
--- PASS: TestEnvelopeTour
--- PASS: (…the nine Chapter 2 fake-model tests…)
--- PASS: (…the six Chapter 4 graph-tour tests…)
PASS
ok  	codeburg.org/lexbit/relurpify/cognitionzoo/critic	0.059s
```

The four Chapter 4→5 additions, and what each asserts:

- `TestCriticPipelineRevisesWhenFlagged` — the full pipeline:
  drafter → critic → gate → approve (human pause, scripted "approved") →
  revise → re-critic → done. Asserts the final verdict is `clean`, the
  revision counter is exactly 1, the final draft is the revision, the
  critic model ran twice (initial critique + re-review), the revise model
  once, the approval decision is recorded in the envelope
  (`approval.revise`, `OriginUser`), and the revise node requested a
  checkpoint (`env.CheckpointRequestSnapshot() != nil`).
- `TestCriticPipelineDenialSkipsRevision` — same graph, scripted denial:
  the revise model is never called, the revision counter never moves, and
  the run ends at the terminal.
- `TestContractDefaultsAreTyped` — `ResolveNodeContract` on shipped node
  values: terminal → `none`/`replay-safe`; HumanNode → `human`/
  `single-shot`; ToolNode over the read-only stringkit tool → `none`/
  `replay-safe` **because the tool declares no effect classes** — the
  contract is classified from what the tool declares — with
  `RequiredCapabilities[0].ID == "tool:read_stringkit"` pinned.
- `TestContractValidationRefusesGarbage` — a node declaring
  `SideEffectClass("wing-it")` fails `Graph.Validate` with
  `invalid side effect class "wing-it"`; a tool-typed node whose contract
  declares no capabilities fails with `declares no required capabilities`.

## Honest notes

- `ConditionalNode` and `HumanNode` carry unexported `id` fields and ship
  no constructors as of this tree, so they cannot be validly constructed
  outside the `agentgraph` package. The lab mirrors HumanNode's contract
  on a hand-implemented `approvalGate` node (the builder's legitimate
  move), and reads HumanNode's *default contract* through its zero value
  via `ResolveNodeContract`. The gate node in the pipeline is a
  `SystemNode` with ConditionalNode semantics (decision in result fields,
  edges route on it).
  `[NEEDS CONFIRMATION: constructors for ConditionalNode/HumanNode at
  BETA-RC1-FALL2026]`
- The revise node requests a checkpoint on the envelope, mirroring the
  shipped ReWOO checkpoint node (`env.RequestCheckpoint("rewoo:<phase>",
  …)`); materialization belongs to the persistence wiring, which arrives
  with runtime composition.
