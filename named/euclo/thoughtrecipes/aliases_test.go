package thoughtrecipes

import (
	"testing"
)

func TestAliasResolverStandardAliases(t *testing.T) {
	resolver := NewAliasResolver(nil)

	tests := []struct {
		alias     string
		wantKey   string
		wantFound bool
	}{
		{"explore_findings", "pipeline.explore", true},
		{"analysis_result", "pipeline.analyze", true},
		{"plan_output", "pipeline.plan", true},
		{"code_changes", "pipeline.code", true},
		{"verify_result", "pipeline.verify", true},
		{"final_output", "pipeline.final_output", true},
		{"review_findings", "euclo.review_findings", true},
		{"root_cause", "euclo.root_cause", true},
		{"root_cause_candidates", "euclo.root_cause_candidates", true},
		{"regression_analysis", "euclo.regression_analysis", true},
		{"verification_summary", "euclo.verification_summary", true},
		{"debug_investigation", "euclo.debug_investigation_summary", true},
		{"repair_readiness", "euclo.debug_repair_readiness", true},
		{"plan_candidates", "euclo.plan_candidates", true},
		{"unknown_alias", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.alias, func(t *testing.T) {
			gotKey, gotFound := resolver.Resolve(tt.alias)
			if gotFound != tt.wantFound {
				t.Errorf("Resolve(%q) found = %v, want %v", tt.alias, gotFound, tt.wantFound)
			}
			if gotFound && gotKey != tt.wantKey {
				t.Errorf("Resolve(%q) key = %q, want %q", tt.alias, gotKey, tt.wantKey)
			}
		})
	}
}

func TestAliasResolverCustomOverride(t *testing.T) {
	// Custom alias that shadows a standard alias
	custom := map[string]string{
		"explore_findings": "custom.pipeline.explore",
	}
	resolver := NewAliasResolver(custom)

	// Should resolve to custom value, not standard
	key, ok := resolver.Resolve("explore_findings")
	if !ok {
		t.Fatal("expected to find custom alias")
	}
	if key != "custom.pipeline.explore" {
		t.Errorf("custom shadow failed: got %q, want %q", key, "custom.pipeline.explore")
	}

	// Non-shadowed standard aliases should still work
	key, ok = resolver.Resolve("plan_output")
	if !ok {
		t.Fatal("expected to find standard alias")
	}
	if key != "pipeline.plan" {
		t.Errorf("standard alias failed: got %q, want %q", key, "pipeline.plan")
	}
}

func TestAliasResolverUnknown(t *testing.T) {
	resolver := NewAliasResolver(nil)

	// Unknown alias should return ("", false)
	key, ok := resolver.Resolve("unknown_alias")
	if ok {
		t.Error("expected unknown alias to not be found")
	}
	if key != "" {
		t.Errorf("expected empty key for unknown alias, got %q", key)
	}

	// MustResolve should return the alias itself as fallback
	mustKey := resolver.MustResolve("unknown_alias")
	if mustKey != "unknown_alias" {
		t.Errorf("MustResolve fallback failed: got %q, want %q", mustKey, "unknown_alias")
	}
}

func TestAliasResolverCustomOnly(t *testing.T) {
	// Custom alias that doesn't shadow anything
	custom := map[string]string{
		"my_custom_value": "my.custom.state.key",
	}
	resolver := NewAliasResolver(custom)

	key, ok := resolver.Resolve("my_custom_value")
	if !ok {
		t.Fatal("expected to find custom-only alias")
	}
	if key != "my.custom.state.key" {
		t.Errorf("custom-only alias failed: got %q, want %q", key, "my.custom.state.key")
	}
}

func TestAliasResolverIsShadowed(t *testing.T) {
	// No custom aliases - nothing shadowed
	r1 := NewAliasResolver(nil)
	if r1.IsShadowed("explore_findings") {
		t.Error("expected no shadowing with nil custom")
	}

	// Custom alias that shadows
	r2 := NewAliasResolver(map[string]string{
		"explore_findings": "custom.key",
	})
	if !r2.IsShadowed("explore_findings") {
		t.Error("expected explore_findings to be shadowed")
	}

	// Custom alias that doesn't shadow
	if r2.IsShadowed("nonexistent") {
		t.Error("expected nonexistent to not be shadowed")
	}
}

func TestAliasResolverListShadowed(t *testing.T) {
	// No custom aliases
	r1 := NewAliasResolver(nil)
	if r1.ListShadowed() != nil {
		t.Error("expected nil for no custom aliases")
	}

	// Some shadowed, some not
	r2 := NewAliasResolver(map[string]string{
		"explore_findings":      "custom1",
		"plan_output":           "custom2",
		"my_custom":             "custom3", // not shadowed
		"analysis_result":       "custom4",
	})

	shadowed := r2.ListShadowed()
	if len(shadowed) != 3 {
		t.Errorf("expected 3 shadowed aliases, got %d: %v", len(shadowed), shadowed)
	}

	// Check that all expected are present
	expected := map[string]bool{
		"explore_findings": true,
		"plan_output":      true,
		"analysis_result":  true,
	}
	for _, s := range shadowed {
		if !expected[s] {
			t.Errorf("unexpected shadowed alias: %s", s)
		}
	}
}

func TestAliasResolverNilReceiver(t *testing.T) {
	var resolver *AliasResolver // nil

	// Should fall back to standard aliases
	key, ok := resolver.Resolve("plan_output")
	if !ok {
		t.Error("expected to find standard alias with nil receiver")
	}
	if key != "pipeline.plan" {
		t.Errorf("got %q, want %q", key, "pipeline.plan")
	}

	// Unknown alias should return ("", false)
	_, ok = resolver.Resolve("unknown")
	if ok {
		t.Error("expected unknown alias to not be found with nil receiver")
	}
}

func TestAliasResolverMustResolveStandard(t *testing.T) {
	resolver := NewAliasResolver(nil)

	// Standard alias should resolve to key
	key := resolver.MustResolve("plan_output")
	if key != "pipeline.plan" {
		t.Errorf("got %q, want %q", key, "pipeline.plan")
	}
}

func TestAliasResolverMustResolveCustom(t *testing.T) {
	resolver := NewAliasResolver(map[string]string{
		"my_alias": "my.custom.key",
	})

	// Custom alias should resolve
	key := resolver.MustResolve("my_alias")
	if key != "my.custom.key" {
		t.Errorf("got %q, want %q", key, "my.custom.key")
	}
}
