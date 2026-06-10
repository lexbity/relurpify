package descriptor

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/schemacoerce"
	"codeburg.org/lexbit/relurpify/governance/classification"
	"codeburg.org/lexbit/relurpify/governance/permissions"
)

type CapabilitySource struct {
	ProviderID string                         `json:"provider_id,omitempty"`
	Scope      classification.CapabilityScope `json:"scope,omitempty"`
	SessionID  string                         `json:"session_id,omitempty"`
}

type AvailabilitySpec struct {
	Available bool              `json:"available"`
	Reason    string            `json:"reason,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type EnabledState int

const (
	EnabledStateUnset EnabledState = iota
	EnabledStateEnabled
	EnabledStateDisabled
)

func (s EnabledState) IsSet() bool {
	return s != EnabledStateUnset
}

func (s EnabledState) IsEnabled() bool {
	return s == EnabledStateEnabled
}

// CoordinationTargetMetadata is the framework-owned metadata block for
// capability-based delegation targets.
type CoordinationTargetMetadata struct {
	Target                 bool                                  `json:"target,omitempty"`
	Role                   agentspec.CoordinationRole            `json:"role,omitempty"`
	TaskTypes              []string                              `json:"task_types,omitempty"`
	ExecutionModes         []agentspec.CoordinationExecutionMode `json:"execution_modes,omitempty"`
	LongRunning            EnabledState                          `json:"long_running,omitempty"`
	MaxDepth               int                                   `json:"max_depth,omitempty"`
	MaxRuntimeSeconds      int                                   `json:"max_runtime_seconds,omitempty"`
	DirectInsertionAllowed EnabledState                          `json:"direct_insertion_allowed,omitempty"`
	ExpectedInput          *schemacoerce.Schema                  `json:"expected_input,omitempty"`
	ExpectedOutput         *schemacoerce.Schema                  `json:"expected_output,omitempty"`
}

type CapabilityDescriptor struct {
	ID              string                            `json:"id"`
	Kind            agentspec.CapabilityKind          `json:"kind"`
	RuntimeFamily   agentspec.CapabilityRuntimeFamily `json:"runtime_family,omitempty"`
	Name            string                            `json:"name"`
	Version         string                            `json:"version,omitempty"`
	Description     string                            `json:"description,omitempty"`
	Category        string                            `json:"category,omitempty"`
	Tags            []string                          `json:"tags,omitempty"`
	Source          CapabilitySource                  `json:"source,omitempty"`
	TrustClass      agentspec.TrustClass              `json:"trust_class,omitempty"`
	EffectClasses   []classification.EffectClass      `json:"effect_classes,omitempty"`
	SessionAffinity string                            `json:"session_affinity,omitempty"`
	InputSchema     *schemacoerce.Schema              `json:"input_schema,omitempty"`
	OutputSchema    *schemacoerce.Schema              `json:"output_schema,omitempty"`
	Availability    AvailabilitySpec                  `json:"availability,omitempty"`
	Coordination    *CoordinationTargetMetadata       `json:"coordination,omitempty"`
	Annotations     map[string]any                    `json:"annotations,omitempty"`
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
	TrustClass() agentspec.TrustClass
}

type CapabilityEffectProvider interface {
	EffectClasses() []classification.EffectClass
}

type SessionAffinityProvider interface {
	SessionAffinity() string
}

type CapabilityRuntimeFamilyAware interface {
	CapabilityRuntimeFamily() agentspec.CapabilityRuntimeFamily
}

type CoordinationMetadataProvider interface {
	CoordinationTargetMetadata() *CoordinationTargetMetadata
}

// ToolDescriptor derives a framework-owned capability descriptor from a tool.
func ToolDescriptor(ctx context.Context, tool ports.Tool) CapabilityDescriptor {
	if tool == nil {
		return CapabilityDescriptor{}
	}
	if provider, ok := tool.(CapabilityDescriptorProvider); ok {
		desc := provider.CapabilityDescriptor()
		if desc.ID == "" {
			desc.ID = ToolCapabilityID(tool)
		}
		if desc.Kind == "" {
			desc.Kind = agentspec.CapabilityKindTool
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
		if !desc.Availability.Available && desc.Availability.Reason == "" && tool.IsAvailable(ctx) {
			desc.Availability.Available = true
		}
		if desc.TrustClass == "" {
			desc.TrustClass = ToolTrustClass(tool)
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
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: ToolCapabilityRuntimeFamily(tool),
		Name:          tool.Name(),
		Description:   tool.Description(),
		Category:      tool.Category(),
		Tags:          ToolCapabilityTags(tool),
		Version:       ToolVersion(tool),
		Source:        ToolCapabilitySource(tool),
		TrustClass:    ToolTrustClass(tool),
		EffectClasses: ToolEffectClasses(tool),
		InputSchema:   ToolInputSchema(tool),
		Availability: AvailabilitySpec{
			Available: tool.IsAvailable(ctx),
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

func ToolCapabilityID(tool ports.Tool) string {
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

func ToolVersion(tool ports.Tool) string {
	if tool == nil {
		return ""
	}
	if provider, ok := tool.(CapabilityVersionProvider); ok {
		return strings.TrimSpace(provider.CapabilityVersion())
	}
	return ""
}

func ToolCapabilitySource(tool ports.Tool) CapabilitySource {
	if tool == nil {
		return CapabilitySource{Scope: classification.CapabilityScopeBuiltin}
	}
	if provider, ok := tool.(CapabilitySourceProvider); ok {
		source := provider.CapabilitySource()
		if source.Scope == "" {
			source.Scope = classification.CapabilityScopeBuiltin
		}
		return source
	}
	return CapabilitySource{Scope: classification.CapabilityScopeBuiltin}
}

func ToolCapabilityRuntimeFamily(tool ports.Tool) agentspec.CapabilityRuntimeFamily {
	if tool == nil {
		return agentspec.CapabilityRuntimeFamilyLocalTool
	}
	if provider, ok := tool.(CapabilityRuntimeFamilyAware); ok {
		if family := provider.CapabilityRuntimeFamily(); family != "" {
			return family
		}
	}
	source := ToolCapabilitySource(tool)
	switch source.Scope {
	case classification.CapabilityScopeProvider, classification.CapabilityScopeRemote:
		return agentspec.CapabilityRuntimeFamilyProvider
	default:
		return agentspec.CapabilityRuntimeFamilyLocalTool
	}
}

func ToolTrustClass(tool ports.Tool) agentspec.TrustClass {
	if tool == nil {
		return agentspec.TrustClassBuiltinTrusted
	}
	if provider, ok := tool.(CapabilityTrustProvider); ok {
		if trust := provider.TrustClass(); trust != "" {
			return trust
		}
	}
	switch ToolCapabilitySource(tool).Scope {
	case classification.CapabilityScopeWorkspace:
		return agentspec.TrustClassWorkspaceTrusted
	case classification.CapabilityScopeProvider:
		return agentspec.TrustClassProviderLocalUntrusted
	case classification.CapabilityScopeRemote:
		return agentspec.TrustClassRemoteDeclared
	default:
		return agentspec.TrustClassBuiltinTrusted
	}
}

func ToolCapabilityTags(tool ports.Tool) []string {
	if tool == nil {
		return nil
	}
	return normalizeCapabilityTags(tool.Tags())
}

func ToolEffectClasses(tool ports.Tool) []classification.EffectClass {
	if tool == nil {
		return nil
	}
	if provider, ok := tool.(CapabilityEffectProvider); ok {
		return normalizeEffectClasses(provider.EffectClasses())
	}
	set := make(map[classification.EffectClass]struct{})
	perms := tool.Permissions().Permissions
	if perms != nil {
		for _, fs := range perms.FileSystem {
			if fs.Action == permissions.FileSystemWrite || fs.Action == permissions.FileSystemExecute {
				set[classification.EffectClassFilesystemMutation] = struct{}{}
				break
			}
		}
		if len(perms.Executables) > 0 || len(perms.Capabilities) > 0 || len(perms.IPC) > 0 {
			set[classification.EffectClassProcessSpawn] = struct{}{}
		}
		if len(perms.Network) > 0 {
			set[classification.EffectClassNetworkEgress] = struct{}{}
			set[classification.EffectClassExternalState] = struct{}{}
		}
	}
	if _, ok := tool.(SessionAffinityProvider); ok {
		set[classification.EffectClassSessionCreation] = struct{}{}
	}
	return effectClassSetToSlice(set)
}

func ToolInputSchema(tool ports.Tool) *schemacoerce.Schema {
	if tool == nil {
		return nil
	}
	params := tool.Parameters()
	properties := make(map[string]*schemacoerce.Schema, len(params))
	required := make([]string, 0, len(params))
	for _, param := range params {
		schema := &schemacoerce.Schema{
			Type:        strings.TrimSpace(string(param.Type)),
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
	return &schemacoerce.Schema{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}
}

func normalizeCapabilityDescriptor(desc CapabilityDescriptor) CapabilityDescriptor {
	if desc.Kind == "" {
		desc.Kind = agentspec.CapabilityKindTool
	}
	if desc.RuntimeFamily == "" {
		desc.RuntimeFamily = defaultCapabilityRuntimeFamily(desc)
	}
	if desc.Source.Scope == "" {
		desc.Source.Scope = classification.CapabilityScopeBuiltin
	}
	desc.Tags = normalizeCapabilityTags(desc.Tags)
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
	case agentspec.CoordinationRolePlanner,
		agentspec.CoordinationRoleArchitect,
		agentspec.CoordinationRoleReviewer,
		agentspec.CoordinationRoleVerifier,
		agentspec.CoordinationRoleExecutor,
		agentspec.CoordinationRoleDomainPack,
		agentspec.CoordinationRoleBackgroundAgent:
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
		case agentspec.CoordinationExecutionModeSync, agentspec.CoordinationExecutionModeSessionBacked, agentspec.CoordinationExecutionModeBackgroundAgent:
		default:
			return fmt.Errorf("coordination execution mode %s invalid", mode)
		}
	}
	switch metadata.LongRunning {
	case EnabledStateUnset, EnabledStateEnabled, EnabledStateDisabled:
	default:
		return fmt.Errorf("coordination long_running state %d invalid", metadata.LongRunning)
	}
	switch metadata.DirectInsertionAllowed {
	case EnabledStateUnset, EnabledStateEnabled, EnabledStateDisabled:
	default:
		return fmt.Errorf("coordination direct_insertion state %d invalid", metadata.DirectInsertionAllowed)
	}
	if metadata.MaxDepth < 0 {
		return fmt.Errorf("coordination max_depth cannot be negative")
	}
	if metadata.MaxRuntimeSeconds < 0 {
		return fmt.Errorf("coordination max_runtime_seconds cannot be negative")
	}
	if metadata.LongRunning == EnabledStateEnabled && !containsCoordinationExecutionMode(metadata.ExecutionModes, agentspec.CoordinationExecutionModeBackgroundAgent) && !containsCoordinationExecutionMode(metadata.ExecutionModes, agentspec.CoordinationExecutionModeSessionBacked) {
		return fmt.Errorf("long-running coordination targets must be session-backed or background-service")
	}
	if metadata.Role == agentspec.CoordinationRoleBackgroundAgent && !containsCoordinationExecutionMode(metadata.ExecutionModes, agentspec.CoordinationExecutionModeBackgroundAgent) {
		return fmt.Errorf("background-agent role requires background-service execution mode")
	}
	return nil
}

func defaultCapabilityRuntimeFamily(desc CapabilityDescriptor) agentspec.CapabilityRuntimeFamily {
	switch desc.Kind {
	case agentspec.CapabilityKindPrompt, agentspec.CapabilityKindResource, agentspec.CapabilityKindSession, agentspec.CapabilityKindSubscription:
		if desc.Source.ProviderID != "" || desc.Source.Scope == classification.CapabilityScopeProvider || desc.Source.Scope == classification.CapabilityScopeRemote {
			return agentspec.CapabilityRuntimeFamilyProvider
		}
		return agentspec.CapabilityRuntimeFamilyRelurpic
	case agentspec.CapabilityKindTool:
		if desc.Source.Scope == classification.CapabilityScopeProvider || desc.Source.Scope == classification.CapabilityScopeRemote || desc.Source.ProviderID != "" {
			return agentspec.CapabilityRuntimeFamilyProvider
		}
		return agentspec.CapabilityRuntimeFamilyLocalTool
	default:
		if desc.Source.Scope == classification.CapabilityScopeProvider || desc.Source.Scope == classification.CapabilityScopeRemote || desc.Source.ProviderID != "" {
			return agentspec.CapabilityRuntimeFamilyProvider
		}
		return agentspec.CapabilityRuntimeFamilyRelurpic
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

func normalizeCoordinationTargetMetadata(metadata *CoordinationTargetMetadata, defaultInput, defaultOutput *schemacoerce.Schema) *CoordinationTargetMetadata {
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
	if clone.Role == agentspec.CoordinationRoleBackgroundAgent {
		clone.LongRunning = EnabledStateEnabled
		if !containsCoordinationExecutionMode(clone.ExecutionModes, agentspec.CoordinationExecutionModeBackgroundAgent) {
			clone.ExecutionModes = append(clone.ExecutionModes, agentspec.CoordinationExecutionModeBackgroundAgent)
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

func normalizeCoordinationExecutionModes(values []agentspec.CoordinationExecutionMode) []agentspec.CoordinationExecutionMode {
	if len(values) == 0 {
		return nil
	}
	set := make(map[agentspec.CoordinationExecutionMode]struct{}, len(values))
	for _, value := range values {
		value = agentspec.CoordinationExecutionMode(strings.TrimSpace(string(value)))
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]agentspec.CoordinationExecutionMode, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func containsCoordinationExecutionMode(values []agentspec.CoordinationExecutionMode, want agentspec.CoordinationExecutionMode) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func cloneSchema(schema *schemacoerce.Schema) *schemacoerce.Schema {
	if schema == nil {
		return nil
	}
	clone := *schema
	if schema.Items != nil {
		clone.Items = cloneSchema(schema.Items)
	}
	if schema.Properties != nil {
		clone.Properties = make(map[string]*schemacoerce.Schema, len(schema.Properties))
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
	case "read-only", "execute", "destructive", "network",
		string(agentspec.TrustClassBuiltinTrusted),
		string(agentspec.TrustClassWorkspaceTrusted),
		string(agentspec.TrustClassProviderLocalUntrusted),
		string(agentspec.TrustClassRemoteDeclared),
		string(agentspec.TrustClassRemoteApproved),
		string(classification.EffectClassFilesystemMutation),
		string(classification.EffectClassProcessSpawn),
		string(classification.EffectClassNetworkEgress),
		string(classification.EffectClassCredentialUse),
		string(classification.EffectClassExternalState),
		string(classification.EffectClassSessionCreation),
		string(classification.EffectClassContextInsertion):
		return true
	default:
		return false
	}
}

func normalizeEffectClasses(classes []classification.EffectClass) []classification.EffectClass {
	if len(classes) == 0 {
		return nil
	}
	set := make(map[classification.EffectClass]struct{}, len(classes))
	for _, class := range classes {
		class = classification.EffectClass(strings.TrimSpace(string(class)))
		if class == "" {
			continue
		}
		set[class] = struct{}{}
	}
	return effectClassSetToSlice(set)
}

func effectClassSetToSlice(set map[classification.EffectClass]struct{}) []classification.EffectClass {
	if len(set) == 0 {
		return nil
	}
	out := make([]classification.EffectClass, 0, len(set))
	for class := range set {
		out = append(out, class)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
