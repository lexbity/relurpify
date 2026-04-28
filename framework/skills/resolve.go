package skills

import (
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/manifest"
)

// ResolveSkills merges skill contributions (flat, no inheritance) into
// baseSpec and returns the updated spec, validated skill manifests, and
// per-skill resolution results.
// Resolution is pure: it does not mutate the capability registry.
func ResolveSkills(workspace string, baseSpec *agentspec.AgentRuntimeSpec, skillNames []string) (*agentspec.AgentRuntimeSpec, []ResolvedSkill, []SkillResolution) {
	spec := agentspec.MergeAgentSpecs(baseSpec)
	results := make([]SkillResolution, 0, len(skillNames))
	var allowedCapabilities []agentspec.CapabilitySelector
	if spec != nil && spec.AllowedCapabilities != nil {
		allowedCapabilities = agentspec.CloneCapabilitySelectors(spec.AllowedCapabilities)
	}
	toolPolicies := cloneToolPolicies(spec.ToolExecutionPolicy)
	capabilityPolicies := append([]agentspec.CapabilityPolicy{}, spec.CapabilityPolicies...)
	insertionPolicies := append([]agentspec.CapabilityInsertionPolicy{}, spec.InsertionPolicies...)
	sessionPolicies := cloneSessionPolicies(spec.SessionPolicies)
	globalPolicies := cloneGlobalPolicies(spec.GlobalPolicies)
	providerPolicies := cloneProviderPolicies(spec.ProviderPolicies)
	providers := cloneProviderConfigs(spec.Providers)
	skillConfig := agentspec.AgentSkillConfig{}
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
		providers = mergeProviderConfigs(providers, convertCoreProviderConfigs(skillManifest.Spec.Providers))
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
	spec.SkillConfig = agentspec.MergeAgentSpecs(
		&agentspec.AgentRuntimeSpec{SkillConfig: spec.SkillConfig},
		agentspec.AgentSpecOverlay{SkillConfig: &skillConfig},
	).SkillConfig
	return spec, resolved, results
}

func convertCoreProviderConfigs(values []core.ProviderConfig) []agentspec.ProviderConfig {
	if len(values) == 0 {
		return nil
	}
	out := make([]agentspec.ProviderConfig, len(values))
	for i, provider := range values {
		out[i] = agentspec.ProviderConfig{
			ID:              provider.ID,
			Kind:            agentspec.ProviderKind(provider.Kind),
			Enabled:         provider.Enabled,
			Target:          provider.Target,
			ActivationScope: provider.ActivationScope,
			TrustBaseline:   agentspec.TrustClass(provider.TrustBaseline),
			Recoverability:  agentspec.RecoverabilityMode(provider.Recoverability),
		}
		if provider.Config != nil {
			out[i].Config = make(map[string]any, len(provider.Config))
			for key, value := range provider.Config {
				out[i].Config[key] = value
			}
		}
	}
	return out
}

// ApplySkills resolves skill contributions and then registers any skill-backed
// capabilities against the provided registry.
func ApplySkills(workspace string, baseSpec *agentspec.AgentRuntimeSpec, skillNames []string,
	registry *capability.Registry, permissions *capability.PermissionManager, agentID string,
) (*agentspec.AgentRuntimeSpec, []SkillResolution) {
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

// ExtractContextContributions extracts context contributions from resolved skills.
// This is used by the contextpolicy compiler to merge skill contributions into the policy bundle.
func ExtractContextContributions(resolved []ResolvedSkill) []SkillContextContribution {
	contributions := make([]SkillContextContribution, 0, len(resolved))
	for _, skill := range resolved {
		if skill.Manifest == nil {
			continue
		}
		contrib := skill.Manifest.Spec.ContextContributions
		if len(contrib.IngestionSources) > 0 || len(contrib.RankerAdmission) > 0 || len(contrib.ScannerConfig.AdditionalSignatures) > 0 {
			contributions = append(contributions, SkillContextContribution{
				SkillName:         skill.Manifest.Metadata.Name,
				IngestionSources:  contrib.IngestionSources,
				RankerAdmission:   contrib.RankerAdmission,
				ScannerSignatures: contrib.ScannerConfig.AdditionalSignatures,
			})
		}
	}
	return contributions
}
