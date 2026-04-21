package dispatch

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/named/euclo/execution"
)

// InvocableRegistry holds a unified map of all invocable capabilities.
// It replaces the separate behaviors and routines maps in the Dispatcher.
type InvocableRegistry struct {
	entries map[string]execution.Invocable
}

// newInvocableRegistry creates a new empty registry.
func newInvocableRegistry() *InvocableRegistry {
	return &InvocableRegistry{
		entries: make(map[string]execution.Invocable),
	}
}

// Register adds an invocable to the registry.
// Returns an error if the ID is empty or if an invocable with the same ID
// is already registered.
func (r *InvocableRegistry) Register(inv execution.Invocable) error {
	if r == nil {
		return fmt.Errorf("cannot register to nil registry")
	}
	if inv == nil {
		return fmt.Errorf("cannot register nil invocable")
	}
	id := strings.TrimSpace(inv.ID())
	if id == "" {
		return fmt.Errorf("cannot register invocable with empty ID")
	}
	if _, exists := r.entries[id]; exists {
		return fmt.Errorf("invocable %q already registered", id)
	}
	r.entries[id] = inv
	return nil
}

// Lookup retrieves an invocable by ID, regardless of its primary/supporting status.
// Returns the invocable and true if found, nil and false otherwise.
func (r *InvocableRegistry) Lookup(id string) (execution.Invocable, bool) {
	if r == nil || r.entries == nil {
		return nil, false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, false
	}
	inv, ok := r.entries[id]
	return inv, ok
}

// Primary returns an invocable by ID only if it is a primary invocable (IsPrimary() == true).
// Returns the invocable and true if found and primary, nil and false otherwise.
func (r *InvocableRegistry) Primary(id string) (execution.Invocable, bool) {
	inv, ok := r.Lookup(id)
	if !ok {
		return nil, false
	}
	if !inv.IsPrimary() {
		return nil, false
	}
	return inv, true
}

// Supporting returns an invocable by ID only if it is a supporting invocable (IsPrimary() == false).
// Returns the invocable and true if found and supporting, nil and false otherwise.
func (r *InvocableRegistry) Supporting(id string) (execution.Invocable, bool) {
	inv, ok := r.Lookup(id)
	if !ok {
		return nil, false
	}
	if inv.IsPrimary() {
		return nil, false
	}
	return inv, true
}

// All returns a slice of all registered invocables.
func (r *InvocableRegistry) All() []execution.Invocable {
	if r == nil || r.entries == nil {
		return nil
	}
	result := make([]execution.Invocable, 0, len(r.entries))
	for _, inv := range r.entries {
		result = append(result, inv)
	}
	return result
}

// Count returns the number of registered invocables.
func (r *InvocableRegistry) Count() int {
	if r == nil || r.entries == nil {
		return 0
	}
	return len(r.entries)
}

// Deregister removes an invocable from the registry by ID.
// Returns true if the invocable was found and removed, false otherwise.
// This enables hot-reload scenarios where recipes can be unregistered.
func (r *InvocableRegistry) Deregister(id string) bool {
	if r == nil || r.entries == nil {
		return false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	if _, exists := r.entries[id]; !exists {
		return false
	}
	delete(r.entries, id)
	return true
}

// Replace overwrites an existing invocable with a new one.
// Returns an error if the invocable ID is not already registered.
// This enables hot-reload scenarios where recipes can be updated without
// re-initializing the full agent.
func (r *InvocableRegistry) Replace(inv execution.Invocable) error {
	if r == nil {
		return fmt.Errorf("cannot replace in nil registry")
	}
	if inv == nil {
		return fmt.Errorf("cannot replace with nil invocable")
	}
	id := strings.TrimSpace(inv.ID())
	if id == "" {
		return fmt.Errorf("cannot replace with invocable having empty ID")
	}
	if _, exists := r.entries[id]; !exists {
		return fmt.Errorf("invocable %q not registered, cannot replace", id)
	}
	r.entries[id] = inv
	return nil
}
