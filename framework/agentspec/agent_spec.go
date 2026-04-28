package agentspec

import (
	"fmt"
	"os"
	"strings"
)

// AgentRuntimeSpec describes CLI/runtime level configuration derived from the
// manifest. These fields are optional from the sandbox point of view but
// provide the additional metadata needed by the orchestrator.
type AgentRuntimeSpec struct {
	Implementation      string                          `yaml:"implementation" json:"implementation"` // e.g. "react", "planner", "coding"
	Mode                AgentMode                       `yaml:"mode" json:"mode"`
	Version             string                          `yaml:"version,omitempty" json:"version,omitempty"`
	Prompt              string                          `yaml:"prompt,omitempty" json:"prompt,omitempty"`
	Model               AgentModelConfig                `yaml:"model" json:"model"`
	AllowedCapabilities []CapabilitySelector            `yaml:"allowed_capabilities,omitempty" json:"allowed_capabilities,omitempty"`
	ToolExecutionPolicy map[string]ToolPolicy           `yaml:"tool_execution_policy,omitempty" json:"tool_execution_policy,omitempty"`
	CapabilityPolicies  []CapabilityPolicy              `yaml:"capability_policies,omitempty" json:"capability_policies,omitempty"`
	ExposurePolicies    []CapabilityExposurePolicy      `yaml:"exposure_policies,omitempty" json:"exposure_policies,omitempty"`
	InsertionPolicies   []CapabilityInsertionPolicy     `yaml:"insertion_policies,omitempty" json:"insertion_policies,omitempty"`
	SessionPolicies     []SessionPolicy                 `yaml:"session_policies,omitempty" json:"session_policies,omitempty"`
	GlobalPolicies      map[string]AgentPermissionLevel `yaml:"policies,omitempty" json:"policies,omitempty"`
	ProviderPolicies    map[string]ProviderPolicy       `yaml:"provider_policies,omitempty" json:"provider_policies,omitempty"`
	Providers           []ProviderConfig                `yaml:"providers,omitempty" json:"providers,omitempty"`
	RuntimeSafety       *RuntimeSafetySpec              `yaml:"runtime_safety,omitempty" json:"runtime_safety,omitempty"`
	SkillConfig         AgentSkillConfig                `yaml:"skill_config,omitempty" json:"skill_config,omitempty"`
	Bash                AgentBashPermissions            `yaml:"bash_permissions,omitempty" json:"bash_permissions,omitempty"`
	Files               AgentFileMatrix                 `yaml:"file_permissions,omitempty" json:"file_permissions,omitempty"`
	Invocation          AgentInvocationSpec             `yaml:"invocation,omitempty" json:"invocation,omitempty"`
	Coordination        AgentCoordinationSpec           `yaml:"coordination,omitempty" json:"coordination,omitempty"`
	Composition         *AgentCompositionSpec           `yaml:"composition,omitempty" json:"composition,omitempty"`
	ArtifactWindow      AgentArtifactWindowSpec         `yaml:"context,omitempty" json:"context,omitempty"`
	Browser             *AgentBrowserSpec               `yaml:"browser,omitempty" json:"browser,omitempty"`
	LSP                 AgentLSPSpec                    `yaml:"lsp,omitempty" json:"lsp,omitempty"`
	Search              AgentSearchSpec                 `yaml:"search,omitempty" json:"search,omitempty"`
	Metadata            AgentMetadata                   `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	ToolCallingIntent   ToolCallingIntent               `yaml:"tool_calling_intent,omitempty" json:"tool_calling_intent,omitempty"`
	NativeToolCalling   *bool                           `yaml:"native_tool_calling,omitempty" json:"native_tool_calling,omitempty"`
	Logging             *AgentLoggingSpec               `yaml:"logging,omitempty" json:"logging,omitempty"`
	// Extensions holds agent-specific extension configurations.
	// The "euclo" key contains Euclo-specific configuration (see named/euclo/euclo_manifest_extension.go).
	Extensions map[string]any `yaml:",inline" json:"extensions,omitempty"`
	// Context holds the context policy configuration for ingestion and persistence.
	Context *ContextPolicySpec `yaml:"context_policy,omitempty" json:"context_policy,omitempty"`
}

// ToolCallingIntent captures the agent's preference for how tool calls should
// be executed when the backend supports multiple calling modes.
type ToolCallingIntent string

const (
	ToolCallingIntentAuto         ToolCallingIntent = "auto"
	ToolCallingIntentPreferNative ToolCallingIntent = "prefer_native"
	ToolCallingIntentPreferPrompt ToolCallingIntent = "prefer_prompt"
)

// AgentLSPSpec configures Language Server Protocol features.
type AgentLSPSpec struct {
	Servers map[string]string `yaml:"servers" json:"servers"` // "go": "gopls", "python": "pyright"
	Enabled bool              `yaml:"enabled" json:"enabled"`
	Timeout string            `yaml:"timeout" json:"timeout"`
}

// AgentSearchSpec configures search/indexing capabilities.
type AgentSearchSpec struct {
	HybridEnabled bool `yaml:"hybrid_enabled" json:"hybrid_enabled"` // Use both vector and AST
	VectorIndex   bool `yaml:"vector_index" json:"vector_index"`
	ASTIndex      bool `yaml:"ast_index" json:"ast_index"`
}

// NativeToolCallingEnabled reports whether native tool calling should be used.
func (a *AgentRuntimeSpec) NativeToolCallingEnabled() bool {
	if a == nil {
		return true
	}
	switch resolveToolCallingIntent(a.ToolCallingIntent, a.NativeToolCalling) {
	case ToolCallingIntentPreferPrompt:
		return false
	case ToolCallingIntentPreferNative:
		return true
	default:
		if a.NativeToolCalling != nil {
			return *a.NativeToolCalling
		}
		return true
	}
}

// ResolveToolCallingIntent returns the effective intent after compatibility
// fallbacks are applied.
func (a *AgentRuntimeSpec) ResolveToolCallingIntent() ToolCallingIntent {
	if a == nil {
		return ToolCallingIntentAuto
	}
	return resolveToolCallingIntent(a.ToolCallingIntent, a.NativeToolCalling)
}

// AgentLoggingSpec controls debug logging toggles for the agent.
type AgentLoggingSpec struct {
	LLM   *bool `yaml:"llm,omitempty" json:"llm,omitempty"`
	Agent *bool `yaml:"agent,omitempty" json:"agent,omitempty"`
}

// AgentMode categorizes the manifest mode.
type AgentMode string

const (
	AgentModePrimary AgentMode = "primary"
	AgentModeSub     AgentMode = "subagent"
	AgentModeSystem  AgentMode = "system"
)

// AgentModelConfig describes an LLM backing the agent.
type AgentModelConfig struct {
	Provider    string  `yaml:"provider" json:"provider"`
	Name        string  `yaml:"name" json:"name"`
	Temperature float64 `yaml:"temperature" json:"temperature"`
	MaxTokens   int     `yaml:"max_tokens" json:"max_tokens"`
}

// ToolPolicy configures visibility and execution gating for a single tool.
// Execution controls whether calls are allowed, denied, or require HITL approval.
type ToolPolicy struct {
	Execute AgentPermissionLevel `yaml:"execute,omitempty" json:"execute,omitempty"` // allow/deny/ask
}

// CapabilityPolicy configures execution gating for capabilities selected by framework-owned metadata.
type CapabilityPolicy struct {
	Selector CapabilitySelector   `yaml:"selector" json:"selector"`
	Execute  AgentPermissionLevel `yaml:"execute,omitempty" json:"execute,omitempty"`
}

// CapabilityInsertionPolicy configures how matching capability output may be inserted into the model-visible window.
type CapabilityInsertionPolicy struct {
	Selector CapabilitySelector `yaml:"selector" json:"selector"`
	Action   InsertionAction    `yaml:"action" json:"action"`
}

type CapabilityExposure string

const (
	CapabilityExposureHidden      CapabilityExposure = "hidden"
	CapabilityExposureInspectable CapabilityExposure = "inspectable"
	CapabilityExposureCallable    CapabilityExposure = "callable"
)

// CapabilityExposurePolicy configures visibility of admitted capabilities.
type CapabilityExposurePolicy struct {
	Selector CapabilitySelector `yaml:"selector" json:"selector"`
	Access   CapabilityExposure `yaml:"access" json:"access"`
}

// CapabilitySelector matches capabilities by identity and explicit metadata instead of raw tool tags.
type CapabilitySelector struct {
	ID                          string                      `yaml:"id,omitempty" json:"id,omitempty"`
	Name                        string                      `yaml:"name,omitempty" json:"name,omitempty"`
	Kind                        CapabilityKind              `yaml:"kind,omitempty" json:"kind,omitempty"`
	RuntimeFamilies             []CapabilityRuntimeFamily   `yaml:"runtime_families,omitempty" json:"runtime_families,omitempty"`
	Tags                        []string                    `yaml:"tags,omitempty" json:"tags,omitempty"`
	ExcludeTags                 []string                    `yaml:"exclude_tags,omitempty" json:"exclude_tags,omitempty"`
	SourceScopes                []CapabilityScope           `yaml:"source_scopes,omitempty" json:"source_scopes,omitempty"`
	TrustClasses                []TrustClass                `yaml:"trust_classes,omitempty" json:"trust_classes,omitempty"`
	RiskClasses                 []RiskClass                 `yaml:"risk_classes,omitempty" json:"risk_classes,omitempty"`
	EffectClasses               []EffectClass               `yaml:"effect_classes,omitempty" json:"effect_classes,omitempty"`
	CoordinationRoles           []CoordinationRole          `yaml:"coordination_roles,omitempty" json:"coordination_roles,omitempty"`
	CoordinationTaskTypes       []string                    `yaml:"coordination_task_types,omitempty" json:"coordination_task_types,omitempty"`
	CoordinationExecutionModes  []CoordinationExecutionMode `yaml:"coordination_execution_modes,omitempty" json:"coordination_execution_modes,omitempty"`
	CoordinationLongRunning     *bool                       `yaml:"coordination_long_running,omitempty" json:"coordination_long_running,omitempty"`
	CoordinationDirectInsertion *bool                       `yaml:"coordination_direct_insertion,omitempty" json:"coordination_direct_insertion,omitempty"`
}

// ProviderPolicy configures activation defaults and trust metadata for provider-backed capabilities.
type ProviderPolicy struct {
	Activate               AgentPermissionLevel `yaml:"activate,omitempty" json:"activate,omitempty"`
	DefaultTrust           TrustClass           `yaml:"default_trust,omitempty" json:"default_trust,omitempty"`
	AllowCredentialSharing bool                 `yaml:"allow_credential_sharing,omitempty" json:"allow_credential_sharing,omitempty"`
}

// AgentBashPermissions constrains shell commands.
type AgentBashPermissions struct {
	AllowPatterns []string             `yaml:"allow_patterns" json:"allow_patterns"`
	DenyPatterns  []string             `yaml:"deny_patterns" json:"deny_patterns"`
	Default       AgentPermissionLevel `yaml:"default" json:"default"`
}

// AgentFileMatrix scopes write/edit operations.
type AgentFileMatrix struct {
	Write AgentFilePermissionSet `yaml:"write" json:"write"`
	Edit  AgentFilePermissionSet `yaml:"edit" json:"edit"`
}

// AgentFilePermissionSet stores glob allow/deny rules.
type AgentFilePermissionSet struct {
	AllowPatterns     []string             `yaml:"allow_patterns" json:"allow_patterns"`
	DenyPatterns      []string             `yaml:"deny_patterns" json:"deny_patterns"`
	Default           AgentPermissionLevel `yaml:"default" json:"default"`
	RequireApproval   bool                 `yaml:"require_approval" json:"require_approval"`
	DocumentationOnly bool                 `yaml:"documentation_only" json:"documentation_only"`
}

// AgentInvocationSpec holds recursion data.
type AgentInvocationSpec struct {
	CanInvokeSubagents bool     `yaml:"can_invoke_subagents" json:"can_invoke_subagents"`
	AllowedSubagents   []string `yaml:"allowed_subagents" json:"allowed_subagents"`
	MaxDepth           int      `yaml:"max_depth" json:"max_depth"`
}

// AgentCoordinationSpec is the canonical configuration surface for delegation,
// handoff, and tiered projection policy. Invocation remains as a compatibility
// input for older manifests.
type AgentCoordinationSpec struct {
	Enabled                   bool                  `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	DelegationTargetSelectors []CapabilitySelector  `yaml:"delegation_target_selectors,omitempty" json:"delegation_target_selectors,omitempty"`
	ResourceHandoffSelectors  []CapabilitySelector  `yaml:"resource_handoff_selectors,omitempty" json:"resource_handoff_selectors,omitempty"`
	MaxDelegationDepth        int                   `yaml:"max_delegation_depth,omitempty" json:"max_delegation_depth,omitempty"`
	AllowRemoteDelegation     bool                  `yaml:"allow_remote_delegation,omitempty" json:"allow_remote_delegation,omitempty"`
	AllowBackgroundDelegation bool                  `yaml:"allow_background_delegation,omitempty" json:"allow_background_delegation,omitempty"`
	RequireApprovalCrossTrust bool                  `yaml:"require_approval_cross_trust,omitempty" json:"require_approval_cross_trust,omitempty"`
	Projection                AgentProjectionPolicy `yaml:"projection,omitempty" json:"projection,omitempty"`
	ScaleOut                  AgentScaleOutPolicy   `yaml:"scale_out,omitempty" json:"scale_out,omitempty"`
}

