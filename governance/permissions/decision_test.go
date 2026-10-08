package permissions

import (
	"strings"
	"testing"
)

func TestParseDecision(t *testing.T) {
	ok := map[string]Decision{
		"allow":   DecisionAllow,
		"ALLOW":   DecisionAllow,
		"Allow":   DecisionAllow,
		" allow ": DecisionAllow,
		"\task\n": DecisionAsk,
		"ask":     DecisionAsk,
		"ASK":     DecisionAsk,
		"deny":    DecisionDeny,
		"DENY":    DecisionDeny,
		"Deny":    DecisionDeny,
	}
	for input, want := range ok {
		got, err := ParseDecision(input)
		if err != nil {
			t.Errorf("ParseDecision(%q) returned error: %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("ParseDecision(%q) = %q, want %q", input, got, want)
		}
	}

	bad := []string{"", "alow", "denyy", "allowx", "yes", "no", "true", "1", "all ow", "ALLOWED", "allow_"}
	for _, input := range bad {
		got, err := ParseDecision(input)
		if err == nil {
			t.Errorf("ParseDecision(%q) = %q, want error", input, got)
			continue
		}
		if got != "" {
			t.Errorf("ParseDecision(%q) returned non-empty %q with error", input, got)
		}
		if !strings.Contains(err.Error(), "allow") || !strings.Contains(err.Error(), "ask") || !strings.Contains(err.Error(), "deny") {
			t.Errorf("ParseDecision(%q) error should list the vocabulary, got %q", input, err.Error())
		}
	}
}

func TestDecisionOr(t *testing.T) {
	cases := []struct {
		input string
		def   Decision
		want  Decision
	}{
		{"", DecisionAsk, DecisionAsk},
		{"", DecisionDeny, DecisionDeny},
		{"", "", DecisionAsk},
		{"  ", DecisionDeny, DecisionDeny},
		{"allow", DecisionDeny, DecisionAllow},
		{"DENY", DecisionAsk, DecisionDeny},
	}
	for _, tc := range cases {
		got, err := DecisionOr(tc.input, tc.def)
		if err != nil {
			t.Errorf("DecisionOr(%q, %q) error: %v", tc.input, tc.def, err)
			continue
		}
		if got != tc.want {
			t.Errorf("DecisionOr(%q, %q) = %q, want %q", tc.input, tc.def, got, tc.want)
		}
	}

	if _, err := DecisionOr("alow", DecisionAsk); err == nil {
		t.Error("DecisionOr must reject a non-vocabulary non-empty value")
	}
}

func TestDecisionValid(t *testing.T) {
	for _, d := range []Decision{DecisionAllow, DecisionAsk, DecisionDeny} {
		if !d.Valid() {
			t.Errorf("%q should be valid", d)
		}
	}
	for _, d := range []Decision{"", "alow", "ALLOW"} {
		if d.Valid() {
			t.Errorf("%q should be invalid", d)
		}
	}
}
