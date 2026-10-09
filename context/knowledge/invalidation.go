package knowledge

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

const (
	// invalidationEventBuffer is the invalidation subscriber's channel depth.
	invalidationEventBuffer = 64
	// invalidationDebounce coalesces a burst of chunk events into one flush.
	invalidationDebounce = 50 * time.Millisecond
	// invalidationDegradedAt is the consecutive-failure threshold at which the
	// loop reports itself degraded.
	invalidationDegradedAt = 5
	// invalidationBackoffCap bounds the linear retry backoff.
	invalidationBackoffCap = 5 * time.Second
)

// StaleChunkReporter can surface stale chunks to domain-specific consumers.
type StaleChunkReporter interface {
	ReportStaleChunks(ctx context.Context, chunkIDs []ChunkID, affectedPaths []string, reason string) error
}

// InvalidationPass reacts to revision drift and surfaces stale chunks. Its loop
// is non-lethal: a store or reporter failure is logged, counted, and retried
// with linear backoff instead of killing the loop.
type InvalidationPass struct {
	Store         *ChunkStore
	Staleness     *StalenessManager
	Events        *EventBus
	Tensions      any
	Reporter      StaleChunkReporter
	WorkspaceRoot string

	mu     sync.Mutex
	cancel context.CancelFunc
	wg     sync.WaitGroup

	stateMu         sync.Mutex
	eventCh         <-chan Event
	pending         map[ChunkID]struct{}
	pendingPaths    map[string]struct{}
	pendingReason   string
	pendingRevision *CodeRevisionChangedPayload
	kick            chan struct{}
}

// Start launches the invalidation loop in the background and returns as soon as
// it is running. Stop cancels the loop and waits for the goroutine to exit.
// A second Start while running returns an error.
func (p *InvalidationPass) Start(ctx context.Context) error {
	if p == nil || p.Events == nil {
		return nil
	}
	p.mu.Lock()
	if p.cancel != nil {
		p.mu.Unlock()
		return fmt.Errorf("invalidation pass already started")
	}
	runCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel

	p.stateMu.Lock()
	p.pending = make(map[ChunkID]struct{})
	p.pendingPaths = make(map[string]struct{})
	p.pendingReason = ""
	p.pendingRevision = nil
	p.kick = make(chan struct{}, 1)
	p.stateMu.Unlock()

	p.wg.Add(1)
	p.mu.Unlock()

	go func() {
		defer p.wg.Done()
		p.run(runCtx)
	}()
	return nil
}

// Stop cancels the invalidation loop and waits for its goroutine to exit. It is
// safe to call before Start and multiple times. The cancel function is cleared
// only after the goroutine has exited so a concurrent Start cannot race with
// the WaitGroup wait.
func (p *InvalidationPass) Stop() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	cancel := p.cancel
	p.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	p.wg.Wait()
	p.mu.Lock()
	p.cancel = nil
	p.mu.Unlock()
	return nil
}

// Drain folds events already queued on this pass's subscription into the
// debounce buffer, without blocking longer than d. It is the subscriber-side
// drain the epoch barrier calls; the bus itself stays channel-neutral.
func (p *InvalidationPass) Drain(d time.Duration) int {
	if p == nil {
		return 0
	}
	if d <= 0 {
		d = invalidationDebounce
	}
	deadline := time.Now().Add(d)
	drained := 0
	for {
		p.stateMu.Lock()
		ch := p.eventCh
		p.stateMu.Unlock()
		if ch == nil {
			return drained
		}
		select {
		case event, ok := <-ch:
			if !ok {
				return drained
			}
			p.receiveEvent(event)
			drained++
			p.signal()
			if time.Now().After(deadline) {
				return drained
			}
		default:
			return drained
		}
	}
}

