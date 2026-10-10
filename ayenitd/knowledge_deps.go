// knowledge_deps.go builds the knowledge handler dependencies from a
// workspace + state dir: the AST index engine, the index manager, the chunk
// store, and the staleness manager. main.go stays thin; the e2e and the
// binary share one wiring path.
package ayenitd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
)

// KnowledgeRunnerDeps bundles the handler dependencies Run needs for the
// day-one knowledge job kinds. Close releases the graph engine.
type KnowledgeRunnerDeps struct {
	IndexManager *ast.IndexManager
	ChunkStore   *knowledge.ChunkStore
	Staleness    *knowledge.StalenessManager

	close func()
}

// Close releases the engine. Safe to call more than once.
func (d *KnowledgeRunnerDeps) Close() {
	if d.close != nil {
		d.close()
	}
}

// BuildKnowledgeRunnerDeps opens the AST index engine under
// <stateDir>/ast and wires the knowledge handler stack against workspace.
func BuildKnowledgeRunnerDeps(ctx context.Context, workspace, stateDir string) (*KnowledgeRunnerDeps, error) {
	indexDir := filepath.Join(stateDir, "ast")
	if err := os.MkdirAll(indexDir, 0o700); err != nil {
		return nil, fmt.Errorf("knowledge deps: create index dir: %w", err)
	}
	engine, err := graphdb.Open(ctx, graphdb.DefaultOptions(indexDir))
	if err != nil {
		return nil, fmt.Errorf("knowledge deps: open index engine: %w", err)
	}
	indexStore := ast.NewGraphIndexStore(engine)
	indexManager := ast.NewIndexManager(indexStore, ast.IndexConfig{
		WorkspacePath:   workspace,
		ParallelWorkers: 2,
	})
	indexManager.GraphDB = engine
	chunkStore := &knowledge.ChunkStore{Graph: engine}
	return &KnowledgeRunnerDeps{
		IndexManager: indexManager,
		ChunkStore:   chunkStore,
		Staleness:    &knowledge.StalenessManager{Store: chunkStore, Propagate: true, MaxDepth: 3},
		close:        func() { _ = engine.Close(context.Background()) },
	}, nil
}
