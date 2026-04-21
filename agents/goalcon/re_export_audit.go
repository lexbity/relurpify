package goalcon

import (
	"codeburg.org/lexbit/relurpify/agents/goalcon/audit"
)

// Re-exports from audit package for backward compatibility
type CapabilityAuditTrail = audit.CapabilityAuditTrail
type AuditEntry = audit.AuditEntry
type ProvenanceCollector = audit.ProvenanceCollector
type ProvenanceSummary = audit.ProvenanceSummary
type MetricsRecorder = audit.MetricsRecorder
type ExecutionMetrics = audit.ExecutionMetrics

// Re-exported constructors
var (
	NewCapabilityAuditTrail = audit.NewCapabilityAuditTrail
	NewProvenanceCollector  = audit.NewProvenanceCollector
	NewMetricsRecorder      = audit.NewMetricsRecorder
)
