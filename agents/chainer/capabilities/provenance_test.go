package capabilities_test

import (
	"testing"

	"github.com/lexcodex/relurpify/agents/chainer/capabilities"
	"github.com/lexcodex/relurpify/framework/core"
)

func TestProvenanceTracker_NewTracker(t *testing.T) {
	tracker := capabilities.NewProvenanceTracker("task-1")

	if tracker == nil {
		t.Fatal("expected tracker")
	}

	if tracker.Count() != 0 {
		t.Fatal("new tracker should have 0 records")
	}
}

func TestProvenanceTracker_Record(t *testing.T) {
	tracker := capabilities.NewProvenanceTracker("task-1")

	err := tracker.Record("link1", "tool1", core.InsertionActionDirect)

	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	if tracker.Count() != 1 {
		t.Errorf("expected 1 record, got %d", tracker.Count())
	}
}

func TestProvenanceTracker_RecordWithApproval(t *testing.T) {
	tracker := capabilities.NewProvenanceTracker("task-1")

	err := tracker.RecordWithApproval("link1", "tool1", core.InsertionActionHITLRequired, "user@example.com")

	if err != nil {
		t.Fatalf("RecordWithApproval failed: %v", err)
	}

	if tracker.Count() != 1 {
		t.Errorf("expected 1 record, got %d", tracker.Count())
	}

	records := tracker.AllRecords()
	if records[0].ApprovedBy != "user@example.com" {
		t.Errorf("expected approval from user@example.com, got %s", records[0].ApprovedBy)
	}
}

func TestProvenanceTracker_AllRecords(t *testing.T) {
	tracker := capabilities.NewProvenanceTracker("task-1")

	tracker.Record("link1", "tool1", core.InsertionActionDirect)
	tracker.Record("link1", "tool2", core.InsertionActionSummarized)
	tracker.Record("link2", "tool1", core.InsertionActionMetadataOnly)

	records := tracker.AllRecords()

	if len(records) != 3 {
		t.Errorf("expected 3 records, got %d", len(records))
	}

	// Verify records are in order
	if records[0].ToolID != "tool1" || records[1].ToolID != "tool2" || records[2].ToolID != "tool1" {
		t.Fatal("records not in expected order")
	}
}

func TestProvenanceTracker_RecordsByLink(t *testing.T) {
	tracker := capabilities.NewProvenanceTracker("task-1")

	tracker.Record("link1", "tool1", core.InsertionActionDirect)
	tracker.Record("link1", "tool2", core.InsertionActionSummarized)
	tracker.Record("link2", "tool1", core.InsertionActionDirect)

	link1Records := tracker.RecordsByLink("link1")
	if len(link1Records) != 2 {
		t.Errorf("expected 2 records for link1, got %d", len(link1Records))
	}

	link2Records := tracker.RecordsByLink("link2")
	if len(link2Records) != 1 {
		t.Errorf("expected 1 record for link2, got %d", len(link2Records))
	}
}

func TestProvenanceTracker_RecordsByTool(t *testing.T) {
	tracker := capabilities.NewProvenanceTracker("task-1")

	tracker.Record("link1", "tool1", core.InsertionActionDirect)
	tracker.Record("link1", "tool2", core.InsertionActionSummarized)
	tracker.Record("link2", "tool1", core.InsertionActionDirect)

	tool1Records := tracker.RecordsByTool("tool1")
	if len(tool1Records) != 2 {
		t.Errorf("expected 2 records for tool1, got %d", len(tool1Records))
	}

	tool2Records := tracker.RecordsByTool("tool2")
	if len(tool2Records) != 1 {
		t.Errorf("expected 1 record for tool2, got %d", len(tool2Records))
	}
}

func TestProvenanceTracker_Count(t *testing.T) {
	tracker := capabilities.NewProvenanceTracker("task-1")

	if tracker.Count() != 0 {
		t.Fatal("new tracker should have 0 count")
	}

	for i := 0; i < 5; i++ {
		tracker.Record("link", "tool", core.InsertionActionDirect)
	}

	if tracker.Count() != 5 {
		t.Errorf("expected 5 records, got %d", tracker.Count())
	}
}

func TestProvenanceTracker_Clear(t *testing.T) {
	tracker := capabilities.NewProvenanceTracker("task-1")

	tracker.Record("link", "tool", core.InsertionActionDirect)
	tracker.Record("link", "tool", core.InsertionActionDirect)

	if tracker.Count() != 2 {
		t.Fatal("should have 2 records before clear")
	}

	tracker.Clear()

	if tracker.Count() != 0 {
		t.Fatal("should have 0 records after clear")
	}
}

