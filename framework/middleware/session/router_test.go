package session

import (
	"context"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type stubPolicyEngine struct {
	decision core.PolicyDecision
	err      error
	lastReq  core.PolicyRequest
}

func (s *stubPolicyEngine) Evaluate(_ context.Context, req core.PolicyRequest) (core.PolicyDecision, error) {
	s.lastReq = req
	return s.decision, s.err
}

type memoryStore struct {
	values      map[string]core.SessionBoundary
	delegations map[string][]core.SessionDelegationRecord
}

func (m *memoryStore) GetBoundary(_ context.Context, key string) (*core.SessionBoundary, error) {
	if boundary, ok := m.values[key]; ok {
		copy := boundary
		return &copy, nil
	}
	return nil, nil
}
func (m *memoryStore) GetBoundaryBySessionID(_ context.Context, sessionID string) (*core.SessionBoundary, error) {
	for _, boundary := range m.values {
		if boundary.SessionID == sessionID {
			copy := boundary
			return &copy, nil
		}
	}
	return nil, nil
}
func (m *memoryStore) UpsertBoundary(_ context.Context, key string, boundary *core.SessionBoundary) error {
	if m.values == nil {
		m.values = map[string]core.SessionBoundary{}
	}
	m.values[key] = *boundary
	return nil
}
func (m *memoryStore) ListBoundaries(_ context.Context, partition string) ([]core.SessionBoundary, error) {
	var out []core.SessionBoundary
	for _, boundary := range m.values {
		if boundary.Partition == partition {
			out = append(out, boundary)
		}
	}
	return out, nil
}
func (m *memoryStore) UpsertDelegation(_ context.Context, record core.SessionDelegationRecord) error {
	if m.delegations == nil {
		m.delegations = map[string][]core.SessionDelegationRecord{}
	}
	m.delegations[record.SessionID] = append(m.delegations[record.SessionID], record)
	return nil
}
func (m *memoryStore) ListDelegationsBySessionID(_ context.Context, sessionID string) ([]core.SessionDelegationRecord, error) {
	if m.delegations == nil {
		return nil, nil
	}
	return append([]core.SessionDelegationRecord(nil), m.delegations[sessionID]...), nil
}
func (m *memoryStore) DeleteBoundary(_ context.Context, key string) error {
	delete(m.values, key)
	return nil
}
func (m *memoryStore) DeleteExpiredBoundaries(_ context.Context, before time.Time) (int, error) {
	deleted := 0
	for key, boundary := range m.values {
		lastActivity := boundary.LastActivityAt
		if lastActivity.IsZero() {
			lastActivity = boundary.CreatedAt
		}
		if !lastActivity.After(before) {
			delete(m.values, key)
			deleted++
		}
	}
	return deleted, nil
}

func TestDefaultRouterRouteCreatesBoundary(t *testing.T) {
	store := &memoryStore{values: map[string]core.SessionBoundary{}}
	router := &DefaultRouter{Store: store, Scope: core.SessionScopePerChannelPeer}

	boundary, err := router.Route(context.Background(), InboundMessage{
		Partition:  "local",
		TenantID:   "tenant-1",
		ChannelID:  "telegram",
		PeerID:     "peer-1",
		ActorID:    "user-1",
		Owner:      core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindUser, ID: "user-1"},
		TrustClass: core.TrustClassWorkspaceTrusted,
	})
	require.NoError(t, err)
	require.NotEmpty(t, boundary.SessionID)
	require.NotEqual(t, "local:telegram:peer-1", boundary.SessionID)
	require.Equal(t, "local:telegram:peer-1", boundary.RoutingKey)
	require.Empty(t, boundary.ActorID)
	require.Equal(t, "tenant-1", boundary.TenantID)
	require.Equal(t, "user-1", boundary.Owner.ID)
}

