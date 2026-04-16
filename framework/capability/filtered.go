package capability

import (
	"context"
	"fmt"
	"sort"

	"github.com/lexcodex/relurpify/framework/core"
)

// FilteredRegistry wraps a Registry and restricts visible capabilities to a
// declared allowed set. An empty allowed set means pass-through (all capabilities
// visible). This is the mechanism through which thought recipe capability scoping
// is enforced — the LLM can only call tools it can see.
type FilteredRegistry struct {
	base    *CapabilityRegistry
	allowed map[string]struct{} // nil = allow all
}

// NewFilteredRegistry builds a filtered view. allowedIDs nil or empty = pass-through.
func NewFilteredRegistry(base *CapabilityRegistry, allowedIDs []string) *FilteredRegistry {
	f := &FilteredRegistry{
		base: base,
	}

	// Only create allowed map if there are specific restrictions
	if len(allowedIDs) > 0 {
		f.allowed = make(map[string]struct{}, len(allowedIDs))
		for _, id := range allowedIDs {
			if id != "" {
				f.allowed[id] = struct{}{}
			}
		}
		// If all IDs were empty, treat as pass-through
		if len(f.allowed) == 0 {
			f.allowed = nil
		}
	}

	return f
}

// IsAllowed reports whether the given capability ID is in the allowed set.
// When in passthrough mode (allowed == nil), all capabilities are allowed.
func (f *FilteredRegistry) IsAllowed(id string) bool {
	if f == nil {
		return true
	}
	if f.allowed == nil {
		return true
	}
	_, ok := f.allowed[id]
	return ok
}

// Intersect returns a new FilteredRegistry further restricted to the intersection
// of the current allowed set and the provided list. Used to apply step-level
// restrictions on top of global restrictions.
func (f *FilteredRegistry) Intersect(allowedIDs []string) *FilteredRegistry {
	if f == nil {
		return NewFilteredRegistry(nil, allowedIDs)
	}

	// If current is passthrough, just use the new restriction
	if f.IsPassthrough() {
		return NewFilteredRegistry(f.base, allowedIDs)
	}

	// If new restriction is empty, keep current restriction
	if len(allowedIDs) == 0 {
		return NewFilteredRegistry(f.base, f.AllowedIDs())
	}

	// Compute intersection
	intersection := make([]string, 0, len(allowedIDs))
	for _, id := range allowedIDs {
		if f.IsAllowed(id) {
			intersection = append(intersection, id)
		}
	}

	return NewFilteredRegistry(f.base, intersection)
}

// IsPassthrough returns true when no filtering is applied.
func (f *FilteredRegistry) IsPassthrough() bool {
	return f == nil || f.allowed == nil
}

// AllowedIDs returns the current allowed ID set. Nil means all allowed.
func (f *FilteredRegistry) AllowedIDs() []string {
	if f == nil || f.allowed == nil {
		return nil
	}
	ids := make([]string, 0, len(f.allowed))
	for id := range f.allowed {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Get returns a tool by name if it is allowed.
func (f *FilteredRegistry) Get(name string) (Tool, bool) {
	if f == nil || f.base == nil {
		return nil, false
	}

	tool, ok := f.base.Get(name)
	if !ok {
		return nil, false
	}

	// Check if the tool is allowed by looking up its capability descriptor
	if desc, ok := f.base.GetCapability(name); ok {
		if !f.IsAllowed(desc.ID) {
			return nil, false
		}
	}

	return tool, true
}

// ModelCallableTools returns only the allowed callable tools.
func (f *FilteredRegistry) ModelCallableTools() []Tool {
	if f == nil || f.base == nil {
		return nil
	}

	allTools := f.base.ModelCallableTools()
	if f.IsPassthrough() {
		return allTools
	}

	filtered := make([]Tool, 0, len(allTools))
	for _, tool := range allTools {
		if desc, ok := f.base.GetCapability(tool.Name()); ok {
			if f.IsAllowed(desc.ID) {
				filtered = append(filtered, tool)
			}
		}
	}

	return filtered
}

// InvokeCapability executes an invocable capability by ID if it is allowed.
func (f *FilteredRegistry) InvokeCapability(ctx context.Context, state *core.Context, name string, args map[string]any) (*core.ToolResult, error) {
	if f == nil || f.base == nil {
		return nil, fmt.Errorf("registry unavailable")
	}

	// Resolve the capability ID from name
	desc, ok := f.base.GetCapability(name)
	if !ok {
		return nil, fmt.Errorf("capability %s not found", name)
	}

	// Check if allowed
	if !f.IsAllowed(desc.ID) {
		return nil, fmt.Errorf("capability %s is not allowed", name)
	}

	return f.base.InvokeCapability(ctx, state, name, args)
}
