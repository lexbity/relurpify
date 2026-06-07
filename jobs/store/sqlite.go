package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"codeburg.org/lexbit/relurpify/jobs"
)

type config struct {
	path string
}

func WithPath(path string) func(*config) { return func(c *config) { c.path = path } }

func Open(options ...func(*config)) (*Store, error) {
	var cfg config
	for _, o := range options {
		o(&cfg)
	}
	if cfg.path == "" {
		return nil, errors.New("sqlite job store path required")
	}
	db, err := sql.Open("sqlite3", cfg.path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

type Store struct {
	db *sql.DB
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS jobs (
			id         TEXT PRIMARY KEY,
			kind       TEXT NOT NULL,
			queue      TEXT NOT NULL,
			state      TEXT NOT NULL DEFAULT 'queued',
			payload    BLOB,
			priority   INTEGER NOT NULL DEFAULT 0,
			max_attempts INTEGER NOT NULL DEFAULT 1,
			backoff_ns INTEGER NOT NULL DEFAULT 0,
			timeout_ns INTEGER NOT NULL DEFAULT 0,
			attempt    INTEGER NOT NULL DEFAULT 0,
			labels     TEXT,
			tags       TEXT,
			correlate_id TEXT,
			resume_token TEXT,
			last_error TEXT,
			created_at DATETIME NOT NULL,
			updated_at DATETIME,
			completed_at DATETIME,
			metadata   TEXT
		);
		CREATE TABLE IF NOT EXISTS job_events (
			id        TEXT PRIMARY KEY,
			job_id    TEXT NOT NULL REFERENCES jobs(id),
			type      TEXT NOT NULL,
			state     TEXT,
			occurred  DATETIME NOT NULL,
			message   TEXT,
			metadata  TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_job_events_job ON job_events(job_id, occurred);
		CREATE TABLE IF NOT EXISTS job_checkpoints (
			id        TEXT PRIMARY KEY,
			job_id    TEXT NOT NULL REFERENCES jobs(id),
			state     BLOB NOT NULL,
			token     TEXT,
			created   DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_job_ckpt_job ON job_checkpoints(job_id, created DESC);
	`)
	return err
}

func rowJob(sc interface {
	Scan(dest ...any) error
}) (jobs.Job, error) {
	var j jobs.Job
	var kind, queue string
	var payloadJSON, labelsJSON, tagsJSON, metadataJSON, resumeToken, lastError sql.NullString
	var priority, maxAttempts, attempt int
	var backoffNS, timeoutNS int64
	var createdAt, updatedAt, completedAt sql.NullTime

	err := sc.Scan(
		&j.ID, &kind, &queue, &j.State,
		&payloadJSON, &priority, &maxAttempts, &backoffNS, &timeoutNS,
		&attempt, &labelsJSON, &tagsJSON, &resumeToken, &lastError,
		&createdAt, &updatedAt, &completedAt, &metadataJSON,
	)
	if err != nil {
		return j, err
	}
	j.Spec = jobs.Spec{
		Kind:        kind,
		Queue:       queue,
		Priority:    priority,
		MaxAttempts: maxAttempts,
		Backoff:     time.Duration(backoffNS),
		Timeout:     time.Duration(timeoutNS),
	}
	if payloadJSON.Valid {
		j.Spec.Payload = decodeJSON(payloadJSON.String)
	}
	if labelsJSON.Valid {
		json.Unmarshal([]byte(labelsJSON.String), &j.Spec.Labels)
	}
	if tagsJSON.Valid {
		json.Unmarshal([]byte(tagsJSON.String), &j.Tags)
	}
	if resumeToken.Valid {
		j.ResumeToken = resumeToken.String
	}
	if lastError.Valid {
		j.LastError = lastError.String
	}
	if createdAt.Valid {
		j.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		j.UpdatedAt = updatedAt.Time
	}
	if completedAt.Valid {
		j.CompletedAt = completedAt.Time
	}
	if metadataJSON.Valid {
		json.Unmarshal([]byte(metadataJSON.String), &j.Metadata)
	}
	return j, nil
}

func bindJob(j jobs.Job) (map[string]any, error) {
	payloadJSON, err := encodeJSON(j.Spec.Payload)
	if err != nil {
		return nil, fmt.Errorf("payload: %w", err)
	}
	labelsJSON, _ := encodeJSON(j.Spec.Labels)
	tagsJSON, _ := encodeJSON(j.Tags)
	metadataJSON, _ := encodeJSON(j.Metadata)
	return map[string]any{
		"id":           j.ID,
		"kind":         j.Spec.Kind,
		"queue":        j.Spec.Queue,
		"state":        string(j.State),
		"payload":      payloadJSON,
		"priority":     j.Spec.Priority,
		"max_attempts": j.Spec.MaxAttempts,
		"backoff_ns":   int64(j.Spec.Backoff),
		"timeout_ns":   int64(j.Spec.Timeout),
		"attempt":      j.Attempt,
		"labels":       labelsJSON,
		"tags":         tagsJSON,
		"correlate_id": j.Spec.CorrelateID,
		"resume_token": nullString(j.ResumeToken),
		"last_error":   nullString(j.LastError),
		"created_at":   j.CreatedAt,
		"updated_at":   nullTime(j.UpdatedAt),
		"completed_at": nullTime(j.CompletedAt),
		"metadata":     metadataJSON,
	}, nil
}

func (s *Store) Create(ctx context.Context, job jobs.Job) error {
	if err := job.Valid(); err != nil {
		return err
	}
	v, err := bindJob(job)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO jobs (id,kind,queue,state,payload,priority,max_attempts,backoff_ns,timeout_ns,
			attempt,labels,tags,correlate_id,resume_token,last_error,created_at,updated_at,completed_at,metadata)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		v["id"], v["kind"], v["queue"], v["state"], v["payload"],
		v["priority"], v["max_attempts"], v["backoff_ns"], v["timeout_ns"],
		v["attempt"], v["labels"], v["tags"], v["correlate_id"],
		v["resume_token"], v["last_error"], v["created_at"],
		v["updated_at"], v["completed_at"], v["metadata"],
	)
	if err != nil {
		if isConstraintErr(err) {
			return jobs.ErrExists
		}
		return fmt.Errorf("create job: %w", err)
	}
	return nil
}

func (s *Store) Update(ctx context.Context, job jobs.Job) error {
	if err := job.Valid(); err != nil {
		return err
	}
	v, err := bindJob(job)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE jobs SET kind=?,queue=?,state=?,payload=?,priority=?,max_attempts=?,backoff_ns=?,timeout_ns=?,
			attempt=?,labels=?,tags=?,correlate_id=?,resume_token=?,last_error=?,created_at=?,updated_at=?,completed_at=?,metadata=?
		WHERE id=?`,
		v["kind"], v["queue"], v["state"], v["payload"],
		v["priority"], v["max_attempts"], v["backoff_ns"], v["timeout_ns"],
		v["attempt"], v["labels"], v["tags"], v["correlate_id"],
		v["resume_token"], v["last_error"], v["created_at"],
		v["updated_at"], v["completed_at"], v["metadata"],
		v["id"],
	)
	if err != nil {
		return fmt.Errorf("update job: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return jobs.ErrNotFound
	}
	return nil
}

func (s *Store) Load(ctx context.Context, id string) (*jobs.Job, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id,kind,queue,state,payload,priority,max_attempts,backoff_ns,timeout_ns,
			attempt,labels,tags,resume_token,last_error,created_at,updated_at,completed_at,metadata
		FROM jobs WHERE id=?`, id)
	job, err := rowJob(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, jobs.ErrNotFound
		}
		return nil, fmt.Errorf("load job: %w", err)
	}
	return &job, nil
}

func (s *Store) List(ctx context.Context, q jobs.Query) ([]jobs.Job, error) {
	where := []string{"1=1"}
	args := []any{}
	if q.Queue != "" {
		where = append(where, "queue=?")
		args = append(args, q.Queue)
	}
	if q.State != "" {
		where = append(where, "state=?")
		args = append(args, string(q.State))
	}
	query := fmt.Sprintf(`SELECT id,kind,queue,state,payload,priority,max_attempts,backoff_ns,timeout_ns,
		attempt,labels,tags,resume_token,last_error,created_at,updated_at,completed_at,metadata
		FROM jobs WHERE %s ORDER BY priority DESC, created_at ASC`, stringsJoin(where, " AND "))
	if q.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", q.Limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()
	var out []jobs.Job
	for rows.Next() {
		job, err := rowJob(rows)
		if err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

func (s *Store) AppendEvent(ctx context.Context, e jobs.Event) error {
	if err := e.Valid(); err != nil {
		return err
	}
	metaJSON, _ := encodeJSON(e.Metadata)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO job_events (id,job_id,type,state,occurred,message,metadata) VALUES (?,?,?,?,?,?,?)`,
		e.ID, e.JobID, string(e.Type), string(e.State), e.Occurred, e.Message, metaJSON)
	if err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	return nil
}

func (s *Store) Events(ctx context.Context, jobID string) ([]jobs.Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,job_id,type,state,occurred,message,metadata FROM job_events WHERE job_id=? ORDER BY occurred ASC`, jobID)
	if err != nil {
		return nil, fmt.Errorf("load events: %w", err)
	}
	defer rows.Close()
	var out []jobs.Event
	for rows.Next() {
		var e jobs.Event
		var metaJSON sql.NullString
		if err := rows.Scan(&e.ID, &e.JobID, &e.Type, &e.State, &e.Occurred, &e.Message, &metaJSON); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		if metaJSON.Valid {
			json.Unmarshal([]byte(metaJSON.String), &e.Metadata)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) SaveCheckpoint(ctx context.Context, c jobs.Checkpoint) error {
	if err := c.Valid(); err != nil {
		return err
	}
	stateJSON, err := encodeJSON(c.State)
	if err != nil {
		return fmt.Errorf("checkpoint state: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO job_checkpoints (id,job_id,state,token,created) VALUES (?,?,?,?,?)`,
		c.ID, c.JobID, stateJSON, c.Token, c.Created)
	if err != nil {
		return fmt.Errorf("save checkpoint: %w", err)
	}
	return nil
}

func (s *Store) LoadCheckpoint(ctx context.Context, jobID string) (*jobs.Checkpoint, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,job_id,state,token,created FROM job_checkpoints WHERE job_id=? ORDER BY created DESC LIMIT 1`, jobID)
	var c jobs.Checkpoint
	var stateJSON string
	if err := row.Scan(&c.ID, &c.JobID, &stateJSON, &c.Token, &c.Created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, jobs.ErrCkptNotFound
		}
		return nil, fmt.Errorf("load checkpoint: %w", err)
	}
	json.Unmarshal([]byte(stateJSON), &c.State)
	return &c, nil
}

func encodeJSON(v any) (string, error) {
	if v == nil {
		return "null", nil
	}
	b, err := json.Marshal(v)
	return string(b), err
}

func decodeJSON(s string) any {
	var v any
	json.Unmarshal([]byte(s), &v)
	return v
}

func nullString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func nullTime(v time.Time) *time.Time {
	if v.IsZero() {
		return nil
	}
	return &v
}

func stringsJoin(elems []string, sep string) string {
	switch len(elems) {
	case 0:
		return ""
	case 1:
		return elems[0]
	}
	n := len(sep) * (len(elems) - 1)
	for _, e := range elems {
		n += len(e)
	}
	var b []byte
	b = append(b, elems[0]...)
	for _, e := range elems[1:] {
		b = append(b, sep...)
		b = append(b, e...)
	}
	return string(b)
}

func isConstraintErr(err error) bool {
	return err != nil && (stringsContains(err.Error(), "UNIQUE constraint") || stringsContains(err.Error(), "UNIQUE"))
}

func stringsContains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