type AgentProjectionPolicy struct {
	Hot      AgentProjectionTier `yaml:"hot,omitempty" json:"hot,omitempty"`
	Warm     AgentProjectionTier `yaml:"warm,omitempty" json:"warm,omitempty"`
	Cold     AgentProjectionTier `yaml:"cold,omitempty" json:"cold,omitempty"`
	Strategy string              `yaml:"strategy,omitempty" json:"strategy,omitempty"`
}

type AgentProjectionTier struct {
	MaxItems       int      `yaml:"max_items,omitempty" json:"max_items,omitempty"`
	MaxTokens      int      `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
	MaxBytes       int64    `yaml:"max_bytes,omitempty" json:"max_bytes,omitempty"`
	Persist        bool     `yaml:"persist,omitempty" json:"persist,omitempty"`
	ResourceScopes []string `yaml:"resource_scopes,omitempty" json:"resource_scopes,omitempty"`
}

type AgentScaleOutPolicy struct {
	Mode                string            `yaml:"mode,omitempty" json:"mode,omitempty"`
	PreferredModelClass string            `yaml:"preferred_model_class,omitempty" json:"preferred_model_class,omitempty"`
	PreferredProviders  []string          `yaml:"preferred_providers,omitempty" json:"preferred_providers,omitempty"`
	Metadata            map[string]string `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// AgentArtifactWindowSpec limits the artifact streaming window.
type AgentArtifactWindowSpec struct {
	MaxFiles            int    `yaml:"max_files" json:"max_files"`
	MaxTokens           int    `yaml:"max_tokens" json:"max_tokens"`
	IncludeGitHistory   bool   `yaml:"include_git_history" json:"include_git_history"`
	IncludeDependencies bool   `yaml:"include_dependencies" json:"include_dependencies"`
	CompressionStrategy string `yaml:"compression_strategy" json:"compression_strategy"` // "summary", "truncate", "hybrid"
	ProgressiveLoading  *bool  `yaml:"progressive_loading,omitempty" json:"progressive_loading,omitempty"`
}

// AgentBrowserSpec configures the model-facing browser tool and its action
// policies without bypassing manifest network/filesystem enforcement. The
// action policy map only accepts browser actions that are implemented end to
// end by the browser service.
type AgentBrowserSpec struct {
	Enabled         bool                            `yaml:"enabled" json:"enabled"`
	DefaultBackend  string                          `yaml:"default_backend,omitempty" json:"default_backend,omitempty"`
	AllowedBackends []string                        `yaml:"allowed_backends,omitempty" json:"allowed_backends,omitempty"`
	Actions         map[string]AgentPermissionLevel `yaml:"actions,omitempty" json:"actions,omitempty"`
	Extraction      AgentBrowserExtractionSpec      `yaml:"extraction,omitempty" json:"extraction,omitempty"`
	Downloads       AgentBrowserDownloadSpec        `yaml:"downloads,omitempty" json:"downloads,omitempty"`
	Credentials     AgentBrowserCredentialsSpec     `yaml:"credentials,omitempty" json:"credentials,omitempty"`
}

type AgentBrowserExtractionSpec struct {
	DefaultMode       string `yaml:"default_mode,omitempty" json:"default_mode,omitempty"`
	MaxHTMLTokens     int    `yaml:"max_html_tokens,omitempty" json:"max_html_tokens,omitempty"`
	MaxSnapshotTokens int    `yaml:"max_snapshot_tokens,omitempty" json:"max_snapshot_tokens,omitempty"`
}

type AgentBrowserDownloadSpec struct {
	Enabled   bool   `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Directory string `yaml:"directory,omitempty" json:"directory,omitempty"`
}

type AgentBrowserCredentialsSpec struct {
	RequireHITL bool `yaml:"require_hitl,omitempty" json:"require_hitl,omitempty"`
}

// AgentSkillConfig carries skill-derived agent policy hints. These hints may
// narrow behavior but must never bypass registry permissions or sandbox rules.
type AgentSkillConfig struct {
	PhaseCapabilities        map[string][]string                  `yaml:"phase_capabilities,omitempty" json:"phase_capabilities,omitempty"`
	PhaseCapabilitySelectors map[string][]SkillCapabilitySelector `yaml:"phase_capability_selectors,omitempty" json:"phase_capability_selectors,omitempty"`
	Verification             AgentVerificationPolicy              `yaml:"verification,omitempty" json:"verification,omitempty"`
	Recovery                 AgentRecoveryPolicy                  `yaml:"recovery,omitempty" json:"recovery,omitempty"`
	Planning                 AgentPlanningPolicy                  `yaml:"planning,omitempty" json:"planning,omitempty"`
	Review                   AgentReviewPolicy                    `yaml:"review,omitempty" json:"review,omitempty"`
	ContextHints             AgentSkillContextHints               `yaml:"context_hints,omitempty" json:"context_hints,omitempty"`
}

type AgentVerificationPolicy struct {
	SuccessTools               []string                  `yaml:"success_tools,omitempty" json:"success_tools,omitempty"`
	SuccessCapabilitySelectors []SkillCapabilitySelector `yaml:"success_capability_selectors,omitempty" json:"success_capability_selectors,omitempty"`
	StopOnSuccess              bool                      `yaml:"stop_on_success,omitempty" json:"stop_on_success,omitempty"`
}

type AgentRecoveryPolicy struct {
	FailureProbeTools               []string                  `yaml:"failure_probe_tools,omitempty" json:"failure_probe_tools,omitempty"`
	FailureProbeCapabilitySelectors []SkillCapabilitySelector `yaml:"failure_probe_capability_selectors,omitempty" json:"failure_probe_capability_selectors,omitempty"`
}

type AgentPlanningPolicy struct {
	RequiredBeforeEdit          []SkillCapabilitySelector `yaml:"required_before_edit,omitempty" json:"required_before_edit,omitempty"`
	PreferredEditCapabilities   []SkillCapabilitySelector `yaml:"preferred_edit_capabilities,omitempty" json:"preferred_edit_capabilities,omitempty"`
	PreferredVerifyCapabilities []SkillCapabilitySelector `yaml:"preferred_verify_capabilities,omitempty" json:"preferred_verify_capabilities,omitempty"`
	StepTemplates               []SkillStepTemplate       `yaml:"step_templates,omitempty" json:"step_templates,omitempty"`
	RequireVerificationStep     bool                      `yaml:"require_verification_step,omitempty" json:"require_verification_step,omitempty"`
}

type SkillStepTemplate struct {
	Kind        string `yaml:"kind,omitempty" json:"kind,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

type AgentReviewPolicy struct {
	Criteria        []string                 `yaml:"criteria,omitempty" json:"criteria,omitempty"`
	FocusTags       []string                 `yaml:"focus_tags,omitempty" json:"focus_tags,omitempty"`
	ApprovalRules   AgentReviewApprovalRules `yaml:"approval_rules,omitempty" json:"approval_rules,omitempty"`
	SeverityWeights map[string]float64       `yaml:"severity_weights,omitempty" json:"severity_weights,omitempty"`
}

type AgentReviewApprovalRules struct {
	RequireVerificationEvidence bool `yaml:"require_verification_evidence,omitempty" json:"require_verification_evidence,omitempty"`
	RejectOnUnresolvedErrors    bool `yaml:"reject_on_unresolved_errors,omitempty" json:"reject_on_unresolved_errors,omitempty"`
}

type AgentSkillContextHints struct {
	PreferredDetailLevel string   `yaml:"preferred_detail_level,omitempty" json:"preferred_detail_level,omitempty"`
	ProtectPatterns      []string `yaml:"protect_patterns,omitempty" json:"protect_patterns,omitempty"`
}

type SkillCapabilitySelector struct {
	Capability      string                    `yaml:"capability,omitempty" json:"capability,omitempty"`
	RuntimeFamilies []CapabilityRuntimeFamily `yaml:"runtime_families,omitempty" json:"runtime_families,omitempty"`
	Tags            []string                  `yaml:"tags,omitempty" json:"tags,omitempty"`
	ExcludeTags     []string                  `yaml:"exclude_tags,omitempty" json:"exclude_tags,omitempty"`
}

// AgentMetadata captures auxiliary metadata for display.
type AgentMetadata struct {
	Author   string   `yaml:"author" json:"author"`
	Tags     []string `yaml:"tags" json:"tags"`
	Priority int      `yaml:"priority" json:"priority"`
}

// AgentPermissionLevel enumerates allow/deny/ask.
type AgentPermissionLevel string

const (
	AgentPermissionAllow AgentPermissionLevel = "allow"
	AgentPermissionDeny  AgentPermissionLevel = "deny"
	AgentPermissionAsk   AgentPermissionLevel = "ask"
)

// Validate ensures the agent runtime section is well-formed.
func (a *AgentRuntimeSpec) Validate() error {
	if a == nil {
		return nil
	}
	if a.Mode == "" {
		return fmt.Errorf("agent mode required")
	}
	switch a.Mode {
	case AgentModePrimary, AgentModeSub, AgentModeSystem:
	default:
		return fmt.Errorf("invalid agent mode %s", a.Mode)
	}
	if err := a.Model.Validate(); err != nil {
		return fmt.Errorf("model invalid: %w", err)
	}
	switch a.ToolCallingIntent {
	case "", ToolCallingIntentAuto, ToolCallingIntentPreferNative, ToolCallingIntentPreferPrompt:
	default:
		return fmt.Errorf("tool_calling_intent %q invalid", a.ToolCallingIntent)
	}
	for name, policy := range a.ToolExecutionPolicy {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("tool policy contains empty tool name")
		}
		switch policy.Execute {
		case AgentPermissionAllow, AgentPermissionAsk, AgentPermissionDeny, "":
		default:
			return fmt.Errorf("tool policy %s execute=%s invalid", name, policy.Execute)
		}
	}
	for i, policy := range a.CapabilityPolicies {
		if err := ValidateCapabilityPolicy(policy); err != nil {
			return fmt.Errorf("capability_policies[%d] invalid: %w", i, err)
		}
	}
	for i, policy := range a.ExposurePolicies {
		if err := ValidateCapabilityExposurePolicy(policy); err != nil {
			return fmt.Errorf("exposure_policies[%d] invalid: %w", i, err)
		}
	}
	for i, policy := range a.InsertionPolicies {
		if err := ValidateCapabilityInsertionPolicy(policy); err != nil {
			return fmt.Errorf("insertion_policies[%d] invalid: %w", i, err)
		}
	}
	seenSessionPolicyIDs := make(map[string]struct{}, len(a.SessionPolicies))
	for i, policy := range a.SessionPolicies {
		if err := ValidateSessionPolicy(policy); err != nil {
			return fmt.Errorf("session_policies[%d] invalid: %w", i, err)
		}
		if _, exists := seenSessionPolicyIDs[policy.ID]; exists {
			return fmt.Errorf("session_policies[%d] duplicates id %q", i, policy.ID)
		}
		seenSessionPolicyIDs[policy.ID] = struct{}{}
	}
	for key, level := range a.GlobalPolicies {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("policies contains empty key")
		}
		if err := ValidatePolicyClassKey(key); err != nil {
			return fmt.Errorf("policies[%s] invalid: %w", key, err)
		}
		switch level {
		case AgentPermissionAllow, AgentPermissionAsk, AgentPermissionDeny, "":
		default:
			return fmt.Errorf("policies[%s]=%s invalid", key, level)
		}
	}
	for providerID, policy := range a.ProviderPolicies {
		if strings.TrimSpace(providerID) == "" {
			return fmt.Errorf("provider_policies contains empty provider ID")
		}
		if err := ValidateProviderPolicy(policy); err != nil {
			return fmt.Errorf("provider_policies[%s] invalid: %w", providerID, err)
		}
	}
	for idx, provider := range a.Providers {
		if err := provider.Validate(); err != nil {
			return fmt.Errorf("providers[%d] invalid: %w", idx, err)
		}
	}
	if a.RuntimeSafety != nil {
		if err := a.RuntimeSafety.Validate(); err != nil {
			return fmt.Errorf("runtime_safety invalid: %w", err)
		}
	}
	if err := a.Coordination.Validate(a.Invocation); err != nil {
		return fmt.Errorf("coordination invalid: %w", err)
	}
	if a.Composition != nil {
		if err := a.Composition.Validate(); err != nil {
			return fmt.Errorf("composition invalid: %w", err)
		}
	}
	for _, selector := range a.AllowedCapabilities {
		if err := ValidateCapabilitySelector(selector); err != nil {
			return fmt.Errorf("allowed_capabilities invalid: %w", err)
		}
	}
	for phase, tools := range a.SkillConfig.PhaseCapabilities {
		if strings.TrimSpace(phase) == "" {
			return fmt.Errorf("skill_manifest.phase_capabilities contains empty phase")
		}
		for _, tool := range tools {
			if strings.TrimSpace(tool) == "" {
				return fmt.Errorf("skill_manifest.phase_capabilities[%s] contains empty capability", phase)
			}
		}
	}
	for phase, selectors := range a.SkillConfig.PhaseCapabilitySelectors {
		if strings.TrimSpace(phase) == "" {
			return fmt.Errorf("skill_manifest.phase_capability_selectors contains empty phase")
		}
		for _, selector := range selectors {
			if err := ValidateSkillCapabilitySelector(selector); err != nil {
				return fmt.Errorf("skill_manifest.phase_capability_selectors[%s] invalid: %w", phase, err)
			}
		}
	}
	for _, tool := range a.SkillConfig.Verification.SuccessTools {
		if strings.TrimSpace(tool) == "" {
			return fmt.Errorf("skill_manifest.verification.success_tools contains empty tool")
		}
	}
	for _, selector := range a.SkillConfig.Verification.SuccessCapabilitySelectors {
		if err := ValidateSkillCapabilitySelector(selector); err != nil {
			return fmt.Errorf("skill_manifest.verification.success_capability_selectors invalid: %w", err)
		}
	}
	for _, tool := range a.SkillConfig.Recovery.FailureProbeTools {
		if strings.TrimSpace(tool) == "" {
			return fmt.Errorf("skill_manifest.recovery.failure_probe_tools contains empty tool")
		}
	}
	for _, selector := range a.SkillConfig.Recovery.FailureProbeCapabilitySelectors {
		if err := ValidateSkillCapabilitySelector(selector); err != nil {
			return fmt.Errorf("skill_manifest.recovery.failure_probe_capability_selectors invalid: %w", err)
		}
	}
	for _, selector := range a.SkillConfig.Planning.RequiredBeforeEdit {
		if err := ValidateSkillCapabilitySelector(selector); err != nil {
			return fmt.Errorf("skill_manifest.planning.required_before_edit invalid: %w", err)
		}
	}
	for _, selector := range a.SkillConfig.Planning.PreferredEditCapabilities {
		if err := ValidateSkillCapabilitySelector(selector); err != nil {
			return fmt.Errorf("skill_manifest.planning.preferred_edit_capabilities invalid: %w", err)
		}
	}
	for _, selector := range a.SkillConfig.Planning.PreferredVerifyCapabilities {
		if err := ValidateSkillCapabilitySelector(selector); err != nil {
			return fmt.Errorf("skill_manifest.planning.preferred_verify_capabilities invalid: %w", err)
		}
	}
	for _, step := range a.SkillConfig.Planning.StepTemplates {
		if strings.TrimSpace(step.Kind) == "" {
			return fmt.Errorf("skill_manifest.planning.step_templates contains empty kind")
		}
		if strings.TrimSpace(step.Description) == "" {
			return fmt.Errorf("skill_manifest.planning.step_templates[%s] contains empty description", step.Kind)
		}
	}
	for _, criterion := range a.SkillConfig.Review.Criteria {
		if strings.TrimSpace(criterion) == "" {
			return fmt.Errorf("skill_manifest.review.criteria contains empty criterion")
		}
	}
	for _, tag := range a.SkillConfig.Review.FocusTags {
		if strings.TrimSpace(tag) == "" {
			return fmt.Errorf("skill_manifest.review.focus_tags contains empty tag")
		}
	}
	for severity, weight := range a.SkillConfig.Review.SeverityWeights {
		if strings.TrimSpace(severity) == "" {
			return fmt.Errorf("skill_manifest.review.severity_weights contains empty severity")
		}
		if weight < 0 {
			return fmt.Errorf("skill_manifest.review.severity_weights[%s] must be >= 0", severity)
		}
	}
	if a.Browser != nil {
		if err := a.Browser.Validate(); err != nil {
			return fmt.Errorf("browser config invalid: %w", err)
		}
	}
	if err := a.Files.Validate(); err != nil {
		return err
	}
	return nil
}

