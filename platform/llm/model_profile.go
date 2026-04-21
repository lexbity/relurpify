package llm

import (
	"strings"

	ollamapkg "codeburg.org/lexbit/relurpify/platform/llm/ollama"
	openaicompatpkg "codeburg.org/lexbit/relurpify/platform/llm/openaicompat"
)

// ModelProfile captures model-specific quirks and configuration.
type ModelProfile struct {
	Provider string `yaml:"provider,omitempty"`
	Model    string `yaml:"model,omitempty"`
	Pattern  string `yaml:"pattern,omitempty"`

	ToolCalling struct {
		NativeAPI               bool `yaml:"native_api"`
		DoubleEncodedArgs       bool `yaml:"double_encoded_args"`
		MultilineStringLiterals bool `yaml:"multiline_string_literals"`
		MaxToolsPerCall         int  `yaml:"max_tools_per_call"`
	} `yaml:"tool_calling"`

	Repair struct {
		Strategy    string `yaml:"strategy"`
		MaxAttempts int    `yaml:"max_attempts"`
	} `yaml:"repair"`

	Schema struct {
		FlattenNested     bool `yaml:"flatten_nested"`
		MaxDescriptionLen int  `yaml:"max_description_len"`
	} `yaml:"schema"`

	SourcePath string `yaml:"-"`
}

// Normalize applies compatibility defaults and canonicalizes string fields.
func (p *ModelProfile) Normalize() {
	if p == nil {
		return
	}
	p.Provider = strings.ToLower(strings.TrimSpace(p.Provider))
	p.Model = strings.TrimSpace(p.Model)
	p.Pattern = strings.TrimSpace(p.Pattern)
	if p.Repair.Strategy == "" {
		p.Repair.Strategy = "heuristic-only"
	}
	if p.Repair.MaxAttempts < 0 {
		p.Repair.MaxAttempts = 0
	}
}

// Clone returns a deep copy of the profile.
func (p *ModelProfile) Clone() *ModelProfile {
	if p == nil {
		return nil
	}
	clone := *p
	return &clone
}

// IsExactModelMatch reports whether the profile pins a specific model name.
func (p *ModelProfile) IsExactModelMatch() bool {
	if p == nil {
		return false
	}
	if p.Model != "" {
		return !hasGlobMeta(p.Model)
	}
	return p.Pattern != "" && !hasGlobMeta(p.Pattern)
}

// MatchPattern returns the effective model selector used by registry matching.
func (p *ModelProfile) MatchPattern() string {
	if p == nil {
		return ""
	}
	if p.Model != "" {
		return p.Model
	}
	return p.Pattern
}

// AsOllamaProfile converts the shared schema into the Ollama transport's local
// profile type.
func (p *ModelProfile) AsOllamaProfile() *ollamapkg.ModelProfile {
	if p == nil {
		return nil
	}
	return &ollamapkg.ModelProfile{
		Provider: p.Provider,
		Model:    p.Model,
		Pattern:  p.Pattern,
		ToolCalling: struct {
			NativeAPI               bool `yaml:"native_api"`
			DoubleEncodedArgs       bool `yaml:"double_encoded_args"`
			MultilineStringLiterals bool `yaml:"multiline_string_literals"`
			MaxToolsPerCall         int  `yaml:"max_tools_per_call"`
		}{
			NativeAPI:               p.ToolCalling.NativeAPI,
			DoubleEncodedArgs:       p.ToolCalling.DoubleEncodedArgs,
			MultilineStringLiterals: p.ToolCalling.MultilineStringLiterals,
			MaxToolsPerCall:         p.ToolCalling.MaxToolsPerCall,
		},
		Repair: struct {
			Strategy    string `yaml:"strategy"`
			MaxAttempts int    `yaml:"max_attempts"`
		}{
			Strategy:    p.Repair.Strategy,
			MaxAttempts: p.Repair.MaxAttempts,
		},
		Schema: struct {
			FlattenNested     bool `yaml:"flatten_nested"`
			MaxDescriptionLen int  `yaml:"max_description_len"`
		}{
			FlattenNested:     p.Schema.FlattenNested,
			MaxDescriptionLen: p.Schema.MaxDescriptionLen,
		},
	}
}

// AsOpenAICompatProfile converts the shared schema into the OpenAI-compatible
// transport's local profile type.
func (p *ModelProfile) AsOpenAICompatProfile() *openaicompatpkg.ModelProfile {
	if p == nil {
		return nil
	}
	return &openaicompatpkg.ModelProfile{
		Provider: p.Provider,
		Model:    p.Model,
		Pattern:  p.Pattern,
		ToolCalling: struct {
			NativeAPI               bool `yaml:"native_api"`
			DoubleEncodedArgs       bool `yaml:"double_encoded_args"`
			MultilineStringLiterals bool `yaml:"multiline_string_literals"`
			MaxToolsPerCall         int  `yaml:"max_tools_per_call"`
		}{
			NativeAPI:               p.ToolCalling.NativeAPI,
			DoubleEncodedArgs:       p.ToolCalling.DoubleEncodedArgs,
			MultilineStringLiterals: p.ToolCalling.MultilineStringLiterals,
			MaxToolsPerCall:         p.ToolCalling.MaxToolsPerCall,
		},
		Repair: struct {
			Strategy    string `yaml:"strategy"`
			MaxAttempts int    `yaml:"max_attempts"`
		}{
			Strategy:    p.Repair.Strategy,
			MaxAttempts: p.Repair.MaxAttempts,
		},
		Schema: struct {
			FlattenNested     bool `yaml:"flatten_nested"`
			MaxDescriptionLen int  `yaml:"max_description_len"`
		}{
			FlattenNested:     p.Schema.FlattenNested,
			MaxDescriptionLen: p.Schema.MaxDescriptionLen,
		},
	}
}

func hasGlobMeta(s string) bool {
	return strings.ContainsAny(s, "*?[")
}
