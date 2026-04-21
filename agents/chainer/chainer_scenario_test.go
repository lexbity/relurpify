//go:build scenario

package chainer_test

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/agents/chainer"
	"codeburg.org/lexbit/relurpify/framework/core"
	agenttestscenario "codeburg.org/lexbit/relurpify/testutil/agenttestscenario"
	testutil "codeburg.org/lexbit/relurpify/testutil/euclotestutil"
)

func TestChainerAgent_Scenario_MultiLink_StateFlowsForward(t *testing.T) {
	f := agenttestscenario.NewFixture(t,
		testutil.Turn("chat").
			ExpectingPromptFragment("first step").
			Responding("v1").
			Build(),
		testutil.Turn("chat").
			ExpectingPromptFragment("v1").
			Responding("v2").
			Build(),
	)

	agent := chainer.New(f.Env, chainer.WithChain(&chainer.Chain{Links: []chainer.Link{
		chainer.NewLink("one", "first step {{.Instruction}}", nil, "out.one", nil),
		chainer.NewLink("two", "second step {{index .Input \"out.one\"}}", []string{"out.one"}, "out.two", nil),
	}}))

	state := core.NewContext()
	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "chainer-scenario-state-flow",
		Instruction: "implement feature",
	}, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	agenttestscenario.RequireResultSuccess(t, result)
	agenttestscenario.RequireContextKey(t, state, "out.one", "v1")
	agenttestscenario.RequireContextKey(t, state, "out.two", "v2")
	agenttestscenario.RequireModelExhausted(t, f)
}
