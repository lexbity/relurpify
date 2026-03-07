package core

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
	AllowedTools        []string                        `yaml:"allowed_tools,omitempty" json:"allowed_tools,omitempty"`
	ToolExecutionPolicy map[string]ToolPolicy           `yaml:"tool_execution_policy,omitempty" json:"tool_execution_policy,omitempty"`
	GlobalPolicies      map[string]AgentPermissionLevel `yaml:"policies,omitempty" json:"policies,omitempty"`
	SkillConfig         AgentSkillConfig                `yaml:"skill_config,omitempty" json:"skill_config,omitempty"`
	Bash                AgentBashPermissions            `yaml:"bash_permissions,omitempty" json:"bash_permissions,omitempty"`
	Files               AgentFileMatrix                 `yaml:"file_permissions,omitempty" json:"file_permissions,omitempty"`
	Invocation          AgentInvocationSpec             `yaml:"invocation,omitempty" json:"invocation,omitempty"`
	Context             AgentContextSpec                `yaml:"context,omitempty" json:"context,omitempty"`
	Browser             *AgentBrowserSpec               `yaml:"browser,omitempty" json:"browser,omitempty"`
	LSP                 AgentLSPSpec                    `yaml:"lsp,omitempty" json:"lsp,omitempty"`
	Search              AgentSearchSpec                 `yaml:"search,omitempty" json:"search,omitempty"`
	Metadata            AgentMetadata                   `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	OllamaToolCalling   *bool                           `yaml:"ollama_tool_calling,omitempty" json:"ollama_tool_calling,omitempty"`
	Logging             *AgentLoggingSpec               `yaml:"logging,omitempty" json:"logging,omitempty"`
}

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

// ToolCallingEnabled reports whether Ollama tool calling should be used.
func (a *AgentRuntimeSpec) ToolCallingEnabled() bool {
	if a == nil || a.OllamaToolCalling == nil {
		return true
	}
	return *a.OllamaToolCalling
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

// AgentContextSpec limits context window.
type AgentContextSpec struct {
	MaxFiles            int    `yaml:"max_files" json:"max_files"`
	MaxTokens           int    `yaml:"max_tokens" json:"max_tokens"`
	IncludeGitHistory   bool   `yaml:"include_git_history" json:"include_git_history"`
	IncludeDependencies bool   `yaml:"include_dependencies" json:"include_dependencies"`
	CompressionStrategy string `yaml:"compression_strategy" json:"compression_strategy"` // "summary", "truncate", "hybrid"
	ProgressiveLoading  bool   `yaml:"progressive_loading" json:"progressive_loading"`
}

// AgentBrowserSpec configures the model-facing browser tool and its action
// policies without bypassing manifest network/filesystem enforcement.
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
	PhaseTools     map[string][]string            `yaml:"phase_tools,omitempty" json:"phase_tools,omitempty"`
	PhaseSelectors map[string][]SkillToolSelector `yaml:"phase_selectors,omitempty" json:"phase_selectors,omitempty"`
	Verification   AgentVerificationPolicy        `yaml:"verification,omitempty" json:"verification,omitempty"`
	Recovery       AgentRecoveryPolicy            `yaml:"recovery,omitempty" json:"recovery,omitempty"`
	Planning       AgentPlanningPolicy            `yaml:"planning,omitempty" json:"planning,omitempty"`
	Review         AgentReviewPolicy              `yaml:"review,omitempty" json:"review,omitempty"`
	ContextHints   AgentSkillContextHints         `yaml:"context_hints,omitempty" json:"context_hints,omitempty"`
}

type AgentVerificationPolicy struct {
	SuccessTools     []string            `yaml:"success_tools,omitempty" json:"success_tools,omitempty"`
	SuccessSelectors []SkillToolSelector `yaml:"success_selectors,omitempty" json:"success_selectors,omitempty"`
	StopOnSuccess    bool                `yaml:"stop_on_success,omitempty" json:"stop_on_success,omitempty"`
}

type AgentRecoveryPolicy struct {
	FailureProbeTools     []string            `yaml:"failure_probe_tools,omitempty" json:"failure_probe_tools,omitempty"`
	FailureProbeSelectors []SkillToolSelector `yaml:"failure_probe_selectors,omitempty" json:"failure_probe_selectors,omitempty"`
}

type AgentPlanningPolicy struct {
	RequiredBeforeEdit      []SkillToolSelector `yaml:"required_before_edit,omitempty" json:"required_before_edit,omitempty"`
	PreferredEditTools      []SkillToolSelector `yaml:"preferred_edit_tools,omitempty" json:"preferred_edit_tools,omitempty"`
	PreferredVerifyTools    []SkillToolSelector `yaml:"preferred_verify_tools,omitempty" json:"preferred_verify_tools,omitempty"`
	StepTemplates           []SkillStepTemplate `yaml:"step_templates,omitempty" json:"step_templates,omitempty"`
	RequireVerificationStep bool                `yaml:"require_verification_step,omitempty" json:"require_verification_step,omitempty"`
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

type SkillToolSelector struct {
	Tool        string   `yaml:"tool,omitempty" json:"tool,omitempty"`
	Tags        []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	ExcludeTags []string `yaml:"exclude_tags,omitempty" json:"exclude_tags,omitempty"`
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
	for _, tool := range a.AllowedTools {
		if strings.TrimSpace(tool) == "" {
			return fmt.Errorf("allowed_tools contains empty entry")
		}
	}
	for phase, tools := range a.SkillConfig.PhaseTools {
		if strings.TrimSpace(phase) == "" {
			return fmt.Errorf("skill_config.phase_tools contains empty phase")
		}
		for _, tool := range tools {
			if strings.TrimSpace(tool) == "" {
				return fmt.Errorf("skill_config.phase_tools[%s] contains empty tool", phase)
			}
		}
	}
	for phase, selectors := range a.SkillConfig.PhaseSelectors {
		if strings.TrimSpace(phase) == "" {
			return fmt.Errorf("skill_config.phase_selectors contains empty phase")
		}
		for _, selector := range selectors {
			if err := ValidateSkillToolSelector(selector); err != nil {
				return fmt.Errorf("skill_config.phase_selectors[%s] invalid: %w", phase, err)
			}
		}
	}
	for _, tool := range a.SkillConfig.Verification.SuccessTools {
		if strings.TrimSpace(tool) == "" {
			return fmt.Errorf("skill_config.verification.success_tools contains empty tool")
		}
	}
	for _, selector := range a.SkillConfig.Verification.SuccessSelectors {
		if err := ValidateSkillToolSelector(selector); err != nil {
			return fmt.Errorf("skill_config.verification.success_selectors invalid: %w", err)
		}
	}
	for _, tool := range a.SkillConfig.Recovery.FailureProbeTools {
		if strings.TrimSpace(tool) == "" {
			return fmt.Errorf("skill_config.recovery.failure_probe_tools contains empty tool")
		}
	}
	for _, selector := range a.SkillConfig.Recovery.FailureProbeSelectors {
		if err := ValidateSkillToolSelector(selector); err != nil {
			return fmt.Errorf("skill_config.recovery.failure_probe_selectors invalid: %w", err)
		}
	}
	for _, selector := range a.SkillConfig.Planning.RequiredBeforeEdit {
		if err := ValidateSkillToolSelector(selector); err != nil {
			return fmt.Errorf("skill_config.planning.required_before_edit invalid: %w", err)
		}
	}
	for _, selector := range a.SkillConfig.Planning.PreferredEditTools {
		if err := ValidateSkillToolSelector(selector); err != nil {
			return fmt.Errorf("skill_config.planning.preferred_edit_tools invalid: %w", err)
		}
	}
	for _, selector := range a.SkillConfig.Planning.PreferredVerifyTools {
		if err := ValidateSkillToolSelector(selector); err != nil {
			return fmt.Errorf("skill_config.planning.preferred_verify_tools invalid: %w", err)
		}
	}
	for _, step := range a.SkillConfig.Planning.StepTemplates {
		if strings.TrimSpace(step.Kind) == "" {
			return fmt.Errorf("skill_config.planning.step_templates contains empty kind")
		}
		if strings.TrimSpace(step.Description) == "" {
			return fmt.Errorf("skill_config.planning.step_templates[%s] contains empty description", step.Kind)
		}
	}
	for _, criterion := range a.SkillConfig.Review.Criteria {
		if strings.TrimSpace(criterion) == "" {
			return fmt.Errorf("skill_config.review.criteria contains empty criterion")
		}
	}
	for _, tag := range a.SkillConfig.Review.FocusTags {
		if strings.TrimSpace(tag) == "" {
			return fmt.Errorf("skill_config.review.focus_tags contains empty tag")
		}
	}
	for severity, weight := range a.SkillConfig.Review.SeverityWeights {
		if strings.TrimSpace(severity) == "" {
			return fmt.Errorf("skill_config.review.severity_weights contains empty severity")
		}
		if weight < 0 {
			return fmt.Errorf("skill_config.review.severity_weights[%s] must be >= 0", severity)
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
	"list_tabs":              {},
	"switch_tab":             {},
	"wait_for_download":      {},
	"download_status":        {},
	"download":               {},
	"new_tab":                {},
	"fill_credentials":       {},
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

func ValidateSkillToolSelector(selector SkillToolSelector) error {
	if strings.TrimSpace(selector.Tool) == "" && len(selector.Tags) == 0 {
		return fmt.Errorf("selector requires tool or tags")
	}
	if strings.TrimSpace(selector.Tool) != "" && strings.Contains(selector.Tool, " ") {
		return fmt.Errorf("selector tool %q invalid", selector.Tool)
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
