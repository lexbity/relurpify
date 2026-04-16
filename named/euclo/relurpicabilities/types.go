package relurpicabilities

import (
	"sort"
	"strings"
)

const (
	CapabilityChatAsk                = "euclo:chat.ask"
	CapabilityChatImplement          = "euclo:chat.implement"
	CapabilityChatInspect            = "euclo:chat.inspect"
	CapabilityArchaeologyExplore     = "euclo:archaeology.explore"
	CapabilityArchaeologyCompilePlan = "euclo:archaeology.compile-plan"
	CapabilityArchaeologyImplement   = "euclo:archaeology.implement-plan"
	CapabilityDebugInvestigateRepair = "euclo:debug.investigate-repair"
	CapabilityDebugRepairSimple      = "euclo:debug.repair.simple"
	CapabilityBKCCompile             = "euclo:bkc.compile"
	CapabilityBKCStream              = "euclo:bkc.stream"
	CapabilityBKCCheckpoint          = "euclo:bkc.checkpoint"
	CapabilityBKCInvalidate          = "euclo:bkc.invalidate"

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
	ModeFamilies            []string           `json:"mode_families,omitempty"`    // ordered; first entry is primary mode
	TriggerPriority         int                `json:"trigger_priority,omitempty"` // higher = considered first during keyword tie-breaking
	PrimaryCapable          bool               `json:"primary_capable"`
	SupportingOnly          bool               `json:"supporting_only"`
	Mutability              MutabilityContract `json:"mutability,omitempty"`
	ArchaeoAssociated       bool               `json:"archaeo_associated"`
	LazySemanticAcquisition bool               `json:"lazy_semantic_acquisition"`
	LLMDependent            bool               `json:"llm_dependent"`
	ArchaeoOperation        string             `json:"archaeo_operation,omitempty"`
	ParadigmMix             []string           `json:"paradigm_mix,omitempty"`
	TransitionCompatible    []string           `json:"transition_compatible,omitempty"`
	SupportingCapabilities  []string           `json:"supporting_capabilities,omitempty"`
	Summary                 string             `json:"summary,omitempty"`
	Keywords                []string           `json:"keywords,omitempty"` // Tier-1 static match terms; case-insensitive substring
	DefaultForMode          bool               `json:"default_for_mode"`   // Tier-3 fallback when no other match within ModeFamily
}

// PrimaryMode returns the primary (first) mode family for backward compatibility.
func (d Descriptor) PrimaryMode() string {
	if len(d.ModeFamilies) == 0 {
		return ""
	}
	return d.ModeFamilies[0]
}

type Registry struct {
	descriptors map[string]Descriptor
}

// containsString checks if a string slice contains a specific value.
func containsString(slice []string, value string) bool {
	for _, s := range slice {
		if s == value {
			return true
		}
	}
	return false
}

