package core

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type CapabilityKind string

const (
	CapabilityKindTool         CapabilityKind = "tool"
	CapabilityKindPrompt       CapabilityKind = "prompt"
	CapabilityKindResource     CapabilityKind = "resource"
	CapabilityKindSession      CapabilityKind = "session"
	CapabilityKindSubscription CapabilityKind = "subscription"
)

type CapabilityScope string

const (
	CapabilityScopeBuiltin   CapabilityScope = "builtin"
	CapabilityScopeWorkspace CapabilityScope = "workspace"
	CapabilityScopeProvider  CapabilityScope = "provider"
	CapabilityScopeRemote    CapabilityScope = "remote"
)

type CapabilityRuntimeFamily string

const (
	// CapabilityRuntimeFamilyLocalTool identifies local callable tool execution.
	CapabilityRuntimeFamilyLocalTool CapabilityRuntimeFamily = "local-tool"
	// CapabilityRuntimeFamilyProvider identifies provider-backed capability execution.
	CapabilityRuntimeFamilyProvider  CapabilityRuntimeFamily = "provider"
	// CapabilityRuntimeFamilyRelurpic identifies higher-order execution behavior
	// composed from capabilities, skills, sub-agents, or multiple execution
	// paradigms. Relurpic is a runtime-family classification inside the canonical
	// capability model, not a separate capability system.
	CapabilityRuntimeFamilyRelurpic  CapabilityRuntimeFamily = "relurpic"
)

type TrustClass string

const (
	TrustClassBuiltinTrusted         TrustClass = "builtin-trusted"
	TrustClassWorkspaceTrusted       TrustClass = "workspace-trusted"
	TrustClassProviderLocalUntrusted TrustClass = "provider-local-untrusted"
	TrustClassRemoteDeclared         TrustClass = "remote-declared-untrusted"
	TrustClassRemoteApproved         TrustClass = "remote-approved"
)

type RiskClass string

const (
	RiskClassReadOnly     RiskClass = "read-only"
	RiskClassDestructive  RiskClass = "destructive"
	RiskClassExecute      RiskClass = "execute"
	RiskClassNetwork      RiskClass = "network"
	RiskClassCredentialed RiskClass = "credentialed"
	RiskClassExfiltration RiskClass = "exfiltration-sensitive"
	RiskClassSessioned    RiskClass = "sessioned"
)

type EffectClass string

const (
	EffectClassFilesystemMutation EffectClass = "filesystem-mutation"
	EffectClassProcessSpawn       EffectClass = "process-spawn"
	EffectClassNetworkEgress      EffectClass = "network-egress"
	EffectClassCredentialUse      EffectClass = "credential-use"
	EffectClassExternalState      EffectClass = "external-state-change"
	EffectClassSessionCreation    EffectClass = "long-lived-session-creation"
	EffectClassContextInsertion   EffectClass = "model-context-insertion"
)

type CapabilitySource struct {
	ProviderID string          `json:"provider_id,omitempty" yaml:"provider_id,omitempty"`
	Scope      CapabilityScope `json:"scope,omitempty" yaml:"scope,omitempty"`
	SessionID  string          `json:"session_id,omitempty" yaml:"session_id,omitempty"`
}

