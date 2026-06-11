package registry

import (
	"context"

	"codeburg.org/lexbit/relurpify/capability/handler"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/model"
)

// ModelCallableTools returns callable local tools for agent-internal use such
// as phase filtering and budget enforcement. Only local ports.Tool implementations
// are included; non-local capabilities appear in ModelCallableLLMToolSpecs.
func (r *CapabilityRegistry) ModelCallableTools(ctx context.Context) []ports.Tool {
	if r == nil {
		return nil
	}
	if r.delegate != nil {
		all := r.delegate.ModelCallableTools(ctx)
		if r.toolIDAllowlist == nil {
			return all
		}
		filtered := make([]ports.Tool, 0, len(all))
		for _, t := range all {
			if desc, ok := r.delegate.GetCapability(t.Name()); ok {
				if r.isAllowlisted(desc.ID) {
					filtered = append(filtered, t)
				}
			}
		}
		return filtered
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	entries := r.localToolEntriesLocked()
	res := make([]ports.Tool, 0, len(entries))
	for _, entry := range entries {
		if r.effectiveExposureLocked(entry.descriptor) != agentspec.CapabilityExposureCallable {
			continue
		}
		if !toolAvailableForPrompt(ctx, entry.legacyTool) {
			continue
		}
		res = append(res, entry.legacyTool)
	}
	return res
}

// ModelCallableLLMToolSpecs returns the provider-agnostic tool specs for all
// callable capabilities: local tools and non-local invocable capabilities
// (provider-backed, Relurpic). This is what callers should pass to
// LanguageModel.ChatWithTools — Ollama-specific formatting is handled in
// platform/llm, not here.
func (r *CapabilityRegistry) ModelCallableLLMToolSpecs(ctx context.Context) []model.LLMToolSpec {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]model.LLMToolSpec, 0, len(r.entries))
	for _, entry := range r.entries {
		if entry == nil {
			continue
		}
		if r.effectiveExposureLocked(entry.descriptor) != agentspec.CapabilityExposureCallable {
			continue
		}
		if entry.legacyTool != nil {
			if !toolAvailableForPrompt(ctx, entry.legacyTool) {
				continue
			}
			res = append(res, ports.LLMToolSpecFromTool(unwrapTool(entry.legacyTool)))
		} else if _, ok := entry.handler.(handler.InvocableCapabilityHandler); ok {
			res = append(res, LLMToolSpecFromDescriptor(entry.descriptor))
		}
	}
	return res
}

func toolAvailableForPrompt(ctx context.Context, tool ports.Tool) bool {
	if tool == nil {
		return false
	}
	return tool.IsAvailable(ctx)
}

// GetModelTool resolves a callable local tool by name for post-LLM dispatch.
func (r *CapabilityRegistry) GetModelTool(name string) (ports.Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	name = r.normalizeToolNameLocked(name)
	entry, ok := r.localToolEntryByNameLocked(name)
	if !ok || entry == nil || entry.legacyTool == nil {
		return nil, false
	}
	if r.effectiveExposureLocked(entry.descriptor) != agentspec.CapabilityExposureCallable {
		return nil, false
	}
	return entry.legacyTool, true
}
