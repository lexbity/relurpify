// Package htn implements a Hierarchical Task Network (HTN) agent.
//
// Framework ownership note:
//
// The framework layer owns manifests, config schemas, effective contract
// resolution, and capability admission policy. The htn package owns only the
// runtime-facing HTN execution model: method selection, decomposition,
// capability-routed primitive dispatch, graph execution, and HTN-specific
// state publication.
//
// HTN planning decomposes complex tasks into networks of primitive subtasks
// according to a method library (declared recipes). The language model never
// decides how to structure work — it only executes focused leaf tasks, making
// this pattern maximally small-model-friendly.
//
// Callers register Methods that map TaskType values to ordered SubtaskSpec
// sequences. HTNAgent classifies the incoming task, chooses a method,
// decomposes the task into primitive work, and executes those primitive steps
// through framework-owned runtime surfaces.
//
// Current execution uses `graph.PlanExecutor`, workflow retrieval, workflow
// checkpointing, and shared `core.Context` state rather than a hidden package-
// private loop. The HTN runtime note in docs/dev/htn-runtime.md defines the
// execution contract and follow-on extension areas.
//
// Default built-in methods cover code generation, modification, review, and
// analysis workflows. Additional methods can be registered at construction time
// or overridden per-agent.
package htn
