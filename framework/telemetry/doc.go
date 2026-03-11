// Package telemetry collects structured audit records for every significant
// agent execution event: LLM calls, tool invocations, graph node transitions,
// and capability dispatch outcomes.
//
// Records are written as append-only structured log entries in the workspace
// telemetry directory. event_adapter.go bridges the telemetry sink to the
// shared framework/event log, so a single event emission propagates to both
// the local audit trail and the Nexus gateway's observability stream.
package telemetry
