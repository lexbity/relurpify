package ayenitd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"codeburg.org/lexbit/relurpify/framework/config"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/memory/db"
	"codeburg.org/lexbit/relurpify/framework/patterns"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
)

var (
	mkdirAllFn                    = os.MkdirAll
	newSQLiteWorkflowStateStoreFn = db.NewSQLiteWorkflowStateStore
	newSQLitePlanStoreFn          = frameworkplan.NewSQLitePlanStore
	openPatternsSQLiteFn          = patterns.OpenSQLite
	newSQLitePatternStoreFn       = patterns.NewSQLitePatternStore
	newSQLiteCommentStoreFn       = patterns.NewSQLiteCommentStore
	newKnowledgeStoreFn           = memory.NewInMemoryKnowledgeStore
)

// openRuntimeStores opens all SQLite stores required for a workspace.
// Extracted from app/relurpish/runtime/runtime.go.
func openRuntimeStores(workspace string) (*db.SQLiteWorkflowStateStore, frameworkplan.PlanStore, patterns.PatternStore, patterns.CommentStore, memory.KnowledgeStore, io.Closer, error) {
	paths := config.New(workspace)
	if err := mkdirAllFn(paths.SessionsDir(), 0o755); err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("create sessions directory: %w", err)
	}
	if err := mkdirAllFn(paths.MemoryDir(), 0o755); err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("create memory directory: %w", err)
	}

	workflowStore, err := newSQLiteWorkflowStateStoreFn(paths.WorkflowStateFile())
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("open workflow state store: %w", err)
	}
	planStore, err := newSQLitePlanStoreFn(workflowStore.DB())
	if err != nil {
		_ = workflowStore.Close()
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("open living plan store: %w", err)
	}

	patternDBPath := filepath.Join(paths.ConfigRoot(), "patterns.db")
	patternDB, err := openPatternsSQLiteFn(patternDBPath)
	if err != nil {
		_ = workflowStore.Close()
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("open patterns store: %w", err)
	}
	patternStore, err := newSQLitePatternStoreFn(patternDB)
	if err != nil {
		_ = patternDB.Close()
		_ = workflowStore.Close()
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("open pattern catalog: %w", err)
	}
	commentStore, err := newSQLiteCommentStoreFn(patternDB)
	if err != nil {
		_ = patternDB.Close()
		_ = workflowStore.Close()
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("open comment catalog: %w", err)
	}
	// Create a knowledge store
	knowledgeStore := newKnowledgeStoreFn()

	return workflowStore, planStore, patternStore, commentStore, knowledgeStore, patternDB, nil
}
