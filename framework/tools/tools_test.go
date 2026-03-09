package tools

import "testing"

func TestDecideByPatternsForwardsToToolHelpers(t *testing.T) {
	decision, matched := DecideByPatterns("src/main.go", []string{"src/**"}, []string{"src/private/**"}, AgentPermissionAsk)
	if decision != AgentPermissionAllow {
		t.Fatalf("expected allow decision, got %q", decision)
	}
	if matched != "src/**" {
		t.Fatalf("expected src/** match, got %q", matched)
	}
}
