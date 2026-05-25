package model

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Provider describes one typed model provider definition.
type Provider struct {
	Schema                string   `yaml:"schema" json:"schema"`
	Name                  string   `yaml:"name" json:"name"`
	Endpoint              string   `yaml:"endpoint" json:"endpoint"`
	Kind                  string   `yaml:"kind" json:"kind"`
	RequestTimeoutSeconds int      `yaml:"request_timeout_seconds,omitempty" json:"request_timeout_seconds,omitempty"`
	AvailableModels       []string `yaml:"available_models,omitempty" json:"available_models,omitempty"`
	NativeToolCalling     bool     `yaml:"native_tool_calling,omitempty" json:"native_tool_calling,omitempty"`
	MaxConcurrent         int      `yaml:"max_concurrent,omitempty" json:"max_concurrent,omitempty"`
	SourcePath            string   `yaml:"-" json:"-"`
}

// LoadProviderFile loads and validates a single provider file.
func LoadProviderFile(path string) (*Provider, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var provider Provider
	if DecodeWithSchema != nil {
		if _, err := DecodeWithSchema(path, data, &provider); err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("DecodeWithSchema not initialized")
	}
	if err := provider.Validate(); err != nil {
		return nil, err
	}
	provider.SourcePath = path
	return &provider, nil
}

// LoadProviderDir loads every provider file in a directory in deterministic order.
func LoadProviderDir(dir string) ([]*Provider, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("provider dir required")
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
		if !strings.HasSuffix(name, ".provider.yaml") {
			continue
		}
		paths = append(paths, filepath.Join(dir, name))
	}
	sort.Strings(paths)
	out := make([]*Provider, 0, len(paths))
	for _, path := range paths {
		provider, err := LoadProviderFile(path)
		if err != nil {
			return nil, err
		}
		out = append(out, provider)
	}
	return out, nil
}

// Validate enforces the provider schema contract.
func (p Provider) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("provider name required")
	}
	switch strings.ToLower(strings.TrimSpace(p.Kind)) {
	case "ollama", "openai_compatible", "openai-compatible", "lmstudio":
	default:
		return fmt.Errorf("provider %q kind %q invalid", p.Name, p.Kind)
	}
	endpoint := strings.TrimSpace(p.Endpoint)
	if endpoint == "" {
		return fmt.Errorf("provider %q endpoint required", p.Name)
	}
	if !(strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://")) {
		return fmt.Errorf("provider %q endpoint must be http or https", p.Name)
	}
	if p.RequestTimeoutSeconds < 0 {
		return fmt.Errorf("provider %q request_timeout_seconds must be >= 0", p.Name)
	}
	if p.MaxConcurrent < 0 {
		return fmt.Errorf("provider %q max_concurrent must be >= 0", p.Name)
	}
	for i, model := range p.AvailableModels {
		if strings.TrimSpace(model) == "" {
			return fmt.Errorf("provider %q available_models[%d] required", p.Name, i)
		}
	}
	return nil
}
