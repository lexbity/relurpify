package session

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/event"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
)

// ErrSessionBoundaryViolation is returned when a caller crosses a session boundary.
var ErrSessionBoundaryViolation = errors.New("session boundary violation")

const DefaultBoundaryIdleTTL = 24 * time.Hour

// Router routes inbound messages to session boundaries.
type Router interface {
	Route(ctx context.Context, msg InboundMessage) (*core.SessionBoundary, error)
	Authorize(ctx context.Context, req AuthorizationRequest) error
}

// InboundMessage is the normalized input to the session router.
type InboundMessage struct {
	Partition  string
	TenantID   string
	ChannelID  string
	PeerID     string
	ThreadID   string
	ActorID    string
	Owner      identity.SubjectRef
	Binding    *identity.ExternalSessionBinding
	TrustClass core.TrustClass
}

// Store persists session boundaries and state.
type Store interface {
	GetBoundary(ctx context.Context, key string) (*core.SessionBoundary, error)
	GetBoundaryBySessionID(ctx context.Context, sessionID string) (*core.SessionBoundary, error)
	UpsertBoundary(ctx context.Context, key string, boundary *core.SessionBoundary) error
	ListBoundaries(ctx context.Context, partition string) ([]core.SessionBoundary, error)
	UpsertDelegation(ctx context.Context, record core.SessionDelegationRecord) error
	ListDelegationsBySessionID(ctx context.Context, sessionID string) ([]core.SessionDelegationRecord, error)
	ListDelegationsByTenantID(ctx context.Context, tenantID string) ([]core.SessionDelegationRecord, error)
	DeleteBoundary(ctx context.Context, key string) error
	DeleteExpiredBoundaries(ctx context.Context, before time.Time) (int, error)
}

type PolicyEngine = authorization.PolicyEngine

// AuthorizationRequest is the normalized session authorization input.
type AuthorizationRequest struct {
	Actor         core.EventActor
	Authenticated bool
	Operation     core.SessionOperation
	Boundary      *core.SessionBoundary
}

// DefaultRouter is the standard implementation of Router.
type DefaultRouter struct {
	Policy  PolicyEngine
	Store   Store
	Log     event.Log
	Scope   core.SessionScope
	IdleTTL time.Duration
	now     func() time.Time
}

func (r *DefaultRouter) Route(ctx context.Context, msg InboundMessage) (*core.SessionBoundary, error) {
	if r == nil || r.Store == nil {
		return nil, fmt.Errorf("session router unavailable")
	}
	scope := r.Scope
	if scope == "" {
		scope = core.SessionScopePerChannelPeer
	}
	partition := strings.TrimSpace(msg.Partition)
	if partition == "" {
		partition = "local"
	}
	now := r.nowUTC()
	if _, err := r.Store.DeleteExpiredBoundaries(ctx, now.Add(-r.boundaryIdleTTL())); err != nil {
		return nil, err
	}
	key := core.SessionBoundaryKey(scope, partition, msg.ChannelID, msg.PeerID, msg.ThreadID)
	boundary, err := r.Store.GetBoundary(ctx, key)
	if err != nil {
		return nil, err
	}
	if boundary != nil {
		if boundary.RoutingKey == "" {
			boundary.RoutingKey = key
		}
		if boundary.CreatedAt.IsZero() {
			boundary.CreatedAt = now
		}
		boundary.LastActivityAt = now
		if err := r.Store.UpsertBoundary(ctx, key, boundary); err != nil {
			return nil, err
		}
		return boundary, nil
	}
	sessionID, err := generateOpaqueSessionID()
	if err != nil {
		return nil, err
	}
	boundary = &core.SessionBoundary{
		SessionID:      sessionID,
		RoutingKey:     key,
		TenantID:       msg.TenantID,
		Scope:          scope,
		Partition:      partition,
		ActorID:        routedActorID(msg),
		Owner:          core.DelegationSubjectRef{TenantID: msg.Owner.TenantID, Kind: string(msg.Owner.Kind), ID: msg.Owner.ID},
		ChannelID:      msg.ChannelID,
		PeerID:         msg.PeerID,
		Binding:        sessionBindingFromIdentity(msg.Binding),
		TrustClass:     msg.TrustClass,
		CreatedAt:      now,
		LastActivityAt: now,
	}
	if err := r.Store.UpsertBoundary(ctx, key, boundary); err != nil {
		return nil, err
	}
	if r.Log != nil {
		payload := []byte(fmt.Sprintf(`{"session_id":%q,"partition":%q}`, boundary.SessionID, boundary.Partition))
		_, _ = r.Log.Append(ctx, partition, []core.FrameworkEvent{{
			Timestamp: time.Now().UTC(),
			Type:      core.FrameworkEventSessionCreated,
			Payload:   payload,
			Actor:     core.EventActor{Kind: "system", ID: "session-router", TenantID: boundary.TenantID},
			Partition: partition,
		}})
	}
	return boundary, nil
}

func (r *DefaultRouter) boundaryIdleTTL() time.Duration {
	if r == nil || r.IdleTTL <= 0 {
		return DefaultBoundaryIdleTTL
	}
	return r.IdleTTL
}

func (r *DefaultRouter) nowUTC() time.Time {
	if r != nil && r.now != nil {
		return r.now().UTC()
	}
	return time.Now().UTC()
}

