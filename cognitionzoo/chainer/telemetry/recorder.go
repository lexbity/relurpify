package telemetry

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// EventRecorder captures and stores execution events for later querying.
//
// EventRecorder is thread-safe and can be shared across execution stages.
// It stores events in-memory and provides query methods for analysis and debugging.
type EventRecorder struct {
	mu     sync.RWMutex
	events []*ChainerEvent
}

// NewEventRecorder creates a new event recorder.
func NewEventRecorder() *EventRecorder {
	return &EventRecorder{
		events: make([]*ChainerEvent, 0),
	}
}

// Record captures an event.
func (r *EventRecorder) Record(event *ChainerEvent) error {
	if r == nil {
		return fmt.Errorf("event recorder not initialized")
	}
	if event == nil {
		return fmt.Errorf("event required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = append(r.events, event)
	return nil
}

// AllEvents returns all recorded events for a task, sorted by timestamp.
func (r *EventRecorder) AllEvents(taskID string) []*ChainerEvent {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Filter events by taskID
	var filtered []*ChainerEvent
	for _, e := range r.events {
		if e.TaskID == taskID {
			filtered = append(filtered, e)
		}
	}

	// Sort by timestamp
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp.Before(filtered[j].Timestamp)
	})

	return filtered
}

// RecordedEvents returns events for a specific task and link, sorted by timestamp.
func (r *EventRecorder) RecordedEvents(taskID, linkName string) []*ChainerEvent {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Filter by task and link
	var filtered []*ChainerEvent
	for _, e := range r.events {
		if e.TaskID == taskID && e.LinkName == linkName {
			filtered = append(filtered, e)
		}
	}

	// Sort by timestamp
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp.Before(filtered[j].Timestamp)
	})

	return filtered
}

// EventsByKind returns all events of a specific kind for a task.
func (r *EventRecorder) EventsByKind(taskID string, kind ChainerEventKind) []*ChainerEvent {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var filtered []*ChainerEvent
	for _, e := range r.events {
		if e.TaskID == taskID && e.Kind == kind {
			filtered = append(filtered, e)
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp.Before(filtered[j].Timestamp)
	})

	return filtered
}

// Count returns the total number of recorded events.
func (r *EventRecorder) Count() int {
	if r == nil {
		return 0
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.events)
}

// Clear removes all recorded events.
func (r *EventRecorder) Clear() {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = make([]*ChainerEvent, 0)
}

// ExecutionSummary provides a high-level overview of task execution.
type ExecutionSummary struct {
	TaskID           string
	TotalEvents      int
	TotalLinks       int
	SuccessfulLinks  int
	FailedLinks      int
	ErrorCount       int
	RetryCount       int
	CompressionCount int
	ResumeCount      int
	TotalDuration    time.Duration
	StartTime        time.Time
	EndTime          time.Time
}

// Summary generates an ExecutionSummary for a task.
func (r *EventRecorder) Summary(taskID string) *ExecutionSummary {
	if r == nil {
		return nil
	}

	events := r.AllEvents(taskID)
	if len(events) == 0 {
		return &ExecutionSummary{TaskID: taskID}
	}

	summary := &ExecutionSummary{
		TaskID:      taskID,
		TotalEvents: len(events),
		StartTime:   events[0].Timestamp,
		EndTime:     events[len(events)-1].Timestamp,
	}
	summary.TotalDuration = summary.EndTime.Sub(summary.StartTime)

	// Count event kinds
	linkStarts := make(map[string]bool)
	for _, e := range events {
		switch e.Kind {
		case KindLinkStart:
			linkStarts[e.LinkName] = true
		case KindLinkFinish:
			summary.SuccessfulLinks++
		case KindLinkError:
			summary.FailedLinks++
			summary.ErrorCount++
		case KindRetryAttempt:
			summary.RetryCount++
		case KindCompressionEvent:
			summary.CompressionCount++
		case KindResumeEvent:
			summary.ResumeCount++
		}
	}

	summary.TotalLinks = len(linkStarts)
	return summary
}