func resolveToolCallingIntent(intent ToolCallingIntent, legacy *bool) ToolCallingIntent {
	switch intent {
	case ToolCallingIntentAuto, ToolCallingIntentPreferNative, ToolCallingIntentPreferPrompt:
		return intent
	case "":
		if legacy != nil {
			if *legacy {
				return ToolCallingIntentPreferNative
			}
			return ToolCallingIntentPreferPrompt
		}
		return ToolCallingIntentAuto
	default:
		return intent
	}
}

func ValidateCapabilityPolicy(policy CapabilityPolicy) error {
	if err := ValidateCapabilitySelector(policy.Selector); err != nil {
		return err
	}
	switch policy.Execute {
	case AgentPermissionAllow, AgentPermissionAsk, AgentPermissionDeny, "":
		return nil
	default:
		return fmt.Errorf("execute=%s invalid", policy.Execute)
	}
}

func ValidateCapabilityExposurePolicy(policy CapabilityExposurePolicy) error {
	if err := ValidateCapabilitySelector(policy.Selector); err != nil {
		return err
	}
	switch policy.Access {
	case CapabilityExposureHidden, CapabilityExposureInspectable, CapabilityExposureCallable:
		return nil
	default:
		return fmt.Errorf("access=%s invalid", policy.Access)
	}
}

