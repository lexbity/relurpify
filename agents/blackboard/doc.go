// Package blackboard implements a Blackboard architecture agent.
//
// The blackboard pattern uses a shared in-memory workspace (the blackboard)
// that multiple knowledge sources (KS) read and write. A controller evaluates
// KS activation conditions each cycle and selects which specialist runs next.
// Execution order is data-driven, not structurally predetermined, making this
// pattern well-suited for tasks where the exact shape of work is not known
// upfront.
//
// BlackboardAgent runs on the graph runtime: controller cycles, checkpoint
// boundaries, capability invocation, retrieval, structured persistence,
// telemetry, and resumable recovery are expressed through framework-owned
// execution surfaces rather than a package-private loop.
//
// Built-in knowledge sources cover the Explorer → Analyzer → Planner → Review
// → Executor → FailureTriage → Verifier → Summarizer lifecycle. Custom
// knowledge sources can be registered alongside the built-ins or used as a
// standalone replacement.
//
// The blackboard runtime note in docs/dev/blackboard-runtime.md remains the
// architectural contract for this package and now documents the implemented
// graph-native model plus follow-on extension areas.
package blackboard
