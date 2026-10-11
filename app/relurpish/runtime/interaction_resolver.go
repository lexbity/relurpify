package runtime

import (
	"context"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

// runtimeInteractionResolver is the relurpish TUI's minimal implementation of
// interaction.Resolver (D12). It bridges the resolver boundary onto the
// runtime's existing frame-resolution path — ResolveInteractionFrame writes the
// human answer onto the live interaction envelope — and so adds no new
// user-visible behavior. Resolution is bounded by the frame's deadline; after
// it, Resolve returns Expired (the surface can still resolve frames through
// the running task's interaction path, which is unchanged).
type runtimeInteractionResolver struct {
	runtime *Runtime
	drops   atomic.Uint64
	logOnce sync.Once
}

var _ interaction.Resolver = (*runtimeInteractionResolver)(nil)

// Resolve waits until the frame is answered on the live interaction envelope,
// its deadline passes, or ctx is cancelled.
func (r *runtimeInteractionResolver) Resolve(ctx context.Context, frame *interaction.InteractionFrame) (interaction.FrameResolution, error) {
	if r == nil || r.runtime == nil {
		return interaction.FrameResolution{Status: interaction.ResolutionExpired}, nil
	}
	if frame == nil || strings.TrimSpace(frame.ID) == "" {
		return interaction.FrameResolution{}, interaction.ErrUnknownFrame
	}
	deadline := frame.Deadline(time.Now().UTC())
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		env := r.runtime.interactionEnvelope(frame.TaskID)
		if env != nil {
			if live, ok := findInteractionFrame(env, frame.ID); ok && live != nil && live.Response != nil {
				answer, _ := interaction.ResponseValue(live)
				return interaction.FrameResolution{
					Status: interaction.ResolutionAnswered,
					Answer: strings.TrimSpace(answer),
					Value:  live.Response.ExtraData,
				}, nil
			}
			if live, ok := findInteractionFrame(env, frame.ID); ok && live != nil && live.Expired(time.Now().UTC()) {
				return interaction.FrameResolution{Status: interaction.ResolutionExpired}, nil
			}
		}
		select {
		case <-ctx.Done():
			return interaction.FrameResolution{Status: interaction.ResolutionExpired}, ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return interaction.FrameResolution{Status: interaction.ResolutionExpired}, nil
			}
		}
	}
}

// Notify delivers informational frames without blocking. The TUI renders these
// via the running task's interaction path; a notification with no live
// envelope is dropped — allowed under the contract's backpressure clause, and
// never silently: the drop is counted (DroppedTotal) and logged once (D8).
func (r *runtimeInteractionResolver) Notify(_ context.Context, frame *interaction.InteractionFrame) error {
	if r == nil || frame == nil {
		return nil
	}
	env := r.runtime.interactionEnvelope(frame.TaskID)
	if env == nil {
		r.drops.Add(1)
		r.logFirstDrop(frame)
	}
	return nil
}

// DroppedTotal reports how many Notify frames were dropped for want of a
// live envelope. The contract makes the drop MUST be counted and surfaced
// through the implementation's own stats — this is that counter.
func (r *runtimeInteractionResolver) DroppedTotal() uint64 {
	if r == nil {
		return 0
	}
	return r.drops.Load()
}

// logFirstDrop logs the first drop once: a running counter needs one
// diagnostic breadcrumb, not one log line per dropped frame.
func (r *runtimeInteractionResolver) logFirstDrop(frame *interaction.InteractionFrame) {
	r.logOnce.Do(func() {
		log.Printf("interaction: Notify frame dropped (no live envelope for task %q); further drops are counted via DroppedTotal", frame.TaskID)
	})
}

// interactionResolver returns the runtime's resolver adapter. It is cheap and
// stateless: called at graph construction time.
func (r *Runtime) interactionResolver() interaction.Resolver {
	return &runtimeInteractionResolver{runtime: r}
}
