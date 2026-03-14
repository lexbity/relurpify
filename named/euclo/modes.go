package euclo

import (
	"fmt"
	"sort"
	"strings"
)

type ModeDescriptor struct {
	ModeID                      string
	IntentFamily                string
	EditPolicy                  string
	EvidencePolicy              string
	VerificationPolicy          string
	ReviewPolicy                string
	DefaultExecutionProfiles    []string
	FallbackExecutionProfiles   []string
	PreferredCapabilityFamilies []string
	ContextStrategy             string
	RecoveryPolicy              string
	ReportingPolicy             string
}

type ModeRegistry struct {
	descriptors map[string]ModeDescriptor
}

type ModeResolution struct {
	ModeID      string   `json:"mode_id"`
	Source      string   `json:"source"`
	ReasonCodes []string `json:"reason_codes,omitempty"`
	Constraints []string `json:"constraints,omitempty"`
}

func NewModeRegistry() *ModeRegistry {
	return &ModeRegistry{descriptors: map[string]ModeDescriptor{}}
}

func DefaultModeRegistry() *ModeRegistry {
	registry := NewModeRegistry()
	for _, descriptor := range []ModeDescriptor{
		{
			ModeID:                      "code",
			IntentFamily:                "implementation",
			EditPolicy:                  "allowed",
			EvidencePolicy:              "local_evidence_before_edit",
			VerificationPolicy:          "required",
			ReviewPolicy:                "secondary",
			DefaultExecutionProfiles:    []string{"edit_verify_repair"},
			FallbackExecutionProfiles:   []string{"reproduce_localize_patch", "plan_stage_execute", "review_suggest_implement"},
			PreferredCapabilityFamilies: []string{"bounded_implementation", "verification", "targeted_planning"},
			ContextStrategy:             "narrow_to_wide",
			RecoveryPolicy:              "repair_then_escalate",
			ReportingPolicy:             "artifact_summary",
		},
		{
			ModeID:                      "debug",
			IntentFamily:                "debugging",
			EditPolicy:                  "delayed",
			EvidencePolicy:              "reproduction_or_localization_required",
			VerificationPolicy:          "rerun_relevant_failure_required",
			ReviewPolicy:                "secondary",
			DefaultExecutionProfiles:    []string{"reproduce_localize_patch"},
			FallbackExecutionProfiles:   []string{"trace_execute_analyze", "edit_verify_repair", "plan_stage_execute"},
			PreferredCapabilityFamilies: []string{"debugging", "tracing", "diagnostics"},
			ContextStrategy:             "localize_then_expand",
			RecoveryPolicy:              "gather_more_evidence",
			ReportingPolicy:             "root_cause_first",
		},
		{
			ModeID:                      "tdd",
			IntentFamily:                "test_driven_development",
			EditPolicy:                  "allowed",
			EvidencePolicy:              "test_artifact_first",
			VerificationPolicy:          "tests_required",
			ReviewPolicy:                "secondary",
			DefaultExecutionProfiles:    []string{"test_driven_generation"},
			FallbackExecutionProfiles:   []string{"edit_verify_repair", "plan_stage_execute"},
			PreferredCapabilityFamilies: []string{"test_generation", "implementation", "verification"},
			ContextStrategy:             "targeted",
			RecoveryPolicy:              "failing_test_driven",
			ReportingPolicy:             "test_and_patch_summary",
		},
		{
			ModeID:                      "review",
			IntentFamily:                "review",
			EditPolicy:                  "disallowed",
			EvidencePolicy:              "evidence_first",
			VerificationPolicy:          "optional",
			ReviewPolicy:                "primary",
			DefaultExecutionProfiles:    []string{"review_suggest_implement"},
			FallbackExecutionProfiles:   []string{"plan_stage_execute"},
			PreferredCapabilityFamilies: []string{"review", "analysis"},
			ContextStrategy:             "read_heavy",
			RecoveryPolicy:              "request_clarification",
			ReportingPolicy:             "findings_first",
		},
		{
			ModeID:                      "planning",
			IntentFamily:                "planning",
			EditPolicy:                  "disallowed",
			EvidencePolicy:              "context_collection",
			VerificationPolicy:          "optional",
			ReviewPolicy:                "secondary",
			DefaultExecutionProfiles:    []string{"plan_stage_execute"},
			FallbackExecutionProfiles:   []string{"review_suggest_implement"},
			PreferredCapabilityFamilies: []string{"planning", "analysis"},
			ContextStrategy:             "expand_carefully",
			RecoveryPolicy:              "clarify_scope",
			ReportingPolicy:             "plan_summary",
		},
	} {
		_ = registry.Register(descriptor)
	}
	return registry
}

func (r *ModeRegistry) Register(descriptor ModeDescriptor) error {
	if r == nil {
		return fmt.Errorf("mode registry unavailable")
	}
	id := strings.TrimSpace(strings.ToLower(descriptor.ModeID))
	if id == "" {
		return fmt.Errorf("mode id required")
	}
	descriptor.ModeID = id
	if len(descriptor.DefaultExecutionProfiles) == 0 {
		return fmt.Errorf("mode %s requires at least one default execution profile", id)
	}
	r.descriptors[id] = descriptor
	return nil
}

func (r *ModeRegistry) Lookup(modeID string) (ModeDescriptor, bool) {
	if r == nil {
		return ModeDescriptor{}, false
	}
	descriptor, ok := r.descriptors[strings.TrimSpace(strings.ToLower(modeID))]
	return descriptor, ok
}

func (r *ModeRegistry) List() []ModeDescriptor {
	if r == nil {
		return nil
	}
	keys := make([]string, 0, len(r.descriptors))
	for key := range r.descriptors {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]ModeDescriptor, 0, len(keys))
	for _, key := range keys {
		out = append(out, r.descriptors[key])
	}
	return out
}
