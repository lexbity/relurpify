package telemetry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type snapshotTelemetryStub struct {
	events []Event
}

func (s *snapshotTelemetryStub) Emit(event Event) {
	s.events = append(s.events, event)
}

func TestSnapshotEmitter_EmitsEveryInterval(t *testing.T) {
	advisor := &ContextBudgetAdvisor{ModelContextSize: 4096}
	sink := &snapshotTelemetryStub{}

	advisor.RecordCall(TokenUsage{PromptTokens: 100})
	advisor.RecordCall(TokenUsage{PromptTokens: 100})
	advisor.RecordCall(TokenUsage{PromptTokens: 100})

	emitter := NewSnapshotEmitter(advisor, sink, 1)
	emitter.Observe()

	require.Len(t, sink.events, 1)
	require.Equal(t, EventBudgetSnapshot, string(sink.events[0].Type))
}
