package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"codeburg.org/lexbit/relurpify/telemetry"
)

// ReapOptions tunes the boot-time orphan sweep (SBH-1 D-11).
type ReapOptions struct {
	// Binary is the docker CLI; default "docker".
	Binary string
	// Now is the injectable clock.
	Now func() time.Time
	// MinAge is the grace a dead-owner container gets before reaping
	// (the crashed supervisor's NEXT boot may arrive sooner). Default 5m.
	MinAge time.Duration
	// MaxAge is the absolute age cap: any managed container older than this is
	// reclaimed regardless of owner liveness. Default 24h.
	MaxAge time.Duration
	// Budget bounds the whole sweep. Default 5s.
	Budget time.Duration
	// Telemetry receives the sandbox.orphan_reaped events (nil-safe).
	Telemetry telemetry.Telemetry
}

// ReapReport summarizes one orphan sweep.
type ReapReport struct {
	Scanned     int
	Reaped      int
	ReapedNames []string
	Errors      []string
}

// ReapOrphans lists every container tagged relurpify.managed=true and reaps
// the ones whose owner is gone: owner PID dead AND past the grace age, OR past
// the absolute MaxAge regardless (bounded by Budget). A live owner PID — ours
// or a concurrently running second instance — is never reaped (R-7).
func ReapOrphans(ctx context.Context, opts ReapOptions) (ReapReport, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if strings.TrimSpace(opts.Binary) == "" {
		opts.Binary = "docker"
	}
	if opts.MinAge <= 0 {
		opts.MinAge = 5 * time.Minute
	}
	if opts.MaxAge <= 0 {
		opts.MaxAge = 24 * time.Hour
	}
	if opts.Budget <= 0 {
		opts.Budget = 5 * time.Second
	}
	report := ReapReport{}
	ownPID := os.Getpid()
	start := time.Now()
	binaryPath, err := exec.LookPath(opts.Binary)
	if err != nil {
		return report, fmt.Errorf("orphan sweep: %s not found: %w", opts.Binary, err)
	}

	deadline, cancel := context.WithTimeout(ctx, opts.Budget)
	defer cancel()
	// The listing gets its own sane bound independent of the (possibly tiny)
	// reap budget so a budget-exhausted sweep still returns a scanned report.
	psCtx, psCancel := context.WithTimeout(ctx, 5*time.Second)
	defer psCancel()
	out, err := exec.CommandContext(psCtx, binaryPath,
		"ps", "-a",
		"--filter", "label="+LabelManaged+"=true",
		"--format", "{{.Names}}\t{{.Labels}}",
	).Output()
	if err != nil {
		return report, fmt.Errorf("orphan sweep: docker ps failed: %w", err)
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		report.Scanned++
		name, labels := splitPSLine(line)
		if name == "" {
			continue
		}
		ownerPID, _ := labelInt(labels, LabelPID)
		created, ok := labelInt(labels, LabelCreated)
		if !ok {
			created = 0
		}
		now := opts.Now()
		age := now.Sub(time.Unix(created, 0))
		if age < 0 {
			age = 0
		}
		ownerDead := int64(ownPID) != ownerPID && !pidAlive(ownerPID)
		shouldReap := (ownerDead && age >= opts.MinAge) || age >= opts.MaxAge

		// Budget check before each reap: a sweep that spends its whole budget
		// on one hung rm must still return.
		if shouldReap && time.Since(start) < opts.Budget {
			if reapErr := reapOne(deadline, binaryPath, name); reapErr != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("%s: %v", name, reapErr))
				continue
			}
			report.Reaped++
			report.ReapedNames = append(report.ReapedNames, name)
			emitOrphanReaped(ctx, opts.Telemetry, name, age)
		}
	}
	return report, nil
}

// splitPSLine splits "name\tlabels" from docker ps --format.
func splitPSLine(line string) (name, labels string) {
	if idx := strings.IndexByte(line, '\t'); idx >= 0 {
		return strings.TrimSpace(line[:idx]), line[idx+1:]
	}
	return strings.TrimSpace(line), ""
}

// labelInt parses "relurpify.pid=1234,relurpify.created=1700000000" style
// label lists for an integer label value.
func labelInt(labels, key string) (int64, bool) {
	for _, pair := range strings.Split(labels, ",") {
		pair = strings.TrimSpace(pair)
		k, v, ok := strings.Cut(pair, "=")
		if !ok || strings.TrimSpace(k) != key {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

// pidAlive reports whether another process owns the pid. A permission error
// means the process exists (owner-check only); ESRCH means it is gone.
func pidAlive(pid int64) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(int(pid), 0)
	switch {
	case err == nil:
		return true
	case errors.Is(err, syscall.EPERM):
		return true
	default:
		return false // ESRCH and friends ⇒ no live owner
	}
}

// reapOne force-removes a container, bounded by the sweep deadline.
func reapOne(ctx context.Context, binaryPath, name string) error {
	rm := exec.CommandContext(ctx, binaryPath, "rm", "-f", name)
	out, err := rm.CombinedOutput()
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		detail := strings.TrimSpace(string(out))
		if detail != "" {
			return fmt.Errorf("docker rm -f: %w (%s)", err, detail)
		}
		return fmt.Errorf("docker rm -f: %w", err)
	}
	return nil
}

// emitOrphanReaped records the sandbox.orphan_reaped lifecycle event.
func emitOrphanReaped(ctx context.Context, sink telemetry.Telemetry, name string, age time.Duration) {
	emitCommandEvent(ctx, sink, telemetry.EventSandboxOrphanReaped,
		"sandbox orphan reaped",
		map[string]any{
			"name":        name,
			"age_seconds": int64(age / time.Second),
		},
	)
}
