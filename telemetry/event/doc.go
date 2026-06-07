// Package event defines the shared event log that serves as the canonical
// source of truth for all agent execution events across the Relurpify runtime.
//
// The event log is the single bus through which graph node transitions, tool
// call outcomes, LLM interactions, and HITL approvals are published. Consumers
// — the telemetry package for local audit, and the Nexus gateway materializer
// for distributed observability — subscribe to this log independently, keeping
// the core agent runtime decoupled from specific recording concerns.
package event
