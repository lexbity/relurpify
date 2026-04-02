//go:build scenario

package goalcon_test

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/agents/goalcon"
	"github.com/lexcodex/relurpify/framework/core"
	agenttestscenario "github.com/lexcodex/relurpify/testutil/agenttestscenario"
	testutil "github.com/lexcodex/relurpify/testutil/euclotestutil"
)

func TestGoalConAgent_Scenario_ClassifierDisabled_SkipsLLM(t *testing.T) {
	f := agenttestscenario.NewFixture(t)

	agent := goalcon.New(f.Env, goalcon.DefaultOperatorRegistry())
	agent.ClassifierConfig = goalcon.ClassifierConfig{
		Enabled: false,
		Cache:   goalcon.NewGoalCache(8),
	}

	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "goalcon-scenario-skip-llm",
		Type:        core.TaskTypeAnalysis,
		Instruction: "analyze the code",
	}, core.NewContext())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	agenttestscenario.RequireResultSuccess(t, result)
	agenttestscenario.Require(t, len(f.Model.Calls) == 0, "expected no model calls, got %d", len(f.Model.Calls))
}

func TestGoalConAgent_Scenario_ClassifierConsultedOnce(t *testing.T) {
	f := agenttestscenario.NewFixture(t,
		testutil.Turn("generate").
			ExpectingPromptFragment("fix something unclear").
			Responding(`{"predicates":["file_content_known","edit_plan_known","file_modified"],"confidence":0.95,"reasoning":"clear enough"}`).
			Build(),
	)

	agent := goalcon.New(f.Env, goalcon.DefaultOperatorRegistry())
	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "goalcon-scenario-llm",
		Type:        core.TaskTypeCodeModification,
		Instruction: "fix something unclear",
	}, core.NewContext())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	agenttestscenario.RequireResultSuccess(t, result)
	agenttestscenario.RequireModelExhausted(t, f)
}

func TestGoalConAgent_Scenario_OperatorExecution_CallsExecutor(t *testing.T) {
	f := agenttestscenario.NewFixture(t)

	agent := goalcon.New(f.Env, goalcon.DefaultOperatorRegistry())
	agent.ClassifierConfig = goalcon.ClassifierConfig{
		Enabled: false,
		Cache:   goalcon.NewGoalCache(8),
	}
	agent.PlanExecutor = f.Exec

	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "goalcon-scenario-executor",
		Type:        core.TaskTypeCodeModification,
		Instruction: "fix the failing test",
	}, core.NewContext())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	agenttestscenario.RequireResultSuccess(t, result)
	agenttestscenario.Require(t, f.Exec.Calls > 0, "expected executor calls, got %d", f.Exec.Calls)
}
