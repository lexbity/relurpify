package manifest

import (
	"fmt"
	"os"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/cfgload"
	"codeburg.org/lexbit/relurpify/framework/core"
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
	Requires            SkillRequiresSpec                     `yaml:"requires,omitempty" json:"requires,omitempty"`
	PromptSnippets      []string                              `yaml:"prompt_snippets,omitempty" json:"prompt_snippets,omitempty"`
	AllowedCapabilities  []agentspec.CapabilitySelector        `yaml:"allowed_capabilities,omitempty" json:"allowed_capabilities,omitempty"`
	ToolExecutionPolicy  map[string]agentspec.ToolPolicy       `yaml:"tool_execution_policy,omitempty" json:"tool_execution_policy,omitempty"`
	CapabilityPolicies   []agentspec.CapabilityPolicy          `yaml:"capability_policies,omitempty" json:"capability_policies,omitempty"`
	InsertionPolicies    []agentspec.CapabilityInsertionPolicy `yaml:"insertion_policies,omitempty" json:"insertion_policies,omitempty"`
	SessionPolicies      []agentspec.SessionPolicy             `yaml:"session_policies,omitempty" json:"session_policies,omitempty"`
	GlobalPolicies       map[string]agentspec.AgentPermissionLevel `yaml:"policies,omitempty" json:"policies,omitempty"`
	ProviderPolicies     map[string]agentspec.ProviderPolicy    `yaml:"provider_policies,omitempty" json:"provider_policies,omitempty"`
	Providers            []core.ProviderConfig                 `yaml:"providers,omitempty" json:"providers,omitempty"`
	ResourcePaths        SkillResourceSpec                     `yaml:"resource_paths,omitempty" json:"resource_paths,omitempty"`
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
	if _, err := cfgload.DecodeWithSchema(path, data, cfgload.NewSchemaRegistry(), &manifest); err != nil {
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
	for i, selector := range m.Spec.AllowedCapabilities {
		if err := agentspec.ValidateCapabilitySelector(selector); err != nil {
			return fmt.Errorf("allowed_capabilities[%d] invalid: %w", i, err)
		}
	}
	for key, level := range m.Spec.GlobalPolicies {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("policies contains empty key")
		}
		switch level {
		case agentspec.AgentPermissionAllow, agentspec.AgentPermissionAsk, agentspec.AgentPermissionDeny, "":
		default:
			return fmt.Errorf("policies[%s]=%s invalid", key, level)
		}
	}
	for providerID, policy := range m.Spec.ProviderPolicies {
		if strings.TrimSpace(providerID) == "" {
			return fmt.Errorf("provider_policies contains empty provider ID")
		}
		if err := agentspec.ValidateProviderPolicy(policy); err != nil {
			return fmt.Errorf("provider_policies[%s] invalid: %w", providerID, err)
		}
	}
	for idx, provider := range m.Spec.Providers {
		if err := provider.Validate(); err != nil {
			return fmt.Errorf("providers[%d] invalid: %w", idx, err)
		}
	}
	for i, policy := range m.Spec.CapabilityPolicies {
		if err := agentspec.ValidateCapabilityPolicy(policy); err != nil {
			return fmt.Errorf("capability_policies[%d] invalid: %w", i, err)
		}
	}
	for i, policy := range m.Spec.InsertionPolicies {
		if err := agentspec.ValidateCapabilityInsertionPolicy(policy); err != nil {
			return fmt.Errorf("insertion_policies[%d] invalid: %w", i, err)
		}
	}
	seenSessionPolicyIDs := make(map[string]struct{}, len(m.Spec.SessionPolicies))
	for i, policy := range m.Spec.SessionPolicies {
		if err := agentspec.ValidateSessionPolicy(policy); err != nil {
			return fmt.Errorf("session_policies[%d] invalid: %w", i, err)
		}
		if _, exists := seenSessionPolicyIDs[policy.ID]; exists {
			return fmt.Errorf("session_policies[%d] duplicates id %q", i, policy.ID)
		}
		seenSessionPolicyIDs[policy.ID] = struct{}{}
	}
	return nil
}
