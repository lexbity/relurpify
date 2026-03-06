package core

import "strings"

// AgentSpecOverlay defines optional overrides for an agent spec.
type AgentSpecOverlay struct {
	Implementation      *string                  `yaml:"implementation,omitempty" json:"implementation,omitempty"`
	Mode                *AgentMode               `yaml:"mode,omitempty" json:"mode,omitempty"`
	Version             *string                  `yaml:"version,omitempty" json:"version,omitempty"`
	Prompt              *string                  `yaml:"prompt,omitempty" json:"prompt,omitempty"`
	ModelOverlay        *AgentModelConfigOverlay `yaml:"model,omitempty" json:"model,omitempty"`
	AllowedTools        []string                 `yaml:"allowed_tools,omitempty" json:"allowed_tools,omitempty"`
	ToolExecutionPolicy map[string]ToolPolicy    `yaml:"tool_execution_policy,omitempty" json:"tool_execution_policy,omitempty"`
	SkillConfig         *AgentSkillConfig        `yaml:"skill_config,omitempty" json:"skill_config,omitempty"`
	Bash                *AgentBashPermissions    `yaml:"bash_permissions,omitempty" json:"bash_permissions,omitempty"`
	Files               *AgentFileMatrix         `yaml:"file_permissions,omitempty" json:"file_permissions,omitempty"`
	Invocation          *AgentInvocationSpec     `yaml:"invocation,omitempty" json:"invocation,omitempty"`
	ContextOverlay      *AgentContextSpecOverlay `yaml:"context,omitempty" json:"context,omitempty"`
	LSPOverlay          *AgentLSPSpecOverlay     `yaml:"lsp,omitempty" json:"lsp,omitempty"`
	SearchOverlay       *AgentSearchSpecOverlay  `yaml:"search,omitempty" json:"search,omitempty"`
	Metadata            *AgentMetadata           `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	OllamaToolCalling   *bool                    `yaml:"ollama_tool_calling,omitempty" json:"ollama_tool_calling,omitempty"`
	Logging             *AgentLoggingSpec        `yaml:"logging,omitempty" json:"logging,omitempty"`
}

// MergeAgentSpecs applies overlays to a base spec in order.
func MergeAgentSpecs(base *AgentRuntimeSpec, overlays ...AgentSpecOverlay) *AgentRuntimeSpec {
	spec := cloneAgentSpec(base)
	for _, overlay := range overlays {
		applyAgentSpecOverlay(spec, overlay)
	}
	return spec
}

// AgentSpecOverlayFromSpec converts a concrete spec into a full overlay.
func AgentSpecOverlayFromSpec(spec *AgentRuntimeSpec) AgentSpecOverlay {
	if spec == nil {
		return AgentSpecOverlay{}
	}
	implementation := spec.Implementation
	mode := spec.Mode
	version := spec.Version
	prompt := spec.Prompt
	modelOverlay := AgentModelConfigOverlay{
		Provider:    &spec.Model.Provider,
		Name:        &spec.Model.Name,
		Temperature: &spec.Model.Temperature,
		MaxTokens:   &spec.Model.MaxTokens,
	}
	var allowedTools []string
	if spec.AllowedTools != nil {
		allowedTools = append([]string{}, spec.AllowedTools...)
	}
	bash := spec.Bash
	files := spec.Files
	invocation := spec.Invocation
	contextOverlay := AgentContextSpecOverlay{
		MaxFiles:            &spec.Context.MaxFiles,
		MaxTokens:           &spec.Context.MaxTokens,
		IncludeGitHistory:   &spec.Context.IncludeGitHistory,
		IncludeDependencies: &spec.Context.IncludeDependencies,
		CompressionStrategy: &spec.Context.CompressionStrategy,
		ProgressiveLoading:  &spec.Context.ProgressiveLoading,
	}
	lspOverlay := AgentLSPSpecOverlay{
		Servers: spec.LSP.Servers,
		Enabled: &spec.LSP.Enabled,
		Timeout: &spec.LSP.Timeout,
	}
	searchOverlay := AgentSearchSpecOverlay{
		HybridEnabled: &spec.Search.HybridEnabled,
		VectorIndex:   &spec.Search.VectorIndex,
		ASTIndex:      &spec.Search.ASTIndex,
	}
	skillConfig := cloneAgentSkillConfig(spec.SkillConfig)
	metadata := spec.Metadata
	toolCalling := spec.OllamaToolCalling
	var logging *AgentLoggingSpec
	if spec.Logging != nil {
		llm := spec.Logging.LLM
		agent := spec.Logging.Agent
		logging = &AgentLoggingSpec{LLM: llm, Agent: agent}
	}
	return AgentSpecOverlay{
		Implementation:      &implementation,
		Mode:                &mode,
		Version:             &version,
		Prompt:              &prompt,
		ModelOverlay:        &modelOverlay,
		AllowedTools:        allowedTools,
		ToolExecutionPolicy: cloneToolPolicies(spec.ToolExecutionPolicy),
		SkillConfig:         &skillConfig,
		Bash:                &bash,
		Files:               &files,
		Invocation:          &invocation,
		ContextOverlay:      &contextOverlay,
		LSPOverlay:          &lspOverlay,
		SearchOverlay:       &searchOverlay,
		Metadata:            &metadata,
		OllamaToolCalling:   toolCalling,
		Logging:             logging,
	}
}

func applyAgentSpecOverlay(spec *AgentRuntimeSpec, overlay AgentSpecOverlay) {
	if overlay.Implementation != nil {
		spec.Implementation = *overlay.Implementation
	}
	if overlay.Mode != nil {
		spec.Mode = *overlay.Mode
	}
	if overlay.Version != nil {
		spec.Version = *overlay.Version
	}
	if overlay.Prompt != nil {
		spec.Prompt = *overlay.Prompt
	}
	if overlay.AllowedTools != nil {
		if len(overlay.AllowedTools) == 0 {
			spec.AllowedTools = []string{}
		} else {
			spec.AllowedTools = mergeStringList(spec.AllowedTools, overlay.AllowedTools)
		}
	}
	if overlay.ToolExecutionPolicy != nil {
		if spec.ToolExecutionPolicy == nil {
			spec.ToolExecutionPolicy = make(map[string]ToolPolicy, len(overlay.ToolExecutionPolicy))
		}
		for name, policy := range overlay.ToolExecutionPolicy {
			spec.ToolExecutionPolicy[name] = policy
		}
	}
	if overlay.SkillConfig != nil {
		spec.SkillConfig = mergeAgentSkillConfig(spec.SkillConfig, *overlay.SkillConfig)
	}
	if overlay.Bash != nil {
		spec.Bash = *overlay.Bash
	}
	if overlay.Files != nil {
		spec.Files = *overlay.Files
	}
	if overlay.Invocation != nil {
		spec.Invocation = *overlay.Invocation
	}
	if overlay.ModelOverlay != nil {
		spec.Model = MergeAgentModelConfig(spec.Model, *overlay.ModelOverlay)
	}
	if overlay.ContextOverlay != nil {
		spec.Context = MergeAgentContextSpec(spec.Context, *overlay.ContextOverlay)
	}
	if overlay.LSPOverlay != nil {
		spec.LSP = MergeAgentLSPSpec(spec.LSP, *overlay.LSPOverlay)
	}
	if overlay.SearchOverlay != nil {
		spec.Search = MergeAgentSearchSpec(spec.Search, *overlay.SearchOverlay)
	}
	if overlay.Metadata != nil {
		spec.Metadata = *overlay.Metadata
	}
	if overlay.OllamaToolCalling != nil {
		spec.OllamaToolCalling = overlay.OllamaToolCalling
	}
	if overlay.Logging != nil {
		if spec.Logging == nil {
			spec.Logging = &AgentLoggingSpec{}
		}
		if overlay.Logging.LLM != nil {
			spec.Logging.LLM = overlay.Logging.LLM
		}
		if overlay.Logging.Agent != nil {
			spec.Logging.Agent = overlay.Logging.Agent
		}
	}
}

func cloneAgentSpec(spec *AgentRuntimeSpec) *AgentRuntimeSpec {
	if spec == nil {
		return &AgentRuntimeSpec{}
	}
	clone := *spec
	if spec.ToolExecutionPolicy != nil {
		clone.ToolExecutionPolicy = cloneToolPolicies(spec.ToolExecutionPolicy)
	}
	clone.SkillConfig = cloneAgentSkillConfig(spec.SkillConfig)
	if spec.AllowedTools != nil {
		clone.AllowedTools = append([]string{}, spec.AllowedTools...)
	}
	clone.Bash.AllowPatterns = append([]string{}, spec.Bash.AllowPatterns...)
	clone.Bash.DenyPatterns = append([]string{}, spec.Bash.DenyPatterns...)
	clone.Files.Write.AllowPatterns = append([]string{}, spec.Files.Write.AllowPatterns...)
	clone.Files.Write.DenyPatterns = append([]string{}, spec.Files.Write.DenyPatterns...)
	clone.Files.Edit.AllowPatterns = append([]string{}, spec.Files.Edit.AllowPatterns...)
	clone.Files.Edit.DenyPatterns = append([]string{}, spec.Files.Edit.DenyPatterns...)
	clone.Invocation.AllowedSubagents = append([]string{}, spec.Invocation.AllowedSubagents...)
	clone.Metadata.Tags = append([]string{}, spec.Metadata.Tags...)
	if spec.Logging != nil {
		llm := spec.Logging.LLM
		agent := spec.Logging.Agent
		clone.Logging = &AgentLoggingSpec{LLM: llm, Agent: agent}
	}
	if spec.LSP.Servers != nil {
		clone.LSP.Servers = make(map[string]string, len(spec.LSP.Servers))
		for k, v := range spec.LSP.Servers {
			clone.LSP.Servers[k] = v
		}
	}
	return &clone
}

func mergeStringList(base, extra []string) []string {
	if len(extra) == 0 {
		return base
	}
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, entry := range append(base, extra...) {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		out = append(out, entry)
	}
	return out
}

func cloneToolPolicies(policies map[string]ToolPolicy) map[string]ToolPolicy {
	if policies == nil {
		return nil
	}
	clone := make(map[string]ToolPolicy, len(policies))
	for name, policy := range policies {
		clone[name] = policy
	}
	return clone
}

func cloneAgentSkillConfig(input AgentSkillConfig) AgentSkillConfig {
	out := AgentSkillConfig{
		Verification: input.Verification,
		Recovery:     input.Recovery,
		Planning:     input.Planning,
		Review:       input.Review,
		ContextHints: input.ContextHints,
	}
	if input.PhaseTools != nil {
		out.PhaseTools = make(map[string][]string, len(input.PhaseTools))
		for phase, tools := range input.PhaseTools {
			out.PhaseTools[phase] = append([]string{}, tools...)
		}
	}
	if input.PhaseSelectors != nil {
		out.PhaseSelectors = make(map[string][]SkillToolSelector, len(input.PhaseSelectors))
		for phase, selectors := range input.PhaseSelectors {
			out.PhaseSelectors[phase] = append([]SkillToolSelector{}, selectors...)
		}
	}
	out.Verification.SuccessTools = append([]string{}, input.Verification.SuccessTools...)
	out.Verification.SuccessSelectors = append([]SkillToolSelector{}, input.Verification.SuccessSelectors...)
	out.Recovery.FailureProbeTools = append([]string{}, input.Recovery.FailureProbeTools...)
	out.Recovery.FailureProbeSelectors = append([]SkillToolSelector{}, input.Recovery.FailureProbeSelectors...)
	out.Planning.RequiredBeforeEdit = append([]SkillToolSelector{}, input.Planning.RequiredBeforeEdit...)
	out.Planning.PreferredEditTools = append([]SkillToolSelector{}, input.Planning.PreferredEditTools...)
	out.Planning.PreferredVerifyTools = append([]SkillToolSelector{}, input.Planning.PreferredVerifyTools...)
	out.Planning.StepTemplates = append([]SkillStepTemplate{}, input.Planning.StepTemplates...)
	out.Review.Criteria = append([]string{}, input.Review.Criteria...)
	out.Review.FocusTags = append([]string{}, input.Review.FocusTags...)
	if input.Review.SeverityWeights != nil {
		out.Review.SeverityWeights = make(map[string]float64, len(input.Review.SeverityWeights))
		for k, v := range input.Review.SeverityWeights {
			out.Review.SeverityWeights[k] = v
		}
	}
	out.ContextHints.ProtectPatterns = append([]string{}, input.ContextHints.ProtectPatterns...)
	return out
}

func mergeAgentSkillConfig(base, overlay AgentSkillConfig) AgentSkillConfig {
	merged := cloneAgentSkillConfig(base)
	if overlay.PhaseTools != nil {
		if merged.PhaseTools == nil {
			merged.PhaseTools = make(map[string][]string, len(overlay.PhaseTools))
		}
		for phase, tools := range overlay.PhaseTools {
			merged.PhaseTools[phase] = mergeStringList(merged.PhaseTools[phase], tools)
		}
	}
	if overlay.PhaseSelectors != nil {
		if merged.PhaseSelectors == nil {
			merged.PhaseSelectors = make(map[string][]SkillToolSelector, len(overlay.PhaseSelectors))
		}
		for phase, selectors := range overlay.PhaseSelectors {
			merged.PhaseSelectors[phase] = mergeSkillToolSelectors(merged.PhaseSelectors[phase], selectors)
		}
	}
	merged.Verification.SuccessTools = mergeStringList(merged.Verification.SuccessTools, overlay.Verification.SuccessTools)
	merged.Verification.SuccessSelectors = mergeSkillToolSelectors(merged.Verification.SuccessSelectors, overlay.Verification.SuccessSelectors)
	merged.Verification.StopOnSuccess = merged.Verification.StopOnSuccess || overlay.Verification.StopOnSuccess
	merged.Recovery.FailureProbeTools = mergeStringList(merged.Recovery.FailureProbeTools, overlay.Recovery.FailureProbeTools)
	merged.Recovery.FailureProbeSelectors = mergeSkillToolSelectors(merged.Recovery.FailureProbeSelectors, overlay.Recovery.FailureProbeSelectors)
	merged.Planning.RequiredBeforeEdit = mergeSkillToolSelectors(merged.Planning.RequiredBeforeEdit, overlay.Planning.RequiredBeforeEdit)
	merged.Planning.PreferredEditTools = mergeSkillToolSelectors(merged.Planning.PreferredEditTools, overlay.Planning.PreferredEditTools)
	merged.Planning.PreferredVerifyTools = mergeSkillToolSelectors(merged.Planning.PreferredVerifyTools, overlay.Planning.PreferredVerifyTools)
	merged.Planning.StepTemplates = mergeStepTemplates(merged.Planning.StepTemplates, overlay.Planning.StepTemplates)
	merged.Planning.RequireVerificationStep = merged.Planning.RequireVerificationStep || overlay.Planning.RequireVerificationStep
	merged.Review.Criteria = mergeStringList(merged.Review.Criteria, overlay.Review.Criteria)
	merged.Review.FocusTags = mergeStringList(merged.Review.FocusTags, overlay.Review.FocusTags)
	merged.Review.ApprovalRules.RequireVerificationEvidence = merged.Review.ApprovalRules.RequireVerificationEvidence || overlay.Review.ApprovalRules.RequireVerificationEvidence
	merged.Review.ApprovalRules.RejectOnUnresolvedErrors = merged.Review.ApprovalRules.RejectOnUnresolvedErrors || overlay.Review.ApprovalRules.RejectOnUnresolvedErrors
	if overlay.Review.SeverityWeights != nil {
		if merged.Review.SeverityWeights == nil {
			merged.Review.SeverityWeights = make(map[string]float64, len(overlay.Review.SeverityWeights))
		}
		for k, v := range overlay.Review.SeverityWeights {
			merged.Review.SeverityWeights[k] = v
		}
	}
	if overlay.ContextHints.PreferredDetailLevel != "" {
		merged.ContextHints.PreferredDetailLevel = overlay.ContextHints.PreferredDetailLevel
	}
	merged.ContextHints.ProtectPatterns = mergeStringList(merged.ContextHints.ProtectPatterns, overlay.ContextHints.ProtectPatterns)
	return merged
}

func mergeStepTemplates(base, extra []SkillStepTemplate) []SkillStepTemplate {
	if len(extra) == 0 {
		return append([]SkillStepTemplate{}, base...)
	}
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]SkillStepTemplate, 0, len(base)+len(extra))
	for _, step := range append(append([]SkillStepTemplate{}, base...), extra...) {
		key := strings.TrimSpace(step.Kind) + "|" + strings.TrimSpace(step.Description)
		if key == "|" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, step)
	}
	return out
}

func mergeSkillToolSelectors(base, extra []SkillToolSelector) []SkillToolSelector {
	if len(extra) == 0 {
		return append([]SkillToolSelector{}, base...)
	}
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]SkillToolSelector, 0, len(base)+len(extra))
	for _, selector := range append(append([]SkillToolSelector{}, base...), extra...) {
		key := selector.Tool + "|" + strings.Join(selector.Tags, ",") + "|" + strings.Join(selector.ExcludeTags, ",")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, selector)
	}
	return out
}
