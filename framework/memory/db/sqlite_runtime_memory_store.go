package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/perfstats"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	_ "github.com/mattn/go-sqlite3"
)

const (
	runtimeMemorySearchSchemaVersion = "2"
	declarativeSearchReadyMetaKey    = "declarative_search_ready"
	proceduralSearchReadyMetaKey     = "procedural_search_ready"
)

// SQLiteRuntimeMemoryStore persists declarative and procedural memory in separate tables.
type SQLiteRuntimeMemoryStore struct {
	db                *sql.DB
	retrieve          retrieval.RetrieverService
	retrievalEmbedder retrieval.Embedder
	corpusScope       string // corpus scope for anchor binding lookups
	declSearchFTS     bool
	procSearchFTS     bool
}

type scoredMemoryRecord struct {
	record memory.MemoryRecord
	score  float64
}

func NewSQLiteRuntimeMemoryStore(path string) (*SQLiteRuntimeMemoryStore, error) {
	return NewSQLiteRuntimeMemoryStoreWithRetrieval(path, SQLiteRuntimeRetrievalOptions{})
}

// SQLiteRuntimeRetrievalOptions controls retrieval-service wiring for the runtime store.
type SQLiteRuntimeRetrievalOptions struct {
	Embedder       retrieval.Embedder
	Telemetry      core.Telemetry
	ServiceOptions retrieval.ServiceOptions
	CorpusScope    string // optional corpus scope for anchor binding
}

// NewSQLiteRuntimeMemoryStoreWithRetrieval opens or creates the runtime memory database
// and configures retrieval with the supplied runtime dependencies.
func NewSQLiteRuntimeMemoryStoreWithRetrieval(path string, opts SQLiteRuntimeRetrievalOptions) (*SQLiteRuntimeMemoryStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("runtime memory db path required")
	}
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000", filepath.Clean(path))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	store := &SQLiteRuntimeMemoryStore{
		db:                db,
		retrieve:          retrieval.NewServiceWithOptions(db, opts.Embedder, opts.Telemetry, opts.ServiceOptions),
		retrievalEmbedder: opts.Embedder,
		corpusScope:       opts.CorpusScope,
	}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteRuntimeMemoryStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// DB exposes the underlying SQLite handle for advanced integrations.
func (s *SQLiteRuntimeMemoryStore) DB() *sql.DB {
	if s == nil {
		return nil
	}
	return s.db
}

// RetrievalService exposes the retrieval-backed runtime integration over the same database.
func (s *SQLiteRuntimeMemoryStore) RetrievalService() retrieval.RetrieverService {
	if s == nil {
		return nil
	}
	return s.retrieve
}

// Remember stores generic memory through the declarative lane so existing
// callers can adopt the structured store without changing their interface.
func (s *SQLiteRuntimeMemoryStore) Remember(ctx context.Context, key string, value map[string]interface{}, scope memory.MemoryScope) error {
	if strings.TrimSpace(key) == "" {
		return errors.New("memory key required")
	}
	if value == nil {
		value = map[string]interface{}{}
	}
	record := memory.DeclarativeMemoryRecord{
		RecordID:   key,
		Scope:      scope,
		Kind:       inferDeclarativeKind(value),
		Title:      firstNonEmptyString(value["title"], value["type"], key),
		Content:    mustJSONString(value),
		Summary:    firstNonEmptyString(value["summary"], value["decision"], value["task"], value["type"]),
		TaskID:     firstNonEmptyString(value["task_id"]),
		WorkflowID: firstNonEmptyString(value["workflow_id"]),
		ProjectID:  firstNonEmptyString(value["project_id"]),
		Metadata:   cloneMap(value),
		Verified:   boolFromAny(value["verified"]),
	}
	if artifactRef, ok := value["artifact_ref"]; ok && artifactRef != nil {
		record.ArtifactRef = fmt.Sprint(artifactRef)
	}
	if tags, ok := value["tags"].([]string); ok {
		record.Tags = append([]string{}, tags...)
	}
	return s.PutDeclarative(ctx, record)
}

// Recall reads a generic memory record from the declarative lane first and then
// falls back to the procedural lane for compatibility.
func (s *SQLiteRuntimeMemoryStore) Recall(ctx context.Context, key string, scope memory.MemoryScope) (*memory.MemoryRecord, bool, error) {
	record, ok, err := s.GetDeclarative(ctx, key)
	if err != nil || ok {
		if !ok || record == nil {
			return nil, ok, err
		}
		out := declarativeToGenericRecord(*record)
		if scope != "" && out.Scope != scope {
			return nil, false, nil
		}
		return &out, true, nil
	}
	proc, ok, err := s.GetProcedural(ctx, key)
	if err != nil || !ok {
		return nil, ok, err
	}
	out := proceduralToGenericRecord(*proc)
	if scope != "" && out.Scope != scope {
		return nil, false, nil
	}
	return &out, true, nil
}

// Search exposes a generic compatibility view over both structured lanes.
func (s *SQLiteRuntimeMemoryStore) Search(ctx context.Context, query string, scope memory.MemoryScope) ([]memory.MemoryRecord, error) {
	decl, err := s.SearchDeclarative(ctx, memory.DeclarativeMemoryQuery{
		Query: query,
		Scope: scope,
		Limit: 25,
	})
	if err != nil {
		return nil, err
	}
	proc, err := s.SearchProcedural(ctx, memory.ProceduralMemoryQuery{
		Query: query,
		Scope: scope,
		Limit: 25,
	})
	if err != nil {
		return nil, err
	}
	out := make([]memory.MemoryRecord, 0, len(decl)+len(proc))
	for _, record := range decl {
		out = append(out, declarativeToGenericRecord(record))
	}
	for _, record := range proc {
		out = append(out, proceduralToGenericRecord(record))
	}
	query = strings.TrimSpace(query)
	if query == "" {
		sort.Slice(out, func(i, j int) bool {
			if out[i].Timestamp.Equal(out[j].Timestamp) {
				return out[i].Key < out[j].Key
			}
			return out[i].Timestamp.After(out[j].Timestamp)
		})
		return out, nil
	}
	scored := make([]scoredMemoryRecord, 0, len(out))
	for _, record := range out {
		scored = append(scored, scoredMemoryRecord{
			record: record,
			score:  scoreGenericMemoryRecord(record, query),
		})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			if scored[i].record.Timestamp.Equal(scored[j].record.Timestamp) {
				return scored[i].record.Key < scored[j].record.Key
			}
			return scored[i].record.Timestamp.After(scored[j].record.Timestamp)
		}
		return scored[i].score > scored[j].score
	})
	for i := range scored {
		out[i] = scored[i].record
	}
	return out, nil
}

