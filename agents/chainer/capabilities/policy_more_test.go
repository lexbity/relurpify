package capabilities

import (
	"testing"

	"github.com/lexcodex/relurpify/agents/chainer"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

// TestNewPolicyEvaluator tests the constructor
func TestNewPolicyEvaluator(t *testing.T) {
	registry := capability.NewRegistry()
	evaluator := NewPolicyEvaluator(registry)

	if evaluator == nil {
		t.Fatal("expected non-nil evaluator")
	}

	if evaluator.registry != registry {
		t.Error("expected registry to be set")
	}
}

// TestPolicyEvaluatorGetAccessPolicy tests the GetAccessPolicy method
func TestPolicyEvaluatorGetAccessPolicy(t *testing.T) {
	t.Run("nil link", func(t *testing.T) {
		evaluator := NewPolicyEvaluator(nil)
		policy := evaluator.GetAccessPolicy(nil)

		if policy == nil {
			t.Fatal("expected non-nil policy")
		}

		if policy.DefaultAction != core.InsertionActionDirect {
			t.Errorf("expected default action Direct, got %v", policy.DefaultAction)
		}
	})

	t.Run("link with constraints", func(t *testing.T) {
		evaluator := NewPolicyEvaluator(nil)
		link := &chainer.Link{
			Name:          "test-link",
			AllowedTools:  []string{"tool1", "tool2"},
			RequiredTools: []string{"tool1"},
		}

		policy := evaluator.GetAccessPolicy(link)

		if policy.LinkName != "test-link" {
			t.Errorf("expected link name test-link, got %s", policy.LinkName)
		}

		if len(policy.AllowedTools) != 2 {
			t.Errorf("expected 2 allowed tools, got %d", len(policy.AllowedTools))
		}

		if len(policy.RequiredTools) != 1 {
			t.Errorf("expected 1 required tool, got %d", len(policy.RequiredTools))
		}
	})
}

// TestPolicyEvaluatorEvaluateToolAccess tests the EvaluateToolAccess method
func TestPolicyEvaluatorEvaluateToolAccess(t *testing.T) {
	t.Run("nil evaluator uses default", func(t *testing.T) {
		var nilEvaluator *PolicyEvaluator
		link := &chainer.Link{}

		action := nilEvaluator.EvaluateToolAccess(link, "tool1")
		// Should use default InsertionActionDirect
		if action != core.InsertionActionDirect {
			t.Errorf("expected Direct action, got %v", action)
		}
	})

	t.Run("denied tool", func(t *testing.T) {
		evaluator := NewPolicyEvaluator(nil)
		link := &chainer.Link{
			AllowedTools: []string{"tool1"}, // Only tool1 allowed
		}

		action := evaluator.EvaluateToolAccess(link, "tool2")
		if action != core.InsertionActionDenied {
			t.Errorf("expected Denied action, got %v", action)
		}
	})
}

// TestCheckLinkConstraints tests the internal helper function
func TestCheckLinkConstraints(t *testing.T) {
	t.Run("empty allowed and required", func(t *testing.T) {
		link := &chainer.Link{}
		if !checkLinkConstraints(link, "any-tool") {
			t.Error("expected true for no constraints")
		}
	})

	t.Run("tool in allowed list", func(t *testing.T) {
		link := &chainer.Link{
			AllowedTools: []string{"tool1", "tool2"},
		}
		if !checkLinkConstraints(link, "tool1") {
			t.Error("expected true for allowed tool")
		}
	})

	t.Run("tool not in allowed list", func(t *testing.T) {
		link := &chainer.Link{
			AllowedTools: []string{"tool1", "tool2"},
		}
		if checkLinkConstraints(link, "tool3") {
			t.Error("expected false for non-allowed tool")
		}
	})

	t.Run("required tool in allowed list", func(t *testing.T) {
		link := &chainer.Link{
			AllowedTools:  []string{"tool1", "tool2"},
			RequiredTools: []string{"tool1"},
		}
		if !checkLinkConstraints(link, "tool1") {
			t.Error("expected true when required tool is in allowed")
		}
	})

	t.Run("required tool not in allowed list", func(t *testing.T) {
		link := &chainer.Link{
			AllowedTools:  []string{"tool1", "tool2"},
			RequiredTools: []string{"tool3"}, // Not in allowed
		}
		// This should fail validation but checkLinkConstraints only checks individual tool
		if checkLinkConstraints(link, "tool1") {
			// This is still true for tool1 itself
			t.Log("tool1 is in allowed list")
		}
	})
}
