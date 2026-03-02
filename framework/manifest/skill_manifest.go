package manifest

import (
	"fmt"
	"github.com/lexcodex/relurpify/framework/core"
	"gopkg.in/yaml.v3"
	"os"
	"strings"
)

// SkillManifest defines a reusable skill package.
type SkillManifest struct {
	APIVersion string           `yaml:"apiVersion" json:"apiVersion"`
	Kind       string           `yaml:"kind" json:"kind"`
	Metadata   ManifestMetadata `yaml:"metadata" json:"metadata"`
	Spec       SkillSpec        `yaml:"spec" json:"spec"`
	SourcePath string           `yaml:"-" json:"-"`
}

// SkillSpec defines prompt snippets, tool allowances, execution policies, and resource paths.
type SkillSpec struct {
	Requires            SkillRequiresSpec          `yaml:"requires,omitempty" json:"requires,omitempty"`
	PromptSnippets      []string                   `yaml:"prompt_snippets,omitempty" json:"prompt_snippets,omitempty"`
	AllowedTools        []string                   `yaml:"allowed_tools,omitempty" json:"allowed_tools,omitempty"`
	ToolExecutionPolicy map[string]core.ToolPolicy `yaml:"tool_execution_policy,omitempty" json:"tool_execution_policy,omitempty"`
	ResourcePaths       SkillResourceSpec          `yaml:"resource_paths,omitempty" json:"resource_paths,omitempty"`
}

// SkillRequiresSpec declares binary prerequisites for a skill.
type SkillRequiresSpec struct {
	Bins []string `yaml:"bins,omitempty" json:"bins,omitempty"`
}

// SkillResourceSpec declares resource paths.
type SkillResourceSpec struct {
	Scripts   []string `yaml:"scripts,omitempty" json:"scripts,omitempty"`
	Resources []string `yaml:"resources,omitempty" json:"resources,omitempty"`
	Templates []string `yaml:"templates,omitempty" json:"templates,omitempty"`
}

// LoadSkillManifest parses and validates a skill manifest file.
func LoadSkillManifest(path string) (*SkillManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest SkillManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	manifest.SourcePath = path
	return &manifest, nil
}

// Validate enforces manifest semantics.
func (m *SkillManifest) Validate() error {
	if m.APIVersion == "" {
		return fmt.Errorf("skill manifest missing apiVersion")
	}
	if m.Kind == "" {
		return fmt.Errorf("skill manifest missing kind")
	}
	if m.Metadata.Name == "" {
		return fmt.Errorf("skill manifest missing metadata.name")
	}
	if strings.ToLower(m.Kind) != strings.ToLower("SkillManifest") {
		return fmt.Errorf("skill manifest kind must be SkillManifest")
	}
	for _, bin := range m.Spec.Requires.Bins {
		if strings.TrimSpace(bin) == "" {
			return fmt.Errorf("requires.bins contains empty entry")
		}
		if strings.Contains(bin, "/") {
			return fmt.Errorf("requires.bins entry %q must not contain '/'", bin)
		}
	}
	return nil
}
