package graphdb

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIndexFileMeta_Basic(t *testing.T) {
	engine, _ := newTestEngine(t)

	meta := FileMeta{
		Path:        "src/main.go",
		ContentHash: "sha256:abc123",
		MediaType:   "text/x-go",
		SizeBytes:   1024,
	}
	err := engine.IndexFileMeta("file:1", "workspace:/root", []string{"tag:code"}, meta)
	require.NoError(t, err)

	node, ok := engine.GetNode("file:1")
	require.True(t, ok)
	require.Equal(t, NodeKind("file:text/x-go"), node.Kind)
	require.Equal(t, "workspace:/root", node.SourceID)
	require.Equal(t, "path:src/main.go", node.StableID)
	require.Contains(t, node.Labels, "media:text/x-go")
	require.Contains(t, node.Labels, "hash:sha256:abc123")
	require.Contains(t, node.Labels, "tag:code")
}

func TestIndexFileMeta_MediaTypeKind(t *testing.T) {
	engine, _ := newTestEngine(t)

	err := engine.IndexFileMeta("img:1", "ws:/", nil, FileMeta{
		Path:      "assets/screenshot.png",
		MediaType: "image/png",
	})
	require.NoError(t, err)

	nodes := engine.ListNodes("file:image/png")
	require.Len(t, nodes, 1)
}

func TestFileMetaFromNode(t *testing.T) {
	engine, _ := newTestEngine(t)

	err := engine.IndexFileMeta("f:1", "ws:/", nil, FileMeta{
		Path:      "doc/readme.md",
		MediaType: "text/markdown",
		SizeBytes: 500,
	})
	require.NoError(t, err)

	node, ok := engine.GetNode("f:1")
	require.True(t, ok)

	meta, err := FileMetaFromNode(node)
	require.NoError(t, err)
	require.Equal(t, "doc/readme.md", meta.Path)
	require.Equal(t, "text/markdown", meta.MediaType)
	require.Equal(t, int64(500), meta.SizeBytes)
}

func TestQueryFileMetaByMedia(t *testing.T) {
	engine, _ := newTestEngine(t)

	require.NoError(t, engine.IndexFileMeta("a", "ws:/", nil, FileMeta{Path: "a.png", MediaType: "image/png"}))
	require.NoError(t, engine.IndexFileMeta("b", "ws:/", nil, FileMeta{Path: "b.png", MediaType: "image/png"}))
	require.NoError(t, engine.IndexFileMeta("c", "ws:/", nil, FileMeta{Path: "c.go", MediaType: "text/x-go"}))

	nodes := engine.QueryFileMetaByMedia("image/png")
	require.Len(t, nodes, 2)

	nodes = engine.QueryFileMetaByMedia("text/x-go")
	require.Len(t, nodes, 1)

	nodes = engine.QueryFileMetaByMedia("unknown")
	require.Empty(t, nodes)
}

func TestQueryFileMetaByHash(t *testing.T) {
	engine, _ := newTestEngine(t)

	require.NoError(t, engine.IndexFileMeta("a", "ws:/", nil, FileMeta{Path: "a.go", ContentHash: "sha256:x"}))
	require.NoError(t, engine.IndexFileMeta("b", "ws:/", nil, FileMeta{Path: "b.go", ContentHash: "sha256:y"}))

	nodes := engine.QueryFileMetaByHash("sha256:x")
	require.Len(t, nodes, 1)
	require.Equal(t, "a", nodes[0].ID)

	nodes = engine.QueryFileMetaByHash("sha256:missing")
	require.Empty(t, nodes)
}

func TestQueryFileMetaBySource(t *testing.T) {
	engine, _ := newTestEngine(t)

	require.NoError(t, engine.IndexFileMeta("a", "ws:root", nil, FileMeta{Path: "a.go", MediaType: "text/x-go"}))
	require.NoError(t, engine.IndexFileMeta("b", "ws:root", nil, FileMeta{Path: "b.go", MediaType: "text/x-go"}))

	nodes := engine.QueryFileMetaBySource("ws:root")
	require.Len(t, nodes, 2)
}

func TestPropsTooLarge(t *testing.T) {
	engine, _ := newTestEngine(t)

	// Create a FileMeta with a very long path to trigger the limit.
	longPath := make([]byte, MaxInlinePropsBytes)
	for i := range longPath {
		longPath[i] = 'x'
	}
	meta := FileMeta{
		Path: string(longPath),
	}
	err := engine.IndexFileMeta("oversized", "ws:/", nil, meta)
	require.ErrorIs(t, err, ErrPropsTooLarge)
}

func TestGenerateSyntheticFileMeta(t *testing.T) {
	meta := GenerateSyntheticFileMeta("src/main.go", "text/x-go", 2048)
	require.Equal(t, "src/main.go", meta.Path)
	require.Equal(t, "text/x-go", meta.MediaType)
	require.Equal(t, int64(2048), meta.SizeBytes)
	require.NotEmpty(t, meta.ContentHash)
}

func TestGenerateSyntheticRepo(t *testing.T) {
	engine, _ := newTestEngine(t)
	count := GenerateSyntheticRepo(engine, "/repo", 3, 10)
	// 10 files per type * 8 media types = 80
	require.Equal(t, 80, count)

	// Verify counts by media type
	images := engine.QueryFileMetaByMedia("image/png")
	require.Len(t, images, 10)

	source := engine.QueryFileMetaByMedia("text/x-go")
	require.Len(t, source, 10)

	// All should be queryable by source
	srcNodes := engine.QueryFileMetaBySource("/repo")
	require.Len(t, srcNodes, 80)
}

func TestGenerateSyntheticRepo_Huge(t *testing.T) {
	engine, _ := newTestEngine(t)
	count := GenerateSyntheticRepo(engine, "/big-repo", 100, 5000)
	// 5000 per type * 8 = 40000
	require.Equal(t, 40000, count)

	// Verify memory is bounded (we don't load payload bytes)
	require.Equal(t, 40000, len(engine.store.nodes))

	// Media query
	docs := engine.QueryFileMetaByMedia("text/markdown")
	require.Len(t, docs, 5000)

	// Hash query
	nodes := engine.QueryFileMetaByHash("sha256:" + fmt.Sprintf("%x", []byte("/big-repo/file-doc-0.md")))
	require.Len(t, nodes, 1)
}

func TestFileMetaRoundTripWithEngine(t *testing.T) {
	engine, _ := newTestEngine(t)

	original := FileMeta{
		Path:        "assets/logo.png",
		ContentHash: "sha256:def456",
		MediaType:   "image/png",
		SizeBytes:   481920,
		MTimeUnix:   1780950000,
		ArtifactRef: "artifact://logo",
	}
	require.NoError(t, engine.IndexFileMeta("logo", "ws:/", nil, original))

	node, ok := engine.GetNode("logo")
	require.True(t, ok)

	restored, err := FileMetaFromNode(node)
	require.NoError(t, err)
	require.Equal(t, original.Path, restored.Path)
	require.Equal(t, original.ContentHash, restored.ContentHash)
	require.Equal(t, original.MediaType, restored.MediaType)
	require.Equal(t, original.SizeBytes, restored.SizeBytes)
	require.Equal(t, original.MTimeUnix, restored.MTimeUnix)
	require.Equal(t, original.ArtifactRef, restored.ArtifactRef)
}
