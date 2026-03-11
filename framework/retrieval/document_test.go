package retrieval

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCanonicalizeURI_NormalizesLocalPaths(t *testing.T) {
	require.Equal(t, "docs/spec.md", CanonicalizeURI("./docs/../docs/spec.md"))
	require.Equal(t, "https://example.com/a/b", CanonicalizeURI("https://example.com/a/b"))
}

func TestDocumentIdentityIsStableForEquivalentPaths(t *testing.T) {
	now := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)
	a := NewDocumentRecord("./framework/../framework/core/context.go", []byte("hello"), "go", "v1", "workspace", []string{"internal"}, now, now)
	b := NewDocumentRecord("framework/core/context.go", []byte("hello"), "go", "v1", "workspace", []string{"internal"}, now, now)

	require.Equal(t, a.DocID, b.DocID)
	require.Equal(t, a.CanonicalURI, b.CanonicalURI)
	require.Equal(t, a.ContentHash, b.ContentHash)
	require.Equal(t, "workspace", a.CorpusScope)
	require.Equal(t, []string{"internal"}, a.PolicyTags)
	require.Equal(t, now, a.SourceUpdatedAt)
	require.Equal(t, now, a.LastIngestedAt)
	require.Equal(t, now, a.CreatedAt)
	require.Equal(t, now, a.UpdatedAt)
}

func TestVersionIDChangesWithContentHash(t *testing.T) {
	now := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)
	docV1 := NewDocumentRecord("docs/spec.md", []byte("one"), "markdown", "v1", "workspace", nil, now, now)
	docV2 := NewDocumentRecord("docs/spec.md", []byte("two"), "markdown", "v1", "workspace", nil, now, now)

	require.Equal(t, docV1.DocID, docV2.DocID)
	require.NotEqual(t, docV1.ContentHash, docV2.ContentHash)
	require.NotEqual(t,
		NewDocumentVersionRecord(docV1, now).VersionID,
		NewDocumentVersionRecord(docV2, now).VersionID,
	)
}

func TestChunkIDPrefersStructuralKeyOverOffsets(t *testing.T) {
	now := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)
	chunkA := NewChunkRecord("doc:1", "ver:1", "func x", "pkg/Foo", 10, 20, now)
	chunkB := NewChunkRecord("doc:1", "ver:2", "func x changed", "pkg/Foo", 100, 120, now)
	chunkC := NewChunkRecord("doc:1", "ver:2", "func y", "", 100, 120, now)

	require.Equal(t, chunkA.ChunkID, chunkB.ChunkID)
	require.NotEqual(t, chunkA.ChunkID, chunkC.ChunkID)
	require.Equal(t, "pkg/Foo", chunkA.StructuralKey)
}

func TestChunkVersionMirrorsChunkRecord(t *testing.T) {
	now := time.Date(2026, 3, 11, 12, 0, 0, 0, time.UTC)
	chunk := NewChunkRecord("doc:1", "ver:2", "body", "pkg/Foo", 12, 48, now)
	version := chunk.ChunkVersion()

	require.Equal(t, chunk.ChunkID, version.ChunkID)
	require.Equal(t, chunk.VersionID, version.VersionID)
	require.Equal(t, chunk.Text, version.Text)
	require.Equal(t, chunk.StartOffset, version.StartOffset)
	require.Equal(t, chunk.EndOffset, version.EndOffset)
}
