package relurpicabilities

import "sort"

const (
	CapabilityChatAsk                = "euclo:chat.ask"
	CapabilityChatImplement          = "euclo:chat.implement"
	CapabilityChatInspect            = "euclo:chat.inspect"
	CapabilityArchaeologyExplore     = "euclo:archaeology.explore"
	CapabilityArchaeologyCompilePlan = "euclo:archaeology.compile-plan"
	CapabilityArchaeologyImplement   = "euclo:archaeology.implement-plan"
	CapabilityDebugInvestigate       = "euclo:debug.investigate"

	CapabilityChatDirectEditExecution      = "euclo:chat.direct-edit-execution"
	CapabilityChatLocalReview              = "euclo:chat.local-review"
	CapabilityChatTargetedVerification     = "euclo:chat.targeted-verification-repair"
	CapabilityArchaeologyPatternSurface    = "euclo:archaeology.pattern-surface"
	CapabilityArchaeologyProspectiveAssess = "euclo:archaeology.prospective-assess"
	CapabilityArchaeologyConvergenceGuard  = "euclo:archaeology.convergence-guard"
	CapabilityArchaeologyCoherenceAssess   = "euclo:archaeology.coherence-assess"
	CapabilityArchaeologyScopeExpand       = "euclo:archaeology.scope-expansion-assess"
	CapabilityDebugRootCause               = "euclo:debug.root-cause"
	CapabilityDebugHypothesisRefine        = "euclo:debug.hypothesis-refine"
	CapabilityDebugLocalization            = "euclo:debug.localization"
	CapabilityDebugFlawSurface             = "euclo:debug.flaw-surface"
	CapabilityDebugVerificationRepair      = "euclo:debug.verification-repair"
)

type MutabilityContract string

const (
	MutabilityNonMutating        MutabilityContract = "non_mutating"
	MutabilityInspectFirst       MutabilityContract = "inspect_first"
	MutabilityPolicyConstrained  MutabilityContract = "policy_constrained_mutation"
	MutabilityPlanBoundExecution MutabilityContract = "plan_bound_execution"
)

type Descriptor struct {
	ID                      string             `json:"id,omitempty"`
	DisplayName             string             `json:"display_name,omitempty"`
	ModeFamily              string             `json:"mode_family,omitempty"`
	PrimaryCapable          bool               `json:"primary_capable"`
	SupportingOnly          bool               `json:"supporting_only"`
	Mutability              MutabilityContract `json:"mutability,omitempty"`
	ArchaeoAssociated       bool               `json:"archaeo_associated"`
	LazySemanticAcquisition bool               `json:"lazy_semantic_acquisition"`
	LLMDependent            bool               `json:"llm_dependent"`
	ArchaeoOperation        string             `json:"archaeo_operation,omitempty"`
	ExecutorRecipe          string             `json:"executor_recipe,omitempty"`
	ParadigmMix             []string           `json:"paradigm_mix,omitempty"`
	TransitionCompatible    []string           `json:"transition_compatible,omitempty"`
	SupportingCapabilities  []string           `json:"supporting_capabilities,omitempty"`
	Summary                 string             `json:"summary,omitempty"`
}

type Registry struct {
	descriptors map[string]Descriptor
}

func NewRegistry() *Registry {
	return &Registry{descriptors: map[string]Descriptor{}}
}

