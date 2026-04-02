package runtime

import (
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestBuildDeferredExecutionIssues_IncludesExecutionWaiver(t *testing.T) {
	state := core.NewContext()
	now := time.Now().UTC()
	state.Set("euclo.execution_waiver", ExecutionWaiver{
		WaiverID:   "waiver-1",
		Kind:       WaiverKindVerification,
		Reason:     "operator approved running without executable verification",
		RunID:      "run-1",
		CreatedAt:  now,
		ArchaeoRef: "archaeo:waiver/1",
	})
	uow := UnitOfWork{
		WorkflowID:  "wf-1",
		RunID:       "run-1",
		ExecutionID: "exec-1",
	}

	issues := BuildDeferredExecutionIssues(nil, uow, state, now)
	if len(issues) != 1 {
		t.Fatalf("expected 1 waiver issue, got %d", len(issues))
	}
	issue := issues[0]
	if issue.Kind != DeferredIssueWaiver {
		t.Fatalf("expected waiver issue kind, got %q", issue.Kind)
	}
	if issue.Status != DeferredIssueStatusAcknowledged {
		t.Fatalf("expected acknowledged status, got %q", issue.Status)
	}
	if issue.RunID != "run-1" {
		t.Fatalf("expected run id to propagate, got %q", issue.RunID)
	}
	if got := issue.ArchaeoRefs["waiver"]; len(got) != 1 || got[0] != "archaeo:waiver/1" {
		t.Fatalf("unexpected archaeo refs: %#v", issue.ArchaeoRefs)
	}
}
