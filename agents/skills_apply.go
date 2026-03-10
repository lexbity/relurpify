package agents

import (
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
)

// ApplySkills merges skill contributions (flat, no inheritance) into baseSpec
// and returns the updated spec plus per-skill resolution results.
func ApplySkills(workspace string, baseSpec *core.AgentRuntimeSpec, skillNames []string,
	registry *capability.Registry, permissions *capability.PermissionManager, agentID string,
) (*core.AgentRuntimeSpec, []SkillResolution) {
	spec := core.MergeAgentSpecs(baseSpec)
	results := make([]SkillResolution, 0, len(skillNames))
	allowedCapabilities := core.EffectiveAllowedCapabilitySelectors(spec)
	toolPolicies := cloneToolPolicies(spec.ToolExecutionPolicy)
	capabilityPolicies := append([]core.CapabilityPolicy{}, spec.CapabilityPolicies...)
	insertionPolicies := append([]core.CapabilityInsertionPolicy{}, spec.InsertionPolicies...)
	globalPolicies := cloneGlobalPolicies(spec.GlobalPolicies)
	providerPolicies := cloneProviderPolicies(spec.ProviderPolicies)
	providers := cloneProviderConfigs(spec.Providers)
	skillConfig := core.AgentSkillConfig{}

	_ = permissions
	_ = agentID

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
		if err := registerSkillCapabilities(registry, skillManifest, paths); err != nil {
			results = append(results, logSkillError(workspace, skillName, "capability_registration_failed", err, paths))
			continue
		}

		allowedCapabilities = mergeCapabilitySelectors(allowedCapabilities, skillAllowedCapabilities(skillManifest.Spec))
		mergeToolExecutionPolicies(&toolPolicies, skillManifest.Spec.ToolExecutionPolicy)
		capabilityPolicies = appendCapabilityPolicies(capabilityPolicies, skillManifest.Spec.CapabilityPolicies)
		insertionPolicies = appendInsertionPolicies(insertionPolicies, skillManifest.Spec.InsertionPolicies)
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
	}

	spec.AllowedCapabilities = allowedCapabilities
	spec.ToolExecutionPolicy = toolPolicies
	spec.CapabilityPolicies = capabilityPolicies
	spec.InsertionPolicies = insertionPolicies
	spec.GlobalPolicies = globalPolicies
	spec.ProviderPolicies = providerPolicies
	spec.Providers = providers
	spec.SkillConfig = core.MergeAgentSpecs(
		&core.AgentRuntimeSpec{SkillConfig: spec.SkillConfig},
		core.AgentSpecOverlay{SkillConfig: &skillConfig},
	).SkillConfig
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
