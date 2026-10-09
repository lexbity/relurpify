package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/session"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// buildDegradedRuntime constructs the same degraded runtime New() produces on a
// recoverable boot failure, with the boot reason retained on Readiness.
func buildDegradedRuntime(t *testing.T, reason string) *Runtime {
	t.Helper()
	return newDegradedRuntime(context.Background(), Config{Workspace: t.TempDir()}, errors.New(reason))
}

// TestDegradedRunTask proves AC-8: a task submitted to a degraded runtime is
// rejected with a typed *ErrRuntimeDegraded, emits task.rejected, and leaves no
// lifecycle records (the guard runs before envelope tracking and workflow
// creation), rather than panicking on the nil agent.
func TestDegradedRunTask(t *testing.T) {
	rt := buildDegradedRuntime(t, "compose failed: test")
	require.NotNil(t, rt)
	require.Nil(t, rt.Agent, "degraded runtime has no agent by construction")

	sink := &recordingTelemetry{}
	rt.Workspace.Telemetry = sink

	result, err := rt.RunTask(context.Background(), &execution.Task{ID: "task-1", Instruction: "do something"})
	require.Nil(t, result)

	var degraded *ErrRuntimeDegraded
	require.ErrorAs(t, err, &degraded, "RunTask on a degraded runtime must return *ErrRuntimeDegraded")
	require.Contains(t, degraded.Reason, "compose failed: test")
	require.Contains(t, err.Error(), "runtime degraded, task rejected")
	require.True(t, errors.Is(err, &ErrRuntimeDegraded{}),
		"the degraded category must be matchable with errors.Is")

	// Guard ordering: no interaction envelope registered, no workflow active.
	require.Nil(t, rt.interactionEnvelope("task-1"),
		"rejected task must not leave an interaction envelope")
	require.Equal(t, "", rt.ActiveWorkflowID(),
		"rejected task must not create an active workflow")

	// Exactly one task.rejected event, carrying the boot failure reason.
	require.Len(t, sink.events, 1)
	ev := sink.events[0]
	require.Equal(t, telemetry.EventTaskRejected, ev.Type)
	require.Equal(t, "task-1", ev.TaskID)
	require.Equal(t, degraded.Reason, ev.Metadata["reason"])
}

// TestDegradedResumeSession proves the shared executeTask path rejects a resume
// with the same typed error: ResumeSession must not panic on the nil agent.
func TestDegradedResumeSession(t *testing.T) {
	rt := buildDegradedRuntime(t, "compose failed: resume")
	sink := &recordingTelemetry{}
	rt.Workspace.Telemetry = sink

	outcome, err := rt.ResumeSession(context.Background(), "", "continue the work")
	require.Nil(t, outcome)

	var degraded *ErrRuntimeDegraded
	require.ErrorAs(t, err, &degraded)
	require.Contains(t, degraded.Reason, "compose failed: resume")

	require.Len(t, sink.events, 1)
	require.Equal(t, telemetry.EventTaskRejected, sink.events[0].Type)
}

// TestDegradedReasonFallback covers the defensive path: a runtime whose
// workspace carries no stored reason still produces a non-empty typed error,
// and a nil telemetry sink does not panic the rejection path.
func TestDegradedReasonFallback(t *testing.T) {
	rt := &Runtime{
		Workspace: &session.Workspace{Readiness: session.Readiness{Degraded: true}},
	}

	result, err := rt.RunTask(context.Background(), &execution.Task{ID: "task-no-reason"})
	require.Nil(t, result)

	var degraded *ErrRuntimeDegraded
	require.ErrorAs(t, err, &degraded)
	require.Equal(t, "boot failed", degraded.Reason)
}

// TestDegradedRuntimeNoLifecycleRepository documents that a degraded runtime
// has no lifecycle repository; the rejection guard is what keeps the missing
// repository from ever being consulted for a rejected task.
func TestDegradedRuntimeNoLifecycleRepository(t *testing.T) {
	rt := buildDegradedRuntime(t, "compose failed: lifecycle")
	require.Nil(t, rt.AgentLifecycle)
}

// TestDegradedResumeInteractionTask proves the interaction-resume path shares
// the typed rejection: with a resume-ready envelope but a nil agent, it returns
// *ErrRuntimeDegraded instead of the former generic error.
func TestDegradedResumeInteractionTask(t *testing.T) {
	env := contextdata.NewEnvelope("task-resume", "session-resume")
	env.SetWorkingValueWithClass(euclostate.KeyTaskInput, &execution.Task{ID: "task-resume"}, contextdata.MemoryClassTask)

	rt := &Runtime{
		Workspace: &session.Workspace{Readiness: session.Readiness{Degraded: true, Reason: "compose failed: interaction"}},
	}

	result, err := rt.resumeInteractionTask(context.Background(), env)
	require.Nil(t, result)

	var degraded *ErrRuntimeDegraded
	require.ErrorAs(t, err, &degraded)
	require.Contains(t, degraded.Reason, "compose failed: interaction")
}
