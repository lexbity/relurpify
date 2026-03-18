package runtime

import (
	"testing"

	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

func TestCollectSignals_Keywords(t *testing.T) {
	signals := CollectSignals(TaskEnvelope{
		Instruction:    "debug this failing test",
		EditPermitted:  true,
		CapabilitySnapshot: euclotypes.CapabilitySnapshot{HasWriteTools: true},
	})

	if len(signals) == 0 {
		t.Fatal("expected signals")
	}
	hasDebug := false
	for _, s := range signals {
		if s.Mode == "debug" && s.Kind == "keyword" {
			hasDebug = true
		}
	}
	if !hasDebug {
		t.Error("expected debug keyword signal")
	}
}

func TestCollectSignals_FilePatterns(t *testing.T) {
	signals := CollectSignals(TaskEnvelope{
		Instruction:   "fix the handler_test.go file",
		EditPermitted: true,
	})

	hasTestPattern := false
	for _, s := range signals {
		if s.Kind == "file_pattern" && s.Mode == "tdd" {
			hasTestPattern = true
		}
	}
	if !hasTestPattern {
		t.Error("expected tdd file_pattern signal for _test.go")
	}
}

func TestCollectSignals_TaskStructure_Debug(t *testing.T) {
	cases := []string{
		"this used to work but now fails",
		"the API broke after last deploy",
		"there's a regression in auth",
		"login stopped working yesterday",
	}
	for _, instruction := range cases {
		signals := collectTaskStructureSignals(instruction)
		hasDebug := false
		for _, s := range signals {
			if s.Mode == "debug" {
				hasDebug = true
				break
			}
		}
		if !hasDebug {
			t.Errorf("expected debug signal for %q", instruction)
		}
	}
}

func TestCollectSignals_TaskStructure_Planning(t *testing.T) {
	cases := []string{
		"how should we migrate the database",
		"I need a strategy for this rewrite",
		"redesign the auth system",
	}
	for _, instruction := range cases {
		signals := collectTaskStructureSignals(instruction)
		hasPlanning := false
		for _, s := range signals {
			if s.Mode == "planning" {
				hasPlanning = true
				break
			}
		}
		if !hasPlanning {
			t.Errorf("expected planning signal for %q", instruction)
		}
	}
}

func TestCollectSignals_ErrorText(t *testing.T) {
	cases := []struct {
		instruction string
		value       string
	}{
		{"panic: runtime error: invalid memory address", "panic"},
		{"goroutine 1 [running]:", "goroutine_dump"},
		{"error: cannot find module", "error_prefix"},
		{"fatal: not a git repository", "fatal"},
		{"nil pointer dereference at main.go:42", "nil_pointer"},
		{"exit code 1", "exit_code"},
		{"at pkg/handler.go:15", "go_file_line"},
	}
	for _, tc := range cases {
		signals := collectErrorTextSignals(tc.instruction)
		found := false
		for _, s := range signals {
			if s.Value == tc.value {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q signal for %q", tc.value, tc.instruction)
		}
	}
}

func TestCollectSignals_ContextHints(t *testing.T) {
	signals := CollectSignals(TaskEnvelope{
		ModeHint:      "debug",
		EditPermitted: true,
	})

	hasHint := false
	for _, s := range signals {
		if s.Kind == "context_hint" && s.Mode == "debug" {
			hasHint = true
		}
	}
	if !hasHint {
		t.Error("expected context_hint signal for mode_hint")
	}
}

func TestCollectSignals_WorkspaceState(t *testing.T) {
	signals := CollectSignals(TaskEnvelope{
		EditPermitted:         false,
		PreviousArtifactKinds: []string{"euclo.plan", "euclo.verification"},
	})

	hasReadOnly := false
	hasPlan := false
	hasVerification := false
	for _, s := range signals {
		if s.Kind == "workspace_state" {
			switch s.Value {
			case "read_only":
				hasReadOnly = true
			case "has_plan_artifact":
				hasPlan = true
			case "has_verification_artifact":
				hasVerification = true
			}
		}
	}
	if !hasReadOnly {
		t.Error("expected read_only signal")
	}
	if !hasPlan {
		t.Error("expected has_plan_artifact signal")
	}
	if !hasVerification {
		t.Error("expected has_verification_artifact signal")
	}
}

func TestScoreSignals(t *testing.T) {
	signals := []ClassificationSignal{
		{Kind: "keyword", Value: "debug", Weight: 0.3, Mode: "debug"},
		{Kind: "keyword", Value: "failing", Weight: 0.3, Mode: "debug"},
		{Kind: "keyword", Value: "fix", Weight: 0.3, Mode: "code"},
		{Kind: "error_text", Value: "panic", Weight: 0.6, Mode: "debug"},
	}

	candidates := ScoreSignals(signals)
	if len(candidates) != 2 {
		t.Fatalf("candidates: got %d, want 2", len(candidates))
	}
	// Debug should be first (0.3 + 0.3 + 0.6 = 1.2 vs code 0.3).
	if candidates[0].Mode != "debug" {
		t.Errorf("top mode: got %q, want debug", candidates[0].Mode)
	}
	if candidates[0].Score != 1.2 {
		t.Errorf("debug score: got %f, want 1.2", candidates[0].Score)
	}
}

func TestIsAmbiguous(t *testing.T) {
	// Not ambiguous: clear winner.
	candidates := []ModeCandidate{
		{Mode: "debug", Score: 1.2},
		{Mode: "code", Score: 0.3},
	}
	if IsAmbiguous(candidates) {
		t.Error("expected not ambiguous (1.2 vs 0.3)")
	}

	// Ambiguous: close scores.
	candidates = []ModeCandidate{
		{Mode: "debug", Score: 0.6},
		{Mode: "code", Score: 0.55},
	}
	if !IsAmbiguous(candidates) {
		t.Error("expected ambiguous (0.6 vs 0.55)")
	}

	// Single candidate: not ambiguous.
	if IsAmbiguous([]ModeCandidate{{Mode: "code", Score: 0.5}}) {
		t.Error("single candidate should not be ambiguous")
	}
}

func TestNormalizeConfidence(t *testing.T) {
	candidates := []ModeCandidate{
		{Mode: "debug", Score: 1.2},
	}
	conf := NormalizeConfidence(candidates, 1.5)
	if conf < 0.79 || conf > 0.81 {
		t.Errorf("confidence: got %f, want ~0.8", conf)
	}

	// Empty.
	conf = NormalizeConfidence(nil, 1.0)
	if conf != 0 {
		t.Errorf("empty: got %f", conf)
	}
}
