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

// SkillSpec defines prompt snippets, tool overlays, and resources.
type SkillSpec struct {
	PromptSnippets     []string                   `yaml:"prompt_snippets,omitempty" json:"prompt_snippets,omitempty"`
	ToolMatrixOverride *core.ToolMatrixOverride   `yaml:"tool_matrix_override,omitempty" json:"tool_matrix_override,omitempty"`
	ToolPolicies       map[string]core.ToolPolicy `yaml:"tool_policies,omitempty" json:"tool_policies,omitempty"`
	RequiredTools      []string                   `yaml:"required_tools,omitempty" json:"required_tools,omitempty"`
	Resources          SkillResourceSpec          `yaml:"resources,omitempty" json:"resources,omitempty"`
	AgentOverlay       *core.AgentSpecOverlay     `yaml:"agent_overlay,omitempty" json:"agent_overlay,omitempty"`
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
	for _, tool := range m.Spec.RequiredTools {
		if strings.TrimSpace(tool) == "" {
			return fmt.Errorf("required_tools contains empty entry")
		}
	}
	return nil
}
