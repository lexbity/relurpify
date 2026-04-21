package capabilities

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// TestNewProvenanceTracker tests the constructor
func TestNewProvenanceTracker(t *testing.T) {
	tracker := NewProvenanceTracker("task-123")

	if tracker == nil {
		t.Fatal("expected non-nil tracker")
	}

	if tracker.taskID != "task-123" {
		t.Errorf("expected task ID task-123, got %s", tracker.taskID)
	}

	if tracker.records == nil {
		t.Error("expected records to be initialized")
	}
}

// TestProvenanceTrackerAllRecordsNil tests nil receiver
func TestProvenanceTrackerAllRecordsNil(t *testing.T) {
	var nilTracker *ProvenanceTracker
	records := nilTracker.AllRecords()
	if records != nil {
		t.Error("expected nil for nil tracker")
	}
}

// TestProvenanceTrackerCountNil tests nil receiver
func TestProvenanceTrackerCountNil(t *testing.T) {
	var nilTracker *ProvenanceTracker
	if nilTracker.Count() != 0 {
		t.Error("expected 0 for nil tracker")
	}
}

// TestProvenanceTrackerClearNil tests nil receiver
func TestProvenanceTrackerClearNil(t *testing.T) {
	var nilTracker *ProvenanceTracker
	// Should not panic
	nilTracker.Clear()
}

// TestProvenanceTrackerSummaryNil tests nil receiver
func TestProvenanceTrackerSummaryNil(t *testing.T) {
	var nilTracker *ProvenanceTracker
	summary := nilTracker.Summary()
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if summary.TaskID != "" {
		t.Error("expected empty task ID for nil tracker")
	}
}

// TestProvenanceTrackerClear tests the Clear method
func TestProvenanceTrackerClear(t *testing.T) {
	tracker := NewProvenanceTracker("task-123")

	// Add some records
	tracker.Record("link1", "tool1", core.InsertionActionDirect)
	tracker.Record("link2", "tool2", core.InsertionActionSummarized)

	if tracker.Count() != 2 {
		t.Fatalf("expected 2 records, got %d", tracker.Count())
	}

	// Clear
	tracker.Clear()

	if tracker.Count() != 0 {
		t.Errorf("expected 0 records after clear, got %d", tracker.Count())
	}

	// Verify records slice is reset (not nil, but empty)
	if tracker.records == nil {
		t.Error("expected records to be reinitialized after clear")
	}
}

// TestProvenanceTrackerSummary tests the Summary method
func TestProvenanceTrackerSummary(t *testing.T) {
	t.Run("empty tracker summary", func(t *testing.T) {
		tracker := NewProvenanceTracker("task-empty")
		summary := tracker.Summary()

		if summary.TaskID != "task-empty" {
			t.Errorf("expected task ID task-empty, got %s", summary.TaskID)
		}
		if summary.TotalTools != 0 {
			t.Errorf("expected 0 total tools, got %d", summary.TotalTools)
		}
	})

	t.Run("tracker with records summary", func(t *testing.T) {
		tracker := NewProvenanceTracker("task-full")

		tracker.Record("link1", "tool1", core.InsertionActionDirect)
		tracker.Record("link2", "tool1", core.InsertionActionDirect) // Same tool
		tracker.Record("link3", "tool2", core.InsertionActionSummarized)
		tracker.Record("link4", "tool3", core.InsertionActionDenied)
		tracker.Record("link5", "tool4", core.InsertionActionMetadataOnly)
		tracker.Record("link6", "tool5", core.InsertionActionHITLRequired)

		summary := tracker.Summary()

		if summary.TaskID != "task-full" {
			t.Errorf("expected task ID task-full, got %s", summary.TaskID)
		}
		if summary.TotalTools != 6 {
			t.Errorf("expected 6 total tools, got %d", summary.TotalTools)
		}
		if summary.UniqueTools != 5 {
			t.Errorf("expected 5 unique tools, got %d", summary.UniqueTools)
		}
		if summary.DirectInclusions != 2 {
			t.Errorf("expected 2 direct inclusions, got %d", summary.DirectInclusions)
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
	})
}

// TestProvenanceTrackerWrapResult tests the WrapResult method
func TestProvenanceTrackerWrapResult(t *testing.T) {
	t.Run("nil tracker wrap result", func(t *testing.T) {
		var nilTracker *ProvenanceTracker
		result := nilTracker.WrapResult("tool1", "data", core.InsertionActionDirect)

		if result == nil {
			t.Fatal("expected non-nil result")
		}

		if result["tool_id"] != "tool1" {
			t.Error("expected tool_id in result")
		}
		if result["result"] != "data" {
			t.Error("expected result data")
		}
		if result["insertion_action"] != core.InsertionActionDirect {
			t.Error("expected insertion_action")
		}
	})

	t.Run("valid tracker wrap result", func(t *testing.T) {
		tracker := NewProvenanceTracker("task-123")
		result := tracker.WrapResult("tool1", map[string]any{"key": "value"}, core.InsertionActionSummarized)

		if result == nil {
			t.Fatal("expected non-nil result")
		}

		if result["tool_id"] != "tool1" {
			t.Error("expected tool_id in result")
		}

		wrappedData, ok := result["result"].(map[string]any)
		if !ok {
			t.Fatal("expected result to be map")
		}
		if wrappedData["key"] != "value" {
			t.Error("expected data to be preserved")
		}
	})
}
