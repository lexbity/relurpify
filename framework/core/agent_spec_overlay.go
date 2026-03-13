package core

import "strings"

// AgentSpecOverlay defines optional overrides for an agent spec.
type AgentSpecOverlay struct {
	Implementation      *string                         `yaml:"implementation,omitempty" json:"implementation,omitempty"`
	Mode                *AgentMode                      `yaml:"mode,omitempty" json:"mode,omitempty"`
	Version             *string                         `yaml:"version,omitempty" json:"version,omitempty"`
	Prompt              *string                         `yaml:"prompt,omitempty" json:"prompt,omitempty"`
	ModelOverlay        *AgentModelConfigOverlay        `yaml:"model,omitempty" json:"model,omitempty"`
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
	SkillConfig         *AgentSkillConfig               `yaml:"skill_config,omitempty" json:"skill_config,omitempty"`
	Bash                *AgentBashPermissions           `yaml:"bash_permissions,omitempty" json:"bash_permissions,omitempty"`
	Files               *AgentFileMatrix                `yaml:"file_permissions,omitempty" json:"file_permissions,omitempty"`
	Invocation          *AgentInvocationSpec            `yaml:"invocation,omitempty" json:"invocation,omitempty"`
	Coordination        *AgentCoordinationSpec          `yaml:"coordination,omitempty" json:"coordination,omitempty"`
	Composition         *AgentCompositionSpec           `yaml:"composition,omitempty" json:"composition,omitempty"`
	ContextOverlay      *AgentContextSpecOverlay        `yaml:"context,omitempty" json:"context,omitempty"`
	LSPOverlay          *AgentLSPSpecOverlay            `yaml:"lsp,omitempty" json:"lsp,omitempty"`
	SearchOverlay       *AgentSearchSpecOverlay         `yaml:"search,omitempty" json:"search,omitempty"`
	Metadata            *AgentMetadata                  `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	OllamaToolCalling   *bool                           `yaml:"ollama_tool_calling,omitempty" json:"ollama_tool_calling,omitempty"`
	Logging             *AgentLoggingSpec               `yaml:"logging,omitempty" json:"logging,omitempty"`
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
	bash := spec.Bash
	files := spec.Files
	invocation := spec.Invocation
	coordination := cloneAgentCoordinationSpec(spec.Coordination)
	composition := cloneAgentCompositionSpec(spec.Composition)
	contextOverlay := AgentContextSpecOverlay{
		MaxFiles:            &spec.Context.MaxFiles,
		MaxTokens:           &spec.Context.MaxTokens,
		IncludeGitHistory:   &spec.Context.IncludeGitHistory,
		IncludeDependencies: &spec.Context.IncludeDependencies,
		CompressionStrategy: &spec.Context.CompressionStrategy,
	}
	if spec.Context.ProgressiveLoading != nil {
		progressive := *spec.Context.ProgressiveLoading
		contextOverlay.ProgressiveLoading = &progressive
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
		AllowedCapabilities: cloneCapabilitySelectors(spec.AllowedCapabilities),
		ToolExecutionPolicy: cloneToolPolicies(spec.ToolExecutionPolicy),
		CapabilityPolicies:  cloneCapabilityPolicies(spec.CapabilityPolicies),
		ExposurePolicies:    cloneExposurePolicies(spec.ExposurePolicies),
		InsertionPolicies:   cloneInsertionPolicies(spec.InsertionPolicies),
		SessionPolicies:     cloneSessionPolicies(spec.SessionPolicies),
		GlobalPolicies:      cloneGlobalPolicies(spec.GlobalPolicies),
		ProviderPolicies:    cloneProviderPolicies(spec.ProviderPolicies),
		Providers:           cloneProviderConfigs(spec.Providers),
		RuntimeSafety:       cloneRuntimeSafetySpec(spec.RuntimeSafety),
		SkillConfig:         &skillConfig,
		Bash:                &bash,
		Files:               &files,
		Invocation:          &invocation,
		Coordination:        &coordination,
		Composition:         composition,
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
	if overlay.AllowedCapabilities != nil {
		spec.AllowedCapabilities = mergeCapabilitySelectors(spec.AllowedCapabilities, overlay.AllowedCapabilities)
	}
	if overlay.ToolExecutionPolicy != nil {
		if spec.ToolExecutionPolicy == nil {
			spec.ToolExecutionPolicy = make(map[string]ToolPolicy, len(overlay.ToolExecutionPolicy))
		}
		for name, policy := range overlay.ToolExecutionPolicy {
			spec.ToolExecutionPolicy[name] = policy
		}
	}
	if overlay.CapabilityPolicies != nil {
		spec.CapabilityPolicies = append(spec.CapabilityPolicies, cloneCapabilityPolicies(overlay.CapabilityPolicies)...)
	}
	if overlay.ExposurePolicies != nil {
		spec.ExposurePolicies = append(spec.ExposurePolicies, cloneExposurePolicies(overlay.ExposurePolicies)...)
	}
	if overlay.InsertionPolicies != nil {
		spec.InsertionPolicies = append(spec.InsertionPolicies, cloneInsertionPolicies(overlay.InsertionPolicies)...)
	}
	if overlay.SessionPolicies != nil {
		spec.SessionPolicies = append(spec.SessionPolicies, cloneSessionPolicies(overlay.SessionPolicies)...)
	}
	if overlay.GlobalPolicies != nil {
		if spec.GlobalPolicies == nil {
			spec.GlobalPolicies = make(map[string]AgentPermissionLevel, len(overlay.GlobalPolicies))
		}
		for key, level := range overlay.GlobalPolicies {
			spec.GlobalPolicies[key] = level
		}
	}
	if overlay.ProviderPolicies != nil {
		if spec.ProviderPolicies == nil {
			spec.ProviderPolicies = make(map[string]ProviderPolicy, len(overlay.ProviderPolicies))
		}
		for key, policy := range overlay.ProviderPolicies {
			spec.ProviderPolicies[key] = policy
		}
	}
	if overlay.Providers != nil {
		spec.Providers = mergeProviderConfigs(spec.Providers, overlay.Providers)
	}
	if overlay.RuntimeSafety != nil {
		spec.RuntimeSafety = cloneRuntimeSafetySpec(overlay.RuntimeSafety)
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
	if overlay.Coordination != nil {
		spec.Coordination = mergeAgentCoordinationSpec(spec.Coordination, *overlay.Coordination)
	}
	if overlay.Composition != nil {
		spec.Composition = cloneAgentCompositionSpec(overlay.Composition)
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
	clone.AllowedCapabilities = cloneCapabilitySelectors(spec.AllowedCapabilities)
	clone.ToolExecutionPolicy = cloneToolPolicies(spec.ToolExecutionPolicy)
	clone.CapabilityPolicies = cloneCapabilityPolicies(spec.CapabilityPolicies)
	clone.ExposurePolicies = cloneExposurePolicies(spec.ExposurePolicies)
	clone.InsertionPolicies = cloneInsertionPolicies(spec.InsertionPolicies)
	clone.SessionPolicies = cloneSessionPolicies(spec.SessionPolicies)
	clone.GlobalPolicies = cloneGlobalPolicies(spec.GlobalPolicies)
	clone.ProviderPolicies = cloneProviderPolicies(spec.ProviderPolicies)
	clone.Providers = cloneProviderConfigs(spec.Providers)
	clone.RuntimeSafety = cloneRuntimeSafetySpec(spec.RuntimeSafety)
	clone.SkillConfig = cloneAgentSkillConfig(spec.SkillConfig)
	clone.Bash.AllowPatterns = append([]string{}, spec.Bash.AllowPatterns...)
	clone.Bash.DenyPatterns = append([]string{}, spec.Bash.DenyPatterns...)
	clone.Files.Write.AllowPatterns = append([]string{}, spec.Files.Write.AllowPatterns...)
	clone.Files.Write.DenyPatterns = append([]string{}, spec.Files.Write.DenyPatterns...)
	clone.Files.Edit.AllowPatterns = append([]string{}, spec.Files.Edit.AllowPatterns...)
	clone.Files.Edit.DenyPatterns = append([]string{}, spec.Files.Edit.DenyPatterns...)
	clone.Invocation.AllowedSubagents = append([]string{}, spec.Invocation.AllowedSubagents...)
	clone.Coordination = cloneAgentCoordinationSpec(spec.Coordination)
	clone.Composition = cloneAgentCompositionSpec(spec.Composition)
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

func cloneAgentCompositionSpec(spec *AgentCompositionSpec) *AgentCompositionSpec {
	if spec == nil {
		return nil
	}
	clone := *spec
	if spec.Policy != nil {
		policy := *spec.Policy
		clone.Policy = &policy
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

func cloneProviderConfigs(values []ProviderConfig) []ProviderConfig {
	if len(values) == 0 {
		return nil
	}
	out := make([]ProviderConfig, len(values))
	copy(out, values)
	for idx := range out {
		if len(values[idx].Config) == 0 {
			continue
		}
		out[idx].Config = make(map[string]any, len(values[idx].Config))
		for key, value := range values[idx].Config {
			out[idx].Config[key] = value
		}
	}
	return out
}

func mergeProviderConfigs(base, extra []ProviderConfig) []ProviderConfig {
	if len(extra) == 0 {
		return cloneProviderConfigs(base)
	}
	merged := cloneProviderConfigs(base)
	index := make(map[string]int, len(merged))
	for idx, provider := range merged {
		index[provider.ID] = idx
	}
	for _, provider := range extra {
		if idx, ok := index[provider.ID]; ok {
			merged[idx] = provider
			continue
		}
		index[provider.ID] = len(merged)
		merged = append(merged, provider)
	}
	return merged
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

func cloneInsertionPolicies(policies []CapabilityInsertionPolicy) []CapabilityInsertionPolicy {
	if policies == nil {
		return nil
	}
	out := make([]CapabilityInsertionPolicy, len(policies))
	for i, policy := range policies {
		out[i] = policy
		out[i].Selector = cloneCapabilitySelector(policy.Selector)
	}
	return out
}

func cloneCapabilityPolicies(policies []CapabilityPolicy) []CapabilityPolicy {
	if policies == nil {
		return nil
	}
	clone := make([]CapabilityPolicy, len(policies))
	for i, policy := range policies {
		clone[i] = policy
		clone[i].Selector = cloneCapabilitySelector(policy.Selector)
	}
	return clone
}

func cloneCapabilitySelectors(selectors []CapabilitySelector) []CapabilitySelector {
	if selectors == nil {
		return nil
	}
	out := make([]CapabilitySelector, len(selectors))
	for i, selector := range selectors {
		out[i] = cloneCapabilitySelector(selector)
	}
	return out
}

func cloneCapabilitySelector(selector CapabilitySelector) CapabilitySelector {
	if selector.Tags != nil {
		selector.Tags = append([]string{}, selector.Tags...)
	}
	if selector.ExcludeTags != nil {
		selector.ExcludeTags = append([]string{}, selector.ExcludeTags...)
	}
	if selector.SourceScopes != nil {
		selector.SourceScopes = append([]CapabilityScope{}, selector.SourceScopes...)
	}
	if selector.TrustClasses != nil {
		selector.TrustClasses = append([]TrustClass{}, selector.TrustClasses...)
	}
	if selector.RiskClasses != nil {
		selector.RiskClasses = append([]RiskClass{}, selector.RiskClasses...)
	}
	if selector.EffectClasses != nil {
		selector.EffectClasses = append([]EffectClass{}, selector.EffectClasses...)
	}
	if selector.CoordinationRoles != nil {
		selector.CoordinationRoles = append([]CoordinationRole{}, selector.CoordinationRoles...)
	}
	if selector.CoordinationTaskTypes != nil {
		selector.CoordinationTaskTypes = append([]string{}, selector.CoordinationTaskTypes...)
	}
	if selector.CoordinationExecutionModes != nil {
		selector.CoordinationExecutionModes = append([]CoordinationExecutionMode{}, selector.CoordinationExecutionModes...)
	}
	if selector.CoordinationLongRunning != nil {
		value := *selector.CoordinationLongRunning
		selector.CoordinationLongRunning = &value
	}
	if selector.CoordinationDirectInsertion != nil {
		value := *selector.CoordinationDirectInsertion
		selector.CoordinationDirectInsertion = &value
	}
	return selector
}

func cloneAgentCoordinationSpec(spec AgentCoordinationSpec) AgentCoordinationSpec {
	clone := spec
	clone.DelegationTargetSelectors = cloneCapabilitySelectors(spec.DelegationTargetSelectors)
	clone.ResourceHandoffSelectors = cloneCapabilitySelectors(spec.ResourceHandoffSelectors)
	clone.Projection = cloneAgentProjectionPolicy(spec.Projection)
	clone.ScaleOut = cloneAgentScaleOutPolicy(spec.ScaleOut)
	return clone
}

func mergeAgentCoordinationSpec(base, overlay AgentCoordinationSpec) AgentCoordinationSpec {
	merged := cloneAgentCoordinationSpec(base)
	if overlay.Enabled {
		merged.Enabled = true
	}
	if overlay.MaxDelegationDepth != 0 {
		merged.MaxDelegationDepth = overlay.MaxDelegationDepth
	}
	if overlay.AllowRemoteDelegation {
		merged.AllowRemoteDelegation = true
	}
	if overlay.AllowBackgroundDelegation {
		merged.AllowBackgroundDelegation = true
	}
	if overlay.RequireApprovalCrossTrust {
		merged.RequireApprovalCrossTrust = true
	}
	merged.DelegationTargetSelectors = mergeCapabilitySelectors(merged.DelegationTargetSelectors, overlay.DelegationTargetSelectors)
	merged.ResourceHandoffSelectors = mergeCapabilitySelectors(merged.ResourceHandoffSelectors, overlay.ResourceHandoffSelectors)
	merged.Projection = mergeAgentProjectionPolicy(merged.Projection, overlay.Projection)
	merged.ScaleOut = mergeAgentScaleOutPolicy(merged.ScaleOut, overlay.ScaleOut)
	return merged
}

func cloneAgentProjectionPolicy(input AgentProjectionPolicy) AgentProjectionPolicy {
	out := input
	out.Hot = cloneAgentProjectionTier(input.Hot)
	out.Warm = cloneAgentProjectionTier(input.Warm)
	out.Cold = cloneAgentProjectionTier(input.Cold)
	return out
}

func mergeAgentProjectionPolicy(base, overlay AgentProjectionPolicy) AgentProjectionPolicy {
	merged := cloneAgentProjectionPolicy(base)
	if overlay.Strategy != "" {
		merged.Strategy = overlay.Strategy
	}
	merged.Hot = mergeAgentProjectionTier(merged.Hot, overlay.Hot)
	merged.Warm = mergeAgentProjectionTier(merged.Warm, overlay.Warm)
	merged.Cold = mergeAgentProjectionTier(merged.Cold, overlay.Cold)
	return merged
}

func cloneAgentProjectionTier(input AgentProjectionTier) AgentProjectionTier {
	out := input
	out.ResourceScopes = append([]string{}, input.ResourceScopes...)
	return out
}

func mergeAgentProjectionTier(base, overlay AgentProjectionTier) AgentProjectionTier {
	merged := cloneAgentProjectionTier(base)
	if overlay.MaxItems != 0 {
		merged.MaxItems = overlay.MaxItems
	}
	if overlay.MaxTokens != 0 {
		merged.MaxTokens = overlay.MaxTokens
	}
	if overlay.MaxBytes != 0 {
		merged.MaxBytes = overlay.MaxBytes
	}
	if overlay.Persist {
		merged.Persist = true
	}
	if overlay.ResourceScopes != nil {
		merged.ResourceScopes = mergeStringList(merged.ResourceScopes, overlay.ResourceScopes)
	}
	return merged
}

func cloneAgentScaleOutPolicy(input AgentScaleOutPolicy) AgentScaleOutPolicy {
	out := input
	out.PreferredProviders = append([]string{}, input.PreferredProviders...)
	if input.Metadata != nil {
		out.Metadata = make(map[string]string, len(input.Metadata))
		for key, value := range input.Metadata {
			out.Metadata[key] = value
		}
	}
	return out
}

func mergeAgentScaleOutPolicy(base, overlay AgentScaleOutPolicy) AgentScaleOutPolicy {
	merged := cloneAgentScaleOutPolicy(base)
	if overlay.Mode != "" {
		merged.Mode = overlay.Mode
	}
	if overlay.PreferredModelClass != "" {
		merged.PreferredModelClass = overlay.PreferredModelClass
	}
	if overlay.PreferredProviders != nil {
		merged.PreferredProviders = mergeStringList(merged.PreferredProviders, overlay.PreferredProviders)
	}
	if overlay.Metadata != nil {
		if merged.Metadata == nil {
			merged.Metadata = map[string]string{}
		}
		for key, value := range overlay.Metadata {
			merged.Metadata[key] = value
		}
	}
	return merged
}

func cloneGlobalPolicies(policies map[string]AgentPermissionLevel) map[string]AgentPermissionLevel {
	if policies == nil {
		return nil
	}
	clone := make(map[string]AgentPermissionLevel, len(policies))
	for key, value := range policies {
		clone[key] = value
	}
	return clone
}

func cloneProviderPolicies(policies map[string]ProviderPolicy) map[string]ProviderPolicy {
	if policies == nil {
		return nil
	}
	clone := make(map[string]ProviderPolicy, len(policies))
	for key, value := range policies {
		clone[key] = value
	}
	return clone
}

func cloneExposurePolicies(policies []CapabilityExposurePolicy) []CapabilityExposurePolicy {
	if len(policies) == 0 {
		return nil
	}
	out := make([]CapabilityExposurePolicy, len(policies))
	for i, policy := range policies {
		out[i] = policy
		out[i].Selector = cloneCapabilitySelector(policy.Selector)
	}
	return out
}

func cloneSessionPolicies(policies []SessionPolicy) []SessionPolicy {
	if len(policies) == 0 {
		return nil
	}
	out := make([]SessionPolicy, len(policies))
	for i, policy := range policies {
		out[i] = policy
		out[i].Selector.Partitions = append([]string{}, policy.Selector.Partitions...)
		out[i].Selector.ChannelIDs = append([]string{}, policy.Selector.ChannelIDs...)
		out[i].Selector.Scopes = append([]SessionScope{}, policy.Selector.Scopes...)
		out[i].Selector.TrustClasses = append([]TrustClass{}, policy.Selector.TrustClasses...)
		out[i].Selector.Operations = append([]SessionOperation{}, policy.Selector.Operations...)
		out[i].Selector.ActorKinds = append([]string{}, policy.Selector.ActorKinds...)
		out[i].Selector.ActorIDs = append([]string{}, policy.Selector.ActorIDs...)
		out[i].Approvers = append([]string{}, policy.Approvers...)
	}
	return out
}

func cloneRuntimeSafetySpec(spec *RuntimeSafetySpec) *RuntimeSafetySpec {
	if spec == nil {
		return nil
	}
	clone := *spec
	return &clone
}

func cloneAgentSkillConfig(input AgentSkillConfig) AgentSkillConfig {
	out := AgentSkillConfig{
		Verification: input.Verification,
		Recovery:     input.Recovery,
		Planning:     input.Planning,
		Review:       input.Review,
		ContextHints: input.ContextHints,
	}
	if input.PhaseCapabilities != nil {
		out.PhaseCapabilities = make(map[string][]string, len(input.PhaseCapabilities))
		for phase, tools := range input.PhaseCapabilities {
			out.PhaseCapabilities[phase] = append([]string{}, tools...)
		}
	}
	if input.PhaseCapabilitySelectors != nil {
		out.PhaseCapabilitySelectors = make(map[string][]SkillCapabilitySelector, len(input.PhaseCapabilitySelectors))
		for phase, selectors := range input.PhaseCapabilitySelectors {
			out.PhaseCapabilitySelectors[phase] = cloneSkillCapabilitySelectors(selectors)
		}
	}
	out.Verification.SuccessTools = append([]string{}, input.Verification.SuccessTools...)
	out.Verification.SuccessCapabilitySelectors = cloneSkillCapabilitySelectors(input.Verification.SuccessCapabilitySelectors)
	out.Recovery.FailureProbeTools = append([]string{}, input.Recovery.FailureProbeTools...)
	out.Recovery.FailureProbeCapabilitySelectors = cloneSkillCapabilitySelectors(input.Recovery.FailureProbeCapabilitySelectors)
	out.Planning.RequiredBeforeEdit = cloneSkillCapabilitySelectors(input.Planning.RequiredBeforeEdit)
	out.Planning.PreferredEditCapabilities = cloneSkillCapabilitySelectors(input.Planning.PreferredEditCapabilities)
	out.Planning.PreferredVerifyCapabilities = cloneSkillCapabilitySelectors(input.Planning.PreferredVerifyCapabilities)
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
	if overlay.PhaseCapabilities != nil {
		if merged.PhaseCapabilities == nil {
			merged.PhaseCapabilities = make(map[string][]string, len(overlay.PhaseCapabilities))
		}
		for phase, tools := range overlay.PhaseCapabilities {
			merged.PhaseCapabilities[phase] = mergeStringList(merged.PhaseCapabilities[phase], tools)
		}
	}
	if overlay.PhaseCapabilitySelectors != nil {
		if merged.PhaseCapabilitySelectors == nil {
			merged.PhaseCapabilitySelectors = make(map[string][]SkillCapabilitySelector, len(overlay.PhaseCapabilitySelectors))
		}
		for phase, selectors := range overlay.PhaseCapabilitySelectors {
			merged.PhaseCapabilitySelectors[phase] = mergeSkillCapabilitySelectors(merged.PhaseCapabilitySelectors[phase], selectors)
		}
	}
	merged.Verification.SuccessTools = mergeStringList(merged.Verification.SuccessTools, overlay.Verification.SuccessTools)
	merged.Verification.SuccessCapabilitySelectors = mergeSkillCapabilitySelectors(merged.Verification.SuccessCapabilitySelectors, overlay.Verification.SuccessCapabilitySelectors)
	merged.Verification.StopOnSuccess = merged.Verification.StopOnSuccess || overlay.Verification.StopOnSuccess
	merged.Recovery.FailureProbeTools = mergeStringList(merged.Recovery.FailureProbeTools, overlay.Recovery.FailureProbeTools)
	merged.Recovery.FailureProbeCapabilitySelectors = mergeSkillCapabilitySelectors(merged.Recovery.FailureProbeCapabilitySelectors, overlay.Recovery.FailureProbeCapabilitySelectors)
	merged.Planning.RequiredBeforeEdit = mergeSkillCapabilitySelectors(merged.Planning.RequiredBeforeEdit, overlay.Planning.RequiredBeforeEdit)
	merged.Planning.PreferredEditCapabilities = mergeSkillCapabilitySelectors(merged.Planning.PreferredEditCapabilities, overlay.Planning.PreferredEditCapabilities)
	merged.Planning.PreferredVerifyCapabilities = mergeSkillCapabilitySelectors(merged.Planning.PreferredVerifyCapabilities, overlay.Planning.PreferredVerifyCapabilities)
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

func mergeSkillCapabilitySelectors(base, extra []SkillCapabilitySelector) []SkillCapabilitySelector {
	if len(extra) == 0 {
		return cloneSkillCapabilitySelectors(base)
	}
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]SkillCapabilitySelector, 0, len(base)+len(extra))
	for _, selector := range append(append([]SkillCapabilitySelector{}, base...), extra...) {
		key := selector.Capability + "|" + joinRuntimeFamilies(selector.RuntimeFamilies) + "|" + strings.Join(selector.Tags, ",") + "|" + strings.Join(selector.ExcludeTags, ",")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, cloneSkillCapabilitySelector(selector))
	}
	return out
}

func cloneSkillCapabilitySelectors(input []SkillCapabilitySelector) []SkillCapabilitySelector {
	if input == nil {
		return nil
	}
	out := make([]SkillCapabilitySelector, len(input))
	for i, selector := range input {
		out[i] = cloneSkillCapabilitySelector(selector)
	}
	return out
}

func cloneSkillCapabilitySelector(input SkillCapabilitySelector) SkillCapabilitySelector {
	input.RuntimeFamilies = append([]CapabilityRuntimeFamily{}, input.RuntimeFamilies...)
	input.Tags = append([]string{}, input.Tags...)
	input.ExcludeTags = append([]string{}, input.ExcludeTags...)
	return input
}

func mergeCapabilitySelectors(base, extra []CapabilitySelector) []CapabilitySelector {
	if len(extra) == 0 {
		return append([]CapabilitySelector{}, base...)
	}
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]CapabilitySelector, 0, len(base)+len(extra))
	for _, selector := range append(append([]CapabilitySelector{}, base...), extra...) {
		key := selector.ID + "|" + selector.Name + "|" + string(selector.Kind) + "|" +
			strings.Join(selector.Tags, ",") + "|" + strings.Join(selector.ExcludeTags, ",") + "|" +
			joinCapabilityScopes(selector.SourceScopes) + "|" + joinTrustClasses(selector.TrustClasses) + "|" +
			joinRiskClasses(selector.RiskClasses) + "|" + joinEffectClasses(selector.EffectClasses) + "|" +
			joinCoordinationRoles(selector.CoordinationRoles) + "|" + strings.Join(selector.CoordinationTaskTypes, ",") + "|" +
			joinCoordinationExecutionModes(selector.CoordinationExecutionModes) + "|" + boolPointerKey(selector.CoordinationLongRunning) + "|" +
			boolPointerKey(selector.CoordinationDirectInsertion)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, cloneCapabilitySelector(selector))
	}
	return out
}

func joinCapabilityScopes(values []CapabilityScope) string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return strings.Join(out, ",")
}

func joinCoordinationRoles(values []CoordinationRole) string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return strings.Join(out, ",")
}

func joinCoordinationExecutionModes(values []CoordinationExecutionMode) string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return strings.Join(out, ",")
}

func boolPointerKey(value *bool) string {
	if value == nil {
		return ""
	}
	if *value {
		return "true"
	}
	return "false"
}

func joinRuntimeFamilies(values []CapabilityRuntimeFamily) string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return strings.Join(out, ",")
}

func joinTrustClasses(values []TrustClass) string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return strings.Join(out, ",")
}

func joinRiskClasses(values []RiskClass) string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return strings.Join(out, ",")
}

func joinEffectClasses(values []EffectClass) string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return strings.Join(out, ",")
}
