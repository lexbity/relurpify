package framework

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/cfgload"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// WorkspaceBuilder builds deterministic workspace fixtures.
type WorkspaceBuilder struct {
	basePath string
	files    map[string]string
	dirs     []string
}

// NewWorkspaceBuilder creates a new workspace builder.
func NewWorkspaceBuilder(basePath string) *WorkspaceBuilder {
	return &WorkspaceBuilder{
		basePath: basePath,
		files:    make(map[string]string),
		dirs:     make([]string, 0),
	}
}

// NewTempWorkspaceBuilder creates a workspace builder rooted in a fresh temp directory.
func NewTempWorkspaceBuilder(t *testing.T) *WorkspaceBuilder {
	t.Helper()
	return NewWorkspaceBuilder(t.TempDir())
}

// WithFile adds a file to the workspace.
func (b *WorkspaceBuilder) WithFile(path, content string) *WorkspaceBuilder {
	b.files[path] = content
	return b
}

// WithDirectory adds a directory to the workspace.
func (b *WorkspaceBuilder) WithDirectory(path string) *WorkspaceBuilder {
	b.dirs = append(b.dirs, path)
	return b
}

// Build creates the workspace directory structure and files.
func (b *WorkspaceBuilder) Build() error {
	if b == nil {
		return fmt.Errorf("workspace builder is nil")
	}
	if strings.TrimSpace(b.basePath) == "" {
		return fmt.Errorf("workspace builder base path is empty")
	}
	if err := os.MkdirAll(b.basePath, 0o755); err != nil {
		return err
	}

	// Create directories first
	for _, dir := range uniqueSortedStrings(b.dirs) {
		fullPath := filepath.Join(b.basePath, dir)
		if err := os.MkdirAll(fullPath, 0o755); err != nil {
			return err
		}
	}

	// Create files
	paths := make([]string, 0, len(b.files))
	for path := range b.files {
		paths = append(paths, path)
	}
	for _, path := range SortStrings(paths) {
		content := b.files[path]
		fullPath := filepath.Join(b.basePath, path)
		dir := filepath.Dir(fullPath)
		if dir != b.basePath && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			return err
		}
	}

	return nil
}

// SmallWorkspace returns a builder for a small, simple workspace fixture.
func SmallWorkspace(basePath string) *WorkspaceBuilder {
	return NewWorkspaceBuilder(basePath).
		WithFile("main.go", "package main\n\nfunc main() {}\n").
		WithFile("README.md", "# Test Project\n").
		WithFile("config.yaml", "key: value\n")
}

// MixedLanguageWorkspace returns a builder for a workspace with multiple language files.
func MixedLanguageWorkspace(basePath string) *WorkspaceBuilder {
	return NewWorkspaceBuilder(basePath).
		WithDirectory("src").
		WithDirectory("python").
		WithFile("main.go", "package main\n\nfunc main() {}\n").
		WithFile("src/helper.go", "package src\n\nfunc Helper() {}\n").
		WithFile("python/script.py", "#!/usr/bin/env python3\nprint('hello')\n").
		WithFile("README.md", "# Mixed Language Project\n").
		WithFile("config.json", `{"key": "value"}`+"\n")
}

// ManifestBuilder builds deterministic manifest fixtures.
type ManifestBuilder struct {
	manifest *cfgload.AgentManifest
}

// NewManifestBuilder creates a new manifest builder with defaults.
func NewManifestBuilder() *ManifestBuilder {
	return &ManifestBuilder{
		manifest: &cfgload.AgentManifest{
			APIVersion: "relurpify/v1alpha1",
			Kind:       "AgentManifest",
			Metadata: cfgload.ManifestMetadata{
				Name:    "test-agent",
				Version: "1.0.0",
			},
			Spec: cfgload.ManifestSpec{
				Image:   "test-image:latest",
				Runtime: "gvisor",
				Policy: &cfgload.ManifestPolicySpec{
					Permissions: contracts.PermissionSet{
						FileSystem: []contracts.FileSystemPermission{
							{Action: contracts.FileSystemRead, Path: "${workspace}/**"},
							{Action: contracts.FileSystemWrite, Path: "${workspace}/**"},
						},
					},
				},
			},
		},
	}
}

