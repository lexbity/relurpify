package contextdata

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrBranchOrder is returned by ApplyBranchMerges when the supplied units are
// not in strictly ascending Index order. Unit order is the caller's obligation
// because the merge is defined in terms of declaration order, not slice order.
var ErrBranchOrder = errors.New("contextdata: branch merge units must be in strictly ascending index order")

// BranchMergeUnit carries one branch's delta plus its declaration index.
//
// Index is the branch's edge-declaration order (0-based); units MUST be passed
// to ApplyBranchMerges in strictly ascending Index order. Delta is computed
// relative to the common fork-time parent state (see ComputeBranchDelta). Env
// is the branch-final envelope and supplies the values and references that the
// delta selects.
type BranchMergeUnit struct {
	Index int
	ID    string
	Delta BranchDelta
	Env   *Envelope
}

// MergeStats reports what a merge did. Conflicted keys are sorted.
type MergeStats struct {
	// UnitsApplied is the number of merge units processed.
	UnitsApplied int
	// KeysWritten is the number of working-memory keys installed (adds plus
	// modifications) on the parent.
	KeysWritten int
	// KeysDeleted is the number of working-memory keys removed from the parent.
	KeysDeleted int
	// KeysSkipped is the number of delta-named keys whose value was absent from
	// the unit envelope. Defensive: unreachable when deltas and envelopes come
	// from the same branch execution, but counted so a mismatch is observable.
	KeysSkipped int
	// Conflicts lists, sorted, every key mentioned by more than one unit. The
	// last mentioning unit wins; the list exists for observability, not to
	// reject the merge.
	Conflicts []string
	// RefsStreamed is the number of streamed-context references newly unioned
	// into the parent.
	RefsStreamed int
	// RefsRetrieval is the number of retrieval references newly unioned into
	// the parent.
	RefsRetrieval int
}

