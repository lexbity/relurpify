// Package jobs answers: how is deferred or long-running work durably scheduled,
// tracked, resumed, and cancelled?
//
// The contract for the durable queue (executed by the ayenitd runner, stored
// by context/jobsstore on Badger): jobs are created by the spool-ingesting
// runner, claimed atomically by executor workers (queue order, priority
// descending, FIFO within a queue), retried with exponential backoff
// (NextBackoff), and cancelled effectively only while queued — a running job
// cannot be cancelled cross-process (no channel to the runner); shutdown
// grace bounds attempts instead.
//
// Delivery is at-least-once (crash recovery at store Open re-queues
// interrupted jobs); handlers MUST be idempotent per Spec.CorrelateID
// (handler.go, H1-H5).
package jobs
