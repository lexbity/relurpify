// Package event defines the shared event log that serves as the canonical
// source of truth for all agent execution events across the Relurpify runtime.
//
// The event log is the single bus through which graph node transitions, tool
// call outcomes, LLM interactions, and HITL approvals are published. Consumers
// subscribe to this log independently, keeping agent execution decoupled from
// specific recording concerns.
package event
