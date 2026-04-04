package llm

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProfileRegistry loads ModelProfile files from a directory and matches them
// by model name. The registry falls back to the built-in default profile
// when no file matches and no default.yaml is present.
type ProfileRegistry struct {
	profiles []*ModelProfile
}

// NewProfileRegistry loads all *.yaml files from configDir.
// Missing directory returns an empty registry using built-in defaults.
func NewProfileRegistry(configDir string) (*ProfileRegistry, error) {
	reg := &ProfileRegistry{}
	if configDir == "" {
		return reg, nil
	}
	entries, err := os.ReadDir(configDir)
	if err != nil {
		if os.IsNotExist(err) {
			return reg, nil
		}
		return nil, fmt.Errorf("read model profiles dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		path := filepath.Join(configDir, name)
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open profile %s: %w", path, err)
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("read profile %s: %w", path, err)
		}
		var profile ModelProfile
		if err := yaml.Unmarshal(data, &profile); err != nil {
			return nil, fmt.Errorf("parse profile %s: %w", path, err)
		}
		reg.profiles = append(reg.profiles, &profile)
	}
	return reg, nil
}

// Match returns the best-matching profile for modelName.
// Matching priority: longest specific prefix > "*" wildcard > built-in default.
func (r *ProfileRegistry) Match(modelName string) *ModelProfile {
	var bestMatch *ModelProfile
	bestScore := -1
	for _, p := range r.profiles {
		score := matchScore(p.Pattern, modelName)
		if score > bestScore {
			bestScore = score
			bestMatch = p
		}
	}
	if bestMatch == nil {
		// Return built-in default
		return &ModelProfile{
			Pattern: "*",
			ToolCalling: struct {
				NativeAPI               bool `yaml:"native_api"`
				DoubleEncodedArgs       bool `yaml:"double_encoded_args"`
				MultilineStringLiterals bool `yaml:"multiline_string_literals"`
				MaxToolsPerCall         int  `yaml:"max_tools_per_call"`
			}{
				NativeAPI:               false,
				DoubleEncodedArgs:       false,
				MultilineStringLiterals: false,
				MaxToolsPerCall:         0,
			},
			Repair: struct {
				Strategy    string `yaml:"strategy"`
				MaxAttempts int    `yaml:"max_attempts"`
			}{
				Strategy:    "heuristic-only",
				MaxAttempts: 0,
			},
			Schema: struct {
				FlattenNested     bool `yaml:"flatten_nested"`
				MaxDescriptionLen int  `yaml:"max_description_len"`
			}{
				FlattenNested:     false,
				MaxDescriptionLen: 0,
			},
		}
	}
	return bestMatch
}

// matchScore returns a score for how well pattern matches modelName.
// Higher score means better match.
func matchScore(pattern, modelName string) int {
	if pattern == "*" {
		return 0
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		if strings.HasPrefix(modelName, prefix) {
			return len(prefix)
		}
	}
	if pattern == modelName {
		return len(pattern) + 1
	}
	return -1
}
