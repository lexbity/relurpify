package runtime

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionBoundaryKey(t *testing.T) {
	event := core.FrameworkEvent{
		Partition: "local",
		Actor:     core.EventActor{ID: "user-42"},
	}
	key := sessionBoundaryKey(event, "telegram")
	assert.Equal(t, "local|telegram|user-42", key)
}

func TestSessionBoundaryKeyDistinctActors(t *testing.T) {
	e1 := core.FrameworkEvent{Partition: "local", Actor: core.EventActor{ID: "user-1"}}
	e2 := core.FrameworkEvent{Partition: "local", Actor: core.EventActor{ID: "user-2"}}
	assert.NotEqual(t, sessionBoundaryKey(e1, "ch"), sessionBoundaryKey(e2, "ch"))
}

func TestSessionBoundaryKeyDistinctChannels(t *testing.T) {
	e := core.FrameworkEvent{Partition: "local", Actor: core.EventActor{ID: "user-1"}}
	assert.NotEqual(t, sessionBoundaryKey(e, "discord"), sessionBoundaryKey(e, "telegram"))
}

func TestDispatcherIgnoresAgentEvents(t *testing.T) {
	conn := &writeOnlyConn{}
	client := &NexusClient{conn: conn}
	agent := &fakeAgent{}
	rt := &Runtime{
		Agent:   agent,
		Context: core.NewContext(),
		Config:  Config{Workspace: t.TempDir()},
	}
	d := newSessionDispatcher(rt, client)
	d.idleTimeout = 50 * time.Millisecond

	payload, _ := json.Marshal(map[string]any{
		"text": "run something",
	})
	// Events from agents should be silently dropped — no worker created.
	d.Dispatch(t.Context(), core.FrameworkEvent{
		Type:    core.FrameworkEventMessageInbound,
		Payload: payload,
		Actor:   core.EventActor{Kind: "agent", ID: "a1"},
	})
	d.mu.Lock()
	workerCount := len(d.workers)
	d.mu.Unlock()
	assert.Equal(t, 0, workerCount)
}

func TestDispatcherCreatesWorkerPerSession(t *testing.T) {
	conn := &writeOnlyConn{}
	client := &NexusClient{conn: conn}
	agent := &fakeAgent{}
	rt := &Runtime{
		Agent:   agent,
		Context: core.NewContext(),
		Config:  Config{Workspace: t.TempDir()},
	}
	d := newSessionDispatcher(rt, client)
	d.idleTimeout = 50 * time.Millisecond

	makeEvent := func(actorID string) core.FrameworkEvent {
		payload, _ := json.Marshal(map[string]any{"text": "hello"})
		return core.FrameworkEvent{
			Type:      core.FrameworkEventMessageInbound,
			Payload:   payload,
			Partition: "local",
			Actor:     core.EventActor{Kind: "channel", ID: actorID},
		}
	}

	// Two distinct actor IDs should get two workers.
	d.Dispatch(t.Context(), makeEvent("user-A"))
	d.Dispatch(t.Context(), makeEvent("user-B"))

	require.Eventually(t, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.workers) == 2
	}, time.Second, 10*time.Millisecond)
}

func TestDispatcherWorkersReapedAfterIdle(t *testing.T) {
	conn := &writeOnlyConn{}
	client := &NexusClient{conn: conn}
	agent := &fakeAgent{}
	rt := &Runtime{
		Agent:   agent,
		Context: core.NewContext(),
		Config:  Config{Workspace: t.TempDir()},
	}
	d := newSessionDispatcher(rt, client)
	d.idleTimeout = 50 * time.Millisecond

	payload, _ := json.Marshal(map[string]any{"text": "hello"})
	d.Dispatch(t.Context(), core.FrameworkEvent{
		Type:      core.FrameworkEventMessageInbound,
		Payload:   payload,
		Partition: "local",
		Actor:     core.EventActor{Kind: "channel", ID: "user-X"},
	})

	// Worker should eventually reap itself after idle timeout.
	require.Eventually(t, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.workers) == 0
	}, 2*time.Second, 20*time.Millisecond)
}
