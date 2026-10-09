package thoughtrecipe

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
)

func telDepsWithSink(t *testing.T, sink *recordingTelemetrySink) *paradigm.Deps {
	t.Helper()
	return &paradigm.Deps{Telemetry: sink}
}

func telHasEvent(t *testing.T, sink *recordingTelemetrySink, eventType string) bool {
	t.Helper()
	for _, event := range sink.Events() {
		if string(event.Type) == eventType {
			return true
		}
	}
	return false
}

// recordingCaptureSink collects enqueued grounding items.
type recordingCaptureSink struct {
	items []knowledge.GroundingItem
}

func (s *recordingCaptureSink) EnqueueCapture(item knowledge.GroundingItem) {
	s.items = append(s.items, item)
}

func captureBindingFor(source, dest string, episteme *EpistemicExpr) CaptureBinding {
	binding := CaptureBinding{
		Source:      &PathExpr{Raw: source},
		Destination: PathExpr{Raw: dest},
	}
	if episteme != nil {
		binding.Epistemics = episteme
	}
	return binding
}

func TestBuildCaptureItemsGroundsUserOriginGiven(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	env.SetWorkingValueWithOrigin("input.prompt", "verbatim user text", contextdata.MemoryClassTask, contextdata.OriginUser)
	step := ExecutionStep{ID: "step.1", Sources: []string{"input.prompt"}}

	items, downgrades := buildCaptureItems(step, env, []CaptureBinding{
		captureBindingFor("input.prompt", "state.answer", &EpistemicExpr{Value: "given"}),
	}, nil)
	require.Len(t, items, 1)
	require.Empty(t, downgrades)
	require.Equal(t, knowledge.EpistemicGiven, items[0].Epistemics)
	require.Equal(t, contextdata.OriginUser, items[0].Origin)
	require.Equal(t, knowledge.ChunkKindCapture, items[0].Kind)
	require.Equal(t, "state.answer", items[0].StateKey)
	require.Equal(t, "step.1", items[0].NodeID)
}

func TestBuildCaptureItemsDowngradesGivenOnNonUserOrigin(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	// The source is present but its recorded origin is llm (unset default).
	env.SetWorkingValueWithClass("findings", "agent text", contextdata.MemoryClassTask)
	step := ExecutionStep{ID: "step.1"}

	items, downgrades := buildCaptureItems(step, env, []CaptureBinding{
		captureBindingFor("findings", "state.answer", &EpistemicExpr{Value: "given"}),
	}, nil)
	require.Len(t, items, 1)
	require.Len(t, downgrades, 1)
	require.Equal(t, "state.answer", downgrades[0])
	require.Equal(t, knowledge.EpistemicClaimed, items[0].Epistemics, "given downgraded to claimed")
}

func TestBuildCaptureItemsSkipsScratchDestination(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	step := ExecutionStep{ID: "step.1"}
	items, downgrades := buildCaptureItems(step, env, []CaptureBinding{
		captureBindingFor("findings", "scratch.tmp", nil),
		captureBindingFor("findings", "state.kept", nil),
	}, nil)
	require.Len(t, items, 1)
	require.Equal(t, "state.kept", items[0].StateKey)
	require.Empty(t, downgrades)
}

func TestEnqueueCaptureItemsUsesSinkFromContext(t *testing.T) {
	ctx := context.Background()
	env := contextdata.NewEnvelope("task-1", "session-1")
	env.SetWorkingValueWithOrigin("user.prompt", "text", contextdata.MemoryClassTask, contextdata.OriginUser)
	step := ExecutionStep{ID: "step.1"}

	sink := &recordingCaptureSink{}
	core := &stepCore{id: "step.1", step: step}
	core.enqueueCaptureItems(agentgraph.WithCaptureSink(ctx, sink), env, []CaptureBinding{
		captureBindingFor("user.prompt", "state.answer", &EpistemicExpr{Value: "given"}),
	}, nil)
	require.Len(t, sink.items, 1)
	require.Equal(t, knowledge.EpistemicGiven, sink.items[0].Epistemics)
}

func TestEnqueueCaptureItemsSinkAbsentIsExplicit(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	step := ExecutionStep{ID: "step.1"}
	tel := &recordingTelemetrySink{}
	core := &stepCore{id: "step.1", step: step, deps: telDepsWithSink(t, tel)}
	core.enqueueCaptureItems(context.Background(), env, []CaptureBinding{
		captureBindingFor("findings", "state.answer", nil),
	}, nil)
	require.True(t, telHasEvent(t, tel, "capture.sink_absent"), "missing sink must be observable")
}

func TestBuildToolResultItemKindAndOrigin(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	step := ExecutionStep{ID: "step.1"}
	item := buildToolResultItem(step, env, map[string]any{"output": "payload"})
	require.Equal(t, knowledge.ChunkKindTool, item.Kind)
	require.Equal(t, contextdata.OriginTool, item.Origin)
	require.Equal(t, knowledge.EpistemicClaimed, item.Epistemics)
}