func TestDefaultRouterRouteKeepsActorIDForRestrictedExternalSession(t *testing.T) {
	store := &memoryStore{values: map[string]core.SessionBoundary{}}
	router := &DefaultRouter{Store: store, Scope: core.SessionScopePerChannelPeer}

	boundary, err := router.Route(context.Background(), InboundMessage{
		Partition: "local",
		TenantID:  core.RestrictedExternalTenantID,
		ChannelID: "webchat",
		PeerID:    "conv-1",
		ActorID:   "external-user-1",
		Binding: &core.ExternalSessionBinding{
			Provider:       core.ExternalProviderWebchat,
			ConversationID: "conv-1",
			ExternalUserID: "external-user-1",
		},
		TrustClass: core.TrustClassRemoteDeclared,
	})
	require.NoError(t, err)
	require.Equal(t, "external-user-1", boundary.ActorID)
	require.Empty(t, boundary.Owner.ID)
}

func TestDefaultRouterAuthorizeRejectsActorMismatch(t *testing.T) {
	router := &DefaultRouter{}
	err := router.Authorize(context.Background(), AuthorizationRequest{
		Actor: core.EventActor{Kind: "user", ID: "other-user", TenantID: "tenant-1", SubjectKind: core.SubjectKindUser},
		Boundary: &core.SessionBoundary{
			SessionID: "local:telegram:peer-1",
			Partition: "local",
			ActorID:   "user-1",
			TenantID:  "tenant-1",
			Owner:     core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindUser, ID: "user-1"},
		},
	})
	require.ErrorIs(t, err, ErrSessionBoundaryViolation)
}

func TestDefaultRouterAuthorizeRejectsAuthenticatedOperatorWithoutPolicy(t *testing.T) {
	router := &DefaultRouter{}
	err := router.Authorize(context.Background(), AuthorizationRequest{
		Actor:         core.EventActor{Kind: "agent", ID: "agent-1", TenantID: "tenant-1"},
		Authenticated: true,
		Operation:     core.SessionOperationSend,
		Boundary: &core.SessionBoundary{
			SessionID: "local:telegram:peer-1",
			Partition: "local",
			ActorID:   "user-1",
			TenantID:  "tenant-1",
			Owner:     core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindUser, ID: "user-1"},
		},
	})
	require.ErrorIs(t, err, ErrSessionBoundaryViolation)
}

func TestDefaultRouterAuthorizeRejectsCrossTenantOperator(t *testing.T) {
	router := &DefaultRouter{}
	err := router.Authorize(context.Background(), AuthorizationRequest{
		Actor:         core.EventActor{Kind: "agent", ID: "agent-1", TenantID: "tenant-b"},
		Authenticated: true,
		Operation:     core.SessionOperationSend,
		Boundary: &core.SessionBoundary{
			SessionID: "sess_1",
			TenantID:  "tenant-a",
			Partition: "local",
			ActorID:   "user-1",
			Owner:     core.SubjectRef{TenantID: "tenant-a", Kind: core.SubjectKindUser, ID: "user-1"},
		},
	})
	require.ErrorIs(t, err, ErrSessionBoundaryViolation)
}

func TestDefaultRouterAuthorizeUsesPolicyEngine(t *testing.T) {
	engine := &stubPolicyEngine{decision: core.PolicyDecisionAllow("allowed")}
	router := &DefaultRouter{Policy: engine}
	err := router.Authorize(context.Background(), AuthorizationRequest{
		Actor:         core.EventActor{Kind: "agent", ID: "agent-1", TenantID: "tenant-1"},
		Authenticated: true,
		Operation:     core.SessionOperationSend,
		Boundary: &core.SessionBoundary{
			SessionID: "local:telegram:peer-1",
			Partition: "local",
			ActorID:   "user-1",
			TenantID:  "tenant-1",
			Owner:     core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindUser, ID: "user-1"},
			ChannelID: "telegram",
			Scope:     core.SessionScopePerChannelPeer,
			Binding: &core.ExternalSessionBinding{
				Provider:       core.ExternalProviderTelegram,
				AccountID:      "bot-1",
				ChannelID:      "telegram",
				ConversationID: "peer-1",
				ExternalUserID: "telegram-user-1",
			},
			TrustClass: core.TrustClassRemoteApproved,
		},
	})
	require.NoError(t, err)
	require.Equal(t, core.PolicyTargetSession, engine.lastReq.Target)
	require.Equal(t, core.SessionOperationSend, engine.lastReq.SessionOperation)
	require.True(t, engine.lastReq.Authenticated)
	require.True(t, engine.lastReq.ChannelID == "telegram")
	require.True(t, engine.lastReq.HasExternalBinding)
	require.Equal(t, core.ExternalProviderTelegram, engine.lastReq.ExternalProvider)
	require.Equal(t, "bot-1", engine.lastReq.ExternalAccountID)
	require.Equal(t, "peer-1", engine.lastReq.ExternalConversationID)
	require.True(t, engine.lastReq.ResolvedExternal)
	require.False(t, engine.lastReq.RestrictedExternal)
}

