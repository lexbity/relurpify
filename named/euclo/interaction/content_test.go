package interaction

import (
	"encoding/json"
	"testing"
)

func TestProposalContent_JSONRoundTrip(t *testing.T) {
	c := ProposalContent{
		Interpretation: "Refactor database layer",
		Scope:          []string{"db/conn.go", "db/query.go"},
		Approach:       "edit_verify_repair",
		Constraints:    []string{"no breaking changes"},
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded ProposalContent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Interpretation != c.Interpretation {
		t.Errorf("interpretation: got %q, want %q", decoded.Interpretation, c.Interpretation)
	}
	if len(decoded.Scope) != 2 {
		t.Errorf("scope: got %d items, want 2", len(decoded.Scope))
	}
}

func TestQuestionContent_JSONRoundTrip(t *testing.T) {
	c := QuestionContent{
		Question: "Which approach do you prefer?",
		Options: []QuestionOption{
			{ID: "1", Label: "Option A", Description: "Fast but risky"},
			{ID: "2", Label: "Option B", Description: "Slow but safe"},
		},
		AllowFreetext: true,
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded QuestionContent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded.Options) != 2 {
		t.Errorf("options: got %d, want 2", len(decoded.Options))
	}
	if !decoded.AllowFreetext {
		t.Error("allow_freetext: got false, want true")
	}
}

func TestCandidatesContent_JSONRoundTrip(t *testing.T) {
	c := CandidatesContent{
		Candidates: []Candidate{
			{ID: "c1", Summary: "Plan A", Properties: map[string]string{"risk": "low"}},
			{ID: "c2", Summary: "Plan B", Properties: map[string]string{"risk": "high"}},
		},
		RecommendedID: "c1",
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded CandidatesContent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.RecommendedID != "c1" {
		t.Errorf("recommended_id: got %q, want %q", decoded.RecommendedID, "c1")
	}
	if len(decoded.Candidates) != 2 {
		t.Errorf("candidates: got %d, want 2", len(decoded.Candidates))
	}
}

func TestComparisonContent_JSONRoundTrip(t *testing.T) {
	c := ComparisonContent{
		Dimensions: []string{"Risk", "Speed", "Complexity"},
		Matrix: [][]string{
			{"Low", "Fast", "Simple"},
			{"High", "Slow", "Complex"},
		},
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded ComparisonContent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded.Dimensions) != 3 {
		t.Errorf("dimensions: got %d, want 3", len(decoded.Dimensions))
	}
	if len(decoded.Matrix) != 2 {
		t.Errorf("matrix rows: got %d, want 2", len(decoded.Matrix))
	}
}

func TestDraftContent_JSONRoundTrip(t *testing.T) {
	c := DraftContent{
		Kind: "plan",
		Items: []DraftItem{
			{ID: "s1", Content: "Step 1: Read files", Editable: true, Removable: true},
			{ID: "s2", Content: "Step 2: Apply changes", Editable: true, Removable: false},
		},
		Addable: true,
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded DraftContent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Kind != "plan" {
		t.Errorf("kind: got %q, want %q", decoded.Kind, "plan")
	}
	if len(decoded.Items) != 2 {
		t.Errorf("items: got %d, want 2", len(decoded.Items))
	}
	if !decoded.Items[0].Removable {
		t.Error("item 0 removable: got false, want true")
	}
}

func TestResultContent_JSONRoundTrip(t *testing.T) {
	c := ResultContent{
		Status: "failed",
		Evidence: []EvidenceItem{
			{Kind: "stacktrace", Detail: "panic at line 42", Location: "main.go:42", Confidence: 0.95},
			{Kind: "test_correlation", Detail: "TestFoo fails consistently"},
		},
		Gaps: []string{"no coverage for edge case"},
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded ResultContent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Status != "failed" {
		t.Errorf("status: got %q, want %q", decoded.Status, "failed")
	}
	if len(decoded.Evidence) != 2 {
		t.Errorf("evidence: got %d, want 2", len(decoded.Evidence))
	}
	if decoded.Evidence[0].Confidence != 0.95 {
		t.Errorf("confidence: got %f, want 0.95", decoded.Evidence[0].Confidence)
	}
}

func TestFindingsContent_JSONRoundTrip(t *testing.T) {
	c := FindingsContent{
		Critical: []Finding{
			{Location: "auth.go:15", Description: "SQL injection", Suggestion: "Use parameterized query"},
		},
		Warning: []Finding{
			{Location: "handler.go:80", Description: "Missing error check"},
		},
		Info: []Finding{},
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded FindingsContent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded.Critical) != 1 {
		t.Errorf("critical: got %d, want 1", len(decoded.Critical))
	}
	if decoded.Critical[0].Suggestion != "Use parameterized query" {
		t.Errorf("suggestion: got %q", decoded.Critical[0].Suggestion)
	}
}

func TestTransitionContent_JSONRoundTrip(t *testing.T) {
	c := TransitionContent{
		FromMode:  "code",
		ToMode:    "debug",
		Reason:    "Verification failed repeatedly",
		Artifacts: []string{"explore", "edit_intent"},
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded TransitionContent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.FromMode != "code" || decoded.ToMode != "debug" {
		t.Errorf("modes: got %s→%s, want code→debug", decoded.FromMode, decoded.ToMode)
	}
}

func TestHelpContent_JSONRoundTrip(t *testing.T) {
	c := HelpContent{
		Mode:         "planning",
		CurrentPhase: "generate",
		PhaseMap: []PhaseInfo{
			{ID: "scope", Label: "Scope"},
			{ID: "generate", Label: "Generate", Current: true},
			{ID: "refine", Label: "Refine"},
		},
		AvailableActions: []ActionInfo{
			{Phrase: "alternatives", Description: "Generate more plan candidates"},
		},
		AvailableTransitions: []TransitionInfo{
			{Phrase: "execute", TargetMode: "code"},
		},
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded HelpContent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.CurrentPhase != "generate" {
		t.Errorf("current_phase: got %q, want %q", decoded.CurrentPhase, "generate")
	}
	if len(decoded.PhaseMap) != 3 {
		t.Errorf("phase_map: got %d, want 3", len(decoded.PhaseMap))
	}
	if !decoded.PhaseMap[1].Current {
		t.Error("phase_map[1].current: got false, want true")
	}
}

func TestStatusContent_JSONRoundTrip(t *testing.T) {
	c := StatusContent{
		Message:  "Analyzing files...",
		Progress: 0.45,
		Phase:    "explore",
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded StatusContent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Progress != 0.45 {
		t.Errorf("progress: got %f, want 0.45", decoded.Progress)
	}
}

func TestSummaryContent_JSONRoundTrip(t *testing.T) {
	c := SummaryContent{
		Description: "Applied 3 edits, all tests pass",
		Artifacts:   []string{"edit_intent", "verification"},
		Changes:     []string{"server/handler.go", "server/handler_test.go"},
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded SummaryContent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded.Artifacts) != 2 {
		t.Errorf("artifacts: got %d, want 2", len(decoded.Artifacts))
	}
	if len(decoded.Changes) != 2 {
		t.Errorf("changes: got %d, want 2", len(decoded.Changes))
	}
}
