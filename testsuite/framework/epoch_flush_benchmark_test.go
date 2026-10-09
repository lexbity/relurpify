package framework

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
)

// TestEpochFlushBudget64Captures is the NFR-1 tripwire: a 64-capture epoch
// barrier flush against a real (warm) Badger temp store must stay inside 3×
// the NFR-1 budget (p99 ≤ 300 ms, p50 ≤ 60 ms). The NFR numbers are the SLO;
// this test is the CI tripwire with headroom against scheduler noise.
func TestEpochFlushBudget64Captures(t *testing.T) {
	ctx := context.Background()
	engine, err := graphdb.Open(ctx, graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = engine.Close(ctx) })
	store := &knowledge.ChunkStore{Graph: engine}
	grounder := knowledge.NewGroundingService(store, nil, nil, nil)

	const batchSize = 64
	buildBatch := func(iteration int) []knowledge.GroundingItem {
		items := make([]knowledge.GroundingItem, 0, batchSize)
		for i := 0; i < batchSize; i++ {
			items = append(items, knowledge.GroundingItem{
				Value:          fmt.Sprintf("finding-%d-%d", iteration, i),
				TypeAnnotation: "ReviewFindings",
				Epistemics:     knowledge.EpistemicClaimed,
				Origin:         contextdata.OriginLLM,
				StateKey:       fmt.Sprintf("state.finding.%d", i),
				NodeID:         "node-a",
				TaskID:         fmt.Sprintf("task-%d", iteration),
				SessionID:      "session-bench",
				WorkspaceID:    "ws",
				Kind:           knowledge.ChunkKindCapture,
			})
		}
		return items
	}

	// Warm the store (first txn pays Badger cold-start write paths) and record
	// the subsequent flush latencies.
	_, err = grounder.Ground(ctx, buildBatch(0))
	require.NoError(t, err)

	durations := make([]time.Duration, 0, 20)
	for iteration := 1; iteration <= 20; iteration++ {
		started := time.Now()
		_, err := grounder.Ground(ctx, buildBatch(iteration))
		require.NoError(t, err)
		durations = append(durations, time.Since(started))
	}
	require.Len(t, durations, 20)

	sorted := append([]time.Duration(nil), durations...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	p50 := sorted[(len(sorted)-1)/2]
	p99 := sorted[len(sorted)-1] // 20 samples: the slowest is the empirical p99 proxy

	t.Logf("epoch flush (64 captures, warm store): p50=%s p99(proxy)=%s", p50, p99)
	require.LessOrEqual(t, p99, 300*time.Millisecond,
		"NFR-1 tripwire: epoch barrier flush p99 must stay under 3× the 100ms SLO")
	require.LessOrEqual(t, p50, 60*time.Millisecond,
		"NFR-1 tripwire: epoch barrier flush p50 must stay under 3× the 20ms SLO")
}
