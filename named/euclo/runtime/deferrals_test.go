package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestLoadDeferredIssuesFromWorkspace_RoundTripsPersistedIssues(t *testing.T) {
	workspace := t.TempDir()
	state := core.NewContext()
	task := &core.Task{Context: map[string]any{"workspace": workspace}}
	now := time.Date(2026, 4, 16, 12, 30, 0, 0, time.UTC)
	issues := []DeferredExecutionIssue{
		{
			IssueID:               "issue-1",
			WorkflowID:            "wf-1",
			RunID:                 "run-1",
			ExecutionID:           "exec-1",
			ActivePlanID:          "plan-1",
			ActivePlanVersion:     3,
			StepID:                "step-1",
			RelatedStepIDs:        []string{"step-1", "step-2"},
			Kind:                  DeferredIssueAmbiguity,
			Severity:              DeferredIssueSeverityHigh,
			Status:                DeferredIssueStatusOpen,
			Title:                 "Missing context",
			Summary:               "The surface should preserve this issue.",
			WhyNotResolvedInline:  "Not enough context to resolve inline.",
			RecommendedReentry:    "archaeology",
			RecommendedNextAction: "inspect the workspace artifacts",
			Evidence: DeferredExecutionEvidence{
				ShortReasoningSummary:  "round trip",
				RelevantProvenanceRefs: []string{"prov-1"},
			},
			ArchaeoRefs: map[string][]string{
				"pattern_refs": []string{"pattern-1"},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			IssueID:               "issue-2",
			WorkflowID:            "wf-1",
			Kind:                  DeferredIssueVerificationConcern,
			Severity:              DeferredIssueSeverityMedium,
			Status:                DeferredIssueStatusAcknowledged,
			Title:                 "Verification concern",
			RecommendedNextAction: "re-run verification",
			CreatedAt:             now,
		},
	}
	written := PersistDeferredExecutionIssuesToWorkspace(task, state, issues)
	if len(written) != len(issues) {
		t.Fatalf("PersistDeferredExecutionIssuesToWorkspace wrote %d issues, want %d", len(written), len(issues))
	}

	loaded := LoadDeferredIssuesFromWorkspace(workspace)
	if len(loaded) != len(issues) {
		t.Fatalf("LoadDeferredIssuesFromWorkspace returned %d issues, want %d", len(loaded), len(issues))
	}
	if loaded[0].IssueID != "issue-1" || loaded[0].Title != "Missing context" || loaded[0].RecommendedNextAction != "inspect the workspace artifacts" {
		t.Fatalf("unexpected loaded issue: %+v", loaded[0])
	}
	if loaded[0].WorkspaceArtifactPath == "" {
		t.Fatal("expected workspace artifact path to be populated")
	}
	if loaded[1].Status != DeferredIssueStatusAcknowledged {
		t.Fatalf("unexpected second issue status: %+v", loaded[1])
	}
}

func TestLoadDeferredIssuesFromWorkspace_SkipsMalformedMarkdown(t *testing.T) {
	workspace := t.TempDir()
	dir := filepath.Join(workspace, "relurpify_cfg", "artifacts", "euclo", "deferred")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	good := filepath.Join(dir, "good.md")
	bad := filepath.Join(dir, "bad.md")
	if err := os.WriteFile(good, []byte(`---
issue_id: "good"
workflow_id: "wf"
kind: "ambiguity"
severity: "low"
status: "open"
title: "Good"
recommended_next_action: "continue"
created_at: "2026-04-16T12:30:00Z"
---

# Good
`), 0o644); err != nil {
		t.Fatalf("write good: %v", err)
	}
	if err := os.WriteFile(bad, []byte("not frontmatter"), 0o644); err != nil {
		t.Fatalf("write bad: %v", err)
	}

	loaded := LoadDeferredIssuesFromWorkspace(workspace)
	if len(loaded) != 1 {
		t.Fatalf("expected only valid issue to be loaded, got %d", len(loaded))
	}
	if loaded[0].IssueID != "good" {
		t.Fatalf("unexpected loaded issue: %+v", loaded[0])
	}
}

func TestLoadDeferredIssuesFromWorkspace_MissingDirReturnsNil(t *testing.T) {
	if loaded := LoadDeferredIssuesFromWorkspace(filepath.Join(t.TempDir(), "missing")); loaded != nil {
		t.Fatalf("expected nil, got %+v", loaded)
	}
}

func TestBuildDeferralsSurfaceSummary_GroupsIssues(t *testing.T) {
	summary := BuildDeferralsSurfaceSummary("wf-1", []DeferredExecutionIssue{
		{IssueID: "a", WorkflowID: "wf-1", Kind: DeferredIssueAmbiguity, Severity: DeferredIssueSeverityLow, Status: DeferredIssueStatusOpen, Title: "A", RecommendedNextAction: "next-a"},
		{IssueID: "b", WorkflowID: "wf-1", Kind: DeferredIssueAmbiguity, Severity: DeferredIssueSeverityHigh, Status: DeferredIssueStatusAcknowledged, Title: "B", RecommendedNextAction: "next-b"},
		{IssueID: "c", WorkflowID: "wf-1", Kind: DeferredIssueVerificationConcern, Severity: DeferredIssueSeverityHigh, Status: DeferredIssueStatusResolved, Title: "C", RecommendedNextAction: "next-c"},
	})
	if summary.TotalOpen != 2 {
		t.Fatalf("TotalOpen = %d, want 2", summary.TotalOpen)
	}
	if summary.BySeverity[string(DeferredIssueSeverityLow)] != 1 || summary.BySeverity[string(DeferredIssueSeverityHigh)] != 1 {
		t.Fatalf("unexpected severity grouping: %+v", summary.BySeverity)
	}
	if summary.ByKind[string(DeferredIssueAmbiguity)] != 2 {
		t.Fatalf("unexpected kind grouping: %+v", summary.ByKind)
	}
	if len(summary.Issues) != 2 {
		t.Fatalf("expected only open issues in summary, got %+v", summary.Issues)
	}
}

func TestAssembleDeferralNextActions_FiltersAndSorts(t *testing.T) {
	actions := AssembleDeferralNextActions([]DeferredExecutionIssue{
		{IssueID: "low", Title: "Low", Kind: DeferredIssueAmbiguity, Severity: DeferredIssueSeverityLow, Status: DeferredIssueStatusOpen},
		{IssueID: "resolved", Title: "Resolved", Kind: DeferredIssueAmbiguity, Severity: DeferredIssueSeverityCritical, Status: DeferredIssueStatusResolved},
		{IssueID: "high-1", Title: "High 1", Kind: DeferredIssueVerificationConcern, Severity: DeferredIssueSeverityHigh, Status: DeferredIssueStatusOpen},
		{IssueID: "crit-1", Title: "Critical 1", Kind: DeferredIssuePatternTension, Severity: DeferredIssueSeverityCritical, Status: DeferredIssueStatusOpen},
	})
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %#v", actions)
	}
	if actions[0].IssueID != "crit-1" || actions[1].IssueID != "high-1" {
		t.Fatalf("expected critical then high ordering, got %#v", actions)
	}
	if actions[0].SuggestedPrompt == "" || actions[1].SuggestedPrompt == "" {
		t.Fatalf("expected non-empty prompts, got %#v", actions)
	}
	if want := "Run verification with focus on: High 1"; actions[1].SuggestedPrompt != want {
		t.Fatalf("unexpected prompt: got %q want %q", actions[1].SuggestedPrompt, want)
	}
}

