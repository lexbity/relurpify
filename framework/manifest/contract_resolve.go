package manifest

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// SkillResolver is an interface for resolving skills, provided by the skills package.
type SkillResolver interface {
	ResolveSkills(workspace string, baseSpec *agentspec.AgentRuntimeSpec, skillNames []string) (*agentspec.AgentRuntimeSpec, []ResolvedSkill, []SkillResolution)
}

// ResolvedSkill captures the effective runtime-facing result of resolving a single skill.
type ResolvedSkill struct {
	Name        string
	Manifest    *SkillManifest
	Spec        *agentspec.AgentRuntimeSpec
	Permissions []contracts.PermissionDescriptor
}

// SkillResolution records the outcome of attempting to resolve a skill.
type SkillResolution struct {
	Name     string
	Applied  bool
	Error    error
	Messages []string
}

// ResolveEffectiveAgentContract merges manifest defaults, skill
// contributions, and optional overlays into one runtime-facing contract.
// The resolver parameter is provided by the skills package to avoid import cycles.
func ResolveEffectiveAgentContract(workspace string, m *AgentManifest, opts ResolveOptions, resolver SkillResolver) (*EffectiveAgentContract, error) {
	if m == nil {
		return nil, fmt.Errorf("agent manifest required")
	}
	if strings.TrimSpace(workspace) == "" {
		return nil, fmt.Errorf("workspace required")
	}

	permissions, err := ResolveEffectivePermissions(workspace, m)
	if err != nil {
		return nil, fmt.Errorf("resolve permissions: %w", err)
	}
	resources, err := ResolveEffectiveResources(workspace, m)
	if err != nil {
		return nil, fmt.Errorf("resolve resources: %w", err)
	}

	baseSpec := ApplyManifestDefaultsForAgent(m.Metadata.Name, m.Spec.Agent, m.Spec.Defaults)
	if baseSpec == nil {
		baseSpec = &agentspec.AgentRuntimeSpec{}
	}

	var resolvedSpec *agentspec.AgentRuntimeSpec
	var resolvedSkills []ResolvedSkill
	var skillResults []SkillResolution

	if resolver != nil {
		resolvedSpec, resolvedSkills, skillResults = resolver.ResolveSkills(workspace, baseSpec, m.Spec.Skills)
	} else {
		resolvedSpec = baseSpec
	}

	finalSpec := ResolveAgentSpec(opts.GlobalConfig, resolvedSpec, opts.AgentOverlays...)

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
		ResolvedSkills: append([]ResolvedSkill{}, resolvedSkills...),
		SkillResults:   append([]SkillResolution{}, skillResults...),
		Sources:        sources,
	}, nil
}