// Forget deletes compatibility records from both structured lanes.
func (s *SQLiteRuntimeMemoryStore) Forget(ctx context.Context, key string, scope memory.MemoryScope) error {
	if strings.TrimSpace(key) == "" {
		return errors.New("memory key required")
	}
	if err := s.deleteDeclarative(ctx, key, scope); err != nil {
		return err
	}
	return s.deleteProcedural(ctx, key, scope)
}

// Summarize produces a compact compatibility summary over both structured lanes.
func (s *SQLiteRuntimeMemoryStore) Summarize(ctx context.Context, scope memory.MemoryScope) (string, error) {
	records, err := s.Search(ctx, "", scope)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("Summary for scope ")
	b.WriteString(string(scope))
	b.WriteString(":\n")
	for _, record := range records {
		b.WriteString("- ")
		b.WriteString(record.Key)
		b.WriteString(": ")
		if summary, ok := record.Value["summary"]; ok && fmt.Sprint(summary) != "" {
			b.WriteString(fmt.Sprint(summary))
		} else {
			b.WriteString(mustJSONString(record.Value))
		}
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func (s *SQLiteRuntimeMemoryStore) init() error {
	ctx := context.Background()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS declarative_memory (
			record_id TEXT PRIMARY KEY,
			scope TEXT NOT NULL,
			kind TEXT NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL DEFAULT '',
			summary TEXT NOT NULL DEFAULT '',
			workflow_id TEXT NOT NULL DEFAULT '',
			task_id TEXT NOT NULL DEFAULT '',
			project_id TEXT NOT NULL DEFAULT '',
			artifact_ref TEXT NOT NULL DEFAULT '',
			tags_json TEXT NOT NULL DEFAULT '[]',
			metadata_json TEXT NOT NULL DEFAULT '{}',
			verified INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS procedural_memory (
			routine_id TEXT PRIMARY KEY,
			scope TEXT NOT NULL,
			kind TEXT NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			summary TEXT NOT NULL DEFAULT '',
			workflow_id TEXT NOT NULL DEFAULT '',
			task_id TEXT NOT NULL DEFAULT '',
			project_id TEXT NOT NULL DEFAULT '',
			body_ref TEXT NOT NULL DEFAULT '',
			inline_body TEXT NOT NULL DEFAULT '',
			capability_dependencies_json TEXT NOT NULL DEFAULT '[]',
			verification_metadata_json TEXT NOT NULL DEFAULT '{}',
			policy_snapshot_id TEXT NOT NULL DEFAULT '',
			verified INTEGER NOT NULL DEFAULT 0,
			version INTEGER NOT NULL DEFAULT 1,
			reuse_count INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_declarative_memory_scope_kind ON declarative_memory(scope, kind, updated_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_declarative_memory_task ON declarative_memory(task_id, workflow_id, updated_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_procedural_memory_scope_kind ON procedural_memory(scope, kind, updated_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_procedural_memory_task ON procedural_memory(task_id, workflow_id, updated_at DESC);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	declCreated, err := createDeclarativeSearchTable(ctx, s.db)
	if err != nil {
		return err
	}
	procCreated, err := createProceduralSearchTable(ctx, s.db)
	if err != nil {
		return err
	}
	capCreated, err := createProceduralCapabilityTable(ctx, s.db)
	if err != nil {
		return err
	}
	if err := createRuntimeMemorySearchMetaTable(ctx, s.db); err != nil {
		return err
	}
	if err := ensureRuntimeMemorySearchReady(ctx, s.db, declCreated, procCreated || capCreated); err != nil {
		return err
	}
	s.declSearchFTS, err = isRuntimeMemoryFTSBacked(ctx, s.db, "declarative_memory_search")
	if err != nil {
		return err
	}
	s.procSearchFTS, err = isRuntimeMemoryFTSBacked(ctx, s.db, "procedural_memory_search")
	if err != nil {
		return err
	}
	return retrieval.EnsureSchema(ctx, s.db)
}

func (s *SQLiteRuntimeMemoryStore) PutDeclarative(ctx context.Context, record memory.DeclarativeMemoryRecord) error {
	if strings.TrimSpace(record.RecordID) == "" {
		return errors.New("declarative memory record_id required")
	}
	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	record.UpdatedAt = now

	// Bind anchors if corpus scope is configured
	if strings.TrimSpace(s.corpusScope) != "" && s.db != nil {
		if record.Metadata == nil {
			record.Metadata = make(map[string]any)
		}
		_ = bindAnchorIDsToMetadata(ctx, s.db, record.Content, record.Metadata, s.corpusScope)
	}

	_, err := s.db.ExecContext(ctx, `INSERT INTO declarative_memory
		(record_id, scope, kind, title, content, summary, workflow_id, task_id, project_id, artifact_ref, tags_json, metadata_json, verified, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(record_id) DO UPDATE SET
			scope = excluded.scope,
			kind = excluded.kind,
			title = excluded.title,
			content = excluded.content,
			summary = excluded.summary,
			workflow_id = excluded.workflow_id,
			task_id = excluded.task_id,
			project_id = excluded.project_id,
			artifact_ref = excluded.artifact_ref,
			tags_json = excluded.tags_json,
			metadata_json = excluded.metadata_json,
			verified = excluded.verified,
			updated_at = excluded.updated_at`,
		record.RecordID,
		string(record.Scope),
		string(record.Kind),
		record.Title,
		record.Content,
		record.Summary,
		record.WorkflowID,
		record.TaskID,
		record.ProjectID,
		record.ArtifactRef,
		mustJSONString(record.Tags),
		mustJSONString(record.Metadata),
		boolToInt(record.Verified),
		runtimeTimeString(record.CreatedAt),
		runtimeTimeString(record.UpdatedAt),
	)
	if err != nil {
		return err
	}
	if err := s.upsertDeclarativeSearchIndex(ctx, record); err != nil {
		return err
	}
	return s.indexDeclarativeRecord(ctx, record)
}

func (s *SQLiteRuntimeMemoryStore) GetDeclarative(ctx context.Context, recordID string) (*memory.DeclarativeMemoryRecord, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT record_id, scope, kind, title, content, summary, workflow_id, task_id, project_id, artifact_ref, tags_json, metadata_json, verified, created_at, updated_at
		FROM declarative_memory WHERE record_id = ?`, recordID)
	record, err := scanDeclarativeRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	// Check for stale anchors if the store has retrieval DB access
	if s.db != nil && record != nil {
		boundAnchorIDs := loadBoundAnchorIDs(*record)
		if len(boundAnchorIDs) > 0 {
			staleAnchors, checkErr := checkStaleAnchors(ctx, s.db, boundAnchorIDs)
			if checkErr == nil && len(staleAnchors) > 0 {
				record.StaleAnchors = staleAnchors
			}
		}
	}

	return record, true, nil
}

func (s *SQLiteRuntimeMemoryStore) SearchDeclarative(ctx context.Context, query memory.DeclarativeMemoryQuery) ([]memory.DeclarativeMemoryRecord, error) {
	started := time.Now()
	defer func() {
		perfstats.ObserveRuntimeMemorySearch(time.Since(started))
	}()
	if query.Limit <= 0 {
		query.Limit = 5
	}
	queryText := strings.TrimSpace(query.Query)
	queryLower := strings.ToLower(queryText)
	queryNeedle := "%" + queryLower + "%"
	var b strings.Builder
	b.WriteString(`SELECT dm.record_id, dm.scope, dm.kind, dm.title, dm.content, dm.summary, dm.workflow_id, dm.task_id, dm.project_id, dm.artifact_ref, dm.tags_json, dm.metadata_json, dm.verified, dm.created_at, dm.updated_at FROM declarative_memory dm`)
	args := make([]any, 0, 8)
	if queryText != "" {
		if s.declSearchFTS {
			b.WriteString(` JOIN declarative_memory_search dms ON dms.record_id = dm.record_id`)
		} else {
			b.WriteString(` JOIN declarative_memory_search dms ON dms.record_id = dm.record_id`)
		}
	}
	b.WriteString(` WHERE 1=1`)
	if query.Scope != "" {
		b.WriteString(` AND dm.scope = ?`)
		args = append(args, string(query.Scope))
	}
	if query.TaskID != "" {
		b.WriteString(` AND dm.task_id = ?`)
		args = append(args, query.TaskID)
	}
	if query.WorkflowID != "" {
		b.WriteString(` AND dm.workflow_id = ?`)
		args = append(args, query.WorkflowID)
	}
	if query.ProjectID != "" {
		b.WriteString(` AND dm.project_id = ?`)
		args = append(args, query.ProjectID)
	}
	if len(query.Kinds) > 0 {
		b.WriteString(` AND dm.kind IN (` + placeholders(len(query.Kinds)) + `)`)
		for _, kind := range query.Kinds {
			args = append(args, string(kind))
		}
	}
	if queryText != "" {
		if s.declSearchFTS {
			b.WriteString(` AND declarative_memory_search MATCH ?`)
			args = append(args, runtimeMemoryFTSQuery(queryText))
			b.WriteString(` ORDER BY
				CASE
					WHEN dms.title_norm = ? THEN 0
					WHEN dms.title_norm LIKE ? THEN 1
					WHEN dms.summary_norm LIKE ? THEN 2
					ELSE 3
				END ASC,
				dm.verified DESC,
				bm25(declarative_memory_search) ASC,
				dm.updated_at DESC,
				dm.record_id ASC LIMIT ?`)
			args = append(args, queryLower, queryNeedle, queryNeedle)
		} else {
			b.WriteString(` AND lower(dms.search_text) LIKE ?`)
			args = append(args, queryNeedle)
			b.WriteString(` ORDER BY
				CASE
					WHEN dms.title_norm = ? THEN 0
					WHEN dms.title_norm LIKE ? THEN 1
					WHEN dms.summary_norm LIKE ? THEN 2
					ELSE 3
				END ASC,
				dm.verified DESC,
				dm.updated_at DESC,
				dm.record_id ASC LIMIT ?`)
			args = append(args, queryLower, queryNeedle, queryNeedle)
		}
	} else {
		b.WriteString(` ORDER BY dm.verified DESC, dm.updated_at DESC, dm.record_id ASC LIMIT ?`)
	}
	args = append(args, query.Limit)
	rows, err := s.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]memory.DeclarativeMemoryRecord, 0, query.Limit)
	for rows.Next() {
		record, err := scanDeclarativeRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *SQLiteRuntimeMemoryStore) PutProcedural(ctx context.Context, record memory.ProceduralMemoryRecord) error {
	if strings.TrimSpace(record.RoutineID) == "" {
		return errors.New("procedural memory routine_id required")
	}
	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	record.UpdatedAt = now
	if record.Version <= 0 {
		record.Version = 1
	}

	// Bind anchors if corpus scope is configured
	if strings.TrimSpace(s.corpusScope) != "" && s.db != nil {
		if record.VerificationMetadata == nil {
			record.VerificationMetadata = make(map[string]any)
		}
		// Use description and name for anchor binding
		contentToAnalyze := record.Description + " " + record.Name
		_ = bindAnchorIDsToMetadata(ctx, s.db, contentToAnalyze, record.VerificationMetadata, s.corpusScope)
	}

	_, err := s.db.ExecContext(ctx, `INSERT INTO procedural_memory
		(routine_id, scope, kind, name, description, summary, workflow_id, task_id, project_id, body_ref, inline_body, capability_dependencies_json, verification_metadata_json, policy_snapshot_id, verified, version, reuse_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(routine_id) DO UPDATE SET
			scope = excluded.scope,
			kind = excluded.kind,
			name = excluded.name,
			description = excluded.description,
			summary = excluded.summary,
			workflow_id = excluded.workflow_id,
			task_id = excluded.task_id,
			project_id = excluded.project_id,
			body_ref = excluded.body_ref,
			inline_body = excluded.inline_body,
			capability_dependencies_json = excluded.capability_dependencies_json,
			verification_metadata_json = excluded.verification_metadata_json,
			policy_snapshot_id = excluded.policy_snapshot_id,
			verified = excluded.verified,
			version = excluded.version,
			reuse_count = excluded.reuse_count,
			updated_at = excluded.updated_at`,
		record.RoutineID,
		string(record.Scope),
		string(record.Kind),
		record.Name,
		record.Description,
		record.Summary,
		record.WorkflowID,
		record.TaskID,
		record.ProjectID,
		record.BodyRef,
		record.InlineBody,
		mustJSONString(record.CapabilityDependencies),
		mustJSONString(record.VerificationMetadata),
		record.PolicySnapshotID,
		boolToInt(record.Verified),
		record.Version,
		record.ReuseCount,
		runtimeTimeString(record.CreatedAt),
		runtimeTimeString(record.UpdatedAt),
	)
	if err != nil {
		return err
	}
	return s.upsertProceduralSearchIndex(ctx, record)
}

func (s *SQLiteRuntimeMemoryStore) GetProcedural(ctx context.Context, routineID string) (*memory.ProceduralMemoryRecord, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT routine_id, scope, kind, name, description, summary, workflow_id, task_id, project_id, body_ref, inline_body, capability_dependencies_json, verification_metadata_json, policy_snapshot_id, verified, version, reuse_count, created_at, updated_at
		FROM procedural_memory WHERE routine_id = ?`, routineID)
	record, err := scanProceduralRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	// Check for stale anchors if the store has retrieval DB access
	if s.db != nil && record != nil {
		boundAnchorIDs := loadBoundAnchorIDs(*record)
		if len(boundAnchorIDs) > 0 {
			staleAnchors, checkErr := checkStaleAnchors(ctx, s.db, boundAnchorIDs)
			if checkErr == nil && len(staleAnchors) > 0 {
				record.StaleAnchors = staleAnchors
			}
		}
	}

	return record, true, nil
}

func (s *SQLiteRuntimeMemoryStore) SearchProcedural(ctx context.Context, query memory.ProceduralMemoryQuery) ([]memory.ProceduralMemoryRecord, error) {
	started := time.Now()
	defer func() {
		perfstats.ObserveRuntimeMemorySearch(time.Since(started))
	}()
	if query.Limit <= 0 {
		query.Limit = 5
	}
	queryText := strings.TrimSpace(query.Query)
	queryLower := strings.ToLower(queryText)
	queryNeedle := "%" + queryLower + "%"
	capabilityName := strings.TrimSpace(query.CapabilityName)
	capabilityLower := strings.ToLower(capabilityName)
	var b strings.Builder
	b.WriteString(`SELECT pm.routine_id, pm.scope, pm.kind, pm.name, pm.description, pm.summary, pm.workflow_id, pm.task_id, pm.project_id, pm.body_ref, pm.inline_body, pm.capability_dependencies_json, pm.verification_metadata_json, pm.policy_snapshot_id, pm.verified, pm.version, pm.reuse_count, pm.created_at, pm.updated_at FROM procedural_memory pm`)
	args := make([]any, 0, 8)
	if queryText != "" {
		b.WriteString(` JOIN procedural_memory_search pms ON pms.routine_id = pm.routine_id`)
	}
	if capabilityName != "" {
		b.WriteString(` JOIN procedural_memory_capabilities pmc ON pmc.routine_id = pm.routine_id`)
	}
	b.WriteString(` WHERE 1=1`)
	if query.Scope != "" {
		b.WriteString(` AND pm.scope = ?`)
		args = append(args, string(query.Scope))
	}
	if query.TaskID != "" {
		b.WriteString(` AND pm.task_id = ?`)
		args = append(args, query.TaskID)
	}
	if query.WorkflowID != "" {
		b.WriteString(` AND pm.workflow_id = ?`)
		args = append(args, query.WorkflowID)
	}
	if query.ProjectID != "" {
		b.WriteString(` AND pm.project_id = ?`)
		args = append(args, query.ProjectID)
	}
	if len(query.Kinds) > 0 {
		b.WriteString(` AND pm.kind IN (` + placeholders(len(query.Kinds)) + `)`)
		for _, kind := range query.Kinds {
			args = append(args, string(kind))
		}
	}
	if queryText != "" {
		if s.procSearchFTS {
			b.WriteString(` AND procedural_memory_search MATCH ?`)
			args = append(args, runtimeMemoryFTSQuery(queryText))
		} else {
			b.WriteString(` AND lower(pms.search_text) LIKE ?`)
			args = append(args, queryNeedle)
		}
	}
	if capabilityName != "" {
		b.WriteString(` AND pmc.capability_name = ?`)
		args = append(args, capabilityLower)
	}
	if queryText != "" && s.procSearchFTS {
		b.WriteString(` ORDER BY
			CASE
				WHEN pms.name_norm = ? THEN 0
				WHEN pms.name_norm LIKE ? THEN 1
				WHEN pms.summary_norm LIKE ? THEN 2
				ELSE 3
			END ASC,
			pm.verified DESC,
			bm25(procedural_memory_search) ASC,
			pm.reuse_count DESC,
			pm.updated_at DESC,
			pm.routine_id ASC LIMIT ?`)
		args = append(args, queryLower, queryNeedle, queryNeedle)
	} else if queryText != "" {
		b.WriteString(` ORDER BY
			CASE
				WHEN ? <> '' AND pms.name_norm = ? THEN 0
				WHEN ? <> '' AND pms.name_norm LIKE ? THEN 1
				WHEN ? <> '' AND pms.summary_norm LIKE ? THEN 2
				ELSE 3
			END ASC,
			pm.verified DESC,
			pm.reuse_count DESC,
			pm.updated_at DESC,
			pm.routine_id ASC LIMIT ?`)
		args = append(args,
			queryLower, queryLower,
			queryLower, queryNeedle,
			queryLower, queryNeedle,
		)
	} else {
		b.WriteString(` ORDER BY
			pm.verified DESC,
			pm.reuse_count DESC,
			pm.updated_at DESC,
			pm.routine_id ASC LIMIT ?`)
	}
	args = append(args, query.Limit)
	rows, err := s.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]memory.ProceduralMemoryRecord, 0, query.Limit)
	for rows.Next() {
		record, err := scanProceduralRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *SQLiteRuntimeMemoryStore) deleteDeclarative(ctx context.Context, key string, scope memory.MemoryScope) error {
	stmt := `DELETE FROM declarative_memory WHERE record_id = ?`
	args := []any{key}
	if scope != "" {
		stmt += ` AND scope = ?`
		args = append(args, string(scope))
	}
	_, err := s.db.ExecContext(ctx, stmt, args...)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM declarative_memory_search WHERE record_id = ?`, key); err != nil {
		return err
	}
	if err := retrieval.NewIngestionPipeline(s.db, nil).TombstoneDocument(ctx, declarativeRetrievalURI(key, scope)); err != nil {
		return err
	}
	return nil
}

func (s *SQLiteRuntimeMemoryStore) deleteProcedural(ctx context.Context, key string, scope memory.MemoryScope) error {
	stmt := `DELETE FROM procedural_memory WHERE routine_id = ?`
	args := []any{key}
	if scope != "" {
		stmt += ` AND scope = ?`
		args = append(args, string(scope))
	}
	_, err := s.db.ExecContext(ctx, stmt, args...)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM procedural_memory_search WHERE routine_id = ?`, key); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM procedural_memory_capabilities WHERE routine_id = ?`, key); err != nil {
		return err
	}
	return nil
}

func scanDeclarativeRecord(scanner interface{ Scan(dest ...any) error }) (*memory.DeclarativeMemoryRecord, error) {
	var record memory.DeclarativeMemoryRecord
	var scope, kind, tagsJSON, metadataJSON, createdAt, updatedAt string
	var verified int
	err := scanner.Scan(&record.RecordID, &scope, &kind, &record.Title, &record.Content, &record.Summary, &record.WorkflowID, &record.TaskID, &record.ProjectID, &record.ArtifactRef, &tagsJSON, &metadataJSON, &verified, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	record.Scope = memory.MemoryScope(scope)
	record.Kind = memory.DeclarativeMemoryKind(kind)
	record.Verified = verified == 1
	if tagsJSON != "" && tagsJSON != "[]" {
		_ = json.Unmarshal([]byte(tagsJSON), &record.Tags)
	}
	if metadataJSON != "" && metadataJSON != "{}" {
		_ = json.Unmarshal([]byte(metadataJSON), &record.Metadata)
	}
	record.CreatedAt = parseTimeValue(createdAt)
	record.UpdatedAt = parseTimeValue(updatedAt)
	return &record, nil
}

func scanProceduralRecord(scanner interface{ Scan(dest ...any) error }) (*memory.ProceduralMemoryRecord, error) {
	var record memory.ProceduralMemoryRecord
	var scope, kind, depsJSON, verificationJSON, createdAt, updatedAt string
	var verified int
	err := scanner.Scan(&record.RoutineID, &scope, &kind, &record.Name, &record.Description, &record.Summary, &record.WorkflowID, &record.TaskID, &record.ProjectID, &record.BodyRef, &record.InlineBody, &depsJSON, &verificationJSON, &record.PolicySnapshotID, &verified, &record.Version, &record.ReuseCount, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	record.Scope = memory.MemoryScope(scope)
	record.Kind = memory.ProceduralMemoryKind(kind)
	record.Verified = verified == 1
	if depsJSON != "" && depsJSON != "[]" {
		_ = json.Unmarshal([]byte(depsJSON), &record.CapabilityDependencies)
	}
	if verificationJSON != "" && verificationJSON != "{}" {
		_ = json.Unmarshal([]byte(verificationJSON), &record.VerificationMetadata)
	}
	record.CreatedAt = parseTimeValue(createdAt)
	record.UpdatedAt = parseTimeValue(updatedAt)
	return &record, nil
}

func mustJSONString(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func createDeclarativeSearchTable(ctx context.Context, db *sql.DB) (bool, error) {
	exists, err := runtimeMemoryTableExists(ctx, db, "declarative_memory_search")
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	if _, err := db.ExecContext(ctx, `CREATE VIRTUAL TABLE declarative_memory_search USING fts5(
		record_id UNINDEXED,
		title,
		title_norm UNINDEXED,
		summary,
		summary_norm UNINDEXED,
		content,
		tags,
		search_text
	);`); err == nil {
		return true, nil
	} else if !strings.Contains(strings.ToLower(err.Error()), "no such module: fts5") {
		return false, err
	}
	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS declarative_memory_search (
		record_id TEXT PRIMARY KEY,
		title TEXT NOT NULL DEFAULT '',
		title_norm TEXT NOT NULL DEFAULT '',
		summary TEXT NOT NULL DEFAULT '',
		summary_norm TEXT NOT NULL DEFAULT '',
		content TEXT NOT NULL DEFAULT '',
		tags TEXT NOT NULL DEFAULT '',
		search_text TEXT NOT NULL DEFAULT ''
	);`)
	return err == nil, err
}

func createProceduralSearchTable(ctx context.Context, db *sql.DB) (bool, error) {
	exists, err := runtimeMemoryTableExists(ctx, db, "procedural_memory_search")
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	if _, err := db.ExecContext(ctx, `CREATE VIRTUAL TABLE procedural_memory_search USING fts5(
		routine_id UNINDEXED,
		name,
		name_norm UNINDEXED,
		description,
		summary,
		summary_norm UNINDEXED,
		inline_body,
		capability_names,
		search_text
	);`); err == nil {
		return true, nil
	} else if !strings.Contains(strings.ToLower(err.Error()), "no such module: fts5") {
		return false, err
	}
	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS procedural_memory_search (
		routine_id TEXT PRIMARY KEY,
		name TEXT NOT NULL DEFAULT '',
		name_norm TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		summary TEXT NOT NULL DEFAULT '',
		summary_norm TEXT NOT NULL DEFAULT '',
		inline_body TEXT NOT NULL DEFAULT '',
		capability_names TEXT NOT NULL DEFAULT '',
		search_text TEXT NOT NULL DEFAULT ''
	);`)
	return err == nil, err
}

func createProceduralCapabilityTable(ctx context.Context, db *sql.DB) (bool, error) {
	exists, err := runtimeMemoryTableExists(ctx, db, "procedural_memory_capabilities")
	if err != nil {
		return false, err
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS procedural_memory_capabilities (
			routine_id TEXT NOT NULL,
			capability_name TEXT NOT NULL,
			PRIMARY KEY(routine_id, capability_name)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_procedural_memory_capability_name ON procedural_memory_capabilities(capability_name, routine_id);`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return false, err
		}
	}
	return !exists, nil
}

func createRuntimeMemorySearchMetaTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS runtime_memory_search_meta (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);`)
	return err
}

func ensureRuntimeMemorySearchReady(ctx context.Context, db *sql.DB, declForceRebuild, procForceRebuild bool) error {
	declReady, err := runtimeMemorySearchMetaValue(ctx, db, declarativeSearchReadyMetaKey)
	if err != nil {
		return err
	}
	if declReady != runtimeMemorySearchSchemaVersion {
		if err := rebuildDeclarativeSearchProjection(ctx, db); err != nil {
			return err
		}
		declForceRebuild = true
	}
	if declForceRebuild {
		if err := backfillDeclarativeSearchIndex(ctx, db); err != nil {
			return err
		}
		if err := setRuntimeMemorySearchMetaValue(ctx, db, declarativeSearchReadyMetaKey, runtimeMemorySearchSchemaVersion); err != nil {
			return err
		}
	}
	procReady, err := runtimeMemorySearchMetaValue(ctx, db, proceduralSearchReadyMetaKey)
	if err != nil {
		return err
	}
	if procReady != runtimeMemorySearchSchemaVersion {
		if err := rebuildProceduralSearchProjection(ctx, db); err != nil {
			return err
		}
		procForceRebuild = true
	}
	if procForceRebuild {
		if err := backfillProceduralSearchIndex(ctx, db); err != nil {
			return err
		}
		if err := setRuntimeMemorySearchMetaValue(ctx, db, proceduralSearchReadyMetaKey, runtimeMemorySearchSchemaVersion); err != nil {
			return err
		}
	}
	return nil
}

func rebuildDeclarativeSearchProjection(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS declarative_memory_search`); err != nil {
		return err
	}
	_, err := createDeclarativeSearchTable(ctx, db)
	return err
}

