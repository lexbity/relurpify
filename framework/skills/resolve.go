package skills

import (
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
)

// ResolveSkills merges skill contributions (flat, no inheritance) into
// baseSpec and returns the updated spec, validated skill manifests, and
// per-skill resolution results.
// Resolution is pure: it does not mutate the capability registry.
func ResolveSkills(workspace string, baseSpec *core.AgentRuntimeSpec, skillNames []string) (*core.AgentRuntimeSpec, []ResolvedSkill, []SkillResolution) {
	spec := core.MergeAgentSpecs(baseSpec)
	results := make([]SkillResolution, 0, len(skillNames))
	var allowedCapabilities []core.CapabilitySelector
	if spec != nil && spec.AllowedCapabilities != nil {
		allowedCapabilities = core.EffectiveAllowedCapabilitySelectors(spec)
	}
	toolPolicies := cloneToolPolicies(spec.ToolExecutionPolicy)
	capabilityPolicies := append([]core.CapabilityPolicy{}, spec.CapabilityPolicies...)
	insertionPolicies := append([]core.CapabilityInsertionPolicy{}, spec.InsertionPolicies...)
	sessionPolicies := cloneSessionPolicies(spec.SessionPolicies)
	globalPolicies := cloneGlobalPolicies(spec.GlobalPolicies)
	providerPolicies := cloneProviderPolicies(spec.ProviderPolicies)
	providers := cloneProviderConfigs(spec.Providers)
	skillConfig := core.AgentSkillConfig{}
	resolved := make([]ResolvedSkill, 0, len(skillNames))

	for _, name := range skillNames {
		skillName := strings.TrimSpace(name)
		if skillName == "" {
			continue
		}

		skillManifest, err := manifest.LoadSkill(workspace, skillName)
		if err != nil {
			results = append(results, logSkillError(workspace, skillName, "load_failed", err,
				SkillPaths{Root: SkillRoot(workspace, skillName)}))
			continue
		}

		if missingBin := findMissingBin(skillManifest.Spec.Requires.Bins); missingBin != "" {
			binErr := fmt.Errorf("required binary %q not found in PATH", missingBin)
			paths := resolveSkillPaths(skillManifest)
			results = append(results, logSkillError(workspace, skillName, "missing_binary", binErr, paths))
			continue
		}

		paths := resolveSkillPaths(skillManifest)
		if err := validateSkillPaths(paths); err != nil {
			results = append(results, logSkillError(workspace, skillName, "missing_resources", err, paths))
			continue
		}

		allowedCapabilities = mergeCapabilitySelectors(allowedCapabilities, skillAllowedCapabilities(skillManifest.Spec))
		mergeToolExecutionPolicies(&toolPolicies, skillManifest.Spec.ToolExecutionPolicy)
		capabilityPolicies = appendCapabilityPolicies(capabilityPolicies, skillManifest.Spec.CapabilityPolicies)
		insertionPolicies = appendInsertionPolicies(insertionPolicies, skillManifest.Spec.InsertionPolicies)
		sessionPolicies = appendSessionPolicies(sessionPolicies, skillManifest.Spec.SessionPolicies)
		mergeGlobalPolicies(&globalPolicies, skillManifest.Spec.GlobalPolicies)
		mergeProviderPolicies(&providerPolicies, skillManifest.Spec.ProviderPolicies)
		providers = mergeProviderConfigs(providers, skillManifest.Spec.Providers)
		if len(skillManifest.Spec.PromptSnippets) > 0 {
			spec.Prompt = mergePromptSnippets(spec.Prompt, skillManifest.Spec.PromptSnippets)
		}
		skillConfig = mergeSkillConfig(skillConfig, skillManifest.Spec)

		results = append(results, SkillResolution{
			Name:    skillManifest.Metadata.Name,
			Applied: true,
			Paths:   paths,
		})
		resolved = append(resolved, ResolvedSkill{
			Manifest: skillManifest,
			Paths:    paths,
		})
	}

	spec.AllowedCapabilities = allowedCapabilities
	spec.ToolExecutionPolicy = toolPolicies
	spec.CapabilityPolicies = capabilityPolicies
	spec.InsertionPolicies = insertionPolicies
	spec.SessionPolicies = sessionPolicies
	spec.GlobalPolicies = globalPolicies
	spec.ProviderPolicies = providerPolicies
	spec.Providers = providers
	spec.SkillConfig = core.MergeAgentSpecs(
		&core.AgentRuntimeSpec{SkillConfig: spec.SkillConfig},
		core.AgentSpecOverlay{SkillConfig: &skillConfig},
	).SkillConfig
	return spec, resolved, results
}

// ApplySkills resolves skill contributions and then registers any skill-backed
// capabilities against the provided registry.
func ApplySkills(workspace string, baseSpec *core.AgentRuntimeSpec, skillNames []string,
	registry *capability.Registry, permissions *capability.PermissionManager, agentID string,
) (*core.AgentRuntimeSpec, []SkillResolution) {
	spec, resolved, resolutionResults := ResolveSkills(workspace, baseSpec, skillNames)
	results := make([]SkillResolution, 0, len(resolutionResults)+len(resolved))
	for _, result := range resolutionResults {
		if !result.Applied {
			results = append(results, result)
		}
	}
	_ = permissions
	_ = agentID
	for _, entry := range resolved {
		if err := registerSkillCapabilities(registry, entry.Manifest, entry.Paths); err != nil {
			results = append(results, logSkillError(workspace, entry.Manifest.Metadata.Name, "capability_registration_failed", err, entry.Paths))
			continue
		}
		results = append(results, SkillResolution{
			Name:    entry.Manifest.Metadata.Name,
			Applied: true,
			Paths:   entry.Paths,
		})
	}
	return spec, results
}

func logSkillError(workspace, name, reason string, err error, paths SkillPaths) SkillResolution {
	entry := fmt.Sprintf("[WARNING] %s skill %s (%s): %s", time.Now().UTC().Format(time.RFC3339), name, reason, err.Error())
	logSkillMessage(workspace, entry)
	return SkillResolution{
		Name:    name,
		Applied: false,
		Error:   err.Error(),
		Paths:   paths,
	}
}
