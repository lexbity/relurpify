package model

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Profile describes a typed model profile definition.
type Profile struct {
	Schema      string `yaml:"schema" json:"schema"`
	Pattern     string `yaml:"pattern" json:"pattern"`
	ToolCalling struct {
		Intent             string `yaml:"intent" json:"intent"`
		MaxConcurrentTools int    `yaml:"max_concurrent_tools,omitempty" json:"max_concurrent_tools,omitempty"`
		DoubleEncodeArgs   bool   `yaml:"double_encode_args,omitempty" json:"double_encode_args,omitempty"`
	} `yaml:"tool_calling" json:"tool_calling"`
	Context struct {
		MaxTokens             int `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
		ResponseReserveTokens int `yaml:"response_reserve_tokens,omitempty" json:"response_reserve_tokens,omitempty"`
	} `yaml:"context" json:"context"`
	Generation struct {
		Temperature float64 `yaml:"temperature,omitempty" json:"temperature,omitempty"`
		TopP        float64 `yaml:"top_p,omitempty" json:"top_p,omitempty"`
	} `yaml:"generation" json:"generation"`
	SourcePath string `yaml:"-" json:"-"`
}

// LoadProfileFile loads and validates a single profile file.
func LoadProfileFile(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var profile Profile
	if DecodeWithSchema != nil {
		if _, err := DecodeWithSchema(path, data, &profile); err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("DecodeWithSchema not initialized")
	}
	if err := profile.Validate(); err != nil {
		return nil, err
	}
	profile.SourcePath = path
	return &profile, nil
}

// LoadProfileDir loads every profile file in a directory in deterministic order.
func LoadProfileDir(dir string) ([]*Profile, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("profile dir required")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".llm.yaml") {
			continue
		}
		paths = append(paths, filepath.Join(dir, name))
	}
	sort.Strings(paths)
	out := make([]*Profile, 0, len(paths))
	for _, path := range paths {
		profile, err := LoadProfileFile(path)
		if err != nil {
			return nil, err
		}
		out = append(out, profile)
	}
	return out, nil
}

// Validate enforces the profile schema contract.
func (p Profile) Validate() error {
	if strings.TrimSpace(p.Pattern) == "" {
		return fmt.Errorf("profile pattern required")
	}
	if _, err := filepath.Match(p.Pattern, "model"); err != nil {
		return fmt.Errorf("profile pattern invalid: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(p.ToolCalling.Intent)) {
	case "native", "prompt_based", "auto":
	default:
		return fmt.Errorf("profile %q tool_calling.intent invalid", p.Pattern)
	}
	if p.ToolCalling.MaxConcurrentTools < 0 || p.ToolCalling.MaxConcurrentTools > 32 {
		return fmt.Errorf("profile %q max_concurrent_tools must be in [0,32]", p.Pattern)
	}
	if p.Context.MaxTokens <= 0 {
		return fmt.Errorf("profile %q context.max_tokens must be positive", p.Pattern)
	}
	if p.Generation.Temperature < 0 || p.Generation.Temperature > 2 || math.IsNaN(p.Generation.Temperature) {
		return fmt.Errorf("profile %q generation.temperature must be in [0,2]", p.Pattern)
	}
	if p.Generation.TopP < 0 || p.Generation.TopP > 1 || math.IsNaN(p.Generation.TopP) {
		return fmt.Errorf("profile %q generation.top_p must be in [0,1]", p.Pattern)
	}
	return nil
}
