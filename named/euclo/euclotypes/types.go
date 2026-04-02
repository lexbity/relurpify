package euclotypes

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
)

// ============================================================================
// Artifacts
// ============================================================================

type ArtifactKind string

const (
	ArtifactKindIntake                  ArtifactKind = "euclo.intake"
	ArtifactKindClassification          ArtifactKind = "euclo.classification"
	ArtifactKindModeResolution          ArtifactKind = "euclo.mode_resolution"
	ArtifactKindExecutionProfile        ArtifactKind = "euclo.execution_profile"
	ArtifactKindRetrievalPolicy         ArtifactKind = "euclo.retrieval_policy"
	ArtifactKindContextExpansion        ArtifactKind = "euclo.context_expansion"
	ArtifactKindCapabilityRouting       ArtifactKind = "euclo.capability_routing"
	ArtifactKindVerificationPolicy      ArtifactKind = "euclo.verification_policy"
	ArtifactKindSuccessGate             ArtifactKind = "euclo.success_gate"
	ArtifactKindActionLog               ArtifactKind = "euclo.action_log"
	ArtifactKindProofSurface            ArtifactKind = "euclo.proof_surface"
	ArtifactKindWorkflowRetrieval       ArtifactKind = "euclo.workflow_retrieval"
	ArtifactKindExplore                 ArtifactKind = "euclo.explore"
	ArtifactKindTrace                   ArtifactKind = "euclo.trace"
	ArtifactKindAnalyze                 ArtifactKind = "euclo.analyze"
	ArtifactKindReviewFindings          ArtifactKind = "euclo.review_findings"
	ArtifactKindCompatibilityAssessment ArtifactKind = "euclo.compatibility_assessment"
	ArtifactKindPlan                    ArtifactKind = "euclo.plan"
	ArtifactKindMigrationPlan           ArtifactKind = "euclo.migration_plan"
	ArtifactKindPlanCandidates          ArtifactKind = "euclo.plan_candidates"
	ArtifactKindEditIntent              ArtifactKind = "euclo.edit_intent"
	ArtifactKindEditExecution           ArtifactKind = "euclo.edit_execution"
	ArtifactKindVerificationPlan        ArtifactKind = "euclo.verification_plan"
	ArtifactKindVerification            ArtifactKind = "euclo.verification"
	ArtifactKindTDDLifecycle            ArtifactKind = "euclo.tdd_lifecycle"
	ArtifactKindWaiver                  ArtifactKind = "euclo.waiver"
	ArtifactKindDiffSummary             ArtifactKind = "euclo.diff_summary"
	ArtifactKindVerificationSummary     ArtifactKind = "euclo.verification_summary"
	ArtifactKindProfileSelection        ArtifactKind = "euclo.profile_selection"
	ArtifactKindReproduction            ArtifactKind = "euclo.reproduction"
	ArtifactKindRootCause               ArtifactKind = "euclo.root_cause"
	ArtifactKindRootCauseCandidates     ArtifactKind = "euclo.root_cause_candidates"
	ArtifactKindRegressionAnalysis      ArtifactKind = "euclo.regression_analysis"
	ArtifactKindCompiledExecution       ArtifactKind = "euclo.compiled_execution"
	ArtifactKindExecutionStatus         ArtifactKind = "euclo.execution_status"
	ArtifactKindDeferredExecutionIssues ArtifactKind = "euclo.deferred_execution_issues"
	ArtifactKindContextCompaction       ArtifactKind = "euclo.context_compaction"
	ArtifactKindFinalReport             ArtifactKind = "euclo.final_report"
	ArtifactKindRecoveryTrace           ArtifactKind = "euclo.recovery_trace"
)

// Artifact is Euclo's normalized runtime view over workflow and state outputs.
type Artifact struct {
	ID           string
	Kind         ArtifactKind
	Summary      string
	Metadata     map[string]any
	Payload      any
	ProducerID   string   // capability ID that produced this artifact
	Status       string   // "produced" / "pending" / "failed"
	EvidenceRefs []string // references to evidence-bearing records
}

type WorkflowArtifactWriter interface {
	UpsertWorkflowArtifact(ctx context.Context, artifact memory.WorkflowArtifactRecord) error
}

type WorkflowArtifactReader interface {
	ListWorkflowArtifacts(ctx context.Context, workflowID, runID string) ([]memory.WorkflowArtifactRecord, error)
}