func (r *DefaultRouter) Authorize(ctx context.Context, req AuthorizationRequest) error {
	boundary := req.Boundary
	if boundary == nil {
		return fmt.Errorf("%w: boundary missing", ErrSessionBoundaryViolation)
	}
	if boundary.Partition == "" {
		return fmt.Errorf("%w: partition missing", ErrSessionBoundaryViolation)
	}
	if req.Operation == "" {
		req.Operation = core.SessionOperationResume
	}
	if boundary.TenantID != "" {
		if strings.TrimSpace(req.Actor.TenantID) == "" {
			return fmt.Errorf("%w: actor tenant missing for session %s", ErrSessionBoundaryViolation, boundary.SessionID)
		}
		if !strings.EqualFold(req.Actor.TenantID, boundary.TenantID) {
			return fmt.Errorf("%w: actor tenant %s cannot access session %s in tenant %s", ErrSessionBoundaryViolation, req.Actor.TenantID, boundary.SessionID, boundary.TenantID)
		}
	}
	isOwner := boundary.OwnerMatches(req.Actor)
	isDelegated, err := r.isDelegated(ctx, boundary, req.Actor, req.Operation)
	if err != nil {
		return err
	}
	hasExternalBinding := boundary.Binding != nil
	resolvedExternal := hasExternalBinding && boundary.Owner.ID != ""
	restrictedExternal := boundary.AllowsLegacyActorOwnership()
	externalProvider := identity.ExternalProvider("")
	externalAccountID := ""
	externalChannelID := ""
	externalConversationID := ""
	externalThreadID := ""
	externalUserID := ""
	if boundary.Binding != nil {
		externalProvider = identity.ExternalProvider(boundary.Binding.Provider)
		externalAccountID = boundary.Binding.AccountID
		externalChannelID = boundary.Binding.ChannelID
		externalConversationID = boundary.Binding.ConversationID
		externalThreadID = boundary.Binding.ThreadID
		externalUserID = boundary.Binding.ExternalUserID
	}
	if r != nil && r.Policy != nil {
		decision, err := authorization.EvaluatePolicyRequest(ctx, r.Policy, core.PolicyRequest{
			Target:                 core.PolicyTargetSession,
			Actor:                  req.Actor,
			Authenticated:          req.Authenticated,
			ActorTenantID:          req.Actor.TenantID,
			ResourceTenantID:       boundary.TenantID,
			TrustClass:             boundary.TrustClass,
			Partition:              boundary.Partition,
			ChannelID:              boundary.ChannelID,
			SessionID:              boundary.SessionID,
			SessionScope:           boundary.Scope,
			SessionOperation:       req.Operation,
			SessionOwnerID:         sessionOwnerID(boundary),
			IsOwner:                isOwner,
			IsDelegated:            isDelegated,
			ExternalProvider:       string(externalProvider),
			ExternalAccountID:      externalAccountID,
			ExternalChannelID:      externalChannelID,
			ExternalConversationID: externalConversationID,
			ExternalThreadID:       externalThreadID,
			ExternalUserID:         externalUserID,
			HasExternalBinding:     hasExternalBinding,
			ResolvedExternal:       resolvedExternal,
			RestrictedExternal:     restrictedExternal,
		})
		if err != nil {
			return err
		}
		switch decision.Effect {
		case "allow":
			return nil
		case "require_approval":
			return fmt.Errorf("%w: approval required for session %s", ErrSessionBoundaryViolation, boundary.SessionID)
		default:
			return fmt.Errorf("%w: actor %s cannot access session %s", ErrSessionBoundaryViolation, req.Actor.ID, boundary.SessionID)
		}
	}
	if sessionOwnerID(boundary) == "" {
		return fmt.Errorf("%w: session %s has no owner", ErrSessionBoundaryViolation, boundary.SessionID)
	}
	if !isOwner && !isDelegated {
		return fmt.Errorf("%w: actor %s cannot access session %s", ErrSessionBoundaryViolation, req.Actor.ID, boundary.SessionID)
	}
	return nil
}

var generateOpaqueSessionID = func() (string, error) {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "sess_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

func sessionOwnerID(boundary *core.SessionBoundary) string {
	if boundary == nil {
		return ""
	}
	if boundary.HasCanonicalOwner() {
		return boundary.Owner.ID
	}
	if boundary.AllowsLegacyActorOwnership() {
		return boundary.ActorID
	}
	return ""
}

func routedActorID(msg InboundMessage) string {
	if msg.Owner.ID != "" {
		return ""
	}
	return msg.ActorID
}

func (r *DefaultRouter) isDelegated(ctx context.Context, boundary *core.SessionBoundary, actor core.EventActor, operation core.SessionOperation) (bool, error) {
	if r == nil || r.Store == nil || boundary == nil {
		return false, nil
	}
	if strings.TrimSpace(actor.ID) == "" || actor.SubjectKind == "" {
		return false, nil
	}
	records, err := r.Store.ListDelegationsBySessionID(ctx, boundary.SessionID)
	if err != nil {
		return false, err
	}
	now := r.nowUTC()
	for _, record := range records {
		if record.Allows(actor, operation, now) {
			return true, nil
		}
	}
	return false, nil
}

func sessionBindingFromIdentity(b *identity.ExternalSessionBinding) *core.SessionBinding {
	if b == nil {
		return nil
	}
	return &core.SessionBinding{
		Provider:       string(b.Provider),
		AccountID:      b.AccountID,
		ChannelID:      b.ChannelID,
		ConversationID: b.ConversationID,
		ThreadID:       b.ThreadID,
		ExternalUserID: b.ExternalUserID,
	}
}
