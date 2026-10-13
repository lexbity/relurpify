package otelleg

// Lifecycle family names (spec D2). They label span keys and are matched by the
// assembler's name/attribute switches.
const (
	familyGraph      = "graph"
	familyNode       = "node"
	familyAgent      = "agent"
	familyDelegation = "delegation"
	familyLLM        = "llm"
	familyTool       = "tool"
	familyCapability = "capability"
)

// Correlation and metadata keys consumed by the projection and the assembler.
// Centralizing the spellings keeps producers and consumers from drifting.
const (
	keyRunID        = "run_id"
	keyAgentID      = "agent_id"
	keyNodeID       = "node_id"
	keyTaskID       = "task_id"
	keySpanID       = "span_id"
	keyParentSpanID = "parent_span_id"
	keyCallID       = "call_id"
	keyRequestID    = "request_id"
	keyDelegationID = "delegation_id"

	metaTool            = "tool"
	metaModel           = "model"
	metaCapability      = "capability"
	metaCapabilityID    = "capability_id"
	metaKind            = "kind"
	metaRuntimeFamily   = "runtime_family"
	metaFinishReason    = "finish_reason"
	metaToolCallingMode = "tool_calling_mode"
	metaSystem          = "system"
	metaSpanAttrs       = "span_attrs"
	metaOTelSpan        = "otel_span"
	metaTrustClass      = "trust_class"
	metaEffectClass     = "effect_class"
	metaSuccess         = "success"
	metaError           = "error"
	metaToolError       = "tool_error"
	metaCapabilityError = "capability_error"
	metaStatus          = "status"
	metaDurationMS      = "duration_ms"
	metaExitCode        = "exit_code"
	metaStdoutBytes     = "stdout_bytes"
	metaArtifactRef     = "artifact_ref"
)

// Status strings returned by spanStatus and mapped to OTel status codes.
const (
	statusOK    = "ok"
	statusError = "error"
)
