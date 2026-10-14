package observing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingUsageObserver is a UsageObserver double that records every token
// usage record and hands back queued reset notices.
type recordingUsageObserver struct {
	usage  []TokenUsage
	reset  []any
	resetI int
}

func (o *recordingUsageObserver) RecordTokenUsage(usage TokenUsage) {
	o.usage = append(o.usage, usage)
}

func (o *recordingUsageObserver) ConsumeResetNotice() (any, bool) {
	if o.resetI >= len(o.reset) {
		return nil, false
	}
	v := o.reset[o.resetI]
	o.resetI++
	return v, true
}

// noopSnapshotObserver is a SnapshotObserver double.
type noopSnapshotObserver struct{ calls int }

func (o *noopSnapshotObserver) Observe() { o.calls++ }

// stubBudgetItem is a minimal BudgetItem double.
type stubBudgetItem struct {
	id       string
	tokens   int
	priority int
}

func (i *stubBudgetItem) GetID() string                 { return i.id }
func (i *stubBudgetItem) GetTokenCount() int            { return i.tokens }
func (i *stubBudgetItem) GetPriority() int              { return i.priority }
func (i *stubBudgetItem) CanCompress() bool             { return false }
func (i *stubBudgetItem) Compress() (BudgetItem, error) { return nil, errors.New("not compressible") }
func (i *stubBudgetItem) CanEvict() bool                { return true }

// stubBudgetManager is a minimal BudgetManager double.
type stubBudgetManager struct {
	allocated map[string]int
	remaining int
}

func (m *stubBudgetManager) Allocate(category string, tokens int, _ BudgetItem) error {
	m.allocated[category] += tokens
	return nil
}

func (m *stubBudgetManager) Free(category string, tokens int, _ string) {
	m.allocated[category] -= tokens
}

func (m *stubBudgetManager) GetRemainingBudget(string) int { return m.remaining }
func (m *stubBudgetManager) ShouldCompress() bool          { return false }
func (m *stubBudgetManager) CanAddTokens(tokens int) bool  { return tokens <= m.remaining }

// Compile-time checks that the doubles satisfy the observer vocabulary.
var (
	_ UsageObserver    = (*recordingUsageObserver)(nil)
	_ SnapshotObserver = (*noopSnapshotObserver)(nil)
	_ BudgetItem       = (*stubBudgetItem)(nil)
	_ BudgetManager    = (*stubBudgetManager)(nil)
)

func TestPerfStatsRecordTracksDurationsAndErrors(t *testing.T) {
	tests := []struct {
		name    string
		records []struct {
			duration time.Duration
			err      error
		}
		wantInvocations int
		wantMin         time.Duration
		wantMax         time.Duration
		wantErrors      int
		wantAverage     time.Duration
		wantSuccessRate float64
	}{
		{
			name: "successes and failures interleaved",
			records: []struct {
				duration time.Duration
				err      error
			}{
				{10 * time.Millisecond, nil},
				{30 * time.Millisecond, errors.New("boom")},
				{20 * time.Millisecond, nil},
			},
			wantInvocations: 3,
			wantMin:         10 * time.Millisecond,
			wantMax:         30 * time.Millisecond,
			wantErrors:      1,
			wantAverage:     20 * time.Millisecond,
			wantSuccessRate: 2.0 / 3.0,
		},
		{
			name: "first failure sets the minimum baseline",
			records: []struct {
				duration time.Duration
				err      error
			}{
				{5 * time.Millisecond, errors.New("fast failure")},
				{50 * time.Millisecond, nil},
			},
			wantInvocations: 2,
			wantMin:         5 * time.Millisecond,
			wantMax:         50 * time.Millisecond,
			wantErrors:      1,
			wantAverage:     27*time.Millisecond + 500*time.Microsecond,
			wantSuccessRate: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stats PerfStats
			for _, r := range tt.records {
				stats.Record(r.duration, r.err)
			}

			assert.Equal(t, tt.wantInvocations, stats.TotalInvocations)
			assert.Equal(t, tt.wantMin, stats.MinDuration)
			assert.Equal(t, tt.wantMax, stats.MaxDuration)
			assert.Equal(t, tt.wantErrors, stats.ErrorCount)
			assert.Equal(t, tt.wantAverage, stats.AverageDuration())
			assert.InDelta(t, tt.wantSuccessRate, stats.SuccessRate(), 1e-9)
		})
	}
}

func TestPerfStatsEmptyReturnsZeroValues(t *testing.T) {
	var stats PerfStats
	assert.Equal(t, time.Duration(0), stats.AverageDuration())
	assert.Equal(t, 0.0, stats.SuccessRate())
}

func TestUsageObserverContextRoundTrip(t *testing.T) {
	ctx := context.Background()
	assert.Nil(t, UsageObserverFromContext(ctx), "absent observer must read back as nil")

	obs := &recordingUsageObserver{}
	ctx = WithUsageObserver(ctx, obs)
	require.Same(t, obs, UsageObserverFromContext(ctx))

	// A non-UsageObserver value under the same key shape must not type-assert.
	assert.Nil(t, UsageObserverFromContext(WithUsageObserver(ctx, nil)))
}

func TestSnapshotObserverContextRoundTrip(t *testing.T) {
	ctx := context.Background()
	assert.Nil(t, SnapshotObserverFromContext(ctx), "absent observer must read back as nil")

	obs := &noopSnapshotObserver{}
	ctx = WithSnapshotObserver(ctx, obs)
	require.Same(t, obs, SnapshotObserverFromContext(ctx))
}

func TestUsageObserverReceivesRecordedUsage(t *testing.T) {
	obs := &recordingUsageObserver{}
	ctx := WithUsageObserver(context.Background(), obs)

	UsageObserverFromContext(ctx).RecordTokenUsage(TokenUsage{
		PromptTokens:     100,
		CompletionTokens: 20,
		TotalTokens:      120,
		Estimated:        true,
		EstimationMethod: "chars/4",
	})

	require.Len(t, obs.usage, 1)
	assert.Equal(t, TokenUsage{
		PromptTokens:     100,
		CompletionTokens: 20,
		TotalTokens:      120,
		Estimated:        true,
		EstimationMethod: "chars/4",
	}, obs.usage[0])
}

func TestSnapshotObserverIsObserved(t *testing.T) {
	obs := &noopSnapshotObserver{}
	ctx := WithSnapshotObserver(context.Background(), obs)

	SnapshotObserverFromContext(ctx).Observe()
	SnapshotObserverFromContext(ctx).Observe()

	assert.Equal(t, 2, obs.calls)
}

func TestBudgetVocabularyDoubles(t *testing.T) {
	item := &stubBudgetItem{id: "item-1", tokens: 64, priority: 3}
	mgr := &stubBudgetManager{allocated: map[string]int{}, remaining: 128}

	require.NoError(t, mgr.Allocate("tool-output", item.GetTokenCount(), item))
	assert.Equal(t, 64, mgr.allocated["tool-output"])
	assert.True(t, mgr.CanAddTokens(64))
	assert.False(t, mgr.CanAddTokens(129))
	assert.Equal(t, 128, mgr.GetRemainingBudget("tool-output"))
	assert.False(t, mgr.ShouldCompress())

	mgr.Free("tool-output", item.GetTokenCount(), item.GetID())
	assert.Equal(t, 0, mgr.allocated["tool-output"])

	assert.Equal(t, "item-1", item.GetID())
	assert.Equal(t, 3, item.GetPriority())
	assert.False(t, item.CanCompress())
	assert.True(t, item.CanEvict())
	_, err := item.Compress()
	assert.Error(t, err)
}
