package runtime

import (
	"context"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	intentcontext "codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

type recordingExecutor struct {
	mu        sync.Mutex
	execCount int
	lastTask  *core.Task
	lastEnv   *contextdata.Envelope
}

func (r *recordingExecutor) Initialize(*core.Config) error { return nil }

func (r *recordingExecutor) Execute(ctx context.Context, task *core.Task, env *contextdata.Envelope) (*core.Result, error) {
	_ = ctx
	r.mu.Lock()
	defer r.mu.Unlock()
	r.execCount++
	r.lastTask = task
	r.lastEnv = env
	return &core.Result{NodeID: "recording", Success: true}, nil
}

func (r *recordingExecutor) Capabilities() []string { return nil }

func (r *recordingExecutor) BuildGraph(*core.Task) (*agentgraph.Graph, error) { return nil, nil }

func TestResolveInteractionFrameResumesClarificationTask(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	task := &core.Task{ID: "task-1", Instruction: "clarify request"}
	env.SetWorkingValue("task.input", task, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.interaction.frame_seq", 1, contextdata.MemoryClassTask)

	frame := interaction.NewClarificationFrame("task-1", "session-1", "Pick one", []string{"review", "implement"}, nil)
	env.SetWorkingValue("euclo.interaction.frame.0", frame, contextdata.MemoryClassTask)

	executor := &recordingExecutor{}
	rt := &Runtime{
		Agent:                executor,
		interactionEnvelopes: map[string]*contextdata.Envelope{"task-1": env},
	}

	if err := rt.ResolveInteractionFrame(context.Background(), "task-1", frame.ID, "implement", ""); err != nil {
		t.Fatalf("resolve interaction frame failed: %v", err)
	}
	if executor.execCount != 1 {
		t.Fatalf("execute count = %d, want 1", executor.execCount)
	}
	if executor.lastTask != task {
		t.Fatalf("task pointer mismatch: got %#v want %#v", executor.lastTask, task)
	}
	if executor.lastEnv != env {
		t.Fatalf("envelope pointer mismatch: got %#v want %#v", executor.lastEnv, env)
	}
	if frame.Response == nil || frame.Response.ChosenSlot != "implement" {
		t.Fatalf("frame response = %#v", frame.Response)
	}
	if got, ok := env.GetWorkingValue(intentcontext.ClarificationStateKey); !ok || got == nil {
		t.Fatal("expected clarification state to be written back")
	}
	if got, ok := env.GetWorkingValue("euclo.interaction.frame_requested"); !ok || got != false {
		t.Fatalf("frame_requested = %#v ok=%v, want false", got, ok)
	}
}

func TestResolveInteractionFrameDoesNotResumeOutcomeFeedback(t *testing.T) {
	env := contextdata.NewEnvelope("task-2", "session-2")
	task := &core.Task{ID: "task-2", Instruction: "collect feedback"}
	env.SetWorkingValue("task.input", task, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.interaction.frame_seq", 1, contextdata.MemoryClassTask)

	frame := interaction.NewOutcomeFeedbackFrame("task-2", "session-2", "complete")
	env.SetWorkingValue("euclo.interaction.frame.0", frame, contextdata.MemoryClassTask)

	executor := &recordingExecutor{}
	rt := &Runtime{
		Agent:                executor,
		interactionEnvelopes: map[string]*contextdata.Envelope{"task-2": env},
	}

	if err := rt.ResolveInteractionFrame(context.Background(), "task-2", frame.ID, "negative", ""); err != nil {
		t.Fatalf("resolve interaction frame failed: %v", err)
	}
	if executor.execCount != 0 {
		t.Fatalf("execute count = %d, want 0", executor.execCount)
	}
	if frame.Response == nil || frame.Response.ChosenSlot != "negative" {
		t.Fatalf("frame response = %#v", frame.Response)
	}
	if got, ok := env.GetWorkingValue("euclo.interaction.frame_requested"); !ok || got != false {
		t.Fatalf("frame_requested = %#v ok=%v, want false", got, ok)
	}
}
