// Package manifest parses and serialises agent security contracts expressed as
// YAML manifests (apiVersion: relurpify/v1alpha1, kind: AgentManifest).
// It defines the AgentManifest schema covering metadata, permissions, resources,
// security policy, audit settings, and skill dependencies.
package manifest

import (
	"crypto/sha256"
	"fmt"
	"github.com/lexcodex/relurpify/framework/core"
	"gopkg.in/yaml.v3"
	"os"
	"strings"
	"time"
)

// AgentManifest defines the security contract for an agent.
type AgentManifest struct {
	APIVersion string           `yaml:"apiVersion" json:"apiVersion"`
	Kind       string           `yaml:"kind" json:"kind"`
	Metadata   ManifestMetadata `yaml:"metadata" json:"metadata"`
	Spec       ManifestSpec     `yaml:"spec" json:"spec"`
	SourcePath string           `yaml:"-" json:"-"`
}

// AgentManifestSnapshot captures a validated manifest together with its load
// fingerprint and timestamp.
type AgentManifestSnapshot struct {
	Manifest    *AgentManifest
	Fingerprint [32]byte
	LoadedAt    time.Time
	SourcePath  string
	Warnings    []string
}

// ManifestMetadata describes identity fields.
type ManifestMetadata struct {
	Name        string `yaml:"name" json:"name"`
	Version     string `yaml:"version" json:"version"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// ManifestSpec encodes runtime, permission, resource, and security sections.
type ManifestSpec struct {
	Image       string                               `yaml:"image" json:"image"`
	Runtime     string                               `yaml:"runtime" json:"runtime"`
	Policy      *ManifestPolicySpec                  `yaml:"policy,omitempty" json:"policy,omitempty"`
	Permissions core.PermissionSet                   `yaml:"permissions" json:"permissions"`
	Resources   ResourceSpec                         `yaml:"resources" json:"resources"`
	Security    SecuritySpec                         `yaml:"security" json:"security"`
	Audit       AuditSpec                            `yaml:"audit" json:"audit"`
	Agent       *core.AgentRuntimeSpec               `yaml:"agent,omitempty" json:"agent,omitempty"`
	Skills      []string                             `yaml:"skills,omitempty" json:"skills,omitempty"`
	Policies    map[string]core.AgentPermissionLevel `yaml:"policies,omitempty" json:"policies,omitempty"`
	Defaults    *ManifestDefaults                    `yaml:"defaults,omitempty" json:"defaults,omitempty"`

	CompatibilityWarnings []string `yaml:"-" json:"-"`
}

// ManifestPolicySpec groups policy-adjacent fields under spec.policy.
type ManifestPolicySpec struct {
	Permissions core.PermissionSet                   `yaml:"permissions,omitempty" json:"permissions,omitempty"`
	Resources   ResourceSpec                         `yaml:"resources,omitempty" json:"resources,omitempty"`
	Security    SecuritySpec                         `yaml:"security,omitempty" json:"security,omitempty"`
	Audit       AuditSpec                            `yaml:"audit,omitempty" json:"audit,omitempty"`
	Policies    map[string]core.AgentPermissionLevel `yaml:"policies,omitempty" json:"policies,omitempty"`
	Defaults    *ManifestDefaults                    `yaml:"defaults,omitempty" json:"defaults,omitempty"`
}

// ManifestDefaults defines global defaults applied before skills.
type ManifestDefaults struct {
	Permissions *core.PermissionSet `yaml:"permissions,omitempty" json:"permissions,omitempty"`
	Resources   *ResourceSpec       `yaml:"resources,omitempty" json:"resources,omitempty"`
}

// ResourceSpec declares resource limits.
type ResourceSpec struct {
	Limits ResourceLimit `yaml:"limits" json:"limits"`
}

// ResourceLimit tracks CPU/memory/disk quotas.
type ResourceLimit struct {
	CPU     string `yaml:"cpu" json:"cpu"`
	Memory  string `yaml:"memory" json:"memory"`
	DiskIO  string `yaml:"disk_io" json:"disk_io"`
	Network string `yaml:"network,omitempty" json:"network,omitempty"`
}

// SecuritySpec enumerates container security toggles.
type SecuritySpec struct {
	RunAsUser       int  `yaml:"run_as_user" json:"run_as_user"`
	ReadOnlyRoot    bool `yaml:"read_only_root" json:"read_only_root"`
	NoNewPrivileges bool `yaml:"no_new_privileges" json:"no_new_privileges"`
}

// AuditSpec configures audit verbosity.
type AuditSpec struct {
	Level         string `yaml:"level" json:"level"`
	RetentionDays int    `yaml:"retention_days" json:"retention_days"`
}

// SaveAgentManifest marshals m to YAML and overwrites path.
// This preserves the manifest structure but will not retain hand-written comments.
func SaveAgentManifest(path string, m *AgentManifest) error {
	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// LoadAgentManifestSnapshot parses, validates, and fingerprints a manifest file.
func LoadAgentManifestSnapshot(path string) (*AgentManifestSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var loaded AgentManifest
	if err := yaml.Unmarshal(data, &loaded); err != nil {
		return nil, err
	}
	if err := loaded.Validate(); err != nil {
		return nil, err
	}
	loaded.SourcePath = path
	sum := sha256.Sum256(data)
	return &AgentManifestSnapshot{
		Manifest:    &loaded,
		Fingerprint: sum,
		LoadedAt:    time.Now().UTC(),
		SourcePath:  path,
		Warnings:    append([]string{}, loaded.Spec.CompatibilityWarnings...),
	}, nil
}

// LoadAgentManifest parses and validates a manifest file.
func LoadAgentManifest(path string) (*AgentManifest, error) {
	snapshot, err := LoadAgentManifestSnapshot(path)
	if err != nil {
		return nil, err
	}
	return snapshot.Manifest, nil
}

// CloneAgentManifest returns a deep copy of m so callers can mutate the clone
// without affecting the original manifest or snapshot.
func CloneAgentManifest(m *AgentManifest) (*AgentManifest, error) {
	if m == nil {
		return nil, nil
	}
	data, err := yaml.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal manifest clone: %w", err)
	}
	var out AgentManifest
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("unmarshal manifest clone: %w", err)
	}
	out.SourcePath = m.SourcePath
	return &out, nil
}

// Validate enforces manifest semantics.
func (m *AgentManifest) Validate() error {
	if m.APIVersion == "" {
		return fmt.Errorf("manifest missing apiVersion")
	}
	if m.Kind == "" {
		return fmt.Errorf("manifest missing kind")
	}
	if m.Metadata.Name == "" {
		return fmt.Errorf("manifest missing metadata.name")
	}
	if err := m.Spec.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate enforces manifest spec semantics, including the policy/agent split.
func (m *ManifestSpec) Validate() error {
	if m == nil {
		return fmt.Errorf("manifest spec missing")
	}
	if m.Image == "" {
		return fmt.Errorf("manifest missing spec.image")
	}
	if strings.ToLower(m.Runtime) != "gvisor" {
		return fmt.Errorf("runtime must be gVisor, got %s", m.Runtime)
	}
	policy := m.effectivePolicy()
	if hasPermissionScopes(policy.Permissions) {
		if err := policy.Permissions.Validate(); err != nil {
			return fmt.Errorf("permissions invalid: %w", err)
		}
	}
	if policy.Defaults != nil && policy.Defaults.Permissions != nil {
		if err := policy.Defaults.Permissions.Validate(); err != nil {
			return fmt.Errorf("defaults permissions invalid: %w", err)
		}
	}
	if !hasPermissionScopes(policy.Permissions) && (policy.Defaults == nil || policy.Defaults.Permissions == nil) {
		return fmt.Errorf("manifest missing permissions (spec.policy.permissions or spec.policy.defaults.permissions required)")
	}
	if policy.Policies != nil {
		for key, level := range policy.Policies {
			if strings.TrimSpace(key) == "" {
				return fmt.Errorf("policy contains empty key")
			}
			if strings.TrimSpace(string(level)) == "" {
				continue
			}
		}
	}
	if m.Agent != nil {
		if err := m.Agent.Validate(); err != nil {
			return fmt.Errorf("agent spec invalid: %w", err)
		}
	}
	for _, skill := range m.Skills {
		if strings.TrimSpace(skill) == "" {
			return fmt.Errorf("manifest skills contains empty entry")
		}
	}
	return nil
}

func (m *ManifestSpec) UnmarshalYAML(value *yaml.Node) error {
	type manifestSpecAlias ManifestSpec
	var raw manifestSpecAlias
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*m = ManifestSpec(raw)
	if raw.Policy != nil {
		m.applyPolicy(*raw.Policy)
	} else {
		policy := m.policyFromFlat()
		m.Policy = &policy
	}
	m.CompatibilityWarnings = compatibilityWarnings(ManifestSpec(raw))
	return nil
}

func (m ManifestSpec) MarshalYAML() (interface{}, error) {
	type out struct {
		Image   string                 `yaml:"image,omitempty"`
		Runtime string                 `yaml:"runtime,omitempty"`
		Policy  *ManifestPolicySpec    `yaml:"policy,omitempty"`
		Agent   *core.AgentRuntimeSpec `yaml:"agent,omitempty"`
		Skills  []string               `yaml:"skills,omitempty"`
	}
	policy := m.effectivePolicy().clone()
	return out{
		Image:   m.Image,
		Runtime: m.Runtime,
		Policy:  &policy,
		Agent:   m.Agent,
		Skills:  append([]string{}, m.Skills...),
	}, nil
}

func (m *ManifestSpec) applyPolicy(policy ManifestPolicySpec) {
	if m == nil {
		return
	}
	clone := policy.clone()
	m.Policy = &clone
	m.Permissions = clone.Permissions
	m.Resources = clone.Resources
	m.Security = clone.Security
	m.Audit = clone.Audit
	m.Policies = clone.Policies
	m.Defaults = clone.Defaults
}

func (m *ManifestSpec) effectivePolicy() ManifestPolicySpec {
	if m == nil {
		return ManifestPolicySpec{}
	}
	if m.Policy != nil {
		return m.Policy.clone()
	}
	return m.policyFromFlat()
}

func (m *ManifestSpec) policyFromFlat() ManifestPolicySpec {
	if m == nil {
		return ManifestPolicySpec{}
	}
	return ManifestPolicySpec{
		Permissions: m.Permissions,
		Resources:   m.Resources,
		Security:    m.Security,
		Audit:       m.Audit,
		Policies:    cloneAgentPolicyMap(m.Policies),
		Defaults:    cloneManifestDefaults(m.Defaults),
	}
}

func (p ManifestPolicySpec) clone() ManifestPolicySpec {
	return ManifestPolicySpec{
		Permissions: p.Permissions,
		Resources:   p.Resources,
		Security:    p.Security,
		Audit:       p.Audit,
		Policies:    cloneAgentPolicyMap(p.Policies),
		Defaults:    cloneManifestDefaults(p.Defaults),
	}
}

func (p ManifestPolicySpec) hasLegacyFlatFields() bool {
	return hasPermissionScopes(p.Permissions) ||
		p.Resources != (ResourceSpec{}) ||
		p.Security != (SecuritySpec{}) ||
		p.Audit != (AuditSpec{}) ||
		len(p.Policies) > 0 ||
		p.Defaults != nil
}

func cloneAgentPolicyMap(values map[string]core.AgentPermissionLevel) map[string]core.AgentPermissionLevel {
	if values == nil {
		return nil
	}
	out := make(map[string]core.AgentPermissionLevel, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func cloneManifestDefaults(defaults *ManifestDefaults) *ManifestDefaults {
	if defaults == nil {
		return nil
	}
	clone := *defaults
	if defaults.Permissions != nil {
		perms := *defaults.Permissions
		clone.Permissions = &perms
	}
	if defaults.Resources != nil {
		resources := *defaults.Resources
		clone.Resources = &resources
	}
	return &clone
}

func compatibilityWarnings(raw ManifestSpec) []string {
	var warnings []string
	legacyPolicy := ManifestPolicySpec{
		Permissions: raw.Permissions,
		Resources:   raw.Resources,
		Security:    raw.Security,
		Audit:       raw.Audit,
		Policies:    raw.Policies,
		Defaults:    raw.Defaults,
	}
	if raw.Policy == nil && legacyPolicy.hasLegacyFlatFields() {
		warnings = append(warnings, "spec.policy is missing; legacy flat policy fields were loaded")
	}
	if raw.Policy != nil && legacyPolicy.hasLegacyFlatFields() {
		warnings = append(warnings, "legacy flat policy fields were ignored in favor of spec.policy")
	}
	if raw.Agent != nil && raw.Agent.NativeToolCalling != nil && raw.Agent.ToolCallingIntent == "" {
		warnings = append(warnings, "spec.agent.native_tool_calling is deprecated; use spec.agent.tool_calling_intent")
	}
	return warnings
}

func hasPermissionScopes(perms core.PermissionSet) bool {
	return len(perms.FileSystem) > 0 ||
		len(perms.Executables) > 0 ||
		len(perms.Network) > 0 ||
		len(perms.Capabilities) > 0 ||
		len(perms.IPC) > 0
}
