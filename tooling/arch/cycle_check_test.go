package arch

import (
	"testing"
)

const (
	ImportPathA_cycle_check_test = "codeburg.org/lexbit/relurpify/a"
	ImportPathB_cycle_check_test = "codeburg.org/lexbit/relurpify/b"
	ImportPathC_cycle_check_test = "codeburg.org/lexbit/relurpify/c"
	Fmt_cycle_check_test         = "fmt"
)

func TestCheckCycles_noCycle(t *testing.T) {
	forward := map[string][]string{
		ImportPathA_cycle_check_test: {ImportPathB_cycle_check_test, Fmt_cycle_check_test},
		ImportPathB_cycle_check_test: {ImportPathC_cycle_check_test},
		ImportPathC_cycle_check_test: {"os"},
	}
	violations := CheckCycles(forward, Allowlist{})
	if len(violations) != 0 {
		t.Errorf("expected no cycles, got %v", violations)
	}
}

func TestCheckCycles_directCycle(t *testing.T) {
	forward := map[string][]string{
		ImportPathA_cycle_check_test: {ImportPathB_cycle_check_test},
		ImportPathB_cycle_check_test: {ImportPathA_cycle_check_test},
	}
	violations := CheckCycles(forward, Allowlist{})
	if len(violations) == 0 {
		t.Fatal("expected cycle detection, got none")
	}
}

func TestCheckCycles_selfCycle(t *testing.T) {
	forward := map[string][]string{
		ImportPathA_cycle_check_test: {ImportPathA_cycle_check_test},
	}
	violations := CheckCycles(forward, Allowlist{})
	if len(violations) == 0 {
		t.Fatal("expected self-cycle detection")
	}
}

func TestCheckCycles_indirectCycle(t *testing.T) {
	forward := map[string][]string{
		ImportPathA_cycle_check_test: {ImportPathB_cycle_check_test},
		ImportPathB_cycle_check_test: {ImportPathC_cycle_check_test},
		ImportPathC_cycle_check_test: {ImportPathA_cycle_check_test},
	}
	violations := CheckCycles(forward, Allowlist{})
	if len(violations) == 0 {
		t.Fatal("expected indirect cycle detection")
	}
}

func TestCheckCycles_allowlist(t *testing.T) {
	forward := map[string][]string{
		ImportPathA_cycle_check_test: {ImportPathB_cycle_check_test},
		ImportPathB_cycle_check_test: {ImportPathA_cycle_check_test},
	}
	allowlist := Allowlist{entries: map[string]map[string]bool{
		"cycle": {
			"cycle: codeburg.org/lexbit/relurpify/a → codeburg.org/lexbit/relurpify/a": true,
			"cycle: codeburg.org/lexbit/relurpify/b → codeburg.org/lexbit/relurpify/b": true,
		},
	}}
	violations := CheckCycles(forward, allowlist)
	if len(violations) != 0 {
		t.Errorf("expected allowlist to exempt cycle, got %v", violations)
	}
}
