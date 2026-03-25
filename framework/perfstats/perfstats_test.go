package perfstats

import (
	"testing"
	"time"
)

func TestResetAndSnapshot(t *testing.T) {
	Reset()
	IncBranchClone()
	ObserveBranchMerge(5 * time.Millisecond)
	ObserveContextBudgetRescan(12)
	IncProgressiveFileRead(true, true)
	IncRetrievalSchemaCheck()
	IncRetrievalCorpusStamp()
	ObserveRuntimeMemorySearch(7 * time.Millisecond)
	IncCapabilityRegistryRebuild()

	got := Get()
	if got.BranchClones != 1 {
		t.Fatalf("BranchClones = %d", got.BranchClones)
	}
	if got.BranchMergeCount != 1 || got.BranchMergeDurationNanos == 0 {
		t.Fatalf("unexpected branch merge stats: %+v", got)
	}
	if got.ContextBudgetRescanCount != 1 || got.ContextBudgetRescanItems != 12 {
		t.Fatalf("unexpected budget rescan stats: %+v", got)
	}
	if got.ProgressiveFileReadCount != 1 || got.ProgressiveFileRereadCount != 1 || got.ProgressiveDemotionReadCount != 1 {
		t.Fatalf("unexpected progressive loader stats: %+v", got)
	}
	if got.RetrievalSchemaCheckCount != 1 || got.RetrievalCorpusStampCount != 1 {
		t.Fatalf("unexpected retrieval stats: %+v", got)
	}
	if got.RuntimeMemorySearchCount != 1 || got.RuntimeMemorySearchDurationNanos == 0 {
		t.Fatalf("unexpected runtime memory stats: %+v", got)
	}
	if got.CapabilityRegistryRebuildCount != 1 {
		t.Fatalf("unexpected registry rebuild stats: %+v", got)
	}

	Reset()
	got = Get()
	if got != (Snapshot{}) {
		t.Fatalf("expected zero snapshot after reset, got %+v", got)
	}
}