// WithName sets the manifest name.
func (b *ManifestBuilder) WithName(name string) *ManifestBuilder {
	b.manifest.Metadata.Name = name
	return b
}

// WithVersion sets the manifest version.
func (b *ManifestBuilder) WithVersion(version string) *ManifestBuilder {
	b.manifest.Metadata.Version = version
	return b
}

// WithFileSystemPermission adds a filesystem permission.
func (b *ManifestBuilder) WithFileSystemPermission(action contracts.FileSystemAction, path string) *ManifestBuilder {
	if b.manifest.Spec.Policy == nil {
		b.manifest.Spec.Policy = &cfgload.ManifestPolicySpec{}
	}
	b.manifest.Spec.Policy.Permissions.FileSystem = append(
		b.manifest.Spec.Policy.Permissions.FileSystem,
		contracts.FileSystemPermission{Action: action, Path: path},
	)
	return b
}

// WithNetworkPermission adds a network permission.
func (b *ManifestBuilder) WithNetworkPermission(direction, protocol, host string, port int) *ManifestBuilder {
	if b.manifest.Spec.Policy == nil {
		b.manifest.Spec.Policy = &cfgload.ManifestPolicySpec{}
	}
	b.manifest.Spec.Policy.Permissions.Network = append(
		b.manifest.Spec.Policy.Permissions.Network,
		contracts.NetworkPermission{Direction: direction, Protocol: protocol, Host: host, Port: port},
	)
	return b
}

// WithHITLRequired marks a permission as requiring HITL approval.
func (b *ManifestBuilder) WithHITLRequired() *ManifestBuilder {
	if len(b.manifest.Spec.Policy.Permissions.FileSystem) > 0 {
		b.manifest.Spec.Policy.Permissions.FileSystem[len(b.manifest.Spec.Policy.Permissions.FileSystem)-1].HITLRequired = true
	}
	if len(b.manifest.Spec.Policy.Permissions.Network) > 0 {
		b.manifest.Spec.Policy.Permissions.Network[len(b.manifest.Spec.Policy.Permissions.Network)-1].HITLRequired = true
	}
	return b
}

// Build returns the constructed manifest.
func (b *ManifestBuilder) Build() *cfgload.AgentManifest {
	if b == nil || b.manifest == nil {
		return nil
	}
	clone, err := cfgload.CloneAgentManifest(b.manifest)
	if err != nil {
		return b.manifest
	}
	clone.Spec.Permissions.Sort()
	if clone.Spec.Policy != nil {
		clone.Spec.Policy.Permissions.Sort()
		if clone.Spec.Policy.Defaults != nil && clone.Spec.Policy.Defaults.Permissions != nil {
			clone.Spec.Policy.Defaults.Permissions.Sort()
		}
	}
	if clone.Spec.Defaults != nil && clone.Spec.Defaults.Permissions != nil {
		clone.Spec.Defaults.Permissions.Sort()
	}
	return clone
}

// ValidManifest returns a builder for a valid manifest fixture.
func ValidManifest() *ManifestBuilder {
	return NewManifestBuilder()
}

// InvalidManifestMissingAPIVersion returns a builder for an invalid manifest (missing apiVersion).
func InvalidManifestMissingAPIVersion() *ManifestBuilder {
	return &ManifestBuilder{
		manifest: &cfgload.AgentManifest{
			Kind: "AgentManifest",
			Metadata: cfgload.ManifestMetadata{
				Name:    "test-agent",
				Version: "1.0.0",
			},
			Spec: cfgload.ManifestSpec{
				Image:   "test-image:latest",
				Runtime: "gvisor",
			},
		},
	}
}

