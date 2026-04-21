package agenttest

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"codeburg.org/lexbit/relurpify/framework/memory"
	memdb "codeburg.org/lexbit/relurpify/framework/memory/db"
	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
	_ "github.com/mattn/go-sqlite3"
)

type testStoreBundle struct {
	PlanStore     frameworkplan.PlanStore
	WorkflowStore memory.WorkflowStateStore
	cleanup       func()
}

func (b *testStoreBundle) Close() {
	if b.cleanup != nil {
		b.cleanup()
	}
}

func newTestStoreBundle() (*testStoreBundle, error) {
	planDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, fmt.Errorf("plan store db open: %w", err)
	}
	planStore, err := frameworkplan.NewSQLitePlanStore(planDB)
	if err != nil {
		planDB.Close()
		return nil, fmt.Errorf("plan store init: %w", err)
	}

	workflowPath := filepath.Join(os.TempDir(), fmt.Sprintf("agenttest-wf-%d.db", time.Now().UnixNano()))
	workflowStore, err := memdb.NewSQLiteWorkflowStateStore(workflowPath)
	if err != nil {
		planDB.Close()
		return nil, fmt.Errorf("workflow store init: %w", err)
	}

	return &testStoreBundle{
		PlanStore:     planStore,
		WorkflowStore: workflowStore,
		cleanup: func() {
			planDB.Close()
			_ = workflowStore.Close()
			_ = os.Remove(workflowPath)
		},
	}, nil
}