func TestDefaultRouterAuthorizeDeniesWhenPolicyEngineDenies(t *testing.T) {
	engine := &stubPolicyEngine{decision: core.PolicyDecisionDeny("blocked")}
	router := &DefaultRouter{Policy: engine}
	err := router.Authorize(context.Background(), AuthorizationRequest{
		Actor:         core.EventActor{Kind: "agent", ID: "agent-1", TenantID: "tenant-1"},
		Authenticated: true,
		Operation:     core.SessionOperationSend,
		Boundary: &core.SessionBoundary{
			SessionID:  "local:telegram:peer-1",
			Partition:  "local",
			ActorID:    "user-1",
			TenantID:   "tenant-1",
			Owner:      core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindUser, ID: "user-1"},
			ChannelID:  "telegram",
			Scope:      core.SessionScopePerChannelPeer,
			TrustClass: core.TrustClassRemoteApproved,
		},
	})
	require.ErrorIs(t, err, ErrSessionBoundaryViolation)
}

func TestDefaultRouterAuthorizePassesDelegationStateToPolicyEngine(t *testing.T) {
	engine := &stubPolicyEngine{decision: core.PolicyDecisionAllow("allowed")}
	store := &memoryStore{
		delegations: map[string][]core.SessionDelegationRecord{
			"sess_1": {{
				TenantID:  "tenant-1",
				SessionID: "sess_1",
				Grantee: core.SubjectRef{
					TenantID: "tenant-1",
					Kind:     core.SubjectKindServiceAccount,
					ID:       "operator-1",
				},
				Operations: []core.SessionOperation{core.SessionOperationSend},
				CreatedAt:  time.Now().UTC(),
			}},
		},
	}
	router := &DefaultRouter{Policy: engine, Store: store}
	err := router.Authorize(context.Background(), AuthorizationRequest{
		Actor:         core.EventActor{Kind: "agent", ID: "operator-1", TenantID: "tenant-1", SubjectKind: core.SubjectKindServiceAccount},
		Authenticated: true,
		Operation:     core.SessionOperationSend,
		Boundary: &core.SessionBoundary{
			SessionID:  "sess_1",
			Partition:  "local",
			TenantID:   "tenant-1",
			Owner:      core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindUser, ID: "user-1"},
			ChannelID:  "discord",
			Scope:      core.SessionScopePerChannelPeer,
			TrustClass: core.TrustClassRemoteApproved,
		},
	})
	require.NoError(t, err)
	require.True(t, engine.lastReq.IsDelegated)
	require.False(t, engine.lastReq.IsOwner)
}

func TestDefaultRouterAuthorizeRejectsOwnerlessBoundaryWithoutPolicy(t *testing.T) {
	router := &DefaultRouter{}
	err := router.Authorize(context.Background(), AuthorizationRequest{
		Actor:         core.EventActor{Kind: "agent", ID: "agent-1", TenantID: "tenant-1"},
		Authenticated: true,
		Operation:     core.SessionOperationSend,
		Boundary: &core.SessionBoundary{
			SessionID: "sess_1",
			Partition: "local",
			TenantID:  "tenant-1",
		},
	})
	require.ErrorIs(t, err, ErrSessionBoundaryViolation)
	require.ErrorContains(t, err, "has no owner")
}

