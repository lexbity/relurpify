package perfstats

import (
	"sync/atomic"
	"time"
)

type snapshot struct {
	BranchClones                     int64
	BranchMergeCount                 int64
	BranchMergeDurationNanos         int64
	ContextBudgetRescanCount         int64
	ContextBudgetRescanItems         int64
	ProgressiveFileReadCount         int64
	ProgressiveFileRereadCount       int64
	ProgressiveDemotionReadCount     int64
	RetrievalSchemaCheckCount        int64
	RetrievalCorpusStampCount        int64
	RuntimeMemorySearchCount         int64
	RuntimeMemorySearchDurationNanos int64
	CapabilityRegistryRebuildCount   int64
}

var counters snapshot

func Reset() {
	atomic.StoreInt64(&counters.BranchClones, 0)
	atomic.StoreInt64(&counters.BranchMergeCount, 0)
	atomic.StoreInt64(&counters.BranchMergeDurationNanos, 0)
	atomic.StoreInt64(&counters.ContextBudgetRescanCount, 0)
	atomic.StoreInt64(&counters.ContextBudgetRescanItems, 0)
	atomic.StoreInt64(&counters.ProgressiveFileReadCount, 0)
	atomic.StoreInt64(&counters.ProgressiveFileRereadCount, 0)
	atomic.StoreInt64(&counters.ProgressiveDemotionReadCount, 0)
	atomic.StoreInt64(&counters.RetrievalSchemaCheckCount, 0)
	atomic.StoreInt64(&counters.RetrievalCorpusStampCount, 0)
	atomic.StoreInt64(&counters.RuntimeMemorySearchCount, 0)
	atomic.StoreInt64(&counters.RuntimeMemorySearchDurationNanos, 0)
	atomic.StoreInt64(&counters.CapabilityRegistryRebuildCount, 0)
}

type Snapshot struct {
	BranchClones                     int64
	BranchMergeCount                 int64
	BranchMergeDurationNanos         int64
	ContextBudgetRescanCount         int64
	ContextBudgetRescanItems         int64
	ProgressiveFileReadCount         int64
	ProgressiveFileRereadCount       int64
	ProgressiveDemotionReadCount     int64
	RetrievalSchemaCheckCount        int64
	RetrievalCorpusStampCount        int64
	RuntimeMemorySearchCount         int64
	RuntimeMemorySearchDurationNanos int64
	CapabilityRegistryRebuildCount   int64
}

func Get() Snapshot {
	return Snapshot{
		BranchClones:                     atomic.LoadInt64(&counters.BranchClones),
		BranchMergeCount:                 atomic.LoadInt64(&counters.BranchMergeCount),
		BranchMergeDurationNanos:         atomic.LoadInt64(&counters.BranchMergeDurationNanos),
		ContextBudgetRescanCount:         atomic.LoadInt64(&counters.ContextBudgetRescanCount),
		ContextBudgetRescanItems:         atomic.LoadInt64(&counters.ContextBudgetRescanItems),
		ProgressiveFileReadCount:         atomic.LoadInt64(&counters.ProgressiveFileReadCount),
		ProgressiveFileRereadCount:       atomic.LoadInt64(&counters.ProgressiveFileRereadCount),
		ProgressiveDemotionReadCount:     atomic.LoadInt64(&counters.ProgressiveDemotionReadCount),
		RetrievalSchemaCheckCount:        atomic.LoadInt64(&counters.RetrievalSchemaCheckCount),
		RetrievalCorpusStampCount:        atomic.LoadInt64(&counters.RetrievalCorpusStampCount),
		RuntimeMemorySearchCount:         atomic.LoadInt64(&counters.RuntimeMemorySearchCount),
		RuntimeMemorySearchDurationNanos: atomic.LoadInt64(&counters.RuntimeMemorySearchDurationNanos),
		CapabilityRegistryRebuildCount:   atomic.LoadInt64(&counters.CapabilityRegistryRebuildCount),
	}
}

func IncBranchClone() {
	atomic.AddInt64(&counters.BranchClones, 1)
}

func ObserveBranchMerge(duration time.Duration) {
	atomic.AddInt64(&counters.BranchMergeCount, 1)
	atomic.AddInt64(&counters.BranchMergeDurationNanos, duration.Nanoseconds())
}

func ObserveContextBudgetRescan(itemCount int) {
	atomic.AddInt64(&counters.ContextBudgetRescanCount, 1)
	atomic.AddInt64(&counters.ContextBudgetRescanItems, int64(itemCount))
}

func IncProgressiveFileRead(reread bool, demotion bool) {
	atomic.AddInt64(&counters.ProgressiveFileReadCount, 1)
	if reread {
		atomic.AddInt64(&counters.ProgressiveFileRereadCount, 1)
	}
	if demotion {
		atomic.AddInt64(&counters.ProgressiveDemotionReadCount, 1)
	}
}

func IncRetrievalSchemaCheck() {
	atomic.AddInt64(&counters.RetrievalSchemaCheckCount, 1)
}

func IncRetrievalCorpusStamp() {
	atomic.AddInt64(&counters.RetrievalCorpusStampCount, 1)
}

func ObserveRuntimeMemorySearch(duration time.Duration) {
	atomic.AddInt64(&counters.RuntimeMemorySearchCount, 1)
	atomic.AddInt64(&counters.RuntimeMemorySearchDurationNanos, duration.Nanoseconds())
}

func IncCapabilityRegistryRebuild() {
	atomic.AddInt64(&counters.CapabilityRegistryRebuildCount, 1)
}
