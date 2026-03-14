package euclo

import (
	"fmt"
	"sort"
	"strings"
)

type ExecutionProfileDescriptor struct {
	ProfileID            string
	SupportedModes       []string
	FallbackProfiles     []string
	RequiredArtifacts    []string
	CompletionContract   string
	PhaseRoutes          map[string]string
	MutationPolicy       string
	VerificationRequired bool
}

type ExecutionProfileRegistry struct {
	descriptors map[string]ExecutionProfileDescriptor
}

type ExecutionProfileSelection struct {
	ProfileID            string            `json:"profile_id"`
	FallbackProfileIDs   []string          `json:"fallback_profile_ids,omitempty"`
	RequiredArtifacts    []string          `json:"required_artifacts,omitempty"`
	CompletionContract   string            `json:"completion_contract,omitempty"`
	PhaseRoutes          map[string]string `json:"phase_routes,omitempty"`
	ReasonCodes          []string          `json:"reason_codes,omitempty"`
	MutationAllowed      bool              `json:"mutation_allowed"`
	VerificationRequired bool              `json:"verification_required"`
}

func NewExecutionProfileRegistry() *ExecutionProfileRegistry {
	return &ExecutionProfileRegistry{descriptors: map[string]ExecutionProfileDescriptor{}}
}

func DefaultExecutionProfileRegistry() *ExecutionProfileRegistry {
	registry := NewExecutionProfileRegistry()
	for _, descriptor := range []ExecutionProfileDescriptor{
		{
			ProfileID:            "edit_verify_repair",
			SupportedModes:       []string{"code", "debug", "tdd"},
			FallbackProfiles:     []string{"reproduce_localize_patch", "plan_stage_execute"},
			RequiredArtifacts:    []string{"euclo.intake", "euclo.classification", "euclo.verification"},
			CompletionContract:   "edits_planned_and_verification_recorded",
			PhaseRoutes:          map[string]string{"explore": "react", "plan": "pipeline", "edit": "pipeline", "verify": "react"},
			MutationPolicy:       "allowed",
			VerificationRequired: true,
		},
		{
			ProfileID:            "reproduce_localize_patch",
			SupportedModes:       []string{"debug", "code"},
			FallbackProfiles:     []string{"trace_execute_analyze", "edit_verify_repair"},
			RequiredArtifacts:    []string{"euclo.intake", "euclo.classification", "euclo.verification"},
			CompletionContract:   "reproduction_or_localization_before_patch",
			PhaseRoutes:          map[string]string{"reproduce": "react", "localize": "react", "patch": "pipeline", "verify": "react"},
			MutationPolicy:       "delayed",
			VerificationRequired: true,
		},
		{
			ProfileID:            "test_driven_generation",
			SupportedModes:       []string{"tdd", "code"},
			FallbackProfiles:     []string{"edit_verify_repair"},
			RequiredArtifacts:    []string{"euclo.intake", "euclo.classification", "euclo.verification"},
			CompletionContract:   "tests_or_failures_recorded_before_completion",
			PhaseRoutes:          map[string]string{"plan_tests": "planner", "implement": "pipeline", "verify": "react"},
			MutationPolicy:       "allowed",
			VerificationRequired: true,
		},
		{
			ProfileID:            "review_suggest_implement",
			SupportedModes:       []string{"review", "planning", "code"},
			FallbackProfiles:     []string{"plan_stage_execute"},
			RequiredArtifacts:    []string{"euclo.intake", "euclo.classification"},
			CompletionContract:   "review_findings_or_change_plan_produced",
			PhaseRoutes:          map[string]string{"review": "reflection", "summarize": "react"},
			MutationPolicy:       "disallowed",
			VerificationRequired: false,
		},
		{
			ProfileID:            "plan_stage_execute",
			SupportedModes:       []string{"planning", "code", "review", "debug"},
			FallbackProfiles:     []string{"review_suggest_implement"},
			RequiredArtifacts:    []string{"euclo.intake", "euclo.classification"},
			CompletionContract:   "plan_or_staged_strategy_produced",
			PhaseRoutes:          map[string]string{"plan": "planner", "stage": "pipeline", "summarize": "react"},
			MutationPolicy:       "disallowed",
			VerificationRequired: false,
		},
		{
			ProfileID:            "trace_execute_analyze",
			SupportedModes:       []string{"debug"},
			FallbackProfiles:     []string{"reproduce_localize_patch"},
			RequiredArtifacts:    []string{"euclo.intake", "euclo.classification"},
			CompletionContract:   "trace_or_diagnostic_evidence_produced",
			PhaseRoutes:          map[string]string{"trace": "react", "analyze": "reflection"},
			MutationPolicy:       "disallowed",
			VerificationRequired: false,
		},
	} {
		_ = registry.Register(descriptor)
	}
	return registry
}

func (r *ExecutionProfileRegistry) Register(descriptor ExecutionProfileDescriptor) error {
	if r == nil {
		return fmt.Errorf("execution profile registry unavailable")
	}
	id := strings.TrimSpace(strings.ToLower(descriptor.ProfileID))
	if id == "" {
		return fmt.Errorf("profile id required")
	}
	descriptor.ProfileID = id
	if len(descriptor.SupportedModes) == 0 {
		return fmt.Errorf("profile %s requires supported modes", id)
	}
	r.descriptors[id] = descriptor
	return nil
}

func (r *ExecutionProfileRegistry) Lookup(profileID string) (ExecutionProfileDescriptor, bool) {
	if r == nil {
		return ExecutionProfileDescriptor{}, false
	}
	descriptor, ok := r.descriptors[strings.TrimSpace(strings.ToLower(profileID))]
	return descriptor, ok
}

func (r *ExecutionProfileRegistry) List() []ExecutionProfileDescriptor {
	if r == nil {
		return nil
	}
	keys := make([]string, 0, len(r.descriptors))
	for key := range r.descriptors {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]ExecutionProfileDescriptor, 0, len(keys))
	for _, key := range keys {
		out = append(out, r.descriptors[key])
	}
	return out
}
