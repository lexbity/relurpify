package interaction

import (
	"context"
	"errors"
)

// ResolutionStatus is the terminal state of one interaction frame (D12).
type ResolutionStatus string

const (
	// ResolutionAnswered means the frame was answered within its deadline.
	ResolutionAnswered ResolutionStatus = "answered"
	// ResolutionDenied means the frame was explicitly rejected.
	ResolutionDenied ResolutionStatus = "denied"
	// ResolutionExpired means the frame's deadline passed without an answer
	// (or a resolution arrived after the deadline — stale consent).
	ResolutionExpired ResolutionStatus = "expired"
)

// FrameResolution is the outcome of resolving one interaction frame.
// Answer is set only when Status is ResolutionAnswered; Value carries
// structured slot fills (e.g. ClarificationResumeMetadata).
type FrameResolution struct {
	Status ResolutionStatus
	Answer string
	Value  map[string]any
}

// Resolver answers interaction frames. It is required at Euclo construction
// (D12): a surface always exists, and absence is a construction error.
//
// Resolve blocks until the frame is answered, denied, or its deadline passes,
// and returns the resolution. Resolution is exactly-once per frame ID: a
// duplicate Resolve for an already-resolved ID returns the recorded resolution
// (never a second grant, never a second prompt), and a resolution of an
// expired frame returns {Status: Expired}.
//
// Notify delivers an informational frame (progress, selection notice). It MUST
// NOT block on consumer availability and MAY drop under backpressure; a drop
// MUST be counted and surfaced by the implementation's own stats.
type Resolver interface {
	Resolve(ctx context.Context, frame *InteractionFrame) (FrameResolution, error)
	Notify(ctx context.Context, frame *InteractionFrame) error
}

var (
	// ErrUnknownFrame reports a resolution attempt for a frame ID the
	// resolver has never held (or whose window already closed and was
	// reaped).
	ErrUnknownFrame = errors.New("interaction: unknown frame")
	// ErrDuplicateFrame reports a registration of a frame ID already held in
	// the open state.
	ErrDuplicateFrame = errors.New("interaction: duplicate frame")
)