// CollectArtifactsFromState adapts legacy pipeline/workflow state into a small,
// typed Euclo artifact set.
func CollectArtifactsFromState(state *core.Context) []Artifact {
	if state == nil {
		return nil
	}
	defs := []struct {
		Key       string
		Kind      ArtifactKind
		Normalize func(any) (any, map[string]any)
	}{
		{Key: "euclo.envelope", Kind: ArtifactKindIntake},
		{Key: "euclo.classification", Kind: ArtifactKindClassification},
		{Key: "euclo.mode_resolution", Kind: ArtifactKindModeResolution},
		{Key: "euclo.execution_profile_selection", Kind: ArtifactKindExecutionProfile},
		{Key: "euclo.retrieval_policy", Kind: ArtifactKindRetrievalPolicy},
		{Key: "euclo.context_expansion", Kind: ArtifactKindContextExpansion},
		{Key: "euclo.capability_family_routing", Kind: ArtifactKindCapabilityRouting},
		{Key: "euclo.verification_policy", Kind: ArtifactKindVerificationPolicy},
		{Key: "euclo.success_gate", Kind: ArtifactKindSuccessGate},
		{Key: "euclo.action_log", Kind: ArtifactKindActionLog},
		{Key: "euclo.proof_surface", Kind: ArtifactKindProofSurface},
		{Key: "pipeline.workflow_retrieval", Kind: ArtifactKindWorkflowRetrieval, Normalize: normalizeWorkflowRetrieval},
		{Key: "pipeline.explore", Kind: ArtifactKindExplore},
		{Key: "euclo.trace", Kind: ArtifactKindTrace},
		{Key: "pipeline.analyze", Kind: ArtifactKindAnalyze},
		{Key: "euclo.review_findings", Kind: ArtifactKindReviewFindings},
		{Key: "euclo.compatibility_assessment", Kind: ArtifactKindCompatibilityAssessment},
		{Key: "pipeline.plan", Kind: ArtifactKindPlan},
		{Key: "euclo.migration_plan", Kind: ArtifactKindMigrationPlan},
		{Key: "euclo.plan_candidates", Kind: ArtifactKindPlanCandidates},
		{Key: "pipeline.code", Kind: ArtifactKindEditIntent, Normalize: normalizeEditIntent},
		{Key: "euclo.edit_execution", Kind: ArtifactKindEditExecution},
		{Key: "euclo.verification_plan", Kind: ArtifactKindVerificationPlan},
		{Key: "pipeline.verify", Kind: ArtifactKindVerification},
		{Key: "euclo.tdd.lifecycle", Kind: ArtifactKindTDDLifecycle},
		{Key: "euclo.waiver", Kind: ArtifactKindWaiver},
		{Key: "euclo.diff_summary", Kind: ArtifactKindDiffSummary},
		{Key: "euclo.verification_summary", Kind: ArtifactKindVerificationSummary},
		{Key: "euclo.profile_selection", Kind: ArtifactKindProfileSelection},
		{Key: "euclo.reproduction", Kind: ArtifactKindReproduction},
		{Key: "euclo.root_cause", Kind: ArtifactKindRootCause},
		{Key: "euclo.root_cause_candidates", Kind: ArtifactKindRootCauseCandidates},
		{Key: "euclo.regression_analysis", Kind: ArtifactKindRegressionAnalysis},
		{Key: "euclo.compiled_execution", Kind: ArtifactKindCompiledExecution},
		{Key: "euclo.execution_status", Kind: ArtifactKindExecutionStatus},
		{Key: "euclo.deferred_execution_issues", Kind: ArtifactKindDeferredExecutionIssues},
		{Key: "euclo.context_compaction", Kind: ArtifactKindContextCompaction},
		{Key: "pipeline.final_output", Kind: ArtifactKindFinalReport},
	}

	out := make([]Artifact, 0, len(defs))
	for _, def := range defs {
		raw, ok := state.Get(def.Key)
		if !ok || raw == nil {
			continue
		}
		payload := raw
		metadata := map[string]any{"source_key": def.Key}
		if def.Normalize != nil {
			payload, metadata = def.Normalize(raw)
			if metadata == nil {
				metadata = map[string]any{}
			}
			metadata["source_key"] = def.Key
		}
		out = append(out, Artifact{
			ID:       strings.ReplaceAll(string(def.Kind), ".", "_"),
			Kind:     def.Kind,
			Summary:  artifactSummary(payload),
			Metadata: metadata,
			Payload:  payload,
			Status:   "produced",
		})
	}
	return out
}

func PersistWorkflowArtifacts(ctx context.Context, store WorkflowArtifactWriter, workflowID, runID string, artifacts []Artifact) error {
	if store == nil || strings.TrimSpace(workflowID) == "" || len(artifacts) == 0 {
		return nil
	}
	for _, artifact := range artifacts {
		payload, err := json.Marshal(artifact.Payload)
		if err != nil {
			return fmt.Errorf("marshal artifact %s: %w", artifact.Kind, err)
		}
		metadata := cloneMap(artifact.Metadata)
		if metadata == nil {
			metadata = map[string]any{}
		}
		if artifact.ProducerID != "" {
			metadata["producer_id"] = artifact.ProducerID
		}
		if artifact.Status != "" {
			metadata["status"] = artifact.Status
		}
		if len(artifact.EvidenceRefs) > 0 {
			metadata["evidence_refs"] = artifact.EvidenceRefs
		}
		record := memory.WorkflowArtifactRecord{
			ArtifactID:      firstNonEmpty(strings.TrimSpace(artifact.ID), strings.ReplaceAll(string(artifact.Kind), ".", "_")),
			WorkflowID:      strings.TrimSpace(workflowID),
			RunID:           strings.TrimSpace(runID),
			Kind:            string(artifact.Kind),
			ContentType:     "application/json",
			StorageKind:     memory.ArtifactStorageInline,
			SummaryText:     artifact.Summary,
			SummaryMetadata: metadata,
			InlineRawText:   string(payload),
			RawSizeBytes:    int64(len(payload)),
		}
		if err := store.UpsertWorkflowArtifact(ctx, record); err != nil {
			return fmt.Errorf("persist artifact %s: %w", artifact.Kind, err)
		}
	}
	return nil
}

func LoadPersistedArtifacts(ctx context.Context, store WorkflowArtifactReader, workflowID, runID string) ([]Artifact, error) {
	if store == nil || strings.TrimSpace(workflowID) == "" {
		return nil, nil
	}
	records, err := store.ListWorkflowArtifacts(ctx, strings.TrimSpace(workflowID), strings.TrimSpace(runID))
	if err != nil {
		return nil, err
	}
	out := make([]Artifact, 0, len(records))
	for _, record := range records {
		payload := decodeArtifactPayload(record.InlineRawText)
		metadata := cloneMap(record.SummaryMetadata)
		artifact := Artifact{
			ID:       strings.TrimSpace(record.ArtifactID),
			Kind:     ArtifactKind(strings.TrimSpace(record.Kind)),
			Summary:  strings.TrimSpace(record.SummaryText),
			Metadata: metadata,
			Payload:  payload,
		}
		if metadata != nil {
			if pid, ok := metadata["producer_id"].(string); ok {
				artifact.ProducerID = pid
			}
			if status, ok := metadata["status"].(string); ok {
				artifact.Status = status
			}
			if refs, ok := metadata["evidence_refs"].([]any); ok {
				for _, ref := range refs {
					if s, ok := ref.(string); ok {
						artifact.EvidenceRefs = append(artifact.EvidenceRefs, s)
					}
				}
			}
		}
		out = append(out, artifact)
	}
	return out, nil
}

