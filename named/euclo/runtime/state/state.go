package state

import (
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	runtimepkg "github.com/lexcodex/relurpify/named/euclo/runtime"
)

// RecoveryTrace represents the recovery trace state with typed fields.
// This replaces the raw map[string]any used for recovery_trace.
type RecoveryTrace struct {
	Status       string `json:"status"`
	AttemptCount int    `json:"attempt_count"`
	MaxAttempts  int    `json:"max_attempts"`
	Reason       string `json:"reason,omitempty"`
}

// EucloExecutionState is a struct with typed fields for all euclo state.
// It provides a typed overlay over the untyped *core.Context state bus.
type EucloExecutionState struct {
	// Verification and assurance state
	VerificationPolicy runtimepkg.VerificationPolicy
	Verification       runtimepkg.VerificationEvidence
	SuccessGate        runtimepkg.SuccessGateResult
	AssuranceClass     runtimepkg.AssuranceClass
	ExecutionWaiver    runtimepkg.ExecutionWaiver
	Waiver             any // Raw waiver data from state
	RecoveryTrace      RecoveryTrace

	// Behavior and execution state
	BehaviorTrace Trace
	Artifacts     []euclotypes.Artifact
	ActionLog     []runtimepkg.ActionLogEntry
	ProofSurface  runtimepkg.ProofSurface
	FinalReport   map[string]any

	// Runtime state
	ContextRuntime               runtimepkg.ContextRuntimeState
	SecurityRuntime              runtimepkg.SecurityRuntimeState
	SharedContextRuntime         runtimepkg.SharedContextRuntimeState
	CapabilityContractRuntime    runtimepkg.CapabilityContractRuntimeState
	ArchaeologyCapabilityRuntime runtimepkg.ArchaeologyCapabilityRuntimeState
	DebugCapabilityRuntime       runtimepkg.DebugCapabilityRuntimeState
	ChatCapabilityRuntime        runtimepkg.ChatCapabilityRuntimeState
	ExecutorRuntime              runtimepkg.ExecutorRuntimeState

	// Provider and restore state
	ProviderRestore any

	// Summary and findings state
	VerificationSummary map[string]any
	ReviewFindings      map[string]any
	RootCause           map[string]any
	RootCauseCandidates map[string]any
	RegressionAnalysis  map[string]any
	PlanCandidates      map[string]any

	// Edit and intent state
	EditExecution runtimepkg.EditExecutionRecord
	EditIntent    map[string]any

	// Classification and routing state
	WorkflowID                      string
	ClassificationSource            string
	ClassificationMeta              string
	PreClassifiedCapabilitySequence []string
	CapabilitySequenceOperator      string
	UserRecipeSignals               []runtimepkg.UserRecipeSignalSource

	// Policy state
	RetrievalPolicy  runtimepkg.RetrievalPolicy
	ContextLifecycle runtimepkg.ContextLifecycleState
	SessionID        string

	// Deferred and interaction state
	DeferredIssues                 []runtimepkg.DeferredExecutionIssue
	DeferralPlan                   any
	InteractionScript              any
	RequiresEvidenceBeforeMutation bool

	// Session resume state
	SessionResumeContext  any
	ArchaeoPhaseState     any
	CodeRevision          string
	ResumeSemanticContext any

	// Unit of work state
	UnitOfWork           runtimepkg.UnitOfWork
	UnitOfWorkID         string
	RootUnitOfWorkID     string
	UnitOfWorkHistory    []runtimepkg.UnitOfWorkHistoryEntry
	UnitOfWorkTransition runtimepkg.UnitOfWorkTransitionState

	// Envelope and classification state
	Envelope                  runtimepkg.TaskEnvelope
	Classification            runtimepkg.TaskClassification
	ModeResolution            runtimepkg.ModeResolution
	ExecutionProfileSelection runtimepkg.ExecutionProfileSelection
	Mode                      string
	ExecutionProfile          string
	SemanticInputs            runtimepkg.SemanticInputBundle
	ResolvedExecutionPolicy   runtimepkg.ResolvedExecutionPolicy
	ExecutorDescriptor        runtimepkg.WorkUnitExecutorDescriptor

	// Execution status state
	ExecutionStatus   runtimepkg.RuntimeExecutionStatus
	CompiledExecution runtimepkg.CompiledExecution

	// Pipeline state
	PipelineExplore           map[string]any
	PipelineAnalyze           map[string]any
	PipelinePlan              map[string]any
	PipelineCode              map[string]any
	PipelineVerify            map[string]any
	PipelineFinalOutput       map[string]any
	PipelineWorkflowRetrieval map[string]any
}

