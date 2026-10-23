# Chapter 4 — Your First Graph

You own a model now — a scripted one that always says what it means. But a
model alone is not an agent; it is a component. In this framework, an agent
is a **graph**: typed nodes, wired by edges, executed as a deterministic
state machine while a single envelope threads through. This chapter you
build one with your own hands — implement the node interface before you
meet the shipped nodes, build, seal, execute, break the wiring on purpose,
and read your first preflight report.

By the end you will have run the framework's own machinery over stringkit,
driven by your fake model, inside a plain Go test — no registry, no
sandbox, no model backend. That test is the seed of the critic pipeline:
Part II's remaining chapters grow exactly this file.

## Where you are

Chapters 1–3 left a green baseline and a `cognitionzoo/critic/` package
whose ten tests pass in under sixty milliseconds: the fake model and its
contract tests, plus the envelope tour. This chapter adds one more test
file — `graph_tour_test.go` — and takes you inside
`execution/agentgraph`, the package whose doc opens: *"a deterministic
state-machine workflow runtime for Relurpify agents."*

## Technical requirements

Nothing new. Go, the Chapter 1 checkout, the labs folder. The graph
runtime is pure Go; nothing here touches a network, a sandbox, or Python.

## The Node interface is three methods

Everything in a graph — every shipped node and every node you will ever
write — implements this (`execution/agentgraph/graph.go:42`):

```go
// Node describes the unit of work executed inside a graph.
type Node interface {
	ID() string
	Type() NodeType
	Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error)
}
```

Read it as a bargain. `ID` gives the graph a name to wire edges with.
`Type` declares what kind of thing you are — the node types are a closed
set: `tool`, `conditional`, `human`, `terminal`, `system`, `observation`,
`stream`. `Execute` receives the context (already stamped with the task
and node identity — the runtime does that before calling you) and the
envelope, and returns a `Result` plus an error. You implemented `Result`
in Chapter 3; you implemented the envelope tour in the same chapter. There
is nothing else to learn to write a node.

The book's first node is a drafter: it asks the fake model for a draft,
writes the draft into working memory with its class and origin, and
returns the draft as a result field:

```go
type draftNode struct {
	id    string
	model *critic.FakeModel
}

func (n *draftNode) ID() string                { return n.id }
func (n *draftNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeObservation }

func (n *draftNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	ch, err := n.model.GenerateStream(ctx, "draft a plan against stringkit", nil)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	for chunk := range ch {
		b.WriteString(chunk)
	}
	draft := b.String()
	env.SetWorkingValueWithOrigin("critic.draft", draft,
		contextdata.MemoryClassTask, contextdata.OriginLLM)
	return &execution.Result{
		NodeID:  n.id,
		Success: true,
		Data:    execution.NewToolResultPayload(map[string]any{"draft": draft}),
	}, nil
}
```

Why `NodeTypeObservation` for a model-backed node? Because that is the
shipped convention: the react paradigm's think node — the one whose
telemetry you watched in Chapter 1 — declares the same type. A model's
output is an observation over the envelope, not a side effect.

Note what the node does *not* do: it does not know about edges, does not
choose its successor, does not touch the graph. Routing is the graph's
job. A node computes; the graph decides.

## Building the machine

Four build-phase calls, in the order you will always use them:

```go
g := agentgraph.NewGraph()
g.AddNode(intake)                            // any Node
g.AddEdge("intake", "drafter", nil, false)   // from, to, ConditionFunc, parallel
g.SetStart("intake")
result, err := g.Execute(ctx, env)
```

`NewGraph` installs sane defaults, including a **visit cap of 1,024** —
the cycle guard, bumpable with `SetMaxNodeVisits`. `AddEdge`'s last two
arguments are this chapter's vocabulary: a `ConditionFunc` (evaluated
against the result and the envelope; a nil condition is always true) and
the `parallel` flag (Chapter 6's subject — for now, serial edges make a
classic state machine: one node runs, one successor is chosen).

Two behaviors are enforced at build time, and both are structural
refusals rather than silent misbehavior:

```text
$ # SetStart("missing")            → "start node missing not found"
$ # AddEdge("done", "ghost", …)    → "node ghost not defined"
$ # Execute with no SetStart       → "graph has no start node"
```

