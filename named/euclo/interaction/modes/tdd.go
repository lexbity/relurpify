package modes

import (
	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// TDDMode builds the phase machine for the TDD interaction mode.
//
// Phases: specify → test_draft → review_tests → implement → green
func TDDMode(emitter interaction.FrameEmitter, resolver *interaction.AgencyResolver) *interaction.PhaseMachine {
	RegisterTDDTriggers(resolver)

	return interaction.NewPhaseMachine(interaction.PhaseMachineConfig{
		Mode:     "tdd",
		Emitter:  emitter,
		Resolver: resolver,
		Phases: []interaction.PhaseDefinition{
			{
				ID:        "specify",
				Label:     "Specify",
				Handler:   &BehaviorSpecPhase{},
				Skippable: true,
				SkipWhen:  skipSpecify,
			},
			{
				ID:      "test_draft",
				Label:   "Test Draft",
				Handler: &TestDraftPhase{},
			},
			{
				ID:      "review_tests",
				Label:   "Review Tests",
				Handler: &TestResultPhase{},
			},
			{
				ID:      "implement",
				Label:   "Implement",
				Handler: &CodeExecutionPhase{},
			},
			{
				ID:      "green",
				Label:   "Green",
				Handler: &GreenStatusPhase{},
			},
		},
	})
}

// skipSpecify skips when instruction already specifies exact test cases.
func skipSpecify(state map[string]any, _ *interaction.ArtifactBundle) bool {
	if v, _ := state["has_test_specs"].(bool); v {
		return true
	}
	return false
}

// RegisterTDDTriggers registers agency triggers for the TDD mode.
func RegisterTDDTriggers(resolver *interaction.AgencyResolver) {
	if resolver == nil {
		return
	}
	resolver.RegisterTrigger("tdd", interaction.AgencyTrigger{
		Phrases:     []string{"refactor", "refactor this"},
		Description: "Transition to code mode with test-green constraint",
	})
	resolver.RegisterTrigger("tdd", interaction.AgencyTrigger{
		Phrases:     []string{"add more tests", "more tests"},
		PhaseJump:   "specify",
		Description: "Add more test cases to the specification",
	})
	resolver.RegisterTrigger("tdd", interaction.AgencyTrigger{
		Phrases:     []string{"show matrix", "show specs"},
		Description: "Display accumulated behavior specifications",
	})
}

// TDDPhaseIDs returns the ordered phase IDs for TDD mode.
func TDDPhaseIDs() []string {
	return []string{"specify", "test_draft", "review_tests", "implement", "green"}
}

// TDDPhaseLabels returns phase labels for the help surface.
func TDDPhaseLabels() []interaction.PhaseInfo {
	ids := TDDPhaseIDs()
	labels := []string{"Specify", "Test Draft", "Review Tests", "Implement", "Green"}
	out := make([]interaction.PhaseInfo, len(ids))
	for i := range ids {
		out[i] = interaction.PhaseInfo{ID: ids[i], Label: labels[i]}
	}
	return out
}

// BehaviorSpec accumulates behavior specifications across question rounds.
type BehaviorSpec struct {
	FunctionTarget string         `json:"function_target"`
	HappyPaths     []BehaviorCase `json:"happy_paths,omitempty"`
	EdgeCases      []BehaviorCase `json:"edge_cases,omitempty"`
	ErrorCases     []BehaviorCase `json:"error_cases,omitempty"`
}

// BehaviorCase is a single test case specification.
type BehaviorCase struct {
	Description string `json:"description"`
	Input       string `json:"input,omitempty"`
	Expected    string `json:"expected,omitempty"`
}

// AllCases returns all behavior cases across all categories.
func (s *BehaviorSpec) AllCases() []BehaviorCase {
	out := make([]BehaviorCase, 0, len(s.HappyPaths)+len(s.EdgeCases)+len(s.ErrorCases))
	out = append(out, s.HappyPaths...)
	out = append(out, s.EdgeCases...)
	out = append(out, s.ErrorCases...)
	return out
}

// TotalCases returns the total number of behavior cases.
func (s *BehaviorSpec) TotalCases() int {
	return len(s.HappyPaths) + len(s.EdgeCases) + len(s.ErrorCases)
}
