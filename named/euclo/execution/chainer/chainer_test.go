package chainer

import (
	"context"
	"testing"

	chainerpkg "github.com/lexcodex/relurpify/agents/chainer"
	"github.com/lexcodex/relurpify/framework/core"
	testutil "github.com/lexcodex/relurpify/testutil/euclotestutil"
)

func TestNewReturnsConfiguredRunner(t *testing.T) {
	env := testutil.Env(t)

	runner := New(env)
	if runner == nil {
		t.Fatal("expected runner")
	}
	if runner.Model != env.Model {
		t.Fatal("expected model to be wired from environment")
	}
	if runner.Tools != env.Registry {
		t.Fatal("expected registry to be wired from environment")
	}
}

func TestExecuteChainRunsProvidedChain(t *testing.T) {
	model := testutil.NewScenarioStubModel(
		testutil.Turn("chat").
			Responding("v1").
			Build(),
		testutil.Turn("chat").
			Responding("v2").
			Build(),
	)
	env := testutil.Env(t)
	env.Model = model

	chain := &Chain{Links: []Link{
		chainerpkg.NewLink("one", "first {{.Instruction}}", nil, "out.one", nil),
		chainerpkg.NewLink("two", "second {{index .Input \"out.one\"}}", []string{"out.one"}, "out.two", nil),
	}}

	state := core.NewContext()
	result, err := ExecuteChain(context.Background(), env, &core.Task{
		ID:          "chain-task",
		Instruction: "implement",
	}, state, chain)
	if err != nil {
		t.Fatalf("ExecuteChain: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("unexpected result: %+v", result)
	}
	if got := state.GetString("out.one"); got != "v1" {
		t.Fatalf("unexpected first output: %q", got)
	}
	if got := state.GetString("out.two"); got != "v2" {
		t.Fatalf("unexpected second output: %q", got)
	}
	model.AssertExhausted(t)
}