func RestoreStateFromArtifacts(state *core.Context, artifacts []Artifact) {
	if state == nil || len(artifacts) == 0 {
		return
	}
	for _, artifact := range artifacts {
		key := StateKeyForArtifactKind(artifact.Kind)
		if key == "" {
			continue
		}
		state.Set(key, artifact.Payload)
	}
	state.Set("euclo.artifacts", append([]Artifact{}, artifacts...))
}

func AssembleFinalReport(artifacts []Artifact) map[string]any {
	report := map[string]any{
		"artifacts": len(artifacts),
	}
	if len(artifacts) == 0 {
		return report
	}
	order := []ArtifactKind{
		ArtifactKindIntake,
		ArtifactKindClassification,
		ArtifactKindModeResolution,
		ArtifactKindExecutionProfile,
		ArtifactKindRetrievalPolicy,
		ArtifactKindContextExpansion,
		ArtifactKindCapabilityRouting,
		ArtifactKindVerificationPolicy,
		ArtifactKindActionLog,
		ArtifactKindProofSurface,
		ArtifactKindTrace,
		ArtifactKindReviewFindings,
		ArtifactKindCompatibilityAssessment,
		ArtifactKindMigrationPlan,
		ArtifactKindPlanCandidates,
		ArtifactKindEditIntent,
		ArtifactKindEditExecution,
		ArtifactKindVerificationPlan,
		ArtifactKindVerification,
		ArtifactKindTDDLifecycle,
		ArtifactKindWaiver,
		ArtifactKindDiffSummary,
		ArtifactKindVerificationSummary,
		ArtifactKindProfileSelection,
		ArtifactKindReproduction,
		ArtifactKindRootCause,
		ArtifactKindRootCauseCandidates,
		ArtifactKindRegressionAnalysis,
		ArtifactKindCompiledExecution,
		ArtifactKindExecutionStatus,
		ArtifactKindDeferredExecutionIssues,
		ArtifactKindContextCompaction,
		ArtifactKindSuccessGate,
		ArtifactKindFinalReport,
	}
	for _, kind := range order {
		if artifact, ok := firstArtifactOfKind(artifacts, kind); ok {
			switch kind {
			case ArtifactKindIntake:
				report["task"] = artifact.Payload
			case ArtifactKindClassification:
				report["classification"] = artifact.Payload
			case ArtifactKindModeResolution:
				report["mode"] = artifact.Payload
			case ArtifactKindExecutionProfile:
				report["execution_profile"] = artifact.Payload
			case ArtifactKindRetrievalPolicy:
				report["retrieval_policy"] = artifact.Payload
			case ArtifactKindContextExpansion:
				report["context_expansion"] = artifact.Payload
			case ArtifactKindCapabilityRouting:
				report["capability_routing"] = artifact.Payload
			case ArtifactKindVerificationPolicy:
				report["verification_policy"] = artifact.Payload
			case ArtifactKindActionLog:
				report["action_log"] = artifact.Payload
			case ArtifactKindProofSurface:
				report["proof_surface"] = artifact.Payload
			case ArtifactKindTrace:
				report["trace"] = artifact.Payload
			case ArtifactKindReviewFindings:
				report["review_findings"] = artifact.Payload
			case ArtifactKindCompatibilityAssessment:
				report["compatibility_assessment"] = artifact.Payload
			case ArtifactKindMigrationPlan:
				report["migration_plan"] = artifact.Payload
			case ArtifactKindPlanCandidates:
				report["plan_candidates"] = artifact.Payload
			case ArtifactKindEditIntent:
				report["edit_intent"] = artifact.Payload
			case ArtifactKindEditExecution:
				report["edit_execution"] = artifact.Payload
			case ArtifactKindVerificationPlan:
				report["verification_plan"] = artifact.Payload
			case ArtifactKindVerification:
				report["verification"] = artifact.Payload
			case ArtifactKindTDDLifecycle:
				report["tdd_lifecycle"] = artifact.Payload
			case ArtifactKindWaiver:
				report["waiver"] = artifact.Payload
			case ArtifactKindDiffSummary:
				report["diff_summary"] = artifact.Payload
			case ArtifactKindVerificationSummary:
				report["verification_summary"] = artifact.Payload
			case ArtifactKindProfileSelection:
				report["profile_selection"] = artifact.Payload
			case ArtifactKindReproduction:
				report["reproduction"] = artifact.Payload
			case ArtifactKindRootCause:
				report["root_cause"] = artifact.Payload
			case ArtifactKindRootCauseCandidates:
				report["root_cause_candidates"] = artifact.Payload
			case ArtifactKindRegressionAnalysis:
				report["regression_analysis"] = artifact.Payload
			case ArtifactKindCompiledExecution:
				report["compiled_execution"] = artifact.Payload
				switch typed := artifact.Payload.(type) {
				case map[string]any:
					if raw, ok := typed["semantic_inputs"]; ok {
						report["semantic_inputs"] = raw
					}
					if raw, ok := typed["resolved_execution_policy"]; ok {
						report["resolved_execution_policy"] = raw
					}
					if raw, ok := typed["executor_descriptor"]; ok {
						report["executor_descriptor"] = raw
					}
					if raw, ok := typed["provider_snapshot_refs"]; ok {
						report["provider_snapshot_refs"] = raw
					}
					if raw, ok := typed["provider_session_snapshot_refs"]; ok {
						report["provider_session_snapshot_refs"] = raw
					}
				default:
					if blob, ok := normalizeRuntimeCompiledExecution(typed); ok {
						if raw, ok := blob["semantic_inputs"]; ok {
							report["semantic_inputs"] = raw
						}
						if raw, ok := blob["resolved_execution_policy"]; ok {
							report["resolved_execution_policy"] = raw
						}
						if raw, ok := blob["executor_descriptor"]; ok {
							report["executor_descriptor"] = raw
						}
						if raw, ok := blob["provider_snapshot_refs"]; ok {
							report["provider_snapshot_refs"] = raw
						}
						if raw, ok := blob["provider_session_snapshot_refs"]; ok {
							report["provider_session_snapshot_refs"] = raw
						}
					}
				}
			case ArtifactKindExecutionStatus:
				report["execution_status"] = artifact.Payload
			case ArtifactKindDeferredExecutionIssues:
				report["deferred_execution_issues"] = artifact.Payload
			case ArtifactKindContextCompaction:
				report["context_compaction"] = artifact.Payload
			case ArtifactKindSuccessGate:
				report["success_gate"] = artifact.Payload
			case ArtifactKindFinalReport:
				if payload, ok := artifact.Payload.(map[string]any); ok {
					for key, value := range payload {
						report[key] = value
					}
				}
			}
		}
	}
	if resultClass := executionResultClassFromReport(report); resultClass != "" {
		report["result_class"] = resultClass
	}
	if assuranceClass := assuranceClassFromReport(report); assuranceClass != "" {
		report["assurance_class"] = assuranceClass
	}
	if deferredRefs := deferredIssueIDsFromReport(report); len(deferredRefs) > 0 {
		report["deferred_issue_ids"] = deferredRefs
	}
	summaries := make([]map[string]any, 0, len(artifacts))
	for _, artifact := range artifacts {
		summaries = append(summaries, map[string]any{
			"kind":    artifact.Kind,
			"summary": artifact.Summary,
		})
	}
	report["artifact_summaries"] = summaries
	return report
}

