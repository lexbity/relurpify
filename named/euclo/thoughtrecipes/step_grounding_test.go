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

// TestBuildCaptureItemsToolOriginGroundsAsToolKind pins the absorbed input
// taxonomy at the production capture site: a capture whose dataflow floor is
// tool output grounds as ChunkKindTool; agent-claim captures stay
// ChunkKindCapture.
func TestBuildCaptureItemsToolOriginGroundsAsToolKind(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	env.SetWorkingValueWithOrigin("tools.grep.result", "match line", contextdata.MemoryClassTask, contextdata.OriginTool)
	env.SetWorkingValueWithClass("findings", "agent text", contextdata.MemoryClassTask)
	step := ExecutionStep{ID: "step.1", Sources: []string{"tools.grep.result"}}

	items, downgrades := buildCaptureItems(step, env, []CaptureBinding{
		captureBindingFor("tools.grep.result", "state.evidence", nil),
		captureBindingFor("findings", "state.summary", nil),
	}, nil)
	require.Len(t, items, 2)
	require.Empty(t, downgrades)
	require.Equal(t, knowledge.ChunkKindTool, items[0].Kind, "tool-floor capture grounds as a tool fact")
	require.Equal(t, contextdata.OriginTool, items[0].Origin)
	require.Equal(t, knowledge.ChunkKindCapture, items[1].Kind, "agent-claim capture stays a capture")
	require.Equal(t, contextdata.OriginLLM, items[1].Origin)
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
	require.NoError(t, core.enqueueCaptureItems(agentgraph.WithCaptureSink(ctx, sink), env, []CaptureBinding{
		captureBindingFor("user.prompt", "state.answer", &EpistemicExpr{Value: "given"}),
	}, nil))
	require.Len(t, sink.items, 1)
	require.Equal(t, knowledge.EpistemicGiven, sink.items[0].Epistemics)
}
