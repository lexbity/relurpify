// spoolclient.go is the app's submission edge (S9): the only channel the app
// uses to reach the runner. It never opens the jobs store (R3 — the Badger
// lock belongs to the runner alone); Submit validates, assigns a CorrelateID
// (timestamp + crypto/rand), writes the crash-atomic spool file, and returns
// an accepted handle. Submission works whether the runner is up, starting,
// or dead — a dead runner leaves spool files a restarted runner drains
// (the degraded-boot property, Q13).
//
// Accepted-handle asymmetry, documented (S9 step 2): the store assigns the
// canonical job ID, so Submit's returned Job carries the correlate-derived
// provisional ID (`job-<correlateID>`) in state queued; the store-assigned
// real ID lands in status.json later.
package ayenitd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/jobs"
)

// SpoolClient submits jobs to the runner through the file spool.
type SpoolClient struct {
	dirs      SpoolDirs
	producer  string
	workspace string
}

// NewSpoolClient builds a client over the spool layout of a state dir. The
// workspace root rides on the client so scheduled submitters (FR-23) can
// stamp payload workspace roots without reaching back into config.
func NewSpoolClient(stateDir, producer, workspace string) (*SpoolClient, error) {
	dirs, err := OpenSpoolDirs(stateDir)
	if err != nil {
		return nil, fmt.Errorf("spoolclient: %w", err)
	}
	return &SpoolClient{dirs: dirs, producer: producer, workspace: workspace}, nil
}

// Workspace returns the workspace root the client was constructed for.
func (c *SpoolClient) Workspace() string { return c.workspace }

// Submit validates the spec, assigns a fresh CorrelateID, writes the spool
// file, and returns the accepted handle (provisional ID, state queued).
func (c *SpoolClient) Submit(ctx context.Context, spec jobs.Spec) (jobs.Job, error) {
	if err := spec.Valid(); err != nil {
		return jobs.Job{}, fmt.Errorf("spoolclient: invalid spec: %w", err)
	}
	correlate, err := NewCorrelateID()
	if err != nil {
		return jobs.Job{}, fmt.Errorf("spoolclient: correlate id: %w", err)
	}
	spec.CorrelateID = correlate
	now := time.Now().UTC()
	f := spoolFile{
		CorrelateID: correlate,
		SubmittedAt: now,
		Producer:    c.producer,
		Spec:        spec,
	}
	if err := WriteSpoolFile(c.dirs, f); err != nil {
		return jobs.Job{}, fmt.Errorf("spoolclient: submit: %w", err)
	}
	accepted := f.materialize(now)
	_ = ctx // the spool never blocks on a remote; ctx documents the contract
	return accepted, nil
}

// NewCorrelateID mints a ULID-shaped id: unix-milli timestamp + crypto/rand
// suffix. No external dependency; uniqueness within a millisecond comes from
// 8 random bytes.
func NewCorrelateID() (string, error) {
	var rnd [8]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return "", fmt.Errorf("crypto/rand: %w", err)
	}
	return fmt.Sprintf("turn-%013d-%s", time.Now().UnixMilli(), hex.EncodeToString(rnd[:])), nil
}
