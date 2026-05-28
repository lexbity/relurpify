package intentcontext

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
)

// StateStore reads and writes task-scoped clarification state from an envelope.
type StateStore interface {
	Read(ctx context.Context, env *contextdata.Envelope) (*ClarificationState, error)
	Write(ctx context.Context, env *contextdata.Envelope, state *ClarificationState) error
}

// EnvelopeStateStore is the envelope-backed StateStore implementation.
type EnvelopeStateStore struct{}

// NewStateStore returns a new envelope-backed clarification state store.
func NewStateStore() *EnvelopeStateStore {
	return &EnvelopeStateStore{}
}

// Read extracts the canonical clarification state from the envelope.
func (s *EnvelopeStateStore) Read(ctx context.Context, env *contextdata.Envelope) (*ClarificationState, error) {
	_ = ctx
	if s == nil {
		s = &EnvelopeStateStore{}
	}
	value, ok := env.GetWorkingValue(ClarificationStateKey)
	if !ok || value == nil {
		return NewState(env.TaskID, env.SessionID), nil
	}
	state, ok := value.(*ClarificationState)
	if !ok {
		return nil, fmt.Errorf("clarification state read: expected %T, got %T", (*ClarificationState)(nil), value)
	}
	clone := state.Clone()
	clone.Normalize()
	if err := clone.Validate(); err != nil {
		return nil, err
	}
	return clone, nil
}

// Write stores the canonical clarification state and its derived working-memory keys.
func (s *EnvelopeStateStore) Write(ctx context.Context, env *contextdata.Envelope, state *ClarificationState) error {
	_ = ctx
	if s == nil {
		s = &EnvelopeStateStore{}
	}
	if state == nil {
		return fmt.Errorf("clarification state write: nil state")
	}
	if state.StateVersion == 0 {
		return fmt.Errorf("clarification state write: missing state version")
	}
	if strings.TrimSpace(state.TaskID) == "" {
		state.TaskID = env.TaskID
	}
	if strings.TrimSpace(state.SessionID) == "" {
		state.SessionID = env.SessionID
	}
	if state.TaskID != env.TaskID {
		return fmt.Errorf("clarification state write: task mismatch %q != %q", state.TaskID, env.TaskID)
	}
	if state.SessionID != env.SessionID {
		return fmt.Errorf("clarification state write: session mismatch %q != %q", state.SessionID, env.SessionID)
	}

	current, err := s.readCurrent(env)
	if err != nil {
		return err
	}
	if current != nil && state.StateVersion < current.StateVersion {
		return fmt.Errorf("clarification state write: version regression %d < %d", state.StateVersion, current.StateVersion)
	}

	clone := state.Clone()
	clone.Normalize()
	if err := clone.Validate(); err != nil {
		return err
	}
	clone.LastUpdatedAt = time.Now().UTC()

	for _, key := range ClarificationWorkingMemoryKeys() {
		env.DeleteWorkingValue(key)
	}
	env.SetWorkingValue(ClarificationStateKey, clone, contextdata.MemoryClassTask)
	env.SetWorkingValue(ClarificationAmbiguityKey, clone.Ambiguity, contextdata.MemoryClassTask)
	env.SetWorkingValue(ClarificationTurnsKey, append([]ClarificationTurn(nil), clone.Turns...), contextdata.MemoryClassTask)
	env.SetWorkingValue(ClarificationConfirmedEntitiesKey, append([]ConfirmedEntity(nil), clone.ConfirmedEntities...), contextdata.MemoryClassTask)
	env.SetWorkingValue(ClarificationConfirmedScopesKey, append([]ConfirmedScope(nil), clone.ConfirmedScopes...), contextdata.MemoryClassTask)
	env.SetWorkingValue(ClarificationRelationIntentsKey, append([]RelationIntent(nil), clone.PendingRelationIntents...), contextdata.MemoryClassTask)
	env.SetWorkingValue(ClarificationGroundedAnchorsKey, append([]retrieval.AnchorRef(nil), clone.GroundedAnchors...), contextdata.MemoryClassTask)
	env.SetWorkingValue(ClarificationPendingProjectionKey, append([]ProjectionIntent(nil), clone.PendingProjection...), contextdata.MemoryClassTask)
	env.SetWorkingValue(ClarificationProjectedMutationsKey, append([]ProjectionRecord(nil), clone.AppliedMutations...), contextdata.MemoryClassTask)
	env.SetWorkingValue(ClarificationActiveThoughtRecipeKey, clone.ActiveThoughtRecipeID, contextdata.MemoryClassTask)
	env.SetWorkingValue(ClarificationLastCheckpointIDKey, clone.LastCheckpointID, contextdata.MemoryClassTask)
	env.SetWorkingValue(ClarificationLastCheckpointSeqKey, clone.LastCheckpointSeq, contextdata.MemoryClassTask)
	return nil
}

func (s *EnvelopeStateStore) readCurrent(env *contextdata.Envelope) (*ClarificationState, error) {
	value, ok := env.GetWorkingValue(ClarificationStateKey)
	if !ok || value == nil {
		return nil, nil
	}
	state, ok := value.(*ClarificationState)
	if !ok {
		return nil, fmt.Errorf("clarification state read: expected %T, got %T", (*ClarificationState)(nil), value)
	}
	return state, nil
}
