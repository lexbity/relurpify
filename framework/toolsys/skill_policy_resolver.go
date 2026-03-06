package toolsys

import (
	"sort"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
)

type ResolvedSkillPolicy struct {
	PhaseTools               map[string][]string
	VerificationSuccessTools []string
	RecoveryProbeTools       []string
	Planning                 ResolvedPlanningPolicy
	Review                   ResolvedReviewPolicy
}

type EffectiveSkillPolicy struct {
	Spec   *core.AgentRuntimeSpec
	Policy ResolvedSkillPolicy
}

type ResolvedPlanningPolicy struct {
	RequiredBeforeEdit      []string
	PreferredEditTools      []string
	PreferredVerifyTools    []string
	StepTemplates           []core.SkillStepTemplate
	RequireVerificationStep bool
}

type ResolvedReviewPolicy struct {
	Criteria        []string
	FocusTags       []string
	ApprovalRules   core.AgentReviewApprovalRules
	SeverityWeights map[string]float64
}

func ResolveSkillPolicy(registry *ToolRegistry, config core.AgentSkillConfig) ResolvedSkillPolicy {
	return ResolvedSkillPolicy{
		PhaseTools:               resolvePhaseTools(registry, config),
		VerificationSuccessTools: resolveToolNames(registry, config.Verification.SuccessTools, config.Verification.SuccessSelectors),
		RecoveryProbeTools:       resolveToolNames(registry, config.Recovery.FailureProbeTools, config.Recovery.FailureProbeSelectors),
		Planning: ResolvedPlanningPolicy{
			RequiredBeforeEdit:      resolveToolNames(registry, nil, config.Planning.RequiredBeforeEdit),
			PreferredEditTools:      resolveToolNames(registry, nil, config.Planning.PreferredEditTools),
			PreferredVerifyTools:    resolveToolNames(registry, nil, config.Planning.PreferredVerifyTools),
			StepTemplates:           append([]core.SkillStepTemplate{}, config.Planning.StepTemplates...),
			RequireVerificationStep: config.Planning.RequireVerificationStep,
		},
		Review: ResolvedReviewPolicy{
			Criteria:        append([]string{}, config.Review.Criteria...),
			FocusTags:       append([]string{}, config.Review.FocusTags...),
			ApprovalRules:   config.Review.ApprovalRules,
			SeverityWeights: cloneSeverityWeights(config.Review.SeverityWeights),
		},
	}
}

func ResolveEffectiveSkillPolicy(task *core.Task, fallback *core.AgentRuntimeSpec, registry *ToolRegistry) EffectiveSkillPolicy {
	spec := EffectiveAgentSpec(task, fallback)
	if spec == nil {
		return EffectiveSkillPolicy{}
	}
	return EffectiveSkillPolicy{
		Spec:   spec,
		Policy: ResolveSkillPolicy(registry, spec.SkillConfig),
	}
}

func EffectiveAgentSpec(task *core.Task, fallback *core.AgentRuntimeSpec) *core.AgentRuntimeSpec {
	if task != nil && task.Context != nil {
		if spec, ok := task.Context["agent_spec"].(*core.AgentRuntimeSpec); ok && spec != nil {
			return spec
		}
	}
	return fallback
}

func resolvePhaseTools(registry *ToolRegistry, config core.AgentSkillConfig) map[string][]string {
	if len(config.PhaseTools) == 0 && len(config.PhaseSelectors) == 0 {
		return nil
	}
	out := make(map[string][]string)
	for phase, tools := range config.PhaseTools {
		out[phase] = mergeResolvedNames(out[phase], resolveToolNames(registry, tools, nil))
	}
	for phase, selectors := range config.PhaseSelectors {
		out[phase] = mergeResolvedNames(out[phase], resolveToolNames(registry, nil, selectors))
	}
	return out
}

func resolveToolNames(registry *ToolRegistry, explicit []string, selectors []core.SkillToolSelector) []string {
	var out []string
	for _, name := range explicit {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if registry != nil {
			if _, ok := registry.Get(name); !ok {
				continue
			}
		}
		out = mergeResolvedNames(out, []string{name})
	}
	if len(selectors) == 0 {
		return out
	}
	candidates := registryToolsSorted(registry)
	for _, selector := range selectors {
		if strings.TrimSpace(selector.Tool) != "" {
			if registry == nil {
				out = mergeResolvedNames(out, []string{selector.Tool})
				continue
			}
			tool, ok := registry.Get(selector.Tool)
			if ok && selectorMatchesTool(selector, tool) {
				out = mergeResolvedNames(out, []string{tool.Name()})
			}
			continue
		}
		for _, tool := range candidates {
			if selectorMatchesTool(selector, tool) {
				out = mergeResolvedNames(out, []string{tool.Name()})
			}
		}
	}
	return out
}

func selectorMatchesTool(selector core.SkillToolSelector, tool core.Tool) bool {
	if tool == nil {
		return false
	}
	if selector.Tool != "" && !strings.EqualFold(strings.TrimSpace(selector.Tool), tool.Name()) {
		return false
	}
	tags := normalizeTags(tool.Tags())
	for _, want := range selector.Tags {
		if _, ok := tags[strings.ToLower(strings.TrimSpace(want))]; !ok {
			return false
		}
	}
	for _, blocked := range selector.ExcludeTags {
		if _, ok := tags[strings.ToLower(strings.TrimSpace(blocked))]; ok {
			return false
		}
	}
	return true
}

func registryToolsSorted(registry *ToolRegistry) []core.Tool {
	if registry == nil {
		return nil
	}
	tools := registry.All()
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name() < tools[j].Name()
	})
	return tools
}

func normalizeTags(tags []string) map[string]struct{} {
	out := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		out[tag] = struct{}{}
	}
	return out
}

func mergeResolvedNames(base, extra []string) []string {
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, name := range append(append([]string{}, base...), extra...) {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func cloneSeverityWeights(input map[string]float64) map[string]float64 {
	if input == nil {
		return nil
	}
	out := make(map[string]float64, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}