The lab runs all three; the test names say what they prove
(`TestValidationCatchesBrokenWiring`). A graph that cannot be wired
correctly refuses to be built — the same fail-loudly taste as
`NewToolNode`, which *panics* on a nil registry rather than pretending a
tool can be invoked ungoverned.

### Sealing: the two-phase lifecycle

The first `Execute` seals the graph. After that, every build-phase
mutator — `AddNode`, `AddEdge`, `SetStart`, `SetMaxNodeVisits`,
`SetTelemetry`, `SetGrounder`, `SetCapabilityCatalog` — returns one
shared error:

```go
// ErrGraphSealed is returned by Graph mutators once Execute has begun…
// Mid-run structure materialization is deliberately unsupported; a mutator
// on a sealed graph fails loudly here instead of racing the run loop.
var ErrGraphSealed = errors.New("agentgraph: graph is sealed (execution in progress or complete)")
```

The doc comment carries the *why*: a mutator during a run would race the
execution loop reading those maps, and the framework prefers a loud error
over a lock dance. Note the asymmetry the comment also states: **sealing
governs mutation, not execution.** `Execute` may be called again on a
sealed graph — it resets visit counts and re-runs the frozen structure.
That is what makes a graph a reusable artifact rather than a one-shot
script, and it is why the chapter's sealing test asserts both halves.

### The execution path

`Execute` does three things before your first node runs: it seals, it
**validates** (no nodes, no start, edges referencing missing nodes — the
errors above), and it runs **preflight**. Then the run loop walks: pick
the start node, count the visit, emit a `node_start` telemetry event,
build the node's context, call `Execute`, wrap any error as
`node <id> execution failed: …`, emit `node_finish`, and follow the edges.

Two facts in that loop are load-bearing for everything you build later:

**Result fields land in working memory automatically.** After a node
succeeds, the runtime writes every result-payload field into the envelope
as `"<nodeID>.<key>"`:

```go
for key, value := range execution.ResultFields(result.Data) {
	env.SetWorkingValueWithClass(fmt.Sprintf("%s.%s", current, key), value, contextdata.MemoryClassTask)
}
```

Your drafter returns `{"draft": …}` from node `drafter`; the graph writes
`drafter.draft` into working memory. Downstream nodes read it with
`GetTyped`. This is the framework's native dataflow — nodes communicate
through the envelope, and the graph does the plumbing. (Your own write of
`critic.draft` in the same node shows the other path: a node may also
write envelope state directly, with full class-and-origin provenance.)

**A terminal — or a node with no outgoing edges — ends the run**, and the
last result is returned. `TerminalNode` is one field and a type
declaration; its whole job is to be a named end.

### Preflight: the report before the run

After validation, `Execute` calls `Preflight()` and refuses to run past a
**blocking** issue. The report (`execution/agentgraph/preflight.go`) is a
small, serializable document:

```go
type PreflightReport struct {
	GeneratedAt time.Time
	Issues      []PreflightIssue    // NodeID, Code, Message, Blocking
	Placements  []PlacementDecision // which capability serves which node, and why
}
```

