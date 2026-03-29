package local

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

type executionProfileSelectCapability struct {
	env agentenv.AgentEnvironment
}

func NewExecutionProfileSelectCapability(env agentenv.AgentEnvironment) euclotypes.EucloCodingCapability {
	return &executionProfileSelectCapability{env: env}
}

func (c *executionProfileSelectCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:execution_profile.select",
		Name:          "Execution Profile Select",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "planning", "routing"},
		Annotations: map[string]any{
			"supported_profiles": []string{
				"edit_verify_repair",
				"reproduce_localize_patch",
				"test_driven_generation",
				"review_suggest_implement",
				"plan_stage_execute",
				"trace_execute_analyze",
			},
		},
	}
}

func (c *executionProfileSelectCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{
		RequiredInputs: []euclotypes.ArtifactRequirement{{Kind: euclotypes.ArtifactKindIntake, Required: true}},
		ProducedOutputs: []euclotypes.ArtifactKind{
			euclotypes.ArtifactKindProfileSelection,
		},
	}
}

func (c *executionProfileSelectCapability) Eligible(artifacts euclotypes.ArtifactState, _ euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if !looksLikeProfileSelectionRequest(artifacts) {
		return euclotypes.EligibilityResult{Eligible: false, Reason: "profile selection requires an explicit approach/profile question"}
	}
	return euclotypes.EligibilityResult{Eligible: true, Reason: "explicit profile-selection request"}
}

func (c *executionProfileSelectCapability) Execute(_ context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	envelope := eucloruntime.NormalizeTaskEnvelope(env.Task, env.State, env.Registry)
	classification := eucloruntime.ClassifyTaskScored(envelope)
	modeRegistry := euclotypes.DefaultModeRegistry()
	mode := eucloruntime.ResolveMode(envelope, classification.TaskClassification, modeRegistry)
	profileRegistry := euclotypes.DefaultExecutionProfileRegistry()
	selection := eucloruntime.SelectExecutionProfile(envelope, classification.TaskClassification, mode, profileRegistry)
	reasoning := fmt.Sprintf("selected %s for mode=%s scope=%s risk=%s", selection.ProfileID, mode.ModeID, classification.Scope, classification.RiskLevel)
	confidence := profileConfidence(classification, selection)
	payload := map[string]any{
		"selected_profile": selection.ProfileID,
		"confidence":       confidence,
		"reasoning":        reasoning,
		"fallback_profile": firstFallback(selection),
		"signals_used":     classification.ReasonCodes,
		"mode":             mode.ModeID,
	}
	artifact := euclotypes.Artifact{
		ID:         "profile_selection",
		Kind:       euclotypes.ArtifactKindProfileSelection,
		Summary:    summarizePayload(payload),
		Payload:    payload,
		ProducerID: "euclo:execution_profile.select",
		Status:     "produced",
	}
	mergeStateArtifactsToContext(env.State, []euclotypes.Artifact{artifact})
	return euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusCompleted, Summary: "execution profile selected", Artifacts: []euclotypes.Artifact{artifact}}
}

func looksLikeProfileSelectionRequest(artifacts euclotypes.ArtifactState) bool {
	text := strings.ToLower(strings.TrimSpace(instructionFromArtifacts(artifacts)))
	for _, token := range []string{"what profile", "which profile", "which approach", "what approach", "execution profile"} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func profileConfidence(classification eucloruntime.ScoredClassification, selection euclotypes.ExecutionProfileSelection) float64 {
	confidence := classification.Confidence
	if confidence <= 0 {
		confidence = 0.5
	}
	if selection.ProfileID == "edit_verify_repair" || selection.ProfileID == "plan_stage_execute" {
		confidence += 0.1
	}
	if confidence > 0.99 {
		confidence = 0.99
	}
	return confidence
}

func firstFallback(selection euclotypes.ExecutionProfileSelection) string {
	if len(selection.FallbackProfileIDs) == 0 {
		return ""
	}
	return selection.FallbackProfileIDs[0]
}
