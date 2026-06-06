package framework

import (
	"testing"
	"time"

	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// TestTelemetryEventEmission validates that telemetry events can be emitted
// and captured by the telemetry sink.
func TestTelemetryEventEmission(t *testing.T) {
	t.Run("event can be emitted to sink", func(t *testing.T) {
		sink := &recordingTelemetrySink{}

		event := telemetry.Event{
			Type:      telemetry.EventAgentStart,
			NodeID:    "node-1",
			TaskID:    "task-1",
			Message:   "agent started",
			Timestamp: time.Now().UTC(),
			Metadata: map[string]interface{}{
				"status": "running",
			},
		}

		sink.Emit(event)

		events := sink.Events()
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}

		if events[0].Type != telemetry.EventAgentStart {
			t.Errorf("expected event type %s, got %s", telemetry.EventAgentStart, events[0].Type)
		}
		if events[0].NodeID != "node-1" {
			t.Errorf("expected node ID 'node-1', got %s", events[0].NodeID)
		}
		if events[0].Message != "agent started" {
			t.Errorf("expected message 'agent started', got %s", events[0].Message)
		}
	})

	t.Run("multiple events can be emitted", func(t *testing.T) {
		sink := &recordingTelemetrySink{}

		events := []telemetry.Event{
			{
				Type:      telemetry.EventAgentStart,
				NodeID:    "node-1",
				TaskID:    "task-1",
				Message:   "agent started",
				Timestamp: time.Now().UTC(),
			},
			{
				Type:      telemetry.EventLLMPrompt,
				NodeID:    "node-2",
				TaskID:    "task-1",
				Message:   "LLM prompt",
				Timestamp: time.Now().UTC(),
			},
			{
				Type:      telemetry.EventLLMResponse,
				NodeID:    "node-2",
				TaskID:    "task-1",
				Message:   "LLM response",
				Timestamp: time.Now().UTC(),
			},
		}

		for _, event := range events {
			sink.Emit(event)
		}

		captured := sink.Events()
		if len(captured) != len(events) {
			t.Fatalf("expected %d events, got %d", len(events), len(captured))
		}
	})

	t.Run("event metadata is preserved", func(t *testing.T) {
		sink := &recordingTelemetrySink{}

		event := telemetry.Event{
			Type:      telemetry.EventToolCall,
			NodeID:    "node-1",
			TaskID:    "task-1",
			Message:   "tool called",
			Timestamp: time.Now().UTC(),
			Metadata: map[string]interface{}{
				"tool_name": "read_file",
				"file_path": "/test/file.txt",
				"status":    "success",
			},
		}

		sink.Emit(event)

		events := sink.Events()
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}

		if events[0].Metadata == nil {
			t.Fatal("metadata should not be nil")
		}

		if events[0].Metadata["tool_name"] != "read_file" {
			t.Errorf("expected tool_name 'read_file', got %v", events[0].Metadata["tool_name"])
		}
		if events[0].Metadata["file_path"] != "/test/file.txt" {
			t.Errorf("expected file_path '/test/file.txt', got %v", events[0].Metadata["file_path"])
		}
	})
}

// TestTelemetrySinkCollection validates that the telemetry sink collects
// events correctly and provides access to them.
func TestTelemetrySinkCollection(t *testing.T) {
	t.Run("sink returns copy of events", func(t *testing.T) {
		sink := &recordingTelemetrySink{}

		event := telemetry.Event{
			Type:      telemetry.EventAgentStart,
			NodeID:    "node-1",
			TaskID:    "task-1",
			Message:   "agent started",
			Timestamp: time.Now().UTC(),
		}

		sink.Emit(event)

		events1 := sink.Events()
		events2 := sink.Events()

		if len(events1) != len(events2) {
			t.Errorf("event counts should match: %d vs %d", len(events1), len(events2))
		}

		// Modify the first slice
		events1[0].Message = "modified"

		// The second slice should not be affected (they are copies)
		if events2[0].Message == "modified" {
			t.Error("events should return copies, not references to the same slice")
		}
	})

	t.Run("sink clear removes all events", func(t *testing.T) {
		sink := &recordingTelemetrySink{}

		for i := 0; i < 5; i++ {
			sink.Emit(telemetry.Event{
				Type:      telemetry.EventAgentStart,
				NodeID:    "node-1",
				TaskID:    "task-1",
				Message:   "event",
				Timestamp: time.Now().UTC(),
			})
		}

		if len(sink.Events()) != 5 {
			t.Fatalf("expected 5 events before clear, got %d", len(sink.Events()))
		}

		sink.Clear()

		if len(sink.Events()) != 0 {
			t.Errorf("expected 0 events after clear, got %d", len(sink.Events()))
		}
	})

	t.Run("sink is thread-safe", func(t *testing.T) {
		sink := &recordingTelemetrySink{}

		done := make(chan bool)
		for i := 0; i < 10; i++ {
			go func(id int) {
				for j := 0; j < 100; j++ {
					sink.Emit(telemetry.Event{
						Type:      telemetry.EventAgentStart,
						NodeID:    "node-1",
						TaskID:    "task-1",
						Message:   "event",
						Timestamp: time.Now().UTC(),
					})
				}
				done <- true
			}(i)
		}

		for i := 0; i < 10; i++ {
			<-done
		}

		events := sink.Events()
		if len(events) != 1000 {
			t.Errorf("expected 1000 events from concurrent writes, got %d", len(events))
		}
	})
}

