package identity

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubjectKindValidate(t *testing.T) {
	valid := []SubjectKind{
		SubjectKindUser,
		SubjectKindServiceAccount,
		SubjectKindNode,
		SubjectKindExternalIdentity,
		SubjectKindSystem,
	}
	for _, kind := range valid {
		assert.NoError(t, kind.Validate(), "expected %q to be a valid subject kind", kind)
	}

	for _, kind := range []SubjectKind{"", "robot", "User"} {
		require.Error(t, kind.Validate(), "expected %q to be rejected", kind)
	}
}

func TestAuthMethodValidate(t *testing.T) {
	valid := []AuthMethod{
		AuthMethodAnonymous,
		AuthMethodBearerToken,
		AuthMethodOIDC,
		AuthMethodNodeChallenge,
		AuthMethodProviderBind,
		AuthMethodBootstrapAdmin,
	}
	for _, method := range valid {
		assert.NoError(t, method.Validate(), "expected %q to be a valid auth method", method)
	}

	for _, method := range []AuthMethod{"", "password", "Bearer"} {
		require.Error(t, method.Validate(), "expected %q to be rejected", method)
	}
}

func TestExternalProviderValidate(t *testing.T) {
	for _, provider := range []ExternalProvider{
		ExternalProviderDiscord,
		ExternalProviderTelegram,
		ExternalProviderWebchat,
		ExternalProviderNexus,
	} {
		assert.NoError(t, provider.Validate(), "expected %q to be a valid provider", provider)
	}

	for _, provider := range []ExternalProvider{"", "slack", "Discord"} {
		require.Error(t, provider.Validate(), "expected %q to be rejected", provider)
	}
}

