package framework

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/governance/classification"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/governance/risk"
	"codeburg.org/lexbit/relurpify/platform/fs"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

const (
	documentAPIVersion = "relurpify.io/v1"
	documentKind       = "AgentManifest"
	documentName       = "test-agent"
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
	if err := fs.MkdirAllSecure(b.basePath); err != nil {
		return err
	}

	// Create directories first
	for _, dir := range uniqueSortedStrings(b.dirs) {
		fullPath := filepath.Join(b.basePath, dir)
		if err := fs.MkdirAllSecure(fullPath); err != nil {
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
			if err := fs.MkdirAllSecure(dir); err != nil {
				return err
			}
		}
		if err := fs.WriteFileSecure(fullPath, []byte(content)); err != nil {
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

// DocumentBuilder builds deterministic document fixtures.
type DocumentBuilder struct {
	document *config.Document
}

// NewDocumentBuilder creates a new document builder with defaults.
func NewDocumentBuilder() *DocumentBuilder {
	doc := &config.Document{
		APIVersion: documentAPIVersion,
		Kind:       documentKind,
		Metadata:   config.DocumentMetadata{Name: documentName},
		Spec:       map[string]yaml.Node{},
	}
	builder := &DocumentBuilder{document: doc}
	builder.WithFileSystemPermission(permissions.FileSystemRead, "${workspace}/**")
	builder.WithFileSystemPermission(permissions.FileSystemWrite, "${workspace}/**")
	builder.document.Spec["agent"] = mustEncodeAgentSpec(agentspec.AgentRuntimeSpec{
		Mode:  agentspec.AgentModePrimary,
		Model: agentspec.AgentModelConfig{Provider: "test-provider", Name: "test-model"},
		Bash:  agentspec.AgentBashPermissions{Default: agentspec.AgentPermissionAsk},
	})
	return builder
}

// WithName is retained for compatibility but no longer stored.
func (b *DocumentBuilder) WithName(name string) *DocumentBuilder {
	if b != nil && b.document != nil {
		b.document.Metadata.Name = name
	}
	return b
}

// WithVersion is retained for compatibility but no longer stored.
func (b *DocumentBuilder) WithVersion(version string) *DocumentBuilder {
	if b != nil && b.document != nil {
		b.document.Metadata.Version = version
	}
	return b
}

// WithFileSystemPermission adds a filesystem permission.
func (b *DocumentBuilder) WithFileSystemPermission(action permissions.FileSystemAction, path string) *DocumentBuilder {
	if b == nil || b.document == nil {
		return b
	}
	ps := b.permissionSet()
	ps.FileSystem = append(ps.FileSystem, permissions.FileSystemPermission{Action: action, Path: path})
	b.setPermissionSet(ps)
	return b
}

// WithNetworkPermission adds a network permission.
func (b *DocumentBuilder) WithNetworkPermission(direction, protocol, host string, port int) *DocumentBuilder {
	if b == nil || b.document == nil {
		return b
	}
	ps := b.permissionSet()
	ps.Network = append(ps.Network, permissions.NetworkPermission{Direction: direction, Protocol: protocol, Host: host, Port: port})
	b.setPermissionSet(ps)
	return b
}

// WithHITLRequired marks a permission as requiring HITL approval.
func (b *DocumentBuilder) WithHITLRequired() *DocumentBuilder {
	if b == nil || b.document == nil {
		return b
	}
	ps := b.permissionSet()
	if len(ps.FileSystem) > 0 {
		ps.FileSystem[len(ps.FileSystem)-1].HITLRequired = true
	}
	if len(ps.Network) > 0 {
		ps.Network[len(ps.Network)-1].HITLRequired = true
	}
	b.setPermissionSet(ps)
	return b
}

// Build returns a copy of the constructed document.
func (b *DocumentBuilder) Build() *config.Document {
	if b == nil || b.document == nil {
		return nil
	}
	doc := *b.document
	doc.Spec = cloneDocumentSpec(b.document.Spec)
	return &doc
}

// ValidDocument returns a builder for a valid document fixture.
func ValidDocument() *DocumentBuilder {
	return NewDocumentBuilder()
}

// InvalidDocumentMissingMetadata returns a builder for a document missing identity.
func InvalidDocumentMissingMetadata() *DocumentBuilder {
	return &DocumentBuilder{
		document: &config.Document{
			APIVersion: documentAPIVersion,
			Kind:       documentKind,
			Spec:       map[string]yaml.Node{},
		},
	}
}

// InvalidDocumentWrongKind returns a builder for a document with invalid kind.
func InvalidDocumentWrongKind() *DocumentBuilder {
	return &DocumentBuilder{
		document: &config.Document{
			APIVersion: documentAPIVersion,
			Kind:       "NotAgentManifest",
			Metadata:   config.DocumentMetadata{Name: documentName},
			Spec:       map[string]yaml.Node{},
		},
	}
}

func (b *DocumentBuilder) permissionSet() permissions.PermissionSet {
	if b == nil || b.document == nil {
		return permissions.PermissionSet{}
	}
	node, ok := b.document.Section("permissions")
	if !ok {
		return permissions.PermissionSet{}
	}
	var ps permissions.PermissionSet
	_ = node.Decode(&ps)
	return ps
}

func (b *DocumentBuilder) setPermissionSet(ps permissions.PermissionSet) {
	if b == nil || b.document == nil {
		return
	}
	var node yaml.Node
	_ = node.Encode(ps)
	if b.document.Spec == nil {
		b.document.Spec = map[string]yaml.Node{}
	}
	b.document.Spec["permissions"] = node
}

func cloneDocumentSpec(src map[string]yaml.Node) map[string]yaml.Node {
	if len(src) == 0 {
		return map[string]yaml.Node{}
	}
	dst := make(map[string]yaml.Node, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func mustEncodeAgentSpec(spec agentspec.AgentRuntimeSpec) yaml.Node {
	var node yaml.Node
	if err := node.Encode(spec); err != nil {
		panic(err)
	}
	return node
}

// PolicyRuleBuilder builds deterministic policy rule fixtures.
type PolicyRuleBuilder struct {
	rules []policy.PolicyRule
}

// NewPolicyRuleBuilder creates a new policy rule builder.
func NewPolicyRuleBuilder() *PolicyRuleBuilder {
	return &PolicyRuleBuilder{
		rules: make([]policy.PolicyRule, 0),
	}
}

// WithAllowRule adds an allow rule.
func (b *PolicyRuleBuilder) WithAllowRule(id, capability string) *PolicyRuleBuilder {
	b.rules = append(b.rules, policy.PolicyRule{
		ID:       id,
		Name:     id,
		Enabled:  true,
		Priority: 100,
		Conditions: policy.PolicyConditions{
			Capabilities: []string{capability},
		},
		Effect: policy.PolicyEffect{
			Action: "allow",
		},
	})
	return b
}

// WithDenyRule adds a deny rule.
func (b *PolicyRuleBuilder) WithDenyRule(id, capability, reason string) *PolicyRuleBuilder {
	b.rules = append(b.rules, policy.PolicyRule{
		ID:       id,
		Name:     id,
		Enabled:  true,
		Priority: 100,
		Conditions: policy.PolicyConditions{
			Capabilities: []string{capability},
		},
		Effect: policy.PolicyEffect{
			Action: "deny",
			Reason: reason,
		},
	})
	return b
}

// Build returns the constructed policy rules.
func (b *PolicyRuleBuilder) Build() []policy.PolicyRule {
	if b == nil || len(b.rules) == 0 {
		return nil
	}
	rules := make([]policy.PolicyRule, len(b.rules))
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
		env.SetWorkingValueWithClass(item.key, item.value, item.class)
	}
	return env
}

// MinimalEnvelope returns a builder for a minimal envelope fixture.
func MinimalEnvelope() *EnvelopeBuilder {
	return NewEnvelopeBuilder()
}

// AuditRecordBuilder builds deterministic audit record fixtures.
type AuditRecordBuilder struct {
	record policy.AuditRecord
	meta   map[string]any
}

// NewAuditRecordBuilder creates a new audit record builder.
func NewAuditRecordBuilder() *AuditRecordBuilder {
	return &AuditRecordBuilder{
		record: policy.AuditRecord{
			AgentID:     "test-agent",
			Action:      string(policy.AuditActionRequest),
			Type:        string(policy.AuditActionRequest),
			Permission:  "test_permission",
			Result:      "granted",
			Correlation: "test-correlation",
		},
		meta: make(map[string]any),
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
func (b *AuditRecordBuilder) WithMetadata(key string, value any) *AuditRecordBuilder {
	if b.meta == nil {
		b.meta = make(map[string]any)
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
func (b *AuditRecordBuilder) Build() policy.AuditRecord {
	if b == nil {
		return policy.AuditRecord{}
	}
	out := b.record
	if len(b.meta) > 0 {
		out.Metadata = make(map[string]any, len(b.meta))
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
func NormalizeTelemetryEvents(events []telemetry.Event) []telemetry.Event {
	if len(events) == 0 {
		return nil
	}
	normalized := make([]telemetry.Event, len(events))
	for i, event := range events {
		normalized[i] = event
		normalized[i].Timestamp = time.Time{}
		normalized[i].Metadata = cloneInterfaceMap(event.Metadata)
	}
	return normalized
}

// NormalizeAuditRecords normalizes audit records for deterministic comparison.
func NormalizeAuditRecords(records []policy.AuditRecord) []policy.AuditRecord {
	if len(records) == 0 {
		return nil
	}
	normalized := make([]policy.AuditRecord, len(records))
	for i, record := range records {
		normalized[i] = record
		normalized[i].Timestamp = time.Time{}
		normalized[i].Metadata = cloneInterfaceMap(record.Metadata)
	}
	return normalized
}

// NormalizeFileSystemPermissions normalizes filesystem permissions for deterministic comparison.
func NormalizeFileSystemPermissions(perms []permissions.FileSystemPermission) []permissions.FileSystemPermission {
	if len(perms) == 0 {
		return nil
	}
	normalized := make([]permissions.FileSystemPermission, len(perms))
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
func NormalizeNetworkPermissions(perms []permissions.NetworkPermission) []permissions.NetworkPermission {
	if len(perms) == 0 {
		return nil
	}
	normalized := make([]permissions.NetworkPermission, len(perms))
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
func NormalizePolicyRules(rules []policy.PolicyRule) []policy.PolicyRule {
	if len(rules) == 0 {
		return nil
	}
	normalized := make([]policy.PolicyRule, len(rules))
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

func clonePolicyRule(rule policy.PolicyRule) policy.PolicyRule {
	cloned := rule
	cloned.Conditions = policy.PolicyConditions{
		Actors:                    append([]policy.ActorMatch(nil), rule.Conditions.Actors...),
		Capabilities:              append([]string(nil), rule.Conditions.Capabilities...),
		ExportNames:               append([]string(nil), rule.Conditions.ExportNames...),
		SourceDomains:             append([]string(nil), rule.Conditions.SourceDomains...),
		ContextClasses:            append([]string(nil), rule.Conditions.ContextClasses...),
		SensitivityClasses:        append([]string(nil), rule.Conditions.SensitivityClasses...),
		RouteModes:                append([]string(nil), rule.Conditions.RouteModes...),
		ProviderKinds:             append([]string(nil), rule.Conditions.ProviderKinds...),
		ExternalProviders:         append([]string(nil), rule.Conditions.ExternalProviders...),
		MinRiskClasses:            append([]risk.RiskClass(nil), rule.Conditions.MinRiskClasses...),
		TrustClasses:              append([]string(nil), rule.Conditions.TrustClasses...),
		CapabilityKinds:           append([]string(nil), rule.Conditions.CapabilityKinds...),
		RuntimeFamilies:           append([]string(nil), rule.Conditions.RuntimeFamilies...),
		EffectClasses:             append([]classification.EffectClass(nil), rule.Conditions.EffectClasses...),
		Partitions:                append([]string(nil), rule.Conditions.Partitions...),
		ChannelIDs:                append([]string(nil), rule.Conditions.ChannelIDs...),
		SessionScopes:             append([]policy.SessionScope(nil), rule.Conditions.SessionScopes...),
		SessionOperations:         append([]policy.SessionOperation(nil), rule.Conditions.SessionOperations...),
		RequireOwnership:          cloneBoolPtr(rule.Conditions.RequireOwnership),
		RequireDelegation:         cloneBoolPtr(rule.Conditions.RequireDelegation),
		RequireExternalBinding:    cloneBoolPtr(rule.Conditions.RequireExternalBinding),
		RequireResolvedExternal:   cloneBoolPtr(rule.Conditions.RequireResolvedExternal),
		RequireRestrictedExternal: cloneBoolPtr(rule.Conditions.RequireRestrictedExternal),
		TimeWindow:                cloneTimeWindow(rule.Conditions.TimeWindow),
	}
	cloned.Effect = policy.PolicyEffect{
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

func cloneTimeWindow(v *policy.TimeWindow) *policy.TimeWindow {
	if v == nil {
		return nil
	}
	cloned := *v
	cloned.Days = append([]string(nil), v.Days...)
	return &cloned
}

func cloneRateLimit(v *policy.RateLimit) *policy.RateLimit {
	if v == nil {
		return nil
	}
	cloned := *v
	return &cloned
}

func cloneInterfaceMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(values))
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
