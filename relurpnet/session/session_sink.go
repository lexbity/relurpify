package session

import (
	"context"
	"encoding/json"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/event"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
	"codeburg.org/lexbit/relurpify/relurpnet/channel"
)

type SessionSink struct {
	Log                           event.Log
	Partition                     string
	Router                        Router
	Resolver                      identity.Resolver
	MaxUnresolvedExternalSessions int
}

const unresolvedExternalTenantID = core.RestrictedExternalTenantID
const defaultMaxUnresolvedExternalSessions = 256

func (s *SessionSink) Emit(ctx context.Context, ev core.FrameworkEvent) error {
	if s == nil || s.Log == nil {
		return nil
	}
	partition := ev.Partition
	if partition == "" {
		partition = s.partition()
	}
	ev.Partition = partition

	var inbound channel.InboundMessage
	resolution := identity.Resolution{}
	if ev.Type == core.FrameworkEventMessageInbound {
		if err := json.Unmarshal(ev.Payload, &inbound); err == nil {
			if s.Resolver != nil {
				resolved, err := s.Resolver.ResolveInbound(ctx, inbound)
				if err != nil {
					return err
				}
				resolution = resolved
			}
			if (resolution.Unbound() || resolution.State == identity.ResolutionStateUnknown) && isExternalSessionInbound(inbound, resolution) {
				resolution.TenantID = unresolvedExternalTenantID
			}
			if resolution.TenantID != "" {
				ev.Actor.TenantID = resolution.TenantID
			}
		}
	}
	if _, err := s.Log.Append(ctx, partition, []core.FrameworkEvent{ev}); err != nil {
		return err
	}
	if ev.Type != core.FrameworkEventMessageInbound || s.Router == nil {
		return nil
	}
	if inbound.Channel == "" && inbound.Conversation.ID == "" && inbound.Sender.ChannelID == "" && inbound.Sender.ResolvedID == "" {
		return nil
	}
	trustClass := core.TrustClassRemoteDeclared
	actorID := inbound.Sender.ChannelID
	if resolution.Resolved() {
		trustClass = core.TrustClassRemoteApproved
		if resolution.Owner.ID != "" {
			actorID = resolution.Owner.ID
		}
	}
	if resolution.TenantID == unresolvedExternalTenantID {
		allowed, err := s.allowUnresolvedExternalRoute(ctx, partition, inbound)
		if err != nil {
			return err
		}
		if !allowed {
			return nil
		}
	}
	boundary, err := s.Router.Route(ctx, InboundMessage{
		Partition:  partition,
		TenantID:   resolution.TenantID,
		ChannelID:  inbound.Channel,
		PeerID:     inbound.Conversation.ID,
		ThreadID:   inbound.Conversation.ThreadID,
		ActorID:    actorID,
		Owner:      resolution.Owner,
		Binding:    resolution.Binding,
		TrustClass: trustClass,
	})
	if err != nil || boundary == nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{
		"session_key":     boundary.SessionID,
		"channel":         inbound.Channel,
		"conversation_id": inbound.Conversation.ID,
		"thread_id":       inbound.Conversation.ThreadID,
		"sender_id":       inbound.Sender.ChannelID,
		"content":         inbound.Content,
	})
	_, err = s.Log.Append(ctx, partition, []core.FrameworkEvent{{
		Timestamp: ev.Timestamp,
		Type:      core.FrameworkEventSessionMessage,
		Payload:   payload,
		Actor:     core.EventActor{Kind: "session", ID: boundary.SessionID, TenantID: boundary.TenantID},
		Partition: partition,
		CausedBy:  []uint64{ev.Seq},
	}})
	return err
}

func OutboundMessageForSession(boundary *core.SessionBoundary, content string) (channel.OutboundMessage, error) {
	if boundary == nil {
		return channel.OutboundMessage{}, fmt.Errorf("session boundary required")
	}
	if boundary.ChannelID == "" || boundary.PeerID == "" {
		return channel.OutboundMessage{}, fmt.Errorf("session boundary missing channel routing")
	}
	return channel.OutboundMessage{
		Channel:        boundary.ChannelID,
		ConversationID: boundary.PeerID,
		Content:        channel.MessageContent{Text: content},
	}, nil
}

func isExternalSessionInbound(inbound channel.InboundMessage, resolution identity.Resolution) bool {
	if resolution.Binding != nil {
		return true
	}
	switch inbound.Channel {
	case "discord", "telegram", "webchat":
		return true
	default:
		return false
	}
}

func (s *SessionSink) partition() string {
	if s == nil || s.Partition == "" {
		return "local"
	}
	return s.Partition
}

func (s *SessionSink) maxUnresolvedExternalSessions() int {
	if s == nil || s.MaxUnresolvedExternalSessions <= 0 {
		return defaultMaxUnresolvedExternalSessions
	}
	return s.MaxUnresolvedExternalSessions
}

func (s *SessionSink) allowUnresolvedExternalRoute(ctx context.Context, partition string, inbound channel.InboundMessage) (bool, error) {
	router, ok := s.Router.(*DefaultRouter)
	if !ok || router == nil || router.Store == nil {
		return true, nil
	}
	scope := router.Scope
	if scope == "" {
		scope = core.SessionScopePerChannelPeer
	}
	key := core.SessionBoundaryKey(scope, partition, inbound.Channel, inbound.Conversation.ID, inbound.Conversation.ThreadID)
	boundary, err := router.Store.GetBoundary(ctx, key)
	if err != nil {
		return false, err
	}
	if boundary != nil {
		return true, nil
	}
	boundaries, err := router.Store.ListBoundaries(ctx, partition)
	if err != nil {
		return false, err
	}
	unresolved := 0
	for _, boundary := range boundaries {
		if boundary.TenantID == unresolvedExternalTenantID {
			unresolved++
		}
	}
	return unresolved < s.maxUnresolvedExternalSessions(), nil
}
