package identity

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type tenantStoreFixture struct {
	tenants []TenantRecord
}

func (s tenantStoreFixture) UpsertTenant(context.Context, TenantRecord) error { return nil }

func (s tenantStoreFixture) GetTenant(context.Context, string) (*TenantRecord, error) {
	return nil, nil
}

func (s tenantStoreFixture) ListTenants(context.Context) ([]TenantRecord, error) {
	return append([]TenantRecord(nil), s.tenants...), nil
}

type subjectStoreFixture struct {
	identities map[string]ExternalIdentity
}

func (s subjectStoreFixture) UpsertSubject(context.Context, SubjectRecord) error { return nil }

func (s subjectStoreFixture) GetSubject(context.Context, string, SubjectKind, string) (*SubjectRecord, error) {
	return nil, nil
}

func (s subjectStoreFixture) ListSubjects(context.Context, string) ([]SubjectRecord, error) {
	return nil, nil
}

func (s subjectStoreFixture) UpsertExternalIdentity(context.Context, ExternalIdentity) error {
	return nil
}

func (s subjectStoreFixture) GetExternalIdentity(_ context.Context, tenantID string, provider ExternalProvider, accountID, externalID string) (*ExternalIdentity, error) {
	key := externalIdentityKey(tenantID, provider, accountID, externalID)
	if identity, ok := s.identities[key]; ok {
		record := identity
		return &record, nil
	}
	return nil, nil
}

func (s subjectStoreFixture) ListExternalIdentities(context.Context, string) ([]ExternalIdentity, error) {
	return nil, nil
}

func externalIdentityKey(tenantID string, provider ExternalProvider, accountID, externalID string) string {
	return tenantID + "|" + string(provider) + "|" + accountID + "|" + externalID
}

type inboundMessageFixture struct {
	channel      string
	account      string
	resolvedID   string
	channelID    string
	conversation inboundConversationFixture
}

func (m inboundMessageFixture) GetChannel() string { return m.channel }

func (m inboundMessageFixture) GetAccount() string { return m.account }

func (m inboundMessageFixture) GetSender() InboundSender {
	return inboundSenderFixture{resolvedID: m.resolvedID, channelID: m.channelID}
}

func (m inboundMessageFixture) GetConversation() InboundConversation { return m.conversation }

type inboundSenderFixture struct {
	resolvedID string
	channelID  string
}

func (s inboundSenderFixture) GetResolvedID() string { return s.resolvedID }

func (s inboundSenderFixture) GetChannelID() string { return s.channelID }

type inboundConversationFixture struct {
	id       string
	threadID string
}

func (c inboundConversationFixture) GetID() string { return c.id }

func (c inboundConversationFixture) GetThreadID() string { return c.threadID }

func TestStoreResolverResolvesExternalIdentityAcrossTenants(t *testing.T) {
	resolver := StoreResolver{
		Tenants: tenantStoreFixture{
			tenants: []TenantRecord{{ID: "tenant-a"}, {ID: "tenant-b"}},
		},
		Subjects: subjectStoreFixture{
			identities: map[string]ExternalIdentity{
				externalIdentityKey("tenant-b", ExternalProviderDiscord, "guild-1", "user-42"): {
					TenantID:   "tenant-b",
					Provider:   ExternalProviderDiscord,
					AccountID:  "guild-1",
					ExternalID: "user-42",
					Subject: SubjectRef{
						TenantID: "tenant-b",
						Kind:     SubjectKindUser,
						ID:       "subject-9",
					},
				},
			},
		},
		DefaultTenantID: "tenant-a",
	}

	resolution, err := resolver.ResolveInbound(context.Background(), inboundMessageFixture{
		channel:    "discord",
		account:    "guild-1",
		resolvedID: "user-42",
		conversation: inboundConversationFixture{
			id:       "conv-1",
			threadID: "thread-7",
		},
	})
	require.NoError(t, err)
	require.Equal(t, ResolutionStateResolved, resolution.State)
	require.Equal(t, "tenant-b", resolution.TenantID)
	require.Equal(t, "subject-9", resolution.Owner.ID)
	require.NotNil(t, resolution.Binding)
	require.Equal(t, "conv-1", resolution.Binding.ConversationID)
	require.Equal(t, "thread-7", resolution.Binding.ThreadID)
}

func TestStoreResolverReturnsUnboundWithoutIdentityMatch(t *testing.T) {
	resolver := StoreResolver{
		Tenants: tenantStoreFixture{
			tenants: []TenantRecord{{ID: "tenant-a"}},
		},
		Subjects:        subjectStoreFixture{},
		DefaultTenantID: "tenant-a",
	}

	resolution, err := resolver.ResolveInbound(context.Background(), inboundMessageFixture{
		channel:    "webchat",
		account:    "account-1",
		resolvedID: "missing-user",
		conversation: inboundConversationFixture{
			id: "conv-2",
		},
	})
	require.NoError(t, err)
	require.Equal(t, ResolutionStateUnbound, resolution.State)
	require.Equal(t, "tenant-a", resolution.TenantID)
	require.NotNil(t, resolution.Binding)
	require.Equal(t, "conv-2", resolution.Binding.ConversationID)
}
