package registry

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/capability/handler"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

// RenderPrompt executes a runtime-backed prompt capability by capability ID or public name.
func (r *CapabilityRegistry) RenderPrompt(ctx context.Context, state ports.State, idOrName string, args map[string]any) (*handler.PromptRenderResult, error) {
	if r == nil {
		return nil, fmt.Errorf("registry unavailable")
	}
	entry, err := r.prepareCapabilityInvocation(ctx, state, idOrName, args)
	if err != nil {
		return nil, err
	}
	promptHandler, ok := entry.handler.(handler.PromptCapabilityHandler)
	if !ok {
		return nil, fmt.Errorf("capability %s is not a prompt handler", entry.descriptor.ID)
	}
	return promptHandler.RenderPrompt(ctx, state, args)
}
