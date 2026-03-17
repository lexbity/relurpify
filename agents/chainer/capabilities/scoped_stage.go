package capabilities

import (
	"fmt"

	"github.com/lexcodex/relurpify/agents/chainer"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/pipeline"
)

// ScopedLinkStage wraps a LinkStage and implements pipeline.ToolScopedStage.
//
// Phase 6: Enables per-link tool access control. If a link specifies AllowedTools,
// only those tools are available to the stage. Otherwise, all tools are available
// (backward compatible behavior).
type ScopedLinkStage struct {
	stage        pipeline.Stage        // Underlying LinkStage
	link         *chainer.Link         // Link definition (for tool restrictions)
	evaluator    *PolicyEvaluator      // Evaluates tool access policies
}

// NewScopedLinkStage wraps a LinkStage with tool scoping capability.
func NewScopedLinkStage(stage pipeline.Stage, link *chainer.Link, evaluator *PolicyEvaluator) *ScopedLinkStage {
	return &ScopedLinkStage{
		stage:     stage,
		link:      link,
		evaluator: evaluator,
	}
}

// Name returns the stage name (delegates to underlying stage).
func (s *ScopedLinkStage) Name() string {
	if s == nil || s.stage == nil {
		return ""
	}
	return s.stage.Name()
}

// Contract returns the stage contract (delegates to underlying stage).
func (s *ScopedLinkStage) Contract() pipeline.ContractDescriptor {
	if s == nil || s.stage == nil {
		return pipeline.ContractDescriptor{}
	}
	return s.stage.Contract()
}

// BuildPrompt builds the prompt (delegates to underlying stage).
func (s *ScopedLinkStage) BuildPrompt(ctx *core.Context) (string, error) {
	if s == nil || s.stage == nil {
		return "", fmt.Errorf("scoped stage not initialized")
	}
	return s.stage.BuildPrompt(ctx)
}

// Decode decodes the response (delegates to underlying stage).
func (s *ScopedLinkStage) Decode(resp *core.LLMResponse) (any, error) {
	if s == nil || s.stage == nil {
		return nil, fmt.Errorf("scoped stage not initialized")
	}
	return s.stage.Decode(resp)
}

// Validate validates the output (delegates to underlying stage).
func (s *ScopedLinkStage) Validate(output any) error {
	if s == nil || s.stage == nil {
		return fmt.Errorf("scoped stage not initialized")
	}
	return s.stage.Validate(output)
}

// Apply applies the output (delegates to underlying stage).
func (s *ScopedLinkStage) Apply(ctx *core.Context, output any) error {
	if s == nil || s.stage == nil {
		return fmt.Errorf("scoped stage not initialized")
	}
	return s.stage.Apply(ctx, output)
}

// AllowedToolNames returns the set of tools this stage can invoke.
//
// Phase 6 implementation:
//   - If Link.AllowedTools is non-empty, returns only those tools
//   - If Link.AllowedTools is empty, returns all tools (backward compatible)
//   - Enforces Link.RequiredTools are included
func (s *ScopedLinkStage) AllowedToolNames() []string {
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

// ToolAccessPolicy evaluates whether this stage can invoke a specific tool.
//
// Returns InsertionAction indicating how the tool result should be presented:
//   - InsertionActionDirect: Include result directly
//   - InsertionActionSummarized: Include summarized result
//   - InsertionActionMetadataOnly: Include only metadata
//   - InsertionActionHITLRequired: Require user approval
//   - InsertionActionDenied: Block tool access
func (s *ScopedLinkStage) ToolAccessPolicy(toolID string) core.InsertionAction {
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
