package local

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/guidance"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

func TestDeferralsResolveRoutineResolvesObservationAndRewritesMarkdown(t *testing.T) {
	workspace := t.TempDir()
	task := &core.Task{Context: map[string]any{"workspace": workspace}}
	state := core.NewContext()
	issue := eucloruntime.DeferredExecutionIssue{
		IssueID:               "issue-1",
		WorkflowID:            "wf-1",
		Kind:                  eucloruntime.DeferredIssueAmbiguity,
		Severity:              eucloruntime.DeferredIssueSeverityHigh,
		Status:                eucloruntime.DeferredIssueStatusOpen,
		Title:                 "Need clarification",
		Summary:               "Clarify the ambiguous requirement.",
		RecommendedNextAction: "ask the user",
		CreatedAt:             time.Date(2026, 4, 16, 12, 30, 0, 0, time.UTC),
	}
	written := eucloruntime.PersistDeferredExecutionIssuesToWorkspace(task, state, []eucloruntime.DeferredExecutionIssue{issue})
	if len(written) != 1 {
		t.Fatalf("persisted %d issues, want 1", len(written))
	}
	state.Set("euclo.deferred_execution_issues", []eucloruntime.DeferredExecutionIssue{written[0]})
	state.Set("euclo.deferral_resolve_input", eucloruntime.DeferralResolveInput{
		IssueID:    "issue-1",
		Resolution: "accept",
		Note:       "confirmed with the user",
	})
	dp := &guidance.DeferralPlan{ID: "dp-1", WorkflowID: "wf-1"}
	dp.AddObservation(guidance.EngineeringObservation{ID: "issue-1", Title: "Need clarification", Description: "Clarify the ambiguous requirement."})

	artifacts, err := (DeferralsResolveRoutine{DeferralPlan: dp}).Execute(context.Background(), euclorelurpic.RoutineInput{
		Task:  task,
		State: state,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	if artifacts[0].Kind != euclotypes.ArtifactKindDeferralResolved {
		t.Fatalf("unexpected artifact kind: %s", artifacts[0].Kind)
	}
	if !dp.IsEmpty() {
		t.Fatal("expected deferral plan observation to be resolved")
	}
	raw, err := os.ReadFile(written[0].WorkspaceArtifactPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `status: "resolved"`) {
		t.Fatalf("expected resolved frontmatter, got:\n%s", text)
	}
	if got, ok := state.Get("euclo.deferred_execution_issues"); !ok {
		t.Fatal("expected deferred issues in state")
	} else if issues, ok := got.([]eucloruntime.DeferredExecutionIssue); !ok || len(issues) != 1 || issues[0].Status != eucloruntime.DeferredIssueStatusResolved {
		t.Fatalf("unexpected updated issues: %#v", got)
	}
	if got, ok := state.Get("euclo.deferral_resolved"); !ok {
		t.Fatal("expected deferral resolved payload in state")
	} else if payload, ok := got.(map[string]any); !ok || payload["status"] != string(eucloruntime.DeferredIssueStatusResolved) {
		t.Fatalf("unexpected resolved payload: %#v", got)
	}
}

func TestDeferralsResolveRoutineRejectsInvalidResolution(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.deferral_resolve_input", eucloruntime.DeferralResolveInput{
		IssueID:    "issue-1",
		Resolution: "invalid",
	})

	_, err := (DeferralsResolveRoutine{}).Execute(context.Background(), euclorelurpic.RoutineInput{State: state})
	if err == nil || !strings.Contains(err.Error(), "unknown deferral resolution") {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestDeferralsResolveRoutineReturnsIssueNotFound(t *testing.T) {
	workspace := t.TempDir()
	task := &core.Task{Context: map[string]any{"workspace": workspace}}
	state := core.NewContext()
	state.Set("euclo.deferral_resolve_input", eucloruntime.DeferralResolveInput{
		IssueID:    "missing",
		Resolution: "accept",
	})
	state.Set("euclo.deferred_execution_issues", []eucloruntime.DeferredExecutionIssue{
		{IssueID: "other", WorkflowID: "wf-1", Kind: eucloruntime.DeferredIssueAmbiguity, Severity: eucloruntime.DeferredIssueSeverityLow, Status: eucloruntime.DeferredIssueStatusOpen, Title: "Other", CreatedAt: time.Now().UTC()},
	})

	_, err := (DeferralsResolveRoutine{}).Execute(context.Background(), euclorelurpic.RoutineInput{Task: task, State: state})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestDeferralsResolveRoutineAllowsNilBroker(t *testing.T) {
	workspace := t.TempDir()
	task := &core.Task{Context: map[string]any{"workspace": workspace}}
	state := core.NewContext()
	issue := eucloruntime.DeferredExecutionIssue{
		IssueID:    "issue-1",
		WorkflowID: "wf-1",
		Kind:       eucloruntime.DeferredIssueAmbiguity,
		Severity:   eucloruntime.DeferredIssueSeverityLow,
		Status:     eucloruntime.DeferredIssueStatusOpen,
		Title:      "Need clarification",
		CreatedAt:  time.Now().UTC(),
	}
	written := eucloruntime.PersistDeferredExecutionIssuesToWorkspace(task, state, []eucloruntime.DeferredExecutionIssue{issue})
	state.Set("euclo.deferred_execution_issues", written)
	state.Set("euclo.deferral_resolve_input", eucloruntime.DeferralResolveInput{IssueID: "issue-1", Resolution: "accept"})

	artifacts, err := (DeferralsResolveRoutine{}).Execute(context.Background(), euclorelurpic.RoutineInput{Task: task, State: state})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
}

func TestDeferralsResolveRoutineUsesGuidanceBroker(t *testing.T) {
	workspace := t.TempDir()
	task := &core.Task{Context: map[string]any{"workspace": workspace}}
	state := core.NewContext()
	persisted := eucloruntime.PersistDeferredExecutionIssuesToWorkspace(task, state, []eucloruntime.DeferredExecutionIssue{{
		IssueID:               "issue-1",
		WorkflowID:            "wf-1",
		Kind:                  eucloruntime.DeferredIssueAmbiguity,
		Severity:              eucloruntime.DeferredIssueSeverityLow,
		Status:                eucloruntime.DeferredIssueStatusOpen,
		Title:                 "Need clarification",
		RecommendedNextAction: "ask the user",
		CreatedAt:             time.Now().UTC(),
	}})
	state.Set("euclo.deferred_execution_issues", persisted)
	state.Set("euclo.deferral_resolve_input", eucloruntime.DeferralResolveInput{IssueID: "issue-1", Resolution: "accept"})
	dp := &guidance.DeferralPlan{ID: "dp-1", WorkflowID: "wf-1"}
	dp.AddObservation(guidance.EngineeringObservation{ID: "issue-1", Title: "Need clarification"})
	broker := guidance.NewGuidanceBroker(5 * time.Second)
	events, cancel := broker.Subscribe(4)
	defer cancel()

	_, err := (DeferralsResolveRoutine{DeferralPlan: dp, GuidanceBroker: broker}).Execute(context.Background(), euclorelurpic.RoutineInput{Task: task, State: state})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var gotResolved bool
	for i := 0; i < 2; i++ {
		select {
		case event := <-events:
			if event.Type == guidance.GuidanceEventResolved {
				gotResolved = true
			}
		default:
		}
	}
	if !gotResolved {
		t.Fatal("expected resolved guidance event")
	}
}
