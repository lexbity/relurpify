package contextmgr

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestNewContextPolicyKeepsProgressiveEnabledWhenUnset(t *testing.T) {
	policy := NewContextPolicy(ContextPolicyConfig{}, &core.AgentContextSpec{
		MaxTokens: 12000,
	})

	if !policy.ProgressiveEnabled {
		t.Fatal("expected progressive loading to remain enabled when the flag is unset")
	}
	if policy.Budget == nil || policy.Budget.MaxTokens != 12000 {
		t.Fatalf("expected budget override to apply, got %#v", policy.Budget)
	}
}

func TestNewContextPolicyHonorsExplicitProgressiveDisable(t *testing.T) {
	disabled := false
	policy := NewContextPolicy(ContextPolicyConfig{}, &core.AgentContextSpec{
		MaxTokens:          12000,
		ProgressiveLoading: &disabled,
	})

	if policy.ProgressiveEnabled {
		t.Fatal("expected explicit progressive_loading=false to disable progressive loading")
	}
}

func TestNewContextPolicyHonorsExplicitProgressiveEnable(t *testing.T) {
	enabled := true
	policy := NewContextPolicy(ContextPolicyConfig{}, &core.AgentContextSpec{
		ProgressiveLoading: &enabled,
	})

	if !policy.ProgressiveEnabled {
		t.Fatal("expected explicit progressive_loading=true to enable progressive loading")
	}
}
