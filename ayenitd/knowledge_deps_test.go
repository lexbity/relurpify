package ayenitd

// knowledge_deps_test.go covers BuildKnowledgeRunnerDeps: the runner's
// handler-dependency wiring (AST index engine, index manager, chunk store,
// staleness manager) opened against a workspace + state dir.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildKnowledgeRunnerDeps(t *testing.T) {
	workspace := t.TempDir()
	stateDir := t.TempDir()

	deps, err := BuildKnowledgeRunnerDeps(context.Background(), workspace, stateDir)
	require.NoError(t, err)
	require.NotNil(t, deps.IndexManager)
	require.NotNil(t, deps.ChunkStore)
	require.NotNil(t, deps.Staleness)

	// Close releases the underlying engine; safe to call twice.
	deps.Close()
	deps.Close()
}