func DefaultRegistry() *Registry {
	r := NewRegistry()
	for _, desc := range []Descriptor{
		{
			ID:                   CapabilityChatAsk,
			DisplayName:          "Chat Ask",
			ModeFamily:           "chat",
			PrimaryCapable:       true,
			Mutability:           MutabilityNonMutating,
			ExecutorRecipe:       "chat.ask.react_inquiry",
			ParadigmMix:          []string{"react"},
			TransitionCompatible: []string{"chat", "debug"},
			SupportingCapabilities: []string{
				CapabilityChatInspect,
			},
			Summary: "Non-mutating engineering question answering and explanation.",
		},
		{
			ID:                      CapabilityChatImplement,
			DisplayName:             "Chat Implement",
			ModeFamily:              "chat",
			PrimaryCapable:          true,
			Mutability:              MutabilityPolicyConstrained,
			LazySemanticAcquisition: true,
			ExecutorRecipe:          "chat.implement.react_or_htn",
			ParadigmMix:             []string{"react", "htn"},
			TransitionCompatible:    []string{"chat", "debug", "planning"},
			SupportingCapabilities: []string{
				CapabilityChatInspect,
				CapabilityChatDirectEditExecution,
				CapabilityChatLocalReview,
				CapabilityChatTargetedVerification,
			},
			Summary: "Direct coding and implementation with policy-constrained mutation.",
		},
		{
			ID:                   CapabilityChatInspect,
			DisplayName:          "Chat Inspect",
			ModeFamily:           "chat",
			PrimaryCapable:       true,
			Mutability:           MutabilityInspectFirst,
			ExecutorRecipe:       "chat.inspect.react_inspect",
			ParadigmMix:          []string{"react"},
			TransitionCompatible: []string{"chat", "debug"},
			SupportingCapabilities: []string{
				CapabilityChatLocalReview,
			},
			Summary: "Inspect-first code, state, and tool-output examination.",
		},
		{
			ID:                   CapabilityArchaeologyExplore,
			DisplayName:          "Archaeology Explore",
			ModeFamily:           "planning",
			PrimaryCapable:       true,
			Mutability:           MutabilityNonMutating,
			ArchaeoAssociated:    true,
			LLMDependent:         true,
			ArchaeoOperation:     "explore",
			ExecutorRecipe:       "archaeology.explore.planner_research",
			ParadigmMix:          []string{"planner", "reflection"},
			TransitionCompatible: []string{"planning", "debug"},
			SupportingCapabilities: []string{
				CapabilityArchaeologyPatternSurface,
				CapabilityArchaeologyProspectiveAssess,
				CapabilityArchaeologyConvergenceGuard,
				CapabilityArchaeologyCoherenceAssess,
				CapabilityArchaeologyScopeExpand,
				CapabilityArchaeologyCompilePlan,
			},
			Summary: "Archaeo-backed semantic exploration of the codebase and candidate changes.",
		},
		{
			ID:                   CapabilityArchaeologyCompilePlan,
			DisplayName:          "Archaeology Compile Plan",
			ModeFamily:           "planning",
			PrimaryCapable:       true,
			Mutability:           MutabilityNonMutating,
			ArchaeoAssociated:    true,
			LLMDependent:         true,
			ArchaeoOperation:     "compile_plan",
			ExecutorRecipe:       "archaeology.compile-plan.planner_compile",
			ParadigmMix:          []string{"planner"},
			TransitionCompatible: []string{"planning"},
			SupportingCapabilities: []string{
				CapabilityArchaeologyPatternSurface,
				CapabilityArchaeologyProspectiveAssess,
				CapabilityArchaeologyConvergenceGuard,
				CapabilityArchaeologyCoherenceAssess,
				CapabilityArchaeologyScopeExpand,
			},
			Summary: "Compile a full executable living plan or emit deferred artifacts.",
		},
		{
			ID:                   CapabilityArchaeologyImplement,
			DisplayName:          "Archaeology Implement Plan",
			ModeFamily:           "planning",
			PrimaryCapable:       true,
			Mutability:           MutabilityPlanBoundExecution,
			ArchaeoAssociated:    true,
			LLMDependent:         true,
			ArchaeoOperation:     "implement_plan",
			ExecutorRecipe:       "archaeology.implement-plan.rewoo_execution",
			ParadigmMix:          []string{"rewoo", "planner"},
			TransitionCompatible: []string{"planning", "chat"},
			SupportingCapabilities: []string{
				CapabilityArchaeologyCompilePlan,
				CapabilityArchaeologyExplore,
				CapabilityArchaeologyConvergenceGuard,
				CapabilityArchaeologyCoherenceAssess,
			},
			Summary: "Execute against a compiled living plan under Euclo's single-plan run guarantees.",
		},
		{
			ID:                   CapabilityDebugInvestigate,
			DisplayName:          "Debug Investigate",
			ModeFamily:           "debug",
			PrimaryCapable:       true,
			Mutability:           MutabilityInspectFirst,
			ExecutorRecipe:       "debug.investigate.htn_drilldown",
			ParadigmMix:          []string{"htn", "react"},
			TransitionCompatible: []string{"debug", "chat"},
			SupportingCapabilities: []string{
				CapabilityDebugRootCause,
				CapabilityDebugHypothesisRefine,
				CapabilityDebugLocalization,
				CapabilityDebugFlawSurface,
				CapabilityDebugVerificationRepair,
				CapabilityChatInspect,
			},
			Summary: "Mixed debugging behavior with tool exposition and controlled implementation escalation.",
		},
		{
			ID:             CapabilityChatDirectEditExecution,
			DisplayName:    "Chat Direct Edit Execution",
			ModeFamily:     "chat",
			SupportingOnly: true,
			Mutability:     MutabilityPolicyConstrained,
			ExecutorRecipe: "chat.direct-edit-execution.react_support",
			ParadigmMix:    []string{"react"},
			Summary:        "Direct code editing and patch execution support under chat.implement ownership.",
		},
		{
			ID:             CapabilityChatLocalReview,
			DisplayName:    "Chat Local Review",
			ModeFamily:     "chat",
			SupportingOnly: true,
			Mutability:     MutabilityInspectFirst,
			ExecutorRecipe: "chat.local-review.reflection_support",
			ParadigmMix:    []string{"reflection"},
			Summary:        "Local code and artifact review without taking over execution ownership.",
		},
		{
			ID:             CapabilityChatTargetedVerification,
			DisplayName:    "Chat Targeted Verification Repair",
			ModeFamily:     "chat",
			SupportingOnly: true,
			Mutability:     MutabilityPolicyConstrained,
			ExecutorRecipe: "chat.targeted-verification.htn_support",
			ParadigmMix:    []string{"htn", "react"},
			Summary:        "Targeted verification and bounded repair support for direct coding work.",
		},
		{
			ID:                CapabilityArchaeologyPatternSurface,
			DisplayName:       "Archaeology Pattern Surface",
			ModeFamily:        "planning",
			SupportingOnly:    true,
			Mutability:        MutabilityNonMutating,
			ArchaeoAssociated: true,
			LLMDependent:      true,
			ArchaeoOperation:  "pattern_surface",
			ExecutorRecipe:    "archaeology.pattern-surface.semantic_analysis",
			ParadigmMix:       []string{"planner"},
			Summary:           "Surface codebase patterns and pattern-bearing relationships.",
		},
		{
			ID:                CapabilityArchaeologyProspectiveAssess,
			DisplayName:       "Archaeology Prospective Assess",
			ModeFamily:        "planning",
			SupportingOnly:    true,
			Mutability:        MutabilityNonMutating,
			ArchaeoAssociated: true,
			LLMDependent:      true,
			ArchaeoOperation:  "prospective_assess",
			ExecutorRecipe:    "archaeology.prospective-assess.semantic_analysis",
			ParadigmMix:       []string{"planner", "reflection"},
			Summary:           "Assess prospective structures and plausible engineering directions.",
		},
		{
			ID:                CapabilityArchaeologyConvergenceGuard,
			DisplayName:       "Archaeology Convergence Guard",
			ModeFamily:        "planning",
			SupportingOnly:    true,
			Mutability:        MutabilityNonMutating,
			ArchaeoAssociated: true,
			LLMDependent:      true,
			ArchaeoOperation:  "convergence_guard",
			ExecutorRecipe:    "archaeology.convergence-guard.semantic_analysis",
			ParadigmMix:       []string{"planner", "reflection"},
			Summary:           "Protect convergence and highlight divergence pressure in planning.",
		},
		{
			ID:                CapabilityArchaeologyCoherenceAssess,
			DisplayName:       "Archaeology Coherence Assess",
			ModeFamily:        "planning",
			SupportingOnly:    true,
			Mutability:        MutabilityNonMutating,
			ArchaeoAssociated: true,
			LLMDependent:      true,
			ArchaeoOperation:  "coherence_assess",
			ExecutorRecipe:    "archaeology.coherence-assess.semantic_analysis",
			ParadigmMix:       []string{"reflection"},
			Summary:           "Check coherence across explored semantics and proposed changes.",
		},
		{
			ID:                CapabilityArchaeologyScopeExpand,
			DisplayName:       "Archaeology Scope Expansion Assess",
			ModeFamily:        "planning",
			SupportingOnly:    true,
			Mutability:        MutabilityNonMutating,
			ArchaeoAssociated: true,
			LLMDependent:      true,
			ArchaeoOperation:  "scope_expansion_assess",
			ExecutorRecipe:    "archaeology.scope-expansion.semantic_analysis",
			ParadigmMix:       []string{"planner"},
			Summary:           "Detect and report scope widening during planning and compilation.",
		},
		{
			ID:             CapabilityDebugRootCause,
			DisplayName:    "Debug Root Cause",
			ModeFamily:     "debug",
			SupportingOnly: true,
			Mutability:     MutabilityInspectFirst,
			ExecutorRecipe: "debug.root-cause.htn_support",
			ParadigmMix:    []string{"htn"},
			Summary:        "Drive root-cause investigation from evidence and tool output.",
		},
		{
			ID:             CapabilityDebugHypothesisRefine,
			DisplayName:    "Debug Hypothesis Refine",
			ModeFamily:     "debug",
			SupportingOnly: true,
			Mutability:     MutabilityInspectFirst,
			ExecutorRecipe: "debug.hypothesis-refine.reflective_support",
			ParadigmMix:    []string{"reflection"},
			Summary:        "Refine defect hypotheses from gathered evidence.",
		},
		{
			ID:             CapabilityDebugLocalization,
			DisplayName:    "Debug Localization",
			ModeFamily:     "debug",
			SupportingOnly: true,
			Mutability:     MutabilityInspectFirst,
			ExecutorRecipe: "debug.localization.htn_support",
			ParadigmMix:    []string{"htn", "react"},
			Summary:        "Localize faults through bounded drilling into code and execution data.",
		},
		{
			ID:             CapabilityDebugFlawSurface,
			DisplayName:    "Debug Flaw Surface",
			ModeFamily:     "debug",
			SupportingOnly: true,
			Mutability:     MutabilityInspectFirst,
			ExecutorRecipe: "debug.flaw-surface.reflective_support",
			ParadigmMix:    []string{"reflection"},
			Summary:        "Surface flaws, smells, anti-patterns, and vulnerabilities during investigation.",
		},
		{
			ID:             CapabilityDebugVerificationRepair,
			DisplayName:    "Debug Verification Repair",
			ModeFamily:     "debug",
			SupportingOnly: true,
			Mutability:     MutabilityPolicyConstrained,
			ExecutorRecipe: "debug.verification-repair.htn_support",
			ParadigmMix:    []string{"htn", "react"},
			Summary:        "Support bounded verification and repair before escalation to implementation.",
		},
	} {
		_ = r.Register(desc)
	}
	return r
}

func (r *Registry) Register(desc Descriptor) error {
	if r == nil || desc.ID == "" {
		return nil
	}
	if r.descriptors == nil {
		r.descriptors = map[string]Descriptor{}
	}
	r.descriptors[desc.ID] = desc
	return nil
}

func (r *Registry) Lookup(id string) (Descriptor, bool) {
	if r == nil {
		return Descriptor{}, false
	}
	desc, ok := r.descriptors[id]
	return desc, ok
}

func (r *Registry) IDsForMode(modeFamily string) []string {
	if r == nil {
		return nil
	}
	var out []string
	for _, desc := range r.descriptors {
		if desc.ModeFamily == modeFamily {
			out = append(out, desc.ID)
		}
	}
	sort.Strings(out)
	return out
}

func (r *Registry) SupportingForPrimary(id string) []string {
	if r == nil {
		return nil
	}
	desc, ok := r.Lookup(id)
	if !ok {
		return nil
	}
	out := append([]string(nil), desc.SupportingCapabilities...)
	sort.Strings(out)
	return out
}
