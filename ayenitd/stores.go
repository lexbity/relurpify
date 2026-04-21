package ayenitd

import (
	"fmt"
	"io"
	"os"

	"codeburg.org/lexbit/relurpify/framework/config"
	"codeburg.org/lexbit/relurpify/framework/memory"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
)

var (
	mkdirAllFn          = os.MkdirAll
	newKnowledgeStoreFn = memory.NewInMemoryKnowledgeStore
)

// openRuntimeStores opens all SQLite stores required for a workspace.
// Extracted from app/relurpish/runtime/runtime.go.
func openRuntimeStores(workspace string) (frameworkplan.PlanStore, memory.KnowledgeStore, io.Closer, error) {
	paths := config.New(workspace)
	if err := mkdirAllFn(paths.SessionsDir(), 0o755); err != nil {
		return nil, nil, nil, fmt.Errorf("create sessions directory: %w", err)
	}
	if err := mkdirAllFn(paths.MemoryDir(), 0o755); err != nil {
		return nil, nil, nil, fmt.Errorf("create memory directory: %w", err)
	}
	knowledgeStore := newKnowledgeStoreFn()

	return nil, knowledgeStore, nil, nil
}
