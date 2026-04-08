package ayenitd

import (
	"context"
	"sync"
	"testing"
	"time"

	archaeobkc "github.com/lexcodex/relurpify/archaeo/bkc"
)

func TestGitWatcherServiceEmitsRevisionChanged(t *testing.T) {
	bus := &archaeobkc.EventBus{}
	ch, unsub := bus.Subscribe(1)
	defer unsub()
	var mu sync.Mutex
	revision := "rev-1"
	runGit := func(ctx context.Context, root string, args ...string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		switch args[0] {
		case "rev-parse":
			if revision == "rev-1" {
				revision = "rev-2"
				return "rev-1\n", nil
			}
			return revision + "\n", nil
		case "diff-tree":
			return "main.go\npkg/service.go\n", nil
		default:
			return "", nil
		}
	}
	svc := &GitWatcherService{
		WorkspaceRoot: "/tmp/workspace",
		EventBus:      bus,
		PollInterval:  10 * time.Millisecond,
		RunGit:        runGit,
	}
	done := make(chan error, 1)
	go func() { done <- svc.Start(context.Background()) }()
	defer func() {
		_ = svc.Stop()
		<-done
	}()
	select {
	case event := <-ch:
		if event.Kind != archaeobkc.EventCodeRevisionChanged {
			t.Fatalf("unexpected event kind: %s", event.Kind)
		}
		payload, ok := event.Payload.(archaeobkc.CodeRevisionChangedPayload)
		if !ok {
			t.Fatalf("unexpected payload type: %T", event.Payload)
		}
		if payload.NewRevision != "rev-2" || len(payload.AffectedPaths) != 2 {
			t.Fatalf("unexpected payload: %+v", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("expected git watcher event")
	}
}

func TestGitWatcherServiceNoEventWithoutNewRevision(t *testing.T) {
	bus := &archaeobkc.EventBus{}
	ch, unsub := bus.Subscribe(1)
	defer unsub()
	svc := &GitWatcherService{
		WorkspaceRoot: "/tmp/workspace",
		EventBus:      bus,
		PollInterval:  20 * time.Millisecond,
		RunGit: func(ctx context.Context, root string, args ...string) (string, error) {
			switch args[0] {
			case "rev-parse":
				return "rev-1\n", nil
			case "diff-tree":
				return "main.go\n", nil
			default:
				return "", nil
			}
		},
	}
	done := make(chan error, 1)
	go func() { done <- svc.Start(context.Background()) }()
	time.Sleep(80 * time.Millisecond)
	_ = svc.Stop()
	<-done
	select {
	case event := <-ch:
		t.Fatalf("unexpected event: %+v", event)
	default:
	}
}

func TestGitWatcherServiceStopCancelsLoop(t *testing.T) {
	svc := &GitWatcherService{
		WorkspaceRoot: "/tmp/workspace",
		PollInterval:  time.Hour,
		RunGit: func(ctx context.Context, root string, args ...string) (string, error) {
			return "rev-1\n", nil
		},
	}
	done := make(chan error, 1)
	go func() { done <- svc.Start(context.Background()) }()
	time.Sleep(20 * time.Millisecond)
	if err := svc.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil stop result, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("expected git watcher to exit")
	}
}
