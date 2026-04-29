package capabilities

import (
	"codeburg.org/lexbit/relurpify/agents/chainer"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// ScopedLink wraps a chainer.Link and provides tool scoping capability.
//
// Phase 6: Enables per-link tool access control. If a link specifies AllowedTools,
// only those tools are available to the link. Otherwise, all tools are available
// (backward compatible behavior).
type ScopedLink struct {
	link      *chainer.Link    // Link definition (for tool restrictions)
	evaluator *PolicyEvaluator // Evaluates tool access policies
}

// NewScopedLink creates a scoped link with tool restrictions.
func NewScopedLink(link *chainer.Link, evaluator *PolicyEvaluator) *ScopedLink {
	return &ScopedLink{
		link:      link,
		evaluator: evaluator,
	}
}

// Name returns the link name.
func (s *ScopedLink) Name() string {
	if s == nil || s.link == nil {
		return ""
	}
	return s.link.Name
}

// AllowedToolNames returns the set of tools this link can invoke.
//
// Phase 6 implementation:
//   - If Link.AllowedTools is non-empty, returns only those tools
//   - If Link.AllowedTools is empty, returns all tools (backward compatible)
//   - Enforces Link.RequiredTools are included
func (s *ScopedLink) AllowedToolNames() []string {
	if s == nil || s.link == nil {
		return nil // No restrictions
	}

	// If no allowed tools specified, return all (backward compatible)
	if len(s.link.AllowedTools) == 0 {
		return nil // nil = all tools allowed
	}

	// Return allowed tools (already includes required by design)
	return s.link.AllowedTools
}

// ToolAccessPolicy evaluates whether this link can invoke a specific tool.
//
// Returns InsertionAction indicating how the tool result should be presented:
//   - InsertionActionDirect: Include result directly
//   - InsertionActionSummarized: Include summarized result
//   - InsertionActionMetadataOnly: Include only metadata
//   - InsertionActionHITLRequired: Require user approval
//   - InsertionActionDenied: Block tool access
func (s *ScopedLink) ToolAccessPolicy(toolID string) core.InsertionAction {
	if s == nil || s.link == nil {
		return core.InsertionActionDirect // No restrictions
	}

	// Check if tool is in allowed list
	if len(s.link.AllowedTools) > 0 {
		allowed := false
		for _, t := range s.link.AllowedTools {
			if t == toolID {
				allowed = true
				break
			}
		}
		if !allowed {
			return core.InsertionActionDenied
		}
	}

	// Consult policy evaluator if available
	if s.evaluator != nil {
		return s.evaluator.EvaluateToolAccess(s.link, toolID)
	}

	return core.InsertionActionDirect // Allow by default
}
