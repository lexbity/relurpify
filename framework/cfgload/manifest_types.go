package cfgload

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/cfgload/secretscan"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"gopkg.in/yaml.v3"
)

const DirName = "relurpify_cfg"

// Paths describes the canonical relurpify_cfg layout for one workspace.
type Paths struct {
	Workspace string
}

// New returns the canonical workspace path layout rooted at workspace.
func New(workspace string) Paths {
	if workspace == "" {
		workspace = "."
	}
	return Paths{Workspace: workspace}
}

func (p Paths) ConfigRoot() string {
	return filepath.Join(p.Workspace, DirName)
}

func (p Paths) WorkspaceFile() string {
	return filepath.Join(p.ConfigRoot(), "workspace.yaml")
}

func (p Paths) AgentsDir() string {
	return filepath.Join(p.ConfigRoot(), "agents")
}

func (p Paths) SkillsDir() string {
	return filepath.Join(p.ConfigRoot(), "skills")
}

func (p Paths) StateRoot() string {
	return filepath.Join(p.Workspace, secretscan.RuntimeStateDirName)
}

func (p Paths) RuntimeWorkspaceFile() string {
	return filepath.Join(p.StateRoot(), "workspace.yaml")
}

func (p Paths) RuntimeProvidersFile() string {
	return filepath.Join(p.StateRoot(), "providers.yaml")
}

func (p Paths) RuntimeKeybindingsFile() string {
	return filepath.Join(p.StateRoot(), "keybindings.yaml")
}

func (p Paths) LogsDir() string {
	return filepath.Join(p.StateRoot(), "logs")
}

func (p Paths) LogFile(name string) string {
	if name == "" {
		name = "relurpish.log"
	}
	return filepath.Join(p.LogsDir(), name)
}

func (p Paths) TelemetryDir() string {
	return filepath.Join(p.StateRoot(), "telemetry")
}

func (p Paths) TelemetryFile(name string) string {
	if name == "" {
		name = "telemetry.jsonl"
	}
	return filepath.Join(p.TelemetryDir(), name)
}

func (p Paths) EventsFile() string {
	return filepath.Join(p.StateRoot(), "events.db")
}

func (p Paths) NodesFile() string {
	return filepath.Join(p.StateRoot(), "nodes.db")
}

func (p Paths) SessionStoreFile() string {
	return filepath.Join(p.StateRoot(), "sessions.db")
}

func (p Paths) IdentityStoreFile() string {
	return filepath.Join(p.StateRoot(), "identities.db")
}

func (p Paths) AdminTokenStoreFile() string {
	return filepath.Join(p.StateRoot(), "admin_tokens.db")
}

func (p Paths) MemoryDir() string {
	return filepath.Join(p.StateRoot(), "memory")
}

func (p Paths) ASTIndexDir() string {
	return filepath.Join(p.MemoryDir(), "ast_index")
}

func (p Paths) ASTIndexDB() string {
	return filepath.Join(p.ASTIndexDir(), "index.db")
}

func (p Paths) RetrievalDB() string {
	return filepath.Join(p.MemoryDir(), "retrieval.db")
}

func (p Paths) SessionsDir() string {
	return filepath.Join(p.StateRoot(), "sessions")
}

func (p Paths) CheckpointsDir() string {
	return filepath.Join(p.SessionsDir(), "checkpoints")
}

func (p Paths) WorkflowStateFile() string {
	return filepath.Join(p.SessionsDir(), "workflow_state.db")
}

func (p Paths) ExportsDir() string {
	return filepath.Join(p.StateRoot(), "exports")
}

func (p Paths) TestsuitesDir() string {
	return filepath.Join(p.StateRoot(), "testsuites")
}

func (p Paths) TestRunsDir() string {
	return filepath.Join(p.StateRoot(), "test_run")
}

func (p Paths) TestSetupDir(parts ...string) string {
	segments := append([]string{p.TestRunsDir()}, parts...)
	segments = append(segments, "setup")
	return filepath.Join(segments...)
}

func (p Paths) TestRunDir(parts ...string) string {
	segments := append([]string{p.TestRunsDir()}, parts...)
	segments = append(segments, "execution")
	return filepath.Join(segments...)
}

func (p Paths) TestRunLogsDir(parts ...string) string {
	segments := append([]string{p.TestRunDir(parts...)}, "logs")
	return filepath.Join(segments...)
}

func (p Paths) TestRunTelemetryDir(parts ...string) string {
	segments := append([]string{p.TestRunDir(parts...)}, "telemetry")
	return filepath.Join(segments...)
}

func (p Paths) TestRunArtifactsDir(parts ...string) string {
	segments := append([]string{p.TestRunDir(parts...)}, "artifacts")
	return filepath.Join(segments...)
}

func (p Paths) TestRunTmpDir(parts ...string) string {
	segments := append([]string{p.TestRunsDir()}, parts...)
	segments = append(segments, "tmp")
	return filepath.Join(segments...)
}