func rebuildProceduralSearchProjection(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS procedural_memory_search`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS procedural_memory_capabilities`); err != nil {
		return err
	}
	if _, err := createProceduralSearchTable(ctx, db); err != nil {
		return err
	}
	_, err := createProceduralCapabilityTable(ctx, db)
	return err
}

func backfillDeclarativeSearchIndex(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT record_id, scope, kind, title, content, summary, workflow_id, task_id, project_id, artifact_ref, tags_json, metadata_json, verified, created_at, updated_at FROM declarative_memory`)
	if err != nil {
		return err
	}
	records := make([]memory.DeclarativeMemoryRecord, 0)
	for rows.Next() {
		record, err := scanDeclarativeRecord(rows)
		if err != nil {
			_ = rows.Close()
			return err
		}
		records = append(records, *record)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM declarative_memory_search`); err != nil {
		return err
	}
	for _, record := range records {
		if err := upsertDeclarativeSearchIndex(ctx, db, record); err != nil {
			return err
		}
	}
	return nil
}

func backfillProceduralSearchIndex(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT routine_id, scope, kind, name, description, summary, workflow_id, task_id, project_id, body_ref, inline_body, capability_dependencies_json, verification_metadata_json, policy_snapshot_id, verified, version, reuse_count, created_at, updated_at FROM procedural_memory`)
	if err != nil {
		return err
	}
	records := make([]memory.ProceduralMemoryRecord, 0)
	for rows.Next() {
		record, err := scanProceduralRecord(rows)
		if err != nil {
			_ = rows.Close()
			return err
		}
		records = append(records, *record)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM procedural_memory_search`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM procedural_memory_capabilities`); err != nil {
		return err
	}
	for _, record := range records {
		if err := upsertProceduralSearchIndex(ctx, db, record); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteRuntimeMemoryStore) upsertDeclarativeSearchIndex(ctx context.Context, record memory.DeclarativeMemoryRecord) error {
	return upsertDeclarativeSearchIndex(ctx, s.db, record)
}

func upsertDeclarativeSearchIndex(ctx context.Context, db *sql.DB, record memory.DeclarativeMemoryRecord) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM declarative_memory_search WHERE record_id = ?`, record.RecordID); err != nil {
		return err
	}
	title := strings.TrimSpace(record.Title)
	summary := strings.TrimSpace(record.Summary)
	content := strings.TrimSpace(record.Content)
	tags := normalizeRuntimeMemoryTerms(record.Tags)
	_, err := db.ExecContext(ctx, `INSERT INTO declarative_memory_search(record_id, title, title_norm, summary, summary_norm, content, tags, search_text)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		record.RecordID,
		title,
		normalizeRuntimeMemoryText(title),
		summary,
		normalizeRuntimeMemoryText(summary),
		content,
		tags,
		declarativeSearchText(record),
	)
	return err
}

func (s *SQLiteRuntimeMemoryStore) upsertProceduralSearchIndex(ctx context.Context, record memory.ProceduralMemoryRecord) error {
	return upsertProceduralSearchIndex(ctx, s.db, record)
}

func upsertProceduralSearchIndex(ctx context.Context, db *sql.DB, record memory.ProceduralMemoryRecord) error {
	capabilityNames := proceduralCapabilityNames(record.CapabilityDependencies)
	if _, err := db.ExecContext(ctx, `DELETE FROM procedural_memory_search WHERE routine_id = ?`, record.RoutineID); err != nil {
		return err
	}
	name := strings.TrimSpace(record.Name)
	description := strings.TrimSpace(record.Description)
	summary := strings.TrimSpace(record.Summary)
	inlineBody := strings.TrimSpace(record.InlineBody)
	capabilityText := strings.Join(capabilityNames, " ")
	if _, err := db.ExecContext(ctx, `INSERT INTO procedural_memory_search(routine_id, name, name_norm, description, summary, summary_norm, inline_body, capability_names, search_text)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.RoutineID,
		name,
		normalizeRuntimeMemoryText(name),
		description,
		summary,
		normalizeRuntimeMemoryText(summary),
		inlineBody,
		capabilityText,
		proceduralSearchText(record),
	); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM procedural_memory_capabilities WHERE routine_id = ?`, record.RoutineID); err != nil {
		return err
	}
	for _, name := range capabilityNames {
		if _, err := db.ExecContext(ctx, `INSERT INTO procedural_memory_capabilities(routine_id, capability_name) VALUES (?, ?)`, record.RoutineID, name); err != nil {
			return err
		}
	}
	return nil
}

func isRuntimeMemoryFTSBacked(ctx context.Context, db *sql.DB, table string) (bool, error) {
	row := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE name = ?`, table)
	var sqlText string
	if err := row.Scan(&sqlText); err != nil {
		return false, err
	}
	return strings.Contains(strings.ToLower(sqlText), "using fts5"), nil
}

