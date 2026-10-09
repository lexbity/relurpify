package interaction

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// defaultFrameDeadline is the fallback frame deadline when a frame carries no
// explicit Timeout (D12), matching the default HITL TTL policy.
const defaultFrameDeadline = 5 * time.Minute

// frameState is the lifecycle state of one frame in the store.
type frameState string

const (
	frameStateOpen     frameState = "open"
	frameStateResolved frameState = "resolved"
	frameStateExpired  frameState = "expired"
)

// storedFrame is the resolver's lifecycle record for one frame ID.
type storedFrame struct {
	frame    *InteractionFrame
	state    frameState
	deadline time.Time
	waiters  map[uint64]chan FrameResolution
	resolved FrameResolution
	// emittedLateAnswer deduplicates the frame.expired_late_answer telemetry
	// event across repeated late answers to the same expired frame.
	emittedLateAnswer bool
}

// FrameResolver is the exactly-once, expiring frame store behind the Resolver
// interface (D12). Frames open on first contact, transition to resolved on an
// in-window answer/denial, and expire lazily when their deadline is observed
// to have passed. A resolution arriving after expiry returns
// {Status: Expired}, emits frame.expired_late_answer once, and changes no
// state — the stale-consent hole is closed at the resolver boundary.
type FrameResolver struct {
	mu     sync.Mutex
	frames map[string]*storedFrame
	next   uint64
	now    func() time.Time
	// droppedNotifies counts informational frames dropped under backpressure
	// (Notify MUST NOT block; drops are surfaced via this counter — D12).
	droppedNotifies uint64
}

// NewFrameResolver creates the store. clock is the deterministic time source
// for expiry tests; nil uses time.Now.
func NewFrameResolver(clock func() time.Time) *FrameResolver {
	if clock == nil {
		clock = time.Now
	}
	return &FrameResolver{
		frames: make(map[string]*storedFrame),
		now:    clock,
	}
}

