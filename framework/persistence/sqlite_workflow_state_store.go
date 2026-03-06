package persistence

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
	_ "github.com/mattn/go-sqlite3"
)

const workflowStateSchemaVersion = 3

// SQLiteWorkflowStateStore persists workflow state in SQLite.
type SQLiteWorkflowStateStore struct {
	db *sql.DB
}

// NewSQLiteWorkflowStateStore opens or creates the workflow state database.
func NewSQLiteWorkflowStateStore(dbPath string) (*SQLiteWorkflowStateStore, error) {
	if strings.TrimSpace(dbPath) == "" {
		return nil, errors.New("workflow state db path required")
	}
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000", filepath.Clean(dbPath))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	store := &SQLiteWorkflowStateStore{db: db}
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
		`CREATE TABLE IF NOT EXISTS workflow_invalidation (
			invalidation_id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			source_step_id TEXT NOT NULL,
			invalidated_step_id TEXT NOT NULL,
			reason TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			FOREIGN KEY(workflow_id) REFERENCES workflows(workflow_id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_steps_status ON workflow_steps(workflow_id, status, ordinal);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_step_runs_attempt ON step_runs(workflow_id, step_id, attempt);`,
		`CREATE INDEX IF NOT EXISTS idx_step_runs_workflow_step ON step_runs(workflow_id, step_id, attempt DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_artifacts_scope ON workflow_artifacts(workflow_id, run_id, created_at ASC);`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_stage_results_scope ON workflow_stage_results(workflow_id, run_id, stage_index ASC, retry_attempt ASC, finished_at ASC);`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_stage_results_valid ON workflow_stage_results(workflow_id, run_id, stage_name, validation_ok, retry_attempt DESC, finished_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_events_workflow_created ON workflow_events(workflow_id, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_knowledge_workflow_kind ON workflow_knowledge(workflow_id, kind, created_at DESC);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	if _, err := s.db.Exec(`INSERT INTO schema_metadata (key, value) VALUES ('workflow_state_schema_version', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, fmt.Sprintf("%d", workflowStateSchemaVersion)); err != nil {
		return err
	}
	return nil
}

func (s *SQLiteWorkflowStateStore) SchemaVersion(ctx context.Context) (int, error) {
	row := s.db.QueryRowContext(ctx, `SELECT value FROM schema_metadata WHERE key = 'workflow_state_schema_version'`)
	var value string
	if err := row.Scan(&value); err != nil {
		return 0, err
	}
	var version int
	if _, err := fmt.Sscanf(value, "%d", &version); err != nil {
		return 0, err
	}
	return version, nil
}

