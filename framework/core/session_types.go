package core

import (
	"strings"
	"time"
)

type SessionScope string

const (
	SessionScopeMain           SessionScope = "main"
	SessionScopePerChannelPeer SessionScope = "per-channel-peer"
	SessionScopePerThread      SessionScope = "per-thread"
)

// SessionBoundary is the framework-enforced session security envelope.
type SessionBoundary struct {
	SessionID      string                  `json:"session_id" yaml:"session_id"`
	RoutingKey     string                  `json:"routing_key,omitempty" yaml:"routing_key,omitempty"`
	TenantID       string                  `json:"tenant_id,omitempty" yaml:"tenant_id,omitempty"`
	Scope          SessionScope            `json:"scope" yaml:"scope"`
	Partition      string                  `json:"partition" yaml:"partition"`
	ActorID        string                  `json:"actor_id,omitempty" yaml:"actor_id,omitempty"`
	Owner          SubjectRef              `json:"owner,omitempty" yaml:"owner,omitempty"`
	ChannelID      string                  `json:"channel_id,omitempty" yaml:"channel_id,omitempty"`
	PeerID         string                  `json:"peer_id,omitempty" yaml:"peer_id,omitempty"`
	Binding        *ExternalSessionBinding `json:"binding,omitempty" yaml:"binding,omitempty"`
	TrustClass     TrustClass              `json:"trust_class" yaml:"trust_class"`
	CreatedAt      time.Time               `json:"created_at" yaml:"created_at"`
	LastActivityAt time.Time               `json:"last_activity_at,omitempty" yaml:"last_activity_at,omitempty"`
}

func (b SessionBoundary) OwnerMatches(actor EventActor) bool {
	if b.Owner.ID != "" {
		return b.Owner.Matches(actor)
	}
	if b.ActorID == "" || actor.ID == "" {
		return false
	}
	if b.TenantID != "" && actor.TenantID != "" && !strings.EqualFold(b.TenantID, actor.TenantID) {
		return false
	}
	return strings.EqualFold(b.ActorID, actor.ID)
}

// SessionBoundaryKey returns the canonical routing key for a session scope.
func SessionBoundaryKey(scope SessionScope, partition, channel, peer, thread string) string {
	switch scope {
	case SessionScopeMain:
		return partition
	case SessionScopePerChannelPeer:
		return partition + ":" + channel + ":" + peer
	case SessionScopePerThread:
		return partition + ":" + channel + ":" + peer + ":" + thread
	default:
		return partition + ":" + channel + ":" + peer
	}
}