// KeywordMatch represents a capability descriptor that matched keywords in an instruction.
type KeywordMatch struct {
	Descriptor
	MatchCount      int
	MatchedKeywords []string
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
			ModeFamilies:         []string{"chat"},
			PrimaryCapable:       true,
			Mutability:           MutabilityNonMutating,
			ParadigmMix:          []string{"react"},
			TransitionCompatible: []string{"chat", "debug"},
			Summary:              "Non-mutating engineering question answering and explanation.",
			Keywords:             []string{"explain", "what is", "what does", "how does", "describe", "tell me", "walk me through", "help me understand", "what are", "how is"},
			DefaultForMode:       true,
		},
		{
			ID:                      CapabilityChatImplement,
			DisplayName:             "Chat Implement",
			ModeFamilies:            []string{"chat"},
			PrimaryCapable:          true,
			Mutability:              MutabilityPolicyConstrained,
			LazySemanticAcquisition: true,
			ParadigmMix:             []string{"react", "architect"},
			TransitionCompatible:    []string{"chat", "debug", "planning"},
			SupportingCapabilities: []string{
				CapabilityChatDirectEditExecution,
				CapabilityChatLocalReview,
				CapabilityChatTargetedVerification,
			},
			Summary:  "Direct coding and implementation with policy-constrained mutation.",
			Keywords: []string{"implement", "fix", "refactor", "change", "update", "add", "create", "patch", "rewrite", "make"},
		},
		{
			ID:                   CapabilityChatInspect,
			DisplayName:          "Chat Inspect",
			ModeFamilies:         []string{"chat"},
			PrimaryCapable:       true,
			Mutability:           MutabilityInspectFirst,
			ParadigmMix:          []string{"react"},
			TransitionCompatible: []string{"chat", "debug"},
			SupportingCapabilities: []string{
				CapabilityChatLocalReview,
			},
			Summary:  "Inspect-first code, state, and tool-output examination.",
			Keywords: []string{"compare", "contrast", "inspect", "examine", "evaluate", "assess", "look at", "analyze", "analyse", "review", "surface"},
			// Note: No DefaultForMode - review mode uses explicit routing in workunit.go
		},
		{
			ID:                   CapabilityArchaeologyExplore,
			DisplayName:          "Archaeology Explore",
			ModeFamilies:         []string{"planning"},
			PrimaryCapable:       true,
			Mutability:           MutabilityNonMutating,
			ArchaeoAssociated:    true,
			LLMDependent:         true,
			ArchaeoOperation:     "explore",
			ParadigmMix:          []string{"planner", "reflection"},
			TransitionCompatible: []string{"planning", "debug"},
			SupportingCapabilities: []string{
				CapabilityArchaeologyPatternSurface,
				CapabilityArchaeologyProspectiveAssess,
				CapabilityArchaeologyConvergenceGuard,
				CapabilityArchaeologyCoherenceAssess,
				CapabilityArchaeologyScopeExpand,
			},
			Summary:        "Archaeo-backed semantic exploration of the codebase and candidate changes.",
			Keywords:       []string{"explore", "identify patterns", "surface patterns", "find patterns", "inspect the codebase", "scan for", "survey"},
			DefaultForMode: true,
		},
		{
			ID:                   CapabilityArchaeologyCompilePlan,
			DisplayName:          "Archaeology Compile Plan",
			ModeFamilies:         []string{"planning"},
			PrimaryCapable:       true,
			Mutability:           MutabilityNonMutating,
			ArchaeoAssociated:    true,
			LLMDependent:         true,
			ArchaeoOperation:     "compile_plan",
			ParadigmMix:          []string{"planner"},
			TransitionCompatible: []string{"planning"},
			SupportingCapabilities: []string{
				CapabilityArchaeologyPatternSurface,
				CapabilityArchaeologyProspectiveAssess,
				CapabilityArchaeologyConvergenceGuard,
				CapabilityArchaeologyCoherenceAssess,
				CapabilityArchaeologyScopeExpand,
			},
			Summary:  "Compile a full executable living plan or emit deferred artifacts.",
			Keywords: []string{"compile the plan", "create the plan", "generate the plan", "produce the plan", "write the plan", "finalize the plan"},
		},
		{
			ID:                   CapabilityArchaeologyImplement,
			DisplayName:          "Archaeology Implement Plan",
			ModeFamilies:         []string{"planning"},
			PrimaryCapable:       true,
			Mutability:           MutabilityPlanBoundExecution,
			ArchaeoAssociated:    true,
			LLMDependent:         true,
			ArchaeoOperation:     "implement_plan",
			ParadigmMix:          []string{"rewoo", "planner"},
			TransitionCompatible: []string{"planning", "chat"},
			SupportingCapabilities: []string{
				CapabilityArchaeologyConvergenceGuard,
				CapabilityArchaeologyCoherenceAssess,
			},
			Summary:  "Execute against a compiled living plan under Euclo's single-plan run guarantees.",
			Keywords: []string{"implement the plan", "execute the plan", "carry out", "apply the plan", "execute the compiled plan", "run the plan"},
		},
		{
			ID:                   CapabilityDebugInvestigateRepair,
			DisplayName:          "Debug Investigate-Repair",
			ModeFamilies:         []string{"debug"},
			PrimaryCapable:       true,
			Mutability:           MutabilityInspectFirst,
			ParadigmMix:          []string{"blackboard", "react", "reflection"},
			TransitionCompatible: []string{"debug", "chat"},
			SupportingCapabilities: []string{
				CapabilityDebugRootCause,
				CapabilityDebugHypothesisRefine,
				CapabilityDebugLocalization,
				CapabilityDebugFlawSurface,
				CapabilityDebugVerificationRepair,
			},
			Summary:        "Hypothesis-driven debugging with integrated verification and repair using blackboard architecture.",
			Keywords:       []string{"investigate", "root cause", "diagnose", "trace", "identify the", "identify", "localize", "why does", "why is", "what caused", "find the bug", "debug"},
			DefaultForMode: true,
		},
		{
			ID:                CapabilityBKCCompile,
			DisplayName:       "BKC Compile",
			ModeFamilies:      []string{"planning"},
			SupportingOnly:    true,
			Mutability:        MutabilityNonMutating,
			ArchaeoAssociated: true,
			LLMDependent:      true,
			ArchaeoOperation:  "bkc_compile",
			ParadigmMix:       []string{"planner", "reflection"},
			Summary:           "Compile an LLM-assisted BKC candidate and queue it for archaeology confirmation.",
		},
		{
			ID:                CapabilityBKCStream,
			DisplayName:       "BKC Stream",
			ModeFamilies:      []string{"planning"},
			SupportingOnly:    true,
			Mutability:        MutabilityNonMutating,
			ArchaeoAssociated: true,
			ArchaeoOperation:  "bkc_stream",
			ParadigmMix:       []string{"planner"},
			Summary:           "Stream chunk-backed semantic context into Euclo runtime state.",
		},
		{
			ID:                CapabilityBKCCheckpoint,
			DisplayName:       "BKC Checkpoint",
			ModeFamilies:      []string{"planning"},
			SupportingOnly:    true,
			Mutability:        MutabilityPolicyConstrained,
			ArchaeoAssociated: true,
			LLMDependent:      true,
			ArchaeoOperation:  "bkc_checkpoint",
			ParadigmMix:       []string{"planner", "reflection"},
			Summary:           "Anchor chunk roots to the active living plan version.",
		},
		{
			ID:                CapabilityBKCInvalidate,
			DisplayName:       "BKC Invalidate",
			ModeFamilies:      []string{"planning"},
			SupportingOnly:    true,
			Mutability:        MutabilityNonMutating,
			ArchaeoAssociated: true,
			ArchaeoOperation:  "bkc_invalidate",
			ParadigmMix:       []string{"planner", "reflection"},
			Summary:           "Surface stale BKC chunks and tensions after revision drift.",
		},
		{
			ID:             CapabilityChatDirectEditExecution,
			DisplayName:    "Chat Direct Edit Execution",
			ModeFamilies:   []string{"chat"},
			SupportingOnly: true,
			Mutability:     MutabilityPolicyConstrained,
			ParadigmMix:    []string{"react"},
			Summary:        "Direct code editing and patch execution support under chat.implement ownership.",
		},
		{
			ID:             CapabilityChatLocalReview,
			DisplayName:    "Chat Local Review",
			ModeFamilies:   []string{"chat"},
			SupportingOnly: true,
			Mutability:     MutabilityInspectFirst,
			ParadigmMix:    []string{"reflection"},
			Summary:        "Local code and artifact review without taking over execution ownership.",
		},
		{
			ID:             CapabilityChatTargetedVerification,
			DisplayName:    "Chat Targeted Verification Repair",
			ModeFamilies:   []string{"chat"},
			SupportingOnly: true,
			Mutability:     MutabilityPolicyConstrained,
			ParadigmMix:    []string{"htn", "react"},
			Summary:        "Targeted verification and bounded repair support for direct coding work.",
		},
		{
			ID:                CapabilityArchaeologyPatternSurface,
			DisplayName:       "Archaeology Pattern Surface",
			ModeFamilies:      []string{"planning"},
			SupportingOnly:    true,
			Mutability:        MutabilityNonMutating,
			ArchaeoAssociated: true,
			LLMDependent:      true,
			ArchaeoOperation:  "pattern_surface",
			ParadigmMix:       []string{"planner"},
			Summary:           "Surface codebase patterns and pattern-bearing relationships.",
		},
		{
			ID:                CapabilityArchaeologyProspectiveAssess,
			DisplayName:       "Archaeology Prospective Assess",
			ModeFamilies:      []string{"planning"},
			SupportingOnly:    true,
			Mutability:        MutabilityNonMutating,
			ArchaeoAssociated: true,
			LLMDependent:      true,
			ArchaeoOperation:  "prospective_assess",
			ParadigmMix:       []string{"planner", "reflection"},
			Summary:           "Assess prospective structures and plausible engineering directions.",
		},
		{
			ID:                CapabilityArchaeologyConvergenceGuard,
			DisplayName:       "Archaeology Convergence Guard",
			ModeFamilies:      []string{"planning"},
			SupportingOnly:    true,
			Mutability:        MutabilityNonMutating,
			ArchaeoAssociated: true,
			LLMDependent:      true,
			ArchaeoOperation:  "convergence_guard",
			ParadigmMix:       []string{"planner", "reflection"},
			Summary:           "Protect convergence and highlight divergence pressure in planning.",
		},
		{
			ID:                CapabilityArchaeologyCoherenceAssess,
			DisplayName:       "Archaeology Coherence Assess",
			ModeFamilies:      []string{"planning"},
			SupportingOnly:    true,
			Mutability:        MutabilityNonMutating,
			ArchaeoAssociated: true,
			LLMDependent:      true,
			ArchaeoOperation:  "coherence_assess",
			ParadigmMix:       []string{"reflection"},
			Summary:           "Check coherence across explored semantics and proposed changes.",
		},
		{
			ID:                CapabilityArchaeologyScopeExpand,
			DisplayName:       "Archaeology Scope Expansion Assess",
			ModeFamilies:      []string{"planning"},
			SupportingOnly:    true,
			Mutability:        MutabilityNonMutating,
			ArchaeoAssociated: true,
			LLMDependent:      true,
			ArchaeoOperation:  "scope_expansion_assess",
			ParadigmMix:       []string{"planner"},
			Summary:           "Detect and report scope widening during planning and compilation.",
		},
		{
			ID:             CapabilityDebugRootCause,
			DisplayName:    "Debug Root Cause",
			ModeFamilies:   []string{"debug"},
			SupportingOnly: true,
			Mutability:     MutabilityInspectFirst,
			ParadigmMix:    []string{"htn"},
			Summary:        "Drive root-cause investigation from evidence and tool output.",
		},
		{
			ID:             CapabilityDebugHypothesisRefine,
			DisplayName:    "Debug Hypothesis Refine",
			ModeFamilies:   []string{"debug"},
			SupportingOnly: true,
			Mutability:     MutabilityInspectFirst,
			ParadigmMix:    []string{"reflection"},
			Summary:        "Refine defect hypotheses from gathered evidence.",
		},
		{
			ID:             CapabilityDebugLocalization,
			DisplayName:    "Debug Localization",
			ModeFamilies:   []string{"debug"},
			SupportingOnly: true,
			Mutability:     MutabilityInspectFirst,
			ParadigmMix:    []string{"htn", "react"},
			Summary:        "Localize faults through bounded drilling into code and execution data.",
		},
		{
			ID:             CapabilityDebugFlawSurface,
			DisplayName:    "Debug Flaw Surface",
			ModeFamilies:   []string{"debug"},
			SupportingOnly: true,
			Mutability:     MutabilityInspectFirst,
			ParadigmMix:    []string{"reflection"},
			Summary:        "Surface flaws, smells, anti-patterns, and vulnerabilities during investigation.",
		},
		{
			ID:             CapabilityDebugVerificationRepair,
			DisplayName:    "Debug Verification Repair",
			ModeFamilies:   []string{"debug"},
			SupportingOnly: true,
			Mutability:     MutabilityPolicyConstrained,
			ParadigmMix:    []string{"htn", "react"},
			Summary:        "Support bounded verification and repair before escalation to implementation.",
		},
		{
			ID:                   CapabilityDebugRepairSimple,
			DisplayName:          "Debug Repair Simple",
			ModeFamilies:         []string{"debug"},
			PrimaryCapable:       true,
			Mutability:           MutabilityPolicyConstrained,
			ParadigmMix:          []string{"react"},
			TransitionCompatible: []string{"debug", "chat"},
			SupportingCapabilities: []string{
				CapabilityDebugFlawSurface,
			},
			Summary:  "Direct read-patch-verify repair for well-understood or straightforward defects where root cause is already known or obvious from the defect description.",
			Keywords: []string{"fix this", "fix it", "quick fix", "simple fix", "apply a fix", "patch this", "fix the bug", "correct this", "wrong result", "returns wrong", "subtracts instead", "adds instead", "off by one", "off-by-one"},
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
		if containsString(desc.ModeFamilies, modeFamily) {
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

// IDs returns all registered capability IDs.
// Phase 5: Used by coverage gap detector.
func (r *Registry) IDs() []string {
	if r == nil {
		return nil
	}
	out := make([]string, 0, len(r.descriptors))
	for id := range r.descriptors {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// PrimaryCapabilitiesForMode returns all PrimaryCapable descriptors for modeID, sorted by ID.
func (r *Registry) PrimaryCapabilitiesForMode(modeID string) []Descriptor {
	if r == nil {
		return nil
	}
	var out []Descriptor
	for _, desc := range r.descriptors {
		if containsString(desc.ModeFamilies, modeID) && desc.PrimaryCapable {
			out = append(out, desc)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// FallbackCapabilityForMode returns the DefaultForMode descriptor for modeID if any.
func (r *Registry) FallbackCapabilityForMode(modeID string) (Descriptor, bool) {
	if r == nil {
		return Descriptor{}, false
	}
	for _, desc := range r.descriptors {
		if containsString(desc.ModeFamilies, modeID) && desc.DefaultForMode {
			return desc, true
		}
	}
	return Descriptor{}, false
}

// MatchByKeywords returns all primary-capable descriptors for modeID where at least
// one Keyword (or extra keyword) is a case-insensitive substring of instruction.
// extraKeywords maps capability ID -> additional terms (from manifest config).
func (r *Registry) MatchByKeywords(instruction, modeID string, extraKeywords map[string][]string) []KeywordMatch {
	if r == nil {
		return nil
	}
	lowerInst := strings.ToLower(instruction)
	var matches []KeywordMatch

	for _, desc := range r.descriptors {
		if !desc.PrimaryCapable || !containsString(desc.ModeFamilies, modeID) {
			continue
		}

		// Combine built-in and extra keywords
		allKeywords := append([]string(nil), desc.Keywords...)
		if extra, ok := extraKeywords[desc.ID]; ok {
			allKeywords = append(allKeywords, extra...)
		}

		var matched []string
		for _, kw := range allKeywords {
			if strings.Contains(lowerInst, strings.ToLower(kw)) {
				matched = append(matched, kw)
			}
		}

		if len(matched) > 0 {
			matches = append(matches, KeywordMatch{
				Descriptor:      desc,
				MatchCount:      len(matched),
				MatchedKeywords: matched,
			})
		}
	}

	// Sort by MatchCount descending, then by ID for stability
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].MatchCount != matches[j].MatchCount {
			return matches[i].MatchCount > matches[j].MatchCount
		}
		return matches[i].ID < matches[j].ID
	})

	return matches
}