func TestSubjectRefValidate(t *testing.T) {
	valid := SubjectRef{TenantID: "t-1", Kind: SubjectKindUser, ID: "alice"}
	require.NoError(t, valid.Validate())

	testCases := []struct {
		name    string
		subject SubjectRef
		wantErr string
	}{
		{"missing tenant", SubjectRef{Kind: SubjectKindUser, ID: "alice"}, "tenant_id required"},
		{"blank tenant", SubjectRef{TenantID: "   ", Kind: SubjectKindUser, ID: "alice"}, "tenant_id required"},
		{"invalid kind", SubjectRef{TenantID: "t-1", Kind: "robot", ID: "alice"}, "subject kind robot invalid"},
		{"missing id", SubjectRef{TenantID: "t-1", Kind: SubjectKindUser}, "id required"},
		{"blank id", SubjectRef{TenantID: "t-1", Kind: SubjectKindUser, ID: " \t "}, "id required"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.subject.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestSubjectRefMatches(t *testing.T) {
	subject := SubjectRef{TenantID: "Tenant-1", Kind: SubjectKindUser, ID: "Alice"}

	testCases := []struct {
		name  string
		actor EventActor
		want  bool
	}{
		{
			name:  "exact match",
			actor: EventActor{Kind: "user", ID: "Alice", TenantID: "Tenant-1"},
			want:  true,
		},
		{
			name:  "case-insensitive id and tenant",
			actor: EventActor{Kind: "user", ID: "aLiCe", TenantID: "tenant-1"},
			want:  true,
		},
		{
			name:  "actor without tenant still matches tenant-scoped subject",
			actor: EventActor{Kind: "user", ID: "alice"},
			want:  true,
		},
		{
			name:  "tenant mismatch rejects",
			actor: EventActor{Kind: "user", ID: "alice", TenantID: "tenant-2"},
			want:  false,
		},
		{
			name:  "declared subject kind mismatch rejects",
			actor: EventActor{Kind: "user", ID: "alice", TenantID: "tenant-1", SubjectKind: "service_account"},
			want:  false,
		},
		{
			name:  "matching declared subject kind passes",
			actor: EventActor{Kind: "user", ID: "alice", TenantID: "tenant-1", SubjectKind: "user"},
			want:  true,
		},
		{
			name:  "empty actor id rejects",
			actor: EventActor{Kind: "user", TenantID: "tenant-1"},
			want:  false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, subject.Matches(tc.actor))
		})
	}

	emptyKind := SubjectRef{TenantID: "t-1", ID: "alice"}
	assert.False(t, emptyKind.Matches(EventActor{ID: "alice"}), "subject with empty kind must never match")

	tenantless := SubjectRef{Kind: SubjectKindUser, ID: "alice"}
	assert.True(t, tenantless.Matches(EventActor{ID: "alice", TenantID: "any-tenant"}),
		"subject without tenant must ignore actor tenant")
}

func TestAuthenticatedPrincipalValidate(t *testing.T) {
	issued := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	valid := AuthenticatedPrincipal{
		TenantID:      "t-1",
		Subject:       SubjectRef{TenantID: "t-1", Kind: SubjectKindUser, ID: "alice"},
		AuthMethod:    AuthMethodBearerToken,
		Authenticated: true,
		IssuedAt:      issued,
		ExpiresAt:     issued.Add(time.Hour),
	}
	require.NoError(t, valid.Validate())

	testCases := []struct {
		name    string
		mutate  func(*AuthenticatedPrincipal)
		wantErr string
	}{
		{
			name:    "invalid auth method",
			mutate:  func(p *AuthenticatedPrincipal) { p.AuthMethod = "magic" },
			wantErr: "auth method magic invalid",
		},
		{
			name:    "invalid subject",
			mutate:  func(p *AuthenticatedPrincipal) { p.Subject.ID = "" },
			wantErr: "id required",
		},
		{
			name: "tenant mismatch with subject",
			mutate: func(p *AuthenticatedPrincipal) {
				p.TenantID = "t-2"
			},
			wantErr: "tenant_id must match subject tenant_id",
		},
		{
			name: "expiry before issuance",
			mutate: func(p *AuthenticatedPrincipal) {
				p.ExpiresAt = issued.Add(-time.Minute)
			},
			wantErr: "expires_at must be after issued_at",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			principal := valid
			tc.mutate(&principal)
			err := principal.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}

	t.Run("tenant only on principal is allowed when subject carries it", func(t *testing.T) {
		principal := AuthenticatedPrincipal{
			TenantID:   "t-1",
			Subject:    SubjectRef{TenantID: "T-1", Kind: SubjectKindUser, ID: "alice"},
			AuthMethod: AuthMethodOIDC,
		}
		assert.NoError(t, principal.Validate())
	})

	t.Run("zero timestamps skip expiry ordering check", func(t *testing.T) {
		principal := AuthenticatedPrincipal{
			Subject:    SubjectRef{TenantID: "t-1", Kind: SubjectKindSystem, ID: "system"},
			AuthMethod: AuthMethodBootstrapAdmin,
		}
		assert.NoError(t, principal.Validate())
	})
}

func TestConnectionPrincipalCarriesPrincipalFields(t *testing.T) {
	principal := &AuthenticatedPrincipal{
		Subject:    SubjectRef{TenantID: "t-1", Kind: SubjectKindNode, ID: "node-7"},
		AuthMethod: AuthMethodNodeChallenge,
		SessionID:  "sess-1",
		Scopes:     []string{"read", "write"},
	}
	conn := ConnectionPrincipal{
		Role:          "agent",
		Actor:         EventActor{Kind: "node", ID: "node-7"},
		Authenticated: true,
		Principal:     principal,
		FeedScope:     "workspace",
	}

	assert.Equal(t, "agent", conn.Role)
	assert.True(t, conn.Authenticated)
	assert.Equal(t, "sess-1", conn.Principal.SessionID)
	assert.Equal(t, []string{"read", "write"}, conn.Principal.Scopes)
	assert.Equal(t, SubjectKindNode, conn.Principal.Subject.Kind)
}