func ValidateCapabilitySelector(selector CapabilitySelector) error {
	if strings.TrimSpace(selector.ID) == "" &&
		strings.TrimSpace(selector.Name) == "" &&
		selector.Kind == "" &&
		len(selector.RuntimeFamilies) == 0 &&
		len(selector.Tags) == 0 &&
		len(selector.ExcludeTags) == 0 &&
		len(selector.SourceScopes) == 0 &&
		len(selector.TrustClasses) == 0 &&
		len(selector.RiskClasses) == 0 &&
		len(selector.EffectClasses) == 0 &&
		len(selector.CoordinationRoles) == 0 &&
		len(selector.CoordinationTaskTypes) == 0 &&
		len(selector.CoordinationExecutionModes) == 0 &&
		selector.CoordinationLongRunning == nil &&
		selector.CoordinationDirectInsertion == nil {
		return fmt.Errorf("selector must declare at least one match field")
	}
	for _, tag := range append([]string{}, selector.Tags...) {
		if strings.TrimSpace(tag) == "" {
			return fmt.Errorf("selector contains empty tag")
		}
	}
	for _, tag := range selector.ExcludeTags {
		if strings.TrimSpace(tag) == "" {
			return fmt.Errorf("selector contains empty tag")
		}
	}
	for _, taskType := range selector.CoordinationTaskTypes {
		if strings.TrimSpace(taskType) == "" {
			return fmt.Errorf("selector contains empty coordination task type")
		}
	}
	for _, scope := range selector.SourceScopes {
		switch scope {
		case CapabilityScopeBuiltin, CapabilityScopeWorkspace, CapabilityScopeProvider, CapabilityScopeRemote:
		default:
			return fmt.Errorf("source scope %s invalid", scope)
		}
	}
	for _, family := range selector.RuntimeFamilies {
		switch family {
		case CapabilityRuntimeFamilyLocalTool, CapabilityRuntimeFamilyProvider, CapabilityRuntimeFamilyRelurpic:
		default:
			return fmt.Errorf("runtime family %s invalid", family)
		}
	}
	for _, trust := range selector.TrustClasses {
		switch trust {
		case TrustClassBuiltinTrusted, TrustClassWorkspaceTrusted, TrustClassProviderLocalUntrusted, TrustClassRemoteDeclared, TrustClassRemoteApproved:
		default:
			return fmt.Errorf("trust class %s invalid", trust)
		}
	}
	for _, risk := range selector.RiskClasses {
		switch risk {
		case RiskClassReadOnly, RiskClassDestructive, RiskClassExecute, RiskClassNetwork, RiskClassCredentialed, RiskClassExfiltration, RiskClassSessioned:
		default:
			return fmt.Errorf("risk class %s invalid", risk)
		}
	}
	for _, effect := range selector.EffectClasses {
		switch effect {
		case EffectClassFilesystemMutation, EffectClassProcessSpawn, EffectClassNetworkEgress, EffectClassCredentialUse, EffectClassExternalState, EffectClassSessionCreation, EffectClassContextInsertion:
		default:
			return fmt.Errorf("effect class %s invalid", effect)
		}
	}
	for _, role := range selector.CoordinationRoles {
		switch role {
		case CoordinationRolePlanner,
			CoordinationRoleArchitect,
			CoordinationRoleReviewer,
			CoordinationRoleVerifier,
			CoordinationRoleExecutor,
			CoordinationRoleDomainPack,
			CoordinationRoleBackgroundAgent:
		default:
			return fmt.Errorf("coordination role %s invalid", role)
		}
	}
	for _, mode := range selector.CoordinationExecutionModes {
		switch mode {
		case CoordinationExecutionModeSync, CoordinationExecutionModeSessionBacked, CoordinationExecutionModeBackgroundAgent:
		default:
			return fmt.Errorf("coordination execution mode %s invalid", mode)
		}
	}
	return nil
}