// Trace represents the behavior trace state with typed fields.
// This replaces the raw map[string]any used for behavior trace.
// Fields must match execution.Trace for compatibility.
type Trace struct {
	PrimaryCapabilityID      string   `json:"primary_capability_id,omitempty"`
	SupportingRoutines       []string `json:"supporting_routines,omitempty"`
	RecipeIDs                []string `json:"recipe_ids,omitempty"`
	SpecializedCapabilityIDs []string `json:"specialized_capability_ids,omitempty"`
	ExecutorFamily           string   `json:"executor_family,omitempty"`
	Path                     string   `json:"path,omitempty"`
}

// NewEucloExecutionState creates a new empty EucloExecutionState.
func NewEucloExecutionState() *EucloExecutionState {
	return &EucloExecutionState{}
}

// IsZero returns true if the state has no non-zero values.
func (s *EucloExecutionState) IsZero() bool {
	if s == nil {
		return true
	}
	// Check a representative set of key fields
	return s.VerificationPolicy.PolicyID == "" &&
		s.WorkflowID == "" &&
		s.UnitOfWork.ID == "" &&
		s.Mode == "" &&
		s.AssuranceClass == ""
}

// touch updates the timestamps on any state that tracks modification time.
// This should be called before FlushToContext to ensure timestamps are current.
func (s *EucloExecutionState) touch() {
	now := time.Now().UTC()

	if !s.ContextRuntime.UpdatedAt.IsZero() {
		s.ContextRuntime.UpdatedAt = now
	}
	if !s.SecurityRuntime.UpdatedAt.IsZero() {
		s.SecurityRuntime.UpdatedAt = now
	}
	if !s.SharedContextRuntime.UpdatedAt.IsZero() {
		s.SharedContextRuntime.UpdatedAt = now
	}
	if !s.CapabilityContractRuntime.UpdatedAt.IsZero() {
		s.CapabilityContractRuntime.UpdatedAt = now
	}
	if !s.ArchaeologyCapabilityRuntime.UpdatedAt.IsZero() {
		s.ArchaeologyCapabilityRuntime.UpdatedAt = now
	}
	if !s.DebugCapabilityRuntime.UpdatedAt.IsZero() {
		s.DebugCapabilityRuntime.UpdatedAt = now
	}
	if !s.ChatCapabilityRuntime.UpdatedAt.IsZero() {
		s.ChatCapabilityRuntime.UpdatedAt = now
	}
}

// GetString safely retrieves a string value from core.Context.
// Returns empty string and false if key not found or not a string.
func GetString(ctx *core.Context, key string) (string, bool) {
	if ctx == nil {
		return "", false
	}
	if raw, ok := ctx.Get(key); ok && raw != nil {
		if s, ok := raw.(string); ok {
			return s, true
		}
	}
	return "", false
}

// SetString sets a string value in core.Context.
func SetString(ctx *core.Context, key string, value string) {
	if ctx == nil {
		return
	}
	ctx.Set(key, value)
}

// GetBool safely retrieves a bool value from core.Context.
// Returns false and false if key not found or not a bool.
func GetBool(ctx *core.Context, key string) (bool, bool) {
	if ctx == nil {
		return false, false
	}
	if raw, ok := ctx.Get(key); ok && raw != nil {
		if b, ok := raw.(bool); ok {
			return b, true
		}
	}
	return false, false
}

// SetBool sets a bool value in core.Context.
func SetBool(ctx *core.Context, key string, value bool) {
	if ctx == nil {
		return
	}
	ctx.Set(key, value)
}
