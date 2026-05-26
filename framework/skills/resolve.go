package skills

import (
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/cfgload"
)

// ResolveSkills merges thin skill contributions into baseSpec and returns the
// updated spec, validated skill manifests, and per-skill resolution results.
// Resolution is pure: it does not mutate the capability registry.
func ResolveSkills(workspace string, baseSpec *agentspec.AgentRuntimeSpec, skillNames []string) (*agentspec.AgentRuntimeSpec, []cfgload.ResolvedSkill, []cfgload.SkillResolution) {
	spec := agentspec.MergeAgentSpecs(baseSpec)
	results := make([]cfgload.SkillResolution, 0, len(skillNames))
	var allowedCapabilities []agentspec.CapabilitySelector
	if spec != nil && spec.AllowedCapabilities != nil {
		allowedCapabilities = agentspec.CloneCapabilitySelectors(spec.AllowedCapabilities)
	}
	resolved := make([]cfgload.ResolvedSkill, 0, len(skillNames))

	for _, name := range skillNames {
		skillName := strings.TrimSpace(name)
		if skillName == "" {
			continue
		}

		skillManifest, err := cfgload.LoadSkill(workspace, skillName)
		if err != nil {
			results = append(results, logSkillError(skillName, err))
			continue
		}

		allowedCapabilities = mergeCapabilitySelectors(allowedCapabilities, skillAllowedCapabilities(skillManifest.Spec))
		if len(skillManifest.Spec.PromptSnippets) > 0 {
			spec.Prompt = mergePromptSnippets(spec.Prompt, skillManifest.Spec.PromptSnippets)
		}
		results = append(results, cfgload.SkillResolution{
			Name:    skillManifest.Metadata.Name,
			Applied: true,
		})
		resolved = append(resolved, cfgload.ResolvedSkill{
			Manifest: skillManifest,
		})
	}

	spec.AllowedCapabilities = allowedCapabilities
	return spec, resolved, results
}

func logSkillError(name string, err error) cfgload.SkillResolution {
	return cfgload.SkillResolution{
		Name:    name,
		Applied: false,
		Error:   err,
	}
}

func mergePromptSnippets(base string, snippets []string) string {
	builder := strings.Builder{}
	base = strings.TrimSpace(base)
	if base != "" {
		builder.WriteString(base)
	}
	for _, snippet := range snippets {
		snippet = strings.TrimSpace(snippet)
		if snippet == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(snippet)
	}
	return builder.String()
}
