package contextbudget

import (
	"testing"

	"codeburg.org/lexbit/relurpify/platform/contracts"
	"github.com/stretchr/testify/require"
)

type snapshotTelemetryStub struct {
	events []contracts.Event
}

func (s *snapshotTelemetryStub) Emit(event contracts.Event) {
	s.events = append(s.events, event)
}

func TestSnapshotEmitter_EmitsEveryInterval(t *testing.T) {
	advisor := &ContextBudgetAdvisor{ModelContextSize: 4096}
	telemetry := &snapshotTelemetryStub{}
	emitter := NewSnapshotEmitter(advisor, telemetry, 3)

	advisor.RecordCall(contracts.TokenUsageReport{PromptTokens: 100})
	emitter.Observe()
	require.Len(t, telemetry.events, 0)

	advisor.RecordCall(contracts.TokenUsageReport{PromptTokens: 100})
	emitter.Observe()
	require.Len(t, telemetry.events, 0)

	advisor.RecordCall(contracts.TokenUsageReport{PromptTokens: 100})
	emitter.Observe()
	require.Len(t, telemetry.events, 1)
	require.Equal(t, contracts.EventBudgetSnapshot, telemetry.events[0].Type)

	snapshot, ok := telemetry.events[0].Metadata["snapshot"].(BudgetSnapshot)
	require.True(t, ok)
	require.Equal(t, 3, snapshot.CallCount)
	require.Equal(t, 4096, snapshot.ModelContextSize)
}