func TestDefaultRouterAuthorizeAllowsRestrictedExternalLegacyOwnershipWithoutPolicy(t *testing.T) {
	router := &DefaultRouter{}
	err := router.Authorize(context.Background(), AuthorizationRequest{
		Actor:         core.EventActor{Kind: "channel", ID: "external-user-1", TenantID: core.RestrictedExternalTenantID},
		Authenticated: false,
		Operation:     core.SessionOperationSend,
		Boundary: &core.SessionBoundary{
			SessionID: "sess_restricted",
			Partition: "local",
			TenantID:  core.RestrictedExternalTenantID,
			ActorID:   "external-user-1",
			Binding: &core.ExternalSessionBinding{
				Provider:       core.ExternalProviderWebchat,
				ConversationID: "conv-1",
				ExternalUserID: "external-user-1",
			},
		},
	})
	require.NoError(t, err)
}

func TestDefaultRouterAuthorizeRejectsMissingActorIDForOwnedBoundary(t *testing.T) {
	router := &DefaultRouter{}
	err := router.Authorize(context.Background(), AuthorizationRequest{
		Actor:         core.EventActor{Kind: "agent", TenantID: "tenant-1"},
		Authenticated: true,
		Operation:     core.SessionOperationSend,
		Boundary: &core.SessionBoundary{
			SessionID: "sess_1",
			Partition: "local",
			TenantID:  "tenant-1",
			Owner:     core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindUser, ID: "user-1"},
		},
	})
	require.ErrorIs(t, err, ErrSessionBoundaryViolation)
	require.ErrorContains(t, err, "cannot access session")
}

func TestDefaultRouterAuthorizeAllowsDelegatedActorWithoutOwnership(t *testing.T) {
	store := &memoryStore{
		values: map[string]core.SessionBoundary{},
		delegations: map[string][]core.SessionDelegationRecord{
			"sess_1": {{
				TenantID:  "tenant-1",
				SessionID: "sess_1",
				Grantee: core.SubjectRef{
					TenantID: "tenant-1",
					Kind:     core.SubjectKindServiceAccount,
					ID:       "operator-1",
				},
				Operations: []core.SessionOperation{core.SessionOperationSend},
				CreatedAt:  time.Now().UTC(),
			}},
		},
	}
	router := &DefaultRouter{Store: store}
	err := router.Authorize(context.Background(), AuthorizationRequest{
		Actor:         core.EventActor{Kind: "agent", ID: "operator-1", TenantID: "tenant-1", SubjectKind: core.SubjectKindServiceAccount},
		Authenticated: true,
		Operation:     core.SessionOperationSend,
		Boundary: &core.SessionBoundary{
			SessionID: "sess_1",
			Partition: "local",
			TenantID:  "tenant-1",
			Owner:     core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindUser, ID: "user-1"},
		},
	})
	require.NoError(t, err)
}

func TestDefaultRouterAuthorizeRejectsDelegatedActorForWrongOperation(t *testing.T) {
	store := &memoryStore{
		values: map[string]core.SessionBoundary{},
		delegations: map[string][]core.SessionDelegationRecord{
			"sess_1": {{
				TenantID:  "tenant-1",
				SessionID: "sess_1",
				Grantee: core.SubjectRef{
					TenantID: "tenant-1",
					Kind:     core.SubjectKindServiceAccount,
					ID:       "operator-1",
				},
				Operations: []core.SessionOperation{core.SessionOperationResume},
				CreatedAt:  time.Now().UTC(),
			}},
		},
	}
	router := &DefaultRouter{Store: store}
	err := router.Authorize(context.Background(), AuthorizationRequest{
		Actor:         core.EventActor{Kind: "agent", ID: "operator-1", TenantID: "tenant-1", SubjectKind: core.SubjectKindServiceAccount},
		Authenticated: true,
		Operation:     core.SessionOperationSend,
		Boundary: &core.SessionBoundary{
			SessionID: "sess_1",
			Partition: "local",
			TenantID:  "tenant-1",
			Owner:     core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindUser, ID: "user-1"},
		},
	})
	require.ErrorIs(t, err, ErrSessionBoundaryViolation)
}

