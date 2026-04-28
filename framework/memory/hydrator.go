package memory

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
)

// Hydrate implements the StateHydrator interface.
// Takes memory retrieval results and writes them to agentgraph Context via key mapping.
func (s *WorkingMemoryStore) Hydrate(ctx context.Context, state map[string]any, results []MemoryRecordEnvelope) error {
	if state == nil {
		return fmt.Errorf("state is nil")
	}

	for _, result := range results {
		key := result.Key
		value := result.Entry.Value

		// Write to state using the key
		state[key] = value
	}

	return nil
}

// HydrateIntoEnvelope hydrates memory results into a contextdata.Envelope.
// This is the preferred method for the new tiered context model.
func (s *WorkingMemoryStore) HydrateIntoEnvelope(ctx context.Context, env *contextdata.Envelope, results []MemoryRecordEnvelope) error {
	if env == nil {
		return fmt.Errorf("envelope is nil")
	}

	for _, result := range results {
		key := result.Key
		value := result.Entry.Value
		class := contextdata.MemoryClass(result.Entry.Class)

		// Write to envelope working memory
		env.SetWorkingValue(key, value, class)
	}

	return nil
}

// HydrateWithMapping hydrates state with a custom key mapping function.
func (s *WorkingMemoryStore) HydrateWithMapping(ctx context.Context, state map[string]any, results []MemoryRecordEnvelope, keyMapper func(string) string) error {
	if state == nil {
		return fmt.Errorf("state is nil")
	}

	if keyMapper == nil {
		// Use default identity mapping
		keyMapper = func(k string) string { return k }
	}

	for _, result := range results {
		key := keyMapper(result.Key)
		value := result.Entry.Value

		state[key] = value
	}

	return nil
}

// HydrateIntoEnvelopeWithMapping hydrates an envelope with custom key mapping.
func (s *WorkingMemoryStore) HydrateIntoEnvelopeWithMapping(ctx context.Context, env *contextdata.Envelope, results []MemoryRecordEnvelope, keyMapper func(string) string) error {
	if env == nil {
		return fmt.Errorf("envelope is nil")
	}

	if keyMapper == nil {
		keyMapper = func(k string) string { return k }
	}

	for _, result := range results {
		key := keyMapper(result.Key)
		value := result.Entry.Value
		class := contextdata.MemoryClass(result.Entry.Class)

		env.SetWorkingValue(key, value, class)
	}

	return nil
}

// SimpleStateHydrator is a simple implementation of StateHydrator.
type SimpleStateHydrator struct {
	KeyPrefix string
}

// Hydrate implements StateHydrator.
func (h *SimpleStateHydrator) Hydrate(ctx context.Context, state map[string]any, results []MemoryRecordEnvelope) error {
	if state == nil {
		return fmt.Errorf("state is nil")
	}

	for _, result := range results {
		key := h.KeyPrefix + result.Key
		value := result.Entry.Value

		state[key] = value
	}

	return nil
}

// HydrateIntoEnvelope implements envelope-based hydration.
func (h *SimpleStateHydrator) HydrateIntoEnvelope(ctx context.Context, env *contextdata.Envelope, results []MemoryRecordEnvelope) error {
	if env == nil {
		return fmt.Errorf("envelope is nil")
	}

	for _, result := range results {
		key := h.KeyPrefix + result.Key
		value := result.Entry.Value
		class := contextdata.MemoryClass(result.Entry.Class)

		env.SetWorkingValue(key, value, class)
	}

	return nil
}

// NewSimpleStateHydrator creates a new simple state hydrator.
func NewSimpleStateHydrator(keyPrefix string) *SimpleStateHydrator {
	return &SimpleStateHydrator{
		KeyPrefix: keyPrefix,
	}
}
