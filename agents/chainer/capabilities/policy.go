package capabilities

import (
	"fmt"

	"github.com/lexcodex/relurpify/agents/chainer"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

// PolicyEvaluator determines whether a link can invoke a tool based on
// registry policies, trust classes, and link configurations.
//
// Phase 6 stub: Basic evaluator that checks:
//   1. Tool presence in registry
//   2. Tool trust class (trusted vs untrusted)
//   3. Link AllowedTools/RequiredTools constraints
//
// Phase 6+ will add: user approval tracking, risk class evaluation, audit logging
type PolicyEvaluator struct {
	registry *capability.Registry
}

// NewPolicyEvaluator creates a policy evaluator with a tool registry.
func NewPolicyEvaluator(registry *capability.Registry) *PolicyEvaluator {
	return &PolicyEvaluator{
		registry: registry,
	}
}

// CanInvoke determines if a link can invoke a specific tool.
//
// Evaluation logic:
//   1. Check link AllowedTools whitelist (always enforced)
//   2. Check link RequiredTools constraints
//   3. Check tool exists in registry (if registry present)
//   4. Check tool trust class (if registry present)
//   5. Return decision
func (e *PolicyEvaluator) CanInvoke(link *chainer.Link, toolID string) bool {
	if link == nil {
		return true // No link constraints, allow by default
	}

	if e == nil {
		// No evaluator, but still check link constraints
		return checkLinkConstraints(link, toolID)
	}

	// Phase 6 stub: Basic checks
	// Future phases will add registry-based trust class evaluation

	// Check link constraints first (always enforced)
	if !checkLinkConstraints(link, toolID) {
		return false
	}

	// Registry checks (if available)
	if e.registry == nil {
		return true // No registry, basic checks passed
	}

	// Phase 6+: Would check registry for trust class, risk class, etc.
	return true
}

// checkLinkConstraints checks AllowedTools and RequiredTools constraints.
func checkLinkConstraints(link *chainer.Link, toolID string) bool {
	// Check if tool in allowed list
	if len(link.AllowedTools) > 0 {
		allowed := false
		for _, t := range link.AllowedTools {
			if t == toolID {
				allowed = true
				break
			}
		}
		if !allowed {
			return false // Tool not in allowed list
		}
	}

	// Check required tools are met (all must be in AllowedTools)
	if len(link.RequiredTools) > 0 && len(link.AllowedTools) > 0 {
		for _, required := range link.RequiredTools {
			found := false
			for _, allowed := range link.AllowedTools {
				if allowed == required {
					found = true
					break
				}
			}
			if !found {
				return false // Required tool not in allowed list
			}
		}
	}

	return true
}

// EvaluateToolAccess returns the insertion action for a tool result.
//
// Returns how the tool result should be presented:
//   - InsertionActionDirect: Include result as-is
//   - InsertionActionSummarized: Summarize result before inclusion
//   - InsertionActionMetadataOnly: Include only metadata (no content)
//   - InsertionActionHITLRequired: Require human approval
//   - InsertionActionDenied: Block tool completely
func (e *PolicyEvaluator) EvaluateToolAccess(link *chainer.Link, toolID string) core.InsertionAction {
	// Ensure we have an evaluator to use CanInvoke
	evaluator := e
	if evaluator == nil {
		evaluator = NewPolicyEvaluator(nil)
	}

	// Check if tool is allowed
	if !evaluator.CanInvoke(link, toolID) {
		return core.InsertionActionDenied
	}

	// Phase 6: Default to direct inclusion
	// Phase 6+ will consider:
	// - Tool trust class (untrusted → summarized)
	// - Tool risk class (high-risk → HITL required)
	// - Link data sensitivity

	return core.InsertionActionDirect
}

// ValidateRequiredTools checks that all required tools are available.
//
// Returns error if any required tool is missing or blocked.
func (e *PolicyEvaluator) ValidateRequiredTools(link *chainer.Link) error {
	if link == nil {
		return nil
	}

	if len(link.RequiredTools) == 0 {
		return nil
	}

	// Ensure we have an evaluator to use CanInvoke
	evaluator := e
	if evaluator == nil {
		evaluator = NewPolicyEvaluator(nil)
	}

	for _, required := range link.RequiredTools {
		if !evaluator.CanInvoke(link, required) {
			return fmt.Errorf("chainer: required tool %q not available for link %q", required, link.Name)
		}
	}

	return nil
}

// ToolAccessPolicy returns a summary of which tools are accessible.
type ToolAccessPolicy struct {
	LinkName      string
	AllowedTools  []string // Allowed tools (nil = all)
	RequiredTools []string // Required tools
	DefaultAction core.InsertionAction
}

// GetAccessPolicy returns the tool access policy for a link.
func (e *PolicyEvaluator) GetAccessPolicy(link *chainer.Link) *ToolAccessPolicy {
	if link == nil {
		return &ToolAccessPolicy{
			DefaultAction: core.InsertionActionDirect,
		}
	}

	return &ToolAccessPolicy{
		LinkName:      link.Name,
		AllowedTools:  link.AllowedTools,
		RequiredTools: link.RequiredTools,
		DefaultAction: core.InsertionActionDirect, // Phase 6 default
	}
}
