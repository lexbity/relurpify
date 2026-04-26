package ayenitd

import (
	"fmt"
	"os"
	"path/filepath"

	"codeburg.org/lexbit/relurpify/framework/config"
	"codeburg.org/lexbit/relurpify/framework/memory"
	memorydb "codeburg.org/lexbit/relurpify/framework/memory/db"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
)

var (
	mkdirAllFn          = os.MkdirAll
	newKnowledgeStoreFn = memory.NewInMemoryKnowledgeStore
)

// openRuntimeStores opens all SQLite stores required for a workspace.
// Extracted from app/relurpish/runtime/runtime.go.
func openRuntimeStores(workspace string) (*memorydb.SQLiteWorkflowStateStore, frameworkplan.PlanStore, memory.KnowledgeStore, error) {
	paths := config.New(workspace)
	if err := mkdirAllFn(paths.SessionsDir(), 0o755); err != nil {
		return nil, nil, nil, fmt.Errorf("create sessions directory: %w", err)
	}
	if err := mkdirAllFn(paths.MemoryDir(), 0o755); err != nil {
		return nil, nil, nil, fmt.Errorf("create memory directory: %w", err)
	}
	workflowStore, err := memorydb.NewSQLiteWorkflowStateStore(filepath.Join(paths.MemoryDir(), "workflow_state.db"))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open workflow store: %w", err)
	}
	planDB, err := frameworkplan.OpenSQLite(filepath.Join(paths.SessionsDir(), "plans.db"))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open plan db: %w", err)
	}
	planStore, err := frameworkplan.NewSQLitePlanStore(planDB)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open plan store: %w", err)
	}
	knowledgeStore := newKnowledgeStoreFn()

	return workflowStore, planStore, knowledgeStore, nil
}
