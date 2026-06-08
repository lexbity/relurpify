package contextdata

import "codeburg.org/lexbit/relurpify/capability/ports"

// EnvelopeState wraps Envelope to satisfy ports.State.
// The adapter is needed because Envelope has fields named TaskID and SessionID
// which conflict with the method names ports.State requires.
type EnvelopeState struct {
	env *Envelope
}

// NewEnvelopeState creates a State adapter for the given envelope.
func NewEnvelopeState(env *Envelope) ports.State {
	if env == nil {
		return &EnvelopeState{}
	}
	return &EnvelopeState{env: env}
}

func (s *EnvelopeState) GetWorkingValue(key string) (any, bool) {
	if s.env == nil {
		return nil, false
	}
	return s.env.GetWorkingValue(key)
}

func (s *EnvelopeState) SetWorkingValue(key string, value any) {
	if s.env == nil {
		return
	}
	s.env.SetWorkingValue(key, value)
}

func (s *EnvelopeState) DeleteWorkingValue(key string) {
	if s.env == nil {
		return
	}
	s.env.DeleteWorkingValue(key)
}

func (s *EnvelopeState) ClearWorkingData() {
	if s.env == nil {
		return
	}
	s.env.ClearWorkingData()
}

func (s *EnvelopeState) WorkingMemoryKeys() []string {
	if s.env == nil {
		return nil
	}
	return s.env.WorkingMemoryKeys()
}

func (s *EnvelopeState) Snapshot() map[string]any {
	if s.env == nil {
		return nil
	}
	return s.env.Snapshot()
}

func (s *EnvelopeState) TaskID() string {
	if s.env == nil {
		return ""
	}
	return s.env.TaskID
}

func (s *EnvelopeState) SessionID() string {
	if s.env == nil {
		return ""
	}
	return s.env.SessionID
}

// State wraps the envelope as a ports.State for passing to capability
// handler interfaces without importing capability package types here.
func (e *Envelope) State() ports.State {
	return NewEnvelopeState(e)
}

// EnvelopeFromState extracts the underlying Envelope from a ports.State
// created by Envelope.State(). Returns nil if the state was not created
// from an Envelope.
func EnvelopeFromState(s ports.State) *Envelope {
	if s == nil {
		return nil
	}
	if es, ok := s.(*EnvelopeState); ok {
		return es.env
	}
	return nil
}
