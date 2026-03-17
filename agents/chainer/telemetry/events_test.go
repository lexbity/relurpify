package telemetry_test

import (
	"testing"

	"github.com/lexcodex/relurpify/agents/chainer/telemetry"
)

func TestLinkStartEvent(t *testing.T) {
	event := telemetry.LinkStartEvent("task-1", "step1", 0, []string{"input"}, "output")

	if event == nil {
		t.Fatal("expected event")
	}
	if event.Kind != telemetry.KindLinkStart {
		t.Errorf("expected kind LinkStart, got %v", event.Kind)
	}
	if event.TaskID != "task-1" {
		t.Errorf("expected taskID task-1, got %s", event.TaskID)
	}
	if event.LinkName != "step1" {
		t.Errorf("expected linkName step1, got %s", event.LinkName)
	}
	if event.ChainStep != 0 {
		t.Errorf("expected stepIndex 0, got %d", event.ChainStep)
	}
	if len(event.InputKeys) != 1 || event.InputKeys[0] != "input" {
		t.Errorf("unexpected input keys: %v", event.InputKeys)
	}
	if event.OutputKey != "output" {
		t.Errorf("expected outputKey output, got %s", event.OutputKey)
	}
	if event.Timestamp.IsZero() {
		t.Fatal("timestamp not set")
	}
}

func TestLinkFinishEvent(t *testing.T) {
	response := "result text"
	event := telemetry.LinkFinishEvent("task-1", "step1", 0, "output", response)

	if event.Kind != telemetry.KindLinkFinish {
		t.Errorf("expected kind LinkFinish, got %v", event.Kind)
	}
	if event.ResponseText != response {
		t.Errorf("expected response %s, got %s", response, event.ResponseText)
	}
}

func TestLinkErrorEvent(t *testing.T) {
	event := telemetry.LinkErrorEvent("task-1", "step1", 0, "connection failed", "NetworkError")

	if event.Kind != telemetry.KindLinkError {
		t.Errorf("expected kind LinkError, got %v", event.Kind)
	}
	if event.ErrorMessage != "connection failed" {
		t.Errorf("unexpected error message")
	}
	if event.ErrorType != "NetworkError" {
		t.Errorf("unexpected error type")
	}
}

func TestParsingFailureEvent(t *testing.T) {
	response := "invalid json"
	event := telemetry.ParsingFailureEvent("task-1", "step1", 0, response, "JSON parse error")

	if event.Kind != telemetry.KindParsingFailure {
		t.Errorf("expected kind ParsingFailure, got %v", event.Kind)
	}
	if event.ResponseText != response {
		t.Errorf("unexpected response text")
	}
	if event.ErrorMessage != "JSON parse error" {
		t.Errorf("unexpected error message")
	}
}

func TestRetryAttemptEvent(t *testing.T) {
	event := telemetry.RetryAttemptEvent("task-1", "step1", 0, 2, 3, "parse error")

	if event.Kind != telemetry.KindRetryAttempt {
		t.Errorf("expected kind RetryAttempt, got %v", event.Kind)
	}
	if event.AttemptNumber != 2 {
		t.Errorf("expected attempt 2, got %d", event.AttemptNumber)
	}
	if event.MaxRetries != 3 {
		t.Errorf("expected maxRetries 3, got %d", event.MaxRetries)
	}
	if event.RetryReason != "parse error" {
		t.Errorf("unexpected retry reason")
	}
}

func TestCompressionEvent(t *testing.T) {
	event := telemetry.CompressionEvent("task-1", 500, 1000, "adaptive")

	if event.Kind != telemetry.KindCompressionEvent {
		t.Errorf("expected kind CompressionEvent, got %v", event.Kind)
	}
	if event.BudgetRemaining != 500 {
		t.Errorf("expected remaining 500, got %d", event.BudgetRemaining)
	}
	if event.BudgetLimit != 1000 {
		t.Errorf("expected limit 1000, got %d", event.BudgetLimit)
	}
	if event.CompressionMode != "adaptive" {
		t.Errorf("unexpected compression mode")
	}
}

func TestResumeEvent(t *testing.T) {
	event := telemetry.ResumeEvent("task-1", 3)

	if event.Kind != telemetry.KindResumeEvent {
		t.Errorf("expected kind ResumeEvent, got %v", event.Kind)
	}
	if event.ResumedFromStepIndex != 3 {
		t.Errorf("expected resumed from 3, got %d", event.ResumedFromStepIndex)
	}
}

func TestResponseTruncation(t *testing.T) {
	longResponse := "x" // will be repeated to create a long string
	for i := 0; i < 1000; i++ {
		longResponse += "x"
	}

	event := telemetry.LinkFinishEvent("task-1", "step1", 0, "output", longResponse)

	if len(event.ResponseText) > 503 { // 500 + "..."
		t.Errorf("response not truncated: len=%d", len(event.ResponseText))
	}

	if event.ResponseText[len(event.ResponseText)-3:] != "..." {
		t.Fatal("response should end with ...")
	}
}

func TestShortResponseNoTruncation(t *testing.T) {
	shortResponse := "short"
	event := telemetry.LinkFinishEvent("task-1", "step1", 0, "output", shortResponse)

	if event.ResponseText != shortResponse {
		t.Errorf("short response should not be modified")
	}
}