func (p Paths) ModelProfilesDir() string {
	return filepath.Join(p.ConfigRoot(), "model", "profiles")
}

// GovernanceRoots returns the canonical workspace governance paths that should
// be protected from agent writes and executable mutation, including the
// relurpify_cfg root itself.
func (p Paths) GovernanceRoots(extra ...string) []string {
	roots := []string{
		p.ConfigRoot(),
		p.WorkspaceFile(),
		p.AgentsDir(),
		p.ModelProfilesDir(),
	}
	for _, path := range extra {
		if strings.TrimSpace(path) == "" {
			continue
		}
		roots = append(roots, path)
	}
	return roots
}

// AgentManifest defines the file-backed security contract for an agent.
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
	Image       string                                    `yaml:"image" json:"image"`
	Runtime     string                                    `yaml:"runtime" json:"runtime"`
	Policy      *ManifestPolicySpec                       `yaml:"policy,omitempty" json:"policy,omitempty"`
	Permissions contracts.PermissionSet                   `yaml:"permissions" json:"permissions"`
	Resources   ResourceSpec                              `yaml:"resources" json:"resources"`
	Security    SecuritySpec                              `yaml:"security" json:"security"`
	Audit       AuditSpec                                 `yaml:"audit" json:"audit"`
	Agent       *agentspec.AgentRuntimeSpec               `yaml:"agent,omitempty" json:"agent,omitempty"`
	Skills      []string                                  `yaml:"skills,omitempty" json:"skills,omitempty"`
	Policies    map[string]agentspec.AgentPermissionLevel `yaml:"policies,omitempty" json:"policies,omitempty"`
	Defaults    *ManifestDefaults                         `yaml:"defaults,omitempty" json:"defaults,omitempty"`
	// Context holds the context policy configuration for ingestion and persistence.
	Context *ContextPolicy `yaml:"context,omitempty" json:"context,omitempty"`

	CompatibilityWarnings []string `yaml:"-" json:"-"`
}

// ManifestPolicySpec groups policy-adjacent fields under spec.policy.
type ManifestPolicySpec struct {
	Permissions contracts.PermissionSet                   `yaml:"permissions,omitempty" json:"permissions,omitempty"`
	Resources   ResourceSpec                              `yaml:"resources,omitempty" json:"resources,omitempty"`
	Security    SecuritySpec                              `yaml:"security,omitempty" json:"security,omitempty"`
	Audit       AuditSpec                                 `yaml:"audit,omitempty" json:"audit,omitempty"`
	Policies    map[string]agentspec.AgentPermissionLevel `yaml:"policies,omitempty" json:"policies,omitempty"`
	Defaults    *ManifestDefaults                         `yaml:"defaults,omitempty" json:"defaults,omitempty"`
}

