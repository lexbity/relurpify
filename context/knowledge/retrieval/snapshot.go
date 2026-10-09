package retrieval

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"codeburg.org/lexbit/relurpify/context/knowledge"
)

// snapshotTTL bounds how long a corpus snapshot serves retrieval without a
// refresh. Serve-stale (§ degraded mode) tolerates up to 2×TTL before a
// failed rebuild becomes a hard error.
const snapshotTTL = 30 * time.Second

// CorpusSnapshot is one generation of the retrieval corpus: a single decode
// pass over the store, shared by every ranker for the generation's lifetime.
// A snapshot-lag cannot surface a tombstoned or stale-marked chunk into
// context — admission re-checks chunk state by ID through the store's
// tombstone-filtering Load.
type CorpusSnapshot struct {
	// Chunks is the generation's decoded corpus, in store order.
	Chunks []knowledge.KnowledgeChunk
	// Generation monotonically increases per rebuild; 0 means none.
	Generation uint64
	// BuiltAt is the snapshot's build time (TTL/serve-stale accounting).
	BuiltAt time.Time

	byID map[knowledge.ChunkID]int32 // index into Chunks
}

// Lookup returns the chunk with the given ID in O(1).
func (s *CorpusSnapshot) Lookup(id knowledge.ChunkID) (knowledge.KnowledgeChunk, bool) {
	if s == nil {
		return knowledge.KnowledgeChunk{}, false
	}
	if idx, ok := s.byID[id]; ok && int(idx) < len(s.Chunks) {
		return s.Chunks[idx], true
	}
	return knowledge.KnowledgeChunk{}, false
}

// buildCorpusSnapshot decodes the corpus in a single store pass.
func buildCorpusSnapshot(store *knowledge.ChunkStore, generation uint64, builtAt time.Time) (*CorpusSnapshot, error) {
	chunks, err := store.FindAll()
	if err != nil {
		return nil, err
	}
	snap := &CorpusSnapshot{
		Generation: generation,
		Chunks:     chunks,
		byID:       make(map[knowledge.ChunkID]int32, len(chunks)),
		BuiltAt:    builtAt,
	}
	for i, chunk := range chunks {
		snap.byID[chunk.ID] = int32(i)
	}
	return snap, nil
}

// snapshotState is the retriever's snapshot lifecycle: RLock fast path on a
// fresh snapshot, single-flight rebuild otherwise, serve-stale when a rebuild
// fails and the previous generation is still within 2×TTL.
type snapshotState struct {
	mu         sync.RWMutex
	current    *CorpusSnapshot
	generation uint64
	building   atomic.Bool
	now        func() time.Time
	// build is the snapshot constructor; a field so tests can inject
	// build failures for the serve-stale path.
	build func(store *knowledge.ChunkStore, generation uint64, builtAt time.Time) (*CorpusSnapshot, error)
}

func newSnapshotState(now func() time.Time) *snapshotState {
	return &snapshotState{now: now, build: buildCorpusSnapshot}
}

// errBuildTimeout is returned when no single-flight builder published a new
// generation within the wait window.
var errBuildTimeout = errors.New("corpus snapshot rebuild timed out")

// get returns the live snapshot: the current one when fresh, a single-flight
// rebuild when stale, and — only when the rebuild fails — the previous
// generation while it is within 2×TTL (serve-stale). Beyond that window a
// failed build is a hard error: context compilation without retrieval is a
// lie the compiler must not tell.
// getOutcome distinguishes how the snapshot was obtained (telemetry only).
type getOutcome int

const (
	// outcomeFresh means the live snapshot was within TTL and served as-is.
	outcomeFresh getOutcome = iota
	// outcomeBuilt means this call rebuilt the snapshot.
	outcomeBuilt
	// outcomeServedStale means the rebuild failed and the previous
	// generation was served within the serve-stale window.
	outcomeServedStale
)

func (ss *snapshotState) get(store *knowledge.ChunkStore) (*CorpusSnapshot, getOutcome, error) {
	ss.mu.RLock()
	snap := ss.current
	ss.mu.RUnlock()

	if snap != nil && ss.now().Sub(snap.BuiltAt) < snapshotTTL {
		return snap, outcomeFresh, nil
	}

	// Single-flight: the first stale reader rebuilds; the rest wait bounded
	// for a newer generation to publish.
	staleGeneration := uint64(0)
	if snap != nil {
		staleGeneration = snap.Generation
	}
	if ss.building.CompareAndSwap(false, true) {
		defer ss.building.Store(false)
		next := staleGeneration + 1
		built, err := ss.build(store, next, ss.now())
		if err != nil {
			ss.mu.RLock()
			previous := ss.current
			ss.mu.RUnlock()
			if previous != nil && ss.now().Sub(previous.BuiltAt) < 2*snapshotTTL {
				return previous, outcomeServedStale, nil
			}
			return nil, outcomeFresh, err
		}
		ss.mu.Lock()
		ss.generation = next
		ss.current = built
		ss.mu.Unlock()
		return built, outcomeBuilt, nil
	}

	// Wait for the in-flight builder (invalidate drops to nil, so a
	// concurrently published rebuild always carries a newer generation).
	deadline := time.Now().Add(snapshotTTL)
	for time.Now().Before(deadline) {
		ss.mu.RLock()
		snap = ss.current
		ss.mu.RUnlock()
		if snap != nil && snap.Generation > staleGeneration {
			return snap, outcomeBuilt, nil
		}
		time.Sleep(time.Millisecond)
	}
	return nil, outcomeFresh, errBuildTimeout
}

// invalidate drops the current snapshot so the next get rebuilds (generation
// bump). Called on chunk lifecycle events and explicitly by the ingester.
func (ss *snapshotState) invalidate() {
	ss.mu.Lock()
	ss.current = nil
	ss.mu.Unlock()
}
