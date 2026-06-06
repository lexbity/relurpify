package arch

import (
	"testing"
)

func TestAllowlistContains(t *testing.T) {
	a := Allowlist{entries: map[string]map[string]bool{
		"cycle": {"cycle: a depends on b": true},
		"layer": {},
	}}
	if !a.Contains("cycle", "cycle: a depends on b") {
		t.Error("allowlist should contain known cycle")
	}
	if a.Contains("cycle", "cycle: x depends on y") {
		t.Error("allowlist should not contain unknown cycle")
	}
	if a.Contains("layer", "layer: anything") {
		t.Error("empty allowlist category should not match anything")
	}
	if a.Contains("nonexistent", "anything") {
		t.Error("nonexistent category should not match")
	}
}

func TestLoadAllowlist(t *testing.T) {
	a, err := LoadAllowlist("testdata/allowlist.yaml")
	if err != nil {
		t.Fatalf("LoadAllowlist: %v", err)
	}
	if !a.Contains("cycle", "cycle: test cycle") {
		t.Error("should contain test cycle")
	}
	if !a.Contains("layer", "layer: test layer violation") {
		t.Error("should contain test layer violation")
	}
	if !a.Contains("consumer", "consumer: test consumer violation") {
		t.Error("should contain test consumer violation")
	}
	if !a.Contains("glob", "glob: test glob pattern") {
		t.Error("should contain test glob violation")
	}
	if !a.Contains("stub", "stub: test stub marker") {
		t.Error("should contain test stub violation")
	}
}

func TestValidateAllowlist_noStale(t *testing.T) {
	a := Allowlist{entries: map[string]map[string]bool{
		"consumer": {"consumer: dead has no non-test importers": true},
	}}
	violations := map[string][]string{
		"consumer": {"consumer: dead has no non-test importers"},
	}
	stale := ValidateAllowlist(a, violations)
	if len(stale) != 0 {
		t.Errorf("expected no stale entries, got %v", stale)
	}
}

func TestValidateAllowlist_staleEntry(t *testing.T) {
	a := Allowlist{entries: map[string]map[string]bool{
		"consumer": {"consumer: dead has no non-test importers": true},
	}}
	violations := map[string][]string{
		"consumer": {},
	}
	stale := ValidateAllowlist(a, violations)
	if len(stale) == 0 {
		t.Fatal("expected stale allowlist entry")
	}
}