// ManifestDefaults defines global defaults applied before skills.
type ManifestDefaults struct {
	Permissions *contracts.PermissionSet `yaml:"permissions,omitempty" json:"permissions,omitempty"`
	Resources   *ResourceSpec            `yaml:"resources,omitempty" json:"resources,omitempty"`
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
func SaveAgentManifest(path string, m *AgentManifest) error {
	return WriteWithSchema(path, "relurpify/agent/v1", m)
}

// manifestSchemaRegistry returns a schema registry that includes the legacy
// agent manifest schema kind alongside current kinds. Used only for loading
// runtime manifest files that were written with schema: relurpify/agent/v1.
func manifestSchemaRegistry() *SchemaRegistry {
	reg := NewSchemaRegistry()
	_ = reg.Register("agent", 1)
	return reg
}

// LoadAgentManifestSnapshot parses, validates, and fingerprints a manifest file.
func LoadAgentManifestSnapshot(path string) (*AgentManifestSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var loaded AgentManifest
	if _, err := DecodeWithSchema(path, data, manifestSchemaRegistry(), &loaded); err != nil {
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
		Image   string                      `yaml:"image,omitempty"`
		Runtime string                      `yaml:"runtime,omitempty"`
		Policy  *ManifestPolicySpec         `yaml:"policy,omitempty"`
		Agent   *agentspec.AgentRuntimeSpec `yaml:"agent,omitempty"`
		Skills  []string                    `yaml:"skills,omitempty"`
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

func cloneAgentPolicyMap(values map[string]agentspec.AgentPermissionLevel) map[string]agentspec.AgentPermissionLevel {
	if values == nil {
		return nil
	}
	out := make(map[string]agentspec.AgentPermissionLevel, len(values))
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

func hasPermissionScopes(perms contracts.PermissionSet) bool {
	return len(perms.FileSystem) > 0 ||
		len(perms.Executables) > 0 ||
		len(perms.Network) > 0 ||
		len(perms.Capabilities) > 0 ||
		len(perms.IPC) > 0
}

// ContextPolicy defines the context policy section in an agent manifest.
// This mirrors contextpolicy.ContextPolicy to avoid import cycles.
type ContextPolicy struct {
	CompilationMode       string                    `yaml:"compilation_mode,omitempty" json:"compilation_mode,omitempty"`
	DefaultTrustClass     agentspec.TrustClass      `yaml:"default_trust_class,omitempty" json:"default_trust_class,omitempty"`
	Rankers               []agentspec.RankerRef     `yaml:"rankers,omitempty" json:"rankers,omitempty"`
	Scanners              []agentspec.ScannerRef    `yaml:"scanners,omitempty" json:"scanners,omitempty"`
	Summarizers           []agentspec.SummarizerRef `yaml:"summarizers,omitempty" json:"summarizers,omitempty"`
	Quota                 *QuotaSpec                `yaml:"quota,omitempty" json:"quota,omitempty"`
	RateLimit             *RateLimitSpec            `yaml:"rate_limit,omitempty" json:"rate_limit,omitempty"`
	TrustDemotedPolicy    string                    `yaml:"trust_demoted_policy,omitempty" json:"trust_demoted_policy,omitempty"`
	DegradedChunkPolicy   string                    `yaml:"degraded_chunk_policy,omitempty" json:"degraded_chunk_policy,omitempty"`
	BudgetShortfallPolicy string                    `yaml:"budget_shortfall_policy,omitempty" json:"budget_shortfall_policy,omitempty"`
	SubstitutionPrefs     []SubstitutionPreference  `yaml:"substitution_preferences,omitempty" json:"substitution_preferences,omitempty"`
}

// QuotaSpec defines quota limits.
type QuotaSpec struct {
	WindowSize         string `yaml:"window_size,omitempty" json:"window_size,omitempty"`
	MaxChunksPerWindow int    `yaml:"max_chunks_per_window,omitempty" json:"max_chunks_per_window,omitempty"`
	MaxTokensPerWindow int    `yaml:"max_tokens_per_window,omitempty" json:"max_tokens_per_window,omitempty"`
	PrincipalPattern   string `yaml:"principal_pattern,omitempty" json:"principal_pattern,omitempty"`
}

// RateLimitSpec defines rate limiting configuration.
type RateLimitSpec struct {
	RequestsPerSecond float64 `yaml:"requests_per_second,omitempty" json:"requests_per_second,omitempty"`
	BurstSize         int     `yaml:"burst_size,omitempty" json:"burst_size,omitempty"`
}

// SubstitutionPreference defines how to substitute content.
type SubstitutionPreference struct {
	SourceContentType string `yaml:"source_content_type,omitempty" json:"source_content_type,omitempty"`
	TargetContentType string `yaml:"target_content_type,omitempty" json:"target_content_type,omitempty"`
	Strategy          string `yaml:"strategy,omitempty" json:"strategy,omitempty"`
}

// SkillManifest defines a reusable skill package.
type SkillManifest struct {
	APIVersion string           `yaml:"apiVersion" json:"apiVersion"`
	Kind       string           `yaml:"kind" json:"kind"`
	Metadata   ManifestMetadata `yaml:"metadata" json:"metadata"`
	Spec       SkillSpec        `yaml:"spec" json:"spec"`
	SourcePath string           `yaml:"-" json:"-"`
}

// SkillSpec defines the thin skill bundle surface: tools and prompts.
type SkillSpec struct {
	Requires            SkillRequiresSpec              `yaml:"requires,omitempty" json:"requires,omitempty"`
	PromptSnippets      []string                       `yaml:"prompt_snippets,omitempty" json:"prompt_snippets,omitempty"`
	AllowedCapabilities []agentspec.CapabilitySelector `yaml:"allowed_capabilities,omitempty" json:"allowed_capabilities,omitempty"`
}

// SkillRequiresSpec declares binary prerequisites for a skill.
type SkillRequiresSpec struct {
	Bins []string `yaml:"bins,omitempty" json:"bins,omitempty"`
}

// LoadSkillManifest parses and validates a skill manifest file.
func LoadSkillManifest(path string) (*SkillManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest SkillManifest
	if _, err := DecodeWithSchema(path, data, NewSchemaRegistry(), &manifest); err != nil {
		return nil, err
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	manifest.SourcePath = path
	return &manifest, nil
}

// Validate enforces skill manifest semantics.
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
	return nil
}

// LoadSkill loads a skill manifest from the canonical workspace skills directory.
func LoadSkill(workspace, name string) (*SkillManifest, error) {
	if strings.TrimSpace(workspace) == "" {
		return nil, fmt.Errorf("workspace required")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("skill name required")
	}
	manifestPath := filepath.Join(New(workspace).SkillsDir(), name+".skill.yaml")
	return LoadSkillManifest(manifestPath)
}

// LoadSkillList loads a collection of skills by name.
func LoadSkillList(workspace string, names []string) []*SkillManifest {
	var loaded []*SkillManifest
	for _, name := range names {
		skill, err := LoadSkill(workspace, name)
		if err != nil {
			continue
		}
		loaded = append(loaded, skill)
	}
	return loaded
}
