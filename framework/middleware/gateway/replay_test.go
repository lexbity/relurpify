package gateway

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestReplayFramesIncludesReplayComplete(t *testing.T) {
	log := newTestEventLog()
	defer log.Close()

	payload, err := json.Marshal(map[string]string{"text": "hello"})
	require.NoError(t, err)
	_, err = log.Append(context.Background(), "tenant-a", []core.FrameworkEvent{
		{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventMessageInbound,
			Payload:   payload,
			Actor:     core.EventActor{Kind: "channel", ID: "webchat"},
			Partition: "tenant-a",
		},
	})
	require.NoError(t, err)

	server := &Server{Log: log, Partition: "tenant-a"}
	frames, err := server.replayFrames(context.Background(), ConnectionPrincipal{
		Authenticated: true,
		Actor:         core.EventActor{Kind: "admin", ID: "admin-1"},
		Principal: &core.AuthenticatedPrincipal{
			Authenticated: true,
			Scopes:        []string{"gateway:admin", "gateway:admin:global"},
		},
	}, 0)
	require.NoError(t, err)
	require.Len(t, frames, 2)
	require.Equal(t, "event", frames[0].(replayEventFrame).Type)
	require.Equal(t, "replay_complete", frames[1].(replayCompleteFrame).Type)
}

func TestReplayFramesFiltersBySessionAuthorizationForNonAdmin(t *testing.T) {
	log := newTestEventLog()
	defer log.Close()

	var err error
	_, err = log.Append(context.Background(), "local", []core.FrameworkEvent{
		{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventSessionMessage,
			Payload:   mustReplayJSON(map[string]string{"session_key": "sess-a", "text": "tenant-a"}),
			Actor:     core.EventActor{Kind: "session", ID: "sess-a", TenantID: "tenant-a"},
			Partition: "local",
		},
		{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventMessageInbound,
			Payload:   mustReplayJSON(map[string]string{"text": "tenant-a-raw"}),
			Actor:     core.EventActor{Kind: "channel", ID: "discord", TenantID: "tenant-a"},
			Partition: "local",
		},
		{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventSessionMessage,
			Payload:   mustReplayJSON(map[string]string{"session_key": "sess-b", "text": "tenant-a-other"}),
			Actor:     core.EventActor{Kind: "session", ID: "sess-b", TenantID: "tenant-a"},
			Partition: "local",
		},
	})
	require.NoError(t, err)

	server := &Server{
		Log:       log,
		Partition: "local",
		SessionEventAuthorizer: func(_ context.Context, _ ConnectionPrincipal, sessionID string) (bool, error) {
			return sessionID == "sess-a", nil
		},
	}
	frames, err := server.replayFrames(context.Background(), ConnectionPrincipal{
		Authenticated: true,
		Actor:         core.EventActor{Kind: "agent", ID: "svc-1", TenantID: "tenant-a"},
	}, 0)
	require.NoError(t, err)
	require.Len(t, frames, 2)
	eventFrame, ok := frames[0].(replayEventFrame)
	require.True(t, ok)
	require.Equal(t, core.FrameworkEventSessionMessage, eventFrame.Event.Type)
	require.Equal(t, "sess-a", eventFrame.Event.Actor.ID)
	complete := frames[1].(replayCompleteFrame)
	require.Equal(t, 1, complete.EventCount)
}

func TestReplayFramesAllowsAdminAcrossTenants(t *testing.T) {
	log := newTestEventLog()
	defer log.Close()

	var err error
	_, err = log.Append(context.Background(), "local", []core.FrameworkEvent{
		{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventMessageInbound,
			Payload:   mustReplayJSON(map[string]string{"text": "tenant-a"}),
			Actor:     core.EventActor{Kind: "channel", ID: "discord", TenantID: "tenant-a"},
			Partition: "local",
		},
		{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventMessageInbound,
			Payload:   mustReplayJSON(map[string]string{"text": "tenant-b"}),
			Actor:     core.EventActor{Kind: "channel", ID: "discord", TenantID: "tenant-b"},
			Partition: "local",
		},
	})
	require.NoError(t, err)

	server := &Server{Log: log, Partition: "local"}
	frames, err := server.replayFrames(context.Background(), ConnectionPrincipal{
		Authenticated: true,
		Actor:         core.EventActor{Kind: "admin", ID: "admin-1"},
		Principal: &core.AuthenticatedPrincipal{
			Authenticated: true,
			Scopes:        []string{"gateway:admin", "gateway:admin:global"},
		},
	}, 0)
	require.NoError(t, err)
	require.Len(t, frames, 3)
	complete := frames[2].(replayCompleteFrame)
	require.Equal(t, 2, complete.EventCount)
}

func TestReplayFramesTenantAdminRemainsTenantScoped(t *testing.T) {
	log := newTestEventLog()
	defer log.Close()

	var err error
	_, err = log.Append(context.Background(), "local", []core.FrameworkEvent{
		{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventMessageInbound,
			Payload:   mustReplayJSON(map[string]string{"text": "tenant-a"}),
			Actor:     core.EventActor{Kind: "channel", ID: "discord", TenantID: "tenant-a"},
			Partition: "local",
		},
		{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventMessageInbound,
			Payload:   mustReplayJSON(map[string]string{"text": "tenant-b"}),
			Actor:     core.EventActor{Kind: "channel", ID: "discord", TenantID: "tenant-b"},
			Partition: "local",
		},
	})
	require.NoError(t, err)

	server := &Server{Log: log, Partition: "local"}
	frames, err := server.replayFrames(context.Background(), ConnectionPrincipal{
		Authenticated: true,
		Actor:         core.EventActor{Kind: "admin", ID: "admin-1", TenantID: "tenant-a"},
		Principal: &core.AuthenticatedPrincipal{
			Authenticated: true,
			TenantID:      "tenant-a",
			Scopes:        []string{"gateway:admin"},
		},
	}, 0)
	require.NoError(t, err)
	require.Len(t, frames, 2)
	eventFrame := frames[0].(replayEventFrame)
	require.Equal(t, "tenant-a", eventFrame.Event.Actor.TenantID)
	complete := frames[1].(replayCompleteFrame)
	require.Equal(t, 1, complete.EventCount)
}
