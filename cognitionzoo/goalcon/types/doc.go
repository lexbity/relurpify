// Package types defines core types used throughout the goalcon framework.
//
// This package contains:
//   - Predicate: Fact that can be satisfied in world state
//   - GoalCondition: Desired state as conjunction of predicates
//   - WorldState: Tracks which predicates are currently satisfied
//   - Operator: Transforms world state (preconditions → effects)
//   - OperatorRegistry: Registry of available operators
//   - MetricsRecorder: Tracks execution metrics
//   - CapabilityAuditTrail: Audit log of capability invocations
//   - ExecutionTrace: Traces steps and events in execution
//
// These types are shared across all subpackages and form the common data model
// for the goalcon planning and execution framework.
package types
