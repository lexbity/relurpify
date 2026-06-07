package promptprovider

import (
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/execution/prompt"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
)

func TestThoughtRecipePriorStepProviderGolden(t *testing.T) {
	env := contextdata.NewEnvelope("task-prior", "session-prior")

	frameHistory := []string{"review", "analyze", "execute"}
	euclostate.SetFrameHistory(env, frameHistory)

	jobRecords := []euclostate.JobRecord{
		{
			Type:   "execute",
			Status: "completed",
			ID:     "job-3",
		},
	}
	euclostate.SetJobRecords(env, jobRecords)
	euclostate.SetOutcomeCategory(env, "success")
	euclostate.SetOutcomeArtifacts(env, []string{"result.txt", "summary.md"})
	euclostate.SetNegativeConstraints(env, []string{"avoid_mutation"})

	provider := &thoughtrecipePriorStepProvider{}
	out := provider.Provide(prompt.NewRuntimeContext(env, "react", "thoughtrecipe"))
	assertGolden(t, "recipe_prior_step", out.Content)
}
