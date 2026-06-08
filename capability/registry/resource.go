package registry

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/capability/handler"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

// ReadResource executes a runtime-backed resource capability by capability ID or public name.
func (r *CapabilityRegistry) ReadResource(ctx context.Context, state ports.State, idOrName string) (*handler.ResourceReadResult, error) {
	if r == nil {
		return nil, fmt.Errorf("registry unavailable")
	}
	entry, err := r.prepareCapabilityInvocation(ctx, state, idOrName, nil)
	if err != nil {
		return nil, err
	}
	resourceHandler, ok := entry.handler.(handler.ResourceCapabilityHandler)
	if !ok {
		return nil, fmt.Errorf("capability %s is not a resource handler", entry.descriptor.ID)
	}
	return resourceHandler.ReadResource(ctx, state)
}
