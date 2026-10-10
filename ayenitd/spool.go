// spool.go implements the submission edge (Q13): an atomic-rename file
// spool. The app writes one JSON file per submission into pending/; the
// runner claims files via rename to claimed/, materializes them as jobs
// (dedup on CorrelateID), and deletes the claimed file on success.
// Malformed files move to failed/ for inspection — they are never retried.
package ayenitd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"codeburg.org/lexbit/relurpify/jobs"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// SpoolDirs are the spool layout under <state>/jobs/spool/.
type SpoolDirs struct {
	Pending string
	Claimed string
	Failed  string
}

// OpenSpoolDirs materializes the spool layout under <state>/jobs/spool/.
func OpenSpoolDirs(stateDir string) (SpoolDirs, error) {
	root := filepath.Join(stateDir, "jobs", "spool")
	d := SpoolDirs{
		Pending: filepath.Join(root, "pending"),
		Claimed: filepath.Join(root, "claimed"),
		Failed:  filepath.Join(root, "failed"),
	}
	for _, dir := range []string{d.Pending, d.Claimed, d.Failed} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return SpoolDirs{}, fmt.Errorf("spool: create %s: %w", dir, err)
		}
	}
	return d, nil
}

// spoolFile is the strict on-disk submission schema (§5.4). Every field is
// hostile input: unknown fields are rejected, the Spec is validated before
// ingestion, and malformed files land in failed/ untouched.
type spoolFile struct {
	CorrelateID string          `json:"correlate_id"`
	SubmittedAt time.Time       `json:"submitted_at"`
	Producer    string          `json:"producer"`
	Spec        jobs.Spec       `json:"spec"`
	TraceID     string          `json:"trace_id,omitempty"`
	raw         json.RawMessage `json:"-"`
}

// materialize builds the durable Job from the spool file: the store assigns
// the canonical ID; the correlate id is stamped onto the Spec so the store's
// idempotent-ingest index dedups repeat submissions.
func (f spoolFile) materialize(now time.Time) jobs.Job {
	spec := f.Spec
	spec.CorrelateID = f.CorrelateID
	return jobs.Job{
		ID:        fmt.Sprintf("job-%s", f.CorrelateID),
		Spec:      spec,
		State:     jobs.StateQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// spoolWatcher is the runner-internal service that drains pending/ into the
// store. Its poll interval is 500 ms (NFR-1: submit→running p95 ≤ 3 s).
type spoolWatcher struct {
	dirs      SpoolDirs
	store     jobs.Store
	tel       telemetry.Telemetry
	pollEvery time.Duration

	cancel context.CancelFunc
}

func newSpoolWatcher(dirs SpoolDirs, store jobs.Store, tel telemetry.Telemetry) *spoolWatcher {
	return &spoolWatcher{dirs: dirs, store: store, tel: tel, pollEvery: 500 * time.Millisecond}
}

func (w *spoolWatcher) Start(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	go w.loop(runCtx)
	return nil
}

func (w *spoolWatcher) Stop() error {
	if w.cancel != nil {
		w.cancel()
	}
	return nil
}

func (w *spoolWatcher) loop(ctx context.Context) {
	interval := w.pollEvery
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	// Boot recovery first: claimed/ files that never made it into the store
	// (crash between rename and Create) are re-ingested idempotently.
	w.drainOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.drainOnce(ctx)
		}
	}
}

// drainOnce runs one pending→claimed→store pipeline pass.
func (w *spoolWatcher) drainOnce(ctx context.Context) {
	entries, err := os.ReadDir(w.dirs.Pending)
	if err != nil {
		return
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names) // deterministic drain order
	for _, name := range names {
		if ctx.Err() != nil {
			return
		}
		w.ingest(ctx, filepath.Join(w.dirs.Pending, name), filepath.Join(w.dirs.Claimed, name))
	}
	// Boot recovery leg: claimed-but-not-created files (delete the store
	// mid-pipeline in tests to exercise the window).
	claimedEntries, err := os.ReadDir(w.dirs.Claimed)
	if err != nil {
		return
	}
	for _, e := range claimedEntries {
		if ctx.Err() != nil {
			return
		}
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		w.ingest(ctx, filepath.Join(w.dirs.Claimed, e.Name()), "")
	}
}