// InvalidManifestMissingKind returns a builder for an invalid manifest (missing kind).
func InvalidManifestMissingKind() *ManifestBuilder {
	return &ManifestBuilder{
		manifest: &cfgload.AgentManifest{
			APIVersion: "relurpify/v1alpha1",
			Metadata: cfgload.ManifestMetadata{
				Name:    "test-agent",
				Version: "1.0.0",
			},
			Spec: cfgload.ManifestSpec{
				Image:   "test-image:latest",
				Runtime: "gvisor",
			},
		},
	}
}

// PolicyRuleBuilder builds deterministic policy rule fixtures.
type PolicyRuleBuilder struct {
	rules []core.PolicyRule
}

// NewPolicyRuleBuilder creates a new policy rule builder.
func NewPolicyRuleBuilder() *PolicyRuleBuilder {
	return &PolicyRuleBuilder{
		rules: make([]core.PolicyRule, 0),
	}
}

// WithAllowRule adds an allow rule.
func (b *PolicyRuleBuilder) WithAllowRule(id, capability string) *PolicyRuleBuilder {
	b.rules = append(b.rules, core.PolicyRule{
		ID:       id,
		Name:     id,
		Enabled:  true,
		Priority: 100,
		Conditions: core.PolicyConditions{
			Capabilities: []string{capability},
		},
		Effect: core.PolicyEffect{
			Action: "allow",
		},
	})
	return b
}

// WithDenyRule adds a deny rule.
func (b *PolicyRuleBuilder) WithDenyRule(id, capability, reason string) *PolicyRuleBuilder {
	b.rules = append(b.rules, core.PolicyRule{
		ID:       id,
		Name:     id,
		Enabled:  true,
		Priority: 100,
		Conditions: core.PolicyConditions{
			Capabilities: []string{capability},
		},
		Effect: core.PolicyEffect{
			Action: "deny",
			Reason: reason,
		},
	})
	return b
}

// Build returns the constructed policy rules.
func (b *PolicyRuleBuilder) Build() []core.PolicyRule {
	if b == nil || len(b.rules) == 0 {
		return nil
	}
	rules := make([]core.PolicyRule, len(b.rules))
	for i, rule := range b.rules {
		rules[i] = clonePolicyRule(rule)
	}
	return rules
}

// AllowAllPolicy returns a builder for an allow-all policy.
func AllowAllPolicy() *PolicyRuleBuilder {
	return NewPolicyRuleBuilder().
		WithAllowRule("allow-1", "tool:*")
}

// DenySpecificPolicy returns a builder for a policy that denies specific capabilities.
func DenySpecificPolicy(capability string) *PolicyRuleBuilder {
	return NewPolicyRuleBuilder().
		WithAllowRule("allow-all", "tool:*").
		WithDenyRule("deny-specific", capability, "security restriction")
}

// EnvelopeBuilder builds deterministic envelope fixtures.
type EnvelopeBuilder struct {
	taskID    string
	sessionID string
	nodeID    string
	data      []workingValueFixture
}

type workingValueFixture struct {
	key   string
	value any
	class contextdata.MemoryClass
}

// NewEnvelopeBuilder creates a new envelope builder.
func NewEnvelopeBuilder() *EnvelopeBuilder {
	return &EnvelopeBuilder{
		taskID:    "test-task",
		sessionID: "test-session",
		nodeID:    "test-node",
		data:      make([]workingValueFixture, 0),
	}
}

// WithTaskID sets the task ID.
func (b *EnvelopeBuilder) WithTaskID(taskID string) *EnvelopeBuilder {
	b.taskID = taskID
	return b
}

// WithSessionID sets the session ID.
func (b *EnvelopeBuilder) WithSessionID(sessionID string) *EnvelopeBuilder {
	b.sessionID = sessionID
	return b
}

// WithNodeID sets the node ID.
func (b *EnvelopeBuilder) WithNodeID(nodeID string) *EnvelopeBuilder {
	b.nodeID = nodeID
	return b
}

// WithWorkingValue adds a working memory value.
func (b *EnvelopeBuilder) WithWorkingValue(key string, value any, class contextdata.MemoryClass) *EnvelopeBuilder {
	b.data = append(b.data, workingValueFixture{key: key, value: value, class: class})
	return b
}

