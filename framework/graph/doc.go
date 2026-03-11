// Package graph provides a deterministic state-machine workflow runtime for
// Relurpify agents.
//
// # Graph runtime
//
// A Graph is a directed acyclic structure of typed nodes. The executor runs
// nodes in topological order, threading a Context through each step and
// collecting results. Parallel branches clone the context, execute
// independently, and merge via SharedContext when they converge.
//
// # Node types
//
//   - LLMNode: calls the language model and routes its response.
//   - ToolNode: invokes a registered capability and captures the observation.
//   - ConditionalNode: branches on a predicate over the current context.
//   - HumanNode: pauses execution and waits for a HITL response.
//   - SystemNode: injects a system-level message or state transformation.
//   - ObservationNode: records a tool or environment observation.
//   - TerminalNode: signals successful or failed completion.
//
// # Checkpointing
//
// graph_checkpoint.go supports pause-and-resume: a checkpoint captures the
// full execution state (completed nodes, pending branches, context snapshot)
// so interrupted workflows can continue without replaying from the start.
//
// # Plan executor
//
// plan_executor.go compiles a Plan (ordered list of steps) into a linear
// graph, enabling agents that produce structured plans to hand off execution
// to the graph runtime with no manual wiring.
package graph