// run is the invalidation loop body. It returns only when the context is
// cancelled or the event stream closes.
func (p *InvalidationPass) run(ctx context.Context) {
	if p == nil || p.Events == nil {
		return
	}
	ch, unsub := p.Events.Subscribe(invalidationEventBuffer)
	defer unsub()
	p.stateMu.Lock()
	p.eventCh = ch
	p.stateMu.Unlock()
	defer func() {
		p.stateMu.Lock()
		p.eventCh = nil
		p.stateMu.Unlock()
	}()

	var (
		timer  *time.Timer
		timerC <-chan time.Time
	)
	stopTimer := func() {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer = nil
		timerC = nil
	}
	scheduleAfter := func(d time.Duration) {
		if timer == nil {
			timer = time.NewTimer(d)
			timerC = timer.C
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(d)
		timerC = timer.C
	}

	failures := 0
	for {
		select {
		case <-ctx.Done():
			stopTimer()
			return
		case <-p.kickChan():
			if p.hasWork() {
				scheduleAfter(invalidationDebounce)
			}
		case <-timerC:
			if err := p.flush(ctx); err != nil {
				failures++
				p.reportFailure(failures, err)
				stopTimer()
				scheduleAfter(invalidationBackoff(failures))
				continue
			}
			failures = 0
			stopTimer()
		case event, ok := <-ch:
			if !ok {
				stopTimer()
				return
			}
			p.receiveEvent(event)
			if p.hasWork() {
				scheduleAfter(invalidationDebounce)
			}
		}
	}
}

// receiveEvent folds one bus event into the debounce buffer. Revision events
// are queued for synchronous handling at the next flush so a store failure can
// be retried rather than lost.
func (p *InvalidationPass) receiveEvent(event Event) {
	switch event.Kind {
	case EventCodeRevisionChanged:
		payload, ok := event.Payload.(CodeRevisionChangedPayload)
		if !ok {
			return
		}
		p.foldRevision(payload)
	case EventChunkStaled:
		payload, ok := event.Payload.(ChunkStaledPayload)
		if !ok {
			return
		}
		p.foldStaled(payload)
	}
}

// flush drains the pending revision and stale set through the store. Pending
// work is retained on failure so the next attempt retries it.
func (p *InvalidationPass) flush(ctx context.Context) error {
	revision := p.takeRevision()
	if revision != nil {
		if err := p.HandleRevisionChanged(ctx, *revision); err != nil {
			p.restoreRevision(revision)
			return err
		}
	}
	ids, paths, reason, ok := p.takePending()
	if !ok {
		return nil
	}
	manager := p.stalenessManager()
	propagated, err := manager.PropagateSync(ctx, ids, 0)
	if err != nil {
		return err
	}
	if err := p.SurfaceStaleChunks(ctx, ids, paths, reason); err != nil {
		return err
	}
	if len(propagated) > 0 {
		if err := p.SurfaceStaleChunks(ctx, propagated, paths, reason); err != nil {
			return err
		}
	}
	p.clearPending(ids, paths)
	return nil
}

func (p *InvalidationPass) HandleRevisionChanged(ctx context.Context, payload CodeRevisionChangedPayload) error {
	if p == nil || p.Store == nil {
		return nil
	}
	matches, err := p.matchAffectedChunks(payload.AffectedPaths, payload.NewRevision)
	if err != nil || len(matches) == 0 {
		return err
	}
	manager := p.stalenessManager()
	staled, err := manager.BulkMarkStaleCollect(ctx, matches)
	if err != nil || len(staled) == 0 {
		return err
	}
	if p.Events != nil {
		p.Events.EmitChunkStaled(ChunkStaledPayload{
			WorkspaceRoot: firstNonEmpty(payload.WorkspaceRoot, p.WorkspaceRoot),
			ChunkIDs:      chunkIDsToStrings(staled),
			AffectedPaths: append([]string(nil), payload.AffectedPaths...),
			Reason:        "code_revision_changed",
		})
	}
	return nil
}

func (p *InvalidationPass) SurfaceStaleDuringStream(ctx context.Context, result *StreamResult) error {
	if result == nil || len(result.StaleDuringStream) == 0 {
		return nil
	}
	return p.SurfaceStaleChunks(ctx, result.StaleDuringStream, nil, "stale_during_stream")
}

func (p *InvalidationPass) SurfaceStaleChunks(ctx context.Context, chunkIDs []ChunkID, affectedPaths []string, reason string) error {
	if p == nil || len(chunkIDs) == 0 {
		return nil
	}
	if p.Reporter != nil {
		return p.Reporter.ReportStaleChunks(ctx, chunkIDs, affectedPaths, reason)
	}
	return nil
}

func (p *InvalidationPass) matchAffectedChunks(paths []string, newRevision string) ([]ChunkID, error) {
	if p == nil || p.Store == nil {
		return nil, nil
	}
	pathSet := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path != "" {
			pathSet[path] = struct{}{}
		}
	}
	if len(pathSet) == 0 {
		return nil, nil
	}
	matches := make([]ChunkID, 0)
	seen := make(map[ChunkID]struct{})
	for path := range pathSet {
		chunks, err := p.Store.FindByFilePath(path)
		if err != nil {
			return nil, err
		}
		for _, chunk := range chunks {
			if strings.TrimSpace(newRevision) != "" && strings.TrimSpace(chunk.Provenance.CodeStateRef) == strings.TrimSpace(newRevision) {
				continue
			}
			if _, ok := seen[chunk.ID]; ok {
				continue
			}
			seen[chunk.ID] = struct{}{}
			matches = append(matches, chunk.ID)
		}
	}
	return matches, nil
}

