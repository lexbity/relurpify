package config

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/governance/permissions"
)

// EffectiveAgentContract captures the resolved runtime-facing contract derived
// from the manifest and any later overlays.
type EffectiveAgentContract struct {
	AgentID     string
	Manifest    *AgentManifest
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
func ResolveEffectiveResources(_ string, m *AgentManifest) (ResourceSpec, error) {
	base := ResourceSpec{}
	var overlays []*ResourceSpec
	if m != nil && m.Spec.Defaults != nil && m.Spec.Defaults.Resources != nil {
		base = *m.Spec.Defaults.Resources
	}
	if m != nil {
		overlays = append(overlays, &m.Spec.Resources)
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
func ResolveEffectiveAgentContract(workspace string, m *AgentManifest, opts ResolveOptions) (*EffectiveAgentContract, error) {
	if m == nil {
		return nil, fmt.Errorf("agent manifest required")
	}
	if strings.TrimSpace(workspace) == "" {
		return nil, fmt.Errorf("workspace required")
	}

	resources, err := ResolveEffectiveResources(workspace, m)
	if err != nil {
		return nil, fmt.Errorf("resolve resources: %w", err)
	}

	baseSpec := m.Spec.Agent
	if baseSpec == nil {
		baseSpec = &agentspec.AgentRuntimeSpec{}
	}

	sources := SourceSummary{
		ManifestName:    m.Metadata.Name,
		ManifestVersion: m.Metadata.Version,
		Workspace:       workspace,
		GlobalDefaults:  false,
	}

	return &EffectiveAgentContract{
		AgentID:     m.Metadata.Name,
		Manifest:    m,
		AgentSpec:   baseSpec,
		Permissions: m.Spec.Permissions,
		Resources:   resources,
		Sources:     sources,
	}, nil
}
