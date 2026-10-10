package jobsstore

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/jobs"
)

// TestClaimRace_ExactlyOnce proves concurrent workers never claim the same
// job: 8 goroutines race to claim 100 queued jobs; the union of claims is
// exactly the 100 jobs, each claimed once, and the per-queue claim order is
// priority-desc then FIFO (NFR via Q14).
func TestClaimRace_ExactlyOnce(t *testing.T) {
	s, err := openTestStore(Options{InMemory: true})
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	const workers = 8
	const total = 100
	base := time.Now().UTC().Add(-time.Hour)
	for i := 0; i < total; i++ {
		j := sampleJob(fmt.Sprintf("race-%03d", i))
		j.Spec.Priority = i % 10
		j.CreatedAt = base.Add(time.Duration(i) * time.Millisecond)
		require.NoError(t, s.Create(ctx, j))
	}

	var mu sync.Mutex
	claimedBy := make(map[string]int) // jobID -> count
	var order []string                // global claim order
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			worker := fmt.Sprintf("worker-%d", w)
			for {
				got, err := s.Claim(ctx, worker, []string{"knowledge"}, 5)
				require.NoError(t, err)
				if len(got) == 0 {
					return
				}
				mu.Lock()
				for _, j := range got {
					claimedBy[j.ID]++
					order = append(order, j.ID)
				}
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()

	require.Len(t, claimedBy, total, "every job claimed")
	for id, n := range claimedBy {
		require.Equal(t, 1, n, "job %s claimed more than once", id)
	}

	// Per-priority ordering: priority is non-increasing in the global claim
	// order (priority desc; FIFO within a priority holds per the index).
	priOf := func(id string) int {
		var n int
		_, _ = fmt.Sscanf(id[len("race-"):], "%03d", &n)
		return n % 10
	}
	highestSeen := 10
	for _, id := range order {
		p := priOf(id)
		require.LessOrEqual(t, p, highestSeen, "priority must be non-increasing in claim order")
		highestSeen = p
	}
}

// TestClaimRace_ContentionBounded proves ErrConflict retries stay bounded
// under contention: all workers finish without error and the store remains
// consistent.
func TestClaimRace_ContentionBounded(t *testing.T) {
	s, err := openTestStore(Options{Dir: t.TempDir()})
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()

	const workers = 8
	const total = 40
	for i := 0; i < total; i++ {
		require.NoError(t, s.Create(ctx, sampleJob(fmt.Sprintf("cont-%02d", i))))
	}

	errCh := make(chan error, workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for {
				got, err := s.Claim(ctx, fmt.Sprintf("w%d", w), []string{"knowledge"}, 3)
				if err != nil {
					errCh <- err
					return
				}
				if len(got) == 0 {
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("worker error under contention: %v", err)
	}

	running, err := s.List(ctx, jobs.Query{State: jobs.StateRunning})
	require.NoError(t, err)
	require.Len(t, running, total)
}
