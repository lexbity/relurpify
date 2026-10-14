package descriptor

import (
	"context"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/governance/classification"
	"codeburg.org/lexbit/relurpify/userconfig/permissions"
)

// bareTool implements only ports.Tool. Every descriptor helper must fall back
// to sensible defaults for it, since none of the optional metadata interfaces
// are satisfied.
type bareTool struct {
	name        string
	description string
	category    string
	params      []ports.ToolParameter
	available   bool
	perms       ports.ToolPermissions
	tags        []string
}

func (t *bareTool) Name() string        { return t.name }
func (t *bareTool) Description() string { return t.description }
func (t *bareTool) Category() string    { return t.category }
func (t *bareTool) Parameters() []ports.ToolParameter {
	return t.params
}
func (t *bareTool) IsAvailable(_ context.Context) bool { return t.available }
func (t *bareTool) Permissions() ports.ToolPermissions { return t.perms }
func (t *bareTool) Tags() []string                     { return t.tags }
func (t *bareTool) Execute(_ context.Context, _ map[string]any) (*ports.ToolResult, error) {
	return &ports.ToolResult{Success: true}, nil
}

// stubTool additionally satisfies every optional descriptor provider interface
// defined in this package, with configurable return values.
type stubTool struct {
	bareTool
	capabilityID  string
	version       string
	source        CapabilitySource
	runtimeFamily agentspec.CapabilityRuntimeFamily
	trust         agentspec.TrustClass
	effects       []classification.EffectClass
	affinity      string
	coordination  *CoordinationTargetMetadata
}

func (t *stubTool) CapabilityID() string               { return t.capabilityID }
func (t *stubTool) CapabilityVersion() string          { return t.version }
func (t *stubTool) CapabilitySource() CapabilitySource { return t.source }
func (t *stubTool) CapabilityRuntimeFamily() agentspec.CapabilityRuntimeFamily {
	return t.runtimeFamily
}
func (t *stubTool) TrustClass() agentspec.TrustClass            { return t.trust }
func (t *stubTool) EffectClasses() []classification.EffectClass { return t.effects }
func (t *stubTool) SessionAffinity() string                     { return t.affinity }
func (t *stubTool) CoordinationTargetMetadata() *CoordinationTargetMetadata {
	return t.coordination
}

// permissionedTool is the fixture for permission-derived effect classes: it
// intentionally does NOT satisfy CapabilityEffectProvider or
// SessionAffinityProvider, so ToolEffectClasses must derive from permissions.
type permissionedTool struct {
	bareTool
}

// affinityOnlyTool satisfies SessionAffinityProvider without any other
// optional interface, isolating the session-creation effect-class derivation.
type affinityOnlyTool struct {
	bareTool
	affinity string
}

func (t *affinityOnlyTool) SessionAffinity() string { return t.affinity }

// descriptorProviderTool satisfies CapabilityDescriptorProvider and is routed
// through the provider branch of ToolDescriptor.
type descriptorProviderTool struct {
	stubTool
	desc CapabilityDescriptor
}

func (t *descriptorProviderTool) CapabilityDescriptor() CapabilityDescriptor { return t.desc }

// Compile-time checks that the fixtures satisfy the interfaces they claim.
var (
	_ ports.Tool                   = (*bareTool)(nil)
	_ ports.Tool                   = (*stubTool)(nil)
	_ CapabilityIdentityProvider   = (*stubTool)(nil)
	_ CapabilityVersionProvider    = (*stubTool)(nil)
	_ CapabilitySourceProvider     = (*stubTool)(nil)
	_ CapabilityRuntimeFamilyAware = (*stubTool)(nil)
	_ CapabilityTrustProvider      = (*stubTool)(nil)
	_ CapabilityEffectProvider     = (*stubTool)(nil)
	_ SessionAffinityProvider      = (*stubTool)(nil)
	_ CoordinationMetadataProvider = (*stubTool)(nil)
	_ CapabilityDescriptorProvider = (*descriptorProviderTool)(nil)
	_ ports.Tool                   = (*descriptorProviderTool)(nil)
)

// writePerm is a convenience fixture builder for a permission set that grants
// a filesystem write/exec plus optional extras.
func permSet(fsWrite bool, executables, network int) ports.ToolPermissions {
	set := &permissions.PermissionSet{}
	if fsWrite {
		set.FileSystem = append(set.FileSystem, permissions.FileSystemPermission{
			Action: permissions.FileSystemWrite,
			Path:   "/workspace/**",
		})
	}
	for range executables {
		set.Executables = append(set.Executables, permissions.ExecutablePermission{Binary: "go"})
	}
	for range network {
		set.Network = append(set.Network, permissions.NetworkPermission{
			Direction: "egress",
			Protocol:  "https",
			Host:      "example.com",
		})
	}
	return ports.ToolPermissions{Permissions: set}
}
