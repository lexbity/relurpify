package runtime

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"github.com/stretchr/testify/require"
)

type fakeAgent struct {
	task *core.Task
}

func (f *fakeAgent) Initialize(*core.Config) error               { return nil }
func (f *fakeAgent) Capabilities() []core.Capability             { return nil }
func (f *fakeAgent) BuildGraph(*core.Task) (*graph.Graph, error) { return nil, nil }
func (f *fakeAgent) Execute(_ context.Context, task *core.Task, _ *core.Context) (*core.Result, error) {
	f.task = task
	return &core.Result{
		Success: true,
		Data:    map[string]any{"summary": "done"},
	}, nil
}

type writeOnlyConn struct {
	writes []any
}

func (w *writeOnlyConn) WriteJSON(v any) error {
	w.writes = append(w.writes, v)
	return nil
}
func (w *writeOnlyConn) ReadJSON(any) error { return context.Canceled }
func (w *writeOnlyConn) Close() error       { return nil }

func TestDecodeNexusInstruction(t *testing.T) {
	payload, err := json.Marshal(map[string]any{
		"session_key": "sess-1",
		"channel":     "telegram",
		"content": map[string]any{
			"text": "hello",
		},
	})
	require.NoError(t, err)
	instruction, sessionKey, metadata, ok := decodeNexusInstruction(core.FrameworkEvent{
		Seq:     7,
		Type:    core.FrameworkEventMessageInbound,
		Payload: payload,
		Actor:   core.EventActor{Kind: "channel", ID: "telegram"},
	})
	require.True(t, ok)
	require.Equal(t, "hello", instruction)
	require.Equal(t, "sess-1", sessionKey)
	require.Equal(t, "telegram", metadata["channel"])
}

func TestHandleNexusEventExecutesInstructionAndResponds(t *testing.T) {
	conn := &writeOnlyConn{}
	client := &NexusClient{conn: conn}
	agent := &fakeAgent{}
	rt := &Runtime{
		Agent:   agent,
		Context: core.NewContext(),
		Config:  Config{Workspace: t.TempDir()},
	}
	payload, err := json.Marshal(map[string]any{
		"session_key": "sess-1",
		"content": map[string]any{
			"text": "fix it",
		},
	})
	require.NoError(t, err)
	event := core.FrameworkEvent{
		Type:    core.FrameworkEventMessageInbound,
		Payload: payload,
		Actor:   core.EventActor{Kind: "channel", ID: "telegram"},
	}
	instruction, sessionKey, metadata, ok := decodeNexusInstruction(event)
	require.True(t, ok)
	d := newSessionDispatcher(rt, client)
	require.NoError(t, d.process(context.Background(), nexusWorkItem{
		event:       event,
		instruction: instruction,
		sessionKey:  sessionKey,
		metadata:    metadata,
	}))
	require.NotNil(t, agent.task)
	require.Equal(t, "fix it", agent.task.Instruction)
	require.Len(t, conn.writes, 1)
	response := conn.writes[0].(map[string]any)
	require.Equal(t, "message.outbound", response["type"])
}

func TestNexusGatewayProviderRegistersInvocableRemoteCapability(t *testing.T) {
	conn := &fakeNexusConn{
		reads: []map[string]json.RawMessage{
			nexusFrame(t, map[string]any{
				"type":       "connected",
				"session_id": "agent-session",
				"server_seq": 1,
				"capabilities": []map[string]any{{
					"id":             "remote.echo",
					"name":           "remote.echo",
					"kind":           "tool",
					"runtime_family": "provider",
				}},
			}),
		},
		block: make(chan struct{}),
	}
	client := NewNexusClient(NexusConfig{Enabled: true, Address: "ws://nexus"})
	client.Dial = func(context.Context, string, string) (nexusConn, error) { return conn, nil }

	registry := capability.NewCapabilityRegistry()
	rt := &Runtime{
		Tools:   registry,
		Context: core.NewContext(),
	}
	provider := &nexusGatewayRuntimeProvider{client: client}
	require.NoError(t, provider.Initialize(context.Background(), rt))

	done := make(chan struct{})
	go func() {
		defer close(done)
		for len(conn.writes) < 2 {
			time.Sleep(time.Millisecond)
		}
		request := conn.writes[1].(map[string]any)
		require.NoError(t, client.handleFrame(nexusFrame(t, map[string]any{
			"type":           "capability.result",
			"correlation_id": request["correlation_id"],
			"result": map[string]any{
				"success": true,
				"data": map[string]any{
					"echo": "hello",
				},
			},
		})))
	}()

	state := core.NewContext()
	state.Set("session_key", "sess-1")
	result, err := registry.InvokeCapability(context.Background(), state, "remote.echo", map[string]interface{}{"text": "hello"})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "hello", result.Data["echo"])
	request := conn.writes[1].(map[string]any)
	require.Equal(t, "sess-1", request["session_key"])
	close(conn.block)
	<-done
}

func TestNexusGatewayProviderSyncCapabilitiesRevokesRemovedEntries(t *testing.T) {
	registry := capability.NewCapabilityRegistry()
	provider := &nexusGatewayRuntimeProvider{
		client: &NexusClient{},
	}
	rt := &Runtime{
		Tools:   registry,
		Context: core.NewContext(),
	}

	require.NoError(t, provider.syncCapabilities(context.Background(), rt, []core.CapabilityDescriptor{{
		ID:   "remote.echo",
		Name: "remote.echo",
		Kind: core.CapabilityKindTool,
	}}))
	_, ok := registry.GetCapability("remote.echo")
	require.True(t, ok)

	require.NoError(t, provider.syncCapabilities(context.Background(), rt, []core.CapabilityDescriptor{{
		ID:   "remote.search",
		Name: "remote.search",
		Kind: core.CapabilityKindTool,
	}}))
	_, ok = registry.GetCapability("remote.echo")
	require.True(t, ok)
	_, ok = registry.GetCapability("remote.search")
	require.True(t, ok)
	snapshot := registry.CapturePolicySnapshot()
	require.Equal(t, "nexus gateway capability removed", snapshot.Revocations.Capabilities["remote.echo"])
	_, tracked := provider.ids["remote.echo"]
	require.False(t, tracked)
	_, tracked = provider.ids["remote.search"]
	require.True(t, tracked)
}
