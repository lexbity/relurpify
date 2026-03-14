package contract

import (
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/agents"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
)

// ResolveEffectiveAgentContract merges manifest defaults, skill
// contributions, and optional overlays into one runtime-facing contract.
func ResolveEffectiveAgentContract(workspace string, m *manifest.AgentManifest, opts ResolveOptions) (*EffectiveAgentContract, error) {
	if m == nil {
		return nil, fmt.Errorf("agent manifest required")
	}
	if strings.TrimSpace(workspace) == "" {
		return nil, fmt.Errorf("workspace required")
	}

	permissions, err := manifest.ResolveEffectivePermissions(workspace, m)
	if err != nil {
		return nil, fmt.Errorf("resolve permissions: %w", err)
	}
	resources, err := manifest.ResolveEffectiveResources(workspace, m)
	if err != nil {
		return nil, fmt.Errorf("resolve resources: %w", err)
	}

	baseSpec := agents.ApplyManifestDefaultsForAgent(m.Metadata.Name, m.Spec.Agent, m.Spec.Defaults)
	if baseSpec == nil {
		baseSpec = &core.AgentRuntimeSpec{}
	}
	resolvedSpec, resolvedSkills, skillResults := agents.ResolveSkills(workspace, baseSpec, m.Spec.Skills)
	finalSpec := agents.ResolveAgentSpec(opts.GlobalConfig, resolvedSpec, opts.AgentOverlays...)

	sources := SourceSummary{
		ManifestName:     m.Metadata.Name,
		ManifestVersion:  m.Metadata.Version,
		Workspace:        workspace,
		RequestedSkills:  append([]string{}, m.Spec.Skills...),
		GlobalDefaults:   opts.GlobalConfig != nil,
		OverlayCount:     len(opts.AgentOverlays),
		RuntimeOverrides: len(opts.AgentOverlays),
	}
	for _, result := range skillResults {
		if result.Applied {
			sources.AppliedSkills = append(sources.AppliedSkills, result.Name)
			continue
		}
		sources.FailedSkills = append(sources.FailedSkills, result.Name)
	}

	return &EffectiveAgentContract{
		AgentID:        m.Metadata.Name,
		Manifest:       m,
		AgentSpec:      finalSpec,
		Permissions:    permissions,
		Resources:      resources,
		ResolvedSkills: append([]agents.ResolvedSkill{}, resolvedSkills...),
		SkillResults:   append([]agents.SkillResolution{}, skillResults...),
		Sources:        sources,
	}, nil
}
