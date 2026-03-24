# Graph Runtime

## Synopsis

The graph runtime (`framework/graph`) is the execution engine that agents run on. It models agent workflows as deterministic state machines — directed graphs of typed nodes connected by conditional edges. Rather than letting the LLM decide the control flow freely, the agent defines a graph upfront and the runtime walks it node by node.

---

## Why a Graph

Local models are less reliable at maintaining multi-step structure in a single unbounded conversation. A graph encodes the workflow explicitly: the agent declares what steps exist, in what order, and under what conditions to branch. This gives you:

- **Predictability** — you know the possible execution paths at design time
- **Observability** — every node execution is recorded with timing and telemetry
- **Resumability** — the graph state can be checkpointed and restored
- **Cycle detection** — infinite loops are caught before they hang the process

---

## Core Concepts

### Node

A node is a single step. It has a type, an optional `Execute` function, and outgoing edges. When the runtime reaches a node it calls `Execute`, collects the result, and follows the appropriate outgoing edge.

### Edge

An edge connects two nodes. Unconditional edges are always followed. Conditional edges carry a `ConditionFunc` — the edge is followed only when the function returns true. Multiple conditional edges from one node implement branching.

### Graph

A graph is a set of nodes and edges with a designated start node. `graph.Execute(ctx, sharedCtx)` walks the graph until a `Terminal` node is reached, a cycle is detected, or an error occurs.

Before execution, the runtime validates the graph structure and runs preflight
against the active capability catalog when one is configured. Validation and
preflight results are cached until the graph structure or catalog changes.

---

## Node Types

| Type | Purpose |
|------|---------|
| `LLM` | Calls Ollama with the current context; appends the model's response |
| `Tool` | Executes a tool from the registry; appends the result as an Observation |
| `Conditional` | Evaluates a condition and selects the next node; no side effects |
| `Human` | Pauses execution and emits a HITL event; waits for approval or denial |
| `Terminal` | Ends the graph; carries the final result |
| `System` | Injects a system message into the context |
| `Observation` | Records an external event or observation into the context |

---

## Execution Loop

```
graph.Execute(ctx, sharedCtx)
        │
        ▼
  currentNode = startNode
        │
        ▼
  node.Execute(ctx, sharedCtx)
        │
        ├── LLM:       call Ollama → parse tool calls → queue Tool nodes
        ├── Tool:      check permissions → run tool → append result
        ├── Conditional: evaluate ConditionFunc → select edge
        ├── Human:     emit HITL event → block until resolved
        └── Terminal:  return Result
        │
        ▼
  resolve outgoing edges → advance to next node
        │
        ▼
  (repeat until Terminal or error)
```

**Cycle detection** — the runtime tracks per-node visit counts and rejects runs
that exceed the configured `maxNodeVisits` threshold.

**Telemetry** — every node execution is recorded: node type and name, start/end time, input token count, output summary, errors. Records are collected in the shared context and forwarded to the telemetry sinks.

---

## The ReAct Pattern as a Graph

The standard ReAct loop maps to a graph like this:

```
[System] → [LLM: think] → [Tool: act] → [Conditional: done?]
                ▲                               │
                │                               ├─ no  → back to LLM
                └───────────────────────────────┘
                                                │
                                                └─ yes → [Terminal]
```

The conditional edge checks whether the LLM's last response contained any tool calls. If not, the agent is done. If yes, it loops back.

---

## Building a Graph

Agents construct their graph in `Initialize()` or lazily in `Execute()`.

```go
g := graph.NewGraph()

think := &graph.LLMNode{}
act := graph.NewToolNode("act", tool, nil, registry)
done := graph.NewTerminalNode("done")

_ = g.AddNode(think)
_ = g.AddNode(act)
_ = g.AddNode(done)
_ = g.AddEdge(think.ID(), act.ID(), hasToolCallsFn, false)
_ = g.AddEdge(think.ID(), done.ID(), noToolCallsFn, false)
_ = g.AddEdge(act.ID(), think.ID(), nil, false)

_ = g.SetStart(think.ID())
result, err := g.Execute(ctx, sharedCtx)
```

---

## PlanExecutor

`PlanExecutor` is a framework-owned workflow runner for dependency-aware
`PlanStep` execution. It drives a `graph.WorkflowExecutor` through ready steps,
supports optional branch isolation for parallel-ready steps, and leaves
step-shaping, completed-step tracking, and recovery policy to the caller.

Higher-level agent paradigms such as architect- and HTN-style execution build
on this runner, but the runner itself is runtime-oriented rather than
agent-specific.

---

## Checkpointing

`GraphCheckpoint` captures a transition boundary: the completed node, the next
node, transition metadata, execution counters, and a context snapshot.
Checkpoints can be saved and restored, enabling pause-and-resume across process
restarts without replaying completed work.

Checkpointing is wired into the default execution path in two ways:

- callback-based checkpointing via `WithCheckpointing(every N, saveFn)`
- explicit checkpoint persistence via `CheckpointNode`

---

## Parallel Branches

The graph supports forking into parallel branches by cloning the parent
`Context` per branch and executing branches concurrently.

Branch results are not merged by merging whole contexts. Instead, each branch
reports explicit context deltas and the default merge policy:

- merges non-conflicting state-key writes
- rejects conflicting writes to the same key
- rejects variable mutations
- rejects knowledge mutations
- rejects history/compression/phase mutations

This makes branch convergence deterministic and surfaces hidden shared-state
coupling as an error instead of silently overwriting one branch with another.

---