func executionResultClassFromReport(report map[string]any) string {
	for _, key := range []string{"execution_status", "compiled_execution"} {
		payload := payloadMap(report[key])
		if payload == nil {
			continue
		}
		if value := strings.TrimSpace(fmt.Sprint(payload["result_class"])); value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func assuranceClassFromReport(report map[string]any) string {
	for _, key := range []string{"success_gate", "execution_status", "compiled_execution", "proof_surface"} {
		payload := payloadMap(report[key])
		if payload == nil {
			continue
		}
		if value := strings.TrimSpace(fmt.Sprint(payload["assurance_class"])); value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func deferredIssueIDsFromReport(report map[string]any) []string {
	for _, key := range []string{"execution_status", "compiled_execution"} {
		payload := payloadMap(report[key])
		if payload == nil {
			continue
		}
		if values := stringSlice(payload["deferred_issue_ids"]); len(values) > 0 {
			return values
		}
	}
	if values := stringSlice(report["deferred_execution_issues"]); len(values) > 0 {
		return values
	}
	if payload := payloadMap(report["deferred_execution_issues"]); payload != nil {
		if values := stringSlice(payload["deferred_issue_ids"]); len(values) > 0 {
			return values
		}
	}
	if raw, ok := report["deferred_execution_issues"].([]any); ok {
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			if payload := payloadMap(item); payload != nil {
				if id := strings.TrimSpace(fmt.Sprint(payload["issue_id"])); id != "" && id != "<nil>" {
					out = append(out, id)
				}
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

// ValidateArtifactProvenance checks that all "produced" artifacts have a
// ProducerID set. Returns a list of warning strings for artifacts that are
// missing provenance.
func ValidateArtifactProvenance(artifacts []Artifact) []string {
	var warnings []string
	for _, a := range artifacts {
		if a.Status == "produced" && a.ProducerID == "" {
			warnings = append(warnings, fmt.Sprintf("artifact %s (kind=%s) missing ProducerID", a.ID, a.Kind))
		}
	}
	return warnings
}

// ============================================================================
// Execution Capabilities
// ============================================================================

// ExecutionStatus represents the outcome of a coding capability execution.
type ExecutionStatus string

const (
	ExecutionStatusCompleted ExecutionStatus = "completed"
	ExecutionStatusPartial   ExecutionStatus = "partial"
	ExecutionStatusFailed    ExecutionStatus = "failed"
)

// RecoveryStrategy identifies the level at which recovery should be attempted.
type RecoveryStrategy string

const (
	RecoveryStrategyParadigmSwitch     RecoveryStrategy = "paradigm_switch"
	RecoveryStrategyCapabilityFallback RecoveryStrategy = "capability_fallback"
	RecoveryStrategyProfileEscalation  RecoveryStrategy = "profile_escalation"
	RecoveryStrategyModeEscalation     RecoveryStrategy = "mode_escalation"
)

// ArtifactRequirement specifies a single artifact kind that a capability
// needs or optionally consumes.
type ArtifactRequirement struct {
	Kind     ArtifactKind
	Required bool
	MinCount int
}

// ArtifactContract declares the input/output artifact expectations
// for a coding capability.
type ArtifactContract struct {
	RequiredInputs  []ArtifactRequirement
	ProducedOutputs []ArtifactKind
	Predecessors    []string // capability IDs that must have completed
}

// SatisfiedBy returns true when every required input is present in the
// given artifact state with at least MinCount instances.
func (c ArtifactContract) SatisfiedBy(state ArtifactState) bool {
	for _, req := range c.RequiredInputs {
		if !req.Required {
			continue
		}
		count := len(state.OfKind(req.Kind))
		minCount := req.MinCount
		if minCount <= 0 {
			minCount = 1
		}
		if count < minCount {
			return false
		}
	}
	return true
}

// MissingInputs returns the artifact kinds whose requirements are not met
// by the given artifact state.
func (c ArtifactContract) MissingInputs(state ArtifactState) []ArtifactKind {
	var missing []ArtifactKind
	for _, req := range c.RequiredInputs {
		if !req.Required {
			continue
		}
		count := len(state.OfKind(req.Kind))
		minCount := req.MinCount
		if minCount <= 0 {
			minCount = 1
		}
		if count < minCount {
			missing = append(missing, req.Kind)
		}
	}
	return missing
}

// EligibilityResult captures whether a capability can execute in the
// current context.
type EligibilityResult struct {
	Eligible         bool
	Reason           string
	MissingArtifacts []ArtifactKind
}

// CapabilityFailure provides structured information about why a capability
// could not satisfy its contract.
type CapabilityFailure struct {
	Code            string
	Message         string
	Recoverable     bool
	FailedPhase     string
	MissingArtifact ArtifactKind
	ParadigmUsed    string
}

// RecoveryHint advises the profile controller on what recovery action
// to attempt after a capability failure.
type RecoveryHint struct {
	Strategy            RecoveryStrategy
	SuggestedCapability string
	SuggestedParadigm   string
	Context             map[string]any
}

// ExecutionEnvelope carries all runtime context needed by a coding
// capability during execution.
type ExecutionEnvelope struct {
	Task          *core.Task
	Mode          ModeResolution
	Profile       ExecutionProfileSelection
	Registry      *capability.Registry
	State         *core.Context
	Memory        memory.MemoryStore
	Environment   agentenv.AgentEnvironment
	WorkflowStore WorkflowArtifactWriter
	WorkflowID    string
	RunID         string
	Telemetry     core.Telemetry
}

// ExecutionResult is the structured return value from a coding capability.
type ExecutionResult struct {
	Artifacts    []Artifact
	Status       ExecutionStatus
	FailureInfo  *CapabilityFailure
	RecoveryHint *RecoveryHint
	Summary      string
}

// EucloCodingCapability is the concrete runtime interface for Euclo's
// coding-specific relurpic capabilities. Implementations own multi-paradigm
// composition, artifact contracts, and structured failure reporting.
//
// Each implementation registers itself in the framework capability registry
// via its Descriptor(), getting the full policy/safety/exposure/telemetry
// treatment. Internally it has access to the full ExecutionEnvelope.
type EucloCodingCapability interface {
	// Descriptor returns the framework capability descriptor used for
	// registration in the capability registry.
	Descriptor() core.CapabilityDescriptor

	// Contract declares what artifacts this capability requires and produces.
	Contract() ArtifactContract

	// Eligible checks whether the capability can execute given the current
	// artifact state and capability snapshot.
	Eligible(artifacts ArtifactState, snapshot CapabilitySnapshot) EligibilityResult

	// Execute runs the capability and returns a structured result with
	// artifacts, status, and optional recovery hints.
	Execute(ctx context.Context, env ExecutionEnvelope) ExecutionResult
}

// ArtifactState provides typed access over a slice of Artifact values.
type ArtifactState struct {
	artifacts []Artifact
}

// NewArtifactState creates an ArtifactState from a slice of artifacts.
func NewArtifactState(artifacts []Artifact) ArtifactState {
	return ArtifactState{artifacts: artifacts}
}

// Has returns true if at least one artifact of the given kind exists.
func (s ArtifactState) Has(kind ArtifactKind) bool {
	for _, a := range s.artifacts {
		if a.Kind == kind {
			return true
		}
	}
	return false
}

// OfKind returns all artifacts matching the given kind.
func (s ArtifactState) OfKind(kind ArtifactKind) []Artifact {
	var out []Artifact
	for _, a := range s.artifacts {
		if a.Kind == kind {
			out = append(out, a)
		}
	}
	return out
}

// All returns the full artifact slice.
func (s ArtifactState) All() []Artifact {
	return s.artifacts
}

// Len returns the number of artifacts.
func (s ArtifactState) Len() int {
	return len(s.artifacts)
}

// ArtifactStateFromContext builds an ArtifactState from the euclo.artifacts
// key in state, if present.
func ArtifactStateFromContext(state *core.Context) ArtifactState {
	if state == nil {
		return ArtifactState{}
	}
	raw, ok := state.Get("euclo.artifacts")
	if !ok || raw == nil {
		return ArtifactState{}
	}
	artifacts, ok := raw.([]Artifact)
	if !ok {
		return ArtifactState{}
	}
	return NewArtifactState(artifacts)
}

// ============================================================================
// Modes
// ============================================================================

type ModeDescriptor struct {
	ModeID                      string
	IntentFamily                string
	EditPolicy                  string
	EvidencePolicy              string
	VerificationPolicy          string
	ReviewPolicy                string
	DefaultExecutionProfiles    []string
	FallbackExecutionProfiles   []string
	PreferredCapabilityFamilies []string
	ContextStrategy             string
	RecoveryPolicy              string
	ReportingPolicy             string
}

type ModeRegistry struct {
	descriptors map[string]ModeDescriptor
}

type ModeResolution struct {
	ModeID      string   `json:"mode_id"`
	Source      string   `json:"source"`
	ReasonCodes []string `json:"reason_codes,omitempty"`
	Constraints []string `json:"constraints,omitempty"`
}

func NewModeRegistry() *ModeRegistry {
	return &ModeRegistry{descriptors: map[string]ModeDescriptor{}}
}

func DefaultModeRegistry() *ModeRegistry {
	registry := NewModeRegistry()
	for _, descriptor := range []ModeDescriptor{
		{
			ModeID:                      "code",
			IntentFamily:                "implementation",
			EditPolicy:                  "allowed",
			EvidencePolicy:              "local_evidence_before_edit",
			VerificationPolicy:          "required",
			ReviewPolicy:                "secondary",
			DefaultExecutionProfiles:    []string{"edit_verify_repair"},
			FallbackExecutionProfiles:   []string{"reproduce_localize_patch", "plan_stage_execute", "review_suggest_implement"},
			PreferredCapabilityFamilies: []string{"bounded_implementation", "verification", "targeted_planning"},
			ContextStrategy:             "narrow_to_wide",
			RecoveryPolicy:              "repair_then_escalate",
			ReportingPolicy:             "artifact_summary",
		},
		{
			ModeID:                      "debug",
			IntentFamily:                "debugging",
			EditPolicy:                  "delayed",
			EvidencePolicy:              "reproduction_or_localization_required",
			VerificationPolicy:          "rerun_relevant_failure_required",
			ReviewPolicy:                "secondary",
			DefaultExecutionProfiles:    []string{"reproduce_localize_patch"},
			FallbackExecutionProfiles:   []string{"trace_execute_analyze", "edit_verify_repair", "plan_stage_execute"},
			PreferredCapabilityFamilies: []string{"debugging", "tracing", "diagnostics"},
			ContextStrategy:             "localize_then_expand",
			RecoveryPolicy:              "gather_more_evidence",
			ReportingPolicy:             "root_cause_first",
		},
		{
			ModeID:                      "tdd",
			IntentFamily:                "test_driven_development",
			EditPolicy:                  "allowed",
			EvidencePolicy:              "test_artifact_first",
			VerificationPolicy:          "tests_required",
			ReviewPolicy:                "secondary",
			DefaultExecutionProfiles:    []string{"test_driven_generation"},
			FallbackExecutionProfiles:   []string{"edit_verify_repair", "plan_stage_execute"},
			PreferredCapabilityFamilies: []string{"test_generation", "implementation", "verification"},
			ContextStrategy:             "targeted",
			RecoveryPolicy:              "failing_test_driven",
			ReportingPolicy:             "test_and_patch_summary",
		},
		{
			ModeID:                      "review",
			IntentFamily:                "review",
			EditPolicy:                  "disallowed",
			EvidencePolicy:              "evidence_first",
			VerificationPolicy:          "optional",
			ReviewPolicy:                "primary",
			DefaultExecutionProfiles:    []string{"review_suggest_implement"},
			FallbackExecutionProfiles:   []string{"plan_stage_execute"},
			PreferredCapabilityFamilies: []string{"review", "analysis"},
			ContextStrategy:             "read_heavy",
			RecoveryPolicy:              "request_clarification",
			ReportingPolicy:             "findings_first",
		},
		{
			ModeID:                      "planning",
			IntentFamily:                "planning",
			EditPolicy:                  "disallowed",
			EvidencePolicy:              "context_collection",
			VerificationPolicy:          "optional",
			ReviewPolicy:                "secondary",
			DefaultExecutionProfiles:    []string{"plan_stage_execute"},
			FallbackExecutionProfiles:   []string{"review_suggest_implement"},
			PreferredCapabilityFamilies: []string{"planning", "analysis"},
			ContextStrategy:             "expand_carefully",
			RecoveryPolicy:              "clarify_scope",
			ReportingPolicy:             "plan_summary",
		},
	} {
		_ = registry.Register(descriptor)
	}
	return registry
}

func (r *ModeRegistry) Register(descriptor ModeDescriptor) error {
	if r == nil {
		return fmt.Errorf("mode registry unavailable")
	}
	id := strings.TrimSpace(strings.ToLower(descriptor.ModeID))
	if id == "" {
		return fmt.Errorf("mode id required")
	}
	descriptor.ModeID = id
	if len(descriptor.DefaultExecutionProfiles) == 0 {
		return fmt.Errorf("mode %s requires at least one default execution profile", id)
	}
	r.descriptors[id] = descriptor
	return nil
}

func (r *ModeRegistry) Lookup(modeID string) (ModeDescriptor, bool) {
	if r == nil {
		return ModeDescriptor{}, false
	}
	descriptor, ok := r.descriptors[strings.TrimSpace(strings.ToLower(modeID))]
	return descriptor, ok
}

func (r *ModeRegistry) List() []ModeDescriptor {
	if r == nil {
		return nil
	}
	keys := make([]string, 0, len(r.descriptors))
	for key := range r.descriptors {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]ModeDescriptor, 0, len(keys))
	for _, key := range keys {
		out = append(out, r.descriptors[key])
	}
	return out
}

// ============================================================================
// Execution Profiles
// ============================================================================

type ExecutionProfileDescriptor struct {
	ProfileID            string
	SupportedModes       []string
	FallbackProfiles     []string
	RequiredArtifacts    []string
	CompletionContract   string
	PhaseRoutes          map[string]string
	MutationPolicy       string
	VerificationRequired bool
}

type ExecutionProfileRegistry struct {
	descriptors map[string]ExecutionProfileDescriptor
}

type ExecutionProfileSelection struct {
	ProfileID            string            `json:"profile_id"`
	FallbackProfileIDs   []string          `json:"fallback_profile_ids,omitempty"`
	RequiredArtifacts    []string          `json:"required_artifacts,omitempty"`
	CompletionContract   string            `json:"completion_contract,omitempty"`
	PhaseRoutes          map[string]string `json:"phase_routes,omitempty"`
	ReasonCodes          []string          `json:"reason_codes,omitempty"`
	MutationAllowed      bool              `json:"mutation_allowed"`
	VerificationRequired bool              `json:"verification_required"`
}

func NewExecutionProfileRegistry() *ExecutionProfileRegistry {
	return &ExecutionProfileRegistry{descriptors: map[string]ExecutionProfileDescriptor{}}
}

func DefaultExecutionProfileRegistry() *ExecutionProfileRegistry {
	registry := NewExecutionProfileRegistry()
	for _, descriptor := range []ExecutionProfileDescriptor{
		{
			ProfileID:            "edit_verify_repair",
			SupportedModes:       []string{"code", "debug", "tdd"},
			FallbackProfiles:     []string{"reproduce_localize_patch", "plan_stage_execute"},
			RequiredArtifacts:    []string{"euclo.intake", "euclo.classification", "euclo.verification"},
			CompletionContract:   "edits_planned_and_verification_recorded",
			PhaseRoutes:          map[string]string{"explore": "react", "plan": "pipeline", "edit": "pipeline", "verify": "react"},
			MutationPolicy:       "allowed",
			VerificationRequired: true,
		},
		{
			ProfileID:            "reproduce_localize_patch",
			SupportedModes:       []string{"debug", "code"},
			FallbackProfiles:     []string{"trace_execute_analyze", "edit_verify_repair"},
			RequiredArtifacts:    []string{"euclo.intake", "euclo.classification", "euclo.verification"},
			CompletionContract:   "reproduction_or_localization_before_patch",
			PhaseRoutes:          map[string]string{"reproduce": "react", "localize": "react", "patch": "pipeline", "verify": "react"},
			MutationPolicy:       "delayed",
			VerificationRequired: true,
		},
		{
			ProfileID:            "test_driven_generation",
			SupportedModes:       []string{"tdd", "code"},
			FallbackProfiles:     []string{"edit_verify_repair"},
			RequiredArtifacts:    []string{"euclo.intake", "euclo.classification", "euclo.verification"},
			CompletionContract:   "tests_or_failures_recorded_before_completion",
			PhaseRoutes:          map[string]string{"plan_tests": "planner", "implement": "pipeline", "verify": "react"},
			MutationPolicy:       "allowed",
			VerificationRequired: true,
		},
		{
			ProfileID:            "review_suggest_implement",
			SupportedModes:       []string{"review", "planning", "code"},
			FallbackProfiles:     []string{"plan_stage_execute"},
			RequiredArtifacts:    []string{"euclo.intake", "euclo.classification"},
			CompletionContract:   "review_findings_or_change_plan_produced",
			PhaseRoutes:          map[string]string{"review": "reflection", "summarize": "react"},
			MutationPolicy:       "disallowed",
			VerificationRequired: false,
		},
		{
			ProfileID:            "plan_stage_execute",
			SupportedModes:       []string{"planning", "code", "review", "debug"},
			FallbackProfiles:     []string{"review_suggest_implement"},
			RequiredArtifacts:    []string{"euclo.intake", "euclo.classification"},
			CompletionContract:   "plan_or_staged_strategy_produced",
			PhaseRoutes:          map[string]string{"plan": "planner", "stage": "pipeline", "summarize": "react"},
			MutationPolicy:       "disallowed",
			VerificationRequired: false,
		},
		{
			ProfileID:            "trace_execute_analyze",
			SupportedModes:       []string{"debug"},
			FallbackProfiles:     []string{"reproduce_localize_patch"},
			RequiredArtifacts:    []string{"euclo.intake", "euclo.classification"},
			CompletionContract:   "trace_or_diagnostic_evidence_produced",
			PhaseRoutes:          map[string]string{"trace": "react", "analyze": "reflection"},
			MutationPolicy:       "disallowed",
			VerificationRequired: false,
		},
		{
			ProfileID:            "chat_ask_respond",
			SupportedModes:       []string{"chat"},
			FallbackProfiles:     []string{"plan_stage_execute"},
			RequiredArtifacts:    []string{"euclo.intake", "euclo.classification"},
			CompletionContract:   "conversational_response_produced",
			PhaseRoutes:          map[string]string{"answer": "react", "summarize": "react"},
			MutationPolicy:       "disallowed",
			VerificationRequired: false,
		},
	} {
		_ = registry.Register(descriptor)
	}
	return registry
}

func (r *ExecutionProfileRegistry) Register(descriptor ExecutionProfileDescriptor) error {
	if r == nil {
		return fmt.Errorf("execution profile registry unavailable")
	}
	id := strings.TrimSpace(strings.ToLower(descriptor.ProfileID))
	if id == "" {
		return fmt.Errorf("profile id required")
	}
	descriptor.ProfileID = id
	if len(descriptor.SupportedModes) == 0 {
		return fmt.Errorf("profile %s requires supported modes", id)
	}
	r.descriptors[id] = descriptor
	return nil
}

func (r *ExecutionProfileRegistry) Lookup(profileID string) (ExecutionProfileDescriptor, bool) {
	if r == nil {
		return ExecutionProfileDescriptor{}, false
	}
	descriptor, ok := r.descriptors[strings.TrimSpace(strings.ToLower(profileID))]
	return descriptor, ok
}

func (r *ExecutionProfileRegistry) List() []ExecutionProfileDescriptor {
	if r == nil {
		return nil
	}
	keys := make([]string, 0, len(r.descriptors))
	for key := range r.descriptors {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]ExecutionProfileDescriptor, 0, len(keys))
	for _, key := range keys {
		out = append(out, r.descriptors[key])
	}
	return out
}

// ============================================================================
// Capability Snapshot
// ============================================================================

type CapabilitySnapshot struct {
	ToolNames            []string `json:"tool_names,omitempty"`
	HasReadTools         bool     `json:"has_read_tools"`
	HasWriteTools        bool     `json:"has_write_tools"`
	HasExecuteTools      bool     `json:"has_execute_tools"`
	HasNetworkTools      bool     `json:"has_network_tools"`
	HasVerificationTools bool     `json:"has_verification_tools"`
	HasASTOrLSPTools     bool     `json:"has_ast_or_lsp_tools"`
}

// ============================================================================
// Private Helpers
// ============================================================================

func normalizeWorkflowRetrieval(raw any) (any, map[string]any) {
	switch typed := raw.(type) {
	case map[string]any:
		metadata := map[string]any{}
		for _, key := range []string{"query", "scope", "cache_tier", "query_id", "citation_count", "result_size"} {
			if value, ok := typed[key]; ok && value != nil {
				metadata[key] = value
			}
		}
		return cloneMap(typed), metadata
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return map[string]any{}, nil
		}
		return map[string]any{"summary": text, "texts": []string{text}}, map[string]any{"result_size": 1}
	default:
		text := strings.TrimSpace(fmt.Sprint(raw))
		if text == "" || text == "<nil>" {
			return map[string]any{}, nil
		}
		return map[string]any{"summary": text}, nil
	}
}

func normalizeEditIntent(raw any) (any, map[string]any) {
	payload := raw
	metadata := map[string]any{"intent_only": true}
	switch typed := raw.(type) {
	case map[string]any:
		payload = cloneMap(typed)
		if value, ok := typed["intent_only"]; ok {
			metadata["intent_only"] = value
		}
	}
	return payload, metadata
}

func payloadMap(raw any) map[string]any {
	if raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case map[string]any:
		return typed
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			out[key] = value
		}
		return out
	default:
		blob, err := json.Marshal(raw)
		if err != nil {
			return nil
		}
		var out map[string]any
		if err := json.Unmarshal(blob, &out); err != nil {
			return nil
		}
		return out
	}
}

func normalizeRuntimeCompiledExecution(raw any) (map[string]any, bool) {
	value := payloadMap(raw)
	if len(value) == 0 {
		return nil, false
	}
	if value["workflow_id"] == nil && value["execution_id"] == nil && value["unit_of_work_id"] == nil {
		return nil, false
	}
	return value, true
}

func stringSlice(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			value := strings.TrimSpace(fmt.Sprint(item))
			if value == "" || value == "<nil>" {
				continue
			}
			out = append(out, value)
		}
		return out
	default:
		return nil
	}
}