type AvailabilitySpec struct {
	Available bool              `json:"available" yaml:"available"`
	Reason    string            `json:"reason,omitempty" yaml:"reason,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

type Schema struct {
	Type        string             `json:"type,omitempty" yaml:"type,omitempty"`
	Properties  map[string]*Schema `json:"properties,omitempty" yaml:"properties,omitempty"`
	Items       *Schema            `json:"items,omitempty" yaml:"items,omitempty"`
	Required    []string           `json:"required,omitempty" yaml:"required,omitempty"`
	Default     any                `json:"default,omitempty" yaml:"default,omitempty"`
	Enum        []any              `json:"enum,omitempty" yaml:"enum,omitempty"`
	Title       string             `json:"title,omitempty" yaml:"title,omitempty"`
	Description string             `json:"description,omitempty" yaml:"description,omitempty"`
	Format      string             `json:"format,omitempty" yaml:"format,omitempty"`
}

type CoordinationRole string

const (
	CoordinationRolePlanner         CoordinationRole = "planner"
	CoordinationRoleArchitect       CoordinationRole = "architect"
	CoordinationRoleReviewer        CoordinationRole = "reviewer"
	CoordinationRoleVerifier        CoordinationRole = "verifier"
	CoordinationRoleExecutor        CoordinationRole = "executor"
	CoordinationRoleDomainPack      CoordinationRole = "domain-pack"
	CoordinationRoleBackgroundAgent CoordinationRole = "background-agent"
)

type CoordinationExecutionMode string

const (
	CoordinationExecutionModeSync            CoordinationExecutionMode = "sync"
	CoordinationExecutionModeSessionBacked   CoordinationExecutionMode = "session-backed"
	CoordinationExecutionModeBackgroundAgent CoordinationExecutionMode = "background-service"
)

// CoordinationTargetMetadata is the framework-owned metadata block for
// capability-based delegation targets.
type CoordinationTargetMetadata struct {
	Target                 bool                        `json:"target,omitempty" yaml:"target,omitempty"`
	Role                   CoordinationRole            `json:"role,omitempty" yaml:"role,omitempty"`
	TaskTypes              []string                    `json:"task_types,omitempty" yaml:"task_types,omitempty"`
	ExecutionModes         []CoordinationExecutionMode `json:"execution_modes,omitempty" yaml:"execution_modes,omitempty"`
	LongRunning            bool                        `json:"long_running,omitempty" yaml:"long_running,omitempty"`
	MaxDepth               int                         `json:"max_depth,omitempty" yaml:"max_depth,omitempty"`
	MaxRuntimeSeconds      int                         `json:"max_runtime_seconds,omitempty" yaml:"max_runtime_seconds,omitempty"`
	DirectInsertionAllowed bool                        `json:"direct_insertion_allowed,omitempty" yaml:"direct_insertion_allowed,omitempty"`
	ExpectedInput          *Schema                     `json:"expected_input,omitempty" yaml:"expected_input,omitempty"`
	ExpectedOutput         *Schema                     `json:"expected_output,omitempty" yaml:"expected_output,omitempty"`
}

type CapabilityDescriptor struct {
	ID              string                      `json:"id" yaml:"id"`
	Kind            CapabilityKind              `json:"kind" yaml:"kind"`
	RuntimeFamily   CapabilityRuntimeFamily     `json:"runtime_family,omitempty" yaml:"runtime_family,omitempty"`
	Name            string                      `json:"name" yaml:"name"`
	Version         string                      `json:"version,omitempty" yaml:"version,omitempty"`
	Description     string                      `json:"description,omitempty" yaml:"description,omitempty"`
	Category        string                      `json:"category,omitempty" yaml:"category,omitempty"`
	Tags            []string                    `json:"tags,omitempty" yaml:"tags,omitempty"`
	Source          CapabilitySource            `json:"source,omitempty" yaml:"source,omitempty"`
	TrustClass      TrustClass                  `json:"trust_class,omitempty" yaml:"trust_class,omitempty"`
	RiskClasses     []RiskClass                 `json:"risk_classes,omitempty" yaml:"risk_classes,omitempty"`
	EffectClasses   []EffectClass               `json:"effect_classes,omitempty" yaml:"effect_classes,omitempty"`
	SessionAffinity string                      `json:"session_affinity,omitempty" yaml:"session_affinity,omitempty"`
	InputSchema     *Schema                     `json:"input_schema,omitempty" yaml:"input_schema,omitempty"`
	OutputSchema    *Schema                     `json:"output_schema,omitempty" yaml:"output_schema,omitempty"`
	Availability    AvailabilitySpec            `json:"availability,omitempty" yaml:"availability,omitempty"`
	Coordination    *CoordinationTargetMetadata `json:"coordination,omitempty" yaml:"coordination,omitempty"`
	Annotations     map[string]any              `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

type ContentBlock interface {
	ContentType() string
}

type TextContentBlock struct {
	Text       string            `json:"text" yaml:"text"`
	Provenance ContentProvenance `json:"provenance,omitempty" yaml:"provenance,omitempty"`
}

func (TextContentBlock) ContentType() string { return "text" }

type StructuredContentBlock struct {
	Data       any               `json:"data" yaml:"data"`
	Provenance ContentProvenance `json:"provenance,omitempty" yaml:"provenance,omitempty"`
}

func (StructuredContentBlock) ContentType() string { return "structured" }

type ResourceLinkContentBlock struct {
	URI        string            `json:"uri" yaml:"uri"`
	Name       string            `json:"name,omitempty" yaml:"name,omitempty"`
	MIMEType   string            `json:"mime_type,omitempty" yaml:"mime_type,omitempty"`
	Provenance ContentProvenance `json:"provenance,omitempty" yaml:"provenance,omitempty"`
}

func (ResourceLinkContentBlock) ContentType() string { return "resource-link" }

type EmbeddedResourceContentBlock struct {
	Resource   any               `json:"resource" yaml:"resource"`
	Provenance ContentProvenance `json:"provenance,omitempty" yaml:"provenance,omitempty"`
}

func (EmbeddedResourceContentBlock) ContentType() string { return "embedded-resource" }

type BinaryReferenceContentBlock struct {
	Ref        string            `json:"ref" yaml:"ref"`
	MIMEType   string            `json:"mime_type,omitempty" yaml:"mime_type,omitempty"`
	Provenance ContentProvenance `json:"provenance,omitempty" yaml:"provenance,omitempty"`
}

func (BinaryReferenceContentBlock) ContentType() string { return "binary-reference" }

type ErrorContentBlock struct {
	Code       string            `json:"code,omitempty" yaml:"code,omitempty"`
	Message    string            `json:"message" yaml:"message"`
	Provenance ContentProvenance `json:"provenance,omitempty" yaml:"provenance,omitempty"`
}

func (ErrorContentBlock) ContentType() string { return "error" }

// AnchorRef represents a semantic anchor reference for provenance tracking.
type AnchorRef struct {
	AnchorID   string `json:"anchor_id"`
	Term       string `json:"term"`
	Definition string `json:"definition"`
	Class      string `json:"class"`
	Active     bool   `json:"active"`
	CreatedAt  string `json:"created_at"`
}

type ContentProvenance struct {
	CapabilityID string             `json:"capability_id,omitempty" yaml:"capability_id,omitempty"`
	ProviderID   string             `json:"provider_id,omitempty" yaml:"provider_id,omitempty"`
	TrustClass   TrustClass         `json:"trust_class,omitempty" yaml:"trust_class,omitempty"`
	Disposition  ContentDisposition `json:"disposition,omitempty" yaml:"disposition,omitempty"`
	Derivation   *DerivationChain   `json:"derivation,omitempty" yaml:"derivation,omitempty"`        // NEW: transformation history
	Anchors      []AnchorRef        `json:"anchors,omitempty" yaml:"anchors,omitempty"`              // NEW: semantic anchors in result
}

type CapabilityDescriptorProvider interface {
	CapabilityDescriptor() CapabilityDescriptor
}

type CapabilityIdentityProvider interface {
	CapabilityID() string
}

type CapabilitySourceProvider interface {
	CapabilitySource() CapabilitySource
}

type CapabilityVersionProvider interface {
	CapabilityVersion() string
}

type CapabilityTrustProvider interface {
	TrustClass() TrustClass
}

type CapabilityRiskProvider interface {
	RiskClasses() []RiskClass
}

type CapabilityEffectProvider interface {
	EffectClasses() []EffectClass
}

type SessionAffinityProvider interface {
	SessionAffinity() string
}

type CapabilityRuntimeFamilyAware interface {
	CapabilityRuntimeFamily() CapabilityRuntimeFamily
}

type CoordinationMetadataProvider interface {
	CoordinationTargetMetadata() *CoordinationTargetMetadata
}

// ToolDescriptor derives a framework-owned capability descriptor from a tool.
func ToolDescriptor(ctx context.Context, state *Context, tool Tool) CapabilityDescriptor {
	if tool == nil {
		return CapabilityDescriptor{}
	}
	if provider, ok := tool.(CapabilityDescriptorProvider); ok {
		desc := provider.CapabilityDescriptor()
		if desc.ID == "" {
			desc.ID = ToolCapabilityID(tool)
		}
		if desc.Kind == "" {
			desc.Kind = CapabilityKindTool
		}
		if desc.Name == "" {
			desc.Name = tool.Name()
		}
		if desc.RuntimeFamily == "" {
			desc.RuntimeFamily = ToolCapabilityRuntimeFamily(tool)
		}
		if desc.Description == "" {
			desc.Description = tool.Description()
		}
		if desc.Category == "" {
			desc.Category = tool.Category()
		}
		if desc.InputSchema == nil {
			desc.InputSchema = ToolInputSchema(tool)
		}
		if desc.Availability.Available == false && desc.Availability.Reason == "" && tool.IsAvailable(ctx, state) {
			desc.Availability.Available = true
		}
		if desc.TrustClass == "" {
			desc.TrustClass = ToolTrustClass(tool)
		}
		if len(desc.RiskClasses) == 0 {
			desc.RiskClasses = ToolRiskClasses(tool)
		}
		if len(desc.EffectClasses) == 0 {
			desc.EffectClasses = ToolEffectClasses(tool)
		}
		if desc.Source.Scope == "" {
			desc.Source = ToolCapabilitySource(tool)
		}
		if len(desc.Tags) == 0 {
			desc.Tags = ToolCapabilityTags(tool)
		}
		if desc.Coordination == nil {
			if provider, ok := tool.(CoordinationMetadataProvider); ok {
				desc.Coordination = provider.CoordinationTargetMetadata()
			}
		}
		return normalizeCapabilityDescriptor(desc)
	}
	desc := CapabilityDescriptor{
		ID:            ToolCapabilityID(tool),
		Kind:          CapabilityKindTool,
		RuntimeFamily: ToolCapabilityRuntimeFamily(tool),
		Name:          tool.Name(),
		Description:   tool.Description(),
		Category:      tool.Category(),
		Tags:          ToolCapabilityTags(tool),
		Version:       ToolVersion(tool),
		Source:        ToolCapabilitySource(tool),
		TrustClass:    ToolTrustClass(tool),
		RiskClasses:   ToolRiskClasses(tool),
		EffectClasses: ToolEffectClasses(tool),
		InputSchema:   ToolInputSchema(tool),
		Availability: AvailabilitySpec{
			Available: tool.IsAvailable(ctx, state),
		},
	}
	if provider, ok := tool.(SessionAffinityProvider); ok {
		desc.SessionAffinity = provider.SessionAffinity()
	}
	if provider, ok := tool.(CoordinationMetadataProvider); ok {
		desc.Coordination = provider.CoordinationTargetMetadata()
	}
	return normalizeCapabilityDescriptor(desc)
}

// NormalizeCapabilityDescriptor applies the same descriptor cleanup used for tools
// so non-tool capabilities can be registered consistently.
func NormalizeCapabilityDescriptor(desc CapabilityDescriptor) CapabilityDescriptor {
	return normalizeCapabilityDescriptor(desc)
}

func ToolCapabilityID(tool Tool) string {
	if tool == nil {
		return ""
	}
	if provider, ok := tool.(CapabilityIdentityProvider); ok {
		if id := strings.TrimSpace(provider.CapabilityID()); id != "" {
			return id
		}
	}
	return fmt.Sprintf("tool:%s", strings.TrimSpace(tool.Name()))
}

func ToolVersion(tool Tool) string {
	if tool == nil {
		return ""
	}
	if provider, ok := tool.(CapabilityVersionProvider); ok {
		return strings.TrimSpace(provider.CapabilityVersion())
	}
	return ""
}

func ToolCapabilitySource(tool Tool) CapabilitySource {
	if tool == nil {
		return CapabilitySource{Scope: CapabilityScopeBuiltin}
	}
	if provider, ok := tool.(CapabilitySourceProvider); ok {
		source := provider.CapabilitySource()
		if source.Scope == "" {
			source.Scope = CapabilityScopeBuiltin
		}
		return source
	}
	return CapabilitySource{Scope: CapabilityScopeBuiltin}
}

func ToolCapabilityRuntimeFamily(tool Tool) CapabilityRuntimeFamily {
	if tool == nil {
		return CapabilityRuntimeFamilyLocalTool
	}
	if provider, ok := tool.(CapabilityRuntimeFamilyAware); ok {
		if family := provider.CapabilityRuntimeFamily(); family != "" {
			return family
		}
	}
	source := ToolCapabilitySource(tool)
	switch source.Scope {
	case CapabilityScopeProvider, CapabilityScopeRemote:
		return CapabilityRuntimeFamilyProvider
	default:
		return CapabilityRuntimeFamilyLocalTool
	}
}

func ToolTrustClass(tool Tool) TrustClass {
	if tool == nil {
		return TrustClassBuiltinTrusted
	}
	if provider, ok := tool.(CapabilityTrustProvider); ok {
		if trust := provider.TrustClass(); trust != "" {
			return trust
		}
	}
	switch ToolCapabilitySource(tool).Scope {
	case CapabilityScopeWorkspace:
		return TrustClassWorkspaceTrusted
	case CapabilityScopeProvider:
		return TrustClassProviderLocalUntrusted
	case CapabilityScopeRemote:
		return TrustClassRemoteDeclared
	default:
		return TrustClassBuiltinTrusted
	}
}

func ToolCapabilityTags(tool Tool) []string {
	if tool == nil {
		return nil
	}
	return normalizeCapabilityTags(tool.Tags())
}

func ToolRiskClasses(tool Tool) []RiskClass {
	if tool == nil {
		return nil
	}
	if provider, ok := tool.(CapabilityRiskProvider); ok {
		return normalizeRiskClasses(provider.RiskClasses())
	}
	set := make(map[RiskClass]struct{})
	for _, tag := range tool.Tags() {
		switch strings.ToLower(strings.TrimSpace(tag)) {
		case string(RiskClassReadOnly):
			set[RiskClassReadOnly] = struct{}{}
		case string(RiskClassDestructive):
			set[RiskClassDestructive] = struct{}{}
		case string(RiskClassExecute):
			set[RiskClassExecute] = struct{}{}
		case string(RiskClassNetwork):
			set[RiskClassNetwork] = struct{}{}
		case string(RiskClassCredentialed):
			set[RiskClassCredentialed] = struct{}{}
		case string(RiskClassExfiltration):
			set[RiskClassExfiltration] = struct{}{}
		case string(RiskClassSessioned):
			set[RiskClassSessioned] = struct{}{}
		}
	}
	perms := tool.Permissions().Permissions
	if perms != nil {
		if len(perms.Executables) > 0 || len(perms.Capabilities) > 0 || len(perms.IPC) > 0 {
			set[RiskClassExecute] = struct{}{}
		}
		if len(perms.Network) > 0 {
			set[RiskClassNetwork] = struct{}{}
			set[RiskClassExfiltration] = struct{}{}
		}
		if hasFilesystemMutation(perms) {
			set[RiskClassDestructive] = struct{}{}
		}
		if len(set) == 0 && hasFilesystemReadOnly(perms) {
			set[RiskClassReadOnly] = struct{}{}
		}
	}
	return riskClassSetToSlice(set)
}

func ToolEffectClasses(tool Tool) []EffectClass {
	if tool == nil {
		return nil
	}
	if provider, ok := tool.(CapabilityEffectProvider); ok {
		return normalizeEffectClasses(provider.EffectClasses())
	}
	set := make(map[EffectClass]struct{})
	perms := tool.Permissions().Permissions
	if perms != nil {
		for _, fs := range perms.FileSystem {
			if fs.Action == FileSystemWrite || fs.Action == FileSystemExecute {
				set[EffectClassFilesystemMutation] = struct{}{}
				break
			}
		}
		if len(perms.Executables) > 0 || len(perms.Capabilities) > 0 || len(perms.IPC) > 0 {
			set[EffectClassProcessSpawn] = struct{}{}
		}
		if len(perms.Network) > 0 {
			set[EffectClassNetworkEgress] = struct{}{}
			set[EffectClassExternalState] = struct{}{}
		}
	}
	if _, ok := tool.(SessionAffinityProvider); ok {
		set[EffectClassSessionCreation] = struct{}{}
	}
	return effectClassSetToSlice(set)
}

func ToolInputSchema(tool Tool) *Schema {
	if tool == nil {
		return nil
	}
	params := tool.Parameters()
	properties := make(map[string]*Schema, len(params))
	required := make([]string, 0, len(params))
	for _, param := range params {
		schema := &Schema{
			Type:        strings.TrimSpace(param.Type),
			Description: strings.TrimSpace(param.Description),
			Default:     param.Default,
		}
		if schema.Type == "" {
			schema.Type = "string"
		}
		properties[param.Name] = schema
		if param.Required {
			required = append(required, param.Name)
		}
	}
	sort.Strings(required)
	return &Schema{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}
}

func normalizeCapabilityDescriptor(desc CapabilityDescriptor) CapabilityDescriptor {
	if desc.Kind == "" {
		desc.Kind = CapabilityKindTool
	}
	if desc.RuntimeFamily == "" {
		desc.RuntimeFamily = defaultCapabilityRuntimeFamily(desc)
	}
	if desc.Source.Scope == "" {
		desc.Source.Scope = CapabilityScopeBuiltin
	}
	desc.Tags = normalizeCapabilityTags(desc.Tags)
	desc.RiskClasses = normalizeRiskClasses(desc.RiskClasses)
	desc.EffectClasses = normalizeEffectClasses(desc.EffectClasses)
	desc.Coordination = normalizeCoordinationTargetMetadata(desc.Coordination, desc.InputSchema, desc.OutputSchema)
	return desc
}

func ValidateCoordinationTargetMetadata(metadata *CoordinationTargetMetadata) error {
	if metadata == nil {
		return nil
	}
	if !metadata.Target {
		return fmt.Errorf("coordination target must be enabled")
	}
	switch metadata.Role {
	case CoordinationRolePlanner,
		CoordinationRoleArchitect,
		CoordinationRoleReviewer,
		CoordinationRoleVerifier,
		CoordinationRoleExecutor,
		CoordinationRoleDomainPack,
		CoordinationRoleBackgroundAgent:
	default:
		return fmt.Errorf("coordination role %s invalid", metadata.Role)
	}
	if len(metadata.TaskTypes) == 0 {
		return fmt.Errorf("coordination task_types required")
	}
	for _, taskType := range metadata.TaskTypes {
		if strings.TrimSpace(taskType) == "" {
			return fmt.Errorf("coordination task_types cannot contain empty values")
		}
	}
	if len(metadata.ExecutionModes) == 0 {
		return fmt.Errorf("coordination execution_modes required")
	}
	for _, mode := range metadata.ExecutionModes {
		switch mode {
		case CoordinationExecutionModeSync, CoordinationExecutionModeSessionBacked, CoordinationExecutionModeBackgroundAgent:
		default:
			return fmt.Errorf("coordination execution mode %s invalid", mode)
		}
	}
	if metadata.MaxDepth < 0 {
		return fmt.Errorf("coordination max_depth cannot be negative")
	}
	if metadata.MaxRuntimeSeconds < 0 {
		return fmt.Errorf("coordination max_runtime_seconds cannot be negative")
	}
	if metadata.LongRunning && !containsCoordinationExecutionMode(metadata.ExecutionModes, CoordinationExecutionModeBackgroundAgent) && !containsCoordinationExecutionMode(metadata.ExecutionModes, CoordinationExecutionModeSessionBacked) {
		return fmt.Errorf("long-running coordination targets must be session-backed or background-service")
	}
	if metadata.Role == CoordinationRoleBackgroundAgent && !containsCoordinationExecutionMode(metadata.ExecutionModes, CoordinationExecutionModeBackgroundAgent) {
		return fmt.Errorf("background-agent role requires background-service execution mode")
	}
	return nil
}

func defaultCapabilityRuntimeFamily(desc CapabilityDescriptor) CapabilityRuntimeFamily {
	switch desc.Kind {
	case CapabilityKindPrompt, CapabilityKindResource, CapabilityKindSession, CapabilityKindSubscription:
		if desc.Source.ProviderID != "" || desc.Source.Scope == CapabilityScopeProvider || desc.Source.Scope == CapabilityScopeRemote {
			return CapabilityRuntimeFamilyProvider
		}
		return CapabilityRuntimeFamilyRelurpic
	case CapabilityKindTool:
		if desc.Source.Scope == CapabilityScopeProvider || desc.Source.Scope == CapabilityScopeRemote || desc.Source.ProviderID != "" {
			return CapabilityRuntimeFamilyProvider
		}
		return CapabilityRuntimeFamilyLocalTool
	default:
		if desc.Source.Scope == CapabilityScopeProvider || desc.Source.Scope == CapabilityScopeRemote || desc.Source.ProviderID != "" {
			return CapabilityRuntimeFamilyProvider
		}
		return CapabilityRuntimeFamilyRelurpic
	}
}

func normalizeCapabilityTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" || isReservedSecurityTag(tag) {
			continue
		}
		set[tag] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for tag := range set {
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

func normalizeCoordinationTargetMetadata(metadata *CoordinationTargetMetadata, defaultInput, defaultOutput *Schema) *CoordinationTargetMetadata {
	if metadata == nil {
		return nil
	}
	clone := *metadata
	clone.TaskTypes = normalizeStringList(metadata.TaskTypes)
	clone.ExecutionModes = normalizeCoordinationExecutionModes(metadata.ExecutionModes)
	if clone.ExpectedInput == nil {
		clone.ExpectedInput = cloneSchema(defaultInput)
	} else {
		clone.ExpectedInput = cloneSchema(clone.ExpectedInput)
	}
	if clone.ExpectedOutput == nil {
		clone.ExpectedOutput = cloneSchema(defaultOutput)
	} else {
		clone.ExpectedOutput = cloneSchema(clone.ExpectedOutput)
	}
	if clone.Role == CoordinationRoleBackgroundAgent {
		clone.LongRunning = true
		if !containsCoordinationExecutionMode(clone.ExecutionModes, CoordinationExecutionModeBackgroundAgent) {
			clone.ExecutionModes = append(clone.ExecutionModes, CoordinationExecutionModeBackgroundAgent)
			clone.ExecutionModes = normalizeCoordinationExecutionModes(clone.ExecutionModes)
		}
	}
	return &clone
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizeCoordinationExecutionModes(values []CoordinationExecutionMode) []CoordinationExecutionMode {
	if len(values) == 0 {
		return nil
	}
	set := make(map[CoordinationExecutionMode]struct{}, len(values))
	for _, value := range values {
		value = CoordinationExecutionMode(strings.TrimSpace(string(value)))
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]CoordinationExecutionMode, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func containsCoordinationExecutionMode(values []CoordinationExecutionMode, want CoordinationExecutionMode) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func cloneSchema(schema *Schema) *Schema {
	if schema == nil {
		return nil
	}
	clone := *schema
	if schema.Items != nil {
		clone.Items = cloneSchema(schema.Items)
	}
	if schema.Properties != nil {
		clone.Properties = make(map[string]*Schema, len(schema.Properties))
		for key, value := range schema.Properties {
			clone.Properties[key] = cloneSchema(value)
		}
	}
	clone.Required = append([]string{}, schema.Required...)
	clone.Enum = append([]any{}, schema.Enum...)
	return &clone
}

func isReservedSecurityTag(tag string) bool {
	switch tag {
	case strings.ToLower(TagReadOnly),
		strings.ToLower(TagExecute),
		strings.ToLower(TagDestructive),
		strings.ToLower(TagNetwork),
		string(TrustClassBuiltinTrusted),
		string(TrustClassWorkspaceTrusted),
		string(TrustClassProviderLocalUntrusted),
		string(TrustClassRemoteDeclared),
		string(TrustClassRemoteApproved),
		string(RiskClassReadOnly),
		string(RiskClassDestructive),
		string(RiskClassExecute),
		string(RiskClassNetwork),
		string(RiskClassCredentialed),
		string(RiskClassExfiltration),
		string(RiskClassSessioned),
		string(EffectClassFilesystemMutation),
		string(EffectClassProcessSpawn),
		string(EffectClassNetworkEgress),
		string(EffectClassCredentialUse),
		string(EffectClassExternalState),
		string(EffectClassSessionCreation),
		string(EffectClassContextInsertion):
		return true
	default:
		return false
	}
}

func normalizeRiskClasses(classes []RiskClass) []RiskClass {
	if len(classes) == 0 {
		return nil
	}
	set := make(map[RiskClass]struct{}, len(classes))
	for _, class := range classes {
		class = RiskClass(strings.TrimSpace(string(class)))
		if class == "" {
			continue
		}
		set[class] = struct{}{}
	}
	return riskClassSetToSlice(set)
}

func normalizeEffectClasses(classes []EffectClass) []EffectClass {
	if len(classes) == 0 {
		return nil
	}
	set := make(map[EffectClass]struct{}, len(classes))
	for _, class := range classes {
		class = EffectClass(strings.TrimSpace(string(class)))
		if class == "" {
			continue
		}
		set[class] = struct{}{}
	}
	return effectClassSetToSlice(set)
}

func riskClassSetToSlice(set map[RiskClass]struct{}) []RiskClass {
	if len(set) == 0 {
		return nil
	}
	out := make([]RiskClass, 0, len(set))
	for class := range set {
		out = append(out, class)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func effectClassSetToSlice(set map[EffectClass]struct{}) []EffectClass {
	if len(set) == 0 {
		return nil
	}
	out := make([]EffectClass, 0, len(set))
	for class := range set {
		out = append(out, class)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func hasFilesystemMutation(perms *PermissionSet) bool {
	if perms == nil {
		return false
	}
	for _, fs := range perms.FileSystem {
		if fs.Action == FileSystemWrite || fs.Action == FileSystemExecute {
			return true
		}
	}
	return false
}

func hasFilesystemReadOnly(perms *PermissionSet) bool {
	if perms == nil || len(perms.FileSystem) == 0 {
		return false
	}
	for _, fs := range perms.FileSystem {
		if fs.Action != FileSystemRead && fs.Action != FileSystemList {
			return false
		}
	}
	return true
}
