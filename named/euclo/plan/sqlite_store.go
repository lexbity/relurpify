package plan

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	frameworkplan "github.com/lexcodex/relurpify/framework/plan"

	_ "github.com/mattn/go-sqlite3"
)

type SQLitePlanStore struct {
	db *sql.DB
}

func OpenSQLite(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	if path == ":memory:" {
		db.SetMaxOpenConns(1)
	}
	return db, nil
}

func NewSQLitePlanStore(db *sql.DB) (*SQLitePlanStore, error) {
	if db == nil {
		return nil, errors.New("euclo plan store: db required")
	}
	if err := ensureSchema(context.Background(), db); err != nil {
		return nil, err
	}
	return &SQLitePlanStore{db: db}, nil
}

func ensureSchema(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS living_plans (
			plan_id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			title TEXT NOT NULL,
			plan_json TEXT NOT NULL,
			step_order_json TEXT NOT NULL,
			convergence_json TEXT NOT NULL,
			version INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_living_plans_workflow ON living_plans(workflow_id);`,
		`CREATE TABLE IF NOT EXISTS living_plan_steps (
			plan_id TEXT NOT NULL,
			step_id TEXT NOT NULL,
			ordinal INTEGER NOT NULL,
			step_json TEXT NOT NULL,
			status TEXT NOT NULL,
			confidence REAL NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(plan_id, step_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_living_plan_steps_plan_ordinal ON living_plan_steps(plan_id, ordinal);`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLitePlanStore) SavePlan(ctx context.Context, plan *frameworkplan.LivingPlan) error {
	if s == nil || s.db == nil {
		return errors.New("euclo plan store: store not initialized")
	}
	if plan == nil {
		return errors.New("euclo plan store: plan required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	planJSON, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	stepOrderJSON, err := json.Marshal(plan.StepOrder)
	if err != nil {
		return err
	}
	convergenceJSON, err := json.Marshal(plan.ConvergenceTarget)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO living_plans (
		plan_id, workflow_id, title, plan_json, step_order_json, convergence_json, version, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(plan_id) DO UPDATE SET
		workflow_id = excluded.workflow_id,
		title = excluded.title,
		plan_json = excluded.plan_json,
		step_order_json = excluded.step_order_json,
		convergence_json = excluded.convergence_json,
		version = excluded.version,
		created_at = excluded.created_at,
		updated_at = excluded.updated_at`,
		plan.ID, plan.WorkflowID, plan.Title, string(planJSON), string(stepOrderJSON), string(convergenceJSON),
		plan.Version, plan.CreatedAt.UTC().Format(time.RFC3339Nano), plan.UpdatedAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM living_plan_steps WHERE plan_id = ?`, plan.ID); err != nil {
		return err
	}
	for idx, stepID := range plan.StepOrder {
		step := plan.Steps[stepID]
		if step == nil {
			continue
		}
		stepJSON, err := json.Marshal(step)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO living_plan_steps
			(plan_id, step_id, ordinal, step_json, status, confidence, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			plan.ID, stepID, idx, string(stepJSON), step.Status, step.ConfidenceScore, step.UpdatedAt.UTC().Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLitePlanStore) LoadPlan(ctx context.Context, planID string) (*frameworkplan.LivingPlan, error) {
	row := s.db.QueryRowContext(ctx, `SELECT plan_json FROM living_plans WHERE plan_id = ?`, planID)
	var planJSON string
	if err := row.Scan(&planJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return s.loadFromJSON(ctx, planID, planJSON)
}

func (s *SQLitePlanStore) LoadPlanByWorkflow(ctx context.Context, workflowID string) (*frameworkplan.LivingPlan, error) {
	row := s.db.QueryRowContext(ctx, `SELECT plan_id, plan_json FROM living_plans WHERE workflow_id = ? ORDER BY updated_at DESC LIMIT 1`, workflowID)
	var planID, planJSON string
	if err := row.Scan(&planID, &planJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return s.loadFromJSON(ctx, planID, planJSON)
}

func (s *SQLitePlanStore) loadFromJSON(ctx context.Context, planID, planJSON string) (*frameworkplan.LivingPlan, error) {
	var plan frameworkplan.LivingPlan
	if err := json.Unmarshal([]byte(planJSON), &plan); err != nil {
		return nil, err
	}
	if plan.Steps == nil {
		plan.Steps = make(map[string]*frameworkplan.PlanStep)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT step_id, step_json FROM living_plan_steps WHERE plan_id = ? ORDER BY ordinal ASC`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	plan.StepOrder = plan.StepOrder[:0]
	for rows.Next() {
		var stepID, stepJSON string
		if err := rows.Scan(&stepID, &stepJSON); err != nil {
			return nil, err
		}
		var step frameworkplan.PlanStep
		if err := json.Unmarshal([]byte(stepJSON), &step); err != nil {
			return nil, err
		}
		stepCopy := step
		plan.Steps[stepID] = &stepCopy
		plan.StepOrder = append(plan.StepOrder, stepID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &plan, nil
}

func (s *SQLitePlanStore) UpdateStep(ctx context.Context, planID, stepID string, step *frameworkplan.PlanStep) error {
	if step == nil {
		return errors.New("euclo plan store: step required")
	}
	stepJSON, err := json.Marshal(step)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE living_plan_steps
		SET step_json = ?, status = ?, confidence = ?, updated_at = ?
		WHERE plan_id = ? AND step_id = ?`,
		string(stepJSON), step.Status, step.ConfidenceScore, step.UpdatedAt.UTC().Format(time.RFC3339Nano), planID, stepID,
	)
	return err
}

func (s *SQLitePlanStore) InvalidateStep(ctx context.Context, planID, stepID string, rule frameworkplan.InvalidationRule) error {
	plan, err := s.LoadPlan(ctx, planID)
	if err != nil {
		return err
	}
	if plan == nil || plan.Steps[stepID] == nil {
		return nil
	}
	step := plan.Steps[stepID]
	step.Status = frameworkplan.PlanStepInvalidated
	step.InvalidatedBy = append(step.InvalidatedBy, rule)
	step.UpdatedAt = time.Now().UTC()
	return s.UpdateStep(ctx, planID, stepID, step)
}

func (s *SQLitePlanStore) DeletePlan(ctx context.Context, planID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM living_plan_steps WHERE plan_id = ?`, planID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM living_plans WHERE plan_id = ?`, planID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLitePlanStore) ListPlans(ctx context.Context) ([]frameworkplan.PlanSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT p.plan_id, p.workflow_id, p.title, p.updated_at, COUNT(ps.step_id)
		FROM living_plans p
		LEFT JOIN living_plan_steps ps ON ps.plan_id = p.plan_id
		GROUP BY p.plan_id, p.workflow_id, p.title, p.updated_at
		ORDER BY p.updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]frameworkplan.PlanSummary, 0)
	for rows.Next() {
		var summary frameworkplan.PlanSummary
		var updatedAt string
		if err := rows.Scan(&summary.ID, &summary.WorkflowID, &summary.Title, &updatedAt, &summary.StepCount); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return nil, err
		}
		summary.UpdatedAt = parsed
		out = append(out, summary)
	}
	return out, rows.Err()
}
