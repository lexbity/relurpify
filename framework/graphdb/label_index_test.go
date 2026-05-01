package graphdb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLabelIndex_AddLookup(t *testing.T) {
	index := NewLabelIndex()
	index.Add("coverage_hash:abc", "n1")
	index.Add("coverage_hash:abc", "n2")
	index.Add("file_path:src/main.go", "n3")

	require.ElementsMatch(t, []string{"n1", "n2"}, index.Lookup("coverage_hash:abc"))
	require.ElementsMatch(t, []string{"n3"}, index.Lookup("file_path:src/main.go"))
	require.Empty(t, index.Lookup("missing"))
}

func TestLabelIndex_Remove(t *testing.T) {
	index := NewLabelIndex()
	index.Add("coverage_hash:abc", "n1")
	index.Add("coverage_hash:abc", "n2")

	index.Remove("coverage_hash:abc", "n1")
	require.ElementsMatch(t, []string{"n2"}, index.Lookup("coverage_hash:abc"))

	index.Remove("coverage_hash:abc", "n2")
	require.Empty(t, index.Lookup("coverage_hash:abc"))
}

func TestLabelIndex_RebuildFromAOF(t *testing.T) {
	engine, opts := newTestEngine(t)
	require.NoError(t, engine.UpsertNode(NodeRecord{
		ID:     "n1",
		Kind:   "chunk",
		Labels: []string{"coverage_hash:abc", "file_path:src/main.go"},
	}))
	require.NoError(t, engine.UpsertNode(NodeRecord{
		ID:     "n2",
		Kind:   "chunk",
		Labels: []string{"coverage_hash:def"},
	}))
	require.NoError(t, engine.DeleteNode("n2"))
	require.NoError(t, engine.Close())

	reopened, err := Open(opts)
	require.NoError(t, err)
	defer reopened.Close()

	coverage := reopened.ListNodesByLabel("chunk", "coverage_hash:abc")
	require.Len(t, coverage, 1)
	require.Equal(t, "n1", coverage[0].ID)

	prefixed := reopened.ListNodesByLabelPrefix("chunk", "file_path:src")
	require.Len(t, prefixed, 1)
	require.Equal(t, "n1", prefixed[0].ID)
	require.Empty(t, reopened.ListNodesByLabel("chunk", "coverage_hash:def"))
}