// TestTelemetryEventFiltering validates that telemetry events can be
// filtered based on their properties.
func TestTelemetryEventFiltering(t *testing.T) {
	t.Run("events can be filtered by type", func(t *testing.T) {
		sink := &recordingTelemetrySink{}

		events := []telemetry.Event{
			{Type: telemetry.EventAgentStart, NodeID: "node-1", TaskID: "task-1", Message: "start", Timestamp: time.Now().UTC()},
			{Type: telemetry.EventLLMPrompt, NodeID: "node-2", TaskID: "task-1", Message: "prompt", Timestamp: time.Now().UTC()},
			{Type: telemetry.EventLLMResponse, NodeID: "node-2", TaskID: "task-1", Message: "response", Timestamp: time.Now().UTC()},
			{Type: telemetry.EventToolCall, NodeID: "node-3", TaskID: "task-1", Message: "call", Timestamp: time.Now().UTC()},
		}

		for _, event := range events {
			sink.Emit(event)
		}

		allEvents := sink.Events()

		// Filter by LLM events
		var llmEvents []telemetry.Event
		for _, event := range allEvents {
			if event.Type == telemetry.EventLLMPrompt || event.Type == telemetry.EventLLMResponse {
				llmEvents = append(llmEvents, event)
			}
		}

		if len(llmEvents) != 2 {
			t.Errorf("expected 2 LLM events, got %d", len(llmEvents))
		}
	})

	t.Run("events can be filtered by node ID", func(t *testing.T) {
		sink := &recordingTelemetrySink{}

		events := []telemetry.Event{
			{Type: telemetry.EventAgentStart, NodeID: "node-1", TaskID: "task-1", Message: "start", Timestamp: time.Now().UTC()},
			{Type: telemetry.EventLLMPrompt, NodeID: "node-2", TaskID: "task-1", Message: "prompt", Timestamp: time.Now().UTC()},
			{Type: telemetry.EventLLMResponse, NodeID: "node-2", TaskID: "task-1", Message: "response", Timestamp: time.Now().UTC()},
		}

		for _, event := range events {
			sink.Emit(event)
		}

		allEvents := sink.Events()

		// Filter by node ID
		var node2Events []telemetry.Event
		for _, event := range allEvents {
			if event.NodeID == "node-2" {
				node2Events = append(node2Events, event)
			}
		}

		if len(node2Events) != 2 {
			t.Errorf("expected 2 events from node-2, got %d", len(node2Events))
		}
	})

	t.Run("events can be filtered by task ID", func(t *testing.T) {
		sink := &recordingTelemetrySink{}

		events := []telemetry.Event{
			{Type: telemetry.EventAgentStart, NodeID: "node-1", TaskID: "task-1", Message: "start", Timestamp: time.Now().UTC()},
			{Type: telemetry.EventLLMPrompt, NodeID: "node-2", TaskID: "task-1", Message: "prompt", Timestamp: time.Now().UTC()},
			{Type: telemetry.EventLLMResponse, NodeID: "node-2", TaskID: "task-2", Message: "response", Timestamp: time.Now().UTC()},
		}

		for _, event := range events {
			sink.Emit(event)
		}

		allEvents := sink.Events()

		// Filter by task ID
		var task1Events []telemetry.Event
		for _, event := range allEvents {
			if event.TaskID == "task-1" {
				task1Events = append(task1Events, event)
			}
		}

		if len(task1Events) != 2 {
			t.Errorf("expected 2 events from task-1, got %d", len(task1Events))
		}
	})

	t.Run("events can be filtered by metadata", func(t *testing.T) {
		sink := &recordingTelemetrySink{}

		events := []telemetry.Event{
			{
				Type:      telemetry.EventToolCall,
				NodeID:    "node-1",
				TaskID:    "task-1",
				Message:   "call",
				Timestamp: time.Now().UTC(),
				Metadata:  map[string]interface{}{"status": "success"},
			},
			{
				Type:      telemetry.EventToolCall,
				NodeID:    "node-2",
				TaskID:    "task-1",
				Message:   "call",
				Timestamp: time.Now().UTC(),
				Metadata:  map[string]interface{}{"status": "failed"},
			},
			{
				Type:      telemetry.EventToolCall,
				NodeID:    "node-3",
				TaskID:    "task-1",
				Message:   "call",
				Timestamp: time.Now().UTC(),
				Metadata:  map[string]interface{}{"status": "success"},
			},
		}

		for _, event := range events {
			sink.Emit(event)
		}

		allEvents := sink.Events()

		// Filter by metadata status
		var successEvents []telemetry.Event
		for _, event := range allEvents {
			if event.Metadata != nil && event.Metadata["status"] == "success" {
				successEvents = append(successEvents, event)
			}
		}

		if len(successEvents) != 2 {
			t.Errorf("expected 2 events with status 'success', got %d", len(successEvents))
		}
	})
}
