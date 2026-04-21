package telemetry_test

import (
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/agents/chainer/telemetry"
)

func TestEventRecorder_Record(t *testing.T) {
	recorder := telemetry.NewEventRecorder()

	event := telemetry.LinkStartEvent("task-1", "step1", 0, []string{"input"}, "output")
	err := recorder.Record(event)

	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	if recorder.Count() != 1 {
		t.Errorf("expected 1 event, got %d", recorder.Count())
	}
}

func TestEventRecorder_RecordNil(t *testing.T) {
	recorder := telemetry.NewEventRecorder()

	err := recorder.Record(nil)
	if err == nil {
		t.Fatal("expected error for nil event")
	}

	if recorder.Count() != 0 {
		t.Errorf("should not record nil event")
	}
}

func TestEventRecorder_AllEvents(t *testing.T) {
	recorder := telemetry.NewEventRecorder()

	e1 := telemetry.LinkStartEvent("task-1", "step1", 0, nil, "output")
	e2 := telemetry.LinkFinishEvent("task-1", "step1", 0, "output", "result")
	e3 := telemetry.LinkStartEvent("task-2", "step1", 0, nil, "output")

	recorder.Record(e1)
	recorder.Record(e2)
	recorder.Record(e3)

	events := recorder.AllEvents("task-1")
	if len(events) != 2 {
		t.Errorf("expected 2 events for task-1, got %d", len(events))
	}

	// Verify sorting for task-1
	if len(events) > 1 && !events[0].Timestamp.Before(events[1].Timestamp) {
		t.Fatal("events not sorted by timestamp")
	}

	events = recorder.AllEvents("task-2")
	if len(events) != 1 {
		t.Errorf("expected 1 event for task-2, got %d", len(events))
	}
}

func TestEventRecorder_RecordedEvents(t *testing.T) {
	recorder := telemetry.NewEventRecorder()

	e1 := telemetry.LinkStartEvent("task-1", "step1", 0, nil, "output")
	e2 := telemetry.LinkFinishEvent("task-1", "step1", 0, "output", "result")
	e3 := telemetry.LinkStartEvent("task-1", "step2", 1, nil, "output2")

	recorder.Record(e1)
	recorder.Record(e2)
	recorder.Record(e3)

	events := recorder.RecordedEvents("task-1", "step1")
	if len(events) != 2 {
		t.Errorf("expected 2 events for step1, got %d", len(events))
	}

	events = recorder.RecordedEvents("task-1", "step2")
	if len(events) != 1 {
		t.Errorf("expected 1 event for step2, got %d", len(events))
	}
}

func TestEventRecorder_EventsByKind(t *testing.T) {
	recorder := telemetry.NewEventRecorder()

	e1 := telemetry.LinkStartEvent("task-1", "step1", 0, nil, "output")
	e2 := telemetry.LinkFinishEvent("task-1", "step1", 0, "output", "result")
	e3 := telemetry.LinkErrorEvent("task-1", "step2", 1, "error", "NetworkError")

	recorder.Record(e1)
	recorder.Record(e2)
	recorder.Record(e3)

	startEvents := recorder.EventsByKind("task-1", telemetry.KindLinkStart)
	if len(startEvents) != 1 {
		t.Errorf("expected 1 LinkStart event, got %d", len(startEvents))
	}

	errorEvents := recorder.EventsByKind("task-1", telemetry.KindLinkError)
	if len(errorEvents) != 1 {
		t.Errorf("expected 1 LinkError event, got %d", len(errorEvents))
	}

	finishEvents := recorder.EventsByKind("task-1", telemetry.KindLinkFinish)
	if len(finishEvents) != 1 {
		t.Errorf("expected 1 LinkFinish event, got %d", len(finishEvents))
	}
}

func TestEventRecorder_Count(t *testing.T) {
	recorder := telemetry.NewEventRecorder()

	if recorder.Count() != 0 {
		t.Fatal("new recorder should have 0 events")
	}

	for i := 0; i < 5; i++ {
		recorder.Record(telemetry.LinkStartEvent("task-1", "step1", 0, nil, "output"))
	}

	if recorder.Count() != 5 {
		t.Errorf("expected 5 events, got %d", recorder.Count())
	}
}

func TestEventRecorder_Clear(t *testing.T) {
	recorder := telemetry.NewEventRecorder()

	recorder.Record(telemetry.LinkStartEvent("task-1", "step1", 0, nil, "output"))
	recorder.Record(telemetry.LinkFinishEvent("task-1", "step1", 0, "output", "result"))

	if recorder.Count() != 2 {
		t.Fatal("should have 2 events before clear")
	}

	recorder.Clear()

	if recorder.Count() != 0 {
		t.Fatal("should have 0 events after clear")
	}
}

