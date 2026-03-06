package manifest

import (
	"fmt"
	"github.com/lexcodex/relurpify/framework/core"
	"gopkg.in/yaml.v3"
	"os"
	"strings"
)

// SkillManifest defines a reusable skill package.
type SkillManifest struct {
	APIVersion string           `yaml:"apiVersion" json:"apiVersion"`
	Kind       string           `yaml:"kind" json:"kind"`
	Metadata   ManifestMetadata `yaml:"metadata" json:"metadata"`
	Spec       SkillSpec        `yaml:"spec" json:"spec"`
	SourcePath string           `yaml:"-" json:"-"`
}

// SkillSpec defines prompt snippets, tool allowances, execution policies, and resource paths.
type SkillSpec struct {
	Requires            SkillRequiresSpec                   `yaml:"requires,omitempty" json:"requires,omitempty"`
	PromptSnippets      []string                            `yaml:"prompt_snippets,omitempty" json:"prompt_snippets,omitempty"`
	AllowedTools        []string                            `yaml:"allowed_tools,omitempty" json:"allowed_tools,omitempty"`
	ToolExecutionPolicy map[string]core.ToolPolicy          `yaml:"tool_execution_policy,omitempty" json:"tool_execution_policy,omitempty"`
	ResourcePaths       SkillResourceSpec                   `yaml:"resource_paths,omitempty" json:"resource_paths,omitempty"`
	PhaseTools          map[string][]string                 `yaml:"phase_tools,omitempty" json:"phase_tools,omitempty"`
	PhaseSelectors      map[string][]core.SkillToolSelector `yaml:"phase_selectors,omitempty" json:"phase_selectors,omitempty"`
	Verification        SkillVerificationSpec               `yaml:"verification,omitempty" json:"verification,omitempty"`
	Recovery            SkillRecoverySpec                   `yaml:"recovery,omitempty" json:"recovery,omitempty"`
	Planning            SkillPlanningSpec                   `yaml:"planning,omitempty" json:"planning,omitempty"`
	Review              SkillReviewSpec                     `yaml:"review,omitempty" json:"review,omitempty"`
	ContextHints        SkillContextHintsSpec               `yaml:"context_hints,omitempty" json:"context_hints,omitempty"`
}

// SkillRequiresSpec declares binary prerequisites for a skill.
type SkillRequiresSpec struct {
	Bins []string `yaml:"bins,omitempty" json:"bins,omitempty"`
}

// SkillResourceSpec declares resource paths.
type SkillResourceSpec struct {
	Scripts   []string `yaml:"scripts,omitempty" json:"scripts,omitempty"`
	Resources []string `yaml:"resources,omitempty" json:"resources,omitempty"`
	Templates []string `yaml:"templates,omitempty" json:"templates,omitempty"`
}

type SkillVerificationSpec struct {
	SuccessTools     []string                 `yaml:"success_tools,omitempty" json:"success_tools,omitempty"`
	SuccessSelectors []core.SkillToolSelector `yaml:"success_selectors,omitempty" json:"success_selectors,omitempty"`
	StopOnSuccess    bool                     `yaml:"stop_on_success,omitempty" json:"stop_on_success,omitempty"`
}

type SkillRecoverySpec struct {
	FailureProbeTools     []string                 `yaml:"failure_probe_tools,omitempty" json:"failure_probe_tools,omitempty"`
	FailureProbeSelectors []core.SkillToolSelector `yaml:"failure_probe_selectors,omitempty" json:"failure_probe_selectors,omitempty"`
}

type SkillPlanningSpec struct {
	RequiredBeforeEdit      []core.SkillToolSelector `yaml:"required_before_edit,omitempty" json:"required_before_edit,omitempty"`
	PreferredEditTools      []core.SkillToolSelector `yaml:"preferred_edit_tools,omitempty" json:"preferred_edit_tools,omitempty"`
	PreferredVerifyTools    []core.SkillToolSelector `yaml:"preferred_verify_tools,omitempty" json:"preferred_verify_tools,omitempty"`
	StepTemplates           []core.SkillStepTemplate `yaml:"step_templates,omitempty" json:"step_templates,omitempty"`
	RequireVerificationStep bool                     `yaml:"require_verification_step,omitempty" json:"require_verification_step,omitempty"`
}

type SkillReviewSpec struct {
	Criteria        []string                      `yaml:"criteria,omitempty" json:"criteria,omitempty"`
	FocusTags       []string                      `yaml:"focus_tags,omitempty" json:"focus_tags,omitempty"`
	ApprovalRules   core.AgentReviewApprovalRules `yaml:"approval_rules,omitempty" json:"approval_rules,omitempty"`
	SeverityWeights map[string]float64            `yaml:"severity_weights,omitempty" json:"severity_weights,omitempty"`
}

