package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/assert"
)

func TestSQLiteDatabaseDetectToolFindsDatabase(t *testing.T) {
	base := t.TempDir()
	dbPath := filepath.Join(base, "data", "app.db")
	assert.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	assert.NoError(t, os.WriteFile(dbPath, []byte("sqlite"), 0o644))

	tool := &SQLiteDatabaseDetectTool{BasePath: base}
	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{"path": filepath.Join(base, "data")})
	assert.NoError(t, err)
	assert.True(t, res.Success)
	assert.Equal(t, dbPath, res.Data["database_path"])
}

func TestSQLiteSchemaInspectToolReturnsStructuredSummary(t *testing.T) {
	base := t.TempDir()
	runner := &stubCommandRunner{
		stdout: `[{"type":"table","name":"users","tbl_name":"users","sql":"CREATE TABLE users(id INTEGER PRIMARY KEY)"},{"type":"index","name":"idx_users_email","tbl_name":"users","sql":"CREATE INDEX idx_users_email ON users(email)"}]`,
	}
	tool := NewSQLiteSchemaInspectTool(base)
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{"database_path": "app.db"})
	assert.NoError(t, err)
	assert.True(t, res.Success)
	assert.Equal(t, []string{"users"}, res.Data["table_names"])
	assert.Equal(t, []string{"idx_users_email"}, res.Data["index_names"])
	assert.Contains(t, res.Data["summary"], "1 tables")
}

func TestSQLiteQueryToolReturnsStructuredRows(t *testing.T) {
	base := t.TempDir()
	runner := &stubCommandRunner{
		stdout: `[{"name":"users"},{"name":"posts"}]`,
	}
	tool := NewSQLiteQueryTool(base)
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{
		"database_path": "app.db",
		"query":         "SELECT name FROM sqlite_master",
	})
	assert.NoError(t, err)
	assert.True(t, res.Success)
	assert.Equal(t, 2, res.Data["row_count"])
}

func TestSQLiteIntegrityCheckToolFailsOnNonOKOutput(t *testing.T) {
	base := t.TempDir()
	runner := &stubCommandRunner{
		stdout: "row 1 missing from index idx_users_email",
		err:    errors.New("exit status 1"),
	}
	tool := NewSQLiteIntegrityCheckTool(base)
	tool.SetCommandRunner(runner)

	res, err := tool.Execute(context.Background(), core.NewContext(), map[string]interface{}{"database_path": "app.db"})
	assert.NoError(t, err)
	assert.False(t, res.Success)
	assert.False(t, res.Data["ok"].(bool))
	assert.Contains(t, res.Data["summary"], "row 1 missing")
}
