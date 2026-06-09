package graphdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

// ────────────────────────────────────────────────────────────────────
// Helpers
// ────────────────────────────────────────────────────────────────────

// createSQLiteASTDB creates a temporary SQLite database with the AST
// schema and returns the path. The caller is responsible for cleanup.
func createSQLiteASTDB(t *testing.T, files int, nodesPerFile int, edges int) string {
	t.Helper()
	path := t.TempDir() + "/ast.db"
	db, err := sql.Open("sqlite3", path)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS files (
			id TEXT PRIMARY KEY, path TEXT NOT NULL UNIQUE,
			relative_path TEXT, language TEXT, category TEXT,
			line_count INTEGER, token_count INTEGER, complexity INTEGER,
			content_hash TEXT, root_node_id TEXT, node_count INTEGER,
			edge_count INTEGER, indexed_at TIMESTAMP, parser_version TEXT,
			summary TEXT, summary_hash TEXT
		);
		CREATE TABLE IF NOT EXISTS nodes (
			id TEXT PRIMARY KEY, parent_id TEXT, file_id TEXT NOT NULL,
			type TEXT NOT NULL, category TEXT, language TEXT,
			start_line INTEGER, end_line INTEGER, start_col INTEGER, end_col INTEGER,
			name TEXT, signature TEXT, doc_string TEXT, attributes TEXT,
			is_exported BOOLEAN, is_deprecated BOOLEAN,
			created_at TIMESTAMP, updated_at TIMESTAMP, content_hash TEXT
		);
		CREATE TABLE IF NOT EXISTS edges (
			id TEXT PRIMARY KEY, source_id TEXT NOT NULL, target_id TEXT NOT NULL,
			type TEXT NOT NULL, attributes TEXT
		);
	`)
	require.NoError(t, err)

	insertFile, err := db.Prepare(`INSERT INTO files (id, path, relative_path, language, category, line_count, content_hash, indexed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	require.NoError(t, err)

	insertNode, err := db.Prepare(`INSERT INTO nodes (id, parent_id, file_id, type, category, language, name, start_line, end_line, is_exported, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	require.NoError(t, err)

	insertEdge, err := db.Prepare(`INSERT INTO edges (id, source_id, target_id, type) VALUES (?, ?, ?, ?)`)
	require.NoError(t, err)

	now := time.Now()
	for i := 0; i < files; i++ {
		fid := fmt.Sprintf("file-%03d", i)
		fpath := fmt.Sprintf("/src/file-%03d.go", i)
		_, err = insertFile.Exec(fid, fpath, fpath, "go", "code", 100+i, "hash"+fid, now)
		require.NoError(t, err)

		for j := 0; j < nodesPerFile; j++ {
			nid := fmt.Sprintf("node-%03d-%03d", i, j)
			_, err = insertNode.Exec(nid, "", fid, "function", "code", "go", "Func"+fmt.Sprint(j), 1, 10, j%2 == 0, now, now)
			require.NoError(t, err)
		}
	}

	for i := 0; i < edges; i++ {
		// Use a simple linear mapping. Each edge gets a unique pair by
		// using the file offset to ensure different (source, target)
		// combinations.
		srcFile := i % files
		tgtFile := (i + 1 + i/files) % files
		srcNode := i % nodesPerFile
		tgtNode := (i + 3) % nodesPerFile
		src := fmt.Sprintf("node-%03d-%03d", srcFile, srcNode)
		tgt := fmt.Sprintf("node-%03d-%03d", tgtFile, tgtNode)
		_, err = insertEdge.Exec(fmt.Sprintf("edge-%03d", i), src, tgt, "calls")
		require.NoError(t, err)
	}

	return path
}

func migratedNodeKindCount(t *testing.T, dir string, kind NodeKind) int {
	t.Helper()
	bb, err := newBadgerBackend(BadgerOptions{Dir: dir})
	require.NoError(t, err)
	defer bb.close()

	var count int
	require.NoError(t, bb.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := keyPrefix(famNode)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			if err := item.Value(func(val []byte) error {
				var node NodeRecord
				if err := json.Unmarshal(val, &node); err != nil {
					return err
				}
				if node.Kind == kind {
					count++
				}
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	}))
	return count
}

func sqliteMigrationStateStatus(t *testing.T, dir string) string {
	t.Helper()
	bb, err := newBadgerBackend(BadgerOptions{Dir: dir})
	require.NoError(t, err)
	defer bb.close()

	var st sqliteMigrationState
	require.NoError(t, bb.db.View(func(txn *badger.Txn) error {
		key := keyMigration(sqliteMigrationName)
		item, err := txn.Get(key)
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &st)
		})
	}))
	return st.Status
}

// ────────────────────────────────────────────────────────────────────
// Tests
// ────────────────────────────────────────────────────────────────────

func TestMigrateSQLiteASTToBadger_EmptyDB(t *testing.T) {
	sqlitePath := createSQLiteASTDB(t, 0, 0, 0)
	badgerDir := t.TempDir()

	require.NoError(t, MigrateSQLiteASTToBadger(context.Background(), sqlitePath, badgerDir))
	require.Equal(t, 0, migratedNodeKindCount(t, badgerDir, "ast_file"))
	require.Equal(t, 0, migratedNodeKindCount(t, badgerDir, "ast_node"))
	require.Equal(t, migrationStatusCompleted, sqliteMigrationStateStatus(t, badgerDir))
}

func TestMigrateSQLiteASTToBadger_FilesOnly(t *testing.T) {
	sqlitePath := createSQLiteASTDB(t, 5, 0, 0)
	badgerDir := t.TempDir()

	require.NoError(t, MigrateSQLiteASTToBadger(context.Background(), sqlitePath, badgerDir))
	require.Equal(t, 5, migratedNodeKindCount(t, badgerDir, "ast_file"))
	require.Equal(t, 0, migratedNodeKindCount(t, badgerDir, "ast_node"))
	require.Equal(t, migrationStatusCompleted, sqliteMigrationStateStatus(t, badgerDir))
}

func TestMigrateSQLiteASTToBadger_WithNodes(t *testing.T) {
	sqlitePath := createSQLiteASTDB(t, 3, 4, 0)
	badgerDir := t.TempDir()

	require.NoError(t, MigrateSQLiteASTToBadger(context.Background(), sqlitePath, badgerDir))
	require.Equal(t, 3, migratedNodeKindCount(t, badgerDir, "ast_file"))
	require.Equal(t, 12, migratedNodeKindCount(t, badgerDir, "ast_node"))
}

func TestMigrateSQLiteASTToBadger_WithEdges(t *testing.T) {
	sqlitePath := createSQLiteASTDB(t, 2, 3, 5)
	badgerDir := t.TempDir()

	require.NoError(t, MigrateSQLiteASTToBadger(context.Background(), sqlitePath, badgerDir))
	require.Equal(t, 2, migratedNodeKindCount(t, badgerDir, "ast_file"))
	require.Equal(t, 6, migratedNodeKindCount(t, badgerDir, "ast_node"))

	// Verify edges exist in the store.
	bb, err := newBadgerBackend(BadgerOptions{Dir: badgerDir})
	require.NoError(t, err)
	defer bb.close()

	var edgeCount int
	require.NoError(t, bb.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := keyPrefix(famEdgeOut)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			edgeCount++
		}
		return nil
	}))
	require.Equal(t, 5, edgeCount)
}

func TestMigrateSQLiteASTToBadger_SkipIfAlreadyCompleted(t *testing.T) {
	sqlitePath := createSQLiteASTDB(t, 3, 0, 0)
	badgerDir := t.TempDir()

	require.NoError(t, MigrateSQLiteASTToBadger(context.Background(), sqlitePath, badgerDir))
	require.Equal(t, 3, migratedNodeKindCount(t, badgerDir, "ast_file"))

	// Second migration should be skipped.
	require.NoError(t, MigrateSQLiteASTToBadger(context.Background(), sqlitePath, badgerDir))
	require.Equal(t, 3, migratedNodeKindCount(t, badgerDir, "ast_file"))
}

func TestMigrateSQLiteASTToBadger_ResumeAfterInterruption(t *testing.T) {
	sqlitePath := createSQLiteASTDB(t, 2, 2, 0)
	badgerDir := t.TempDir()

	// Manually set migration state to in_progress.
	bb, err := newBadgerBackend(BadgerOptions{Dir: badgerDir})
	require.NoError(t, err)
	require.NoError(t, bb.db.Update(func(txn *badger.Txn) error {
		val, err := json.Marshal(sqliteMigrationState{Status: migrationStatusInProgress})
		if err != nil {
			return err
		}
		return txn.Set(keyMigration(sqliteMigrationName), val)
	}))
	bb.close()

	// Migration should complete (resume).
	require.NoError(t, MigrateSQLiteASTToBadger(context.Background(), sqlitePath, badgerDir))
	require.Equal(t, 2, migratedNodeKindCount(t, badgerDir, "ast_file"))
	require.Equal(t, 4, migratedNodeKindCount(t, badgerDir, "ast_node"))
	require.Equal(t, migrationStatusCompleted, sqliteMigrationStateStatus(t, badgerDir))
}

func TestMigrateSQLiteASTToBadger_LargeDataset(t *testing.T) {
	// Use small enough values that we know exact counts are unique.
	sqlitePath := createSQLiteASTDB(t, 5, 3, 7)
	badgerDir := t.TempDir()

	require.NoError(t, MigrateSQLiteASTToBadger(context.Background(), sqlitePath, badgerDir))
	require.Equal(t, 5, migratedNodeKindCount(t, badgerDir, "ast_file"))
	require.Equal(t, 15, migratedNodeKindCount(t, badgerDir, "ast_node"))

	bb, err := newBadgerBackend(BadgerOptions{Dir: badgerDir})
	require.NoError(t, err)
	defer bb.close()

	var edgeCount int
	require.NoError(t, bb.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := keyPrefix(famEdgeOut)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			edgeCount++
		}
		return nil
	}))
	require.Equal(t, 7, edgeCount)
}

func TestMigrateSQLiteASTToBadger_InvalidDB(t *testing.T) {
	badgerDir := t.TempDir()

	// Non-existent path.
	err := MigrateSQLiteASTToBadger(context.Background(), "/nonexistent/path.db", badgerDir)
	require.Error(t, err)
	// Should fail with an error mentioning sqlite or schema.
	hasSQLite := strings.Contains(err.Error(), "sqlite")
	hasSchema := strings.Contains(err.Error(), "schema")
	require.True(t, hasSQLite || hasSchema, "error should mention sqlite or schema")
}

func TestMigrateSQLiteASTToBadger_VerifyStateWritten(t *testing.T) {
	sqlitePath := createSQLiteASTDB(t, 1, 0, 0)
	badgerDir := t.TempDir()

	require.NoError(t, MigrateSQLiteASTToBadger(context.Background(), sqlitePath, badgerDir))

	bb, err := newBadgerBackend(BadgerOptions{Dir: badgerDir})
	require.NoError(t, err)
	defer bb.close()

	var st sqliteMigrationState
	require.NoError(t, bb.db.View(func(txn *badger.Txn) error {
		key := keyMigration(sqliteMigrationName)
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &st)
		})
	}))
	require.Equal(t, migrationStatusCompleted, st.Status)
	require.False(t, st.CompletedAt.IsZero())
}
