package core

import (
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/relurpnet/identity"
)

const RestrictedExternalTenantID = "__relurpify_unresolved_external__"

type SessionScope string

const (
	SessionScopeMain           SessionScope = "main"
	SessionScopePerChannelPeer SessionScope = "per-channel-peer"
	SessionScopePerThread      SessionScope = "per-thread"
)

// SessionBoundary is the framework-enforced session security envelope.
type SessionBoundary struct {
	SessionID      string               `json:"session_id" yaml:"session_id"`
	RoutingKey     string               `json:"routing_key,omitempty" yaml:"routing_key,omitempty"`
	TenantID       string               `json:"tenant_id,omitempty" yaml:"tenant_id,omitempty"`
	Scope          SessionScope         `json:"scope" yaml:"scope"`
	Partition      string               `json:"partition" yaml:"partition"`
	ActorID        string               `json:"actor_id,omitempty" yaml:"actor_id,omitempty"`
	Owner          DelegationSubjectRef `json:"owner,omitempty" yaml:"owner,omitempty"`
	ChannelID      string               `json:"channel_id,omitempty" yaml:"channel_id,omitempty"`
	PeerID         string               `json:"peer_id,omitempty" yaml:"peer_id,omitempty"`
	Binding        *SessionBinding      `json:"binding,omitempty" yaml:"binding,omitempty"`
	TrustClass     TrustClass           `json:"trust_class" yaml:"trust_class"`
	CreatedAt      time.Time            `json:"created_at" yaml:"created_at"`
	LastActivityAt time.Time            `json:"last_activity_at,omitempty" yaml:"last_activity_at,omitempty"`
}

func (b SessionBoundary) OwnerMatches(actor identity.EventActor) bool {
	if b.Owner.ID != "" {
		return b.Owner.Matches(actor)
	}
	if !b.AllowsLegacyActorOwnership() {
		return false
	}
	if b.ActorID == "" || actor.ID == "" {
		return false
	}
	if b.TenantID != "" && actor.TenantID != "" && !strings.EqualFold(b.TenantID, actor.TenantID) {
		return false
	}
	return strings.EqualFold(b.ActorID, actor.ID)
}

func (b SessionBoundary) HasCanonicalOwner() bool {
	return strings.TrimSpace(b.Owner.ID) != ""
}

func (b SessionBoundary) AllowsLegacyActorOwnership() bool {
	return b.Binding != nil &&
		!b.HasCanonicalOwner() &&
		strings.EqualFold(strings.TrimSpace(b.TenantID), RestrictedExternalTenantID)
}

// SessionBinding captures external identity provider linkage for a session.
type SessionBinding struct {
	Provider       string `json:"provider,omitempty" yaml:"provider,omitempty"`
	ProviderID     string `json:"provider_id,omitempty" yaml:"provider_id,omitempty"`
	AccountID      string `json:"account_id,omitempty" yaml:"account_id,omitempty"`
	ChannelID      string `json:"channel_id,omitempty" yaml:"channel_id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty" yaml:"conversation_id,omitempty"`
	ThreadID       string `json:"thread_id,omitempty" yaml:"thread_id,omitempty"`
	ExternalUserID string `json:"external_user_id,omitempty" yaml:"external_user_id,omitempty"`
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
