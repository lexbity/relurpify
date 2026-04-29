// Package persistence provides persistence adapters, schema serialization, validation, and versioning.
//
// This package owns:
// - Schema-encoded read/write models
// - Backend adapter interfaces
// - Serialization and deserialization boundaries
// - Schema version migration helpers
// - Validation of persisted payloads
//
// The persistence layer is an implementation detail. Domain packages (compiler, agentlifecycle)
// depend on narrow adapter interfaces defined here, while concrete implementations live in
// framework/graphdb or other backends.
//
// This package does NOT own business logic or domain semantics. Those live in the
// respective domain packages (compiler, agentlifecycle).
package persistence
