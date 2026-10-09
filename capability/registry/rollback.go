package registry

import (
	"context"
	"errors"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/capability/ports"
	fwtelemetry "codeburg.org/lexbit/relurpify/telemetry"
)

const (
	// rollbackRingCapacity bounds concurrent rollback tokens per registry.
	// 64 covers any plausible multi-tool undo sequence within a session.
	rollbackRingCapacity = 64
	// rollbackTokenTTL bounds how long a rollback token remains usable. The
	// payload reference (Args) is scrubbed on expiry or eviction (SBH-1 D-9,
	// INV-7: bounded secrets retention).
	rollbackTokenTTL = 15 * time.Minute
)

var errRollbackTokenNotFound = errors.New("rollback token not found")

// rollbackExpiredError reports a rollback attempt whose token outlived its
// TTL. The tool name travels so the expiry is attributed in telemetry. The
// bare safeness of the message matters: the undo window is gone and the raw
// args it would have needed have been scrubbed.
type rollbackExpiredError struct {
	tool string
}

func (e *rollbackExpiredError) Error() string {
	return "rollback window expired — re-run the tool"
}

type rollbackRingEntry struct {
	token     ports.RollbackToken
	expiresAt time.Time
}

// rollbackRing is a bounded, TTL'd store of rollback tokens. Entries are
// time-ordered by insertion (oldest first); eviction drops the oldest entry at
// capacity and a lazy sweep on insert drops expired entries, each time
// scrubbing the dropped token's Args reference so raw invocation arguments
// cannot accumulate in a long-lived process.
type rollbackRing struct {
	mu      sync.Mutex
	entries []*rollbackRingEntry
	byID    map[string]*rollbackRingEntry
	now     func() time.Time
}

func newRollbackRing() *rollbackRing {
	return &rollbackRing{
		byID: make(map[string]*rollbackRingEntry),
		now:  time.Now,
	}
}

// store inserts a token, sweeping expired entries first and evicting the
// oldest entry when the ring is at capacity. Returns the token ID.
func (r *rollbackRing) store(token ports.RollbackToken) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	r.sweepLocked(now)
	if len(r.entries) >= rollbackRingCapacity {
		r.evictOldestLocked()
	}
	entry := &rollbackRingEntry{token: token, expiresAt: now.Add(rollbackTokenTTL)}
	r.entries = append(r.entries, entry)
	r.byID[token.InvocationID] = entry
	return token.InvocationID
}

// take removes and returns the token, or an error when it is missing or
// expired. Expired tokens are scrubbed before the error is returned.
func (r *rollbackRing) take(id string) (ports.RollbackToken, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.byID[id]
	if !ok {
		return ports.RollbackToken{}, errRollbackTokenNotFound
	}
	delete(r.byID, id)
	r.removeEntryLocked(entry)
	if !r.now().Before(entry.expiresAt) {
		entry.token.Args = nil
		entry.token.Result = nil
		return ports.RollbackToken{}, &rollbackExpiredError{tool: entry.token.ToolName}
	}
	return entry.token, nil
}

// size reports the number of live tokens (test and observability aid).
func (r *rollbackRing) size() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries)
}

func (r *rollbackRing) removeEntryLocked(entry *rollbackRingEntry) {
	for i, e := range r.entries {
		if e == entry {
			r.entries = append(r.entries[:i], r.entries[i+1:]...)
			return
		}
	}
}

func (r *rollbackRing) sweepLocked(now time.Time) {
	for len(r.entries) > 0 && !now.Before(r.entries[0].expiresAt) {
		expired := r.entries[0]
		r.entries = r.entries[1:]
		delete(r.byID, expired.token.InvocationID)
		expired.token.Args = nil
		expired.token.Result = nil
	}
}

func (r *rollbackRing) evictOldestLocked() {
	if len(r.entries) == 0 {
		return
	}
	oldest := r.entries[0]
	r.entries = r.entries[1:]
	delete(r.byID, oldest.token.InvocationID)
	oldest.token.Args = nil
	oldest.token.Result = nil
}

// isRevertibleTool reports whether the real (unwrapped) tool implements
// ports.RevertibleTool. The registry stores every legacy tool wrapped in an
// instrumentedTool whose Rollback method delegates with an error when the
// underlying tool is not revertible — checking the wrapper would claim
// revertibility for every tool, which is exactly the comment/code mismatch
// P-7 describes.
func isRevertibleTool(tool ports.Tool) bool {
	if tool == nil {
		return false
	}
	_, ok := unwrapTool(tool).(ports.RevertibleTool)
	return ok
}

func (r *CapabilityRegistry) emitRollbackEvent(ctx context.Context, eventType fwtelemetry.EventType, tokenID, tool string) {
	if r == nil {
		return
	}
	r.mu.RLock()
	tel := r.telemetry
	r.mu.RUnlock()
	if tel == nil {
		return
	}
	ev := fwtelemetry.Event{
		Type:      eventType,
		Timestamp: time.Now().UTC(),
		Metadata: map[string]any{
			"tool":     tool,
			"token_id": tokenID,
		},
	}
	fwtelemetry.StampCorrelation(ctx, &ev)
	tel.Emit(ev)
}
