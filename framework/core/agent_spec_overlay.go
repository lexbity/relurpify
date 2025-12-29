package core

import "strings"

// AgentSpecOverlay defines optional overrides for an agent spec.
type AgentSpecOverlay struct {
	Implementation    *string                  `yaml:"implementation,omitempty" json:"implementation,omitempty"`
	Mode              *AgentMode               `yaml:"mode,omitempty" json:"mode,omitempty"`
	Version           *string                  `yaml:"version,omitempty" json:"version,omitempty"`
	Prompt            *string                  `yaml:"prompt,omitempty" json:"prompt,omitempty"`
	ModelOverlay      *AgentModelConfigOverlay `yaml:"model,omitempty" json:"model,omitempty"`
	AllowedTools      []string                 `yaml:"allowed_tools,omitempty" json:"allowed_tools,omitempty"`
	ToolPolicies      map[string]ToolPolicy    `yaml:"tool_policies,omitempty" json:"tool_policies,omitempty"`
	Bash              *AgentBashPermissions    `yaml:"bash_permissions,omitempty" json:"bash_permissions,omitempty"`
	Files             *AgentFileMatrix         `yaml:"file_permissions,omitempty" json:"file_permissions,omitempty"`
	Invocation        *AgentInvocationSpec     `yaml:"invocation,omitempty" json:"invocation,omitempty"`
	ContextOverlay    *AgentContextSpecOverlay `yaml:"context,omitempty" json:"context,omitempty"`
	LSPOverlay        *AgentLSPSpecOverlay     `yaml:"lsp,omitempty" json:"lsp,omitempty"`
	SearchOverlay     *AgentSearchSpecOverlay  `yaml:"search,omitempty" json:"search,omitempty"`
	Metadata          *AgentMetadata           `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	OllamaToolCalling *bool                    `yaml:"ollama_tool_calling,omitempty" json:"ollama_tool_calling,omitempty"`
	Logging           *AgentLoggingSpec        `yaml:"logging,omitempty" json:"logging,omitempty"`
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
	metadata := spec.Metadata
	toolCalling := spec.OllamaToolCalling
	var logging *AgentLoggingSpec
	if spec.Logging != nil {
		llm := spec.Logging.LLM
		agent := spec.Logging.Agent
		logging = &AgentLoggingSpec{LLM: llm, Agent: agent}
	}
	return AgentSpecOverlay{
		Implementation:    &implementation,
		Mode:              &mode,
		Version:           &version,
		Prompt:            &prompt,
		ModelOverlay:      &modelOverlay,
		AllowedTools:      allowedTools,
		ToolPolicies:      cloneToolPolicies(spec.ToolPolicies),
		Bash:              &bash,
		Files:             &files,
		Invocation:        &invocation,
		ContextOverlay:    &contextOverlay,
		LSPOverlay:        &lspOverlay,
		SearchOverlay:     &searchOverlay,
		Metadata:          &metadata,
		OllamaToolCalling: toolCalling,
		Logging:           logging,
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
	if overlay.ToolPolicies != nil {
		if spec.ToolPolicies == nil {
			spec.ToolPolicies = make(map[string]ToolPolicy, len(overlay.ToolPolicies))
		}
		for name, policy := range overlay.ToolPolicies {
			spec.ToolPolicies[name] = policy
		}
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
	if spec.ToolPolicies != nil {
		clone.ToolPolicies = cloneToolPolicies(spec.ToolPolicies)
	}
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
