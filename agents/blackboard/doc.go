// Package blackboard implements a Blackboard architecture agent.
//
// The blackboard pattern uses a shared in-memory workspace (the blackboard)
// that multiple knowledge sources (KS) read and write. A controller evaluates
// KS activation conditions each cycle and selects which specialist runs next.
// Execution order is data-driven, not structurally predetermined, making this
// pattern well-suited for tasks where the exact shape of work is not known
// upfront.
//
// Built-in knowledge sources cover the Explorer → Analyzer → Planner →
// Executor → Verifier lifecycle. Custom knowledge sources can be registered
// alongside the built-ins or used as a standalone replacement.
//
// The blackboard workspace lives in core.Context for in-process sharing and is
// flushed to a CheckpointPersister at configurable intervals for recovery.
package blackboard
