package contextdata

func (e *Envelope) getWorkingValue(key string) (any, bool) {
	if e.WorkingData == nil {
		return nil, false
	}
	val, ok := e.WorkingData[key]
	return val, ok
}

// SetExecutionPhase sets the current execution phase.
func (e *Envelope) SetExecutionPhase(phase string) {
	e.SetWorkingValueWithClass("_execution_phase", phase, MemoryClassTask)
}

// GetExecutionPhase returns the current execution phase.
func (e *Envelope) GetExecutionPhase() string {
	val, _ := e.getWorkingValue("_execution_phase")
	if s, ok := val.(string); ok {
		return s
	}
	return ""
}

// AddInteraction adds an interaction record to the envelope.
func (e *Envelope) AddInteraction(interaction map[string]any) {
	key := "_interactions"
	var interactions []map[string]any
	if val, ok := e.getWorkingValue(key); ok {
		if arr, ok := val.([]map[string]any); ok {
			interactions = arr
		}
	}
	interactions = append(interactions, interaction)
	e.SetWorkingValueWithClass(key, interactions, MemoryClassTask)
}

// GetInteractions returns all interactions recorded in the envelope.
func (e *Envelope) GetInteractions() []map[string]any {
	val, _ := e.getWorkingValue("_interactions")
	if arr, ok := val.([]map[string]any); ok {
		return arr
	}
	return nil
}
