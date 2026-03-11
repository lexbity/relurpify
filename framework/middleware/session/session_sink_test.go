package session_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/identity"
	"github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/framework/middleware/channel"
	"github.com/lexcodex/relurpify/framework/middleware/session"
	"github.com/stretchr/testify/require"
)

func TestSessionSinkEmitsSessionMessage(t *testing.T) {
	log, err := db.NewSQLiteEventLog(filepath.Join(t.TempDir(), "events.db"))
	require.NoError(t, err)
	defer log.Close()
	store, err := db.NewSQLiteSessionStore(filepath.Join(t.TempDir(), "sessions.db"))
	require.NoError(t, err)
	defer store.Close()

	sink := &session.SessionSink{
		Log:       log,
		Partition: "tenant-a",
		Router: &session.DefaultRouter{
			Store: store,
			Log:   log,
			Scope: core.SessionScopePerChannelPeer,
		},
	}
	payload, err := json.Marshal(channel.InboundMessage{
		Channel: "webchat",
		Sender:  channel.Identity{ChannelID: "peer-1"},
		Conversation: channel.Conversation{
			ID: "conv-1",
		},
		Content: channel.MessageContent{Text: "hello"},
	})
	require.NoError(t, err)
	require.NoError(t, sink.Emit(context.Background(), core.FrameworkEvent{
		Timestamp: time.Now().UTC(),
		Type:      core.FrameworkEventMessageInbound,
		Payload:   payload,
		Actor:     core.EventActor{Kind: "channel", ID: "webchat"},
		Partition: "",
	}))

	events, err := log.Read(context.Background(), "tenant-a", 0, 10, false)
	require.NoError(t, err)
	require.Len(t, events, 3)
	require.Equal(t, core.FrameworkEventMessageInbound, events[0].Type)
	require.Equal(t, core.FrameworkEventSessionCreated, events[1].Type)
	require.Equal(t, core.FrameworkEventSessionMessage, events[2].Type)
	require.Equal(t, "tenant-a", events[0].Partition)
	require.Equal(t, "tenant-a", events[2].Partition)
}

func TestSessionSinkResolvesExternalIdentityBeforeRouting(t *testing.T) {
	log, err := db.NewSQLiteEventLog(filepath.Join(t.TempDir(), "events.db"))
	require.NoError(t, err)
	defer log.Close()
	store, err := db.NewSQLiteSessionStore(filepath.Join(t.TempDir(), "sessions.db"))
	require.NoError(t, err)
	defer store.Close()
	identityStore, err := db.NewSQLiteIdentityStore(filepath.Join(t.TempDir(), "identities.db"))
	require.NoError(t, err)
	defer identityStore.Close()
	require.NoError(t, identityStore.UpsertExternalIdentity(context.Background(), core.ExternalIdentity{
		TenantID:   "tenant-1",
		Provider:   core.ExternalProviderDiscord,
		AccountID:  "guild-1",
		ExternalID: "discord-user-1",
		Subject: core.SubjectRef{
			TenantID: "tenant-1",
			Kind:     core.SubjectKindUser,
			ID:       "user-1",
		},
	}))

	sink := &session.SessionSink{
		Log:      log,
		Resolver: identity.StoreResolver{Store: identityStore, DefaultTenantID: "tenant-1"},
		Router: &session.DefaultRouter{
			Store: store,
			Log:   log,
			Scope: core.SessionScopePerChannelPeer,
		},
	}
	payload, err := json.Marshal(channel.InboundMessage{
		Channel: "discord",
		Account: "guild-1",
		Sender:  channel.Identity{ChannelID: "discord-user-1"},
		Conversation: channel.Conversation{
			ID: "channel-1",
		},
		Content: channel.MessageContent{Text: "hello"},
	})
	require.NoError(t, err)
	require.NoError(t, sink.Emit(context.Background(), core.FrameworkEvent{
		Timestamp: time.Now().UTC(),
		Type:      core.FrameworkEventMessageInbound,
		Payload:   payload,
		Actor:     core.EventActor{Kind: "channel", ID: "discord"},
		Partition: "local",
	}))

	boundary, err := store.GetBoundary(context.Background(), "local:discord:channel-1")
	require.NoError(t, err)
	require.NotNil(t, boundary)
	require.NotEqual(t, boundary.RoutingKey, boundary.SessionID)
	require.Equal(t, "tenant-1", boundary.TenantID)
	require.Equal(t, "user-1", boundary.Owner.ID)
	require.Equal(t, core.TrustClassRemoteApproved, boundary.TrustClass)
	require.NotNil(t, boundary.Binding)
	require.Equal(t, core.ExternalProviderDiscord, boundary.Binding.Provider)

	events, err := log.Read(context.Background(), "local", 0, 10, false)
	require.NoError(t, err)
	require.Len(t, events, 3)
	require.Equal(t, "tenant-1", events[0].Actor.TenantID)
}