func runtimeMemoryTableExists(ctx context.Context, db *sql.DB, table string) (bool, error) {
	row := db.QueryRowContext(ctx, `SELECT 1 FROM sqlite_master WHERE name = ?`, table)
	var exists int
	if err := row.Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return exists == 1, nil
}

func runtimeMemorySearchMetaValue(ctx context.Context, db *sql.DB, key string) (string, error) {
	row := db.QueryRowContext(ctx, `SELECT value FROM runtime_memory_search_meta WHERE key = ?`, key)
	var value string
	if err := row.Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return value, nil
}

func setRuntimeMemorySearchMetaValue(ctx context.Context, db *sql.DB, key, value string) error {
	_, err := db.ExecContext(ctx, `INSERT INTO runtime_memory_search_meta(key, value)
		VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func runtimeMemoryFTSQuery(text string) string {
	terms := strings.Fields(text)
	if len(terms) <= 1 {
		return strings.TrimSpace(text)
	}
	return strings.Join(terms, " OR ")
}

func declarativeSearchText(record memory.DeclarativeMemoryRecord) string {
	parts := []string{
		normalizeRuntimeMemoryText(record.Title),
		normalizeRuntimeMemoryText(record.Summary),
		normalizeRuntimeMemoryText(record.Content),
		normalizeRuntimeMemoryTerms(record.Tags),
	}
	return strings.Join(nonEmptyParts(parts), "\n")
}

func proceduralSearchText(record memory.ProceduralMemoryRecord) string {
	parts := []string{
		normalizeRuntimeMemoryText(record.Name),
		normalizeRuntimeMemoryText(record.Description),
		normalizeRuntimeMemoryText(record.Summary),
		normalizeRuntimeMemoryText(record.InlineBody),
		strings.Join(proceduralCapabilityNames(record.CapabilityDependencies), " "),
	}
	return strings.Join(nonEmptyParts(parts), "\n")
}

func proceduralCapabilityNames(deps []core.CapabilitySelector) []string {
	if len(deps) == 0 {
		return nil
	}
	names := make([]string, 0, len(deps))
	for _, dep := range deps {
		name := strings.ToLower(strings.TrimSpace(dep.Name))
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func normalizeRuntimeMemoryTerms(values []string) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		parts = append(parts, strings.ToLower(trimmed))
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

func normalizeRuntimeMemoryText(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func nonEmptyParts(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func scoreGenericMemoryRecord(record memory.MemoryRecord, query string) float64 {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return 0
	}
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return 0
	}
	fields := []struct {
		text        string
		exactWeight float64
		termWeight  float64
	}{
		{text: strings.ToLower(firstNonEmptyString(record.Value["title"], record.Value["name"])), exactWeight: 12, termWeight: 4},
		{text: strings.ToLower(firstNonEmptyString(record.Value["summary"])), exactWeight: 8, termWeight: 3},
		{text: strings.ToLower(firstNonEmptyString(record.Value["content"], record.Value["description"], record.Value["inline_body"])), exactWeight: 5, termWeight: 1.25},
		{text: strings.ToLower(runtimeMemoryTagsText(record)), exactWeight: 4, termWeight: 1.5},
		{text: strings.ToLower(runtimeMemoryCapabilityText(record)), exactWeight: 6, termWeight: 2.5},
	}
	var score float64
	for _, field := range fields {
		if field.text == "" {
			continue
		}
		if strings.Contains(field.text, query) {
			score += field.exactWeight
		}
		for _, term := range terms {
			if strings.Contains(field.text, term) {
				score += field.termWeight
			}
		}
	}
	if boolFromAny(record.Value["verified"]) {
		score += 1.5
	}
	if reuseCount, ok := intFromAny(record.Value["reuse_count"]); ok {
		score += float64(minInt(reuseCount, 5)) * 0.2
	}
	return score
}

func runtimeMemoryTagsText(record memory.MemoryRecord) string {
	if len(record.Tags) > 0 {
		return strings.Join(record.Tags, " ")
	}
	switch tags := record.Value["tags"].(type) {
	case []string:
		return strings.Join(tags, " ")
	case []any:
		parts := make([]string, 0, len(tags))
		for _, tag := range tags {
			parts = append(parts, fmt.Sprint(tag))
		}
		return strings.Join(parts, " ")
	default:
		return ""
	}
}

func runtimeMemoryCapabilityText(record memory.MemoryRecord) string {
	deps, ok := record.Value["capability_dependencies"].([]core.CapabilitySelector)
	if !ok {
		return ""
	}
	names := make([]string, 0, len(deps))
	for _, dep := range deps {
		if name := strings.TrimSpace(dep.Name); name != "" {
			names = append(names, name)
		}
	}
	return strings.Join(names, " ")
}

func intFromAny(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float32:
		return int(typed), true
	case float64:
		return int(typed), true
	default:
		return 0, false
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *SQLiteRuntimeMemoryStore) indexDeclarativeRecord(ctx context.Context, record memory.DeclarativeMemoryRecord) error {
	if s == nil || s.db == nil {
		return nil
	}
	content := retrievalContentForDeclarative(record)
	if strings.TrimSpace(content) == "" {
		return nil
	}
	_, err := retrieval.NewIngestionPipeline(s.db, s.retrievalEmbedder).Ingest(ctx, retrieval.IngestRequest{
		CanonicalURI: declarativeRetrievalURI(record.RecordID, record.Scope),
		Content:      []byte(content),
		SourceType:   "text",
		CorpusScope:  string(record.Scope),
		PolicyTags:   append([]string{}, record.Tags...),
	})
	return err
}

func declarativeRetrievalURI(recordID string, scope memory.MemoryScope) string {
	scopeValue := strings.TrimSpace(string(scope))
	if scopeValue == "" {
		scopeValue = string(memory.MemoryScopeProject)
	}
	return fmt.Sprintf("memory://declarative/%s/%s", scopeValue, strings.TrimSpace(recordID))
}

func retrievalContentForDeclarative(record memory.DeclarativeMemoryRecord) string {
	parts := make([]string, 0, 3)
	if title := strings.TrimSpace(record.Title); title != "" {
		parts = append(parts, title)
	}
	if summary := strings.TrimSpace(record.Summary); summary != "" {
		parts = append(parts, summary)
	}
	if content := strings.TrimSpace(record.Content); content != "" {
		parts = append(parts, content)
	}
	return strings.Join(parts, "\n\n")
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func runtimeTimeString(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTimeValue(value string) time.Time {
	if strings.TrimSpace(value) == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	parts := make([]string, count)
	for i := 0; i < count; i++ {
		parts[i] = "?"
	}
	return strings.Join(parts, ",")
}

func inferDeclarativeKind(value map[string]interface{}) memory.DeclarativeMemoryKind {
	raw := strings.ToLower(strings.TrimSpace(firstNonEmptyString(value["kind"], value["type"])))
	switch memory.DeclarativeMemoryKind(raw) {
	case memory.DeclarativeMemoryKindFact,
		memory.DeclarativeMemoryKindDecision,
		memory.DeclarativeMemoryKindConstraint,
		memory.DeclarativeMemoryKindPreference,
		memory.DeclarativeMemoryKindProjectKnowledge:
		return memory.DeclarativeMemoryKind(raw)
	}
	if strings.Contains(raw, "decision") {
		return memory.DeclarativeMemoryKindDecision
	}
	if strings.Contains(raw, "constraint") {
		return memory.DeclarativeMemoryKindConstraint
	}
	if strings.Contains(raw, "preference") {
		return memory.DeclarativeMemoryKindPreference
	}
	return memory.DeclarativeMemoryKindProjectKnowledge
}

func declarativeToGenericRecord(record memory.DeclarativeMemoryRecord) memory.MemoryRecord {
	value := cloneMap(record.Metadata)
	if value == nil {
		value = map[string]interface{}{}
	}
	value["kind"] = string(record.Kind)
	value["title"] = record.Title
	value["content"] = record.Content
	value["summary"] = record.Summary
	value["task_id"] = record.TaskID
	value["workflow_id"] = record.WorkflowID
	value["project_id"] = record.ProjectID
	value["artifact_ref"] = record.ArtifactRef
	value["verified"] = record.Verified
	if len(record.Tags) > 0 {
		value["tags"] = append([]string{}, record.Tags...)
	}
	value["memory_class"] = "declarative"
	return memory.MemoryRecord{
		Key:       record.RecordID,
		Value:     value,
		Scope:     record.Scope,
		Timestamp: chooseTime(record.UpdatedAt, record.CreatedAt),
		Tags:      append([]string{}, record.Tags...),
	}
}

func proceduralToGenericRecord(record memory.ProceduralMemoryRecord) memory.MemoryRecord {
	value := cloneMap(record.VerificationMetadata)
	if value == nil {
		value = map[string]interface{}{}
	}
	value["kind"] = string(record.Kind)
	value["name"] = record.Name
	value["description"] = record.Description
	value["summary"] = record.Summary
	value["task_id"] = record.TaskID
	value["workflow_id"] = record.WorkflowID
	value["project_id"] = record.ProjectID
	value["body_ref"] = record.BodyRef
	value["inline_body"] = record.InlineBody
	value["policy_snapshot_id"] = record.PolicySnapshotID
	value["verified"] = record.Verified
	value["version"] = record.Version
	value["reuse_count"] = record.ReuseCount
	value["memory_class"] = "procedural"
	if len(record.CapabilityDependencies) > 0 {
		value["capability_dependencies"] = record.CapabilityDependencies
	}
	return memory.MemoryRecord{
		Key:       record.RoutineID,
		Value:     value,
		Scope:     record.Scope,
		Timestamp: chooseTime(record.UpdatedAt, record.CreatedAt),
	}
}

func chooseTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}

func cloneMap(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if strings.TrimSpace(fmt.Sprint(value)) != "" && fmt.Sprint(value) != "<nil>" {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return ""
}

func boolFromAny(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	default:
		return false
	}
}

// loadBoundAnchorIDs extracts anchor IDs from record metadata.
func loadBoundAnchorIDs(record any) []string {
	var metadata map[string]any
	switch r := record.(type) {
	case memory.DeclarativeMemoryRecord:
		metadata = r.Metadata
	case *memory.DeclarativeMemoryRecord:
		if r != nil {
			metadata = r.Metadata
		}
	case memory.ProceduralMemoryRecord:
		metadata = r.VerificationMetadata
	case *memory.ProceduralMemoryRecord:
		if r != nil {
			metadata = r.VerificationMetadata
		}
	default:
		return nil
	}

	if metadata == nil {
		return nil
	}

	raw, ok := metadata["bound_anchors"]
	if !ok {
		return nil
	}

	// Handle []interface{} from JSON unmarshaling
	if anchors, ok := raw.([]interface{}); ok {
		result := make([]string, 0, len(anchors))
		for _, anchor := range anchors {
			if id, ok := anchor.(string); ok && strings.TrimSpace(id) != "" {
				result = append(result, strings.TrimSpace(id))
			}
		}
		return result
	}

	// Handle []string directly
	if anchors, ok := raw.([]string); ok {
		return anchors
	}

	return nil
}

// extractKeywords extracts potential anchor keywords from content (words > 3 chars).
func extractKeywords(content string) []string {
	if strings.TrimSpace(content) == "" {
		return nil
	}

	// Simple word extraction: split by whitespace and filter by length
	words := strings.FieldsFunc(content, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == ',' || r == '.' || r == ';' || r == ':' || r == '!' || r == '?'
	})

	seen := make(map[string]bool)
	var keywords []string
	for _, word := range words {
		word = strings.ToLower(strings.TrimSpace(word))
		// Keep words that are at least 4 characters and not duplicates
		if len(word) >= 4 && !seen[word] {
			keywords = append(keywords, word)
			seen[word] = true
		}
	}
	return keywords
}

// bindAnchorIDsToMetadata extracts keywords from content and queries for matching anchors,
// storing the anchor IDs in metadata["bound_anchors"].
func bindAnchorIDsToMetadata(ctx context.Context, db *sql.DB, content string, metadata map[string]any, corpusScope string) error {
	if db == nil || strings.TrimSpace(content) == "" || strings.TrimSpace(corpusScope) == "" {
		return nil
	}

	if metadata == nil {
		return nil
	}

	keywords := extractKeywords(content)
	if len(keywords) == 0 {
		return nil
	}

	// Query for active anchors matching these terms
	anchors, err := retrieval.AnchorsForTerms(ctx, db, keywords, corpusScope)
	if err != nil {
		// Non-fatal: if anchor binding fails, continue without it
		return nil
	}

	if len(anchors) == 0 {
		return nil
	}

	// Extract anchor IDs and store in metadata
	var anchorIDs []string
	for _, anchor := range anchors {
		if strings.TrimSpace(anchor.AnchorID) != "" {
			anchorIDs = append(anchorIDs, anchor.AnchorID)
		}
	}

	if len(anchorIDs) > 0 {
		metadata["bound_anchors"] = anchorIDs
	}

	return nil
}

// checkStaleAnchors queries the database for the status of bound anchors.
// Returns AnchorRef entries for any that are superseded or invalidated.
func checkStaleAnchors(ctx context.Context, db *sql.DB, boundAnchorIDs []string) ([]memory.AnchorRef, error) {
	if db == nil || len(boundAnchorIDs) == 0 {
		return nil, nil
	}

	placeholders := strings.Repeat("?,", len(boundAnchorIDs)-1) + "?"
	args := make([]any, len(boundAnchorIDs))
	for i, id := range boundAnchorIDs {
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT anchor_id, term, definition, anchor_class, created_at
		FROM retrieval_semantic_anchors
		WHERE anchor_id IN (%s) AND (superseded_by IS NOT NULL OR invalidated_at IS NOT NULL)
	`, placeholders)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var staleAnchors []memory.AnchorRef
	for rows.Next() {
		var ref memory.AnchorRef
		if err := rows.Scan(&ref.AnchorID, &ref.Term, &ref.Definition, &ref.Class, &ref.CreatedAt); err != nil {
			return nil, err
		}
		ref.Active = false
		staleAnchors = append(staleAnchors, ref)
	}
	return staleAnchors, rows.Err()
}
