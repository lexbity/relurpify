package euclo

import (
	"context"

	chainerpkg "github.com/lexcodex/relurpify/agents/chainer"
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
)

// reportFinalCodingCapability produces the final report artifact by composing
// a ChainerAgent with two links: one to gather artifacts and one to compile
// the report summary. This capability is non-mutating.
type reportFinalCodingCapability struct {
	env agentenv.AgentEnvironment
}

func (c *reportFinalCodingCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:report.final_coding",
		Name:          "Final Coding Report",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Tags:          []string{"coding", "reporting"},
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

func (c *reportFinalCodingCapability) Contract() ArtifactContract {
	return ArtifactContract{
		RequiredInputs: []ArtifactRequirement{
			{Kind: ArtifactKindVerification, Required: false},
			{Kind: ArtifactKindEditExecution, Required: false},
			{Kind: ArtifactKindPlan, Required: false},
		},
		ProducedOutputs: []ArtifactKind{
			ArtifactKindFinalReport,
		},
	}
}

func (c *reportFinalCodingCapability) Eligible(artifacts ArtifactState, _ CapabilitySnapshot) EligibilityResult {
	if artifacts.Has(ArtifactKindVerification) || artifacts.Has(ArtifactKindEditExecution) || artifacts.Has(ArtifactKindPlan) {
		return EligibilityResult{Eligible: true, Reason: "reportable artifacts present"}
	}
	return EligibilityResult{
		Eligible: false,
		Reason:   "no reportable artifacts available",
	}
}

func (c *reportFinalCodingCapability) Execute(ctx context.Context, env ExecutionEnvelope) ExecutionResult {
	producerID := "euclo:report.final_coding"

	chain := &chainerpkg.Chain{
		Links: []chainerpkg.Link{
			chainerpkg.NewSummarizeLink("gather", nil, "report.gathered"),
			chainerpkg.NewSummarizeLink("compile", []string{"report.gathered"}, "report.final"),
		},
	}

	reportState := env.State.Clone()
	agent := chainerpkg.New(env.Environment, chainerpkg.WithChain(chain))
	task := &core.Task{
		ID:          "report-final",
		Instruction: "Compile a final report summarizing the artifacts produced during execution.",
		Type:        core.TaskTypeAnalysis,
		Context:     taskContextFrom(env),
	}

	result, err := agent.Execute(ctx, task, reportState)
	if err != nil || result == nil || !result.Success {
		return ExecutionResult{
			Status:  ExecutionStatusFailed,
			Summary: "report generation failed",
			FailureInfo: &CapabilityFailure{
				Code:         "report_failed",
				Message:      errMsg(err, result),
				Recoverable:  false,
				FailedPhase:  "report",
				ParadigmUsed: "chainer",
			},
		}
	}

	return ExecutionResult{
		Status:  ExecutionStatusCompleted,
		Summary: "final report compiled",
		Artifacts: []Artifact{
			{
				ID:         "final_report",
				Kind:       ArtifactKindFinalReport,
				Summary:    resultSummary(result),
				Payload:    result.Data,
				ProducerID: producerID,
				Status:     "produced",
			},
		},
	}
}
