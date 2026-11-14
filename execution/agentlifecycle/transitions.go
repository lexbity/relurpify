package agentlifecycle

import (
	"errors"
	"fmt"
)

// RunState is the validated lifecycle status of one run. Status strings on
// records are validated against this vocabulary before any journal write.
type RunState string

const (
	// RunQueued admits the run; no effect has dispatched.
	RunQueued RunState = "queued"
	// RunRunning means the turn executor owns the run context.
	RunRunning RunState = "running"
	// RunAwaitingInteraction pauses the run at a decision boundary: the turn
	// completed its graph and an interaction frame awaits an answer.
	RunAwaitingInteraction RunState = "awaiting_interaction"
	// RunCancelRequested records a protective stop signal delivery. It is
	// written even when the journal is slow: see Journal.RequestCancel.
	RunCancelRequested RunState = "cancel_requested"
	// RunCompleted is a successful terminal state.
	RunCompleted RunState = "completed"
	// RunFailed is a failure terminal state with a recorded reason.
	RunFailed RunState = "failed"
	// RunCanceled is a terminal state reached after a requested stop.
	RunCanceled RunState = "canceled"
	// RunInterrupted marks a run found nonterminal after a crash or restart.
	// It is terminal: resumption is a new explicitly admitted operation.
	RunInterrupted RunState = "interrupted"
)

// Terminal reports whether the state ends the run. Terminal states never
// reopen; a replacement run carries a new ID.
func (s RunState) Terminal() bool {
	switch s {
	case RunCompleted, RunFailed, RunCanceled, RunInterrupted:
		return true
	}
	return false
}

// ParseRunState validates a stored status string against the vocabulary.
func ParseRunState(raw string) (RunState, error) {
	state := RunState(raw)
	switch state {
	case RunQueued, RunRunning, RunAwaitingInteraction, RunCancelRequested,
		RunCompleted, RunFailed, RunCanceled, RunInterrupted:
		return state, nil
	}
	return "", fmt.Errorf("agentlifecycle: unknown run state %q", raw)
}

// ErrIllegalTransition rejects a journal write whose from→to edge the state
// machine does not define. The journal never rewrites history to make a late
// async result fit.
var ErrIllegalTransition = errors.New("agentlifecycle: illegal run transition")

// ErrStaleCursor rejects a transition whose expected cursor no longer matches:
// another cause moved the run first.
var ErrStaleCursor = errors.New("agentlifecycle: stale run cursor")

// ErrCauseReapplied is never an error for callers: a repeated cause ID returns
// the recorded result. The journal exports it so adapters can classify.
var ErrCauseReapplied = errors.New("agentlifecycle: cause already applied")

// legalTransitions is the §8.2 state machine. Every journal transition MUST be
// an edge in this map; anything else is rejected before persistence.
var legalTransitions = map[RunState][]RunState{
	RunQueued:              {RunRunning, RunCancelRequested, RunCanceled, RunInterrupted},
	RunRunning:             {RunAwaitingInteraction, RunCancelRequested, RunCompleted, RunFailed, RunCanceled, RunInterrupted},
	RunAwaitingInteraction: {RunRunning, RunCancelRequested, RunCanceled, RunInterrupted},
	RunCancelRequested:     {RunCanceled, RunCompleted, RunFailed, RunInterrupted},
	// Terminal states have no outgoing edges.
}

// LegalTransition reports whether from→to is a defined journal edge.
func LegalTransition(from, to RunState) bool {
	if from == to {
		return false
	}
	for _, next := range legalTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}
