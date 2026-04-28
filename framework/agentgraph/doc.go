// Package agentgraph provides a deterministic state-machine workflow runtime for
// Relurpify agents.
//
// # Graph runtime
//
// A Graph is a directed acyclic structure of typed nodes. The executor runs
// nodes in topological order, threading a contextdata.Envelope through each step
// and collecting results. The envelope implements a tiered context model with
// three layers:
//
//   - Working memory: mutable task-specific state (set via SetWorkingValue)
//   - Streamed context: references to externally streamed content
//   - Retrieval results: references to knowledge store queries
//
// Parallel branches clone the envelope via contextdata.CloneEnvelope, execute
// independently, and merge branch deltas back into the parent envelope using
// contextdata.MergeBranchEnvelopes when they converge. The merge validates
// that branches do not conflict on the same keys.
//
// # Node types
//
//   - ToolNode: invokes a registered capability and captures the observation.
//   - ConditionalNode: branches on a predicate over the current envelope.
//   - HumanNode: pauses execution and waits for a HITL response.
//   - SystemNode: injects a system-level message or state transformation.
//   - ObservationNode: records a tool or environment observation.
//   - TerminalNode: signals successful or failed completion.
//   - RetrievalNode: retrieves context from the knowledge store and records
//     retrieval references in the envelope.
//   - CheckpointNode: requests checkpoint materialization via the envelope's
//     RequestCheckpoint method (the compiler owns the actual checkpoint).
//
// External node types (defined in other packages):
//   - LLMNode (agents/llm): calls the language model and routes its response.
//
// # Checkpointing
//
// graph_checkpoint.go supports pause-and-resume: a checkpoint captures the
// transition boundary (completed node, next node, execution counters, and
// envelope snapshot) so interrupted workflows can continue without replaying
// completed work. Checkpoint materialization is request-only: nodes call
// env.RequestCheckpoint(reason, priority, evictMemory) and the compiler
// handles the actual checkpoint creation and storage.
//
// # Plan executor
//
// plan_executor.go executes dependency-aware plan/workflow steps against a
// runtime executor contract with optional branch isolation, retry hooks, and
// checkpoint-friendly state handling. Agent packages may supply step shaping,
// recovery policy, completed-step tracking, and state conventions on top of
// this framework-owned runner. The plan executor uses the same envelope-based
// context model as the graph runtime.
package agentgraph
