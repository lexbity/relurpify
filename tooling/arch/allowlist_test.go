package arch

import (
	"testing"
)

const (
	Consumer_allowlist_test                          = "consumer"
	Consumerdeadhasnonontestimporters_allowlist_test = "consumer: dead has no non-test importers"
	Cycle_allowlist_test                             = "cycle"
	Layer_allowlist_test                             = "layer"
)

func TestAllowlistContains(t *testing.T) {
	a := Allowlist{entries: map[string]map[string]bool{
		Cycle_allowlist_test: {"cycle: a depends on b": true},
		Layer_allowlist_test: {},
	}}
	if !a.Contains(Cycle_allowlist_test, "cycle: a depends on b") {
		t.Error("allowlist should contain known cycle")
	}
	if a.Contains(Cycle_allowlist_test, "cycle: x depends on y") {
		t.Error("allowlist should not contain unknown cycle")
	}
	if a.Contains(Layer_allowlist_test, "layer: anything") {
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
	if !a.Contains(Cycle_allowlist_test, "cycle: test cycle") {
		t.Error("should contain test cycle")
	}
	if !a.Contains(Layer_allowlist_test, "layer: test layer violation") {
		t.Error("should contain test layer violation")
	}
	if !a.Contains(Consumer_allowlist_test, "consumer: test consumer violation") {
		t.Error("should contain test consumer violation")
	}
	if !a.Contains("glob", "glob: test glob pattern") {
		t.Error("should contain test glob violation")
	}
}

func TestValidateAllowlist_noStale(t *testing.T) {
	a := Allowlist{entries: map[string]map[string]bool{
		Consumer_allowlist_test: {Consumerdeadhasnonontestimporters_allowlist_test: true},
	}}
	violations := map[string][]string{
		Consumer_allowlist_test: {Consumerdeadhasnonontestimporters_allowlist_test},
	}
	stale := ValidateAllowlist(a, violations)
	if len(stale) != 0 {
		t.Errorf("expected no stale entries, got %v", stale)
	}
}

func TestValidateAllowlist_staleEntry(t *testing.T) {
	a := Allowlist{entries: map[string]map[string]bool{
		Consumer_allowlist_test: {Consumerdeadhasnonontestimporters_allowlist_test: true},
	}}
	violations := map[string][]string{
		Consumer_allowlist_test: {},
	}
	stale := ValidateAllowlist(a, violations)
	if len(stale) == 0 {
		t.Fatal("expected stale allowlist entry")
	}
}