func TestEventRecorder_NilRecorder(t *testing.T) {
	var recorder *telemetry.EventRecorder

	err := recorder.Record(nil)
	if err == nil {
		t.Fatal("expected error for nil recorder")
	}

	events := recorder.AllEvents("task-1")
	if events != nil {
		t.Fatal("expected nil for nil recorder")
	}

	if recorder.Count() != 0 {
		t.Fatal("count should be 0 for nil recorder")
	}
}

func TestEventRecorder_Summary(t *testing.T) {
	recorder := telemetry.NewEventRecorder()

	// Simulate task execution
	recorder.Record(telemetry.LinkStartEvent("task-1", "step1", 0, nil, "output1"))
	time.Sleep(10 * time.Millisecond)
	recorder.Record(telemetry.LinkFinishEvent("task-1", "step1", 0, "output1", "result1"))

	recorder.Record(telemetry.LinkStartEvent("task-1", "step2", 1, nil, "output2"))
	time.Sleep(10 * time.Millisecond)
	recorder.Record(telemetry.RetryAttemptEvent("task-1", "step2", 1, 1, 2, "parse error"))
	recorder.Record(telemetry.LinkFinishEvent("task-1", "step2", 1, "output2", "result2"))

	recorder.Record(telemetry.CompressionEvent("task-1", 500, 1000, "adaptive"))

	summary := recorder.Summary("task-1")

	if summary.TaskID != "task-1" {
		t.Errorf("expected taskID task-1, got %s", summary.TaskID)
	}

	if summary.TotalEvents != 6 {
		t.Errorf("expected 6 events, got %d", summary.TotalEvents)
	}

	if summary.SuccessfulLinks != 2 {
		t.Errorf("expected 2 successful links, got %d", summary.SuccessfulLinks)
	}

	if summary.RetryCount != 1 {
		t.Errorf("expected 1 retry, got %d", summary.RetryCount)
	}

	if summary.CompressionCount != 1 {
		t.Errorf("expected 1 compression event, got %d", summary.CompressionCount)
	}

	if summary.TotalDuration == 0 {
		t.Fatal("expected positive duration")
	}
}

func TestEventRecorder_SummaryEmpty(t *testing.T) {
	recorder := telemetry.NewEventRecorder()

	summary := recorder.Summary("task-1")

	if summary.TaskID != "task-1" {
		t.Errorf("expected taskID task-1")
	}

	if summary.TotalEvents != 0 {
		t.Errorf("expected 0 events for non-existent task")
	}
}

func TestEventRecorder_Sorting(t *testing.T) {
	recorder := telemetry.NewEventRecorder()

	// Record in reverse order
	e1 := telemetry.LinkStartEvent("task-1", "step1", 0, nil, "output")
	time.Sleep(5 * time.Millisecond)
	e2 := telemetry.LinkFinishEvent("task-1", "step1", 0, "output", "result")

	recorder.Record(e2) // Finish first
	recorder.Record(e1) // Start second (but was created first)

	events := recorder.AllEvents("task-1")
	if len(events) != 2 {
		t.Fatal("expected 2 events")
	}

	// Should be sorted by timestamp (original creation order)
	if !events[0].Timestamp.Before(events[1].Timestamp) {
		t.Fatal("events not sorted by timestamp")
	}
}

func TestEventRecorder_MultipleTasksIsolation(t *testing.T) {
	recorder := telemetry.NewEventRecorder()

	// Add events for task-1
	recorder.Record(telemetry.LinkStartEvent("task-1", "step1", 0, nil, "output"))
	recorder.Record(telemetry.LinkFinishEvent("task-1", "step1", 0, "output", "result"))

	// Add events for task-2
	recorder.Record(telemetry.LinkStartEvent("task-2", "step1", 0, nil, "output"))
	recorder.Record(telemetry.LinkErrorEvent("task-2", "step1", 0, "error", "NetworkError"))

	// Verify isolation
	task1Events := recorder.AllEvents("task-1")
	if len(task1Events) != 2 {
		t.Errorf("expected 2 events for task-1, got %d", len(task1Events))
	}

	task2Events := recorder.AllEvents("task-2")
	if len(task2Events) != 2 {
		t.Errorf("expected 2 events for task-2, got %d", len(task2Events))
	}

	// Verify task-1 doesn't include task-2 events
	for _, e := range task1Events {
		if e.TaskID != "task-1" {
			t.Fatal("task-1 event has wrong taskID")
		}
	}
}