// Build returns the constructed envelope.
func (b *EnvelopeBuilder) Build() *contextdata.Envelope {
	if b == nil {
		return nil
	}
	env := contextdata.NewEnvelope(b.taskID, b.sessionID)
	env.NodeID = b.nodeID
	for _, item := range b.data {
		env.SetWorkingValue(item.key, item.value, item.class)
	}
	return env
}

// MinimalEnvelope returns a builder for a minimal envelope fixture.
func MinimalEnvelope() *EnvelopeBuilder {
	return NewEnvelopeBuilder()
}

// AuditRecordBuilder builds deterministic audit record fixtures.
type AuditRecordBuilder struct {
	record core.AuditRecord
	meta   map[string]interface{}
}

// NewAuditRecordBuilder creates a new audit record builder.
func NewAuditRecordBuilder() *AuditRecordBuilder {
	return &AuditRecordBuilder{
		record: core.AuditRecord{
			AgentID:     "test-agent",
			Action:      string(core.AuditActionRequest),
			Type:        string(core.AuditActionRequest),
			Permission:  "test_permission",
			Result:      "granted",
			Correlation: "test-correlation",
		},
		meta: make(map[string]interface{}),
	}
}

// WithAgentID sets the agent ID.
func (b *AuditRecordBuilder) WithAgentID(agentID string) *AuditRecordBuilder {
	b.record.AgentID = agentID
	return b
}

// WithAction sets the action.
func (b *AuditRecordBuilder) WithAction(action string) *AuditRecordBuilder {
	b.record.Action = action
	return b
}

// WithType sets the type.
func (b *AuditRecordBuilder) WithType(typ string) *AuditRecordBuilder {
	b.record.Type = typ
	return b
}

// WithPermission sets the permission.
func (b *AuditRecordBuilder) WithPermission(permission string) *AuditRecordBuilder {
	b.record.Permission = permission
	return b
}

// WithResult sets the result.
func (b *AuditRecordBuilder) WithResult(result string) *AuditRecordBuilder {
	b.record.Result = result
	return b
}

// WithMetadata sets a metadata key/value on the audit record.
func (b *AuditRecordBuilder) WithMetadata(key string, value interface{}) *AuditRecordBuilder {
	if b.meta == nil {
		b.meta = make(map[string]interface{})
	}
	b.meta[key] = value
	return b
}

// WithCorrelation sets the correlation ID.
func (b *AuditRecordBuilder) WithCorrelation(correlation string) *AuditRecordBuilder {
	b.record.Correlation = correlation
	return b
}

// Build returns the constructed audit record.
func (b *AuditRecordBuilder) Build() core.AuditRecord {
	if b == nil {
		return core.AuditRecord{}
	}
	out := b.record
	if len(b.meta) > 0 {
		out.Metadata = make(map[string]interface{}, len(b.meta))
		for key, value := range b.meta {
			out.Metadata[key] = value
		}
	}
	return out
}

// GrantedAuditRecord returns a builder for a granted audit record.
func GrantedAuditRecord() *AuditRecordBuilder {
	return NewAuditRecordBuilder().WithResult("granted")
}

// DeniedAuditRecord returns a builder for a denied audit record.
func DeniedAuditRecord(reason string) *AuditRecordBuilder {
	return NewAuditRecordBuilder().WithResult("denied").WithMetadata("reason", reason)
}

// SortStrings sorts a slice of strings for deterministic comparison.
func SortStrings(s []string) []string {
	sorted := make([]string, len(s))
	copy(sorted, s)
	sort.Strings(sorted)
	return sorted
}

// NormalizePath normalizes a path for comparison.
func NormalizePath(path string) string {
	path = filepath.Clean(path)
	path = filepath.ToSlash(path)
	path = strings.TrimPrefix(path, "./")
	return path
}

// NormalizePaths normalizes a slice of paths for comparison.
func NormalizePaths(paths []string) []string {
	normalized := make([]string, len(paths))
	for i, path := range paths {
		normalized[i] = NormalizePath(path)
	}
	return SortStrings(normalized)
}