func TestRewriteDeferredIssueMarkdown_AppendsResolutionSection(t *testing.T) {
	workspace := t.TempDir()
	state := core.NewContext()
	task := &core.Task{Context: map[string]any{"workspace": workspace}}
	issue := DeferredExecutionIssue{
		IssueID:               "issue-1",
		WorkflowID:            "wf-1",
		Kind:                  DeferredIssueAmbiguity,
		Severity:              DeferredIssueSeverityHigh,
		Status:                DeferredIssueStatusOpen,
		Title:                 "Need clarification",
		Summary:               "Clarify the ambiguous requirement.",
		RecommendedNextAction: "ask the user for context",
		CreatedAt:             time.Date(2026, 4, 16, 12, 30, 0, 0, time.UTC),
	}
	written := PersistDeferredExecutionIssuesToWorkspace(task, state, []DeferredExecutionIssue{issue})
	if len(written) != 1 {
		t.Fatalf("persisted %d issues, want 1", len(written))
	}

	path := written[0].WorkspaceArtifactPath
	if err := RewriteDeferredIssueMarkdown(path, DeferralResolveInput{IssueID: issue.IssueID, Resolution: "accept", Note: "confirmed with the user"}); err != nil {
		t.Fatalf("RewriteDeferredIssueMarkdown: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `status: "resolved"`) {
		t.Fatalf("expected resolved status in frontmatter, got:\n%s", text)
	}
	if !strings.Contains(text, "## Resolution") || !strings.Contains(text, "confirmed with the user") {
		t.Fatalf("expected resolution section in rewritten markdown, got:\n%s", text)
	}
}

func TestRewriteDeferredIssueMarkdown_AtomicFailureLeavesOriginalFileUntouched(t *testing.T) {
	workspace := t.TempDir()
	dir := filepath.Join(workspace, "relurpify_cfg", "artifacts", "euclo", "deferred")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "issue-1.md")
	original := `---
issue_id: "issue-1"
workflow_id: "wf-1"
kind: "ambiguity"
severity: "low"
status: "open"
title: "Need context"
summary: "Original"
created_at: "2026-04-16T12:30:00Z"
---

# Need context
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	if err := RewriteDeferredIssueMarkdown(path, DeferralResolveInput{IssueID: "issue-1", Resolution: "accept", Note: "note"}); err == nil {
		t.Fatal("expected rewrite to fail in read-only directory")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(raw) != original {
		t.Fatalf("expected original file to remain untouched, got:\n%s", string(raw))
	}
}