func EffectiveAllowedCapabilitySelectors(spec *AgentRuntimeSpec) []CapabilitySelector {
	if spec == nil {
		return nil
	}
	return CloneCapabilitySelectors(spec.AllowedCapabilities)
}

func EffectiveDelegationTargetSelectors(spec *AgentRuntimeSpec) []CapabilitySelector {
	if spec == nil {
		return nil
	}
	selectors := cloneCapabilitySelectors(spec.Coordination.DelegationTargetSelectors)
	if spec.Invocation.CanInvokeSubagents {
		for _, name := range spec.Invocation.AllowedSubagents {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			selectors = append(selectors, CapabilitySelector{
				Name:              name,
				CoordinationRoles: []CoordinationRole{CoordinationRolePlanner, CoordinationRoleArchitect, CoordinationRoleReviewer, CoordinationRoleVerifier, CoordinationRoleExecutor, CoordinationRoleDomainPack, CoordinationRoleBackgroundAgent},
			})
		}
	}
	return dedupeCapabilitySelectors(selectors)
}

func EffectiveCoordination(spec *AgentRuntimeSpec) AgentCoordinationSpec {
	if spec == nil {
		return AgentCoordinationSpec{}
	}
	coordination := cloneAgentCoordinationSpec(spec.Coordination)
	coordination.DelegationTargetSelectors = EffectiveDelegationTargetSelectors(spec)
	if coordination.MaxDelegationDepth == 0 && spec.Invocation.MaxDepth > 0 {
		coordination.MaxDelegationDepth = spec.Invocation.MaxDepth
	}
	if !coordination.Enabled && (len(coordination.DelegationTargetSelectors) > 0 || spec.Invocation.CanInvokeSubagents) {
		coordination.Enabled = true
	}
	return coordination
}

