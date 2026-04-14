package runtime

import (
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

// TaskEnvelope is the typed Euclo intake shape used to normalize coding
// requests before routing deeper into the runtime.
type TaskEnvelope struct {
	TaskID                string                        `json:"task_id,omitempty"`
	Instruction           string                        `json:"instruction,omitempty"`
	Workspace             string                        `json:"workspace,omitempty"`
	ModeHint              string                        `json:"mode_hint,omitempty"`
	ResumedMode           string                        `json:"resumed_mode,omitempty"`
	ExplicitVerification  string                        `json:"explicit_verification,omitempty"`
	EditPermitted         bool                          `json:"edit_permitted"`
	CapabilitySnapshot    euclotypes.CapabilitySnapshot `json:"capability_snapshot"`
	PreviousArtifactKinds []string                      `json:"previous_artifact_kinds,omitempty"`
	ResolvedMode          string                        `json:"resolved_mode,omitempty"`
	ExecutionProfile      string                        `json:"execution_profile,omitempty"`
}

type TaskClassification struct {
	IntentFamilies                 []string      `json:"intent_families,omitempty"`
	RecommendedMode                string        `json:"recommended_mode,omitempty"`
	MixedIntent                    bool          `json:"mixed_intent"`
	EditPermitted                  bool          `json:"edit_permitted"`
	RequiresEvidenceBeforeMutation bool          `json:"requires_evidence_before_mutation"`
	RequiresDeterministicStages    bool          `json:"requires_deterministic_stages"`
	Scope                          string        `json:"scope,omitempty"`
	RiskLevel                      string        `json:"risk_level,omitempty"`
	ReasonCodes                    []string      `json:"reason_codes,omitempty"`
	TaskType                       core.TaskType `json:"task_type,omitempty"`
}

// ExecutionResultClass is Euclo's top-level runtime classification for a work
// unit or run. It is intentionally separate from euclotypes.ExecutionStatus,
// which remains the capability-level outcome type.
type ExecutionResultClass string

const (
	ExecutionResultClassCompleted              ExecutionResultClass = "completed"
	ExecutionResultClassCompletedWithDeferrals ExecutionResultClass = "completed_with_deferrals"
	ExecutionResultClassBlocked                ExecutionResultClass = "blocked"
	ExecutionResultClassFailed                 ExecutionResultClass = "failed"
	ExecutionResultClassCanceled               ExecutionResultClass = "canceled"
	ExecutionResultClassRestoreFailed          ExecutionResultClass = "restore_failed"
)

type AssuranceClass string

const (
	AssuranceClassVerifiedSuccess   AssuranceClass = "verified_success"
	AssuranceClassPartiallyVerified AssuranceClass = "partially_verified"
	AssuranceClassUnverifiedSuccess AssuranceClass = "unverified_success"
	AssuranceClassReviewBlocked     AssuranceClass = "review_blocked"
	AssuranceClassRepairExhausted   AssuranceClass = "repair_exhausted"
	AssuranceClassTDDIncomplete     AssuranceClass = "tdd_incomplete"
	AssuranceClassOperatorDeferred  AssuranceClass = "operator_deferred"
)

// ExecutionStatus is Euclo's runtime execution-state type. It models the
// current lifecycle of a work unit rather than a single capability outcome.
type ExecutionStatus string

const (
	ExecutionStatusPreparing              ExecutionStatus = "preparing"
	ExecutionStatusReady                  ExecutionStatus = "ready"
	ExecutionStatusExecuting              ExecutionStatus = "executing"
	ExecutionStatusVerifying              ExecutionStatus = "verifying"
	ExecutionStatusSurfacing              ExecutionStatus = "surfacing"
	ExecutionStatusCompacted              ExecutionStatus = "compacted"
	ExecutionStatusRestoring              ExecutionStatus = "restoring"
	ExecutionStatusCompleted              ExecutionStatus = "completed"
	ExecutionStatusCompletedWithDeferrals ExecutionStatus = "completed_with_deferrals"
	ExecutionStatusBlocked                ExecutionStatus = "blocked"
	ExecutionStatusFailed                 ExecutionStatus = "failed"
	ExecutionStatusCanceled               ExecutionStatus = "canceled"
	ExecutionStatusRestoreFailed          ExecutionStatus = "restore_failed"
)

type DeferredIssueKind string

const (
	DeferredIssueAmbiguity           DeferredIssueKind = "ambiguity"
	DeferredIssueStaleAssumption     DeferredIssueKind = "stale_assumption"
	DeferredIssuePatternTension      DeferredIssueKind = "pattern_tension"
	DeferredIssueNonfatalFailure     DeferredIssueKind = "nonfatal_failure"
	DeferredIssueVerificationConcern DeferredIssueKind = "verification_concern"
	DeferredIssueProviderConstraint  DeferredIssueKind = "provider_constraint"
	DeferredIssueOperatorDeferred    DeferredIssueKind = "operator_deferred"
	DeferredIssueWaiver              DeferredIssueKind = "waiver"
)

type DeferredIssueSeverity string

const (
	DeferredIssueSeverityLow      DeferredIssueSeverity = "low"
	DeferredIssueSeverityMedium   DeferredIssueSeverity = "medium"
	DeferredIssueSeverityHigh     DeferredIssueSeverity = "high"
	DeferredIssueSeverityCritical DeferredIssueSeverity = "critical"
)

type DeferredIssueStatus string

const (
	DeferredIssueStatusOpen                 DeferredIssueStatus = "open"
	DeferredIssueStatusAcknowledged         DeferredIssueStatus = "acknowledged"
	DeferredIssueStatusResolved             DeferredIssueStatus = "resolved"
	DeferredIssueStatusIgnored              DeferredIssueStatus = "ignored"
	DeferredIssueStatusSuperseded           DeferredIssueStatus = "superseded"
	DeferredIssueStatusReenteredArchaeology DeferredIssueStatus = "reentered_archaeology"
)

type UnitOfWorkStatus string

const (
	UnitOfWorkStatusAssembling             UnitOfWorkStatus = "assembling"
	UnitOfWorkStatusReady                  UnitOfWorkStatus = "ready"
	UnitOfWorkStatusExecuting              UnitOfWorkStatus = "executing"
	UnitOfWorkStatusVerifying              UnitOfWorkStatus = "verifying"
	UnitOfWorkStatusCompacted              UnitOfWorkStatus = "compacted"
	UnitOfWorkStatusRestoring              UnitOfWorkStatus = "restoring"
	UnitOfWorkStatusCompleted              UnitOfWorkStatus = "completed"
	UnitOfWorkStatusCompletedWithDeferrals UnitOfWorkStatus = "completed_with_deferrals"
	UnitOfWorkStatusBlocked                UnitOfWorkStatus = "blocked"
	UnitOfWorkStatusFailed                 UnitOfWorkStatus = "failed"
	UnitOfWorkStatusCanceled               UnitOfWorkStatus = "canceled"
)

type ContextLifecycleStage string

const (
	ContextLifecycleStageActive        ContextLifecycleStage = "active"
	ContextLifecycleStageCompacted     ContextLifecycleStage = "compacted"
	ContextLifecycleStageRestoring     ContextLifecycleStage = "restoring"
	ContextLifecycleStageRestored      ContextLifecycleStage = "restored"
	ContextLifecycleStageRestoreFailed ContextLifecycleStage = "restore_failed"
)

type ExecutorFamily string

const (
	ExecutorFamilyReact      ExecutorFamily = "react_executor"
	ExecutorFamilyPlanner    ExecutorFamily = "planner_executor"
	ExecutorFamilyHTN        ExecutorFamily = "htn_executor"
	ExecutorFamilyRewoo      ExecutorFamily = "rewoo_executor"
	ExecutorFamilyReflection ExecutorFamily = "reflection_executor"
	// NEW: Blackboard executor for hypothesis-driven workflows with shared context
	ExecutorFamilyBlackboard ExecutorFamily = "blackboard_executor"
)

type SemanticRequestRef struct {
	RequestID string `json:"request_id,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Status    string `json:"status,omitempty"`
	Title     string `json:"title,omitempty"`
}

type SemanticFindingSummary struct {
	RefID       string   `json:"ref_id,omitempty"`
	Kind        string   `json:"kind,omitempty"`
	Status      string   `json:"status,omitempty"`
	Title       string   `json:"title,omitempty"`
	Summary     string   `json:"summary,omitempty"`
	RelatedRefs []string `json:"related_refs,omitempty"`
}

type PatternProposalSummary struct {
	ProposalID          string   `json:"proposal_id,omitempty"`
	Title               string   `json:"title,omitempty"`
	Summary             string   `json:"summary,omitempty"`
	PatternRefs         []string `json:"pattern_refs,omitempty"`
	RelatedTensionRefs  []string `json:"related_tension_refs,omitempty"`
	SupportingRefs      []string `json:"supporting_refs,omitempty"`
	RecommendedFollowup string   `json:"recommended_followup,omitempty"`
}

type TensionClusterSummary struct {
	ClusterID          string   `json:"cluster_id,omitempty"`
	Title              string   `json:"title,omitempty"`
	Summary            string   `json:"summary,omitempty"`
	Severity           string   `json:"severity,omitempty"`
	TensionRefs        []string `json:"tension_refs,omitempty"`
	PatternRefs        []string `json:"pattern_refs,omitempty"`
	ProvenanceRefs     []string `json:"provenance_refs,omitempty"`
	RelatedRequestRefs []string `json:"related_request_refs,omitempty"`
}

type CoherenceSuggestion struct {
	SuggestionID        string   `json:"suggestion_id,omitempty"`
	Title               string   `json:"title,omitempty"`
	Summary             string   `json:"summary,omitempty"`
	SuggestedAction     string   `json:"suggested_action,omitempty"`
	TouchedSymbols      []string `json:"touched_symbols,omitempty"`
	PatternRefs         []string `json:"pattern_refs,omitempty"`
	TensionRefs         []string `json:"tension_refs,omitempty"`
	RelevantRequestRefs []string `json:"relevant_request_refs,omitempty"`
}

type ProspectivePairingSummary struct {
	PairingID       string   `json:"pairing_id,omitempty"`
	Title           string   `json:"title,omitempty"`
	Summary         string   `json:"summary,omitempty"`
	ProspectiveRef  string   `json:"prospective_ref,omitempty"`
	PatternRefs     []string `json:"pattern_refs,omitempty"`
	ConvergenceRefs []string `json:"convergence_refs,omitempty"`
	CandidateRefs   []string `json:"candidate_refs,omitempty"`
}

type SemanticInputBundle struct {
	WorkflowID              string                      `json:"workflow_id,omitempty"`
	ExplorationID           string                      `json:"exploration_id,omitempty"`
	BasedOnRevision         string                      `json:"based_on_revision,omitempty"`
	PendingRequests         []SemanticRequestRef        `json:"pending_requests,omitempty"`
	CompletedRequests       []SemanticRequestRef        `json:"completed_requests,omitempty"`
	PatternRefs             []string                    `json:"pattern_refs,omitempty"`
	TensionRefs             []string                    `json:"tension_refs,omitempty"`
	ProspectiveRefs         []string                    `json:"prospective_refs,omitempty"`
	ConvergenceRefs         []string                    `json:"convergence_refs,omitempty"`
	LearningInteractionRefs []string                    `json:"learning_interaction_refs,omitempty"`
	RequestProvenanceRefs   []string                    `json:"request_provenance_refs,omitempty"`
	ProvenanceRefs          []string                    `json:"provenance_refs,omitempty"`
	PatternFindings         []SemanticFindingSummary    `json:"pattern_findings,omitempty"`
	TensionFindings         []SemanticFindingSummary    `json:"tension_findings,omitempty"`
	ProspectiveFindings     []SemanticFindingSummary    `json:"prospective_findings,omitempty"`
	ConvergenceFindings     []SemanticFindingSummary    `json:"convergence_findings,omitempty"`
	LearningFindings        []SemanticFindingSummary    `json:"learning_findings,omitempty"`
	PatternProposals        []PatternProposalSummary    `json:"pattern_proposals,omitempty"`
	TensionClusters         []TensionClusterSummary     `json:"tension_clusters,omitempty"`
	CoherenceSuggestions    []CoherenceSuggestion       `json:"coherence_suggestions,omitempty"`
	ProspectivePairings     []ProspectivePairingSummary `json:"prospective_pairings,omitempty"`
	Source                  string                      `json:"source,omitempty"`
}

type ContextPolicySummary struct {
	MaxTokens           int      `json:"max_tokens,omitempty"`
	CompressionStrategy string   `json:"compression_strategy,omitempty"`
	ProgressiveLoading  bool     `json:"progressive_loading"`
	PreferredDetail     string   `json:"preferred_detail,omitempty"`
	ProtectPatterns     []string `json:"protect_patterns,omitempty"`
}

type ContextRuntimeState struct {
	ModeID                 string         `json:"mode_id,omitempty"`
	ExecutorFamily         ExecutorFamily `json:"executor_family,omitempty"`
	StrategyName           string         `json:"strategy_name,omitempty"`
	PreferredDetail        string         `json:"preferred_detail,omitempty"`
	ProgressiveEnabled     bool           `json:"progressive_enabled"`
	BudgetMaxTokens        int            `json:"budget_max_tokens,omitempty"`
	AvailableContextTokens int            `json:"available_context_tokens,omitempty"`
	BudgetState            string         `json:"budget_state,omitempty"`
	ContextTokens          int            `json:"context_tokens,omitempty"`
	ContextUsagePercent    float64        `json:"context_usage_percent,omitempty"`
	MinHistorySize         int            `json:"min_history_size,omitempty"`
	CompressionThreshold   float64        `json:"compression_threshold,omitempty"`
	InitialLoadAttempted   bool           `json:"initial_load_attempted"`
	InitialLoadCompleted   bool           `json:"initial_load_completed"`
	SignalsHandled         bool           `json:"signals_handled"`
	CompactionEligible     bool           `json:"compaction_eligible"`
	RestoreRequired        bool           `json:"restore_required"`
	CompactionObserved     bool           `json:"compaction_observed"`
	DemotionObserved       bool           `json:"demotion_observed"`
	PruningObserved        bool           `json:"pruning_observed"`
	ProtectedPaths         []string       `json:"protected_paths,omitempty"`
	LastInitialLoadError   string         `json:"last_initial_load_error,omitempty"`
	DebugMessages          []string       `json:"debug_messages,omitempty"`
	UpdatedAt              time.Time      `json:"updated_at,omitempty"`
}

type SecurityDiagnostic struct {
	Kind     string   `json:"kind,omitempty"`
	Subject  string   `json:"subject,omitempty"`
	Severity string   `json:"severity,omitempty"`
	Summary  string   `json:"summary,omitempty"`
	Refs     []string `json:"refs,omitempty"`
}

type CapabilityContractRuntimeState struct {
	PrimaryCapabilityID              string    `json:"primary_capability_id,omitempty"`
	InspectFirst                     bool      `json:"inspect_first"`
	NonMutating                      bool      `json:"non_mutating"`
	RequiresCompiledPlan             bool      `json:"requires_compiled_plan"`
	HasCompiledPlan                  bool      `json:"has_compiled_plan"`
	LazySemanticAcquisitionEligible  bool      `json:"lazy_semantic_acquisition_eligible"`
	LazySemanticAcquisitionTriggered bool      `json:"lazy_semantic_acquisition_triggered"`
	DebugEscalationTarget            string    `json:"debug_escalation_target,omitempty"`
	DebugEscalationTriggered         bool      `json:"debug_escalation_triggered"`
	Blocked                          bool      `json:"blocked"`
	ViolationReason                  string    `json:"violation_reason,omitempty"`
	Diagnostics                      []string  `json:"diagnostics,omitempty"`
	UpdatedAt                        time.Time `json:"updated_at,omitempty"`
}

type ArchaeologyCapabilityRuntimeState struct {
	PrimaryCapabilityID         string    `json:"primary_capability_id,omitempty"`
	PrimaryOperation            string    `json:"primary_operation,omitempty"`
	PrimaryLLMDependent         bool      `json:"primary_llm_dependent"`
	PrimaryArchaeoAssociated    bool      `json:"primary_archaeo_associated"`
	ExecutedRecipeIDs           []string  `json:"executed_recipe_ids,omitempty"`
	SpecializedCapabilityIDs    []string  `json:"specialized_capability_ids,omitempty"`
	BehaviorPath                string    `json:"behavior_path,omitempty"`
	PolicySnapshotID            string    `json:"policy_snapshot_id,omitempty"`
	AdmittedCapabilityIDs       []string  `json:"admitted_capability_ids,omitempty"`
	AdmittedModelTools          []string  `json:"admitted_model_tools,omitempty"`
	SupportingCapabilityIDs     []string  `json:"supporting_capability_ids,omitempty"`
	SupportingOperations        []string  `json:"supporting_operations,omitempty"`
	SupportingLLMDependentCount int       `json:"supporting_llm_dependent_count,omitempty"`
	WorkflowID                  string    `json:"workflow_id,omitempty"`
	ExplorationID               string    `json:"exploration_id,omitempty"`
	PlanID                      string    `json:"plan_id,omitempty"`
	PlanVersion                 int       `json:"plan_version,omitempty"`
	HasCompiledPlan             bool      `json:"has_compiled_plan"`
	PendingRequestCount         int       `json:"pending_request_count,omitempty"`
	CompletedRequestCount       int       `json:"completed_request_count,omitempty"`
	PatternRefCount             int       `json:"pattern_ref_count,omitempty"`
	TensionRefCount             int       `json:"tension_ref_count,omitempty"`
	ProspectiveRefCount         int       `json:"prospective_ref_count,omitempty"`
	ConvergenceRefCount         int       `json:"convergence_ref_count,omitempty"`
	LearningRefCount            int       `json:"learning_ref_count,omitempty"`
	PlanBound                   bool      `json:"plan_bound"`
	LongRunning                 bool      `json:"long_running"`
	Summary                     string    `json:"summary,omitempty"`
	UpdatedAt                   time.Time `json:"updated_at,omitempty"`
}

type DebugCapabilityRuntimeState struct {
	PrimaryCapabilityID                        string    `json:"primary_capability_id,omitempty"`
	SupportingCapabilityIDs                    []string  `json:"supporting_capability_ids,omitempty"`
	ExecutedRecipeIDs                          []string  `json:"executed_recipe_ids,omitempty"`
	SpecializedCapabilityIDs                   []string  `json:"specialized_capability_ids,omitempty"`
	BehaviorPath                               string    `json:"behavior_path,omitempty"`
	PolicySnapshotID                           string    `json:"policy_snapshot_id,omitempty"`
	AdmittedCapabilityIDs                      []string  `json:"admitted_capability_ids,omitempty"`
	AdmittedModelTools                         []string  `json:"admitted_model_tools,omitempty"`
	RootCauseActive                            bool      `json:"root_cause_active"`
	HypothesisRefinementActive                 bool      `json:"hypothesis_refinement_active"`
	LocalizationActive                         bool      `json:"localization_active"`
	FlawSurfacingActive                        bool      `json:"flaw_surfacing_active"`
	VerificationRepairActive                   bool      `json:"verification_repair_active"`
	ToolExpositionFacet                        bool      `json:"tool_exposition_facet"`
	ToolAccessConstrained                      bool      `json:"tool_access_constrained"`
	ToolCapabilityIDs                          []string  `json:"tool_capability_ids,omitempty"`
	ToolOutputSources                          []string  `json:"tool_output_sources,omitempty"`
	VerificationPlanScope                      string    `json:"verification_plan_scope,omitempty"`
	VerificationPlanSource                     string    `json:"verification_plan_source,omitempty"`
	VerificationPlanCommands                   []string  `json:"verification_plan_commands,omitempty"`
	VerificationPlanFiles                      []string  `json:"verification_plan_files,omitempty"`
	VerificationPlanTestFiles                  []string  `json:"verification_plan_test_files,omitempty"`
	VerificationPlanPlannerID                  string    `json:"verification_plan_planner_id,omitempty"`
	VerificationPlanRationale                  string    `json:"verification_plan_rationale,omitempty"`
	VerificationPlanAuditTrail                 []string  `json:"verification_plan_audit_trail,omitempty"`
	VerificationPlanCompatibilitySensitive     bool      `json:"verification_plan_compatibility_sensitive"`
	VerificationPlanSelectionInputs            []string  `json:"verification_plan_selection_inputs,omitempty"`
	VerificationPlanPolicyPreferences          []string  `json:"verification_plan_policy_preferences,omitempty"`
	VerificationPlanPolicyRequiresVerification bool      `json:"verification_plan_policy_requires_verification"`
	VerificationStatus                         string    `json:"verification_status,omitempty"`
	VerificationCheckCount                     int       `json:"verification_check_count,omitempty"`
	ArchaeoAssociated                          bool      `json:"archaeo_associated"`
	PatternRefCount                            int       `json:"pattern_ref_count,omitempty"`
	TensionRefCount                            int       `json:"tension_ref_count,omitempty"`
	MutationObserved                           bool      `json:"mutation_observed"`
	EscalationTarget                           string    `json:"escalation_target,omitempty"`
	EscalationTriggered                        bool      `json:"escalation_triggered"`
	DeniedToolUsage                            []string  `json:"denied_tool_usage,omitempty"`
	Diagnostics                                []string  `json:"diagnostics,omitempty"`
	Summary                                    string    `json:"summary,omitempty"`
	UpdatedAt                                  time.Time `json:"updated_at,omitempty"`
}

type ChatCapabilityRuntimeState struct {
	PrimaryCapabilityID                        string    `json:"primary_capability_id,omitempty"`
	SupportingCapabilityIDs                    []string  `json:"supporting_capability_ids,omitempty"`
	ExecutedRecipeIDs                          []string  `json:"executed_recipe_ids,omitempty"`
	SpecializedCapabilityIDs                   []string  `json:"specialized_capability_ids,omitempty"`
	BehaviorPath                               string    `json:"behavior_path,omitempty"`
	PolicySnapshotID                           string    `json:"policy_snapshot_id,omitempty"`
	AdmittedCapabilityIDs                      []string  `json:"admitted_capability_ids,omitempty"`
	AdmittedModelTools                         []string  `json:"admitted_model_tools,omitempty"`
	AskActive                                  bool      `json:"ask_active"`
	InspectActive                              bool      `json:"inspect_active"`
	ImplementActive                            bool      `json:"implement_active"`
	NonMutating                                bool      `json:"non_mutating"`
	InspectFirst                               bool      `json:"inspect_first"`
	LazySemanticAcquisitionEligible            bool      `json:"lazy_semantic_acquisition_eligible"`
	LazySemanticAcquisitionTriggered           bool      `json:"lazy_semantic_acquisition_triggered"`
	DirectEditExecutionActive                  bool      `json:"direct_edit_execution_active"`
	LocalReviewActive                          bool      `json:"local_review_active"`
	TargetedVerificationRepairActive           bool      `json:"targeted_verification_repair_active"`
	ArchaeoSupportActive                       bool      `json:"archaeo_support_active"`
	ArchaeoSupportTriggered                    bool      `json:"archaeo_support_triggered"`
	MutationObserved                           bool      `json:"mutation_observed"`
	VerificationPlanScope                      string    `json:"verification_plan_scope,omitempty"`
	VerificationPlanSource                     string    `json:"verification_plan_source,omitempty"`
	VerificationPlanCommands                   []string  `json:"verification_plan_commands,omitempty"`
	VerificationPlanFiles                      []string  `json:"verification_plan_files,omitempty"`
	VerificationPlanTestFiles                  []string  `json:"verification_plan_test_files,omitempty"`
	VerificationPlanPlannerID                  string    `json:"verification_plan_planner_id,omitempty"`
	VerificationPlanRationale                  string    `json:"verification_plan_rationale,omitempty"`
	VerificationPlanAuditTrail                 []string  `json:"verification_plan_audit_trail,omitempty"`
	VerificationPlanCompatibilitySensitive     bool      `json:"verification_plan_compatibility_sensitive"`
	VerificationPlanSelectionInputs            []string  `json:"verification_plan_selection_inputs,omitempty"`
	VerificationPlanPolicyPreferences          []string  `json:"verification_plan_policy_preferences,omitempty"`
	VerificationPlanPolicyRequiresVerification bool      `json:"verification_plan_policy_requires_verification"`
	VerificationStatus                         string    `json:"verification_status,omitempty"`
	VerificationCheckCount                     int       `json:"verification_check_count,omitempty"`
	ToolCapabilityIDs                          []string  `json:"tool_capability_ids,omitempty"`
	SharedContextEnabled                       bool      `json:"shared_context_enabled"`
	SharedContextRecentMutationCount           int       `json:"shared_context_recent_mutation_count,omitempty"`
	Diagnostics                                []string  `json:"diagnostics,omitempty"`
	Summary                                    string    `json:"summary,omitempty"`
	UpdatedAt                                  time.Time `json:"updated_at,omitempty"`
}

type SecurityRuntimeState struct {
	ModeID                     string               `json:"mode_id,omitempty"`
	ExecutorFamily             ExecutorFamily       `json:"executor_family,omitempty"`
	AllowedSelectorsConfigured bool                 `json:"allowed_selectors_configured"`
	ExecutionCatalogSnapshotID string               `json:"execution_catalog_snapshot_id,omitempty"`
	PolicySnapshotID           string               `json:"policy_snapshot_id,omitempty"`
	AdmittedCallableCaps       []string             `json:"admitted_callable_capabilities,omitempty"`
	AdmittedInspectableCaps    []string             `json:"admitted_inspectable_capabilities,omitempty"`
	AdmittedModelTools         []string             `json:"admitted_model_tools,omitempty"`
	Blocked                    bool                 `json:"blocked"`
	DeniedCapabilityUsage      []string             `json:"denied_capability_usage,omitempty"`
	DeniedToolUsage            []string             `json:"denied_tool_usage,omitempty"`
	Diagnostics                []SecurityDiagnostic `json:"diagnostics,omitempty"`
	UpdatedAt                  time.Time            `json:"updated_at,omitempty"`
}

type SharedContextRuntimeState struct {
	Enabled             bool           `json:"enabled"`
	ExecutorFamily      ExecutorFamily `json:"executor_family,omitempty"`
	BehaviorFamily      string         `json:"behavior_family,omitempty"`
	Participants        []string       `json:"participants,omitempty"`
	WorkingSetRefs      []string       `json:"working_set_refs,omitempty"`
	RecentMutationKeys  []string       `json:"recent_mutation_keys,omitempty"`
	RecentMutationCount int            `json:"recent_mutation_count,omitempty"`
	UpdatedAt           time.Time      `json:"updated_at,omitempty"`
}

type ResolvedExecutionPolicy struct {
	ModeID                          string                        `json:"mode_id,omitempty"`
	ProfileID                       string                        `json:"profile_id,omitempty"`
	PhaseCapabilityConstraints      map[string][]string           `json:"phase_capability_constraints,omitempty"`
	PreferredPlanningCapabilities   []string                      `json:"preferred_planning_capabilities,omitempty"`
	PreferredVerifyCapabilities     []string                      `json:"preferred_verify_capabilities,omitempty"`
	RecoveryProbeCapabilities       []string                      `json:"recovery_probe_capabilities,omitempty"`
	VerificationSuccessCapabilities []string                      `json:"verification_success_capabilities,omitempty"`
	PlanningStepTemplates           []core.SkillStepTemplate      `json:"planning_step_templates,omitempty"`
	RequireVerificationStep         bool                          `json:"require_verification_step"`
	ReviewCriteria                  []string                      `json:"review_criteria,omitempty"`
	ReviewFocusTags                 []string                      `json:"review_focus_tags,omitempty"`
	ReviewApprovalRules             core.AgentReviewApprovalRules `json:"review_approval_rules,omitempty"`
	ContextPolicy                   ContextPolicySummary          `json:"context_policy,omitempty"`
	ResolvedFromSkillPolicy         bool                          `json:"resolved_from_skill_policy"`
}

type WorkUnitExecutorDescriptor struct {
	ExecutorID    string         `json:"executor_id,omitempty"`
	Family        ExecutorFamily `json:"family,omitempty"`
	RecipeID      string         `json:"recipe_id,omitempty"`
	Reason        string         `json:"reason,omitempty"`
	Compatibility bool           `json:"compatibility"`
}

type ExecutorRuntimeState struct {
	ExecutorID string         `json:"executor_id,omitempty"`
	Family     ExecutorFamily `json:"family,omitempty"`
	Path       string         `json:"path,omitempty"`
	Reason     string         `json:"reason,omitempty"`
}

type UnitOfWorkTransitionState struct {
	PreviousUnitOfWorkID        string    `json:"previous_unit_of_work_id,omitempty"`
	CurrentUnitOfWorkID         string    `json:"current_unit_of_work_id,omitempty"`
	RootUnitOfWorkID            string    `json:"root_unit_of_work_id,omitempty"`
	PreviousModeID              string    `json:"previous_mode_id,omitempty"`
	CurrentModeID               string    `json:"current_mode_id,omitempty"`
	PreviousPrimaryCapabilityID string    `json:"previous_primary_capability_id,omitempty"`
	CurrentPrimaryCapabilityID  string    `json:"current_primary_capability_id,omitempty"`
	Preserved                   bool      `json:"preserved"`
	Rebound                     bool      `json:"rebound"`
	Reason                      string    `json:"reason,omitempty"`
	PreviousArchaeoContext      bool      `json:"previous_archaeo_context"`
	CurrentArchaeoContext       bool      `json:"current_archaeo_context"`
	TransitionCompatibilityOK   bool      `json:"transition_compatibility_ok"`
	UpdatedAt                   time.Time `json:"updated_at,omitempty"`
}

type UnitOfWorkHistoryEntry struct {
	UnitOfWorkID                string    `json:"unit_of_work_id,omitempty"`
	RootUnitOfWorkID            string    `json:"root_unit_of_work_id,omitempty"`
	PredecessorUnitOfWorkID     string    `json:"predecessor_unit_of_work_id,omitempty"`
	ModeID                      string    `json:"mode_id,omitempty"`
	PrimaryRelurpicCapabilityID string    `json:"primary_relurpic_capability_id,omitempty"`
	TransitionReason            string    `json:"transition_reason,omitempty"`
	Rebound                     bool      `json:"rebound"`
	Preserved                   bool      `json:"preserved"`
	UpdatedAt                   time.Time `json:"updated_at,omitempty"`
}

type UnitOfWork struct {
	ID          string `json:"id,omitempty"`
	WorkflowID  string `json:"workflow_id,omitempty"`
	RunID       string `json:"run_id,omitempty"`
	ExecutionID string `json:"execution_id,omitempty"`
	RootID      string `json:"root_id,omitempty"`

	ModeID            string `json:"mode_id,omitempty"`
	ObjectiveKind     string `json:"objective_kind,omitempty"`
	BehaviorFamily    string `json:"behavior_family,omitempty"`
	ContextStrategyID string `json:"context_strategy_id,omitempty"`

	VerificationPolicyID            string   `json:"verification_policy_id,omitempty"`
	DeferralPolicyID                string   `json:"deferral_policy_id,omitempty"`
	CheckpointPolicyID              string   `json:"checkpoint_policy_id,omitempty"`
	PrimaryRelurpicCapabilityID     string   `json:"primary_relurpic_capability_id,omitempty"`
	SupportingRelurpicCapabilityIDs []string `json:"supporting_relurpic_capability_ids,omitempty"`

	SemanticInputs     SemanticInputBundle           `json:"semantic_inputs,omitempty"`
	ResolvedPolicy     ResolvedExecutionPolicy       `json:"resolved_execution_policy,omitempty"`
	ExecutorDescriptor WorkUnitExecutorDescriptor    `json:"executor_descriptor,omitempty"`
	PlanBinding        *UnitOfWorkPlanBinding        `json:"plan_binding,omitempty"`
	ContextBundle      UnitOfWorkContextBundle       `json:"context_bundle,omitempty"`
	RoutineBindings    []UnitOfWorkRoutineBinding    `json:"routine_bindings,omitempty"`
	SkillBindings      []UnitOfWorkSkillBinding      `json:"skill_bindings,omitempty"`
	ToolBindings       []UnitOfWorkToolBinding       `json:"tool_bindings,omitempty"`
	CapabilityBindings []UnitOfWorkCapabilityBinding `json:"capability_bindings,omitempty"`

	PredecessorUnitOfWorkID string                    `json:"predecessor_unit_of_work_id,omitempty"`
	TransitionReason        string                    `json:"transition_reason,omitempty"`
	TransitionState         UnitOfWorkTransitionState `json:"transition_state,omitempty"`
	Status                  UnitOfWorkStatus          `json:"status,omitempty"`
	ResultClass             ExecutionResultClass      `json:"result_class,omitempty"`
	AssuranceClass          AssuranceClass            `json:"assurance_class,omitempty"`
	DeferredIssueIDs        []string                  `json:"deferred_issue_ids,omitempty"`
	CreatedAt               time.Time                 `json:"created_at,omitempty"`
	UpdatedAt               time.Time                 `json:"updated_at,omitempty"`
}

type ContextLifecycleState struct {
	WorkflowID         string                `json:"workflow_id,omitempty"`
	RunID              string                `json:"run_id,omitempty"`
	ExecutionID        string                `json:"execution_id,omitempty"`
	UnitOfWorkID       string                `json:"unit_of_work_id,omitempty"`
	Stage              ContextLifecycleStage `json:"stage,omitempty"`
	CompactionEligible bool                  `json:"compaction_eligible"`
	RestoreRequired    bool                  `json:"restore_required"`
	CompactionCount    int                   `json:"compaction_count,omitempty"`
	RestoreCount       int                   `json:"restore_count,omitempty"`
	LastCompactedAt    time.Time             `json:"last_compacted_at,omitempty"`
	LastRestoredAt     time.Time             `json:"last_restored_at,omitempty"`
	LastRestoreStatus  string                `json:"last_restore_status,omitempty"`
	RestoreSource      string                `json:"restore_source,omitempty"`
	Summary            string                `json:"summary,omitempty"`
	ActivePlanID       string                `json:"active_plan_id,omitempty"`
	ActivePlanVersion  int                   `json:"active_plan_version,omitempty"`
	DeferredIssueIDs   []string              `json:"deferred_issue_ids,omitempty"`
	PreservedArtifacts []string              `json:"preserved_artifact_kinds,omitempty"`
}

type UnitOfWorkPlanBinding struct {
	WorkflowID    string              `json:"workflow_id,omitempty"`
	PlanID        string              `json:"plan_id,omitempty"`
	PlanVersion   int                 `json:"plan_version,omitempty"`
	ActiveStepID  string              `json:"active_step_id,omitempty"`
	RootChunkIDs  []string            `json:"root_chunk_ids,omitempty"`
	StepIDs       []string            `json:"step_ids,omitempty"`
	IsPlanBacked  bool                `json:"is_plan_backed"`
	IsLongRunning bool                `json:"is_long_running"`
	ArchaeoRefs   map[string][]string `json:"archaeo_refs,omitempty"`
}

type UnitOfWorkContextSource struct {
	Kind    string `json:"kind,omitempty"`
	Ref     string `json:"ref,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type UnitOfWorkContextBundle struct {
	Sources            []UnitOfWorkContextSource `json:"sources,omitempty"`
	WorkspacePaths     []string                  `json:"workspace_paths,omitempty"`
	RetrievalRefs      []string                  `json:"retrieval_refs,omitempty"`
	ArtifactKinds      []string                  `json:"artifact_kinds,omitempty"`
	PatternRefs        []string                  `json:"pattern_refs,omitempty"`
	TensionRefs        []string                  `json:"tension_refs,omitempty"`
	ProvenanceRefs     []string                  `json:"provenance_refs,omitempty"`
	LearningRefs       []string                  `json:"learning_refs,omitempty"`
	ContextBudgetClass string                    `json:"context_budget_class,omitempty"`
	CompactionEligible bool                      `json:"compaction_eligible"`
	RestoreRequired    bool                      `json:"restore_required"`
}

type UnitOfWorkRoutineBinding struct {
	RoutineID string `json:"routine_id,omitempty"`
	Family    string `json:"family,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Priority  int    `json:"priority,omitempty"`
	Required  bool   `json:"required"`
}

type UnitOfWorkSkillBinding struct {
	SkillID  string `json:"skill_id,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Required bool   `json:"required"`
}

type UnitOfWorkToolBinding struct {
	ToolID  string `json:"tool_id,omitempty"`
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

type UnitOfWorkCapabilityBinding struct {
	CapabilityID string `json:"capability_id,omitempty"`
	Family       string `json:"family,omitempty"`
	Required     bool   `json:"required"`
}

type CompiledExecution struct {
	WorkflowID       string    `json:"workflow_id,omitempty"`
	RunID            string    `json:"run_id,omitempty"`
	ExecutionID      string    `json:"execution_id,omitempty"`
	UnitOfWorkID     string    `json:"unit_of_work_id,omitempty"`
	RootUnitOfWorkID string    `json:"root_unit_of_work_id,omitempty"`
	CompiledAt       time.Time `json:"compiled_at,omitempty"`
	UpdatedAt        time.Time `json:"updated_at,omitempty"`
	ModeID           string    `json:"mode_id,omitempty"`
	ObjectiveKind    string    `json:"objective_kind,omitempty"`
	BehaviorFamily   string    `json:"behavior_family,omitempty"`

	ContextStrategyID               string   `json:"context_strategy_id,omitempty"`
	VerificationPolicyID            string   `json:"verification_policy_id,omitempty"`
	DeferralPolicyID                string   `json:"deferral_policy_id,omitempty"`
	CheckpointPolicyID              string   `json:"checkpoint_policy_id,omitempty"`
	PrimaryRelurpicCapabilityID     string   `json:"primary_relurpic_capability_id,omitempty"`
	SupportingRelurpicCapabilityIDs []string `json:"supporting_relurpic_capability_ids,omitempty"`

	SemanticInputs     SemanticInputBundle           `json:"semantic_inputs,omitempty"`
	ResolvedPolicy     ResolvedExecutionPolicy       `json:"resolved_execution_policy,omitempty"`
	ExecutorDescriptor WorkUnitExecutorDescriptor    `json:"executor_descriptor,omitempty"`
	PlanBinding        *UnitOfWorkPlanBinding        `json:"plan_binding,omitempty"`
	ContextBundle      UnitOfWorkContextBundle       `json:"context_bundle,omitempty"`
	RoutineBindings    []UnitOfWorkRoutineBinding    `json:"routine_bindings,omitempty"`
	SkillBindings      []UnitOfWorkSkillBinding      `json:"skill_bindings,omitempty"`
	ToolBindings       []UnitOfWorkToolBinding       `json:"tool_bindings,omitempty"`
	CapabilityBindings []UnitOfWorkCapabilityBinding `json:"capability_bindings,omitempty"`

	PredecessorUnitOfWorkID     string                    `json:"predecessor_unit_of_work_id,omitempty"`
	TransitionReason            string                    `json:"transition_reason,omitempty"`
	TransitionState             UnitOfWorkTransitionState `json:"transition_state,omitempty"`
	Status                      ExecutionStatus           `json:"status,omitempty"`
	ResultClass                 ExecutionResultClass      `json:"result_class,omitempty"`
	AssuranceClass              AssuranceClass            `json:"assurance_class,omitempty"`
	DeferredIssueIDs            []string                  `json:"deferred_issue_ids,omitempty"`
	ArchaeoRefs                 map[string][]string       `json:"archaeo_refs,omitempty"`
	ProviderSnapshotRefs        []string                  `json:"provider_snapshot_refs,omitempty"`
	ProviderSessionSnapshotRefs []string                  `json:"provider_session_snapshot_refs,omitempty"`
}

type RuntimeExecutionStatus struct {
	WorkflowID        string               `json:"workflow_id,omitempty"`
	RunID             string               `json:"run_id,omitempty"`
	ExecutionID       string               `json:"execution_id,omitempty"`
	UnitOfWorkID      string               `json:"unit_of_work_id,omitempty"`
	Status            ExecutionStatus      `json:"status,omitempty"`
	ResultClass       ExecutionResultClass `json:"result_class,omitempty"`
	AssuranceClass    AssuranceClass       `json:"assurance_class,omitempty"`
	ActivePlanID      string               `json:"active_plan_id,omitempty"`
	ActivePlanVersion int                  `json:"active_plan_version,omitempty"`
	ActiveStepID      string               `json:"active_step_id,omitempty"`
	DeferredIssueIDs  []string             `json:"deferred_issue_ids,omitempty"`
	UpdatedAt         time.Time            `json:"updated_at,omitempty"`
}

type DeferredExecutionIssue struct {
	IssueID               string                    `json:"issue_id,omitempty"`
	WorkflowID            string                    `json:"workflow_id,omitempty"`
	RunID                 string                    `json:"run_id,omitempty"`
	ExecutionID           string                    `json:"execution_id,omitempty"`
	ActivePlanID          string                    `json:"active_plan_id,omitempty"`
	ActivePlanVersion     int                       `json:"active_plan_version,omitempty"`
	StepID                string                    `json:"step_id,omitempty"`
	RelatedStepIDs        []string                  `json:"related_step_ids,omitempty"`
	Kind                  DeferredIssueKind         `json:"kind,omitempty"`
	Severity              DeferredIssueSeverity     `json:"severity,omitempty"`
	Status                DeferredIssueStatus       `json:"status,omitempty"`
	Title                 string                    `json:"title,omitempty"`
	Summary               string                    `json:"summary,omitempty"`
	WhyNotResolvedInline  string                    `json:"why_not_resolved_inline,omitempty"`
	RecommendedReentry    string                    `json:"recommended_reentry,omitempty"`
	RecommendedNextAction string                    `json:"recommended_next_action,omitempty"`
	WorkspaceArtifactPath string                    `json:"workspace_artifact_path,omitempty"`
	Evidence              DeferredExecutionEvidence `json:"evidence,omitempty"`
	ArchaeoRefs           map[string][]string       `json:"archaeo_refs,omitempty"`
	CreatedAt             time.Time                 `json:"created_at,omitempty"`
	UpdatedAt             time.Time                 `json:"updated_at,omitempty"`
}

type DeferredExecutionEvidence struct {
	TouchedSymbols         []string       `json:"touched_symbols,omitempty"`
	RelevantPatternRefs    []string       `json:"relevant_pattern_refs,omitempty"`
	RelevantTensionRefs    []string       `json:"relevant_tension_refs,omitempty"`
	RelevantAnchorRefs     []string       `json:"relevant_anchor_refs,omitempty"`
	RelevantProvenanceRefs []string       `json:"relevant_provenance_refs,omitempty"`
	RelevantRequestRefs    []string       `json:"relevant_request_refs,omitempty"`
	VerificationRefs       []string       `json:"verification_refs,omitempty"`
	CheckpointRefs         []string       `json:"checkpoint_refs,omitempty"`
	ProviderStateSnapshot  map[string]any `json:"provider_state_snapshot,omitempty"`
	ShortReasoningSummary  string         `json:"short_reasoning_summary,omitempty"`
}

// Re-export commonly used types from euclotypes for convenience
type (
	ModeResolution            = euclotypes.ModeResolution
	ExecutionProfileSelection = euclotypes.ExecutionProfileSelection
	ModeRegistry              = euclotypes.ModeRegistry
	ExecutionProfileRegistry  = euclotypes.ExecutionProfileRegistry
	Artifact                  = euclotypes.Artifact
)

type RetrievalPolicy struct {
	ModeID            string `json:"mode_id"`
	ProfileID         string `json:"profile_id"`
	LocalPathsFirst   bool   `json:"local_paths_first"`
	WidenToWorkflow   bool   `json:"widen_to_workflow"`
	WidenWhenNoLocal  bool   `json:"widen_when_no_local"`
	WorkflowLimit     int    `json:"workflow_limit"`
	WorkflowMaxTokens int    `json:"workflow_max_tokens"`
	ExpansionStrategy string `json:"expansion_strategy"`
}

type ContextExpansion struct {
	LocalPaths        []string       `json:"local_paths,omitempty"`
	WorkflowRetrieval map[string]any `json:"workflow_retrieval,omitempty"`
	WidenedToWorkflow bool           `json:"widened_to_workflow"`
	ExpansionStrategy string         `json:"expansion_strategy,omitempty"`
	Summary           string         `json:"summary,omitempty"`
}

type VerificationPolicy struct {
	PolicyID              string   `json:"policy_id"`
	ModeID                string   `json:"mode_id"`
	ProfileID             string   `json:"profile_id"`
	RequiresVerification  bool     `json:"requires_verification"`
	AcceptedStatuses      []string `json:"accepted_statuses,omitempty"`
	RequiresExecutedCheck bool     `json:"requires_executed_check"`
	ManualOutcomeAllowed  bool     `json:"manual_outcome_allowed"`
}

type VerificationProvenanceClass string

const (
	VerificationProvenanceExecuted VerificationProvenanceClass = "executed"
	VerificationProvenanceReused   VerificationProvenanceClass = "reused"
	VerificationProvenanceFallback VerificationProvenanceClass = "fallback"
	VerificationProvenanceManual   VerificationProvenanceClass = "manual"
	VerificationProvenanceAbsent   VerificationProvenanceClass = "absent"
)

type VerificationCheckRecord struct {
	Name                  string                      `json:"name,omitempty"`
	Command               string                      `json:"command,omitempty"`
	Args                  []string                    `json:"args,omitempty"`
	WorkingDirectory      string                      `json:"working_directory,omitempty"`
	Status                string                      `json:"status,omitempty"`
	ExitStatus            int                         `json:"exit_status,omitempty"`
	DurationMillis        int64                       `json:"duration_millis,omitempty"`
	FilesUnderCheck       []string                    `json:"files_under_check,omitempty"`
	ScopeKind             string                      `json:"scope_kind,omitempty"`
	OriginatingCapability string                      `json:"originating_capability,omitempty"`
	RunID                 string                      `json:"run_id,omitempty"`
	Timestamp             time.Time                   `json:"timestamp,omitempty"`
	Provenance            VerificationProvenanceClass `json:"provenance,omitempty"`
	Details               string                      `json:"details,omitempty"`
}

type VerificationEvidence struct {
	Status          string                      `json:"status"`
	Summary         string                      `json:"summary,omitempty"`
	Checks          []VerificationCheckRecord   `json:"checks,omitempty"`
	Source          string                      `json:"source,omitempty"`
	Provenance      VerificationProvenanceClass `json:"provenance,omitempty"`
	RunID           string                      `json:"run_id,omitempty"`
	Timestamp       time.Time                   `json:"timestamp,omitempty"`
	EvidencePresent bool                        `json:"evidence_present"`
}

type SuccessGateResult struct {
	Allowed              bool           `json:"allowed"`
	Reason               string         `json:"reason,omitempty"`
	Details              []string       `json:"details,omitempty"`
	AssuranceClass       AssuranceClass `json:"assurance_class,omitempty"`
	WaiverApplied        bool           `json:"waiver_applied"`
	DegradationMode      string         `json:"degradation_mode,omitempty"`
	DegradationReason    string         `json:"degradation_reason,omitempty"`
	AutomaticDegradation bool           `json:"automatic_degradation"`
}

type WaiverKind string

const (
	WaiverKindVerification  WaiverKind = "verification"
	WaiverKindTDDRedPhase   WaiverKind = "tdd_red_phase"
	WaiverKindReviewBlock   WaiverKind = "review_block"
	WaiverKindCompatibility WaiverKind = "compatibility_check"
)

type ExecutionWaiver struct {
	WaiverID   string         `json:"waiver_id,omitempty"`
	Kind       WaiverKind     `json:"kind,omitempty"`
	Scope      map[string]any `json:"scope,omitempty"`
	Reason     string         `json:"reason,omitempty"`
	GrantedBy  string         `json:"granted_by,omitempty"`
	RunID      string         `json:"run_id,omitempty"`
	CreatedAt  time.Time      `json:"created_at,omitempty"`
	ExpiresAt  time.Time      `json:"expires_at,omitempty"`
	ArchaeoRef string         `json:"archaeo_ref,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type EditIntent struct {
	Path    string `json:"path"`
	Action  string `json:"action"`
	Content string `json:"content,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type EditOperationRecord struct {
	Path           string         `json:"path"`
	Action         string         `json:"action"`
	Summary        string         `json:"summary,omitempty"`
	Requested      bool           `json:"requested"`
	ApprovalStatus string         `json:"approval_status,omitempty"`
	Status         string         `json:"status"`
	Tool           string         `json:"tool,omitempty"`
	Error          string         `json:"error,omitempty"`
	Result         map[string]any `json:"result,omitempty"`
}

type EditExecutionRecord struct {
	Requested []EditOperationRecord `json:"requested,omitempty"`
	Approved  []EditOperationRecord `json:"approved,omitempty"`
	Executed  []EditOperationRecord `json:"executed,omitempty"`
	Rejected  []EditOperationRecord `json:"rejected,omitempty"`
	Summary   string                `json:"summary,omitempty"`
}

type PhaseCapabilityRoute struct {
	Phase  string `json:"phase"`
	Family string `json:"family"`
	Agent  string `json:"agent"`
}

type CapabilityFamilyRouting struct {
	ModeID            string                 `json:"mode_id"`
	ProfileID         string                 `json:"profile_id"`
	PrimaryFamilyID   string                 `json:"primary_family_id"`
	FallbackFamilyIDs []string               `json:"fallback_family_ids,omitempty"`
	Routes            []PhaseCapabilityRoute `json:"routes,omitempty"`
}

type ActionLogEntry struct {
	Kind      string         `json:"kind"`
	Message   string         `json:"message"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type ProofSurface struct {
	ModeID                 string   `json:"mode_id,omitempty"`
	ProfileID              string   `json:"profile_id,omitempty"`
	PrimaryFamilyID        string   `json:"primary_family_id,omitempty"`
	VerificationStatus     string   `json:"verification_status,omitempty"`
	VerificationProvenance string   `json:"verification_provenance,omitempty"`
	RecoveryStatus         string   `json:"recovery_status,omitempty"`
	SuccessGateReason      string   `json:"success_gate_reason,omitempty"`
	AssuranceClass         string   `json:"assurance_class,omitempty"`
	DegradationMode        string   `json:"degradation_mode,omitempty"`
	DegradationReason      string   `json:"degradation_reason,omitempty"`
	ArtifactKinds          []string `json:"artifact_kinds,omitempty"`
	WorkflowRetrievalUsed  bool     `json:"workflow_retrieval_used"`
	CapabilityIDs          []string `json:"capability_ids,omitempty"`
	GateEvalsCount         int      `json:"gate_evals_count,omitempty"`
	PhasesExecuted         []string `json:"phases_executed,omitempty"`
	RecoveryAttempts       int      `json:"recovery_attempts,omitempty"`
	WaiverApplied          bool     `json:"waiver_applied"`
}

// RuntimeSurfaces abstracts access to workflow and runtime stores
type RuntimeSurfaces struct {
	Workflow *db.SQLiteWorkflowStateStore
	Runtime  memory.RuntimeMemoryStore
}