// Resolve implements Resolver. The frame auto-registers when the resolver has
// never held it; a duplicate Resolve for a resolved ID returns the recorded
// resolution; an expired frame resolves as Expired. Otherwise Resolve blocks
// until the frame is answered, denied, its deadline passes, or ctx is done.
func (r *FrameResolver) Resolve(ctx context.Context, frame *InteractionFrame) (FrameResolution, error) {
	if r == nil {
		return FrameResolution{}, fmt.Errorf("interaction: frame resolver is nil")
	}
	if frame == nil || strings.TrimSpace(frame.ID) == "" {
		return FrameResolution{}, ErrUnknownFrame
	}

	r.mu.Lock()
	sf, ok := r.frames[frame.ID]
	if !ok {
		sf = &storedFrame{
			frame:    frame,
			state:    frameStateOpen,
			deadline: frame.Deadline(r.now()),
			waiters:  make(map[uint64]chan FrameResolution),
		}
		r.frames[frame.ID] = sf
	} else if sf.state == frameStateResolved {
		resolved := sf.resolved
		r.mu.Unlock()
		return resolved, nil
	} else if sf.state == frameStateExpired || r.now().After(sf.deadline) {
		if sf.state != frameStateExpired {
			sf.state = frameStateExpired
		}
		r.mu.Unlock()
		return FrameResolution{Status: ResolutionExpired}, nil
	}
	waiterID := r.next
	r.next++
	waiter := make(chan FrameResolution, 1)
	sf.waiters[waiterID] = waiter
	deadline := sf.deadline
	r.mu.Unlock()

	remaining := deadline.Sub(r.now())
	if remaining <= 0 {
		r.dropWaiter(frame.ID, waiterID)
		r.expire(frame.ID)
		return FrameResolution{Status: ResolutionExpired}, nil
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()

	select {
	case resolution := <-waiter:
		return resolution, nil
	case <-ctx.Done():
		// A cancelled wait must not leave a waiter registered: a late answer
		// for a cancelled frame would otherwise deliver to nobody forever.
		r.dropWaiter(frame.ID, waiterID)
		return FrameResolution{Status: ResolutionExpired}, ctx.Err()
	case <-timer.C:
		r.expire(frame.ID)
		return FrameResolution{Status: ResolutionExpired}, nil
	}
}

// dropWaiter removes one waiter from the frame's open set.
func (r *FrameResolver) dropWaiter(frameID string, waiterID uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sf, ok := r.frames[frameID]
	if !ok || sf.state != frameStateOpen {
		return
	}
	if _, ok := sf.waiters[waiterID]; ok {
		delete(sf.waiters, waiterID)
	}
}

// Notify implements Resolver. It never blocks; if a frame with the same ID is
// already tracked, the notification is dropped and counted.
func (r *FrameResolver) Notify(ctx context.Context, frame *InteractionFrame) (err error) {
	_ = ctx
	if r == nil {
		return nil
	}
	if frame == nil || strings.TrimSpace(frame.ID) == "" {
		return ErrUnknownFrame
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.frames[frame.ID]; ok {
		r.droppedNotifies++
		return nil
	}
	r.frames[frame.ID] = &storedFrame{
		frame:    frame,
		state:    frameStateOpen,
		deadline: frame.Deadline(r.now()),
		waiters:  make(map[uint64]chan FrameResolution),
	}
	return nil
}

// DroppedNotifies reports how many informational frames were dropped under
// backpressure (the resolver's own surfaced stats — D12).
func (r *FrameResolver) DroppedNotifies() uint64 {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.droppedNotifies
}

// Answer resolves an open frame with the given answer and structured value.
// The answer is validated against the frame's Choices when it declares any.
// Exactly-once by frame ID: an answer to an already-resolved frame is an
// idempotent no-op returning the recorded resolution; an answer to an expired
// frame returns {Status: Expired}, emits frame.expired_late_answer once, and
// changes no state.
func (r *FrameResolver) Answer(ctx context.Context, frameID, answer string, value map[string]any) (FrameResolution, error) {
	return r.resolveResult(ctx, frameID, ResolutionAnswered, answer, value)
}

// Deny rejects an open frame. Exactly-once and expiry semantics match Answer.
func (r *FrameResolver) Deny(ctx context.Context, frameID, reason string) (FrameResolution, error) {
	return r.resolveResult(ctx, frameID, ResolutionDenied, reason, nil)
}

func (r *FrameResolver) resolveResult(ctx context.Context, frameID string, status ResolutionStatus, text string, value map[string]any) (FrameResolution, error) {
	if r == nil {
		return FrameResolution{}, fmt.Errorf("interaction: frame resolver is nil")
	}
	frameID = strings.TrimSpace(frameID)
	if frameID == "" {
		return FrameResolution{}, ErrUnknownFrame
	}

	var lateEmit bool
	r.mu.Lock()
	sf, ok := r.frames[frameID]
	if !ok {
		r.mu.Unlock()
		return FrameResolution{}, ErrUnknownFrame
	}
	if sf.frame.Expired(r.now()) || r.now().After(sf.deadline) {
		if sf.state != frameStateExpired {
			sf.state = frameStateExpired
		}
		if !sf.emittedLateAnswer {
			sf.emittedLateAnswer = true
			lateEmit = true
		}
		r.mu.Unlock()
		if lateEmit {
			r.emitLateAnswer(ctx, sf.frame)
		}
		return FrameResolution{Status: ResolutionExpired}, nil
	}
	if sf.state == frameStateResolved {
		resolved := sf.resolved
		r.mu.Unlock()
		return resolved, nil
	}
	if status == ResolutionAnswered {
		if mismatch := validateFrameAnswer(sf.frame, text); mismatch != "" {
			r.mu.Unlock()
			return FrameResolution{}, fmt.Errorf("interaction: %s", mismatch)
		}
	}
	resolution := FrameResolution{Status: status, Answer: text, Value: value}
	sf.state = frameStateResolved
	sf.resolved = resolution
	waiters := sf.waiters
	sf.waiters = make(map[uint64]chan FrameResolution)
	r.mu.Unlock()

	for _, waiter := range waiters {
		waiter <- resolution
	}
	return resolution, nil
}

// expire transitions the frame to expired and releases any open waiters with
// the Expired resolution.
func (r *FrameResolver) expire(frameID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sf, ok := r.frames[frameID]
	if !ok || sf.state != frameStateOpen {
		return
	}
	sf.state = frameStateExpired
	resolution := FrameResolution{Status: ResolutionExpired}
	waiters := sf.waiters
	sf.waiters = make(map[uint64]chan FrameResolution)
	for _, waiter := range waiters {
		waiter <- resolution
	}
}

// validateFrameAnswer matches the answer against the frame's declared choices
// when any exist. Frames without choices accept free text.
func validateFrameAnswer(frame *InteractionFrame, answer string) string {
	if frame == nil || len(frame.Choices) == 0 {
		return ""
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return "answer required for frame with choices"
	}
	for _, choice := range frame.Choices {
		if choice == answer {
			return ""
		}
	}
	return fmt.Sprintf("answer %q is not a valid choice for the frame", answer)
}

// OpenFrames returns a snapshot of the frames still awaiting resolution (state
// open and within their deadline). Resume surfaces [open] frames only; expired
// frames surface as gaps (D12/FR-18).
func (r *FrameResolver) OpenFrames() []*InteractionFrame {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*InteractionFrame
	for _, sf := range r.frames {
		if sf.state != frameStateOpen || r.now().After(sf.deadline) {
			continue
		}
		out = append(out, sf.frame)
	}
	return out
}

// emitLateAnswer surfaces the stale-consent breach as the
// frame.expired_late_answer telemetry event (FR-18).
func (r *FrameResolver) emitLateAnswer(ctx context.Context, frame *InteractionFrame) {
	sink := telemetry.TelemetryFromContext(ctx)
	if sink == nil || frame == nil {
		return
	}
	event := telemetry.Event{
		Type:      telemetry.EventFrameExpiredLateAnswer,
		Message:   "resolution arrived after frame expiry; nothing changed",
		Timestamp: time.Now().UTC(),
		TaskID:    frame.TaskID,
		Metadata: map[string]any{
			"frame_id":   strings.TrimSpace(frame.ID),
			"frame_type": string(frame.Type),
			"frame_seq":  frame.Seq,
		},
	}
	telemetry.StampCorrelation(ctx, &event)
	sink.Emit(event)
}

var _ Resolver = (*FrameResolver)(nil)
