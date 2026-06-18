package config

import (
	"fmt"
	"strings"

	configpermissions "codeburg.org/lexbit/relurpify/userconfig/permissions"
)

// EffectiveAgentContract captures the resolved runtime-facing contract derived
// from a manifest or document plus any later overlays.
type EffectiveAgentContract struct {
	AgentID     string
	AgentSpec   *AgentSpec
	Permissions configpermissions.PermissionSet
	Resources   ResourceSpec
	Security    SecuritySpec
	Sources     SourceSummary
}

// SourceSummary records which inputs contributed to the effective contract so
// callers can inspect how a runtime was resolved.
type SourceSummary struct {
	ManifestName     string
	ManifestVersion  string
	Workspace        string
	GlobalDefaults   bool
	OverlayCount     int
	RuntimeOverrides int
}

// ResolveOptions provides optional inputs layered on top of the loaded config.
type ResolveOptions struct {
}

// ResolveEffectiveResources merges the document's resource section into an
// effective runtime resource view.
func ResolveEffectiveResources(_ string, doc *Document) (ResourceSpec, error) {
	if doc == nil {
		return ResourceSpec{}, fmt.Errorf("document required")
	}
	node, ok := doc.Section("resources")
	if !ok {
		return ResourceSpec{}, nil
	}
	resources, err := DecodeResourceSection(node)
	if err != nil {
		return ResourceSpec{}, fmt.Errorf("decode resources: %w", err)
	}
	if resources == nil {
		return ResourceSpec{}, fmt.Errorf("decode resources: nil result")
	}
	return *resources, nil
}

// MergeResourceSpecs overlays non-empty resource fields on top of a base spec.
func MergeResourceSpecs(base ResourceSpec, overlays ...*ResourceSpec) ResourceSpec {
	merged := base
	for _, overlay := range overlays {
		if overlay == nil {
			continue
		}
		if overlay.Limits.CPU != "" {
			merged.Limits.CPU = overlay.Limits.CPU
		}
		if overlay.Limits.Memory != "" {
			merged.Limits.Memory = overlay.Limits.Memory
		}
		if overlay.Limits.DiskIO != "" {
			merged.Limits.DiskIO = overlay.Limits.DiskIO
		}
		if overlay.Limits.Network != "" {
			merged.Limits.Network = overlay.Limits.Network
		}
	}
	return merged
}

// ResolveEffectiveAgentContract resolves a runtime-facing contract directly
// from a loaded document. This is the canonical resolution path.
func ResolveEffectiveAgentContract(workspace string, doc *Document, _ ResolveOptions) (*EffectiveAgentContract, error) {
	if doc == nil {
		return nil, fmt.Errorf("document required")
	}
	if strings.TrimSpace(workspace) == "" {
		return nil, fmt.Errorf("workspace required")
	}
	if strings.TrimSpace(doc.Kind) != "AgentManifest" {
		return nil, fmt.Errorf("unsupported document kind %q", doc.Kind)
	}
	if strings.TrimSpace(doc.Metadata.Name) == "" {
		return nil, fmt.Errorf("document metadata.name required")
	}

	contract, err := assembleEffectiveAgentContract(doc)
	if err != nil {
		return nil, err
	}
	contract.Sources.Workspace = workspace
	return contract, nil
}

func assembleEffectiveAgentContract(doc *Document) (*EffectiveAgentContract, error) {
	agentID := strings.TrimSpace(doc.Metadata.Name)
	if agentID == "" {
		return nil, fmt.Errorf("document metadata.name required")
	}

	perms, err := decodePermissionsSection(doc)
	if err != nil {
		return nil, fmt.Errorf("decode permissions: %w", err)
	}
	resources, err := ResolveEffectiveResources("", doc)
	if err != nil {
		return nil, fmt.Errorf("resolve resources: %w", err)
	}
	security, err := decodeSecuritySection(doc)
	if err != nil {
		return nil, fmt.Errorf("decode security: %w", err)
	}
	agentSpec, err := decodeAgentSection(doc)
	if err != nil {
		return nil, fmt.Errorf("decode agent: %w", err)
	}
	if agentSpec == nil {
		return nil, fmt.Errorf("agent section required")
	}

	return BuildEffectiveAgentContract(agentID, agentSpec, perms, resources, security, SourceSummary{
		ManifestName:    doc.Metadata.Name,
		ManifestVersion: doc.Metadata.Version,
	}), nil
}

func decodePermissionsSection(doc *Document) (configpermissions.PermissionSet, error) {
	node, ok := doc.Section("permissions")
	if !ok {
		return configpermissions.PermissionSet{}, nil
	}
	ps, err := DecodePermissionsSection(node)
	if err != nil || ps == nil {
		return configpermissions.PermissionSet{}, err
	}
	return *ps, nil
}

func decodeAgentSection(doc *Document) (*AgentSpec, error) {
	node, ok := doc.Section("agent")
	if !ok {
		return nil, fmt.Errorf("agent section not found")
	}
	return DecodeAgentSection(node)
}

func decodeSecuritySection(doc *Document) (SecuritySpec, error) {
	node, ok := doc.Section("security")
	if !ok {
		return SecuritySpec{}, nil
	}
	ss, err := DecodeSecuritySection(node)
	if err != nil || ss == nil {
		return SecuritySpec{}, err
	}
	return *ss, nil
}

// BuildEffectiveAgentContract constructs an EffectiveAgentContract from
// pre-resolved inputs. Callers (typically execution/session) are responsible
// for decoding each section via the appropriate domain's DecodeSection.
func BuildEffectiveAgentContract(agentID string, agentSpec *AgentSpec, perms configpermissions.PermissionSet, resources ResourceSpec, security SecuritySpec, sources SourceSummary) *EffectiveAgentContract {
	return &EffectiveAgentContract{
		AgentID:     agentID,
		AgentSpec:   agentSpec,
		Permissions: perms,
		Resources:   resources,
		Security:    security,
		Sources:     sources,
	}
}
