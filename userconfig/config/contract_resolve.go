package config

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/governance/permissions"
)

// EffectiveAgentContract captures the resolved runtime-facing contract derived
// from the manifest spec and any later overlays.
type EffectiveAgentContract struct {
	AgentID     string
	Spec        *ManifestSpec
	AgentSpec   *agentspec.AgentRuntimeSpec
	Permissions permissions.PermissionSet
	Resources   ResourceSpec
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

// ResolveOptions provides optional inputs layered on top of the raw manifest.
type ResolveOptions struct {
}

// ResolveEffectiveResources merges defaults and manifest resources.
func ResolveEffectiveResources(_ string, spec *ManifestSpec) (ResourceSpec, error) {
	base := ResourceSpec{}
	var overlays []*ResourceSpec
	if spec != nil && spec.Defaults != nil && spec.Defaults.Resources != nil {
		base = *spec.Defaults.Resources
	}
	if spec != nil {
		overlays = append(overlays, &spec.Resources)
	}
	return MergeResourceSpecs(base, overlays...), nil
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

// ResolveEffectiveAgentContract merges manifest defaults and optional overlays
// into one runtime-facing contract. Permission resolution (decode + merge) is
// handled on the governance side via governance/permissions functions.
func ResolveEffectiveAgentContract(workspace string, spec *ManifestSpec, opts ResolveOptions) (*EffectiveAgentContract, error) {
	if spec == nil {
		return nil, fmt.Errorf("manifest spec required")
	}
	if strings.TrimSpace(workspace) == "" {
		return nil, fmt.Errorf("workspace required")
	}

	resources, err := ResolveEffectiveResources(workspace, spec)
	if err != nil {
		return nil, fmt.Errorf("resolve resources: %w", err)
	}

	baseSpec := spec.Agent
	if baseSpec == nil {
		baseSpec = &agentspec.AgentRuntimeSpec{}
	}

	return &EffectiveAgentContract{
		AgentID:     "",
		Spec:        spec,
		AgentSpec:   baseSpec,
		Permissions: spec.Permissions,
		Resources:   resources,
		Sources:     SourceSummary{Workspace: workspace, GlobalDefaults: false},
	}, nil
}

// BuildEffectiveAgentContract constructs an EffectiveAgentContract from
// pre-resolved inputs. Callers (typically execution/session) are responsible
// for decoding each section via the appropriate domain's DecodeSection.
// The Manifest field is left nil — it is not needed for the Document path.
func BuildEffectiveAgentContract(agentID string, agentSpec *agentspec.AgentRuntimeSpec, perms permissions.PermissionSet, resources ResourceSpec, sources SourceSummary) *EffectiveAgentContract {
	return &EffectiveAgentContract{
		AgentID:     agentID,
		AgentSpec:   agentSpec,
		Permissions: perms,
		Resources:   resources,
		Sources:     sources,
	}
}
