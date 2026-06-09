package graphdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
	_ "github.com/mattn/go-sqlite3"
)

// ────────────────────────────────────────────────────────────────────
// Migration state (reuses the aof_to_badger migration prefix key)
// ────────────────────────────────────────────────────────────────────

const sqliteMigrationName = "sqlite_ast_to_badger"

type sqliteMigrationState struct {
	Status      string    `json:"status"`
	TableCounts string    `json:"table_counts,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

// ────────────────────────────────────────────────────────────────────
// SQLite AST → Badger migration
// ────────────────────────────────────────────────────────────────────

// sqliteRow holds a raw row from the SQLite files, nodes, or edges table.
type sqliteFileRow struct {
	ID            string
	Path          string
	RelativePath  sql.NullString
	Language      sql.NullString
	Category      sql.NullString
	LineCount     sql.NullInt64
	TokenCount    sql.NullInt64
	Complexity    sql.NullInt64
	ContentHash   sql.NullString
	RootNodeID    sql.NullString
	NodeCount     sql.NullInt64
	EdgeCount     sql.NullInt64
	IndexedAt     sql.NullTime
	ParserVersion sql.NullString
	Summary       sql.NullString
	SummaryHash   sql.NullString
}

type sqliteNodeRow struct {
	ID           string
	ParentID     sql.NullString
	FileID       string
	NodeType     string
	Category     sql.NullString
	Language     sql.NullString
	StartLine    sql.NullInt64
	EndLine      sql.NullInt64
	StartCol     sql.NullInt64
	EndCol       sql.NullInt64
	Name         sql.NullString
	Signature    sql.NullString
	DocString    sql.NullString
	Attributes   sql.NullString
	IsExported   sql.NullBool
	IsDeprecated sql.NullBool
	CreatedAt    sql.NullTime
	UpdatedAt    sql.NullTime
	ContentHash  sql.NullString
}

type sqliteEdgeRow struct {
	ID         string
	SourceID   string
	TargetID   string
	EdgeType   string
	Attributes sql.NullString
}

// MigrateSQLiteASTToBadger reads all data from an existing SQLite AST
// index database and writes it as canonical graph records into a Badger
// store. The migration is idempotent: already‑completed migrations are
// skipped, and interrupted ones resume by re‑writing.
func MigrateSQLiteASTToBadger(ctx context.Context, sqlitePath string, badgerDir string) error {
	// 1. Open the SQLite source.
	src, err := sql.Open("sqlite3", sqlitePath)
	if err != nil {
		return fmt.Errorf("graphdb migration: open sqlite: %w", err)
	}
	defer src.Close()

	// Verify the SQLite DB has the expected tables.
	if err := verifySQLiteSchema(src); err != nil {
		return err
	}

	// 2. Open the Badger target.
	bb, err := newBadgerBackend(BadgerOptions{Dir: badgerDir})
	if err != nil {
		return fmt.Errorf("graphdb migration: open target: %w", err)
	}
	defer bb.close()

	// 3. Check migration state.
	alreadyDone, err := checkSQLiteMigrationState(bb)
	if err != nil {
		return err
	}
	if alreadyDone {
		return nil
	}

	// 4. Mark in‑progress.
	if err := setSQLiteMigrationState(bb, sqliteMigrationState{
		Status: migrationStatusInProgress,
	}); err != nil {
		return err
	}

	// 5. Migrate files.
	if err := migrateSQLiteFiles(ctx, src, bb); err != nil {
		return err
	}

	// 6. Migrate nodes.
	if err := migrateSQLiteNodes(ctx, src, bb); err != nil {
		return err
	}

	// 7. Migrate edges.
	if err := migrateSQLiteEdges(ctx, src, bb); err != nil {
		return err
	}

	// 8. Rebuild indexes from canonical records.
	if err := bb.rebuildIndexes(); err != nil {
		return fmt.Errorf("graphdb migration: rebuild indexes: %w", err)
	}

	// 9. Mark complete.
	if err := setSQLiteMigrationState(bb, sqliteMigrationState{
		Status:      migrationStatusCompleted,
		CompletedAt: time.Now().UTC(),
	}); err != nil {
		return err
	}

	return nil
}

// ────────────────────────────────────────────────────────────────────
// Migration steps
// ────────────────────────────────────────────────────────────────────

func migrateSQLiteFiles(ctx context.Context, src *sql.DB, bb *badgerBackend) error {
	rows, err := src.Query(`SELECT id, path, relative_path, language, category,
		line_count, token_count, complexity, content_hash, root_node_id,
		node_count, edge_count, indexed_at, parser_version, summary, summary_hash
		FROM files ORDER BY id`)
	if err != nil {
		return fmt.Errorf("graphdb migration: query files: %w", err)
	}
	defer rows.Close()

	var buf []NodeRecord
	for rows.Next() {
		var r sqliteFileRow
		if err := rows.Scan(&r.ID, &r.Path, &r.RelativePath, &r.Language, &r.Category,
			&r.LineCount, &r.TokenCount, &r.Complexity, &r.ContentHash, &r.RootNodeID,
			&r.NodeCount, &r.EdgeCount, &r.IndexedAt, &r.ParserVersion, &r.Summary, &r.SummaryHash); err != nil {
			return fmt.Errorf("graphdb migration: scan file: %w", err)
		}
		buf = append(buf, sqliteFileToNodeRecord(r))
		if len(buf) >= migrationChunkSize {
			if err := writeNodeBatch(ctx, bb, buf); err != nil {
				return err
			}
			buf = buf[:0]
		}
	}
	if len(buf) > 0 {
		if err := writeNodeBatch(ctx, bb, buf); err != nil {
			return err
		}
	}
	return rows.Err()
}

func migrateSQLiteNodes(ctx context.Context, src *sql.DB, bb *badgerBackend) error {
	rows, err := src.Query(`SELECT id, parent_id, file_id, type, category, language,
		start_line, end_line, start_col, end_col, name, signature, doc_string,
		attributes, is_exported, is_deprecated, created_at, updated_at, content_hash
		FROM nodes ORDER BY id`)
	if err != nil {
		return fmt.Errorf("graphdb migration: query nodes: %w", err)
	}
	defer rows.Close()

	var buf []NodeRecord
	for rows.Next() {
		var r sqliteNodeRow
		if err := rows.Scan(&r.ID, &r.ParentID, &r.FileID, &r.NodeType, &r.Category,
			&r.Language, &r.StartLine, &r.EndLine, &r.StartCol, &r.EndCol,
			&r.Name, &r.Signature, &r.DocString, &r.Attributes,
			&r.IsExported, &r.IsDeprecated, &r.CreatedAt, &r.UpdatedAt, &r.ContentHash); err != nil {
			return fmt.Errorf("graphdb migration: scan node: %w", err)
		}
		buf = append(buf, sqliteNodeToNodeRecord(r))
		if len(buf) >= migrationChunkSize {
			if err := writeNodeBatch(ctx, bb, buf); err != nil {
				return err
			}
			buf = buf[:0]
		}
	}
	if len(buf) > 0 {
		if err := writeNodeBatch(ctx, bb, buf); err != nil {
			return err
		}
	}
	return rows.Err()
}

func migrateSQLiteEdges(ctx context.Context, src *sql.DB, bb *badgerBackend) error {
	rows, err := src.Query(`SELECT id, source_id, target_id, type, attributes
		FROM edges ORDER BY id`)
	if err != nil {
		return fmt.Errorf("graphdb migration: query edges: %w", err)
	}
	defer rows.Close()

	var buf []EdgeRecord
	for rows.Next() {
		var r sqliteEdgeRow
		if err := rows.Scan(&r.ID, &r.SourceID, &r.TargetID, &r.EdgeType, &r.Attributes); err != nil {
			return fmt.Errorf("graphdb migration: scan edge: %w", err)
		}
		buf = append(buf, sqliteEdgeToEdgeRecord(r))
		if len(buf) >= migrationChunkSize {
			if err := writeEdgeBatch(ctx, bb, buf); err != nil {
				return err
			}
			buf = buf[:0]
		}
	}
	if len(buf) > 0 {
		if err := writeEdgeBatch(ctx, bb, buf); err != nil {
			return err
		}
	}
	return rows.Err()
}

// ────────────────────────────────────────────────────────────────────
// Row → graph record conversion
// ────────────────────────────────────────────────────────────────────

func sqliteFileToNodeRecord(r sqliteFileRow) NodeRecord {
	props := map[string]any{
		"path":          r.Path,
		"relative_path": r.RelativePath.String,
		"language":      r.Language.String,
		"category":      r.Category.String,
		"line_count":    r.LineCount.Int64,
		"token_count":   r.TokenCount.Int64,
		"complexity":    r.Complexity.Int64,
		"content_hash":  r.ContentHash.String,
		"root_node_id":  r.RootNodeID.String,
		"node_count":    r.NodeCount.Int64,
		"edge_count":    r.EdgeCount.Int64,
	}
	propsRaw, _ := json.Marshal(props)

	labels := []string{"file:" + r.Path}
	if r.Language.Valid {
		labels = append(labels, "lang:"+r.Language.String)
	}
	if r.Category.Valid {
		labels = append(labels, "cat:"+r.Category.String)
	}
	if r.ContentHash.Valid {
		labels = append(labels, "hash:"+r.ContentHash.String)
	}

	now := time.Now().UnixNano()
	return NodeRecord{
		ID:        r.ID,
		Kind:      "ast_file",
		SourceID:  r.Path,
		StableID:  "path:" + r.Path,
		Labels:    labels,
		Props:     propsRaw,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func sqliteNodeToNodeRecord(r sqliteNodeRow) NodeRecord {
	props := map[string]any{
		"parent_id":    r.ParentID.String,
		"file_id":      r.FileID,
		"type":         r.NodeType,
		"category":     r.Category.String,
		"language":     r.Language.String,
		"start_line":   r.StartLine.Int64,
		"end_line":     r.EndLine.Int64,
		"start_col":    r.StartCol.Int64,
		"end_col":      r.EndCol.Int64,
		"name":         r.Name.String,
		"signature":    r.Signature.String,
		"doc_string":   r.DocString.String,
		"is_exported":  r.IsExported.Bool,
		"content_hash": r.ContentHash.String,
	}
	if r.Attributes.Valid {
		props["attributes"] = r.Attributes.String
	}
	propsRaw, _ := json.Marshal(props)

	labels := []string{"file:" + r.FileID, "type:" + r.NodeType}
	if r.Name.Valid && r.Name.String != "" {
		labels = append(labels, "name:"+r.Name.String)
	}
	if r.Category.Valid {
		labels = append(labels, "cat:"+r.Category.String)
	}

	now := time.Now().UnixNano()
	return NodeRecord{
		ID:        r.ID,
		Kind:      "ast_node",
		Labels:    labels,
		Props:     propsRaw,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func sqliteEdgeToEdgeRecord(r sqliteEdgeRow) EdgeRecord {
	props := map[string]any{"edge_id": r.ID, "type": r.EdgeType}
	if r.Attributes.Valid {
		props["attributes"] = r.Attributes.String
	}
	propsRaw, _ := json.Marshal(props)

	now := time.Now().UnixNano()
	return EdgeRecord{
		SourceID:  r.SourceID,
		TargetID:  r.TargetID,
		Kind:      EdgeKind(r.EdgeType),
		Props:     propsRaw,
		CreatedAt: now,
	}
}

// ────────────────────────────────────────────────────────────────────
// Batch helpers
// ────────────────────────────────────────────────────────────────────

func writeNodeBatch(ctx context.Context, bb *badgerBackend, nodes []NodeRecord) error {
	if len(nodes) == 1 {
		return bb.commit(ctx, mutationBatch{
			opName: "upsert_node",
			op:     nodeOp{Node: nodes[0]},
		})
	}
	return bb.commit(ctx, mutationBatch{
		opName: "upsert_nodes",
		op:     nodeBatchOp{Nodes: nodes},
	})
}

func writeEdgeBatch(ctx context.Context, bb *badgerBackend, edges []EdgeRecord) error {
	if len(edges) == 1 {
		return bb.commit(ctx, mutationBatch{
			opName: "link_edge",
			op:     edgeOp{Edge: edges[0]},
		})
	}
	return bb.commit(ctx, mutationBatch{
		opName: "link_edges",
		op:     edgeBatchOp{Edges: edges},
	})
}

// ────────────────────────────────────────────────────────────────────
// Schema verification
// ────────────────────────────────────────────────────────────────────

func verifySQLiteSchema(db *sql.DB) error {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('files','nodes','edges')`).Scan(&count)
	if err != nil {
		return fmt.Errorf("graphdb migration: verify schema: %w", err)
	}
	if count < 3 {
		return errors.New("graphdb migration: SQLite DB missing required tables (files, nodes, edges)")
	}
	return nil
}

// ────────────────────────────────────────────────────────────────────
// Migration state in Badger
// ────────────────────────────────────────────────────────────────────

func checkSQLiteMigrationState(bb *badgerBackend) (bool, error) {
	var done bool
	err := bb.db.Update(func(txn *badger.Txn) error {
		key := keyMigration(sqliteMigrationName)
		item, err := txn.Get(key)
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		var st sqliteMigrationState
		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &st)
		}); err != nil {
			return err
		}
		if st.Status == migrationStatusCompleted {
			done = true
		}
		return nil
	})
	return done, err
}

func setSQLiteMigrationState(bb *badgerBackend, st sqliteMigrationState) error {
	return bb.db.Update(func(txn *badger.Txn) error {
		val, err := json.Marshal(st)
		if err != nil {
			return err
		}
		return txn.Set(keyMigration(sqliteMigrationName), val)
	})
}
