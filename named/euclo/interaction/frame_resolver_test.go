package interaction

import (
	"context"
	"errors"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

func TestFrameResolverResolveReturnsAnswer(t *testing.T) {
	fixed := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	resolver := NewFrameResolver(func() time.Time { return fixed })
	frame := NewClarificationFrame("task-1", "session-1", "Which?", []string{"a", "b"}, nil)
	frame.CreatedAt = fixed
	frame.Timeout = time.Minute

	done := make(chan FrameResolution, 1)
	go func() {
		res, err := resolver.Resolve(context.Background(), frame)
		if err != nil {
			t.Errorf("Resolve: %v", err)
			return
		}
		done <- res
	}()
	// Let the waiter register before answering.
	time.Sleep(5 * time.Millisecond)
	resolution, err := resolver.Answer(context.Background(), frame.ID, "b", map[string]any{"answer": "b"})
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if resolution.Status != ResolutionAnswered || resolution.Answer != "b" {
		t.Fatalf("unexpected resolution: %#v", resolution)
	}
	got := <-done
	if got.Status != ResolutionAnswered || got.Answer != "b" {
		t.Fatalf("Resolve returned %#v", got)
	}
}

func TestFrameResolverDuplicateResolveReturnsRecorded(t *testing.T) {
	fixed := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	resolver := NewFrameResolver(func() time.Time { return fixed })
	frame := NewClarificationFrame("task-1", "session-1", "Which?", []string{"a", "b"}, nil)
	frame.CreatedAt = fixed
	frame.Timeout = time.Minute

	// Register the frame without blocking, then resolve through a waiter.
	if err := resolver.Notify(context.Background(), frame); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	done := make(chan FrameResolution, 1)
	go func() {
		res, _ := resolver.Resolve(context.Background(), frame)
		done <- res
	}()
	time.Sleep(5 * time.Millisecond)
	if _, err := resolver.Answer(context.Background(), frame.ID, "a", nil); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if got := <-done; got.Status != ResolutionAnswered || got.Answer != "a" {
		t.Fatalf("Resolve after Answer returned %#v", got)
	}

	// Now a duplicate Resolve returns the recorded resolution.
	recorded, err := resolver.Resolve(context.Background(), frame)
	if err != nil {
		t.Fatalf("duplicate Resolve: %v", err)
	}
	if recorded.Status != ResolutionAnswered || recorded.Answer != "a" {
		t.Fatalf("duplicate Resolve must return recorded resolution, got %#v", recorded)
	}
	// And a second Answer is an idempotent no-op returning the same resolution.
	again, err := resolver.Answer(context.Background(), frame.ID, "b", nil)
	if err != nil {
		t.Fatalf("duplicate Answer: %v", err)
	}
	if again.Status != ResolutionAnswered || again.Answer != "a" {
		t.Fatalf("duplicate Answer must return recorded resolution, got %#v", again)
	}
}

func TestFrameResolverDeny(t *testing.T) {
	fixed := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	resolver := NewFrameResolver(func() time.Time { return fixed })
	frame := NewClarificationFrame("task-1", "session-1", "Approve?", []string{"approve", "reject"}, nil)
	frame.CreatedAt = fixed
	frame.Timeout = time.Minute

	done := make(chan FrameResolution, 1)
	go func() {
		res, _ := resolver.Resolve(context.Background(), frame)
		done <- res
	}()
	time.Sleep(5 * time.Millisecond)
	denied, err := resolver.Deny(context.Background(), frame.ID, "user said no")
	if err != nil {
		t.Fatalf("Deny: %v", err)
	}
	if denied.Status != ResolutionDenied {
		t.Fatalf("expected denied, got %#v", denied)
	}
	if got := <-done; got.Status != ResolutionDenied {
		t.Fatalf("Resolve returned %#v", got)
	}
}

func TestFrameResolverExpiryTimer(t *testing.T) {
	resolver := NewFrameResolver(nil) // real clock
	frame := NewClarificationFrame("task-1", "session-1", "Which?", []string{"a", "b"}, nil)
	frame.CreatedAt = time.Now().UTC()
	frame.Timeout = 15 * time.Millisecond

	start := time.Now()
	resolution, err := resolver.Resolve(context.Background(), frame)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolution.Status != ResolutionExpired {
		t.Fatalf("expected expired resolution, got %#v", resolution)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("expiry must be deadline-driven, took %v", elapsed)
	}
}

func TestFrameResolverLazyExpiryAndLateAnswer(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	resolver := NewFrameResolver(func() time.Time { return now })
	sink := &captureTelemetry{}
	ctx := telemetry.WithTelemetry(context.Background(), sink)

	frame := NewClarificationFrame("task-1", "session-1", "Which?", []string{"a", "b"}, nil)
	frame.CreatedAt = now
	frame.Timeout = 30 * time.Second

	if err := resolver.Notify(ctx, frame); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	// Advance the clock past the deadline.
	now = now.Add(time.Minute)

	resolution, err := resolver.Resolve(ctx, frame)
	if err != nil {
		t.Fatalf("Resolve on expired: %v", err)
	}
	if resolution.Status != ResolutionExpired {
		t.Fatalf("expected expired, got %#v", resolution)
	}

	// A late answer returns Expired, emits frame.expired_late_answer once, and
	// changes no state (stale-consent hole closed — D12/FR-18).
	late, err := resolver.Answer(ctx, frame.ID, "a", nil)
	if err != nil {
		t.Fatalf("late Answer: %v", err)
	}
	if late.Status != ResolutionExpired {
		t.Fatalf("late answer must resolve as Expired, got %#v", late)
	}
	events := sink.snapshot()
	if len(events) != 1 {
		t.Fatalf("expected 1 frame.expired_late_answer event, got %d", len(events))
	}
	if events[0].Type != telemetry.EventFrameExpiredLateAnswer {
		t.Fatalf("expected %q event, got %q", telemetry.EventFrameExpiredLateAnswer, events[0].Type)
	}
	// The second late answer is idempotent: no additional event.
	if _, err := resolver.Answer(ctx, frame.ID, "b", nil); err != nil {
		t.Fatalf("second late Answer: %v", err)
	}
	if got := len(sink.snapshot()); got != 1 {
		t.Fatalf("late-answer event must fire once, got %d", got)
	}
}

func TestFrameResolverInvalidChoiceRejected(t *testing.T) {
	fixed := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	resolver := NewFrameResolver(func() time.Time { return fixed })
	frame := NewClarificationFrame("task-1", "session-1", "Which?", []string{"a", "b"}, nil)
	frame.CreatedAt = fixed
	frame.Timeout = time.Minute
	if err := resolver.Notify(context.Background(), frame); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	// An answer outside the frame's declared choices is rejected; the frame
	// stays open.
	if _, err := resolver.Answer(context.Background(), frame.ID, "not-a-choice", nil); err == nil {
		t.Fatal("expected answer validation error")
	}
	if open := resolver.OpenFrames(); len(open) != 1 {
		t.Fatalf("invalid answer must not close the frame, got %d open", len(open))
	}
	// The validated answer succeeds within the deadline.
	resolution, err := resolver.Answer(context.Background(), frame.ID, "b", nil)
	if err != nil {
		t.Fatalf("valid answer: %v", err)
	}
	if resolution.Status != ResolutionAnswered || resolution.Answer != "b" {
		t.Fatalf("unexpected resolution %#v", resolution)
	}
}

func TestFrameResolverUnknownFrame(t *testing.T) {
	resolver := NewFrameResolver(nil)
	if _, err := resolver.Answer(context.Background(), "nope", "x", nil); !errors.Is(err, ErrUnknownFrame) {
		t.Fatalf("expected ErrUnknownFrame, got %v", err)
	}
}

func TestFrameResolverNotifyDropsDuplicate(t *testing.T) {
	resolver := NewFrameResolver(nil)
	frame := NewClarificationFrame("task-1", "session-1", "Which?", []string{"a", "b"}, nil)
	if err := resolver.Notify(context.Background(), frame); err != nil {
		t.Fatalf("first Notify: %v", err)
	}
	if err := resolver.Notify(context.Background(), frame); err != nil {
		t.Fatalf("second Notify: %v", err)
	}
	if got := resolver.DroppedNotifies(); got != 1 {
		t.Fatalf("expected 1 dropped notifies, got %d", got)
	}
	if open := resolver.OpenFrames(); len(open) != 1 {
		t.Fatalf("expected 1 open frame, got %d", len(open))
	}
}

func TestResumeFrameSkipsExpired(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "session-1")
	fixed := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	frame := NewClarificationFrame("task-1", "session-1", "Expired?", []string{"yes", "no"}, nil)
	frame.CreatedAt = fixed
	frame.Timeout = time.Minute

	if err := EmitFrame(context.Background(), frame, env, nil); err != nil {
		t.Fatalf("EmitFrame: %v", err)
	}
	// Simulate the deadline passing before resume: the frame must surface as a
	// gap (FR-18).
	frame.CreatedAt = fixed.Add(-2 * time.Minute)
	if got, ok := ResumeFrame(env); ok && got != nil {
		t.Fatalf("expected expired frame to be skipped, got %#v", got)
	}
}
