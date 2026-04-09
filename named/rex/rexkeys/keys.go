package rexkeys

// Canonical rex state keys. Keys marked TRUSTED are server-assigned and should
// never be written from caller-controlled payloads.
const (
	WorkflowID = "workflow_id"
	RunID      = "run_id"

	RexWorkflowID = "rex.workflow_id"
	RexRunID      = "rex.run_id"

	FMPLineageID = "fmp.lineage_id"
	FMPAttemptID = "fmp.attempt_id"

	RexFMPLineageID = "rex.fmp_lineage_id"
	RexFMPAttemptID = "rex.fmp_attempt_id"

	GatewaySessionID = "gateway.session_id"
	GatewayTenantID  = "gateway.tenant_id"

	RexEventType          = "rex.event_type"
	RexEventID            = "rex.event_id"
	RexEventPartition     = "rex.event_partition"
	RexEventIngressOrigin = "rex.event_ingress_origin"

	// TRUSTED - server-set only, never from payload.
	RexAdmissionTenantID = "rex.admission_tenant_id"
	// TRUSTED - server-set only, never from payload.
	RexWorkloadClass = "rex.workload_class"

	SessionID = "session_id"
)