func TestSessionSinkLeavesUnknownIdentityUnbound(t *testing.T) {
	log, err := db.NewSQLiteEventLog(filepath.Join(t.TempDir(), "events.db"))
	require.NoError(t, err)
	defer log.Close()
	store, err := db.NewSQLiteSessionStore(filepath.Join(t.TempDir(), "sessions.db"))
	require.NoError(t, err)
	defer store.Close()

	sink := &session.SessionSink{
		Log:      log,
		Resolver: identity.StoreResolver{DefaultTenantID: "tenant-1"},
		Router: &session.DefaultRouter{
			Store: store,
			Log:   log,
			Scope: core.SessionScopePerChannelPeer,
		},
	}
	payload, err := json.Marshal(channel.InboundMessage{
		Channel: "discord",
		Account: "guild-1",
		Sender:  channel.Identity{ChannelID: "discord-user-404"},
		Conversation: channel.Conversation{
			ID: "channel-2",
		},
		Content: channel.MessageContent{Text: "hello"},
	})
	require.NoError(t, err)
	require.NoError(t, sink.Emit(context.Background(), core.FrameworkEvent{
		Timestamp: time.Now().UTC(),
		Type:      core.FrameworkEventMessageInbound,
		Payload:   payload,
		Actor:     core.EventActor{Kind: "channel", ID: "discord"},
		Partition: "local",
	}))

	boundary, err := store.GetBoundary(context.Background(), "local:discord:channel-2")
	require.NoError(t, err)
	require.NotNil(t, boundary)
	require.NotEqual(t, boundary.RoutingKey, boundary.SessionID)
	require.Equal(t, "__relurpify_unresolved_external__", boundary.TenantID)
	require.Empty(t, boundary.Owner.ID)
	require.Equal(t, core.TrustClassRemoteDeclared, boundary.TrustClass)

	events, err := log.Read(context.Background(), "local", 0, 10, false)
	require.NoError(t, err)
	require.Len(t, events, 3)
	require.Equal(t, "__relurpify_unresolved_external__", events[0].Actor.TenantID)
}

func TestSessionSinkLeavesExternalIdentityWithoutResolverInUnresolvedTenant(t *testing.T) {
	log, err := db.NewSQLiteEventLog(filepath.Join(t.TempDir(), "events.db"))
	require.NoError(t, err)
	defer log.Close()
	store, err := db.NewSQLiteSessionStore(filepath.Join(t.TempDir(), "sessions.db"))
	require.NoError(t, err)
	defer store.Close()

	sink := &session.SessionSink{
		Log: log,
		Router: &session.DefaultRouter{
			Store: store,
			Log:   log,
			Scope: core.SessionScopePerChannelPeer,
		},
	}
	payload, err := json.Marshal(channel.InboundMessage{
		Channel: "discord",
		Account: "guild-1",
		Sender:  channel.Identity{ChannelID: "discord-user-404"},
		Conversation: channel.Conversation{
			ID: "channel-3",
		},
		Content: channel.MessageContent{Text: "hello"},
	})
	require.NoError(t, err)
	require.NoError(t, sink.Emit(context.Background(), core.FrameworkEvent{
		Timestamp: time.Now().UTC(),
		Type:      core.FrameworkEventMessageInbound,
		Payload:   payload,
		Actor:     core.EventActor{Kind: "channel", ID: "discord"},
		Partition: "local",
	}))

	boundary, err := store.GetBoundary(context.Background(), "local:discord:channel-3")
	require.NoError(t, err)
	require.NotNil(t, boundary)
	require.Equal(t, "__relurpify_unresolved_external__", boundary.TenantID)
	require.Empty(t, boundary.Owner.ID)
	require.Equal(t, core.TrustClassRemoteDeclared, boundary.TrustClass)
}

