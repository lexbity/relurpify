package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/event"
	_ "github.com/mattn/go-sqlite3"
)

const eventLogIdempotencyTTL = 24 * time.Hour

var _ event.Log = (*SQLiteEventLog)(nil)

// SQLiteEventLog persists framework events in SQLite.
type SQLiteEventLog struct {
	db          *sql.DB
	path        string
	stopCleanup context.CancelFunc
}

func NewSQLiteEventLog(path string) (*SQLiteEventLog, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("event log path required")
	}
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000", filepath.Clean(path))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	log := &SQLiteEventLog{db: db, path: path}
	if err := log.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	// Prune expired idempotency cache entries in the background so they don't
	// add per-write overhead.  An initial cleanup runs immediately to clear
	// leftovers from previous runs.
	cleanCtx, cancel := context.WithCancel(context.Background())
	log.stopCleanup = cancel
	go log.idemCacheCleanupLoop(cleanCtx)
	return log, nil
}

func (l *SQLiteEventLog) init() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS events (
			seq INTEGER PRIMARY KEY AUTOINCREMENT,
			partition TEXT NOT NULL DEFAULT 'local',
			type TEXT NOT NULL,
			caused_by TEXT,
			payload BLOB,
			actor TEXT,
			idem_key TEXT,
			ts DATETIME NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_events_partition_type ON events(partition, type, seq);`,
		`CREATE TABLE IF NOT EXISTS snapshots (
			partition TEXT PRIMARY KEY,
			seq INTEGER NOT NULL,
			data BLOB NOT NULL,
			ts DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS idem_cache (
			idem_key TEXT PRIMARY KEY,
			seq INTEGER NOT NULL,
			expires DATETIME NOT NULL
		);`,
	}
	for _, stmt := range stmts {
		if _, err := l.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (l *SQLiteEventLog) Append(ctx context.Context, partition string, events []core.FrameworkEvent) ([]uint64, error) {
	if l == nil || l.db == nil {
		return nil, errors.New("event log unavailable")
	}
	if partition == "" {
		partition = "local"
	}
	seqs := make([]uint64, 0, len(events))
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	for _, ev := range events {
		if ev.Timestamp.IsZero() {
			ev.Timestamp = time.Now().UTC()
		}
		if ev.Partition == "" {
			ev.Partition = partition
		}
		if ev.IdempotencyKey != "" {
			var seq uint64
			row := tx.QueryRowContext(ctx, `SELECT seq FROM idem_cache WHERE idem_key = ?`, ev.IdempotencyKey)
			switch scanErr := row.Scan(&seq); scanErr {
			case nil:
				seqs = append(seqs, seq)
				continue
			case sql.ErrNoRows:
			default:
				err = scanErr
				return nil, err
			}
		}
		causedBy, marshalErr := json.Marshal(ev.CausedBy)
		if marshalErr != nil {
			err = marshalErr
			return nil, err
		}
		actor, marshalErr := json.Marshal(ev.Actor)
		if marshalErr != nil {
			err = marshalErr
			return nil, err
		}
		res, execErr := tx.ExecContext(ctx,
			`INSERT INTO events (partition, type, caused_by, payload, actor, idem_key, ts) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			ev.Partition, ev.Type, string(causedBy), []byte(ev.Payload), string(actor), ev.IdempotencyKey, ev.Timestamp.UTC().Format(time.RFC3339Nano),
		)
		if execErr != nil {
			err = execErr
			return nil, err
		}
		lastID, lastErr := res.LastInsertId()
		if lastErr != nil {
			err = lastErr
			return nil, err
		}
		seq := uint64(lastID)
		seqs = append(seqs, seq)
		if ev.IdempotencyKey != "" {
			if _, execErr = tx.ExecContext(ctx, `INSERT OR REPLACE INTO idem_cache (idem_key, seq, expires) VALUES (?, ?, ?)`,
				ev.IdempotencyKey, seq, time.Now().UTC().Add(eventLogIdempotencyTTL).Format(time.RFC3339Nano)); execErr != nil {
				err = execErr
				return nil, err
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return seqs, nil
}

func (l *SQLiteEventLog) Read(ctx context.Context, partition string, afterSeq uint64, limit int, follow bool) ([]core.FrameworkEvent, error) {
	if l == nil || l.db == nil {
		return nil, errors.New("event log unavailable")
	}
	if partition == "" {
		partition = "local"
	}
	for {
		events, err := l.readQuery(ctx, `SELECT seq, ts, type, caused_by, payload, actor, idem_key, partition FROM events WHERE partition = ? AND seq > ? ORDER BY seq ASC`+limitClause(limit), partition, afterSeq)
		if err != nil {
			return nil, err
		}
		if len(events) > 0 || !follow {
			return events, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (l *SQLiteEventLog) ReadByType(ctx context.Context, partition string, typePrefix string, afterSeq uint64, limit int) ([]core.FrameworkEvent, error) {
	if partition == "" {
		partition = "local"
	}
	return l.readQuery(ctx, `SELECT seq, ts, type, caused_by, payload, actor, idem_key, partition FROM events WHERE partition = ? AND seq > ? AND type LIKE ? ORDER BY seq ASC`+limitClause(limit), partition, afterSeq, typePrefix+"%")
}

func (l *SQLiteEventLog) LastSeq(ctx context.Context, partition string) (uint64, error) {
	if partition == "" {
		partition = "local"
	}
	var seq sql.NullInt64
	if err := l.db.QueryRowContext(ctx, `SELECT MAX(seq) FROM events WHERE partition = ?`, partition).Scan(&seq); err != nil {
		return 0, err
	}
	if !seq.Valid {
		return 0, nil
	}
	return uint64(seq.Int64), nil
}

func (l *SQLiteEventLog) TakeSnapshot(ctx context.Context, partition string, seq uint64, data []byte) error {
	if partition == "" {
		partition = "local"
	}
	_, err := l.db.ExecContext(ctx, `INSERT INTO snapshots (partition, seq, data, ts) VALUES (?, ?, ?, ?)
		ON CONFLICT(partition) DO UPDATE SET seq = excluded.seq, data = excluded.data, ts = excluded.ts`,
		partition, seq, data, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (l *SQLiteEventLog) LoadSnapshot(ctx context.Context, partition string) (uint64, []byte, error) {
	if partition == "" {
		partition = "local"
	}
	var seq uint64
	var data []byte
	err := l.db.QueryRowContext(ctx, `SELECT seq, data FROM snapshots WHERE partition = ?`, partition).Scan(&seq, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, nil
	}
	return seq, data, err
}

func (l *SQLiteEventLog) CompactBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	if l == nil || l.db == nil {
		return 0, errors.New("event log unavailable")
	}
	res, err := l.db.ExecContext(ctx, `DELETE FROM events WHERE ts < ?`, cutoff.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (l *SQLiteEventLog) Close() error {
	if l == nil || l.db == nil {
		return nil
	}
	if l.stopCleanup != nil {
		l.stopCleanup()
	}
	return l.db.Close()
}

const idemCacheCleanupInterval = 5 * time.Minute

func (l *SQLiteEventLog) idemCacheCleanupLoop(ctx context.Context) {
	l.pruneIdemCache(ctx)
	ticker := time.NewTicker(idemCacheCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.pruneIdemCache(ctx)
		}
	}
}

func (l *SQLiteEventLog) pruneIdemCache(ctx context.Context) {
	_, _ = l.db.ExecContext(ctx, `DELETE FROM idem_cache WHERE expires <= ?`, time.Now().UTC().Format(time.RFC3339Nano))
}

func (l *SQLiteEventLog) readQuery(ctx context.Context, query string, args ...any) ([]core.FrameworkEvent, error) {
	rows, err := l.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.FrameworkEvent
	for rows.Next() {
		var (
			ev          core.FrameworkEvent
			ts          string
			causedByRaw string
			actorRaw    string
			payload     []byte
		)
		if err := rows.Scan(&ev.Seq, &ts, &ev.Type, &causedByRaw, &payload, &actorRaw, &ev.IdempotencyKey, &ev.Partition); err != nil {
			return nil, err
		}
		ev.Payload = append([]byte(nil), payload...)
		if causedByRaw != "" {
			if err := json.Unmarshal([]byte(causedByRaw), &ev.CausedBy); err != nil {
				return nil, err
			}
		}
		if actorRaw != "" {
			if err := json.Unmarshal([]byte(actorRaw), &ev.Actor); err != nil {
				return nil, err
			}
		}
		if ts != "" {
			parsed, err := time.Parse(time.RFC3339Nano, ts)
			if err != nil {
				return nil, err
			}
			ev.Timestamp = parsed
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

func limitClause(limit int) string {
	if limit <= 0 {
		return ""
	}
	return fmt.Sprintf(" LIMIT %d", limit)
}
