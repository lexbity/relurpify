package local

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

func TestDeferralsSurfaceRoutineUsesStateIssues(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.deferred_execution_issues", []eucloruntime.DeferredExecutionIssue{
		{IssueID: "a", WorkflowID: "wf-1", Kind: eucloruntime.DeferredIssueAmbiguity, Severity: eucloruntime.DeferredIssueSeverityLow, Status: eucloruntime.DeferredIssueStatusOpen, Title: "A", RecommendedNextAction: "next-a"},
		{IssueID: "b", WorkflowID: "wf-1", Kind: eucloruntime.DeferredIssueVerificationConcern, Severity: eucloruntime.DeferredIssueSeverityHigh, Status: eucloruntime.DeferredIssueStatusAcknowledged, Title: "B", RecommendedNextAction: "next-b"},
	})

	artifacts, err := (DeferralsSurfaceRoutine{}).Execute(context.Background(), euclorelurpic.RoutineInput{State: state})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].Kind != euclotypes.ArtifactKindDeferralsSurface {
		t.Fatalf("unexpected artifacts: %+v", artifacts)
	}
	summary, ok := artifacts[0].Payload.(eucloruntime.DeferralsSurfaceSummary)
	if !ok {
		t.Fatalf("unexpected payload type: %T", artifacts[0].Payload)
	}
	if summary.TotalOpen != 2 {
		t.Fatalf("TotalOpen = %d, want 2", summary.TotalOpen)
	}
	if summary.ByKind[string(eucloruntime.DeferredIssueAmbiguity)] != 1 {
		t.Fatalf("unexpected kind counts: %+v", summary.ByKind)
	}
	if got, ok := state.Get("euclo.deferrals_surface"); !ok {
		t.Fatal("expected summary in state")
	} else if stored, ok := got.(eucloruntime.DeferralsSurfaceSummary); !ok || stored.TotalOpen != 2 {
		t.Fatalf("unexpected stored summary: %#v", got)
	}
}

func TestDeferralsSurfaceRoutineFallsBackToWorkspace(t *testing.T) {
	workspace := t.TempDir()
	task := &core.Task{Context: map[string]any{"workspace": workspace}}
	seedState := core.NewContext()
	issues := []eucloruntime.DeferredExecutionIssue{
		{IssueID: "issue-1", WorkflowID: "wf-1", Kind: eucloruntime.DeferredIssueAmbiguity, Severity: eucloruntime.DeferredIssueSeverityMedium, Status: eucloruntime.DeferredIssueStatusOpen, Title: "Issue", RecommendedNextAction: "next"},
	}
	written := eucloruntime.PersistDeferredExecutionIssuesToWorkspace(task, seedState, issues)
	if len(written) != 1 {
		t.Fatalf("persisted %d issues, want 1", len(written))
	}

	state := core.NewContext()
	artifacts, err := (DeferralsSurfaceRoutine{}).Execute(context.Background(), euclorelurpic.RoutineInput{
		Task:  task,
		State: state,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	summary, ok := artifacts[0].Payload.(eucloruntime.DeferralsSurfaceSummary)
	if !ok || summary.TotalOpen != 1 {
		t.Fatalf("unexpected summary: %#v", artifacts[0].Payload)
	}
}

func TestNewDefaultCapabilityRegistryContainsDeferralsSurface(t *testing.T) {
	env := agentenv.AgentEnvironment{Registry: capability.NewRegistry(), Config: &core.Config{Name: "test", Model: "stub"}}
	reg := NewDeferralsSurfaceCapability(env)
	if reg == nil {
		t.Fatal("expected capability")
	}
	if reg.Descriptor().ID != euclorelurpic.CapabilityDeferralsSurface {
		t.Fatalf("unexpected capability id: %s", reg.Descriptor().ID)
	}
}
