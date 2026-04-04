package ayenitd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/framework/patterns"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
)

// openRuntimeStores opens all SQLite stores required for a workspace.
// Extracted from app/relurpish/runtime/runtime.go.
func openRuntimeStores(workspace string) (*db.SQLiteWorkflowStateStore, frameworkplan.PlanStore, patterns.PatternStore, patterns.CommentStore, io.Closer, error) {
	paths := config.New(workspace)
	if err := os.MkdirAll(paths.SessionsDir(), 0o755); err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("create sessions directory: %w", err)
	}
	if err := os.MkdirAll(paths.MemoryDir(), 0o755); err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("create memory directory: %w", err)
	}

	workflowStore, err := db.NewSQLiteWorkflowStateStore(paths.WorkflowStateFile())
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("open workflow state store: %w", err)
	}
	planStore, err := frameworkplan.NewSQLitePlanStore(workflowStore.DB())
	if err != nil {
		_ = workflowStore.Close()
		return nil, nil, nil, nil, nil, fmt.Errorf("open living plan store: %w", err)
	}

	patternDBPath := filepath.Join(paths.ConfigRoot(), "patterns.db")
	patternDB, err := patterns.OpenSQLite(patternDBPath)
	if err != nil {
		_ = workflowStore.Close()
		return nil, nil, nil, nil, nil, fmt.Errorf("open patterns store: %w", err)
	}
	patternStore, err := patterns.NewSQLitePatternStore(patternDB)
	if err != nil {
		_ = patternDB.Close()
		_ = workflowStore.Close()
		return nil, nil, nil, nil, nil, fmt.Errorf("open pattern catalog: %w", err)
	}
	commentStore, err := patterns.NewSQLiteCommentStore(patternDB)
	if err != nil {
		_ = patternDB.Close()
		_ = workflowStore.Close()
		return nil, nil, nil, nil, nil, fmt.Errorf("open comment catalog: %w", err)
	}
	return workflowStore, planStore, patternStore, commentStore, patternDB, nil
}
