// Package model provides context utilities for model call tracking.
package model

import (
	"context"
	"crypto/rand"
	"fmt"
)

// callContextKey is the context key for storing call context.
type callContextKey struct{}

// CallContext holds information about an active model call.
type CallContext struct {
	// CallID is the logical call identifier.
	CallID CallID
	// AttemptID is the physical attempt identifier.
	AttemptID AttemptID
	// GateTicket is the ticket that granted access to the lane.
	GateTicket *Ticket
}

// NewCallContext creates a new CallContext.
func NewCallContext(callID CallID, attemptID AttemptID, ticket *Ticket) *CallContext {
	return &CallContext{
		CallID:     callID,
		AttemptID:  attemptID,
		GateTicket: ticket,
	}
}

// WithCallContext adds a CallContext to the context.
func WithCallContext(ctx context.Context, cc *CallContext) context.Context {
	return context.WithValue(ctx, callContextKey{}, cc)
}

// CallContextFrom returns the CallContext from the context, or nil if not present.
func CallContextFrom(ctx context.Context) *CallContext {
	cc, ok := ctx.Value(callContextKey{}).(*CallContext)
	if !ok {
		return nil
	}
	return cc
}

// CallIDFrom returns the CallID from the context, or an empty string if not present.
func CallIDFrom(ctx context.Context) CallID {
	cc := CallContextFrom(ctx)
	if cc == nil {
		return ""
	}
	return cc.CallID
}

// AttemptIDFrom returns the AttemptID from the context, or an empty string if not present.
func AttemptIDFrom(ctx context.Context) AttemptID {
	cc := CallContextFrom(ctx)
	if cc == nil {
		return ""
	}
	return cc.AttemptID
}

// ReleaseCallContext releases the gate ticket associated with the call context.
// It returns the final gate state.
// This should be called when the physical request completes, even if it failed.
func ReleaseCallContext(ctx context.Context) (GateState, bool) {
	cc := CallContextFrom(ctx)
	if cc == nil || cc.GateTicket == nil {
		return GateState{}, false
	}

	state := cc.GateTicket.Released()
	return state, true
}

// PoisonCallContext poisons the gate from the call context.
// This should be called when a request cannot be canceled and the lane must be poisoned.
// Returns the final gate state.
func PoisonCallContext(ctx context.Context, reason string) (GateState, bool) {
	cc := CallContextFrom(ctx)
	if cc == nil || cc.GateTicket == nil {
		return GateState{}, false
	}

	state := cc.GateTicket.Poison(reason)
	return state, true
}

// GenerateCallID generates a new call ID.
func GenerateCallID() CallID {
	return CallID(fmt.Sprintf("call_%s", generateID()))
}

// GenerateAttemptID generates a new attempt ID.
func GenerateAttemptID() AttemptID {
	return AttemptID(fmt.Sprintf("attempt_%s", generateID()))
}

// generateID returns a 128-bit random identifier encoded as text.
func generateID() string {
	return rand.Text()
}