func (s *SQLiteWorkflowStateStore) CreateWorkflow(ctx context.Context, workflow WorkflowRecord) error {
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
		workflow.Status = WorkflowRunStatusPending
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

func (s *SQLiteWorkflowStateStore) GetWorkflow(ctx context.Context, workflowID string) (*WorkflowRecord, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT workflow_id, task_id, task_type, instruction, status, cursor_step_id, version, metadata_json, created_at, updated_at FROM workflows WHERE workflow_id = ?`, workflowID)
	record, ok, err := scanWorkflow(row)
	return record, ok, err
}

func (s *SQLiteWorkflowStateStore) ListWorkflows(ctx context.Context, limit int) ([]WorkflowRecord, error) {
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
	var records []WorkflowRecord
	for rows.Next() {
		record, err := scanWorkflowRows(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, *record)
	}
	return records, rows.Err()
}

func (s *SQLiteWorkflowStateStore) UpdateWorkflowStatus(ctx context.Context, workflowID string, expectedVersion int64, status WorkflowRunStatus, cursorStepID string) (int64, error) {
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

func (s *SQLiteWorkflowStateStore) CreateRun(ctx context.Context, run WorkflowRunRecord) error {
	if run.RunID == "" || run.WorkflowID == "" {
		return errors.New("run id and workflow id required")
	}
	if run.Status == "" {
		run.Status = WorkflowRunStatusPending
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

func (s *SQLiteWorkflowStateStore) GetRun(ctx context.Context, runID string) (*WorkflowRunRecord, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT run_id, workflow_id, status, agent_name, agent_mode, runtime_version, metadata_json, started_at, finished_at FROM workflow_runs WHERE run_id = ?`, runID)
	var record WorkflowRunRecord
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

func (s *SQLiteWorkflowStateStore) UpdateRunStatus(ctx context.Context, runID string, status WorkflowRunStatus, finishedAt *time.Time) error {
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

func (s *SQLiteWorkflowStateStore) SavePlan(ctx context.Context, plan WorkflowPlanRecord) error {
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
			string(StepStatusPending),
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

func (s *SQLiteWorkflowStateStore) GetActivePlan(ctx context.Context, workflowID string) (*WorkflowPlanRecord, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT plan_id, workflow_id, run_id, plan_hash, plan_json, is_active, created_at FROM workflow_plans WHERE workflow_id = ? AND is_active = 1 ORDER BY created_at DESC LIMIT 1`, workflowID)
	var record WorkflowPlanRecord
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

func (s *SQLiteWorkflowStateStore) ListSteps(ctx context.Context, workflowID string) ([]WorkflowStepRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT workflow_id, plan_id, step_id, ordinal, step_json, status, summary, updated_at FROM workflow_steps WHERE workflow_id = ? ORDER BY ordinal ASC`, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []WorkflowStepRecord
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

func (s *SQLiteWorkflowStateStore) ListReadySteps(ctx context.Context, workflowID string) ([]WorkflowStepRecord, error) {
	steps, err := s.ListSteps(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	statusByStep := make(map[string]StepStatus, len(steps))
	for _, step := range steps {
		statusByStep[step.StepID] = step.Status
	}
	var ready []WorkflowStepRecord
	for _, step := range steps {
		if step.Status != StepStatusPending {
			continue
		}
		ok := true
		for _, dep := range step.Dependencies {
			if statusByStep[dep] != StepStatusCompleted {
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

func (s *SQLiteWorkflowStateStore) UpdateStepStatus(ctx context.Context, workflowID, stepID string, status StepStatus, summary string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE workflow_steps SET status = ?, summary = ?, updated_at = ? WHERE workflow_id = ? AND step_id = ?`, string(status), summary, timeString(time.Now().UTC()), workflowID, stepID)
	return err
}

func (s *SQLiteWorkflowStateStore) InvalidateDependents(ctx context.Context, workflowID, sourceStepID, reason string) ([]InvalidationRecord, error) {
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
	out := make([]InvalidationRecord, 0, len(affected))
	for _, stepID := range affected {
		record := InvalidationRecord{
			InvalidationID:    newRecordID("inval"),
			WorkflowID:        workflowID,
			SourceStepID:      sourceStepID,
			InvalidatedStepID: stepID,
			Reason:            reason,
			CreatedAt:         now,
		}
		if _, err := tx.ExecContext(ctx, `UPDATE workflow_steps SET status = ?, updated_at = ? WHERE workflow_id = ? AND step_id = ?`, string(StepStatusInvalidated), timeString(now), workflowID, stepID); err != nil {
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

func (s *SQLiteWorkflowStateStore) ListInvalidations(ctx context.Context, workflowID string) ([]InvalidationRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT invalidation_id, workflow_id, source_step_id, invalidated_step_id, reason, created_at FROM workflow_invalidation WHERE workflow_id = ? ORDER BY created_at ASC`, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InvalidationRecord
	for rows.Next() {
		var record InvalidationRecord
		var createdAt string
		if err := rows.Scan(&record.InvalidationID, &record.WorkflowID, &record.SourceStepID, &record.InvalidatedStepID, &record.Reason, &createdAt); err != nil {
			return nil, err
		}
		record.CreatedAt = parseTime(createdAt)
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *SQLiteWorkflowStateStore) CreateStepRun(ctx context.Context, run StepRunRecord) error {
	if run.StepRunID == "" || run.WorkflowID == "" || run.RunID == "" || run.StepID == "" {
		return errors.New("step run requires ids")
	}
	if run.Status == "" {
		run.Status = StepStatusPending
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

func (s *SQLiteWorkflowStateStore) ListStepRuns(ctx context.Context, workflowID, stepID string) ([]StepRunRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT step_run_id, workflow_id, run_id, step_id, attempt, status, summary, result_json, verification_ok, error_text, started_at, finished_at FROM step_runs WHERE workflow_id = ? AND step_id = ? ORDER BY attempt ASC`, workflowID, stepID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StepRunRecord
	for rows.Next() {
		record, err := scanStepRunRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *SQLiteWorkflowStateStore) UpsertArtifact(ctx context.Context, artifact StepArtifactRecord) error {
	if artifact.ArtifactID == "" || artifact.WorkflowID == "" || artifact.StepRunID == "" {
		return errors.New("artifact ids required")
	}
	if artifact.StorageKind == "" {
		artifact.StorageKind = ArtifactStorageInline
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
	return err
}

func (s *SQLiteWorkflowStateStore) ListArtifacts(ctx context.Context, workflowID, stepRunID string) ([]StepArtifactRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT artifact_id, workflow_id, step_run_id, kind, content_type, storage_kind, summary_text, summary_metadata_json, inline_raw_text, raw_ref, raw_size_bytes, compression_method, created_at FROM step_artifacts WHERE workflow_id = ? AND step_run_id = ? ORDER BY created_at ASC`, workflowID, stepRunID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StepArtifactRecord
	for rows.Next() {
		record, err := scanArtifactRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *SQLiteWorkflowStateStore) UpsertWorkflowArtifact(ctx context.Context, artifact WorkflowArtifactRecord) error {
	if artifact.ArtifactID == "" || artifact.WorkflowID == "" {
		return errors.New("workflow artifact requires ids")
	}
	if artifact.StorageKind == "" {
		artifact.StorageKind = ArtifactStorageInline
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
	return err
}

func (s *SQLiteWorkflowStateStore) ListWorkflowArtifacts(ctx context.Context, workflowID, runID string) ([]WorkflowArtifactRecord, error) {
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
	var out []WorkflowArtifactRecord
	for rows.Next() {
		record, err := scanWorkflowArtifactRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *SQLiteWorkflowStateStore) SaveStageResult(ctx context.Context, record WorkflowStageResultRecord) error {
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

func (s *SQLiteWorkflowStateStore) ListStageResults(ctx context.Context, workflowID, runID string) ([]WorkflowStageResultRecord, error) {
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
	var out []WorkflowStageResultRecord
	for rows.Next() {
		record, err := scanWorkflowStageResultRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *SQLiteWorkflowStateStore) GetLatestValidStageResult(ctx context.Context, workflowID, runID, stageName string) (*WorkflowStageResultRecord, bool, error) {
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

func (s *SQLiteWorkflowStateStore) PutKnowledge(ctx context.Context, record KnowledgeRecord) error {
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
	return err
}

func (s *SQLiteWorkflowStateStore) ListKnowledge(ctx context.Context, workflowID string, kind KnowledgeKind, unresolvedOnly bool) ([]KnowledgeRecord, error) {
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
	var out []KnowledgeRecord
	for rows.Next() {
		record, err := scanKnowledgeRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *SQLiteWorkflowStateStore) AppendEvent(ctx context.Context, event WorkflowEventRecord) error {
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

func (s *SQLiteWorkflowStateStore) ListEvents(ctx context.Context, workflowID string, limit int) ([]WorkflowEventRecord, error) {
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
	var out []WorkflowEventRecord
	for rows.Next() {
		record, err := scanEventRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *record)
	}
	return out, rows.Err()
}

func (s *SQLiteWorkflowStateStore) LoadStepSlice(ctx context.Context, workflowID, stepID string, eventLimit int) (*WorkflowStepSlice, bool, error) {
	workflow, ok, err := s.GetWorkflow(ctx, workflowID)
	if err != nil || !ok {
		return nil, ok, err
	}
	steps, err := s.ListSteps(ctx, workflowID)
	if err != nil {
		return nil, false, err
	}
	stepMap := make(map[string]WorkflowStepRecord, len(steps))
	for _, step := range steps {
		stepMap[step.StepID] = step
	}
	current, ok := stepMap[stepID]
	if !ok {
		return nil, false, nil
	}
	dependencySteps := make([]WorkflowStepRecord, 0, len(current.Dependencies))
	dependencyRuns := make([]StepRunRecord, 0, len(current.Dependencies))
	artifacts := make([]StepArtifactRecord, 0)
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
	facts, err := s.ListKnowledge(ctx, workflowID, KnowledgeKindFact, false)
	if err != nil {
		return nil, false, err
	}
	issues, err := s.ListKnowledge(ctx, workflowID, KnowledgeKindIssue, true)
	if err != nil {
		return nil, false, err
	}
	decisions, err := s.ListKnowledge(ctx, workflowID, KnowledgeKindDecision, false)
	if err != nil {
		return nil, false, err
	}
	events, err := s.ListEvents(ctx, workflowID, eventLimit)
	if err != nil {
		return nil, false, err
	}
	return &WorkflowStepSlice{
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

func scanWorkflow(row interface{ Scan(dest ...any) error }) (*WorkflowRecord, bool, error) {
	record, err := scanWorkflowCommon(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return record, true, nil
}

func scanWorkflowRows(rows *sql.Rows) (*WorkflowRecord, error) {
	return scanWorkflowCommon(rows.Scan)
}

func scanWorkflowCommon(scan func(dest ...any) error) (*WorkflowRecord, error) {
	var record WorkflowRecord
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

func scanStepRows(rows *sql.Rows) (*WorkflowStepRecord, error) {
	var record WorkflowStepRecord
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

func scanStepRunRows(rows *sql.Rows) (*StepRunRecord, error) {
	var record StepRunRecord
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

func scanArtifactRows(rows *sql.Rows) (*StepArtifactRecord, error) {
	var record StepArtifactRecord
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

func scanWorkflowArtifactRows(rows *sql.Rows) (*WorkflowArtifactRecord, error) {
	var record WorkflowArtifactRecord
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

func scanWorkflowStageResult(row interface{ Scan(dest ...any) error }) (*WorkflowStageResultRecord, bool, error) {
	record, err := scanWorkflowStageResultCommon(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return record, true, nil
}

func scanWorkflowStageResultRows(rows *sql.Rows) (*WorkflowStageResultRecord, error) {
	return scanWorkflowStageResultCommon(rows.Scan)
}

func scanWorkflowStageResultCommon(scan func(dest ...any) error) (*WorkflowStageResultRecord, error) {
	var record WorkflowStageResultRecord
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

func scanKnowledgeRows(rows *sql.Rows) (*KnowledgeRecord, error) {
	var record KnowledgeRecord
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

func scanEventRows(rows *sql.Rows) (*WorkflowEventRecord, error) {
	var record WorkflowEventRecord
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

func mustJSON(value any) string {
	if value == nil {
		return "{}"
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func mustJSONAny(value any) string {
	if value == nil {
		return "null"
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "null"
	}
	return string(data)
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

func ensureTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}

func timeString(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
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

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func planHash(plan core.Plan) string {
	sum := sha1.Sum([]byte(mustJSON(plan)))
	return hex.EncodeToString(sum[:])
}

func newRecordID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

func filterKnowledgeBySteps(records []KnowledgeRecord, stepIDs []string) []KnowledgeRecord {
	if len(stepIDs) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(stepIDs))
	for _, id := range stepIDs {
		allowed[id] = struct{}{}
	}
	out := make([]KnowledgeRecord, 0, len(records))
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