// BuildWorkspace is a convenience function that builds a workspace from a builder.
func BuildWorkspace(t *testing.T, builder *WorkspaceBuilder) string {
	t.Helper()
	if builder == nil {
		t.Fatal("workspace builder is nil")
	}
	if err := builder.Build(); err != nil {
		t.Fatalf("failed to build workspace: %v", err)
	}
	return builder.basePath
}

// NormalizeTelemetryEvents normalizes telemetry events for deterministic comparison.
func NormalizeTelemetryEvents(events []core.Event) []core.Event {
	if len(events) == 0 {
		return nil
	}
	normalized := make([]core.Event, len(events))
	for i, event := range events {
		normalized[i] = event
		normalized[i].Timestamp = time.Time{}
		normalized[i].Metadata = cloneInterfaceMap(event.Metadata)
	}
	return normalized
}

// NormalizeAuditRecords normalizes audit records for deterministic comparison.
func NormalizeAuditRecords(records []core.AuditRecord) []core.AuditRecord {
	if len(records) == 0 {
		return nil
	}
	normalized := make([]core.AuditRecord, len(records))
	for i, record := range records {
		normalized[i] = record
		normalized[i].Timestamp = time.Time{}
		normalized[i].Metadata = cloneInterfaceMap(record.Metadata)
	}
	return normalized
}

// NormalizeFileSystemPermissions normalizes filesystem permissions for deterministic comparison.
func NormalizeFileSystemPermissions(perms []contracts.FileSystemPermission) []contracts.FileSystemPermission {
	if len(perms) == 0 {
		return nil
	}
	normalized := make([]contracts.FileSystemPermission, len(perms))
	copy(normalized, perms)
	sort.SliceStable(normalized, func(i, j int) bool {
		if normalized[i].Action != normalized[j].Action {
			return normalized[i].Action < normalized[j].Action
		}
		if normalized[i].Path != normalized[j].Path {
			return NormalizePath(normalized[i].Path) < NormalizePath(normalized[j].Path)
		}
		if normalized[i].Justification != normalized[j].Justification {
			return normalized[i].Justification < normalized[j].Justification
		}
		if normalized[i].HITLRequired != normalized[j].HITLRequired {
			return !normalized[i].HITLRequired && normalized[j].HITLRequired
		}
		return !normalized[i].ReadOnlyMount && normalized[j].ReadOnlyMount
	})
	for i := range normalized {
		normalized[i].Path = NormalizePath(normalized[i].Path)
	}
	return normalized
}

// NormalizeNetworkPermissions normalizes network permissions for deterministic comparison.
func NormalizeNetworkPermissions(perms []contracts.NetworkPermission) []contracts.NetworkPermission {
	if len(perms) == 0 {
		return nil
	}
	normalized := make([]contracts.NetworkPermission, len(perms))
	copy(normalized, perms)
	sort.SliceStable(normalized, func(i, j int) bool {
		if normalized[i].Direction != normalized[j].Direction {
			return normalized[i].Direction < normalized[j].Direction
		}
		if normalized[i].Protocol != normalized[j].Protocol {
			return normalized[i].Protocol < normalized[j].Protocol
		}
		if normalized[i].Host != normalized[j].Host {
			return normalized[i].Host < normalized[j].Host
		}
		if normalized[i].Port != normalized[j].Port {
			return normalized[i].Port < normalized[j].Port
		}
		if normalized[i].Description != normalized[j].Description {
			return normalized[i].Description < normalized[j].Description
		}
		return !normalized[i].HITLRequired && normalized[j].HITLRequired
	})
	return normalized
}

// NormalizePolicyRules normalizes policy rules for deterministic comparison.
func NormalizePolicyRules(rules []core.PolicyRule) []core.PolicyRule {
	if len(rules) == 0 {
		return nil
	}
	normalized := make([]core.PolicyRule, len(rules))
	for i, rule := range rules {
		normalized[i] = clonePolicyRule(rule)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		if normalized[i].Priority != normalized[j].Priority {
			return normalized[i].Priority < normalized[j].Priority
		}
		if normalized[i].ID != normalized[j].ID {
			return normalized[i].ID < normalized[j].ID
		}
		return normalized[i].Name < normalized[j].Name
	})
	return normalized
}

