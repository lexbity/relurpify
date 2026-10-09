package persistence

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/governance/identity"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

func newWriterTestStore(t *testing.T) *knowledge.ChunkStore {
	t.Helper()
	engine, err := graphdb.Open(context.Background(), graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close(context.Background())) })
	return &knowledge.ChunkStore{Graph: engine}
}

// countingEventLog records the persistence events emitted through the writer.
type countingEventLog struct {
	mu    sync.Mutex
	types []string
}

func (l *countingEventLog) Emit(eventType string, _ map[string]any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.types = append(l.types, eventType)
}

func (l *countingEventLog) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.types)
}

func (l *countingEventLog) countType(eventType string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	count := 0
	for _, recorded := range l.types {
		if recorded == eventType {
			count++
		}
	}
	return count
}

func dedupRequest(content string) PersistenceRequest {
	return PersistenceRequest{
		Content:         []byte(content),
		Kind:            knowledge.ChunkKindDerivation,
		ContentType:     "text/plain",
		SourcePrincipal: identity.SubjectRef{Kind: identity.SubjectKindUser, ID: "principal-1"},
		SourceOrigin:    knowledge.SourceOriginDerivation,
	}
}

// TestWriterDedupConvergesOnOneChunk proves identical content persisted twice
// produces one chunk with one identity and one commit event.
func TestWriterDedupConvergesOnOneChunk(t *testing.T) {
	store := newWriterTestStore(t)
	events := &countingEventLog{}
	writer := NewWriter(store, events, nil, nil)

	first, err := writer.Persist(context.Background(), dedupRequest("identical payload"))
	require.NoError(t, err)
	require.Equal(t, ActionCreated, first.Action)

	second, err := writer.Persist(context.Background(), dedupRequest("identical payload"))
	require.NoError(t, err)
	require.Equal(t, ActionUpdated, second.Action)
	require.Equal(t, first.ChunkID, second.ChunkID)

	all, err := store.FindAll()
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.Equal(t, first.ChunkID, all[0].ID)

	require.Equal(t, 1, events.count(), "dedup must not emit a second commit event")
}

// TestWriterDistinctContentProducesDistinctChunks proves the dedup lookup does
// not collapse distinct content.
func TestWriterDistinctContentProducesDistinctChunks(t *testing.T) {
	store := newWriterTestStore(t)
	writer := NewWriter(store, nil, nil, nil)

	first, err := writer.Persist(context.Background(), dedupRequest("payload-a"))
	require.NoError(t, err)
	second, err := writer.Persist(context.Background(), dedupRequest("payload-b"))
	require.NoError(t, err)
	require.NotEqual(t, first.ChunkID, second.ChunkID)

	all, err := store.FindAll()
	require.NoError(t, err)
	require.Len(t, all, 2)
}

// TestWriterRequiresCanonicalKind proves structural validation rejects a
// request that cannot carry a canonical identity.
func TestWriterRequiresCanonicalKind(t *testing.T) {
	writer := NewWriter(nil, nil, nil, nil)
	result, err := writer.Persist(context.Background(), PersistenceRequest{
		Content:         []byte("x"),
		ContentType:     "text/plain",
		SourcePrincipal: identity.SubjectRef{ID: "p"},
	})
	require.Error(t, err)
	require.NotNil(t, result)
	require.Equal(t, ActionRejected, result.Action)
}

// TestWriterQuarantineUnaffectedByDedup proves the suspicion path still
// quarantines before any store write.
func TestWriterQuarantineUnaffectedByDedup(t *testing.T) {
	store := newWriterTestStore(t)
	writer := NewWriter(store, nil, nil, nil)

	req := dedupRequest("has null byte")
	req.Content = append(req.Content, 0)
	result, err := writer.Persist(context.Background(), req)
	require.Error(t, err)
	require.Equal(t, ActionQuarantined, result.Action)

	all, err := store.FindAll()
	require.NoError(t, err)
	require.Empty(t, all)
}

// TestWriterPreservesTombstone proves an idempotent re-persist of retracted
// content does not resurrect it and is reported as a preserved tombstone.
func TestWriterPreservesTombstone(t *testing.T) {
	store := newWriterTestStore(t)
	events := &countingEventLog{}
	writer := NewWriter(store, events, nil, nil)

	first, err := writer.Persist(context.Background(), dedupRequest("retracted payload"))
	require.NoError(t, err)
	require.NoError(t, store.Tombstone(context.Background(), first.ChunkID, ""))

	second, err := writer.Persist(context.Background(), dedupRequest("retracted payload"))
	require.NoError(t, err)
	require.Equal(t, first.ChunkID, second.ChunkID)

	stored, ok, err := store.LoadIncludingTombstoned(first.ChunkID)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, stored.Tombstoned, "tombstone must survive an equal-derivation re-persist")

	require.Equal(t, 1, events.countType(string(telemetry.EventChunkCommitted)))
	require.Equal(t, 1, events.countType(string(telemetry.EventTombstonePreserved)))
}