// ApplyBranchMerges applies per-branch deltas onto the receiver in a single
// critical section, then unions branch references. It is the envelope-owned
// replacement for the former snapshot-union merge: the envelope, not the
// graph, performs its own state transition.
//
// Unit ordering (D1/D2): units are applied in strictly ascending Index order.
// For each working-memory key the last unit that mentions it wins — a write
// installs that unit's value, a delete removes the key, and silence preserves
// the prior state. Because deltas are relative to the common fork-time parent
// state, the composition is order-sensitive only through Index, never through
// goroutine scheduling.
//
// Conflict policy: a key mentioned by more than one unit is resolved by the
// higher Index; it is recorded in MergeStats.Conflicts rather than rejected.
//
// ApplyBranchMerges is single-use: calling it twice with the same units
// re-applies the deltas and is not idempotent. The caller must compute each
// unit's Delta before calling (after the parent has quiesced, so the fork-time
// base still equals the receiver's state).
func (e *Envelope) ApplyBranchMerges(units []BranchMergeUnit) (MergeStats, error) {
	var stats MergeStats
	if e == nil {
		return stats, nil
	}
	if err := validateBranchOrder(units); err != nil {
		return stats, err
	}
	if len(units) == 0 {
		return stats, nil
	}

	// Snapshot each unit's working data and references before taking the
	// receiver's write lock. Snapshotting takes the unit's own read lock; the
	// two locks must never nest, and the units are goroutine-isolated once the
	// parent has quiesced.
	prepared := make([]preparedBranchMergeUnit, 0, len(units))
	for _, u := range units {
		p := preparedBranchMergeUnit{unit: u}
		if u.Env != nil {
			p.values = u.Env.WorkingDataSnapshot()
			p.refs = u.Env.ReferencesSnapshot()
		}
		prepared = append(prepared, p)
	}

	// Resolve the effective write/delete set from per-unit deltas. Iteration
	// order inside a unit is irrelevant to the resulting map state; Index order
	// across units is what matters and is enforced above.
	writes := make(map[string]any)
	deletes := make(map[string]struct{})
	touched := make(map[string]int)
	for _, p := range prepared {
		for _, k := range sortedUniqueStrings(p.unit.Delta.WorkingMemoryDeleted) {
			deletes[k] = struct{}{}
			delete(writes, k)
			touched[k]++
		}
		writeKeys := append(append([]string(nil), p.unit.Delta.WorkingMemoryAdded...), p.unit.Delta.WorkingMemoryModified...)
		for _, k := range sortedUniqueStrings(writeKeys) {
			if p.values == nil {
				continue
			}
			v, ok := p.values[k]
			if !ok {
				// Defensive: a delta that names a key its envelope does not
				// hold is skipped rather than fabricating a value.
				stats.KeysSkipped++
				continue
			}
			writes[k] = v
			delete(deletes, k)
			touched[k]++
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if len(writes) > 0 && e.WorkingData == nil {
		e.WorkingData = make(map[string]any)
	}
	for k := range deletes {
		if _, present := e.WorkingData[k]; present {
			stats.KeysDeleted++
		}
		delete(e.WorkingData, k)
	}
	for k, v := range writes {
		e.WorkingData[k] = v
		stats.KeysWritten++
	}
	for k, count := range touched {
		if count > 1 {
			stats.Conflicts = append(stats.Conflicts, k)
		}
	}
	sort.Strings(stats.Conflicts)

	e.mergeBranchReferencesLocked(prepared, deletes, &stats)

	stats.UnitsApplied = len(units)
	return stats, nil
}

// preparedBranchMergeUnit is the merge-internal view of a unit: the declared
// unit plus the snapshots taken before the receiver's lock is acquired.
type preparedBranchMergeUnit struct {
	unit   BranchMergeUnit
	values map[string]any
	refs   ReferenceBundle
}

// mergeBranchReferencesLocked unions branch references into the receiver. The
// caller must hold e.mu for writing. References dedup by ID with the first
// occurrence winning position; streamed context is re-sorted by Rank afterward
// for determinism. Working-memory references for keys deleted by this merge
// are dropped so the reference tier stays coherent with WorkingData.
func (e *Envelope) mergeBranchReferencesLocked(
	prepared []preparedBranchMergeUnit,
	deletes map[string]struct{},
	stats *MergeStats,
) {
	seenChunks := make(map[ChunkID]struct{}, len(e.References.StreamedContext))
	for _, ref := range e.References.StreamedContext {
		seenChunks[ref.ChunkID] = struct{}{}
	}
	seenWorking := make(map[string]struct{}, len(e.References.WorkingMemory))
	for _, ref := range e.References.WorkingMemory {
		seenWorking[workingMemoryReferenceKey(ref)] = struct{}{}
	}
	seenRetrieval := make(map[string]struct{}, len(e.References.Retrieval))
	for _, ref := range e.References.Retrieval {
		seenRetrieval[ref.QueryID] = struct{}{}
	}
	checkpointIndex := make(map[string]int, len(e.References.Checkpoints))
	for i, ref := range e.References.Checkpoints {
		checkpointIndex[ref.CheckpointID] = i
	}

	if len(deletes) > 0 && len(e.References.WorkingMemory) > 0 {
		kept := e.References.WorkingMemory[:0]
		for _, ref := range e.References.WorkingMemory {
			if ref.TaskID == e.TaskID {
				if _, gone := deletes[ref.Key]; gone {
					delete(seenWorking, workingMemoryReferenceKey(ref))
					continue
				}
			}
			kept = append(kept, ref)
		}
		e.References.WorkingMemory = kept
	}

	for _, p := range prepared {
		for _, ref := range p.refs.StreamedContext {
			if _, seen := seenChunks[ref.ChunkID]; seen {
				continue
			}
			seenChunks[ref.ChunkID] = struct{}{}
			e.References.StreamedContext = append(e.References.StreamedContext, ref)
			stats.RefsStreamed++
		}
		for _, ref := range p.refs.WorkingMemory {
			if ref.TaskID == e.TaskID {
				if _, gone := deletes[ref.Key]; gone {
					continue
				}
			}
			key := workingMemoryReferenceKey(ref)
			if _, seen := seenWorking[key]; seen {
				continue
			}
			seenWorking[key] = struct{}{}
			e.References.WorkingMemory = append(e.References.WorkingMemory, ref)
		}
		for _, ref := range p.refs.Retrieval {
			if _, seen := seenRetrieval[ref.QueryID]; seen {
				continue
			}
			seenRetrieval[ref.QueryID] = struct{}{}
			e.References.Retrieval = append(e.References.Retrieval, ref)
			stats.RefsRetrieval++
		}
		for _, ref := range p.refs.Checkpoints {
			if idx, seen := checkpointIndex[ref.CheckpointID]; seen {
				e.References.Checkpoints[idx].WorkingMemoryKeys = mergeStringSets(
					e.References.Checkpoints[idx].WorkingMemoryKeys, ref.WorkingMemoryKeys)
				continue
			}
			checkpointIndex[ref.CheckpointID] = len(e.References.Checkpoints)
			e.References.Checkpoints = append(e.References.Checkpoints, cloneCheckpointReference(ref))
		}
	}

	sort.SliceStable(e.References.StreamedContext, func(i, j int) bool {
		return e.References.StreamedContext[i].Rank < e.References.StreamedContext[j].Rank
	})
	for i := range e.References.Checkpoints {
		e.References.Checkpoints[i].WorkingMemoryKeys = dedupeStrings(
			e.References.Checkpoints[i].WorkingMemoryKeys)
	}
}

func validateBranchOrder(units []BranchMergeUnit) error {
	for i := 1; i < len(units); i++ {
		if units[i].Index <= units[i-1].Index {
			return fmt.Errorf("%w: units[%d].Index=%d not greater than units[%d].Index=%d",
				ErrBranchOrder, i, units[i].Index, i-1, units[i-1].Index)
		}
	}
	return nil
}

func workingMemoryReferenceKey(ref WorkingMemoryReference) string {
	return ref.TaskID + "/" + ref.Key
}

// sortedUniqueStrings returns the non-empty, de-duplicated members of values in
// ascending order. Sorting makes per-unit processing order deterministic even
// though the merge itself is order-insensitive within a unit.
func sortedUniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// mergeStringSets unions two string slices, trimming blanks and keeping first
// occurrence order. It is the ordering-preserving counterpart to
// sortedUniqueStrings and is used for checkpoint working-memory key sets.
func mergeStringSets(base, extra []string) []string {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	add := func(values []string) {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	add(base)
	add(extra)
	return out
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return mergeStringSets(nil, values)
}
