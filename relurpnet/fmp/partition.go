package fmp

import (
	"sync/atomic"
)

// PartitionDetector is an interface for detecting partition states.
type PartitionDetector interface {
	IsPartitioned() bool
}

// AtomicPartitionState provides a thread-safe partition state indicator using atomic operations.
type AtomicPartitionState struct {
	partitioned atomic.Bool
}

// IsPartitioned returns true if the partition state is marked as partitioned.
func (p *AtomicPartitionState) IsPartitioned() bool {
	if p == nil {
		return false
	}
	return p.partitioned.Load()
}

// SetPartitioned sets the partition state.
func (p *AtomicPartitionState) SetPartitioned(v bool) {
	if p == nil {
		return
	}
	p.partitioned.Store(v)
}