func TestSessionSinkCapsUnresolvedExternalSessionCreation(t *testing.T) {
	log, err := db.NewSQLiteEventLog(filepath.Join(t.TempDir(), "events.db"))
	require.NoError(t, err)
	defer log.Close()
	store, err := db.NewSQLiteSessionStore(filepath.Join(t.TempDir(), "sessions.db"))
	require.NoError(t, err)
	defer store.Close()

	sink := &session.SessionSink{
		Log:                           log,
		MaxUnresolvedExternalSessions: 1,
		Router: &session.DefaultRouter{
			Store: store,
			Log:   log,
			Scope: core.SessionScopePerChannelPeer,
		},
	}

	firstPayload, err := json.Marshal(channel.InboundMessage{
		Channel: "discord",
		Account: "guild-1",
		Sender:  channel.Identity{ChannelID: "discord-user-1"},
		Conversation: channel.Conversation{
			ID: "channel-1",
		},
		Content: channel.MessageContent{Text: "hello"},
	})
	require.NoError(t, err)
	require.NoError(t, sink.Emit(context.Background(), core.FrameworkEvent{
		Timestamp: time.Now().UTC(),
		Type:      core.FrameworkEventMessageInbound,
		Payload:   firstPayload,
		Actor:     core.EventActor{Kind: "channel", ID: "discord"},
		Partition: "local",
	}))

	secondPayload, err := json.Marshal(channel.InboundMessage{
		Channel: "discord",
		Account: "guild-1",
		Sender:  channel.Identity{ChannelID: "discord-user-2"},
		Conversation: channel.Conversation{
			ID: "channel-2",
		},
		Content: channel.MessageContent{Text: "hello"},
	})
	require.NoError(t, err)
	require.NoError(t, sink.Emit(context.Background(), core.FrameworkEvent{
		Timestamp: time.Now().UTC(),
		Type:      core.FrameworkEventMessageInbound,
		Payload:   secondPayload,
		Actor:     core.EventActor{Kind: "channel", ID: "discord"},
		Partition: "local",
	}))

	firstBoundary, err := store.GetBoundary(context.Background(), "local:discord:channel-1")
	require.NoError(t, err)
	require.NotNil(t, firstBoundary)

	secondBoundary, err := store.GetBoundary(context.Background(), "local:discord:channel-2")
	require.NoError(t, err)
	require.Nil(t, secondBoundary)

	boundaries, err := store.ListBoundaries(context.Background(), "local")
	require.NoError(t, err)
	require.Len(t, boundaries, 1)

	events, err := log.Read(context.Background(), "local", 0, 10, false)
	require.NoError(t, err)
	require.Len(t, events, 4)
	require.Equal(t, core.FrameworkEventMessageInbound, events[0].Type)
	require.Equal(t, core.FrameworkEventSessionCreated, events[1].Type)
	require.Equal(t, core.FrameworkEventSessionMessage, events[2].Type)
	require.Equal(t, core.FrameworkEventMessageInbound, events[3].Type)
}

func TestOutboundMessageForSession(t *testing.T) {
	msg, err := session.OutboundMessageForSession(&core.SessionBoundary{
		SessionID: "local:webchat:conv-1",
		ChannelID: "webchat",
		PeerID:    "conv-1",
	}, "reply")
	require.NoError(t, err)
	require.Equal(t, "webchat", msg.Channel)
	require.Equal(t, "conv-1", msg.ConversationID)
	require.Equal(t, "reply", msg.Content.Text)
}