func (p *InvalidationPass) stalenessManager() *StalenessManager {
	if p.Staleness != nil {
		return p.Staleness
	}
	return &StalenessManager{Store: p.Store, Propagate: true, MaxDepth: 3}
}

// reportFailure logs and, at or beyond the degraded threshold, surfaces a
// knowledge.invalidation_degraded health event.
func (p *InvalidationPass) reportFailure(failures int, err error) {
	log.Printf("invalidation pass failure %d: %v", failures, err)
	if p.Events == nil || err == nil {
		return
	}
	if failures == invalidationDegradedAt || (failures > invalidationDegradedAt && failures%invalidationDegradedAt == 0) {
		p.Events.EmitInvalidationDegraded(InvalidationDegradedPayload{
			WorkspaceRoot: p.WorkspaceRoot,
			FailureCount:  failures,
			Error:         err.Error(),
		})
	}
}

func invalidationBackoff(failures int) time.Duration {
	if failures < 1 {
		failures = 1
	}
	backoff := invalidationDebounce * time.Duration(failures)
	if backoff > invalidationBackoffCap {
		backoff = invalidationBackoffCap
	}
	return backoff
}

func (p *InvalidationPass) signal() {
	p.stateMu.Lock()
	kick := p.kick
	p.stateMu.Unlock()
	if kick == nil {
		return
	}
	select {
	case kick <- struct{}{}:
	default:
	}
}

func (p *InvalidationPass) kickChan() <-chan struct{} {
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	return p.kick
}

func (p *InvalidationPass) hasWork() bool {
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	return len(p.pending) > 0 || p.pendingRevision != nil
}

func (p *InvalidationPass) foldStaled(payload ChunkStaledPayload) {
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	if p.pending == nil {
		p.pending = make(map[ChunkID]struct{})
	}
	if p.pendingPaths == nil {
		p.pendingPaths = make(map[string]struct{})
	}
	for _, id := range chunkIDsFromStrings(payload.ChunkIDs) {
		p.pending[id] = struct{}{}
	}
	for _, path := range payload.AffectedPaths {
		if trimmed := strings.TrimSpace(path); trimmed != "" {
			p.pendingPaths[trimmed] = struct{}{}
		}
	}
	if p.pendingReason == "" {
		p.pendingReason = payload.Reason
	}
}

func (p *InvalidationPass) foldRevision(payload CodeRevisionChangedPayload) {
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	if p.pendingRevision != nil {
		payload.AffectedPaths = append(append([]string(nil), p.pendingRevision.AffectedPaths...), payload.AffectedPaths...)
		if payload.NewRevision == "" {
			payload.NewRevision = p.pendingRevision.NewRevision
		}
		if payload.WorkspaceRoot == "" {
			payload.WorkspaceRoot = p.pendingRevision.WorkspaceRoot
		}
	}
	p.pendingRevision = &payload
}

func (p *InvalidationPass) takeRevision() *CodeRevisionChangedPayload {
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	revision := p.pendingRevision
	p.pendingRevision = nil
	return revision
}

func (p *InvalidationPass) restoreRevision(revision *CodeRevisionChangedPayload) {
	if revision == nil {
		return
	}
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	p.pendingRevision = revision
}

func (p *InvalidationPass) takePending() ([]ChunkID, []string, string, bool) {
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	if len(p.pending) == 0 {
		return nil, nil, "", false
	}
	ids := make([]ChunkID, 0, len(p.pending))
	for id := range p.pending {
		ids = append(ids, id)
	}
	paths := make([]string, 0, len(p.pendingPaths))
	for path := range p.pendingPaths {
		paths = append(paths, path)
	}
	return ids, paths, p.pendingReason, true
}

func (p *InvalidationPass) clearPending(ids []ChunkID, paths []string) {
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	for _, id := range ids {
		delete(p.pending, id)
	}
	for _, path := range paths {
		delete(p.pendingPaths, path)
	}
	if len(p.pending) == 0 && len(p.pendingPaths) == 0 {
		p.pendingReason = ""
	}
}

func chunkIDsFromStrings(ids []string) []ChunkID {
	out := make([]ChunkID, 0, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) != "" {
			out = append(out, ChunkID(strings.TrimSpace(id)))
		}
	}
	return out
}