func TestProvenanceTracker_Summary(t *testing.T) {
	tracker := capabilities.NewProvenanceTracker("task-1")

	tracker.Record("link1", "tool1", core.InsertionActionDirect)
	tracker.Record("link1", "tool2", core.InsertionActionSummarized)
	tracker.Record("link2", "tool1", core.InsertionActionMetadataOnly)
	tracker.Record("link2", "tool3", core.InsertionActionHITLRequired)
	tracker.Record("link2", "tool4", core.InsertionActionDenied)

	summary := tracker.Summary()

	if summary.TaskID != "task-1" {
		t.Errorf("expected task-1, got %s", summary.TaskID)
	}

	if summary.TotalTools != 5 {
		t.Errorf("expected 5 total tools, got %d", summary.TotalTools)
	}

	if summary.UniqueTools != 4 {
		t.Errorf("expected 4 unique tools, got %d", summary.UniqueTools)
	}

	if summary.DirectInclusions != 1 {
		t.Errorf("expected 1 direct inclusion, got %d", summary.DirectInclusions)
	}

	if summary.SummarizedResults != 1 {
		t.Errorf("expected 1 summarized result, got %d", summary.SummarizedResults)
	}

	if summary.MetadataOnlyResults != 1 {
		t.Errorf("expected 1 metadata-only result, got %d", summary.MetadataOnlyResults)
	}

	if summary.ApprovalRequired != 1 {
		t.Errorf("expected 1 approval required, got %d", summary.ApprovalRequired)
	}

	if summary.Denied != 1 {
		t.Errorf("expected 1 denied, got %d", summary.Denied)
	}
}

func TestProvenanceTracker_Summary_Empty(t *testing.T) {
	tracker := capabilities.NewProvenanceTracker("task-1")

	summary := tracker.Summary()

	if summary.TaskID != "task-1" {
		t.Errorf("expected task-1, got %s", summary.TaskID)
	}

	if summary.TotalTools != 0 {
		t.Errorf("expected 0 total tools, got %d", summary.TotalTools)
	}

	if summary.UniqueTools != 0 {
		t.Errorf("expected 0 unique tools, got %d", summary.UniqueTools)
	}
}

func TestProvenanceTracker_WrapResult(t *testing.T) {
	tracker := capabilities.NewProvenanceTracker("task-1")

	result := "test result"
	wrapped := tracker.WrapResult("tool1", result, core.InsertionActionDirect)

	if wrapped["result"] != result {
		t.Errorf("expected wrapped result %v, got %v", result, wrapped["result"])
	}

	if wrapped["insertion_action"] != core.InsertionActionDirect {
		t.Errorf("expected direct action, got %v", wrapped["insertion_action"])
	}

	if wrapped["tool_id"] != "tool1" {
		t.Errorf("expected tool_id tool1, got %v", wrapped["tool_id"])
	}
}

func TestProvenanceTracker_NilTracker(t *testing.T) {
	var tracker *capabilities.ProvenanceTracker

	err := tracker.Record("link", "tool", core.InsertionActionDirect)
	if err == nil {
		t.Fatal("nil tracker should error on record")
	}

	records := tracker.AllRecords()
	if records != nil {
		t.Fatal("nil tracker should return nil for AllRecords")
	}

	if tracker.Count() != 0 {
		t.Fatal("nil tracker should return 0 for Count")
	}
}

func TestProvenanceTracker_Isolation(t *testing.T) {
	// Verify records are isolated between trackers
	tracker1 := capabilities.NewProvenanceTracker("task-1")
	tracker2 := capabilities.NewProvenanceTracker("task-2")

	tracker1.Record("link1", "tool1", core.InsertionActionDirect)
	tracker2.Record("link1", "tool1", core.InsertionActionSummarized)

	records1 := tracker1.AllRecords()
	records2 := tracker2.AllRecords()

	if len(records1) != 1 || len(records2) != 1 {
		t.Fatal("trackers should have independent records")
	}

	if records1[0].InsertionAction != core.InsertionActionDirect {
		t.Fatal("tracker1 should have direct action")
	}

	if records2[0].InsertionAction != core.InsertionActionSummarized {
		t.Fatal("tracker2 should have summarized action")
	}
}
