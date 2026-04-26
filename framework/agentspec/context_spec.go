package agentspec

type AgentContextSpec struct {
	MaxFiles            int    `yaml:"max_files,omitempty" json:"max_files,omitempty"`
	MaxTokens           int    `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
	IncludeGitHistory   bool   `yaml:"include_git_history,omitempty" json:"include_git_history,omitempty"`
	IncludeDependencies bool   `yaml:"include_dependencies,omitempty" json:"include_dependencies,omitempty"`
	CompressionStrategy string `yaml:"compression_strategy,omitempty" json:"compression_strategy,omitempty"`
	ProgressiveLoading  *bool  `yaml:"progressive_loading,omitempty" json:"progressive_loading,omitempty"`
}

type AgentContextSpecOverlay struct {
	MaxFiles            *int    `yaml:"max_files,omitempty" json:"max_files,omitempty"`
	MaxTokens           *int    `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
	IncludeGitHistory   *bool   `yaml:"include_git_history,omitempty" json:"include_git_history,omitempty"`
	IncludeDependencies *bool   `yaml:"include_dependencies,omitempty" json:"include_dependencies,omitempty"`
	CompressionStrategy *string `yaml:"compression_strategy,omitempty" json:"compression_strategy,omitempty"`
	ProgressiveLoading  *bool   `yaml:"progressive_loading,omitempty" json:"progressive_loading,omitempty"`
}

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
			value := *overlay.ProgressiveLoading
			merged.ProgressiveLoading = &value
		}
	}
	return merged
}