func clonePolicyRule(rule core.PolicyRule) core.PolicyRule {
	cloned := rule
	cloned.Conditions = core.PolicyConditions{
		Actors:                    append([]core.ActorMatch(nil), rule.Conditions.Actors...),
		Capabilities:              append([]string(nil), rule.Conditions.Capabilities...),
		ExportNames:               append([]string(nil), rule.Conditions.ExportNames...),
		SourceDomains:             append([]string(nil), rule.Conditions.SourceDomains...),
		ContextClasses:            append([]string(nil), rule.Conditions.ContextClasses...),
		SensitivityClasses:        append([]string(nil), rule.Conditions.SensitivityClasses...),
		RouteModes:                append([]string(nil), rule.Conditions.RouteModes...),
		ProviderKinds:             append([]core.ProviderKind(nil), rule.Conditions.ProviderKinds...),
		ExternalProviders:         append([]string(nil), rule.Conditions.ExternalProviders...),
		MinRiskClasses:            append([]agentspec.RiskClass(nil), rule.Conditions.MinRiskClasses...),
		TrustClasses:              append([]agentspec.TrustClass(nil), rule.Conditions.TrustClasses...),
		CapabilityKinds:           append([]agentspec.CapabilityKind(nil), rule.Conditions.CapabilityKinds...),
		RuntimeFamilies:           append([]agentspec.CapabilityRuntimeFamily(nil), rule.Conditions.RuntimeFamilies...),
		EffectClasses:             append([]agentspec.EffectClass(nil), rule.Conditions.EffectClasses...),
		Partitions:                append([]string(nil), rule.Conditions.Partitions...),
		ChannelIDs:                append([]string(nil), rule.Conditions.ChannelIDs...),
		SessionScopes:             append([]core.SessionScope(nil), rule.Conditions.SessionScopes...),
		SessionOperations:         append([]core.SessionOperation(nil), rule.Conditions.SessionOperations...),
		RequireOwnership:          cloneBoolPtr(rule.Conditions.RequireOwnership),
		RequireDelegation:         cloneBoolPtr(rule.Conditions.RequireDelegation),
		RequireExternalBinding:    cloneBoolPtr(rule.Conditions.RequireExternalBinding),
		RequireResolvedExternal:   cloneBoolPtr(rule.Conditions.RequireResolvedExternal),
		RequireRestrictedExternal: cloneBoolPtr(rule.Conditions.RequireRestrictedExternal),
		TimeWindow:                cloneTimeWindow(rule.Conditions.TimeWindow),
	}
	cloned.Effect = core.PolicyEffect{
		Action:      rule.Effect.Action,
		Approvers:   append([]string(nil), rule.Effect.Approvers...),
		ApprovalTTL: rule.Effect.ApprovalTTL,
		RateLimit:   cloneRateLimit(rule.Effect.RateLimit),
		Reason:      rule.Effect.Reason,
	}
	return cloned
}

func cloneBoolPtr(v *bool) *bool {
	if v == nil {
		return nil
	}
	cloned := *v
	return &cloned
}

func cloneTimeWindow(v *core.TimeWindow) *core.TimeWindow {
	if v == nil {
		return nil
	}
	cloned := *v
	cloned.Days = append([]string(nil), v.Days...)
	return &cloned
}

func cloneRateLimit(v *core.RateLimit) *core.RateLimit {
	if v == nil {
		return nil
	}
	cloned := *v
	return &cloned
}

func cloneInterfaceMap(values map[string]interface{}) map[string]interface{} {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]interface{}, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	items := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		items = append(items, value)
	}
	return SortStrings(items)
}

// NormalizeChunkIDs normalizes chunk IDs for deterministic comparison.
func NormalizeChunkIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	normalized := make([]string, len(ids))
	copy(normalized, ids)
	sort.Strings(normalized)
	return normalized
}
