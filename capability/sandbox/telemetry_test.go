package sandbox

import (
	"context"
	"errors"
	"sync"
	"testing"

	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// commandRecordingSink captures sandbox telemetry events for assertions.
type commandRecordingSink struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (s *commandRecordingSink) Emit(event telemetry.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *commandRecordingSink) Events() []telemetry.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]telemetry.Event(nil), s.events...)
}

func (s *commandRecordingSink) Find(eventType telemetry.EventType) (telemetry.Event, bool) {
	for _, ev := range s.Events() {
		if ev.Type == eventType {
			return ev, true
		}
	}
	return telemetry.Event{}, false
}

func TestAuthorizedRunnerEmitsCommandDenied(t *testing.T) {
	sink := &commandRecordingSink{}
	inner := &fakeRunner{}
	policy := CommandPolicyFunc(func(_ context.Context, req CommandRequest) error {
		return errors.New("denied by bash_permissions")
	})
	auth, err := NewAuthorizedRunner(inner, policy)
	if err != nil {
		t.Fatalf("NewAuthorizedRunner: %v", err)
	}
	auth.SetTelemetry(sink)

	_, err = auth.Run(context.Background(), CommandRequest{Args: []string{"rm", "-rf", "/"}})
	if err == nil {
		t.Fatal("expected a denial error")
	}

	ev, ok := sink.Find(telemetry.EventSandboxCommandDenied)
	if !ok {
		t.Fatalf("expected sandbox.command_denied, got %v", sinkTypes(sink))
	}
	if rule, _ := ev.Metadata["rule"].(string); rule == "" {
		t.Fatalf("denial event missing policy rule: %+v", ev.Metadata)
	}
	if cmd, _ := ev.Metadata["command"].(string); cmd != "rm -rf /" {
		t.Fatalf("unexpected command in denial event: %q", cmd)
	}
}

func TestAuthorizedRunnerRedactsSecretsFromCommandEvents(t *testing.T) {
	sink := &commandRecordingSink{}
	inner := &fakeRunner{}
	auth, err := NewAuthorizedRunner(inner, CommandPolicyFunc(func(context.Context, CommandRequest) error { return nil }))
	if err != nil {
		t.Fatalf("NewAuthorizedRunner: %v", err)
	}
	auth.SetTelemetry(sink)

	// A bearer-style token embedded in an argument must be scrubbed before the
	// event reaches the sink (NFR-7).
	_, err = auth.Run(context.Background(), CommandRequest{Args: []string{"curl", "-H", "Authorization: Bearer sk-secret123"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	ev, ok := sink.Find(telemetry.EventSandboxCommandExecuted)
	if !ok {
		t.Fatalf("expected sandbox.command_executed, got %v", sinkTypes(sink))
	}
	command, _ := ev.Metadata["command"].(string)
	if containsString(command, "sk-secret123") {
		t.Fatalf("secret survived redaction in command: %q", command)
	}
	if _, hasArgs := ev.Metadata["args"]; hasArgs {
		t.Fatal("raw args must not be emitted on sandbox events")
	}
}

func containsString(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func TestAuthorizedRunnerEmitsCommandExecuted(t *testing.T) {
	sink := &commandRecordingSink{}
	inner := &fakeRunner{}
	auth, err := NewAuthorizedRunner(inner, CommandPolicyFunc(func(context.Context, CommandRequest) error { return nil }))
	if err != nil {
		t.Fatalf("NewAuthorizedRunner: %v", err)
	}
	auth.SetTelemetry(sink)

	res, err := auth.Run(context.Background(), CommandRequest{Args: []string{"echo", "hello"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res == nil {
		t.Fatal("expected a command result")
	}

	ev, ok := sink.Find(telemetry.EventSandboxCommandExecuted)
	if !ok {
		t.Fatalf("expected sandbox.command_executed, got %v", sinkTypes(sink))
	}
	if code, _ := ev.Metadata["exit_code"].(int); code != 0 {
		t.Fatalf("unexpected exit_code in %+v", ev.Metadata)
	}
}

func TestAuthorizedRunnerEmitsSandboxFailure(t *testing.T) {
	sink := &commandRecordingSink{}
	inner := &fakeRunner{}
	auth, err := NewAuthorizedRunner(inner, CommandPolicyFunc(func(context.Context, CommandRequest) error { return nil }))
	if err != nil {
		t.Fatalf("NewAuthorizedRunner: %v", err)
	}
	auth.SetTelemetry(sink)

	// fakeRunner returns a runner-level error for the "error" argument — this is
	// an execution failure, not a policy denial.
	_, err = auth.Run(context.Background(), CommandRequest{Args: []string{"error"}})
	if err == nil {
		t.Fatal("expected a runner failure error")
	}

	ev, ok := sink.Find(telemetry.EventSandboxFailure)
	if !ok {
		t.Fatalf("expected sandbox.failure, got %v", sinkTypes(sink))
	}
	if msg, _ := ev.Metadata["error"].(string); msg == "" {
		t.Fatalf("failure event missing error detail: %+v", ev.Metadata)
	}
}

func TestAuthorizedRunnerWithoutTelemetryIsSilent(t *testing.T) {
	inner := &fakeRunner{}
	auth, err := NewAuthorizedRunner(inner, CommandPolicyFunc(func(context.Context, CommandRequest) error { return nil }))
	if err != nil {
		t.Fatalf("NewAuthorizedRunner: %v", err)
	}
	if auth.telemetry != nil {
		t.Fatal("expected nil telemetry on a fresh authorized runner")
	}
	_, err = auth.Run(context.Background(), CommandRequest{Args: []string{"echo", "hi"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// SetTelemetry(nil) must also be safe and silent.
	auth.SetTelemetry(nil)
	_, err = auth.Run(context.Background(), CommandRequest{Args: []string{"echo", "hi"}})
	if err != nil {
		t.Fatalf("Run after SetTelemetry(nil): %v", err)
	}
}

func sinkTypes(sink *commandRecordingSink) []telemetry.EventType {
	var out []telemetry.EventType
	for _, ev := range sink.Events() {
		out = append(out, ev.Type)
	}
	return out
}
