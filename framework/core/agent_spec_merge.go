package core

// AgentModelConfigOverlay defines optional overrides for model configuration.
type AgentModelConfigOverlay struct {
	Provider    *string  `yaml:"provider,omitempty" json:"provider,omitempty"`
	Name        *string  `yaml:"name,omitempty" json:"name,omitempty"`
	Temperature *float64 `yaml:"temperature,omitempty" json:"temperature,omitempty"`
	MaxTokens   *int     `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
}

// MergeAgentModelConfig applies overlays to the base model config.
func MergeAgentModelConfig(base AgentModelConfig, overlays ...AgentModelConfigOverlay) AgentModelConfig {
	merged := base
	for _, overlay := range overlays {
		if overlay.Provider != nil {
			merged.Provider = *overlay.Provider
		}
		if overlay.Name != nil {
			merged.Name = *overlay.Name
		}
		if overlay.Temperature != nil {
			merged.Temperature = *overlay.Temperature
		}
		if overlay.MaxTokens != nil {
			merged.MaxTokens = *overlay.MaxTokens
		}
	}
	return merged
}

// AgentContextSpecOverlay defines optional overrides for context settings.
type AgentContextSpecOverlay struct {
	MaxFiles            *int    `yaml:"max_files,omitempty" json:"max_files,omitempty"`
	MaxTokens           *int    `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
	IncludeGitHistory   *bool   `yaml:"include_git_history,omitempty" json:"include_git_history,omitempty"`
	IncludeDependencies *bool   `yaml:"include_dependencies,omitempty" json:"include_dependencies,omitempty"`
	CompressionStrategy *string `yaml:"compression_strategy,omitempty" json:"compression_strategy,omitempty"`
	ProgressiveLoading  *bool   `yaml:"progressive_loading,omitempty" json:"progressive_loading,omitempty"`
}

// MergeAgentContextSpec applies overlays to the base context spec.
func MergeAgentContextSpec(base AgentContextSpec, overlays ...AgentContextSpecOverlay) AgentContextSpec {
	merged := base
	for _, overlay := range overlays {
		if overlay.MaxFiles != nil {
			merged.MaxFiles = *overlay.MaxFiles
		}
		if overlay.MaxTokens != nil {
			merged.MaxTokens = *overlay.MaxTokens
		}
		if overlay.IncludeGitHistory != nil {
			merged.IncludeGitHistory = *overlay.IncludeGitHistory
		}
		if overlay.IncludeDependencies != nil {
			merged.IncludeDependencies = *overlay.IncludeDependencies
		}
		if overlay.CompressionStrategy != nil {
			merged.CompressionStrategy = *overlay.CompressionStrategy
		}
		if overlay.ProgressiveLoading != nil {
			merged.ProgressiveLoading = *overlay.ProgressiveLoading
		}
	}
	return merged
}

// AgentSearchSpecOverlay defines optional overrides for search settings.
type AgentSearchSpecOverlay struct {
	HybridEnabled *bool `yaml:"hybrid_enabled,omitempty" json:"hybrid_enabled,omitempty"`
	VectorIndex   *bool `yaml:"vector_index,omitempty" json:"vector_index,omitempty"`
	ASTIndex      *bool `yaml:"ast_index,omitempty" json:"ast_index,omitempty"`
}

// MergeAgentSearchSpec applies overlays to the base search spec.
func MergeAgentSearchSpec(base AgentSearchSpec, overlays ...AgentSearchSpecOverlay) AgentSearchSpec {
	merged := base
	for _, overlay := range overlays {
		if overlay.HybridEnabled != nil {
			merged.HybridEnabled = *overlay.HybridEnabled
		}
		if overlay.VectorIndex != nil {
			merged.VectorIndex = *overlay.VectorIndex
		}
		if overlay.ASTIndex != nil {
			merged.ASTIndex = *overlay.ASTIndex
		}
	}
	return merged
}

// AgentLSPSpecOverlay defines optional overrides for LSP settings.
type AgentLSPSpecOverlay struct {
	Servers map[string]string `yaml:"servers,omitempty" json:"servers,omitempty"`
	Enabled *bool             `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Timeout *string           `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

// MergeAgentLSPSpec applies overlays to the base LSP spec.
func MergeAgentLSPSpec(base AgentLSPSpec, overlays ...AgentLSPSpecOverlay) AgentLSPSpec {
	merged := base
	for _, overlay := range overlays {
		if overlay.Enabled != nil {
			merged.Enabled = *overlay.Enabled
		}
		if overlay.Timeout != nil {
			merged.Timeout = *overlay.Timeout
		}
		if overlay.Servers != nil {
			if merged.Servers == nil {
				merged.Servers = make(map[string]string, len(overlay.Servers))
			}
			for key, value := range overlay.Servers {
				merged.Servers[key] = value
			}
		}
	}
	return merged
}