func artifactSummary(raw any) string {
	switch typed := raw.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		for _, key := range []string{"summary", "text", "status", "strategy"} {
			if text := strings.TrimSpace(fmt.Sprint(typed[key])); text != "" && text != "<nil>" {
				return text
			}
		}
		if texts, ok := typed["texts"].([]string); ok && len(texts) > 0 {
			return strings.TrimSpace(texts[0])
		}
	}
	if marshaled, err := json.Marshal(raw); err == nil {
		text := string(marshaled)
		if len(text) > 240 {
			return text[:240]
		}
		return text
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func cloneMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out[key] = input[key]
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func decodeArtifactPayload(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}
	}
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err == nil {
		return payload
	}
	return raw
}

func StateKeyForArtifactKind(kind ArtifactKind) string {
	switch kind {
	case ArtifactKindIntake:
		return "euclo.envelope"
	case ArtifactKindClassification:
		return "euclo.classification"
	case ArtifactKindModeResolution:
		return "euclo.mode_resolution"
	case ArtifactKindExecutionProfile:
		return "euclo.execution_profile_selection"
	case ArtifactKindRetrievalPolicy:
		return "euclo.retrieval_policy"
	case ArtifactKindContextExpansion:
		return "euclo.context_expansion"
	case ArtifactKindCapabilityRouting:
		return "euclo.capability_family_routing"
	case ArtifactKindVerificationPolicy:
		return "euclo.verification_policy"
	case ArtifactKindSuccessGate:
		return "euclo.success_gate"
	case ArtifactKindActionLog:
		return "euclo.action_log"
	case ArtifactKindProofSurface:
		return "euclo.proof_surface"
	case ArtifactKindWorkflowRetrieval:
		return "pipeline.workflow_retrieval"
	case ArtifactKindExplore:
		return "pipeline.explore"
	case ArtifactKindTrace:
		return "euclo.trace"
	case ArtifactKindAnalyze:
		return "pipeline.analyze"
	case ArtifactKindReviewFindings:
		return "euclo.review_findings"
	case ArtifactKindCompatibilityAssessment:
		return "euclo.compatibility_assessment"
	case ArtifactKindPlan:
		return "pipeline.plan"
	case ArtifactKindMigrationPlan:
		return "euclo.migration_plan"
	case ArtifactKindPlanCandidates:
		return "euclo.plan_candidates"
	case ArtifactKindEditIntent:
		return "pipeline.code"
	case ArtifactKindEditExecution:
		return "euclo.edit_execution"
	case ArtifactKindVerificationPlan:
		return "euclo.verification_plan"
	case ArtifactKindVerification:
		return "pipeline.verify"
	case ArtifactKindTDDLifecycle:
		return "euclo.tdd.lifecycle"
	case ArtifactKindWaiver:
		return "euclo.waiver"
	case ArtifactKindDiffSummary:
		return "euclo.diff_summary"
	case ArtifactKindVerificationSummary:
		return "euclo.verification_summary"
	case ArtifactKindProfileSelection:
		return "euclo.profile_selection"
	case ArtifactKindReproduction:
		return "euclo.reproduction"
	case ArtifactKindRootCause:
		return "euclo.root_cause"
	case ArtifactKindRootCauseCandidates:
		return "euclo.root_cause_candidates"
	case ArtifactKindRegressionAnalysis:
		return "euclo.regression_analysis"
	case ArtifactKindCompiledExecution:
		return "euclo.compiled_execution"
	case ArtifactKindExecutionStatus:
		return "euclo.execution_status"
	case ArtifactKindDeferredExecutionIssues:
		return "euclo.deferred_execution_issues"
	case ArtifactKindContextCompaction:
		return "euclo.context_compaction"
	case ArtifactKindFinalReport:
		return "pipeline.final_output"
	case ArtifactKindRecoveryTrace:
		return "euclo.recovery_trace"
	default:
		return ""
	}
}

func firstArtifactOfKind(artifacts []Artifact, kind ArtifactKind) (Artifact, bool) {
	for _, artifact := range artifacts {
		if artifact.Kind == kind {
			return artifact, true
		}
	}
	return Artifact{}, false
}