// ingest claims one spool file (rename pending→claimed), parses it strictly,
// and materializes the job. On success the claimed file is deleted; a
// malformed file moves to failed/. An already-known CorrelateID is counted
// (job.duplicate_ignored), not an error.
func (w *spoolWatcher) ingest(ctx context.Context, src, claimedPath string) {
	data, err := os.ReadFile(src)
	if err != nil {
		return // raced with another drainer; next pass catches it
	}
	if claimedPath != "" {
		if err := os.Rename(src, claimedPath); err != nil {
			return
		}
		src = claimedPath
	}

	var f spoolFile
	if err := json.Unmarshal(data, &f); err != nil {
		w.fail(ctx, src, fmt.Sprintf("malformed spool file: %v", err))
		return
	}
	if f.CorrelateID == "" {
		w.fail(ctx, src, "malformed spool file: correlate_id required")
		return
	}
	if err := f.Spec.Valid(); err != nil {
		w.fail(ctx, src, fmt.Sprintf("invalid spec: %v", err))
		return
	}

	job := f.materialize(time.Now().UTC())
	err = w.store.Create(ctx, job)
	switch {
	case err == nil:
		w.emit(ctx, telemetry.EventJobSubmitted, "spool file ingested", map[string]any{
			"correlate_id": f.CorrelateID, "kind": f.Spec.Kind, "producer": f.Producer, "trace_id": f.TraceID,
		})
	case err == jobs.ErrExists:
		w.emit(ctx, telemetry.EventJobDuplicateIgnored, "duplicate correlate id ignored", map[string]any{
			"correlate_id": f.CorrelateID,
		})
	default:
		// Store unavailable: leave the claimed file for boot re-ingest
		// (the degraded-boot property — the spool never blocks).
		return
	}
	if claimedPath != "" {
		_ = os.Remove(src)
	} else {
		_ = os.Remove(src)
	}
}

// fail moves a malformed file to failed/ and records spool.malformed.
func (w *spoolWatcher) fail(ctx context.Context, src, reason string) {
	dst := filepath.Join(w.dirs.Failed, filepath.Base(src))
	_ = os.Rename(src, dst)
	w.emit(ctx, telemetry.EventSpoolMalformed, reason, map[string]any{
		"file": filepath.Base(src),
	})
}

func (w *spoolWatcher) emit(ctx context.Context, t telemetry.EventType, message string, metadata map[string]any) {
	if w.tel == nil {
		return
	}
	ev := telemetry.Event{Type: t, Message: message, Timestamp: time.Now().UTC(), Metadata: metadata}
	telemetry.StampCorrelation(ctx, &ev)
	w.tel.Emit(ev)
}

// WriteSpoolFile is the app-side submission primitive: temp file in pending/,
// write, fsync, rename, fsync dir (NFR-6 crash atomicity). It lives next to
// the watcher so the wire format has exactly one definition.
func WriteSpoolFile(dirs SpoolDirs, f spoolFile) error {
	data, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("spool: marshal: %w", err)
	}
	name := fmt.Sprintf("%d-%s.job.json", time.Now().UnixNano(), f.CorrelateID)
	tmp := filepath.Join(dirs.Pending, "."+name+".tmp")
	final := filepath.Join(dirs.Pending, name)
	fh, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("spool: temp: %w", err)
	}
	if _, err := fh.Write(data); err != nil {
		fh.Close()
		os.Remove(tmp)
		return fmt.Errorf("spool: write: %w", err)
	}
	if err := fh.Sync(); err != nil {
		fh.Close()
		os.Remove(tmp)
		return fmt.Errorf("spool: fsync: %w", err)
	}
	if err := fh.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("spool: close: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("spool: rename: %w", err)
	}
	if dir, err := os.Open(dirs.Pending); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}
