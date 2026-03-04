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

**Cycle detection** — the runtime tracks visited (node, context-hash) pairs. If the same node is reached with identical context twice, it returns an error rather than looping forever.

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
g := graph.New("my-workflow")

think := g.AddNode(graph.NodeLLM, "think", thinkFn)
act   := g.AddNode(graph.NodeTool, "act", actFn)
done  := g.AddNode(graph.NodeTerminal, "done", nil)

// Conditional: if last LLM response had tool calls, go to act
g.AddEdge(think, act, hasToolCallsFn)
// Otherwise, done
g.AddEdge(think, done, noToolCallsFn)
// After acting, always think again
g.AddEdge(act, think, nil)

g.SetStart(think)
result, err := g.Execute(ctx, sharedCtx)
```

---

## PlanExecutor

`PlanExecutor` takes a `Plan` (from PlannerAgent) and executes each `PlanStep` as its own graph run. Steps are executed in order; a step failure stops the executor unless the step is marked optional.

This is how `CodingAgent` in `architect` mode works: PlannerAgent produces the plan, PlanExecutor drives the coding agent through each step.

---

## Checkpointing

`GraphCheckpoint` captures the full graph state at any node boundary: current node, context snapshot, pending tool calls, telemetry so far. Checkpoints can be saved and restored, enabling pause-and-resume across process restarts.

> **Status:** The checkpoint infrastructure (`CheckpointStore`, `GraphCheckpoint`) is implemented but is not yet wired into the default execution path. Graph runs do not currently auto-checkpoint.

---

## Parallel Branches

The graph supports forking into parallel branches by cloning the shared context per branch and executing branches concurrently. Results are merged by a broker node.

> **Status:** Parallel branching is architecturally supported. The default execution is sequential. Ollama handles one request at a time, so parallel LLM branches queue behind each other regardless.

---

## See Also

- [Agents](../agents.md) — how agents build and use graphs
- [Context Budget](context-budget.md) — token management during graph execution
- [Custom Agents](custom-agents.md) — building agents that use the graph runtime
