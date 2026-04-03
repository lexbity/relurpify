package authorization

import (
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestHITLBrokerSubmitAsyncBroadcastDoesNotDeadlock(t *testing.T) {
	broker := NewHITLBroker(time.Second)
	events, cancel := broker.Subscribe(1)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := broker.SubmitAsync(PermissionRequest{
			Permission:    core.PermissionDescriptor{Resource: "tool", Action: "shell.exec"},
			Justification: "exercise async submit path",
			Scope:         GrantScopeOneTime,
		})
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SubmitAsync returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("SubmitAsync timed out; possible deadlock")
	}

	select {
	case event := <-events:
		if event.Type != HITLEventRequested {
			t.Fatalf("expected requested event, got %s", event.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for broadcast event")
	}
}
