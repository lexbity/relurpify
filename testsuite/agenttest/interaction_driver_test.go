package agenttest

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

func pausedStartedAt() time.Time { return time.Now().UTC() }

// pausedAskExecutor simulates a DSL ask pause: the first Execute returns the
// pause metadata with a pending frame written onto the envelope; once the
// frame carries a response it returns the completed result and records the
// answer it saw.
type pausedAskExecutor struct {
	env     *contextdata.Envelope
	frame   *interaction.InteractionFrame
	answers []string
	calls   int
}

func (x *pausedAskExecutor) Initialize(config *execution.Config) error { return nil }
func (x *pausedAskExecutor) Capabilities() []string                    { return nil }
func (x *pausedAskExecutor) BuildGraph(context.Context, *execution.Task) (*agentgraph.Graph, error) {
	return nil, errors.New("not used")
}

func (x *pausedAskExecutor) Execute(_ context.Context, _ *execution.Task, env *contextdata.Envelope) (*execution.Result, error) {
	x.calls++
	if x.frame != nil && x.frame.Response != nil {
		answer, _ := interaction.ResponseValue(x.frame)
		x.answers = append(x.answers, answer)
		return &execution.Result{NodeID: "root", Success: true}, nil
	}
	return &execution.Result{
		NodeID:   "root",
		Success:  true,
		Metadata: map[string]any{"euclo.interaction.pause": true, "frame_id": x.frame.ID},
	}, nil
}

func askTestEnv(t *testing.T) (*contextdata.Envelope, *interaction.InteractionFrame) {
	t.Helper()
	env := contextdata.NewEnvelope("task-ask", "session-ask")
	frame := &interaction.InteractionFrame{
		ID: "frame-ask-1", Type: interaction.FrameOutcomeFeedback,
		TaskID: "task-ask", Question: "proceed?",
	}
	contextdata.SetTyped(env, "euclo.interaction.frame.ask", frame)
	return env, frame
}

func driverExecutor(x agentgraph.WorkflowExecutor) *PreparedRunExecutor {
	e := &PreparedRunExecutor{}
	e.agentOverride = x
	return e
}

// TestInteractionDriverAnswersScriptedFrame proves the D8 driver: a paused
// run's pending frame is answered from the script (Action as the answer),
// the resume Execute happens, and the (frame, answer) pair lands in the
// records for the case report's frames section.
func TestInteractionDriverAnswersScriptedFrame(t *testing.T) {
	env, frame := askTestEnv(t)
	x := &pausedAskExecutor{env: env, frame: frame}
	e := driverExecutor(x)
	desc := &PreparedRunDescriptor{RunID: "run-ask", Instruction: "do it",
		InteractionScript: []InteractionScriptStep{{Action: "yes"}}}

	paused := &execution.Result{NodeID: "root", Success: true,
		Metadata: map[string]any{"euclo.interaction.pause": true, "frame_id": frame.ID}}
	result, records, err := e.driveInteractionScript(context.Background(), desc,
		&execution.Task{ID: "task-ask"}, env, paused, &bytes.Buffer{})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success, "the resumed run completes")
	require.Equal(t, []string{"yes"}, x.answers, "the scripted answer reached the frame")
	require.Len(t, records, 1)
	require.Equal(t, "frame-ask-1", records[0].FrameID)
	require.Equal(t, "yes", records[0].Answer)
	require.True(t, records[0].Resumed)
}

// TestUnscriptedPauseFailsTyped proves the honesty bound: a paused run with
// an exhausted script fails typed ErrUnansweredFrame — never a silent pass.
func TestUnscriptedPauseFailsTyped(t *testing.T) {
	env, frame := askTestEnv(t)
	x := &pausedAskExecutor{env: env, frame: frame}
	e := driverExecutor(x)
	desc := &PreparedRunDescriptor{RunID: "run-ask", Instruction: "do it"}

	paused := &execution.Result{NodeID: "root", Success: true,
		Metadata: map[string]any{"euclo.interaction.pause": true, "frame_id": frame.ID}}
	_, _, err := e.driveInteractionScript(context.Background(), desc,
		&execution.Task{ID: "task-ask"}, env, paused, nil)
	var unanswered *ErrUnansweredFrame
	require.ErrorAs(t, err, &unanswered)
	require.Equal(t, "frame-ask-1", unanswered.FrameID)
	require.Equal(t, "proceed?", unanswered.Question)
	require.Contains(t, err.Error(), "unanswered_frame")

	// And the classification lands the class in the report.
	report := e.buildCaseReport(desc, paused, err, 1, nil, nil, pausedStartedAt())
	require.False(t, report.Success)
	require.Equal(t, "unanswered_frame", report.FailureKind)
}

// repausingExecutor always returns the pause outcome: the driver's bound is
// what stops it.
type repausingExecutor struct {
	env *contextdata.Envelope
}

func (x *repausingExecutor) Initialize(config *execution.Config) error { return nil }
func (x *repausingExecutor) Capabilities() []string                    { return nil }
func (x *repausingExecutor) BuildGraph(context.Context, *execution.Task) (*agentgraph.Graph, error) {
	return nil, errors.New("not used")
}

func (x *repausingExecutor) Execute(_ context.Context, _ *execution.Task, env *contextdata.Envelope) (*execution.Result, error) {
	// A re-pausing run writes a fresh unanswered ask frame, as a real ask
	// node re-entered without an answer would.
	frame := &interaction.InteractionFrame{
		ID: "frame-repause", Type: interaction.FrameOutcomeFeedback,
		TaskID: "task-ask", Question: "again?",
	}
	contextdata.SetTyped(env, "euclo.interaction.frame.repause", frame)
	return &execution.Result{NodeID: "root", Success: true,
		Metadata: map[string]any{"euclo.interaction.pause": true}}, nil
}

// TestDriverBoundsRoundsAtScriptLength proves the loop bound: a run that
// re-pauses after every answer consumes the script and then fails typed —
// no infinite driver loop.
func TestDriverBoundsRoundsAtScriptLength(t *testing.T) {
	env, _ := askTestEnv(t)
	// This executor re-pauses regardless of answers.
	x := &repausingExecutor{env: env}
	e := driverExecutor(x)
	desc := &PreparedRunDescriptor{RunID: "run-ask",
		InteractionScript: []InteractionScriptStep{{Action: "yes"}, {Action: "again"}}}

	paused := &execution.Result{NodeID: "root", Success: true,
		Metadata: map[string]any{"euclo.interaction.pause": true}}
	_, records, err := e.driveInteractionScript(context.Background(), desc,
		&execution.Task{ID: "task-ask"}, env, paused, nil)
	var unanswered *ErrUnansweredFrame
	require.ErrorAs(t, err, &unanswered)
	require.Len(t, records, 2, "the script's entries were consumed, then the driver stopped")
	require.True(t, strings.Contains(unanswered.Error(), "2 scripted"))
}