type SkillContextHintsSpec struct {
	PreferredDetailLevel string   `yaml:"preferred_detail_level,omitempty" json:"preferred_detail_level,omitempty"`
	ProtectPatterns      []string `yaml:"protect_patterns,omitempty" json:"protect_patterns,omitempty"`
}

// LoadSkillManifest parses and validates a skill manifest file.
func LoadSkillManifest(path string) (*SkillManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest SkillManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	manifest.SourcePath = path
	return &manifest, nil
}

// Validate enforces manifest semantics.
func (m *SkillManifest) Validate() error {
	if m.APIVersion == "" {
		return fmt.Errorf("skill manifest missing apiVersion")
	}
	if m.Kind == "" {
		return fmt.Errorf("skill manifest missing kind")
	}
	if m.Metadata.Name == "" {
		return fmt.Errorf("skill manifest missing metadata.name")
	}
	if strings.ToLower(m.Kind) != strings.ToLower("SkillManifest") {
		return fmt.Errorf("skill manifest kind must be SkillManifest")
	}
	for _, bin := range m.Spec.Requires.Bins {
		if strings.TrimSpace(bin) == "" {
			return fmt.Errorf("requires.bins contains empty entry")
		}
		if strings.Contains(bin, "/") {
			return fmt.Errorf("requires.bins entry %q must not contain '/'", bin)
		}
	}
	for phase, tools := range m.Spec.PhaseTools {
		if strings.TrimSpace(phase) == "" {
			return fmt.Errorf("phase_tools contains empty phase")
		}
		for _, tool := range tools {
			if strings.TrimSpace(tool) == "" {
				return fmt.Errorf("phase_tools[%s] contains empty tool", phase)
			}
		}
	}
	for phase, selectors := range m.Spec.PhaseSelectors {
		if strings.TrimSpace(phase) == "" {
			return fmt.Errorf("phase_selectors contains empty phase")
		}
		for _, selector := range selectors {
			if err := core.ValidateSkillToolSelector(selector); err != nil {
				return fmt.Errorf("phase_selectors[%s] invalid: %w", phase, err)
			}
		}
	}
	for _, selector := range m.Spec.Verification.SuccessSelectors {
		if err := core.ValidateSkillToolSelector(selector); err != nil {
			return fmt.Errorf("verification.success_selectors invalid: %w", err)
		}
	}
	for _, selector := range m.Spec.Recovery.FailureProbeSelectors {
		if err := core.ValidateSkillToolSelector(selector); err != nil {
			return fmt.Errorf("recovery.failure_probe_selectors invalid: %w", err)
		}
	}
	for _, selector := range m.Spec.Planning.RequiredBeforeEdit {
		if err := core.ValidateSkillToolSelector(selector); err != nil {
			return fmt.Errorf("planning.required_before_edit invalid: %w", err)
		}
	}
	for _, selector := range m.Spec.Planning.PreferredEditTools {
		if err := core.ValidateSkillToolSelector(selector); err != nil {
			return fmt.Errorf("planning.preferred_edit_tools invalid: %w", err)
		}
	}
	for _, selector := range m.Spec.Planning.PreferredVerifyTools {
		if err := core.ValidateSkillToolSelector(selector); err != nil {
			return fmt.Errorf("planning.preferred_verify_tools invalid: %w", err)
		}
	}
	for _, step := range m.Spec.Planning.StepTemplates {
		if strings.TrimSpace(step.Kind) == "" {
			return fmt.Errorf("planning.step_templates contains empty kind")
		}
		if strings.TrimSpace(step.Description) == "" {
			return fmt.Errorf("planning.step_templates[%s] contains empty description", step.Kind)
		}
	}
	for _, criterion := range m.Spec.Review.Criteria {
		if strings.TrimSpace(criterion) == "" {
			return fmt.Errorf("review.criteria contains empty criterion")
		}
	}
	for _, tag := range m.Spec.Review.FocusTags {
		if strings.TrimSpace(tag) == "" {
			return fmt.Errorf("review.focus_tags contains empty tag")
		}
	}
	for severity, weight := range m.Spec.Review.SeverityWeights {
		if strings.TrimSpace(severity) == "" {
			return fmt.Errorf("review.severity_weights contains empty severity")
		}
		if weight < 0 {
			return fmt.Errorf("review.severity_weights[%s] must be >= 0", severity)
		}
	}
	return nil
}
