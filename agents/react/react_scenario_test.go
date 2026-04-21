//go:build scenario

package react_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/agents/react"
	"codeburg.org/lexbit/relurpify/framework/core"
	agenttestscenario "codeburg.org/lexbit/relurpify/testutil/agenttestscenario"
	testutil "codeburg.org/lexbit/relurpify/testutil/euclotestutil"
)

func TestReActAgent_Scenario_SingleToolCall_ThenComplete(t *testing.T) {
	f := agenttestscenario.NewFixture(t,
		testutil.Turn("chat_with_tools").
			ExpectingPromptFragment("implement feature").
			Responding("").
			WithToolCall("echo", map[string]interface{}{"value": "hi"}).
			Build(),
		testutil.Turn("chat_with_tools").
			Responding(`{"thought":"done","action":"complete","tool":"","arguments":{},"complete":true,"summary":"ok"}`).
			Build(),
	)
	f.Env.Config.MaxIterations = 3
	f.Env.Config.NativeToolCalling = true

	agent := react.New(f.Env)
	state := core.NewContext()
	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "react-scenario-success",
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "implement feature",
	}, state)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	agenttestscenario.RequireResultSuccess(t, result)
	agenttestscenario.RequireModelExhausted(t, f)
	agenttestscenario.Require(t, strings.TrimSpace(state.GetString("react.final_output_summary")) != "", "expected final output summary")
}

func TestReActAgent_Scenario_MaxIterationsReached(t *testing.T) {
	f := agenttestscenario.NewFixture(t,
		testutil.Turn("chat_with_tools").
			Responding(`{"thought":"keep going","action":"tool","tool":"echo","arguments":{"value":"one"},"complete":false}`).
			Build(),
		testutil.Turn("chat_with_tools").
			Responding(`{"thought":"still going","action":"tool","tool":"echo","arguments":{"value":"two"},"complete":false}`).
			Build(),
	)
	f.Env.Config.MaxIterations = 2
	f.Env.Config.NativeToolCalling = true

	agent := react.New(f.Env)
	state := core.NewContext()
	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "react-scenario-max-iterations",
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "implement feature",
	}, state)
	if err == nil && (result == nil || result.Success) {
		t.Fatalf("expected unsuccessful completion, got result=%+v err=%v", result, err)
	}
	if result != nil && result.Success {
		t.Fatalf("expected unsuccessful result, got %+v", result)
	}
	reason := state.GetString("react.incomplete_reason")
	if err != nil {
		reason = err.Error()
	}
	if !strings.Contains(reason, "iteration budget exhausted") {
		t.Fatalf("unexpected incomplete reason: %q", reason)
	}
	agenttestscenario.RequireModelExhausted(t, f)
}

func TestReActAgent_Scenario_ToolCallError_Propagates(t *testing.T) {
	f := agenttestscenario.NewFixture(t,
		testutil.Turn("chat_with_tools").
			Responding("").
			WithToolCall("echo", map[string]interface{}{"value": "hi"}).
			Build(),
		testutil.Turn("chat_with_tools").
			ReturningError(errors.New("llm unavailable")).
			Build(),
	)
	f.Env.Config.MaxIterations = 3
	f.Env.Config.NativeToolCalling = true

	agent := react.New(f.Env)
	_, err := agent.Execute(context.Background(), &core.Task{
		ID:          "react-scenario-error",
		Type:        core.TaskTypeCodeGeneration,
		Instruction: "implement feature",
	}, core.NewContext())
	if err == nil {
		t.Fatal("expected llm error")
	}
	if !strings.Contains(err.Error(), "llm unavailable") {
		t.Fatalf("unexpected error: %v", err)
	}
	agenttestscenario.RequireModelExhausted(t, f)
}
