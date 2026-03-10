package gateway

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/stretchr/testify/require"
)

func TestReplayFramesIncludesReplayComplete(t *testing.T) {
	log, err := db.NewSQLiteEventLog(filepath.Join(t.TempDir(), "events.db"))
	require.NoError(t, err)
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
			Scopes:        []string{"gateway:admin"},
		},
	}, 0)
	require.NoError(t, err)
	require.Len(t, frames, 2)
	require.Equal(t, "event", frames[0].(replayEventFrame).Type)
	require.Equal(t, "replay_complete", frames[1].(replayCompleteFrame).Type)
}

func TestReplayFramesFiltersByTenantForNonAdmin(t *testing.T) {
	log, err := db.NewSQLiteEventLog(filepath.Join(t.TempDir(), "events.db"))
	require.NoError(t, err)
	defer log.Close()

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
		Actor:         core.EventActor{Kind: "agent", ID: "svc-1", TenantID: "tenant-a"},
	}, 0)
	require.NoError(t, err)
	require.Len(t, frames, 2)
	eventFrame, ok := frames[0].(replayEventFrame)
	require.True(t, ok)
	require.Equal(t, "tenant-a", eventFrame.Event.Actor.TenantID)
	complete := frames[1].(replayCompleteFrame)
	require.Equal(t, 1, complete.EventCount)
}

func TestReplayFramesAllowsAdminAcrossTenants(t *testing.T) {
	log, err := db.NewSQLiteEventLog(filepath.Join(t.TempDir(), "events.db"))
	require.NoError(t, err)
	defer log.Close()

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
			Scopes:        []string{"gateway:admin"},
		},
	}, 0)
	require.NoError(t, err)
	require.Len(t, frames, 3)
	complete := frames[2].(replayCompleteFrame)
	require.Equal(t, 2, complete.EventCount)
}