Preflight answers one question: *can this graph's capability needs be
satisfied at all?* Every node's contract may declare
`RequiredCapabilities` — selectors the capability catalog must match —
and preflight tries each selector against the wired catalog, checking
trust-class floors and risk ceilings on the way. A match becomes a
`PlacementDecision` with its `Reason` ("selected highest-trust lowest-risk
matching capability"); a miss becomes a blocking `capability_missing`
issue.

Whether preflight has opinions depends on what you wire:

- **No catalog** — nothing to check; an empty report. Honest, not lazy:
  without a catalog there is no placement question to answer.
- **An empty catalog** — a `ToolNode` fails hard, because a `ToolNode`
  *carries its own contract* (Chapter 5's subject — the five governance
  classes you met in Chapter 1), and that contract requires exactly the
  capability the tool names:

  ```text
  graph preflight failed: no registered capability matches
  selector tool:read_stringkit (reader)
  ```

- **A matching catalog** — one placement, decided and explained, and the
  run proceeds.

Read the selector string closely: `tool:read_stringkit` — the descriptor
ID derived from the tool's name. The lab builds all three cases against
the **real capability registry** — `registry.NewRegistry()` plus
`RegisterLegacyTool`, the framework's own unit-test pattern — used as both
catalog and invoker, and `Execute` itself refuses to run past a blocking
preflight. The gate-before-the-gates, inside the runtime.

## Hands-on

The chapter overlay is
`labs/testdata/overlay/ch04-first-graph/cognitionzoo/critic/graph_tour_test.go`
— six tests, one file, everything above made executable. Run it:

```bash
./verify.sh build ch04
```

```text
== tier 1 (build): ch04 ==
ok  	codeburg.org/lexbit/relurpify/cognitionzoo/critic	0.059s
[PASS] build: ch04 compiles clean and its tests pass
```

Sixteen tests now pass in the package — nine from Chapters 2–3, six from
this chapter, plus the envelope tour. The six new ones, and the beat each
plays:

1. **`TestFirstGraphRuns`** — the full tour: a `SystemNode` (the escape
   hatch for control-flow that is neither model- nor tool-backed — here it
   injects a synthetic intake message with `OriginTool` provenance) → your
   hand-written drafter (driven by `FakeModel`, one model call) →
   `TerminalNode`. Asserts the result, both envelope paths (node-written
   `critic.draft`, runtime-captured `drafter.draft`), and the empty
   preflight report.
2. **`TestConditionalEdgesRoutes`** — two serial edges out of the drafter
   with complementary `ConditionFunc`s; the draft contains "slugify", so
   the run routes through the polish node and never takes the direct exit.
3. **`TestGraphSealsOnExecute`** — post-seal `AddNode` and `SetStart`
   return `ErrGraphSealed` (asserted with `errors.Is`, not string
   matching); a second `Execute` succeeds.
4. **`TestVisitCapCatchesCycle`** — `a → b → a` with the cap set to 2:

   ```text
   potential cycle detected at node a
   ```

   The cap counts per traversal, so a bounded accidental loop dies in
   bounded time instead of hanging a turn.
5. **`TestValidationCatchesBrokenWiring`** — the three structural
   refusals, table-driven.
6. **`TestPreflightGatesToolPlacement`** — a `ToolNode` over a
   hand-written `ports.Tool` (eight small methods; its `Execute` returns
   the scripted stringkit source and counts its own executions), invoked
   through a governed adapter over the **real** capability registry. Three
   registry states, three outcomes:
   no opinions, blocking `capability_missing`, and a decided placement
   with its reason.

```text
--- PASS: TestFirstGraphRuns (0.00s)
--- PASS: TestConditionalEdgesRoutes (0.00s)
--- PASS: TestGraphSealsOnExecute (0.00s)
--- PASS: TestVisitCapCatchesCycle (0.00s)
--- PASS: TestValidationCatchesBrokenWiring (0.00s)
--- PASS: TestPreflightGatesToolPlacement (0.00s)
PASS
ok  	codeburg.org/lexbit/relurpify/cognitionzoo/critic	0.059s
```

One honesty note, because it is a rule and not a preference: **the book
never routes an action around governance — not even in a test.** The tool
in this chapter is scripted (a test double for *what* the tool returns),
but the invocation path is real: a tiny `governedInvoker` adapter forwards
every call to a real `capability/registry.CapabilityRegistry` — the same
admission → policy → schema-validation → insertion pipeline your
Chapter 1 turn went through — and the test asserts the execution landed
in the tool *through* it (`tool.calls == 1`). What may be faked in this
book is the model, because a model's output is data; what may never be
faked is the capability path, because a model's output is the only thing
that is not an action. Chapter 6 stands up the registry with policy and
approval wiring; Chapter 8's critic will refuse to run without its
permission checker, exactly as ReWOO does.

## Gotchas

- **Sealing is one-way for structure, not for runs.** After `Execute`,
  every mutator returns `ErrGraphSealed` — but `Execute` itself may be
  called again, with visit counts reset. Design your graphs as reusable
  state machines, not scripts.
- **`AddNode` rejects duplicate IDs** ("node %s already exists") — node
  identity is the graph's only addressing, so it is protected at build
  time.
- **Condition functions see the *node's* result and the *live*
  envelope.** A condition that writes to the envelope is a bug: conditions
  route, branches compute (the runtime evaluates all conditions against
  the pre-branch state before any fork).
- **The visit cap is per traversal, not a work budget.** A node visited
  once by the parent and once per parallel branch counts width+1. The cap
  exists to catch accidental cycles — it is not a complexity dial.
- **`NewToolNode` panics on a nil registry** — deliberately. A tool node
  that could execute ungoverned would be a hole in the permit system; the
  framework prefers a crash at build time to a hole at run time.
- **There is no fake invoker in this book, and you should not write one.**
  When a test needs invocation, build the real registry
  (`registry.NewRegistry()` + `RegisterLegacyTool`) and forward to it.
  Scripting what a tool *returns* is a test double; scripting around *how
  it is admitted* is a hole in the permit system wearing a test's clothes.

## Summary

- A node is three methods — `ID`, `Type`, `Execute(ctx, envelope) →
  (Result, error)` — and nothing else. `NodeTypeObservation` is the
  shipped convention for model-backed reasoning (react's think node uses
  it).
- A graph is built with `NewGraph`/`AddNode`/`AddEdge`/`SetStart`,
  validated structurally, sealed by its first `Execute`
  (`ErrGraphSealed` on later mutation), and re-executable on the frozen
  structure.
- The run loop captures every result field into working memory as
  `"<nodeID>.<key>"` with task-class provenance — the framework's native
  dataflow between nodes.
- Preflight is the runtime's own gate: node contracts declare capability
  selectors, the catalog decides, and blocking issues stop `Execute`
  before the first node runs. `ToolNode` brings its governance contract
  with it — the full five-dimension `NodeContract` is Chapter 5's
  subject.
- The visit cap bounds accidental cycles per traversal; the default is
  1,024.

## Check yourself

1. Your node's `Execute` returns `{"draft": …}` from a node whose ID is
   `drafter`. Name two ways that draft becomes visible to the next node,
   and one difference between them.
2. What exactly does `Execute` seal, and what can you still do to a
   sealed graph?
3. A teammate wires a `ToolNode` and gets
   `graph preflight failed: no registered capability matches selector
   tool:file_read`. Give two distinct fixes, and say which one the
   framework would consider honest.
4. Why does `SetMaxNodeVisits` exist when the DAG has no cycles by
   construction? (Hint: what did the lab wire on purpose?)
5. Your `ConditionFunc` writes a working-memory value and returns true.
   Why is that a bug, even if it works today?

*Answers.* 1. The node can write it itself with full provenance
(`SetWorkingValueWithOrigin`, class + origin of its choosing), and the
runtime captures result fields automatically as `drafter.draft` with
task-class provenance; the automatic capture is uniform (every node gets
it) but fixed in class, the manual write is the node's own decision. 2.
The structure: every build-phase mutator returns `ErrGraphSealed`;
`Execute` may still be called again (visit counts reset). 3. Wire a
catalog containing (or build a registry that exposes) the
`tool:file_read` capability — or remove the tool node. The first is
honest: the graph declared a need, satisfy it; adding an exemption or
weakening the contract would be lying to preflight. 4. Because graphs
have conditional edges, and a wrong condition pair makes a cycle; the cap
converts "hang the turn" into "fail with coordinates." 5. Conditions are
evaluated against the pre-branch state before any fork — a writing
condition would race parallel branches and make routing depend on
evaluation order; conditions route, nodes compute.

## Try it yourself

1. **Three-way routing.** Extend `TestConditionalEdgesRoutes` with a
   third edge to a `reject` terminal taken when the draft contains
   "impossible". Script a `FakeTurn` that produces it and assert the run
   ends at `reject`. You are practicing the conditions-route rule with
   three exits from one node.
2. **Pre-flown preflight.** Add a second `ToolNode` to the preflight test
   with a different tool name, wire the seeded catalog to contain only the
   first tool's descriptor, and predict the report before running it:
   one placement, one blocking issue. Then run it and read the report's
   `Issues[0].NodeID`. Preflight reports are for *reading* — practice
   reading one before you have to.

## Where next

Your graph runs, seals, routes, and refuses bad wiring — but its nodes
are anonymous about governance. Next chapter the shipped nodes get their
contracts read aloud: `NodeContract`'s five class dimensions and the four
extra fields that make them nine, `ConditionalNode` (a node that *is* a
decision), and `HumanNode` — a pause inside a graph, with a synchronous
approval callback the text will describe honestly. Then the chapter's
build: the full critic pipeline — drafter → critic → conditional revise →
terminal — as a hand-composed graph, with checkpoint policy on the
revision.

The lab lives at `devdocs/agent-construction-relurpify/labs/` —
`verify.sh gates ch04` re-runs everything this chapter proved, and
`transcripts/ch04-graph-tour.md` holds the captured test run.