func dedupeCapabilitySelectors(input []CapabilitySelector) []CapabilitySelector {
	return MergeCapabilitySelectors(nil, input)
}

func (c AgentCoordinationSpec) Validate(invocation AgentInvocationSpec) error {
	for i, selector := range c.DelegationTargetSelectors {
		if err := ValidateCapabilitySelector(selector); err != nil {
			return fmt.Errorf("delegation_target_selectors[%d] invalid: %w", i, err)
		}
	}
	for i, selector := range c.ResourceHandoffSelectors {
		if err := ValidateCapabilitySelector(selector); err != nil {
			return fmt.Errorf("resource_handoff_selectors[%d] invalid: %w", i, err)
		}
	}
	if c.MaxDelegationDepth < 0 {
		return fmt.Errorf("max_delegation_depth must be >= 0")
	}
	if invocation.MaxDepth < 0 {
		return fmt.Errorf("invocation.max_depth must be >= 0")
	}
	if err := c.Projection.Validate(); err != nil {
		return fmt.Errorf("projection invalid: %w", err)
	}
	if err := c.ScaleOut.Validate(); err != nil {
		return fmt.Errorf("scale_out invalid: %w", err)
	}
	return nil
}

func (p AgentProjectionPolicy) Validate() error {
	switch strings.TrimSpace(strings.ToLower(p.Strategy)) {
	case "", "balanced", "memory-first", "latency-first", "persistence-first":
	default:
		return fmt.Errorf("strategy %q invalid", p.Strategy)
	}
	if err := p.Hot.validate("hot"); err != nil {
		return err
	}
	if err := p.Warm.validate("warm"); err != nil {
		return err
	}
	if err := p.Cold.validate("cold"); err != nil {
		return err
	}
	return nil
}

