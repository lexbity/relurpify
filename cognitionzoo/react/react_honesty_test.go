package react

import (
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
)

func seedUnverifiedScenario(t *testing.T, taskType, instruction string) (*ReActAgent, *contextdata.Envelope, *execution.Task) {
	t.Helper()
	sink := &capturingTelemetry{}
	order := []string{}
	agent, _ := newProbeDeliveryAgent(t, &order, sink)
	agent.Model = &scriptedReactModel{
		scripted: []string{
			`{"thought":"look around","tool":"search_grep","arguments":{"pattern":"bug"},"complete":false}`,
			`{"thought":"done","action":"complete","complete":true,"summary":"fixed the parser bug"}`,
		},
	}
	env := contextdata.NewEnvelope("task-unverified", "session-unverified")
	env.SetWorkingValueWithClass("task.type", taskType, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("task.instruction", instruction, contextdata.MemoryClassTask)
	task := &execution.Task{ID: "task-unverified", Instruction: instruction}
	return agent, env, task
}

// An edit-shaped task that closes with the model declaring complete but
// without an executed write must be marked unverified at loop close: the
// model's assertion of success is not evidence.
func TestLoopCloseMarksEditTaskWithoutWritesUnverified(t *testing.T) {
	agent, env, task := seedUnverifiedScenario(t, "execute", "fix the parser bug")

	runReactLoop(t, agent, task, env)

	unverified, _ := contextdata.GetTyped[bool](env, "euclo.outcome.unverified")
	if !unverified {
		t.Fatal("edit-shaped task closed without a write must be marked unverified")
	}
	reason, _ := contextdata.GetTyped[string](env, "euclo.outcome.unverified_reason")
	if reason == "" {
		t.Fatal("the unverified marker must carry a reason")
	}
}

// A read-only intent has nothing to verify: completion stays clean.
func TestLoopCloseReadonlyTaskNotMarkedUnverified(t *testing.T) {
	agent, env, task := seedUnverifiedScenario(t, "analysis", "analyze why the suite fails")

	runReactLoop(t, agent, task, env)

	unverified, _ := contextdata.GetTyped[bool](env, "euclo.outcome.unverified")
	if unverified {
		t.Fatal("read-only task must not be marked unverified")
	}
}

// An executed write satisfies the evidence bar: the same task with an edit
// observation on record closes verified.
func TestLoopCloseEditObservationSatisfiesEvidence(t *testing.T) {
	agent, env, task := seedUnverifiedScenario(t, "execute", "fix the parser bug")
	env.SetWorkingValueWithClass("react.tool_observations", []ToolObservation{
		{Tool: "file_write", Success: true},
	}, contextdata.MemoryClassTask)

	runReactLoop(t, agent, task, env)

	unverified, _ := contextdata.GetTyped[bool](env, "euclo.outcome.unverified")
	if unverified {
		t.Fatal("a task with an executed edit observation must not be marked unverified")
	}
}
