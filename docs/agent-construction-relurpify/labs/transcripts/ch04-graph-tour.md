# Chapter 4 — graph tour transcripts (captured)

Real output from the verification machine (same environment as the earlier
transcripts). The Chapter 4 overlay (`ch04-first-graph`) adds
`cognitionzoo/critic/graph_tour_test.go` — six tests covering build, seal,
conditions, the visit cap, validation, and preflight — on top of the
Chapter 2–3 package.

## Full critic package test run (ch02+ch03+ch04 overlays)

```text
$ go test ./cognitionzoo/critic/... -count=1 -v
--- PASS: TestEnvelopeTour (0.00s)
--- PASS: TestFakeModelIsALanguageModel (0.00s)
--- PASS: TestGenerateStreamEmitsScriptedChunksInOrder (0.00s)
--- PASS: TestGenerateReturnsJoinedText (0.00s)
--- PASS: TestChatWithToolsReturnsScriptedToolCalls (0.00s)
--- PASS: TestTurnsCycleInOrder (0.00s)
--- PASS: TestGenerateStreamClosesOnCancellation (0.03s)
--- PASS: TestGenerateStreamDropsAndCountsOnSendContention (0.03s)
--- PASS: TestErrorInjectionFailsEveryMethod (0.00s)
--- PASS: TestEmptyScriptFailsCleanly (0.00s)
--- PASS: TestFirstGraphRuns (0.00s)
--- PASS: TestConditionalEdgesRoutes (0.00s)
--- PASS: TestGraphSealsOnExecute (0.00s)
--- PASS: TestVisitCapCatchesCycle (0.00s)
--- PASS: TestValidationCatchesBrokenWiring (0.00s)
--- PASS: TestPreflightGatesToolPlacement (0.00s)
PASS
ok  	codeburg.org/lexbit/relurpify/cognitionzoo/critic	0.059s
```

The six Chapter 4 tests, and what each asserts:

- `TestFirstGraphRuns` — intake (SystemNode) → drafter (hand-implemented
  `Node`, driven by `FakeModel`) → done (TerminalNode). Asserts the run's
  result, the node-written envelope value (`critic.draft`), the
  runtime-captured result fields (`drafter.draft`), one model call, and an
  empty preflight report when no catalog is wired.
- `TestConditionalEdgesRoutes` — a `ConditionFunc` on
  `drafter → polish` routes on the draft's content; the complementary
  edge to `done` is not taken.
- `TestGraphSealsOnExecute` — after `Execute`, `AddNode` and `SetStart`
  return `ErrGraphSealed` (asserted with `errors.Is`), and a second
  `Execute` still succeeds: sealing governs mutation, not execution.
- `TestVisitCapCatchesCycle` — `a → b → a` with
  `SetMaxNodeVisits(2)`: `Execute` fails with
  `potential cycle detected at node a`.
- `TestValidationCatchesBrokenWiring` — `SetStart("missing")` →
  `"start node missing not found"`; `AddEdge("done", "ghost", …)` →
  `"node ghost not defined"`; `Execute` without a start →
  `"graph has no start node"`.
- `TestPreflightGatesToolPlacement` — a `ToolNode` whose tool descriptor
  is `tool:read_stringkit`, against three states of the **real**
  `capability/registry.CapabilityRegistry` (the framework's own unit-test
  pattern: `NewRegistry` + `RegisterLegacyTool`): not wired as catalog
  (report has no opinions), empty (`capability_missing`, blocking —
  `Execute` refuses), and with the tool registered (one placement,
  `SelectedCapabilityID tool:read_stringkit`, with its reason; `Execute`
  runs and the test asserts the invocation landed in the tool through the
  governed pipeline — `tool.calls == 1`). Invocations go through a
  `governedInvoker` adapter that forwards to the registry; there is no
  fake invoker in the book's code.

## The verifier

```text
$ ./verify.sh build ch04
== tier 1 (build): ch04 ==
ok  	codeburg.org/lexbit/relurpify/cognitionzoo/critic	0.059s
[PASS] build: ch04 compiles clean and its tests pass
$ ./verify.sh gates ch04
== tier 2 (gates): ch04 ==
... (build, vet, tests, make lint-arch — all PASS)
[PASS] gates: ch04 holds the framework's law
```