func TestDefaultRouterAuthorizeRejectsLegacyActorFallbackForNormalOwnerlessBoundary(t *testing.T) {
	router := &DefaultRouter{}
	err := router.Authorize(context.Background(), AuthorizationRequest{
		Actor:         core.EventActor{Kind: "user", ID: "user-1", TenantID: "tenant-1"},
		Authenticated: true,
		Operation:     core.SessionOperationSend,
		Boundary: &core.SessionBoundary{
			SessionID: "sess_1",
			Partition: "local",
			TenantID:  "tenant-1",
			ActorID:   "user-1",
		},
	})
	require.ErrorIs(t, err, ErrSessionBoundaryViolation)
	require.ErrorContains(t, err, "has no owner")
}

func TestDefaultRouterRouteRefreshesLastActivity(t *testing.T) {
	store := &memoryStore{values: map[string]core.SessionBoundary{
		"local:telegram:peer-1": {
			SessionID:      "local:telegram:peer-1",
			Scope:          core.SessionScopePerChannelPeer,
			Partition:      "local",
			ActorID:        "user-1",
			ChannelID:      "telegram",
			PeerID:         "peer-1",
			TrustClass:     core.TrustClassWorkspaceTrusted,
			CreatedAt:      time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
			LastActivityAt: time.Date(2026, 3, 1, 10, 5, 0, 0, time.UTC),
		},
	}}
	router := &DefaultRouter{
		Store:   store,
		Scope:   core.SessionScopePerChannelPeer,
		IdleTTL: time.Hour,
		now: func() time.Time {
			return time.Date(2026, 3, 1, 10, 30, 0, 0, time.UTC)
		},
	}

	boundary, err := router.Route(context.Background(), InboundMessage{
		Partition:  "local",
		ChannelID:  "telegram",
		PeerID:     "peer-1",
		ActorID:    "user-1",
		Owner:      core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindUser, ID: "user-1"},
		TenantID:   "tenant-1",
		TrustClass: core.TrustClassWorkspaceTrusted,
	})
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC), boundary.CreatedAt)
	require.Equal(t, time.Date(2026, 3, 1, 10, 30, 0, 0, time.UTC), boundary.LastActivityAt)
}

func TestDefaultRouterRouteRecreatesExpiredBoundary(t *testing.T) {
	store := &memoryStore{values: map[string]core.SessionBoundary{
		"local:telegram:peer-1": {
			SessionID:      "local:telegram:peer-1",
			Scope:          core.SessionScopePerChannelPeer,
			Partition:      "local",
			ActorID:        "user-1",
			ChannelID:      "telegram",
			PeerID:         "peer-1",
			TrustClass:     core.TrustClassWorkspaceTrusted,
			CreatedAt:      time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC),
			LastActivityAt: time.Date(2026, 3, 1, 8, 10, 0, 0, time.UTC),
		},
	}}
	router := &DefaultRouter{
		Store:   store,
		Scope:   core.SessionScopePerChannelPeer,
		IdleTTL: 30 * time.Minute,
		now: func() time.Time {
			return time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
		},
	}

	boundary, err := router.Route(context.Background(), InboundMessage{
		Partition:  "local",
		ChannelID:  "telegram",
		PeerID:     "peer-1",
		ActorID:    "user-1",
		Owner:      core.SubjectRef{TenantID: "tenant-1", Kind: core.SubjectKindUser, ID: "user-1"},
		TenantID:   "tenant-1",
		TrustClass: core.TrustClassWorkspaceTrusted,
	})
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC), boundary.CreatedAt)
	require.Equal(t, time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC), boundary.LastActivityAt)
}
