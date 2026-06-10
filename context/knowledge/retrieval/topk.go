package retrieval

import (
	"container/heap"
	"sort"

	"codeburg.org/lexbit/relurpify/context/knowledge"
)

// boundedTopK collects up to K chunk IDs with optional recency preference.
// Two regimes:
//   - PreferLatest: min-heap on UpdatedAt; keeps the K most recent across
//     all inserted items (O(N log K) time, O(K) memory).
//   - !PreferLatest: capped insertion-order slice; early-stops at K
//     (O(K) time and memory). Items beyond the first K are silently dropped.
type boundedTopK struct {
	k            int
	preferLatest bool
	seen         map[knowledge.ChunkID]struct{}

	// insertion-order regime
	insertOrder []knowledge.ChunkID

	// recency regime (min-heap on updatedAt)
	recencyHeap recencyMinHeap
}

// recencyMinHeap implements heap.Interface for the PreferLatest regime.
type recencyMinHeap []recencyItem

type recencyItem struct {
	id        knowledge.ChunkID
	updatedAt int64
}

func (h recencyMinHeap) Len() int            { return len(h) }
func (h recencyMinHeap) Less(i, j int) bool  { return h[i].updatedAt < h[j].updatedAt }
func (h recencyMinHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *recencyMinHeap) Push(x any)         { *h = append(*h, x.(recencyItem)) }
func (h *recencyMinHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

func newBoundedTopK(k int, preferLatest bool) *boundedTopK {
	if k <= 0 {
		k = defaultMaxCandidates
	}
	return &boundedTopK{
		k:            k,
		preferLatest: preferLatest,
		seen:         make(map[knowledge.ChunkID]struct{}),
	}
}

// offer inserts a node into the bounded collection. Returns true when the
// item was accepted (not a duplicate).
func (b *boundedTopK) offer(node knowledge.ChunkID, updatedAt int64) bool {
	if _, ok := b.seen[node]; ok {
		return false
	}
	b.seen[node] = struct{}{}

	if b.preferLatest {
		// Min-heap on updatedAt: keep only the K most recent.
		if len(b.recencyHeap) < b.k {
			heap.Push(&b.recencyHeap, recencyItem{id: node, updatedAt: updatedAt})
		} else if updatedAt > b.recencyHeap[0].updatedAt {
			heap.Pop(&b.recencyHeap)
			heap.Push(&b.recencyHeap, recencyItem{id: node, updatedAt: updatedAt})
		}
		return true
	}

	// Insertion-order regime: fill up to K, then stop.
	if len(b.insertOrder) < b.k {
		b.insertOrder = append(b.insertOrder, node)
	}
	return true
}

// full reports whether the collector has reached its budget. Only meaningful
// in the insertion-order regime — the recency regime never stops early.
func (b *boundedTopK) full() bool {
	if b.preferLatest {
		return false
	}
	return len(b.insertOrder) >= b.k
}

// ids returns the collected chunk IDs in the correct order.
func (b *boundedTopK) ids() []knowledge.ChunkID {
	if b.preferLatest {
		// Sort recencyHeap descending by updatedAt (most recent first).
		sort.SliceStable(b.recencyHeap, func(i, j int) bool {
			if b.recencyHeap[i].updatedAt == b.recencyHeap[j].updatedAt {
				return i < j
			}
			return b.recencyHeap[i].updatedAt > b.recencyHeap[j].updatedAt
		})
		out := make([]knowledge.ChunkID, len(b.recencyHeap))
		for i, item := range b.recencyHeap {
			out[i] = item.id
		}
		return out
	}
	return b.insertOrder
}
