package knowledge

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// fixedQuota reports the same remaining quota for every principal.
type fixedQuota struct{ remaining int }

func (q fixedQuota) QuotaRemaining(_ string) int { return q.remaining }

// TestGroundingQuarantinesBinaryValue proves null-byte content is rejected by
// admission before any store transaction.
func TestGroundingQuarantinesBinaryValue(t *testing.T) {
	store := newTestStore(t)
	tel := newRecordingBusTelemetry()
	service := newGroundingService(t, store, tel)

	item := groundItem(map[string]any{"text": "a\x00binary"}, EpistemicClaimed, contextdata.OriginLLM)
	report, err := service.Ground(context.Background(), []GroundingItem{item})
	require.NoError(t, err)
	require.Empty(t, report.Grounded)
	require.Empty(t, report.Skipped)
	require.Len(t, report.Quarantined, 1)
	require.Contains(t, report.Quarantined[0].Reason, "binary")

	all, err := store.FindAll()
	require.NoError(t, err)
	require.Empty(t, all, "quarantined content must never reach the store")
	require.Equal(t, 1, tel.count(telemetry.EventCaptureQuarantined))
}

// TestGroundingSuspicionOnHighNonPrintableRatio proves the shared ratio
// predicate gates captures carrying dense control characters.
func TestGroundingSuspicionOnHighNonPrintableRatio(t *testing.T) {
	store := newTestStore(t)
	service := newGroundingService(t, store, nil)

	item := groundItem(map[string]any{"text": string([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})}, EpistemicClaimed, contextdata.OriginLLM)
	report, err := service.Ground(context.Background(), []GroundingItem{item})
	require.NoError(t, err)
	require.Len(t, report.Quarantined, 1)

	all, err := store.FindAll()
	require.NoError(t, err)
	require.Empty(t, all)
}

// TestGroundingSkippedOnQuotaExhausted proves a drained quota skips an item
// with an explicit reason.
func TestGroundingSkippedOnQuotaExhausted(t *testing.T) {
	store := newTestStore(t)
	service := NewGroundingService(store, nil, fixedQuota{remaining: 0}, nil)

	item := groundItem(map[string]any{"text": "quota"}, EpistemicClaimed, contextdata.OriginLLM)
	report, err := service.Ground(context.Background(), []GroundingItem{item})
	require.NoError(t, err)
	require.Empty(t, report.Grounded)
	require.Len(t, report.Skipped, 1)
	require.Equal(t, "quota exceeded", report.Skipped[0].Reason)

	all, err := store.FindAll()
	require.NoError(t, err)
	require.Empty(t, all)
}