func (t AgentProjectionTier) validate(name string) error {
	if t.MaxItems < 0 {
		return fmt.Errorf("%s.max_items must be >= 0", name)
	}
	if t.MaxTokens < 0 {
		return fmt.Errorf("%s.max_tokens must be >= 0", name)
	}
	if t.MaxBytes < 0 {
		return fmt.Errorf("%s.max_bytes must be >= 0", name)
	}
	for _, scope := range t.ResourceScopes {
		if strings.TrimSpace(scope) == "" {
			return fmt.Errorf("%s.resource_scopes contains empty scope", name)
		}
	}
	return nil
}

func (s AgentScaleOutPolicy) Validate() error {
	switch strings.TrimSpace(strings.ToLower(s.Mode)) {
	case "", "local-only", "prefer-local", "prefer-remote", "scale-out-when-available":
	default:
		return fmt.Errorf("mode %q invalid", s.Mode)
	}
	for _, provider := range s.PreferredProviders {
		if strings.TrimSpace(provider) == "" {
			return fmt.Errorf("preferred_providers contains empty provider")
		}
	}
	for key := range s.Metadata {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("metadata contains empty key")
		}
	}
	return nil
}

func ValidateProviderPolicy(policy ProviderPolicy) error {
	switch policy.Activate {
	case AgentPermissionAllow, AgentPermissionAsk, AgentPermissionDeny, "":
	default:
		return fmt.Errorf("activate=%s invalid", policy.Activate)
	}
	switch policy.DefaultTrust {
	case "", TrustClassBuiltinTrusted, TrustClassWorkspaceTrusted, TrustClassProviderLocalUntrusted, TrustClassRemoteDeclared, TrustClassRemoteApproved:
		return nil
	default:
		return fmt.Errorf("default_trust=%s invalid", policy.DefaultTrust)
	}
}

func ValidatePolicyClassKey(key string) error {
	key = strings.ToLower(strings.TrimSpace(key))
	switch key {
	case "":
		return fmt.Errorf("class key required")
	case string(TrustClassBuiltinTrusted),
		string(TrustClassWorkspaceTrusted),
		string(TrustClassProviderLocalUntrusted),
		string(TrustClassRemoteDeclared),
		string(TrustClassRemoteApproved),
		string(CapabilityRuntimeFamilyLocalTool),
		string(CapabilityRuntimeFamilyProvider),
		string(CapabilityRuntimeFamilyRelurpic),
		string(RiskClassReadOnly),
		string(RiskClassDestructive),
		string(RiskClassExecute),
		string(RiskClassNetwork),
		string(RiskClassCredentialed),
		string(RiskClassExfiltration),
		string(RiskClassSessioned),
		string(EffectClassFilesystemMutation),
		string(EffectClassProcessSpawn),
		string(EffectClassNetworkEgress),
		string(EffectClassCredentialUse),
		string(EffectClassExternalState),
		string(EffectClassSessionCreation),
		string(EffectClassContextInsertion):
		return nil
	default:
		return fmt.Errorf("unknown capability class")
	}
}

func ValidateCapabilityInsertionPolicy(policy CapabilityInsertionPolicy) error {
	if err := ValidateCapabilitySelector(policy.Selector); err != nil {
		return err
	}
	switch policy.Action {
	case InsertionActionDirect, InsertionActionSummarized, InsertionActionMetadataOnly, InsertionActionHITLRequired, InsertionActionDenied:
		return nil
	default:
		return fmt.Errorf("action=%s invalid", policy.Action)
	}
}

var validBrowserActions = map[string]struct{}{
	"open":                   {},
	"navigate":               {},
	"click":                  {},
	"type":                   {},
	"wait":                   {},
	"extract":                {},
	"get_text":               {},
	"get_accessibility_tree": {},
	"get_html":               {},
	"current_url":            {},
	"screenshot":             {},
	"execute_js":             {},
	"close":                  {},
}

