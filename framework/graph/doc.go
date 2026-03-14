// Package graph provides a deterministic state-machine workflow runtime for
// Relurpify agents.
//
// # Graph runtime
//
// A Graph is a directed acyclic structure of typed nodes. The executor runs
// nodes in topological order, threading a Context through each step and
// collecting results. Parallel branches clone the context, execute
// independently, and merge explicit state deltas back into the parent context
// when they converge.
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
// transition boundary (completed node, next node, execution counters, and
// context snapshot) so interrupted workflows can continue without replaying
// completed work.
//
// # Plan executor
//
// plan_executor.go executes dependency-aware plan/workflow steps against a
// runtime executor contract with optional branch isolation, retry hooks, and
// checkpoint-friendly state handling. Agent packages may supply step shaping,
// recovery policy, completed-step tracking, and state conventions on top of
// this framework-owned runner.
package graph
