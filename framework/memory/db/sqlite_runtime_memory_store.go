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

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/framework/retrieval"
	_ "github.com/mattn/go-sqlite3"
)

// SQLiteRuntimeMemoryStore persists declarative and procedural memory in separate tables.
type SQLiteRuntimeMemoryStore struct {
	db                *sql.DB
	retrieve          retrieval.RetrieverService
	retrievalEmbedder retrieval.Embedder
}

func NewSQLiteRuntimeMemoryStore(path string) (*SQLiteRuntimeMemoryStore, error) {
	return NewSQLiteRuntimeMemoryStoreWithRetrieval(path, SQLiteRuntimeRetrievalOptions{})
}

// SQLiteRuntimeRetrievalOptions controls retrieval-service wiring for the runtime store.
type SQLiteRuntimeRetrievalOptions struct {
	Embedder retrieval.Embedder
	Telemetry core.Telemetry
	ServiceOptions retrieval.ServiceOptions
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
	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.After(out[j].Timestamp)
	})
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
	return retrieval.EnsureSchema(context.Background(), s.db)
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
	return record, true, nil
}

func (s *SQLiteRuntimeMemoryStore) SearchDeclarative(ctx context.Context, query memory.DeclarativeMemoryQuery) ([]memory.DeclarativeMemoryRecord, error) {
	if query.Limit <= 0 {
		query.Limit = 5
	}
	var b strings.Builder
	b.WriteString(`SELECT record_id, scope, kind, title, content, summary, workflow_id, task_id, project_id, artifact_ref, tags_json, metadata_json, verified, created_at, updated_at FROM declarative_memory WHERE 1=1`)
	args := make([]any, 0, 8)
	if query.Scope != "" {
		b.WriteString(` AND scope = ?`)
		args = append(args, string(query.Scope))
	}
	if query.TaskID != "" {
		b.WriteString(` AND task_id = ?`)
		args = append(args, query.TaskID)
	}
	if query.WorkflowID != "" {
		b.WriteString(` AND workflow_id = ?`)
		args = append(args, query.WorkflowID)
	}
	if query.ProjectID != "" {
		b.WriteString(` AND project_id = ?`)
		args = append(args, query.ProjectID)
	}
	if len(query.Kinds) > 0 {
		b.WriteString(` AND kind IN (` + placeholders(len(query.Kinds)) + `)`)
		for _, kind := range query.Kinds {
			args = append(args, string(kind))
		}
	}
	if strings.TrimSpace(query.Query) != "" {
		b.WriteString(` AND (lower(title) LIKE ? OR lower(content) LIKE ? OR lower(summary) LIKE ?)`)
		needle := "%" + strings.ToLower(strings.TrimSpace(query.Query)) + "%"
		args = append(args, needle, needle, needle)
	}
	b.WriteString(` ORDER BY verified DESC, updated_at DESC LIMIT ?`)
	args = append(args, query.Limit)
	rows, err := s.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []memory.DeclarativeMemoryRecord
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
	return err
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
	return record, true, nil
}

func (s *SQLiteRuntimeMemoryStore) SearchProcedural(ctx context.Context, query memory.ProceduralMemoryQuery) ([]memory.ProceduralMemoryRecord, error) {
	if query.Limit <= 0 {
		query.Limit = 5
	}
	var b strings.Builder
	b.WriteString(`SELECT routine_id, scope, kind, name, description, summary, workflow_id, task_id, project_id, body_ref, inline_body, capability_dependencies_json, verification_metadata_json, policy_snapshot_id, verified, version, reuse_count, created_at, updated_at FROM procedural_memory WHERE 1=1`)
	args := make([]any, 0, 8)
	if query.Scope != "" {
		b.WriteString(` AND scope = ?`)
		args = append(args, string(query.Scope))
	}
	if query.TaskID != "" {
		b.WriteString(` AND task_id = ?`)
		args = append(args, query.TaskID)
	}
	if query.WorkflowID != "" {
		b.WriteString(` AND workflow_id = ?`)
		args = append(args, query.WorkflowID)
	}
	if query.ProjectID != "" {
		b.WriteString(` AND project_id = ?`)
		args = append(args, query.ProjectID)
	}
	if len(query.Kinds) > 0 {
		b.WriteString(` AND kind IN (` + placeholders(len(query.Kinds)) + `)`)
		for _, kind := range query.Kinds {
			args = append(args, string(kind))
		}
	}
	if strings.TrimSpace(query.Query) != "" {
		b.WriteString(` AND (lower(name) LIKE ? OR lower(description) LIKE ? OR lower(summary) LIKE ? OR lower(inline_body) LIKE ?)`)
		needle := "%" + strings.ToLower(strings.TrimSpace(query.Query)) + "%"
		args = append(args, needle, needle, needle, needle)
	}
	if strings.TrimSpace(query.CapabilityName) != "" {
		b.WriteString(` AND lower(capability_dependencies_json) LIKE ?`)
		args = append(args, "%"+strings.ToLower(strings.TrimSpace(query.CapabilityName))+"%")
	}
	b.WriteString(` ORDER BY verified DESC, reuse_count DESC, updated_at DESC LIMIT ?`)
	args = append(args, query.Limit)
	rows, err := s.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []memory.ProceduralMemoryRecord
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
	return err
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
	_ = json.Unmarshal([]byte(tagsJSON), &record.Tags)
	_ = json.Unmarshal([]byte(metadataJSON), &record.Metadata)
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
	_ = json.Unmarshal([]byte(depsJSON), &record.CapabilityDependencies)
	_ = json.Unmarshal([]byte(verificationJSON), &record.VerificationMetadata)
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
