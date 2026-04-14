package db

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
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

// WorkflowStateSchemaVersion is the current schema version for workflow state storage.
const WorkflowStateSchemaVersion = 8
const workflowStateSchemaVersion = WorkflowStateSchemaVersion

// SQLiteWorkflowStateStore persists workflow state in SQLite.
type SQLiteWorkflowStateStore struct {
	db                *sql.DB
	retrieve          retrieval.RetrieverService
	retrievalEmbedder retrieval.Embedder
}

// NewSQLiteWorkflowStateStore opens or creates the workflow state database.
func NewSQLiteWorkflowStateStore(dbPath string) (*SQLiteWorkflowStateStore, error) {
	return NewSQLiteWorkflowStateStoreWithRetrieval(dbPath, SQLiteWorkflowRetrievalOptions{})
}

// SQLiteWorkflowRetrievalOptions controls retrieval-service wiring for the workflow store.
type SQLiteWorkflowRetrievalOptions struct {
	Embedder       retrieval.Embedder
	Telemetry      core.Telemetry
	ServiceOptions retrieval.ServiceOptions
}

// NewSQLiteWorkflowStateStoreWithRetrieval opens or creates the workflow state database
// and configures retrieval with the supplied runtime dependencies.
func NewSQLiteWorkflowStateStoreWithRetrieval(dbPath string, opts SQLiteWorkflowRetrievalOptions) (*SQLiteWorkflowStateStore, error) {
	if strings.TrimSpace(dbPath) == "" {
		return nil, errors.New("workflow state db path required")
	}
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000", filepath.Clean(dbPath))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	store := &SQLiteWorkflowStateStore{
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

// Close releases the underlying database handle.
func (s *SQLiteWorkflowStateStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// DB exposes the underlying SQLite handle for package-local adapters.
func (s *SQLiteWorkflowStateStore) DB() *sql.DB {
	if s == nil {
		return nil
	}
	return s.db
}

// EnsureRetrievalSchema provisions retrieval tables in the same SQLite database
// used for workflow state.
func (s *SQLiteWorkflowStateStore) EnsureRetrievalSchema(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("workflow state db required")
	}
	return retrieval.EnsureSchema(ctx, s.db)
}

// RetrievalService exposes retrieval over the same workflow SQLite database.
func (s *SQLiteWorkflowStateStore) RetrievalService() retrieval.RetrieverService {
	if s == nil {
		return nil
	}
	return s.retrieve
}

func (s *SQLiteWorkflowStateStore) init() error {
	stmts := []string{
		`PRAGMA foreign_keys = ON;`,
		`CREATE TABLE IF NOT EXISTS schema_metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS workflows (
			workflow_id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			task_type TEXT NOT NULL,
			instruction TEXT NOT NULL,
			status TEXT NOT NULL,
			cursor_step_id TEXT NOT NULL DEFAULT '',
			version INTEGER NOT NULL DEFAULT 1,
			metadata_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_workflows_status ON workflows(status);`,
		`CREATE TABLE IF NOT EXISTS workflow_runs (
			run_id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			status TEXT NOT NULL,
			agent_name TEXT NOT NULL DEFAULT '',
			agent_mode TEXT NOT NULL DEFAULT '',
			runtime_version TEXT NOT NULL DEFAULT '',
			metadata_json TEXT NOT NULL DEFAULT '{}',
			started_at TEXT NOT NULL,
			finished_at TEXT,
			FOREIGN KEY(workflow_id) REFERENCES workflows(workflow_id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_runs_status ON workflow_runs(status);`,
		`CREATE TABLE IF NOT EXISTS workflow_plans (
			plan_id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			run_id TEXT NOT NULL,
			plan_hash TEXT NOT NULL,
			plan_json TEXT NOT NULL,
			is_active INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			FOREIGN KEY(workflow_id) REFERENCES workflows(workflow_id) ON DELETE CASCADE,
			FOREIGN KEY(run_id) REFERENCES workflow_runs(run_id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS workflow_steps (
			workflow_id TEXT NOT NULL,
			plan_id TEXT NOT NULL,
			step_id TEXT NOT NULL,
			ordinal INTEGER NOT NULL,
			step_json TEXT NOT NULL,
			status TEXT NOT NULL,
			summary TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL,
			PRIMARY KEY(workflow_id, step_id),
			FOREIGN KEY(workflow_id) REFERENCES workflows(workflow_id) ON DELETE CASCADE,
			FOREIGN KEY(plan_id) REFERENCES workflow_plans(plan_id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS workflow_step_dependencies (
			workflow_id TEXT NOT NULL,
			step_id TEXT NOT NULL,
			dependency_step_id TEXT NOT NULL,
			PRIMARY KEY(workflow_id, step_id, dependency_step_id),
			FOREIGN KEY(workflow_id, step_id) REFERENCES workflow_steps(workflow_id, step_id) ON DELETE CASCADE,
			FOREIGN KEY(workflow_id, dependency_step_id) REFERENCES workflow_steps(workflow_id, step_id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS step_runs (
			step_run_id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			run_id TEXT NOT NULL,
			step_id TEXT NOT NULL,
			attempt INTEGER NOT NULL,
			status TEXT NOT NULL,
			summary TEXT NOT NULL DEFAULT '',
			result_json TEXT NOT NULL DEFAULT '{}',
			verification_ok INTEGER NOT NULL DEFAULT 0,
			error_text TEXT NOT NULL DEFAULT '',
			started_at TEXT NOT NULL,
			finished_at TEXT,
			FOREIGN KEY(workflow_id) REFERENCES workflows(workflow_id) ON DELETE CASCADE,
			FOREIGN KEY(run_id) REFERENCES workflow_runs(run_id) ON DELETE CASCADE,
			FOREIGN KEY(workflow_id, step_id) REFERENCES workflow_steps(workflow_id, step_id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS step_artifacts (
			artifact_id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			step_run_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			content_type TEXT NOT NULL,
			storage_kind TEXT NOT NULL,
			summary_text TEXT NOT NULL DEFAULT '',
			summary_metadata_json TEXT NOT NULL DEFAULT '{}',
			inline_raw_text TEXT NOT NULL DEFAULT '',
			raw_ref TEXT NOT NULL DEFAULT '',
			raw_size_bytes INTEGER NOT NULL DEFAULT 0,
			compression_method TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			FOREIGN KEY(workflow_id) REFERENCES workflows(workflow_id) ON DELETE CASCADE,
			FOREIGN KEY(step_run_id) REFERENCES step_runs(step_run_id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS workflow_artifacts (
			artifact_id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			run_id TEXT,
			kind TEXT NOT NULL,
			content_type TEXT NOT NULL,
			storage_kind TEXT NOT NULL,
			summary_text TEXT NOT NULL DEFAULT '',
			summary_metadata_json TEXT NOT NULL DEFAULT '{}',
			inline_raw_text TEXT NOT NULL DEFAULT '',
			raw_ref TEXT NOT NULL DEFAULT '',
			raw_size_bytes INTEGER NOT NULL DEFAULT 0,
			compression_method TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			FOREIGN KEY(workflow_id) REFERENCES workflows(workflow_id) ON DELETE CASCADE,
			FOREIGN KEY(run_id) REFERENCES workflow_runs(run_id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS workflow_stage_results (
			result_id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			run_id TEXT NOT NULL,
			stage_name TEXT NOT NULL,
			stage_index INTEGER NOT NULL DEFAULT 0,
			contract_name TEXT NOT NULL DEFAULT '',
			contract_version TEXT NOT NULL DEFAULT '',
			prompt_text TEXT NOT NULL DEFAULT '',
			response_json TEXT NOT NULL DEFAULT '',
			decoded_output_json TEXT NOT NULL DEFAULT 'null',
			validation_ok INTEGER NOT NULL DEFAULT 0,
			error_text TEXT NOT NULL DEFAULT '',
			retry_attempt INTEGER NOT NULL DEFAULT 0,
			transition_kind TEXT NOT NULL DEFAULT '',
			next_stage TEXT NOT NULL DEFAULT '',
			transition_reason TEXT NOT NULL DEFAULT '',
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL,
			FOREIGN KEY(workflow_id) REFERENCES workflows(workflow_id) ON DELETE CASCADE,
			FOREIGN KEY(run_id) REFERENCES workflow_runs(run_id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS pipeline_checkpoints (
			checkpoint_id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			workflow_id TEXT NOT NULL DEFAULT '',
			run_id TEXT NOT NULL DEFAULT '',
			stage_name TEXT NOT NULL DEFAULT '',
			stage_index INTEGER NOT NULL DEFAULT 0,
			context_json TEXT NOT NULL DEFAULT '',
			result_json TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS workflow_knowledge (
			record_id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			step_run_id TEXT NOT NULL DEFAULT '',
			step_id TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT '',
			metadata_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			FOREIGN KEY(workflow_id) REFERENCES workflows(workflow_id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS workflow_events (
			event_id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			run_id TEXT NOT NULL DEFAULT '',
			step_id TEXT NOT NULL DEFAULT '',
			event_type TEXT NOT NULL,
			message TEXT NOT NULL DEFAULT '',
			metadata_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			FOREIGN KEY(workflow_id) REFERENCES workflows(workflow_id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS graph_checkpoints (
			checkpoint_id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			workflow_id TEXT NOT NULL DEFAULT '',
			run_id TEXT NOT NULL DEFAULT '',
			completed_node_id TEXT NOT NULL DEFAULT '',
			next_node_id TEXT NOT NULL DEFAULT '',
			graph_hash TEXT NOT NULL DEFAULT '',
			state_json TEXT NOT NULL DEFAULT 'null',
			transition_json TEXT NOT NULL DEFAULT 'null',
			metadata_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS workflow_provider_snapshots (
			snapshot_id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			run_id TEXT NOT NULL DEFAULT '',
			provider_id TEXT NOT NULL,
			recoverability TEXT NOT NULL DEFAULT '',
			descriptor_json TEXT NOT NULL DEFAULT '{}',
			health_json TEXT NOT NULL DEFAULT '{}',
			capability_ids_json TEXT NOT NULL DEFAULT '[]',
			task_id TEXT NOT NULL DEFAULT '',
			metadata_json TEXT NOT NULL DEFAULT '{}',
			state_json TEXT NOT NULL DEFAULT 'null',
			captured_at TEXT NOT NULL,
			FOREIGN KEY(workflow_id) REFERENCES workflows(workflow_id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS workflow_provider_session_snapshots (
			snapshot_id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			run_id TEXT NOT NULL DEFAULT '',
			session_id TEXT NOT NULL,
			provider_id TEXT NOT NULL,
			session_json TEXT NOT NULL DEFAULT '{}',
			metadata_json TEXT NOT NULL DEFAULT '{}',
			state_json TEXT NOT NULL DEFAULT 'null',
			captured_at TEXT NOT NULL,
			FOREIGN KEY(workflow_id) REFERENCES workflows(workflow_id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS workflow_delegations (
			delegation_id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			run_id TEXT NOT NULL DEFAULT '',
			task_id TEXT NOT NULL DEFAULT '',
			state TEXT NOT NULL,
			trust_class TEXT NOT NULL DEFAULT '',
			recoverability TEXT NOT NULL DEFAULT '',
			background INTEGER NOT NULL DEFAULT 0,
			request_json TEXT NOT NULL DEFAULT '{}',
			result_json TEXT NOT NULL DEFAULT 'null',
			metadata_json TEXT NOT NULL DEFAULT '{}',
			started_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(workflow_id) REFERENCES workflows(workflow_id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS workflow_delegation_transitions (
			transition_id TEXT PRIMARY KEY,
			delegation_id TEXT NOT NULL,
			workflow_id TEXT NOT NULL,
			run_id TEXT NOT NULL DEFAULT '',
			from_state TEXT NOT NULL DEFAULT '',
			to_state TEXT NOT NULL,
			metadata_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL,
			FOREIGN KEY(workflow_id) REFERENCES workflows(workflow_id) ON DELETE CASCADE,
			FOREIGN KEY(delegation_id) REFERENCES workflow_delegations(delegation_id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS workflow_invalidation (
			invalidation_id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			source_step_id TEXT NOT NULL,
			invalidated_step_id TEXT NOT NULL,
			reason TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			FOREIGN KEY(workflow_id) REFERENCES workflows(workflow_id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS rex_fmp_lineage_bindings (
			workflow_id TEXT NOT NULL,
			run_id TEXT NOT NULL,
			lineage_id TEXT NOT NULL,
			attempt_id TEXT NOT NULL,
			runtime_id TEXT NOT NULL DEFAULT '',
			session_id TEXT NOT NULL DEFAULT '',
			state TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL,
			PRIMARY KEY (workflow_id, run_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_steps_status ON workflow_steps(workflow_id, status, ordinal);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_step_runs_attempt ON step_runs(workflow_id, step_id, attempt);`,
		`CREATE INDEX IF NOT EXISTS idx_step_runs_workflow_step ON step_runs(workflow_id, step_id, attempt DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_artifacts_scope ON workflow_artifacts(workflow_id, run_id, created_at ASC);`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_artifacts_kind ON workflow_artifacts(workflow_id, kind, created_at ASC);`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_artifacts_kind_workspace ON workflow_artifacts(workflow_id, kind, json_extract(summary_metadata_json, '$.workspace_id'), created_at ASC);`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_artifacts_workspace_kind ON workflow_artifacts(kind, json_extract(summary_metadata_json, '$.workspace_id'), created_at ASC);`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_stage_results_scope ON workflow_stage_results(workflow_id, run_id, stage_index ASC, retry_attempt ASC, finished_at ASC);`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_stage_results_valid ON workflow_stage_results(workflow_id, run_id, stage_name, validation_ok, retry_attempt DESC, finished_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_pipeline_checkpoints_task_created ON pipeline_checkpoints(task_id, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_events_workflow_created ON workflow_events(workflow_id, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_graph_checkpoints_task_created ON graph_checkpoints(task_id, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_provider_snapshots_scope ON workflow_provider_snapshots(workflow_id, run_id, captured_at ASC);`,
		`CREATE INDEX IF NOT EXISTS idx_provider_session_snapshots_scope ON workflow_provider_session_snapshots(workflow_id, run_id, captured_at ASC);`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_delegations_scope ON workflow_delegations(workflow_id, run_id, updated_at ASC);`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_delegation_transitions_scope ON workflow_delegation_transitions(delegation_id, created_at ASC);`,
		`CREATE INDEX IF NOT EXISTS idx_knowledge_workflow_kind ON workflow_knowledge(workflow_id, kind, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_rex_lineage_by_lineage ON rex_fmp_lineage_bindings(lineage_id);`,
		`CREATE INDEX IF NOT EXISTS idx_rex_lineage_by_attempt ON rex_fmp_lineage_bindings(attempt_id);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	currentVersion, err := s.schemaVersionValue(context.Background())
	if err != nil {
		return err
	}
	if currentVersion < workflowStateSchemaVersion {
		if err := s.migrateLineageBindings(context.Background()); err != nil {
			return err
		}
	}
	if _, err := s.db.Exec(`INSERT INTO schema_metadata (key, value) VALUES ('workflow_state_schema_version', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, fmt.Sprintf("%d", workflowStateSchemaVersion)); err != nil {
		return err
	}
	return nil
}

func (s *SQLiteWorkflowStateStore) SchemaVersion(ctx context.Context) (int, error) {
	return s.schemaVersionValue(ctx)
}

func (s *SQLiteWorkflowStateStore) schemaVersionValue(ctx context.Context) (int, error) {
	row := s.db.QueryRowContext(ctx, `SELECT value FROM schema_metadata WHERE key = 'workflow_state_schema_version'`)
	var value string
	if err := row.Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	var version int
	if _, err := fmt.Sscanf(value, "%d", &version); err != nil {
		return 0, err
	}
	return version, nil
}

func (s *SQLiteWorkflowStateStore) migrateLineageBindings(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT workflow_id, run_id, kind, inline_raw_text, created_at FROM workflow_artifacts WHERE kind = 'rex.fmp_lineage'`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var workflowID, runID, kind, rawText, createdAt string
		if err := rows.Scan(&workflowID, &runID, &kind, &rawText, &createdAt); err != nil {
			return err
		}
		if strings.TrimSpace(rawText) == "" {
			continue
		}
		var record LineageBindingRecord
		if err := json.Unmarshal([]byte(rawText), &record); err != nil {
			return err
		}
		if strings.TrimSpace(record.WorkflowID) == "" {
			record.WorkflowID = strings.TrimSpace(workflowID)
		}
		if strings.TrimSpace(record.RunID) == "" {
			record.RunID = firstNonEmptyString(strings.TrimSpace(runID), record.AttemptID)
		}
		if strings.TrimSpace(record.LineageID) == "" || strings.TrimSpace(record.AttemptID) == "" || strings.TrimSpace(record.WorkflowID) == "" || strings.TrimSpace(record.RunID) == "" {
			continue
		}
		if strings.TrimSpace(record.RuntimeID) == "" {
			record.RuntimeID = "rex"
		}
		if record.UpdatedAt.IsZero() {
			record.UpdatedAt = parseTime(createdAt)
		}
		if err := upsertLineageBindingTx(ctx, tx, record); err != nil {
			return err
		}
		_ = kind
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteWorkflowStateStore) CreateWorkflow(ctx context.Context, workflow memory.WorkflowRecord) error {
	if workflow.WorkflowID == "" {
		return errors.New("workflow id required")
	}
	now := ensureTime(workflow.CreatedAt)
	workflow.CreatedAt = now
	workflow.UpdatedAt = ensureTime(workflow.UpdatedAt)
	if workflow.UpdatedAt.IsZero() {
		workflow.UpdatedAt = now
	}
	if workflow.Version <= 0 {
		workflow.Version = 1
	}
	if workflow.Status == "" {
		workflow.Status = memory.WorkflowRunStatusPending
	}
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO workflows (workflow_id, task_id, task_type, instruction, status, cursor_step_id, version, metadata_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		workflow.WorkflowID,
		workflow.TaskID,
		string(workflow.TaskType),
		workflow.Instruction,
		string(workflow.Status),
		workflow.CursorStepID,
		workflow.Version,
		mustJSON(workflow.Metadata),
		timeString(workflow.CreatedAt),
		timeString(workflow.UpdatedAt),
	)
	return err
}

func (s *SQLiteWorkflowStateStore) GetWorkflow(ctx context.Context, workflowID string) (*memory.WorkflowRecord, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT workflow_id, task_id, task_type, instruction, status, cursor_step_id, version, metadata_json, created_at, updated_at FROM workflows WHERE workflow_id = ?`, workflowID)
	record, ok, err := scanWorkflow(row)
	return record, ok, err
}

func (s *SQLiteWorkflowStateStore) ListWorkflows(ctx context.Context, limit int) ([]memory.WorkflowRecord, error) {
	query := `SELECT workflow_id, task_id, task_type, instruction, status, cursor_step_id, version, metadata_json, created_at, updated_at FROM workflows ORDER BY updated_at DESC`
	var rows *sql.Rows
	var err error
	if limit > 0 {
		query += ` LIMIT ?`
		rows, err = s.db.QueryContext(ctx, query, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, query)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []memory.WorkflowRecord
	for rows.Next() {
		record, err := scanWorkflowRows(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, *record)
	}
	return records, rows.Err()
}

func (s *SQLiteWorkflowStateStore) UpdateWorkflowStatus(ctx context.Context, workflowID string, expectedVersion int64, status memory.WorkflowRunStatus, cursorStepID string) (int64, error) {
	if workflowID == "" {
		return 0, errors.New("workflow id required")
	}
	query := `UPDATE workflows SET status = ?, cursor_step_id = ?, version = version + 1, updated_at = ? WHERE workflow_id = ?`
	args := []any{string(status), cursorStepID, timeString(time.Now().UTC()), workflowID}
	if expectedVersion > 0 {
		query += ` AND version = ?`
		args = append(args, expectedVersion)
	}
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if affected == 0 {
		return 0, sql.ErrNoRows
	}
	record, ok, err := s.GetWorkflow(ctx, workflowID)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, sql.ErrNoRows
	}
	return record.Version, nil
}

func (s *SQLiteWorkflowStateStore) CreateRun(ctx context.Context, run memory.WorkflowRunRecord) error {
	if run.RunID == "" || run.WorkflowID == "" {
		return errors.New("run id and workflow id required")
	}
	if run.Status == "" {
		run.Status = memory.WorkflowRunStatusPending
	}
	run.StartedAt = ensureTime(run.StartedAt)
	var finishedAt any
	if run.FinishedAt != nil {
		finishedAt = timeString(*run.FinishedAt)
	}
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO workflow_runs (run_id, workflow_id, status, agent_name, agent_mode, runtime_version, metadata_json, started_at, finished_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.RunID,
		run.WorkflowID,
		string(run.Status),
		run.AgentName,
		run.AgentMode,
		run.RuntimeVersion,
		mustJSON(run.Metadata),
		timeString(run.StartedAt),
		finishedAt,
	)
	return err
}

func (s *SQLiteWorkflowStateStore) GetRun(ctx context.Context, runID string) (*memory.WorkflowRunRecord, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT run_id, workflow_id, status, agent_name, agent_mode, runtime_version, metadata_json, started_at, finished_at FROM workflow_runs WHERE run_id = ?`, runID)
	var record memory.WorkflowRunRecord
	var metadataJSON string
	var startedAt string
	var finishedAt sql.NullString
	err := row.Scan(&record.RunID, &record.WorkflowID, &record.Status, &record.AgentName, &record.AgentMode, &record.RuntimeVersion, &metadataJSON, &startedAt, &finishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	record.Metadata = decodeJSONMap(metadataJSON)
	record.StartedAt = parseTime(startedAt)
	if finishedAt.Valid {
		t := parseTime(finishedAt.String)
		record.FinishedAt = &t
	}
	return &record, true, nil
}

func (s *SQLiteWorkflowStateStore) AggregateWorkflowStatusCounts(ctx context.Context) (map[memory.WorkflowRunStatus]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM workflows GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[memory.WorkflowRunStatus]int{}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[memory.WorkflowRunStatus(strings.TrimSpace(status))] = count
	}
	return counts, rows.Err()
}

func (s *SQLiteWorkflowStateStore) ListRunsByStatus(ctx context.Context, statuses []memory.WorkflowRunStatus, limit int) ([]memory.WorkflowRunRecord, error) {
	if len(statuses) == 0 {
		return nil, nil
	}
	placeholders := make([]string, 0, len(statuses))
	args := make([]any, 0, len(statuses)+1)
	for _, status := range statuses {
		placeholders = append(placeholders, "?")
		args = append(args, string(status))
	}
	query := `SELECT run_id, workflow_id, status, agent_name, agent_mode, runtime_version, metadata_json, started_at, finished_at FROM workflow_runs WHERE status IN (` + strings.Join(placeholders, ",") + `) ORDER BY started_at DESC`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []memory.WorkflowRunRecord
	for rows.Next() {
		var record memory.WorkflowRunRecord
		var metadataJSON string
		var startedAt string
		var finishedAt sql.NullString
		if err := rows.Scan(&record.RunID, &record.WorkflowID, &record.Status, &record.AgentName, &record.AgentMode, &record.RuntimeVersion, &metadataJSON, &startedAt, &finishedAt); err != nil {
			return nil, err
		}
		record.Metadata = decodeJSONMap(metadataJSON)
		record.StartedAt = parseTime(startedAt)
		if finishedAt.Valid {
			t := parseTime(finishedAt.String)
			record.FinishedAt = &t
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *SQLiteWorkflowStateStore) UpdateRunStatus(ctx context.Context, runID string, status memory.WorkflowRunStatus, finishedAt *time.Time) error {
	if runID == "" {
		return errors.New("run id required")
	}
	var finished any
	if finishedAt != nil {
		finished = timeString(finishedAt.UTC())
	}
	_, err := s.db.ExecContext(ctx, `UPDATE workflow_runs SET status = ?, finished_at = ? WHERE run_id = ?`, string(status), finished, runID)
	return err
}

func (s *SQLiteWorkflowStateStore) SavePlan(ctx context.Context, plan memory.WorkflowPlanRecord) error {
	if plan.PlanID == "" || plan.WorkflowID == "" || plan.RunID == "" {
		return errors.New("plan id, workflow id, and run id required")
	}
	if plan.CreatedAt.IsZero() {
		plan.CreatedAt = time.Now().UTC()
	}
	if plan.PlanHash == "" {
		plan.PlanHash = planHash(plan.Plan)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE workflow_plans SET is_active = 0 WHERE workflow_id = ?`, plan.WorkflowID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO workflow_plans (plan_id, workflow_id, run_id, plan_hash, plan_json, is_active, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		plan.PlanID,
		plan.WorkflowID,
		plan.RunID,
		plan.PlanHash,
		mustJSON(plan.Plan),
		boolInt(plan.IsActive),
		timeString(plan.CreatedAt),
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM workflow_step_dependencies WHERE workflow_id = ?`, plan.WorkflowID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM workflow_steps WHERE workflow_id = ?`, plan.WorkflowID); err != nil {
		return err
	}
	for idx, step := range plan.Plan.Steps {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO workflow_steps (workflow_id, plan_id, step_id, ordinal, step_json, status, summary, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, '', ?)`,
			plan.WorkflowID,
			plan.PlanID,
			step.ID,
			idx,
			mustJSON(step),
			string(memory.StepStatusPending),
			timeString(time.Now().UTC()),
		); err != nil {
			return err
		}
		for _, depID := range plan.Plan.Dependencies[step.ID] {
			if _, err := tx.ExecContext(
				ctx,
				`INSERT INTO workflow_step_dependencies (workflow_id, step_id, dependency_step_id) VALUES (?, ?, ?)`,
				plan.WorkflowID,
				step.ID,
				depID,
			); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *SQLiteWorkflowStateStore) GetActivePlan(ctx context.Context, workflowID string) (*memory.WorkflowPlanRecord, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT plan_id, workflow_id, run_id, plan_hash, plan_json, is_active, created_at FROM workflow_plans WHERE workflow_id = ? AND is_active = 1 ORDER BY created_at DESC LIMIT 1`, workflowID)
	var record memory.WorkflowPlanRecord
	var planJSON string
	var createdAt string
	var isActive int
	err := row.Scan(&record.PlanID, &record.WorkflowID, &record.RunID, &record.PlanHash, &planJSON, &isActive, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	record.IsActive = isActive == 1
	record.CreatedAt = parseTime(createdAt)
	if err := json.Unmarshal([]byte(planJSON), &record.Plan); err != nil {
		return nil, false, err
	}
	return &record, true, nil
}

func (s *SQLiteWorkflowStateStore) ListSteps(ctx context.Context, workflowID string) ([]memory.WorkflowStepRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT workflow_id, plan_id, step_id, ordinal, step_json, status, summary, updated_at FROM workflow_steps WHERE workflow_id = ? ORDER BY ordinal ASC`, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []memory.WorkflowStepRecord
	for rows.Next() {
		record, err := scanStepRows(rows)
		if err != nil {
			return nil, err
		}
		deps, err := s.stepDependencies(ctx, workflowID, record.StepID)
		if err != nil {
			return nil, err
		}
		record.Dependencies = deps
		records = append(records, *record)
	}
	return records, rows.Err()
}

func (s *SQLiteWorkflowStateStore) ListReadySteps(ctx context.Context, workflowID string) ([]memory.WorkflowStepRecord, error) {
	steps, err := s.ListSteps(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	statusByStep := make(map[string]memory.StepStatus, len(steps))
	for _, step := range steps {
		statusByStep[step.StepID] = step.Status
	}
	var ready []memory.WorkflowStepRecord
	for _, step := range steps {
		if step.Status != memory.StepStatusPending {
			continue
		}
		ok := true
		for _, dep := range step.Dependencies {
			if statusByStep[dep] != memory.StepStatusCompleted {
				ok = false
				break
			}
		}
		if ok {
			ready = append(ready, step)
		}
	}
	return ready, nil
}

func (s *SQLiteWorkflowStateStore) UpdateStepStatus(ctx context.Context, workflowID, stepID string, status memory.StepStatus, summary string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE workflow_steps SET status = ?, summary = ?, updated_at = ? WHERE workflow_id = ? AND step_id = ?`, string(status), summary, timeString(time.Now().UTC()), workflowID, stepID)
	return err
}

func (s *SQLiteWorkflowStateStore) InvalidateDependents(ctx context.Context, workflowID, sourceStepID, reason string) ([]memory.InvalidationRecord, error) {
	graph, err := s.dependencyGraph(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	visited := map[string]struct{}{}
	queue := []string{sourceStepID}
	var affected []string
	for len(queue) > 0 {
		step := queue[0]
		queue = queue[1:]
		for _, child := range graph[step] {
			if _, ok := visited[child]; ok {
				continue
			}
			visited[child] = struct{}{}
			affected = append(affected, child)
			queue = append(queue, child)
		}
	}
	sort.Strings(affected)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	out := make([]memory.InvalidationRecord, 0, len(affected))
	for _, stepID := range affected {
		record := memory.InvalidationRecord{
			InvalidationID:    newRecordID("inval"),
			WorkflowID:        workflowID,
			SourceStepID:      sourceStepID,
			InvalidatedStepID: stepID,
			Reason:            reason,
			CreatedAt:         now,
		}
		if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status = ?, updated_at = ? WHERE workflow_id = ? AND step_id = ?`, string(memory.StepStatusInvalidated), timeString(now), workflowID, stepID); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO workflow_invalidation (invalidation_id, workflow_id, source_step_id, invalidated_step_id, reason, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			record.InvalidationID, record.WorkflowID, record.SourceStepID, record.InvalidatedStepID, record.Reason, timeString(record.CreatedAt)); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLiteWorkflowStateStore) ListInvalidations(ctx context.Context, workflowID string) ([]memory.InvalidationRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT invalidation_id, workflow_id, source_step_id, invalidated_step_id, reason, created_at FROM workflow_invalidation WHERE workflow_id = ? ORDER BY created_at ASC`, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []memory.InvalidationRecord
	for rows.Next() {
		var record memory.InvalidationRecord
		var createdAt string
		if err := rows.Scan(&record.InvalidationID, &record.WorkflowID, &record.SourceStepID, &record.InvalidatedStepID, &record.Reason, &createdAt); err != nil {
			return nil, err
		}
		record.CreatedAt = parseTime(createdAt)
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *SQLiteWorkflowStateStore) CreateStepRun(ctx context.Context, run memory.StepRunRecord) error {
	if run.StepRunID == "" || run.WorkflowID == "" || run.RunID == "" || run.StepID == "" {
		return errors.New("step run requires ids")
	}
	if run.Status == "" {
		run.Status = memory.StepStatusPending
	}
	if run.Attempt <= 0 {
		run.Attempt = 1
	}
	run.StartedAt = ensureTime(run.StartedAt)
	var finishedAt any
	if run.FinishedAt != nil {
		finishedAt = timeString(*run.FinishedAt)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO step_runs (step_run_id, workflow_id, run_id, step_id, attempt, status, summary, result_json, verification_ok, error_text, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.StepRunID,
		run.WorkflowID,
		run.RunID,
		run.StepID,
		run.Attempt,
		string(run.Status),
		run.Summary,
		mustJSON(run.ResultData),
		boolInt(run.VerificationOK),
		run.ErrorText,
		timeString(run.StartedAt),
		finishedAt,
	)
	return err
}

func (s *SQLiteWorkflowStateStore) ListStepRuns(ctx context.Context, workflowID, stepID string) ([]memory.StepRunRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT step_run_id, workflow_id, run_id, step_id, attempt, status, summary, result_json, verification_ok, error_text, started_at, finished_at FROM step_runs WHERE workflow_id = ? AND step_id = ? ORDER BY attempt ASC`, workflowID, stepID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []memory.StepRunRecord
	for rows.Next() {
		record, err := scanStepRunRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *SQLiteWorkflowStateStore) UpsertArtifact(ctx context.Context, artifact memory.StepArtifactRecord) error {
	if artifact.ArtifactID == "" || artifact.WorkflowID == "" || artifact.StepRunID == "" {
		return errors.New("artifact ids required")
	}
	if artifact.StorageKind == "" {
		artifact.StorageKind = memory.ArtifactStorageInline
	}
	artifact.CreatedAt = ensureTime(artifact.CreatedAt)
	_, err := s.db.ExecContext(ctx, `INSERT INTO step_artifacts (artifact_id, workflow_id, step_run_id, kind, content_type, storage_kind, summary_text, summary_metadata_json, inline_raw_text, raw_ref, raw_size_bytes, compression_method, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(artifact_id) DO UPDATE SET
			kind = excluded.kind,
			content_type = excluded.content_type,
			storage_kind = excluded.storage_kind,
			summary_text = excluded.summary_text,
			summary_metadata_json = excluded.summary_metadata_json,
			inline_raw_text = excluded.inline_raw_text,
			raw_ref = excluded.raw_ref,
			raw_size_bytes = excluded.raw_size_bytes,
			compression_method = excluded.compression_method`,
		artifact.ArtifactID,
		artifact.WorkflowID,
		artifact.StepRunID,
		artifact.Kind,
		artifact.ContentType,
		string(artifact.StorageKind),
		artifact.SummaryText,
		mustJSON(artifact.SummaryMetadata),
		artifact.InlineRawText,
		artifact.RawRef,
		artifact.RawSizeBytes,
		artifact.CompressionMethod,
		timeString(artifact.CreatedAt),
	)
	if err != nil {
		return err
	}
	return s.indexStepArtifact(ctx, artifact)
}

func (s *SQLiteWorkflowStateStore) ListArtifacts(ctx context.Context, workflowID, stepRunID string) ([]memory.StepArtifactRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT artifact_id, workflow_id, step_run_id, kind, content_type, storage_kind, summary_text, summary_metadata_json, inline_raw_text, raw_ref, raw_size_bytes, compression_method, created_at FROM step_artifacts WHERE workflow_id = ? AND step_run_id = ? ORDER BY created_at ASC`, workflowID, stepRunID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []memory.StepArtifactRecord
	for rows.Next() {
		record, err := scanArtifactRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *SQLiteWorkflowStateStore) UpsertWorkflowArtifact(ctx context.Context, artifact memory.WorkflowArtifactRecord) error {
	if artifact.ArtifactID == "" || artifact.WorkflowID == "" {
		return errors.New("workflow artifact requires ids")
	}
	if artifact.StorageKind == "" {
		artifact.StorageKind = memory.ArtifactStorageInline
	}
	artifact.CreatedAt = ensureTime(artifact.CreatedAt)
	var runID any
	if strings.TrimSpace(artifact.RunID) != "" {
		runID = artifact.RunID
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO workflow_artifacts (artifact_id, workflow_id, run_id, kind, content_type, storage_kind, summary_text, summary_metadata_json, inline_raw_text, raw_ref, raw_size_bytes, compression_method, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(artifact_id) DO UPDATE SET
			run_id = excluded.run_id,
			kind = excluded.kind,
			content_type = excluded.content_type,
			storage_kind = excluded.storage_kind,
			summary_text = excluded.summary_text,
			summary_metadata_json = excluded.summary_metadata_json,
			inline_raw_text = excluded.inline_raw_text,
			raw_ref = excluded.raw_ref,
			raw_size_bytes = excluded.raw_size_bytes,
			compression_method = excluded.compression_method`,
		artifact.ArtifactID,
		artifact.WorkflowID,
		runID,
		artifact.Kind,
		artifact.ContentType,
		string(artifact.StorageKind),
		artifact.SummaryText,
		mustJSON(artifact.SummaryMetadata),
		artifact.InlineRawText,
		artifact.RawRef,
		artifact.RawSizeBytes,
		artifact.CompressionMethod,
		timeString(artifact.CreatedAt),
	)
	if err != nil {
		return err
	}
	return s.indexWorkflowArtifact(ctx, artifact)
}

func (s *SQLiteWorkflowStateStore) ListWorkflowArtifacts(ctx context.Context, workflowID, runID string) ([]memory.WorkflowArtifactRecord, error) {
	query := `SELECT artifact_id, workflow_id, run_id, kind, content_type, storage_kind, summary_text, summary_metadata_json, inline_raw_text, raw_ref, raw_size_bytes, compression_method, created_at FROM workflow_artifacts WHERE workflow_id = ?`
	args := []any{workflowID}
	if strings.TrimSpace(runID) != "" {
		query += ` AND run_id = ?`
		args = append(args, runID)
	}
	query += ` ORDER BY created_at ASC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []memory.WorkflowArtifactRecord
	for rows.Next() {
		record, err := scanWorkflowArtifactRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *SQLiteWorkflowStateStore) ListWorkflowArtifactsByKind(ctx context.Context, workflowID, runID, kind string) ([]memory.WorkflowArtifactRecord, error) {
	query := `SELECT artifact_id, workflow_id, run_id, kind, content_type, storage_kind, summary_text, summary_metadata_json, inline_raw_text, raw_ref, raw_size_bytes, compression_method, created_at
		FROM workflow_artifacts WHERE workflow_id = ? AND kind = ?`
	args := []any{workflowID, kind}
	if strings.TrimSpace(runID) != "" {
		query += ` AND run_id = ?`
		args = append(args, runID)
	}
	query += ` ORDER BY created_at ASC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []memory.WorkflowArtifactRecord
	for rows.Next() {
		record, err := scanWorkflowArtifactRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *SQLiteWorkflowStateStore) ListWorkflowArtifactsByKindAndWorkspace(ctx context.Context, workflowID, runID, kind, workspaceID string) ([]memory.WorkflowArtifactRecord, error) {
	query := `SELECT artifact_id, workflow_id, run_id, kind, content_type, storage_kind, summary_text, summary_metadata_json, inline_raw_text, raw_ref, raw_size_bytes, compression_method, created_at
		FROM workflow_artifacts WHERE kind = ? AND json_extract(summary_metadata_json, '$.workspace_id') = ?`
	args := []any{kind, workspaceID}
	if strings.TrimSpace(workflowID) != "" {
		query += ` AND workflow_id = ?`
		args = append(args, workflowID)
	}
	if strings.TrimSpace(runID) != "" {
		query += ` AND run_id = ?`
		args = append(args, runID)
	}
	query += ` ORDER BY created_at ASC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []memory.WorkflowArtifactRecord
	for rows.Next() {
		record, err := scanWorkflowArtifactRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *SQLiteWorkflowStateStore) LatestWorkflowArtifactByKind(ctx context.Context, workflowID, runID, kind string) (*memory.WorkflowArtifactRecord, bool, error) {
	query := `SELECT artifact_id, workflow_id, run_id, kind, content_type, storage_kind, summary_text, summary_metadata_json, inline_raw_text, raw_ref, raw_size_bytes, compression_method, created_at
		FROM workflow_artifacts WHERE workflow_id = ? AND kind = ?`
	args := []any{workflowID, kind}
	if strings.TrimSpace(runID) != "" {
		query += ` AND run_id = ?`
		args = append(args, runID)
	}
	query += ` ORDER BY created_at DESC LIMIT 1`
	row := s.db.QueryRowContext(ctx, query, args...)
	record, err := scanWorkflowArtifactRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return record, true, nil
}

func (s *SQLiteWorkflowStateStore) LatestWorkflowArtifactByKindAndWorkspace(ctx context.Context, workflowID, runID, kind, workspaceID string) (*memory.WorkflowArtifactRecord, bool, error) {
	query := `SELECT artifact_id, workflow_id, run_id, kind, content_type, storage_kind, summary_text, summary_metadata_json, inline_raw_text, raw_ref, raw_size_bytes, compression_method, created_at
		FROM workflow_artifacts WHERE kind = ? AND json_extract(summary_metadata_json, '$.workspace_id') = ?`
	args := []any{kind, workspaceID}
	if strings.TrimSpace(workflowID) != "" {
		query += ` AND workflow_id = ?`
		args = append(args, workflowID)
	}
	if strings.TrimSpace(runID) != "" {
		query += ` AND run_id = ?`
		args = append(args, runID)
	}
	query += ` ORDER BY created_at DESC LIMIT 1`
	row := s.db.QueryRowContext(ctx, query, args...)
	record, err := scanWorkflowArtifactRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return record, true, nil
}

func (s *SQLiteWorkflowStateStore) WorkflowArtifactByID(ctx context.Context, artifactID string) (*memory.WorkflowArtifactRecord, bool, error) {
	artifactID = strings.TrimSpace(artifactID)
	if artifactID == "" {
		return nil, false, nil
	}
	row := s.db.QueryRowContext(ctx, `SELECT artifact_id, workflow_id, run_id, kind, content_type, storage_kind, summary_text, summary_metadata_json, inline_raw_text, raw_ref, raw_size_bytes, compression_method, created_at
		FROM workflow_artifacts WHERE artifact_id = ?`, artifactID)
	record, err := scanWorkflowArtifactRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return record, true, nil
}

func (s *SQLiteWorkflowStateStore) UpsertLineageBinding(ctx context.Context, record LineageBindingRecord) error {
	if s == nil || s.db == nil {
		return errors.New("workflow state db unavailable")
	}
	if strings.TrimSpace(record.WorkflowID) == "" || strings.TrimSpace(record.RunID) == "" || strings.TrimSpace(record.LineageID) == "" || strings.TrimSpace(record.AttemptID) == "" {
		return errors.New("lineage binding requires workflow, run, lineage, and attempt ids")
	}
	record.UpdatedAt = ensureTime(record.UpdatedAt)
	return upsertLineageBindingTx(ctx, s.db, record)
}

func (s *SQLiteWorkflowStateStore) GetLineageBinding(ctx context.Context, workflowID, runID string) (*LineageBindingRecord, bool, error) {
	if s == nil || s.db == nil {
		return nil, false, errors.New("workflow state db unavailable")
	}
	row := s.db.QueryRowContext(ctx, `SELECT workflow_id, run_id, lineage_id, attempt_id, runtime_id, session_id, state, updated_at
		FROM rex_fmp_lineage_bindings WHERE workflow_id = ? AND run_id = ?`, workflowID, runID)
	record, err := scanLineageBindingRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return record, true, nil
}

func (s *SQLiteWorkflowStateStore) FindLineageBindingsByLineageID(ctx context.Context, lineageID string) ([]LineageBindingRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("workflow state db unavailable")
	}
	lineageID = strings.TrimSpace(lineageID)
	if lineageID == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT workflow_id, run_id, lineage_id, attempt_id, runtime_id, session_id, state, updated_at
		FROM rex_fmp_lineage_bindings WHERE lineage_id = ? ORDER BY updated_at ASC`, lineageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LineageBindingRecord
	for rows.Next() {
		record, err := scanLineageBindingRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *SQLiteWorkflowStateStore) FindLineageBindingsByAttemptID(ctx context.Context, attemptID string) ([]LineageBindingRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("workflow state db unavailable")
	}
	attemptID = strings.TrimSpace(attemptID)
	if attemptID == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT workflow_id, run_id, lineage_id, attempt_id, runtime_id, session_id, state, updated_at
		FROM rex_fmp_lineage_bindings WHERE attempt_id = ? ORDER BY updated_at ASC`, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LineageBindingRecord
	for rows.Next() {
		record, err := scanLineageBindingRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *SQLiteWorkflowStateStore) SaveStageResult(ctx context.Context, record memory.WorkflowStageResultRecord) error {
	if record.ResultID == "" || record.WorkflowID == "" || record.RunID == "" || record.StageName == "" {
		return errors.New("stage result requires ids and stage name")
	}
	record.StartedAt = ensureTime(record.StartedAt)
	record.FinishedAt = ensureTime(record.FinishedAt)
	_, err := s.db.ExecContext(ctx, `INSERT INTO workflow_stage_results (
		result_id, workflow_id, run_id, stage_name, stage_index, contract_name, contract_version, prompt_text,
		response_json, decoded_output_json, validation_ok, error_text, retry_attempt, transition_kind, next_stage,
		transition_reason, started_at, finished_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ResultID,
		record.WorkflowID,
		record.RunID,
		record.StageName,
		record.StageIndex,
		record.ContractName,
		record.ContractVersion,
		record.PromptText,
		record.ResponseJSON,
		mustJSONAny(record.DecodedOutput),
		boolInt(record.ValidationOK),
		record.ErrorText,
		record.RetryAttempt,
		record.TransitionKind,
		record.NextStage,
		record.TransitionReason,
		timeString(record.StartedAt),
		timeString(record.FinishedAt),
	)
	return err
}

func (s *SQLiteWorkflowStateStore) ListStageResults(ctx context.Context, workflowID, runID string) ([]memory.WorkflowStageResultRecord, error) {
	query := `SELECT result_id, workflow_id, run_id, stage_name, stage_index, contract_name, contract_version, prompt_text,
		response_json, decoded_output_json, validation_ok, error_text, retry_attempt, transition_kind, next_stage,
		transition_reason, started_at, finished_at
		FROM workflow_stage_results WHERE workflow_id = ?`
	args := []any{workflowID}
	if strings.TrimSpace(runID) != "" {
		query += ` AND run_id = ?`
		args = append(args, runID)
	}
	query += ` ORDER BY stage_index ASC, retry_attempt ASC, finished_at ASC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []memory.WorkflowStageResultRecord
	for rows.Next() {
		record, err := scanWorkflowStageResultRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *SQLiteWorkflowStateStore) GetLatestValidStageResult(ctx context.Context, workflowID, runID, stageName string) (*memory.WorkflowStageResultRecord, bool, error) {
	if strings.TrimSpace(workflowID) == "" || strings.TrimSpace(runID) == "" || strings.TrimSpace(stageName) == "" {
		return nil, false, errors.New("workflow id, run id, and stage name required")
	}
	row := s.db.QueryRowContext(ctx, `SELECT result_id, workflow_id, run_id, stage_name, stage_index, contract_name, contract_version, prompt_text,
		response_json, decoded_output_json, validation_ok, error_text, retry_attempt, transition_kind, next_stage,
		transition_reason, started_at, finished_at
		FROM workflow_stage_results
		WHERE workflow_id = ? AND run_id = ? AND stage_name = ? AND validation_ok = 1
		ORDER BY retry_attempt DESC, finished_at DESC
		LIMIT 1`, workflowID, runID, stageName)
	record, ok, err := scanWorkflowStageResult(row)
	if err != nil || !ok {
		return nil, ok, err
	}
	return record, true, nil
}

func (s *SQLiteWorkflowStateStore) SavePipelineCheckpoint(ctx context.Context, record memory.PipelineCheckpointRecord) error {
	if strings.TrimSpace(record.CheckpointID) == "" || strings.TrimSpace(record.TaskID) == "" {
		return errors.New("pipeline checkpoint requires checkpoint and task ids")
	}
	record.CreatedAt = ensureTime(record.CreatedAt)
	_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO pipeline_checkpoints (
		checkpoint_id, task_id, workflow_id, run_id, stage_name, stage_index, context_json, result_json, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.CheckpointID,
		record.TaskID,
		record.WorkflowID,
		record.RunID,
		record.StageName,
		record.StageIndex,
		record.ContextJSON,
		record.ResultJSON,
		timeString(record.CreatedAt),
	)
	return err
}

func (s *SQLiteWorkflowStateStore) LoadPipelineCheckpoint(ctx context.Context, taskID, checkpointID string) (*memory.PipelineCheckpointRecord, bool, error) {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(checkpointID) == "" {
		return nil, false, errors.New("task id and checkpoint id required")
	}
	row := s.db.QueryRowContext(ctx, `SELECT checkpoint_id, task_id, workflow_id, run_id, stage_name, stage_index, context_json, result_json, created_at
		FROM pipeline_checkpoints WHERE task_id = ? AND checkpoint_id = ?`, taskID, checkpointID)
	var record memory.PipelineCheckpointRecord
	var createdAt string
	if err := row.Scan(
		&record.CheckpointID,
		&record.TaskID,
		&record.WorkflowID,
		&record.RunID,
		&record.StageName,
		&record.StageIndex,
		&record.ContextJSON,
		&record.ResultJSON,
		&createdAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	record.CreatedAt = parseTime(createdAt)
	return &record, true, nil
}

func (s *SQLiteWorkflowStateStore) ListPipelineCheckpoints(ctx context.Context, taskID string) ([]string, error) {
	if strings.TrimSpace(taskID) == "" {
		return nil, errors.New("task id required")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT checkpoint_id FROM pipeline_checkpoints WHERE task_id = ? ORDER BY created_at DESC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var checkpointID string
		if err := rows.Scan(&checkpointID); err != nil {
			return nil, err
		}
		ids = append(ids, checkpointID)
	}
	return ids, rows.Err()
}

func (s *SQLiteWorkflowStateStore) PutKnowledge(ctx context.Context, record memory.KnowledgeRecord) error {
	if record.RecordID == "" || record.WorkflowID == "" || record.Kind == "" {
		return errors.New("knowledge record requires ids and kind")
	}
	record.CreatedAt = ensureTime(record.CreatedAt)
	_, err := s.db.ExecContext(ctx, `INSERT INTO workflow_knowledge (record_id, workflow_id, step_run_id, step_id, kind, title, content, status, metadata_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(record_id) DO UPDATE SET
			title = excluded.title,
			content = excluded.content,
			status = excluded.status,
			metadata_json = excluded.metadata_json`,
		record.RecordID,
		record.WorkflowID,
		record.StepRunID,
		record.StepID,
		string(record.Kind),
		record.Title,
		record.Content,
		record.Status,
		mustJSON(record.Metadata),
		timeString(record.CreatedAt),
	)
	if err != nil {
		return err
	}
	return s.indexKnowledgeRecord(ctx, record)
}

func (s *SQLiteWorkflowStateStore) ListKnowledge(ctx context.Context, workflowID string, kind memory.KnowledgeKind, unresolvedOnly bool) ([]memory.KnowledgeRecord, error) {
	query := `SELECT record_id, workflow_id, step_run_id, step_id, kind, title, content, status, metadata_json, created_at FROM workflow_knowledge WHERE workflow_id = ?`
	args := []any{workflowID}
	if kind != "" {
		query += ` AND kind = ?`
		args = append(args, string(kind))
	}
	if unresolvedOnly {
		query += ` AND (status = '' OR status = 'open' OR status = 'unresolved')`
	}
	query += ` ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []memory.KnowledgeRecord
	for rows.Next() {
		record, err := scanKnowledgeRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *SQLiteWorkflowStateStore) AppendEvent(ctx context.Context, event memory.WorkflowEventRecord) error {
	if event.EventID == "" || event.WorkflowID == "" || event.EventType == "" {
		return errors.New("event requires ids and type")
	}
	event.CreatedAt = ensureTime(event.CreatedAt)
	_, err := s.db.ExecContext(ctx, `INSERT INTO workflow_events (event_id, workflow_id, run_id, step_id, event_type, message, metadata_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		event.EventID,
		event.WorkflowID,
		event.RunID,
		event.StepID,
		event.EventType,
		event.Message,
		mustJSON(event.Metadata),
		timeString(event.CreatedAt),
	)
	return err
}

func (s *SQLiteWorkflowStateStore) ListEvents(ctx context.Context, workflowID string, limit int) ([]memory.WorkflowEventRecord, error) {
	query := `SELECT event_id, workflow_id, run_id, step_id, event_type, message, metadata_json, created_at FROM workflow_events WHERE workflow_id = ? ORDER BY created_at DESC`
	args := []any{workflowID}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []memory.WorkflowEventRecord
	for rows.Next() {
		record, err := scanEventRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *SQLiteWorkflowStateStore) LatestEvent(ctx context.Context, workflowID string) (*memory.WorkflowEventRecord, bool, error) {
	query := `SELECT event_id, workflow_id, run_id, step_id, event_type, message, metadata_json, created_at
		FROM workflow_events WHERE workflow_id = ?
		ORDER BY rowid DESC LIMIT 1`
	row := s.db.QueryRowContext(ctx, query, workflowID)
	record, err := scanEventRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return record, true, nil
}

func (s *SQLiteWorkflowStateStore) LatestEventByTypes(ctx context.Context, workflowID string, eventTypes ...string) (*memory.WorkflowEventRecord, bool, error) {
	workflowID = strings.TrimSpace(workflowID)
	if workflowID == "" || len(eventTypes) == 0 {
		return nil, false, nil
	}
	placeholders := make([]string, 0, len(eventTypes))
	args := make([]any, 0, len(eventTypes)+1)
	args = append(args, workflowID)
	for _, eventType := range eventTypes {
		eventType = strings.TrimSpace(eventType)
		if eventType == "" {
			continue
		}
		placeholders = append(placeholders, "?")
		args = append(args, eventType)
	}
	if len(placeholders) == 0 {
		return nil, false, nil
	}
	query := fmt.Sprintf(`SELECT event_id, workflow_id, run_id, step_id, event_type, message, metadata_json, created_at
		FROM workflow_events WHERE workflow_id = ? AND event_type IN (%s)
		ORDER BY rowid DESC LIMIT 1`, strings.Join(placeholders, ","))
	row := s.db.QueryRowContext(ctx, query, args...)
	record, err := scanEventRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return record, true, nil
}

func (s *SQLiteWorkflowStateStore) ReplaceProviderSnapshots(ctx context.Context, workflowID, runID string, snapshots []memory.WorkflowProviderSnapshotRecord) error {
	if strings.TrimSpace(workflowID) == "" {
		return errors.New("workflow id required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM workflow_provider_snapshots WHERE workflow_id = ? AND run_id = ?`, workflowID, runID); err != nil {
		return err
	}
	for _, snapshot := range snapshots {
		if strings.TrimSpace(snapshot.SnapshotID) == "" || strings.TrimSpace(snapshot.ProviderID) == "" {
			return errors.New("provider snapshot requires ids")
		}
		snapshot.CapturedAt = ensureTime(snapshot.CapturedAt)
		if snapshot.Descriptor.ID == "" {
			snapshot.Descriptor.ID = snapshot.ProviderID
		}
		if snapshot.Descriptor.RecoverabilityMode == "" {
			snapshot.Descriptor.RecoverabilityMode = snapshot.Recoverability
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO workflow_provider_snapshots (snapshot_id, workflow_id, run_id, provider_id, recoverability, descriptor_json, health_json, capability_ids_json, task_id, metadata_json, state_json, captured_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			snapshot.SnapshotID,
			workflowID,
			runID,
			snapshot.ProviderID,
			string(snapshot.Recoverability),
			mustJSON(snapshot.Descriptor),
			mustJSON(snapshot.Health),
			mustJSON(snapshot.CapabilityIDs),
			snapshot.TaskID,
			mustJSON(snapshot.Metadata),
			mustJSONAny(snapshot.State),
			timeString(snapshot.CapturedAt),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteWorkflowStateStore) ListProviderSnapshots(ctx context.Context, workflowID, runID string) ([]memory.WorkflowProviderSnapshotRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT snapshot_id, workflow_id, run_id, provider_id, recoverability, descriptor_json, health_json, capability_ids_json, task_id, metadata_json, state_json, captured_at
		FROM workflow_provider_snapshots WHERE workflow_id = ? AND run_id = ? ORDER BY captured_at ASC, snapshot_id ASC`, workflowID, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []memory.WorkflowProviderSnapshotRecord
	for rows.Next() {
		record, err := scanProviderSnapshotRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *SQLiteWorkflowStateStore) ReplaceProviderSessionSnapshots(ctx context.Context, workflowID, runID string, snapshots []memory.WorkflowProviderSessionSnapshotRecord) error {
	if strings.TrimSpace(workflowID) == "" {
		return errors.New("workflow id required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM workflow_provider_session_snapshots WHERE workflow_id = ? AND run_id = ?`, workflowID, runID); err != nil {
		return err
	}
	for _, snapshot := range snapshots {
		if strings.TrimSpace(snapshot.SnapshotID) == "" || strings.TrimSpace(snapshot.Session.ID) == "" || strings.TrimSpace(snapshot.Session.ProviderID) == "" {
			return errors.New("provider session snapshot requires ids")
		}
		snapshot.CapturedAt = ensureTime(snapshot.CapturedAt)
		if _, err := tx.ExecContext(ctx, `INSERT INTO workflow_provider_session_snapshots (snapshot_id, workflow_id, run_id, session_id, provider_id, session_json, metadata_json, state_json, captured_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			snapshot.SnapshotID,
			workflowID,
			runID,
			snapshot.Session.ID,
			snapshot.Session.ProviderID,
			mustJSON(snapshot.Session),
			mustJSON(snapshot.Metadata),
			mustJSONAny(snapshot.State),
			timeString(snapshot.CapturedAt),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteWorkflowStateStore) ListProviderSessionSnapshots(ctx context.Context, workflowID, runID string) ([]memory.WorkflowProviderSessionSnapshotRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT snapshot_id, workflow_id, run_id, session_json, metadata_json, state_json, captured_at
		FROM workflow_provider_session_snapshots WHERE workflow_id = ? AND run_id = ? ORDER BY captured_at ASC, snapshot_id ASC`, workflowID, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []memory.WorkflowProviderSessionSnapshotRecord
	for rows.Next() {
		record, err := scanProviderSessionSnapshotRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *SQLiteWorkflowStateStore) UpsertDelegation(ctx context.Context, record memory.WorkflowDelegationRecord) error {
	if strings.TrimSpace(record.DelegationID) == "" || strings.TrimSpace(record.WorkflowID) == "" {
		return errors.New("delegation record requires ids")
	}
	record.StartedAt = ensureTime(record.StartedAt)
	record.UpdatedAt = ensureTime(record.UpdatedAt)
	if strings.TrimSpace(record.TaskID) == "" {
		record.TaskID = strings.TrimSpace(record.Request.TaskID)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO workflow_delegations (delegation_id, workflow_id, run_id, task_id, state, trust_class, recoverability, background, request_json, result_json, metadata_json, started_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(delegation_id) DO UPDATE SET
			workflow_id = excluded.workflow_id,
			run_id = excluded.run_id,
			task_id = excluded.task_id,
			state = excluded.state,
			trust_class = excluded.trust_class,
			recoverability = excluded.recoverability,
			background = excluded.background,
			request_json = excluded.request_json,
			result_json = excluded.result_json,
			metadata_json = excluded.metadata_json,
			started_at = excluded.started_at,
			updated_at = excluded.updated_at`,
		record.DelegationID,
		record.WorkflowID,
		record.RunID,
		record.TaskID,
		string(record.State),
		string(record.TrustClass),
		string(record.Recoverability),
		boolInt(record.Background),
		mustJSONAny(record.Request),
		mustJSONAny(record.Result),
		mustJSON(record.Metadata),
		timeString(record.StartedAt),
		timeString(record.UpdatedAt),
	)
	return err
}

func (s *SQLiteWorkflowStateStore) ListDelegations(ctx context.Context, workflowID, runID string) ([]memory.WorkflowDelegationRecord, error) {
	query := `SELECT delegation_id, workflow_id, run_id, task_id, state, trust_class, recoverability, background, request_json, result_json, metadata_json, started_at, updated_at
		FROM workflow_delegations WHERE workflow_id = ?`
	args := []any{workflowID}
	if strings.TrimSpace(runID) != "" {
		query += ` AND run_id = ?`
		args = append(args, runID)
	}
	query += ` ORDER BY updated_at ASC, delegation_id ASC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []memory.WorkflowDelegationRecord
	for rows.Next() {
		record, err := scanDelegationRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *SQLiteWorkflowStateStore) AppendDelegationTransition(ctx context.Context, record memory.WorkflowDelegationTransitionRecord) error {
	if strings.TrimSpace(record.TransitionID) == "" || strings.TrimSpace(record.DelegationID) == "" || strings.TrimSpace(record.WorkflowID) == "" {
		return errors.New("delegation transition requires ids")
	}
	record.CreatedAt = ensureTime(record.CreatedAt)
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO workflow_delegation_transitions (transition_id, delegation_id, workflow_id, run_id, from_state, to_state, metadata_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		record.TransitionID,
		record.DelegationID,
		record.WorkflowID,
		record.RunID,
		string(record.FromState),
		string(record.ToState),
		mustJSON(record.Metadata),
		timeString(record.CreatedAt),
	)
	return err
}

func (s *SQLiteWorkflowStateStore) ListDelegationTransitions(ctx context.Context, delegationID string) ([]memory.WorkflowDelegationTransitionRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT transition_id, delegation_id, workflow_id, run_id, from_state, to_state, metadata_json, created_at
		FROM workflow_delegation_transitions WHERE delegation_id = ? ORDER BY created_at ASC, transition_id ASC`, delegationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []memory.WorkflowDelegationTransitionRecord
	for rows.Next() {
		record, err := scanDelegationTransitionRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *SQLiteWorkflowStateStore) LoadStepSlice(ctx context.Context, workflowID, stepID string, eventLimit int) (*memory.WorkflowStepSlice, bool, error) {
	workflow, ok, err := s.GetWorkflow(ctx, workflowID)
	if err != nil || !ok {
		return nil, ok, err
	}
	steps, err := s.ListSteps(ctx, workflowID)
	if err != nil {
		return nil, false, err
	}
	stepMap := make(map[string]memory.WorkflowStepRecord, len(steps))
	for _, step := range steps {
		stepMap[step.StepID] = step
	}
	current, ok := stepMap[stepID]
	if !ok {
		return nil, false, nil
	}
	dependencySteps := make([]memory.WorkflowStepRecord, 0, len(current.Dependencies))
	dependencyRuns := make([]memory.StepRunRecord, 0, len(current.Dependencies))
	artifacts := make([]memory.StepArtifactRecord, 0)
	for _, depID := range current.Dependencies {
		dep, ok := stepMap[depID]
		if !ok {
			continue
		}
		dependencySteps = append(dependencySteps, dep)
		runs, err := s.ListStepRuns(ctx, workflowID, depID)
		if err != nil {
			return nil, false, err
		}
		if len(runs) == 0 {
			continue
		}
		latest := runs[len(runs)-1]
		dependencyRuns = append(dependencyRuns, latest)
		runArtifacts, err := s.ListArtifacts(ctx, workflowID, latest.StepRunID)
		if err != nil {
			return nil, false, err
		}
		artifacts = append(artifacts, runArtifacts...)
	}
	facts, err := s.ListKnowledge(ctx, workflowID, memory.KnowledgeKindFact, false)
	if err != nil {
		return nil, false, err
	}
	issues, err := s.ListKnowledge(ctx, workflowID, memory.KnowledgeKindIssue, true)
	if err != nil {
		return nil, false, err
	}
	decisions, err := s.ListKnowledge(ctx, workflowID, memory.KnowledgeKindDecision, false)
	if err != nil {
		return nil, false, err
	}
	events, err := s.ListEvents(ctx, workflowID, eventLimit)
	if err != nil {
		return nil, false, err
	}
	return &memory.WorkflowStepSlice{
		Workflow:        *workflow,
		Step:            current,
		DependencySteps: dependencySteps,
		DependencyRuns:  dependencyRuns,
		Artifacts:       artifacts,
		Facts:           filterKnowledgeBySteps(facts, current.Dependencies),
		Issues:          filterKnowledgeBySteps(issues, current.Dependencies),
		Decisions:       filterKnowledgeBySteps(decisions, current.Dependencies),
		RecentEvents:    events,
	}, true, nil
}

func (s *SQLiteWorkflowStateStore) stepDependencies(ctx context.Context, workflowID, stepID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT dependency_step_id FROM workflow_step_dependencies WHERE workflow_id = ? AND step_id = ? ORDER BY dependency_step_id ASC`, workflowID, stepID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var deps []string
	for rows.Next() {
		var dep string
		if err := rows.Scan(&dep); err != nil {
			return nil, err
		}
		deps = append(deps, dep)
	}
	return deps, rows.Err()
}

func (s *SQLiteWorkflowStateStore) dependencyGraph(ctx context.Context, workflowID string) (map[string][]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT step_id, dependency_step_id FROM workflow_step_dependencies WHERE workflow_id = ?`, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	graph := map[string][]string{}
	for rows.Next() {
		var stepID, depID string
		if err := rows.Scan(&stepID, &depID); err != nil {
			return nil, err
		}
		graph[depID] = append(graph[depID], stepID)
	}
	return graph, rows.Err()
}

func scanWorkflow(row interface{ Scan(dest ...any) error }) (*memory.WorkflowRecord, bool, error) {
	record, err := scanWorkflowCommon(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return record, true, nil
}

func scanWorkflowRows(rows *sql.Rows) (*memory.WorkflowRecord, error) {
	return scanWorkflowCommon(rows.Scan)
}

func scanWorkflowCommon(scan func(dest ...any) error) (*memory.WorkflowRecord, error) {
	var record memory.WorkflowRecord
	var metadataJSON string
	var createdAt string
	var updatedAt string
	err := scan(&record.WorkflowID, &record.TaskID, &record.TaskType, &record.Instruction, &record.Status, &record.CursorStepID, &record.Version, &metadataJSON, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	record.Metadata = decodeJSONMap(metadataJSON)
	record.CreatedAt = parseTime(createdAt)
	record.UpdatedAt = parseTime(updatedAt)
	return &record, nil
}

func scanStepRows(rows *sql.Rows) (*memory.WorkflowStepRecord, error) {
	var record memory.WorkflowStepRecord
	var stepJSON string
	var updatedAt string
	err := rows.Scan(&record.WorkflowID, &record.PlanID, &record.StepID, &record.Ordinal, &stepJSON, &record.Status, &record.Summary, &updatedAt)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(stepJSON), &record.Step); err != nil {
		return nil, err
	}
	record.UpdatedAt = parseTime(updatedAt)
	return &record, nil
}

func scanStepRunRows(rows *sql.Rows) (*memory.StepRunRecord, error) {
	var record memory.StepRunRecord
	var resultJSON string
	var startedAt string
	var finishedAt sql.NullString
	var verificationOK int
	err := rows.Scan(&record.StepRunID, &record.WorkflowID, &record.RunID, &record.StepID, &record.Attempt, &record.Status, &record.Summary, &resultJSON, &verificationOK, &record.ErrorText, &startedAt, &finishedAt)
	if err != nil {
		return nil, err
	}
	record.VerificationOK = verificationOK == 1
	record.ResultData = decodeJSONMap(resultJSON)
	record.StartedAt = parseTime(startedAt)
	if finishedAt.Valid {
		t := parseTime(finishedAt.String)
		record.FinishedAt = &t
	}
	return &record, nil
}

func scanArtifactRows(rows *sql.Rows) (*memory.StepArtifactRecord, error) {
	var record memory.StepArtifactRecord
	var metadataJSON string
	var createdAt string
	err := rows.Scan(&record.ArtifactID, &record.WorkflowID, &record.StepRunID, &record.Kind, &record.ContentType, &record.StorageKind, &record.SummaryText, &metadataJSON, &record.InlineRawText, &record.RawRef, &record.RawSizeBytes, &record.CompressionMethod, &createdAt)
	if err != nil {
		return nil, err
	}
	record.SummaryMetadata = decodeJSONMap(metadataJSON)
	record.CreatedAt = parseTime(createdAt)
	return &record, nil
}

func scanWorkflowArtifactRows(rows *sql.Rows) (*memory.WorkflowArtifactRecord, error) {
	var record memory.WorkflowArtifactRecord
	var metadataJSON string
	var createdAt string
	var runID sql.NullString
	err := rows.Scan(&record.ArtifactID, &record.WorkflowID, &runID, &record.Kind, &record.ContentType, &record.StorageKind, &record.SummaryText, &metadataJSON, &record.InlineRawText, &record.RawRef, &record.RawSizeBytes, &record.CompressionMethod, &createdAt)
	if err != nil {
		return nil, err
	}
	if runID.Valid {
		record.RunID = runID.String
	}
	record.SummaryMetadata = decodeJSONMap(metadataJSON)
	record.CreatedAt = parseTime(createdAt)
	return &record, nil
}

func scanWorkflowArtifactRow(row *sql.Row) (*memory.WorkflowArtifactRecord, error) {
	var record memory.WorkflowArtifactRecord
	var runID sql.NullString
	var summaryMetadataJSON string
	var createdAt string
	err := row.Scan(&record.ArtifactID, &record.WorkflowID, &runID, &record.Kind, &record.ContentType, &record.StorageKind, &record.SummaryText, &summaryMetadataJSON, &record.InlineRawText, &record.RawRef, &record.RawSizeBytes, &record.CompressionMethod, &createdAt)
	if err != nil {
		return nil, err
	}
	record.RunID = runID.String
	record.SummaryMetadata = decodeJSONMap(summaryMetadataJSON)
	record.CreatedAt = parseTime(createdAt)
	return &record, nil
}

func scanWorkflowStageResult(row interface{ Scan(dest ...any) error }) (*memory.WorkflowStageResultRecord, bool, error) {
	record, err := scanWorkflowStageResultCommon(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return record, true, nil
}

func scanWorkflowStageResultRows(rows *sql.Rows) (*memory.WorkflowStageResultRecord, error) {
	return scanWorkflowStageResultCommon(rows.Scan)
}

func scanWorkflowStageResultCommon(scan func(dest ...any) error) (*memory.WorkflowStageResultRecord, error) {
	var record memory.WorkflowStageResultRecord
	var decodedJSON string
	var startedAt string
	var finishedAt string
	var validationOK int
	err := scan(
		&record.ResultID,
		&record.WorkflowID,
		&record.RunID,
		&record.StageName,
		&record.StageIndex,
		&record.ContractName,
		&record.ContractVersion,
		&record.PromptText,
		&record.ResponseJSON,
		&decodedJSON,
		&validationOK,
		&record.ErrorText,
		&record.RetryAttempt,
		&record.TransitionKind,
		&record.NextStage,
		&record.TransitionReason,
		&startedAt,
		&finishedAt,
	)
	if err != nil {
		return nil, err
	}
	record.DecodedOutput = decodeJSONAny(decodedJSON)
	record.ValidationOK = validationOK == 1
	record.StartedAt = parseTime(startedAt)
	record.FinishedAt = parseTime(finishedAt)
	return &record, nil
}

func scanKnowledgeRows(rows *sql.Rows) (*memory.KnowledgeRecord, error) {
	var record memory.KnowledgeRecord
	var metadataJSON string
	var createdAt string
	err := rows.Scan(&record.RecordID, &record.WorkflowID, &record.StepRunID, &record.StepID, &record.Kind, &record.Title, &record.Content, &record.Status, &metadataJSON, &createdAt)
	if err != nil {
		return nil, err
	}
	record.Metadata = decodeJSONMap(metadataJSON)
	record.CreatedAt = parseTime(createdAt)
	return &record, nil
}

func scanEventRows(rows *sql.Rows) (*memory.WorkflowEventRecord, error) {
	var record memory.WorkflowEventRecord
	var metadataJSON string
	var createdAt string
	err := rows.Scan(&record.EventID, &record.WorkflowID, &record.RunID, &record.StepID, &record.EventType, &record.Message, &metadataJSON, &createdAt)
	if err != nil {
		return nil, err
	}
	record.Metadata = decodeJSONMap(metadataJSON)
	record.CreatedAt = parseTime(createdAt)
	return &record, nil
}

func scanEventRow(row *sql.Row) (*memory.WorkflowEventRecord, error) {
	var record memory.WorkflowEventRecord
	var metadataJSON string
	var createdAt string
	err := row.Scan(&record.EventID, &record.WorkflowID, &record.RunID, &record.StepID, &record.EventType, &record.Message, &metadataJSON, &createdAt)
	if err != nil {
		return nil, err
	}
	record.Metadata = decodeJSONMap(metadataJSON)
	record.CreatedAt = parseTime(createdAt)
	return &record, nil
}

func scanProviderSnapshotRows(rows *sql.Rows) (*memory.WorkflowProviderSnapshotRecord, error) {
	var record memory.WorkflowProviderSnapshotRecord
	var descriptorJSON string
	var healthJSON string
	var capabilityIDsJSON string
	var metadataJSON string
	var stateJSON string
	var capturedAt string
	err := rows.Scan(
		&record.SnapshotID,
		&record.WorkflowID,
		&record.RunID,
		&record.ProviderID,
		&record.Recoverability,
		&descriptorJSON,
		&healthJSON,
		&capabilityIDsJSON,
		&record.TaskID,
		&metadataJSON,
		&stateJSON,
		&capturedAt,
	)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(descriptorJSON), &record.Descriptor)
	_ = json.Unmarshal([]byte(healthJSON), &record.Health)
	record.CapabilityIDs = decodeJSONStringSlice(capabilityIDsJSON)
	record.Metadata = decodeJSONMap(metadataJSON)
	record.State = decodeJSONAny(stateJSON)
	record.CapturedAt = parseTime(capturedAt)
	return &record, nil
}

func scanProviderSessionSnapshotRows(rows *sql.Rows) (*memory.WorkflowProviderSessionSnapshotRecord, error) {
	var record memory.WorkflowProviderSessionSnapshotRecord
	var sessionJSON string
	var metadataJSON string
	var stateJSON string
	var capturedAt string
	err := rows.Scan(
		&record.SnapshotID,
		&record.WorkflowID,
		&record.RunID,
		&sessionJSON,
		&metadataJSON,
		&stateJSON,
		&capturedAt,
	)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(sessionJSON), &record.Session)
	record.Metadata = decodeJSONMap(metadataJSON)
	record.State = decodeJSONAny(stateJSON)
	record.CapturedAt = parseTime(capturedAt)
	return &record, nil
}

func scanDelegationRows(rows *sql.Rows) (*memory.WorkflowDelegationRecord, error) {
	var record memory.WorkflowDelegationRecord
	var background int
	var requestJSON string
	var resultJSON string
	var metadataJSON string
	var startedAt string
	var updatedAt string
	err := rows.Scan(
		&record.DelegationID,
		&record.WorkflowID,
		&record.RunID,
		&record.TaskID,
		&record.State,
		&record.TrustClass,
		&record.Recoverability,
		&background,
		&requestJSON,
		&resultJSON,
		&metadataJSON,
		&startedAt,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}
	record.Background = background == 1
	_ = json.Unmarshal([]byte(requestJSON), &record.Request)
	if strings.TrimSpace(resultJSON) != "" && strings.TrimSpace(resultJSON) != "null" {
		var result core.DelegationResult
		if err := json.Unmarshal([]byte(resultJSON), &result); err == nil {
			record.Result = &result
		}
	}
	record.Metadata = decodeJSONMap(metadataJSON)
	record.StartedAt = parseTime(startedAt)
	record.UpdatedAt = parseTime(updatedAt)
	return &record, nil
}

func scanDelegationTransitionRows(rows *sql.Rows) (*memory.WorkflowDelegationTransitionRecord, error) {
	var record memory.WorkflowDelegationTransitionRecord
	var metadataJSON string
	var createdAt string
	err := rows.Scan(
		&record.TransitionID,
		&record.DelegationID,
		&record.WorkflowID,
		&record.RunID,
		&record.FromState,
		&record.ToState,
		&metadataJSON,
		&createdAt,
	)
	if err != nil {
		return nil, err
	}
	record.Metadata = decodeJSONMap(metadataJSON)
	record.CreatedAt = parseTime(createdAt)
	return &record, nil
}

func scanLineageBindingRows(rows *sql.Rows) (*LineageBindingRecord, error) {
	var record LineageBindingRecord
	var updatedAt string
	if err := rows.Scan(&record.WorkflowID, &record.RunID, &record.LineageID, &record.AttemptID, &record.RuntimeID, &record.SessionID, &record.State, &updatedAt); err != nil {
		return nil, err
	}
	record.UpdatedAt = parseTime(updatedAt)
	return &record, nil
}

func scanLineageBindingRow(row *sql.Row) (*LineageBindingRecord, error) {
	var record LineageBindingRecord
	var updatedAt string
	if err := row.Scan(&record.WorkflowID, &record.RunID, &record.LineageID, &record.AttemptID, &record.RuntimeID, &record.SessionID, &record.State, &updatedAt); err != nil {
		return nil, err
	}
	record.UpdatedAt = parseTime(updatedAt)
	return &record, nil
}

func upsertLineageBindingTx(ctx context.Context, exec interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, record LineageBindingRecord) error {
	_, err := exec.ExecContext(ctx, `INSERT INTO rex_fmp_lineage_bindings (
		workflow_id, run_id, lineage_id, attempt_id, runtime_id, session_id, state, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(workflow_id, run_id) DO UPDATE SET
		lineage_id = excluded.lineage_id,
		attempt_id = excluded.attempt_id,
		runtime_id = excluded.runtime_id,
		session_id = excluded.session_id,
		state = excluded.state,
		updated_at = excluded.updated_at`,
		record.WorkflowID,
		record.RunID,
		record.LineageID,
		record.AttemptID,
		record.RuntimeID,
		record.SessionID,
		record.State,
		timeString(record.UpdatedAt),
	)
	return err
}

func (s *SQLiteWorkflowStateStore) indexWorkflowArtifact(ctx context.Context, artifact memory.WorkflowArtifactRecord) error {
	if s == nil || s.db == nil {
		return nil
	}
	content := strings.Join(compactNonEmpty(
		artifact.SummaryText,
		artifact.InlineRawText,
		mustJSON(artifact.SummaryMetadata),
	), "\n\n")
	if strings.TrimSpace(content) == "" {
		return nil
	}
	_, err := retrieval.NewIngestionPipeline(s.db, s.retrievalEmbedder).Ingest(ctx, retrieval.IngestRequest{
		CanonicalURI: workflowArtifactRetrievalURI(artifact.WorkflowID, artifact.RunID, artifact.ArtifactID),
		Content:      []byte(content),
		SourceType:   "text",
		CorpusScope:  workflowRetrievalScope(artifact.WorkflowID),
		PolicyTags:   compactNonEmpty("workflow-artifact", artifact.Kind, artifact.ContentType),
	})
	return err
}

func (s *SQLiteWorkflowStateStore) indexStepArtifact(ctx context.Context, artifact memory.StepArtifactRecord) error {
	if s == nil || s.db == nil {
		return nil
	}
	content := strings.Join(compactNonEmpty(
		artifact.SummaryText,
		artifact.InlineRawText,
		mustJSON(artifact.SummaryMetadata),
	), "\n\n")
	if strings.TrimSpace(content) == "" {
		return nil
	}
	_, err := retrieval.NewIngestionPipeline(s.db, s.retrievalEmbedder).Ingest(ctx, retrieval.IngestRequest{
		CanonicalURI: stepArtifactRetrievalURI(artifact.WorkflowID, artifact.StepRunID, artifact.ArtifactID),
		Content:      []byte(content),
		SourceType:   "text",
		CorpusScope:  workflowRetrievalScope(artifact.WorkflowID),
		PolicyTags:   compactNonEmpty("step-artifact", artifact.Kind, artifact.ContentType),
	})
	return err
}

func (s *SQLiteWorkflowStateStore) indexKnowledgeRecord(ctx context.Context, record memory.KnowledgeRecord) error {
	if s == nil || s.db == nil {
		return nil
	}
	content := strings.Join(compactNonEmpty(record.Title, record.Content, mustJSON(record.Metadata)), "\n\n")
	if strings.TrimSpace(content) == "" {
		return nil
	}
	_, err := retrieval.NewIngestionPipeline(s.db, s.retrievalEmbedder).Ingest(ctx, retrieval.IngestRequest{
		CanonicalURI: workflowKnowledgeRetrievalURI(record.WorkflowID, record.RecordID),
		Content:      []byte(content),
		SourceType:   "text",
		CorpusScope:  workflowRetrievalScope(record.WorkflowID),
		PolicyTags:   compactNonEmpty("workflow-knowledge", string(record.Kind), record.Status),
	})
	return err
}

func workflowRetrievalScope(workflowID string) string {
	return "workflow:" + strings.TrimSpace(workflowID)
}

func workflowArtifactRetrievalURI(workflowID, runID, artifactID string) string {
	return fmt.Sprintf("workflow://artifact/%s/%s/%s", strings.TrimSpace(workflowID), strings.TrimSpace(runID), strings.TrimSpace(artifactID))
}

func stepArtifactRetrievalURI(workflowID, stepRunID, artifactID string) string {
	return fmt.Sprintf("workflow://step-artifact/%s/%s/%s", strings.TrimSpace(workflowID), strings.TrimSpace(stepRunID), strings.TrimSpace(artifactID))
}

func workflowKnowledgeRetrievalURI(workflowID, recordID string) string {
	return fmt.Sprintf("workflow://knowledge/%s/%s", strings.TrimSpace(workflowID), strings.TrimSpace(recordID))
}

func compactNonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || value == "{}" || value == "null" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func mustJSON(value any) string {
	if value == nil {
		return "{}"
	}
	data, err := json.Marshal(core.RedactAny(value))
	if err != nil {
		return "{}"
	}
	return string(data)
}

func mustJSONAny(value any) string {
	if value == nil {
		return "null"
	}
	data, err := json.Marshal(core.RedactAny(value))
	if err != nil {
		return "null"
	}
	return string(data)
}

// UpdateWorkflowMetadata merges the provided fields into the workflow's metadata.
// It uses a read-modify-write approach for compatibility with all SQLite versions.
func (s *SQLiteWorkflowStateStore) UpdateWorkflowMetadata(ctx context.Context, workflowID string, updates map[string]any) error {
	if workflowID == "" {
		return errors.New("workflow id required")
	}
	if len(updates) == 0 {
		return nil
	}

	// Read current metadata
	row := s.db.QueryRowContext(ctx, `SELECT metadata_json FROM workflows WHERE workflow_id = ?`, workflowID)
	var metadataJSON string
	if err := row.Scan(&metadataJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil // Workflow doesn't exist, return without error (idempotent)
		}
		return err
	}

	// Decode existing metadata
	metadata := decodeJSONMap(metadataJSON)

	// Merge updates (updates take precedence)
	for k, v := range updates {
		metadata[k] = v
	}

	// Write back merged metadata
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE workflows SET metadata_json = ?, updated_at = ? WHERE workflow_id = ?`,
		mustJSON(metadata),
		timeString(time.Now().UTC()),
		workflowID,
	)
	return err
}

func decodeJSONMap(value string) map[string]any {
	if strings.TrimSpace(value) == "" {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(value), &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func decodeJSONAny(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var out any
	if err := json.Unmarshal([]byte(value), &out); err != nil {
		return nil
	}
	return out
}

func decodeJSONStringSlice(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(value), &out); err != nil {
		return nil
	}
	return out
}

func ensureTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}

func timeString(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func parseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return t
}

func planHash(plan core.Plan) string {
	sum := sha1.Sum([]byte(mustJSON(plan)))
	return hex.EncodeToString(sum[:])
}

func newRecordID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

func filterKnowledgeBySteps(records []memory.KnowledgeRecord, stepIDs []string) []memory.KnowledgeRecord {
	if len(stepIDs) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(stepIDs))
	for _, id := range stepIDs {
		allowed[id] = struct{}{}
	}
	out := make([]memory.KnowledgeRecord, 0, len(records))
	for _, record := range records {
		if record.StepID == "" {
			continue
		}
		if _, ok := allowed[record.StepID]; ok {
			out = append(out, record)
		}
	}
	return out
}