var validBrowserBackends = map[string]struct{}{
	"cdp":       {},
	"webdriver": {},
	"bidi":      {},
}

// Validate ensures the browser section contains only supported action and
// backend names.
func (b *AgentBrowserSpec) Validate() error {
	if b == nil {
		return nil
	}
	if err := validateBrowserBackendName(b.DefaultBackend, "default_backend"); err != nil {
		return err
	}
	for _, backend := range b.AllowedBackends {
		if err := validateBrowserBackendName(backend, "allowed_backends"); err != nil {
			return err
		}
	}
	for action, policy := range b.Actions {
		action = strings.TrimSpace(action)
		if action == "" {
			return fmt.Errorf("actions contains empty key")
		}
		if _, ok := validBrowserActions[action]; !ok {
			return fmt.Errorf("actions[%s] invalid", action)
		}
		switch policy {
		case AgentPermissionAllow, AgentPermissionAsk, AgentPermissionDeny, "":
		default:
			return fmt.Errorf("actions[%s] policy=%s invalid", action, policy)
		}
	}
	if b.Extraction.MaxHTMLTokens < 0 {
		return fmt.Errorf("extraction.max_html_tokens must be >= 0")
	}
	if b.Extraction.MaxSnapshotTokens < 0 {
		return fmt.Errorf("extraction.max_snapshot_tokens must be >= 0")
	}
	return nil
}

func validateBrowserBackendName(value string, field string) error {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return nil
	}
	if _, ok := validBrowserBackends[value]; ok {
		return nil
	}
	return fmt.Errorf("%s %q invalid", field, value)
}

func ValidateSkillCapabilitySelector(selector SkillCapabilitySelector) error {
	name := selector.CapabilityName()
	if name == "" && len(selector.RuntimeFamilies) == 0 && len(selector.Tags) == 0 {
		return fmt.Errorf("selector requires capability, runtime families, or tags")
	}
	if name != "" && strings.Contains(name, " ") {
		return fmt.Errorf("selector capability %q invalid", name)
	}
	for _, family := range selector.RuntimeFamilies {
		switch family {
		case CapabilityRuntimeFamilyLocalTool, CapabilityRuntimeFamilyProvider, CapabilityRuntimeFamilyRelurpic:
		default:
			return fmt.Errorf("selector runtime family %s invalid", family)
		}
	}
	for _, tag := range selector.Tags {
		if strings.TrimSpace(tag) == "" {
			return fmt.Errorf("selector contains empty tag")
		}
	}
	for _, tag := range selector.ExcludeTags {
		if strings.TrimSpace(tag) == "" {
			return fmt.Errorf("selector contains empty exclude tag")
		}
	}
	return nil
}

func (s SkillCapabilitySelector) CapabilityName() string {
	return strings.TrimSpace(s.Capability)
}

// Validate ensures model configuration is provided.
func (m AgentModelConfig) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("model name required")
	}
	if m.Provider == "" {
		return fmt.Errorf("model provider required")
	}
	return nil
}

// Validate ensures file permission sets are consistent.
func (f AgentFileMatrix) Validate() error {
	if err := f.Write.validate("write"); err != nil {
		return err
	}
	if err := f.Edit.validate("edit"); err != nil {
		return err
	}
	return nil
}

func (set AgentFilePermissionSet) validate(label string) error {
	for _, pattern := range append([]string{}, append(set.AllowPatterns, set.DenyPatterns...)...) {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			return fmt.Errorf("%s permission contains empty glob", label)
		}
		if strings.Contains(pattern, string(os.PathSeparator)+string(os.PathSeparator)) {
			return fmt.Errorf("%s permission glob %s invalid", label, pattern)
		}
	}
	switch set.Default {
	case AgentPermissionAllow, AgentPermissionAsk, AgentPermissionDeny, "":
	default:
		return fmt.Errorf("%s permission default %s invalid", label, set.Default)
	}
	return nil
}

// ContextPolicySpec defines the context policy configuration for an agent.
// This is a simplified version for agentspec; the full version is in contextpolicy.
type ContextPolicySpec struct {
	CompilationMode       string          `yaml:"compilation_mode,omitempty" json:"compilation_mode,omitempty"`
	DefaultTrustClass     TrustClass      `yaml:"default_trust_class,omitempty" json:"default_trust_class,omitempty"`
	TrustDemotedPolicy    string          `yaml:"trust_demoted_policy,omitempty" json:"trust_demoted_policy,omitempty"`
	DegradedChunkPolicy   string          `yaml:"degraded_chunk_policy,omitempty" json:"degraded_chunk_policy,omitempty"`
	BudgetShortfallPolicy string          `yaml:"budget_shortfall_policy,omitempty" json:"budget_shortfall_policy,omitempty"`
	Rankers               []RankerRef     `yaml:"rankers,omitempty" json:"rankers,omitempty"`
	Scanners              []ScannerRef    `yaml:"scanners,omitempty" json:"scanners,omitempty"`
	Summarizers           []SummarizerRef `yaml:"summarizers,omitempty" json:"summarizers,omitempty"`
}

// RankerRef references a ranker capability.
type RankerRef struct {
	ID       string         `yaml:"id" json:"id"`
	Priority int            `yaml:"priority,omitempty" json:"priority,omitempty"`
	Config   map[string]any `yaml:"config,omitempty" json:"config,omitempty"`
}

// ScannerRef references a scanner capability.
type ScannerRef struct {
	ID       string         `yaml:"id" json:"id"`
	Priority int            `yaml:"priority,omitempty" json:"priority,omitempty"`
	Config   map[string]any `yaml:"config,omitempty" json:"config,omitempty"`
}

// SummarizerRef references a summarizer capability.
type SummarizerRef struct {
	ID          string         `yaml:"id" json:"id"`
	ModelRef    string         `yaml:"model_ref,omitempty" json:"model_ref,omitempty"`
	ProseConfig map[string]any `yaml:"prose_config,omitempty" json:"prose_config,omitempty"`
	CodeConfig  map[string]any `yaml:"code_config,omitempty" json:"code_config,omitempty"`
}
