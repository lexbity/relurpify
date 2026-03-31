package plan

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type SQLitePlanStore struct {
	db *sql.DB
}

func OpenSQLite(path string) (*sql.DB, error) {
	return sql.Open("sqlite3", path)
}

func NewSQLitePlanStore(db *sql.DB) (*SQLitePlanStore, error) {
	if db == nil {
		return nil, errors.New("plan db required")
	}
	store := &SQLitePlanStore{db: db}
	if err := store.ensureSchema(context.Background()); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *SQLitePlanStore) ensureSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS living_plans (
	plan_id TEXT PRIMARY KEY,
	workflow_id TEXT NOT NULL,
	title TEXT NOT NULL,
	plan_json TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_living_plans_workflow_updated
	ON living_plans (workflow_id, updated_at DESC);
`)
	return err
}

func (s *SQLitePlanStore) SavePlan(ctx context.Context, plan *LivingPlan) error {
	if s == nil || s.db == nil {
		return errors.New("plan store unavailable")
	}
	if plan == nil || plan.ID == "" || plan.WorkflowID == "" {
		return errors.New("plan id and workflow id required")
	}
	now := time.Now().UTC()
	if plan.CreatedAt.IsZero() {
		plan.CreatedAt = now
	}
	if plan.UpdatedAt.IsZero() {
		plan.UpdatedAt = now
	}
	payload, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO living_plans (plan_id, workflow_id, title, plan_json, updated_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(plan_id) DO UPDATE SET
		 	workflow_id = excluded.workflow_id,
		 	title = excluded.title,
		 	plan_json = excluded.plan_json,
		 	updated_at = excluded.updated_at`,
		plan.ID,
		plan.WorkflowID,
		plan.Title,
		string(payload),
		plan.UpdatedAt.UTC().Format(time.RFC3339Nano),
		plan.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *SQLitePlanStore) LoadPlan(ctx context.Context, planID string) (*LivingPlan, error) {
	return s.loadOne(ctx, `SELECT plan_json FROM living_plans WHERE plan_id = ?`, planID)
}

func (s *SQLitePlanStore) LoadPlanByWorkflow(ctx context.Context, workflowID string) (*LivingPlan, error) {
	return s.loadOne(ctx, `SELECT plan_json FROM living_plans WHERE workflow_id = ? ORDER BY updated_at DESC LIMIT 1`, workflowID)
}

func (s *SQLitePlanStore) loadOne(ctx context.Context, query string, arg string) (*LivingPlan, error) {
	if s == nil || s.db == nil || arg == "" {
		return nil, nil
	}
	var raw string
	err := s.db.QueryRowContext(ctx, query, arg).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var plan LivingPlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return nil, err
	}
	return &plan, nil
}

func (s *SQLitePlanStore) UpdateStep(ctx context.Context, planID, stepID string, step *PlanStep) error {
	if step == nil {
		return nil
	}
	plan, err := s.LoadPlan(ctx, planID)
	if err != nil || plan == nil {
		return err
	}
	if plan.Steps == nil {
		plan.Steps = map[string]*PlanStep{}
	}
	copy := *step
	plan.Steps[stepID] = &copy
	plan.UpdatedAt = time.Now().UTC()
	return s.SavePlan(ctx, plan)
}

func (s *SQLitePlanStore) InvalidateStep(ctx context.Context, planID, stepID string, rule InvalidationRule) error {
	plan, err := s.LoadPlan(ctx, planID)
	if err != nil || plan == nil {
		return err
	}
	step := plan.Steps[stepID]
	if step == nil {
		return nil
	}
	step.InvalidatedBy = append(step.InvalidatedBy, rule)
	step.Status = PlanStepInvalidated
	step.UpdatedAt = time.Now().UTC()
	plan.UpdatedAt = step.UpdatedAt
	return s.SavePlan(ctx, plan)
}

func (s *SQLitePlanStore) DeletePlan(ctx context.Context, planID string) error {
	if s == nil || s.db == nil || planID == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM living_plans WHERE plan_id = ?`, planID)
	return err
}

func (s *SQLitePlanStore) ListPlans(ctx context.Context) ([]PlanSummary, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT plan_id, workflow_id, title, plan_json, updated_at FROM living_plans ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var summaries []PlanSummary
	for rows.Next() {
		var (
			id         string
			workflowID string
			title      string
			raw        string
			updatedRaw string
		)
		if err := rows.Scan(&id, &workflowID, &title, &raw, &updatedRaw); err != nil {
			return nil, err
		}
		var plan LivingPlan
		if err := json.Unmarshal([]byte(raw), &plan); err != nil {
			return nil, err
		}
		updatedAt, err := time.Parse(time.RFC3339Nano, updatedRaw)
		if err != nil {
			updatedAt = plan.UpdatedAt
		}
		summaries = append(summaries, PlanSummary{
			ID:         id,
			WorkflowID: workflowID,
			Title:      title,
			StepCount:  len(plan.Steps),
			UpdatedAt:  updatedAt,
		})
	}
	return summaries, rows.Err()
}
