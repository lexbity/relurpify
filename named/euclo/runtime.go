package euclo

import (
	"context"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/runtime"
)

// Re-export types and functions from runtime subpackage for backward compatibility.
type (
	TaskEnvelope          = runtime.TaskEnvelope
	TaskClassification    = runtime.TaskClassification
	RetrievalPolicy       = runtime.RetrievalPolicy
	ContextExpansion      = runtime.ContextExpansion
	VerificationPolicy    = runtime.VerificationPolicy
	VerificationEvidence  = runtime.VerificationEvidence
	SuccessGateResult     = runtime.SuccessGateResult
	EditIntent            = runtime.EditIntent
	EditOperationRecord   = runtime.EditOperationRecord
	EditExecutionRecord   = runtime.EditExecutionRecord
	PhaseCapabilityRoute  = runtime.PhaseCapabilityRoute
	CapabilityFamilyRouting = runtime.CapabilityFamilyRouting
	ActionLogEntry        = runtime.ActionLogEntry
	ProofSurface          = runtime.ProofSurface
	RuntimeSurfaces       = runtime.RuntimeSurfaces
)

var (
	NormalizeTaskEnvelope         = runtime.NormalizeTaskEnvelope
	ClassifyTask                  = runtime.ClassifyTask
	ResolveMode                   = runtime.ResolveMode
	SelectExecutionProfile        = runtime.SelectExecutionProfile
	ResolveRetrievalPolicy        = runtime.ResolveRetrievalPolicy
	ExpandContext                 = runtime.ExpandContext
	ApplyContextExpansion         = runtime.ApplyContextExpansion
	ResolveVerificationPolicy     = runtime.ResolveVerificationPolicy
	NormalizeVerificationEvidence = runtime.NormalizeVerificationEvidence
	EvaluateSuccessGate           = runtime.EvaluateSuccessGate
	ApplyEditIntentArtifacts      = runtime.ApplyEditIntentArtifacts
	ExecuteEditIntents            = runtime.ExecuteEditIntents
	RouteCapabilityFamilies       = runtime.RouteCapabilityFamilies
	BuildActionLog                = runtime.BuildActionLog
	BuildProofSurface             = runtime.BuildProofSurface
	EmitObservabilityTelemetry    = runtime.EmitObservabilityTelemetry
	SnapshotCapabilities          = runtime.SnapshotCapabilities
)

// Private helpers used by agent.go and profile_controller.go

func resolveRuntimeSurfaces(store memory.MemoryStore) runtime.RuntimeSurfaces {
	switch typed := store.(type) {
	case *memory.CompositeRuntimeStore:
		surfaces := runtime.RuntimeSurfaces{Runtime: typed.RuntimeMemoryStore}
		if workflow, ok := typed.WorkflowStateStore.(*db.SQLiteWorkflowStateStore); ok {
			surfaces.Workflow = workflow
		}
		return surfaces
	case memory.RuntimeMemoryStore:
		return runtime.RuntimeSurfaces{Runtime: typed}
	default:
		return runtime.RuntimeSurfaces{}
	}
}

func ensureWorkflowRun(ctx context.Context, store *db.SQLiteWorkflowStateStore, task *core.Task, state *core.Context) (string, string, error) {
	// Forward to runtime package
	return runtime.EnsureWorkflowRun(ctx, store, task, state)
}

func applyContextExpansion(state *core.Context, task *core.Task, expansion ContextExpansion) *core.Task {
	// Forward to runtime package
	return runtime.ApplyContextExpansion(state, task, expansion)
}

func classificationContextPayload(classification TaskClassification) map[string]any {
	return map[string]any{
		"intent_families":                   append([]string{}, classification.IntentFamilies...),
		"recommended_mode":                  classification.RecommendedMode,
		"mixed_intent":                      classification.MixedIntent,
		"edit_permitted":                    classification.EditPermitted,
		"requires_evidence_before_mutation": classification.RequiresEvidenceBeforeMutation,
		"requires_deterministic_stages":     classification.RequiresDeterministicStages,
		"scope":                             classification.Scope,
		"risk_level":                        classification.RiskLevel,
		"reason_codes":                      append([]string{}, classification.ReasonCodes...),
	}
}

func snapshotCapabilities(registry *capability.Registry) euclotypes.CapabilitySnapshot {
	// Forward to runtime package
	return runtime.SnapshotCapabilities(registry)
}

func stringValue(raw any) string {
	if raw == nil {
		return ""
	}
	if s, ok := raw.(string); ok {
		return s
	}
	return ""
}
