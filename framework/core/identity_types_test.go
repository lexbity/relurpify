package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSubjectRefValidate(t *testing.T) {
	ref := SubjectRef{
		TenantID: "tenant-1",
		Kind:     SubjectKindUser,
		ID:       "user-1",
	}

	require.NoError(t, ref.Validate())
}

func TestSubjectRefMatchesActor(t *testing.T) {
	ref := SubjectRef{
		TenantID: "tenant-1",
		Kind:     SubjectKindUser,
		ID:       "user-1",
	}
	actor := EventActor{
		ID:          "user-1",
		TenantID:    "tenant-1",
		SubjectKind: SubjectKindUser,
	}

	require.True(t, ref.Matches(actor))
}

func TestAuthenticatedPrincipalValidateRejectsTenantMismatch(t *testing.T) {
	err := (AuthenticatedPrincipal{
		TenantID:   "tenant-a",
		AuthMethod: AuthMethodBearerToken,
		Subject: SubjectRef{
			TenantID: "tenant-b",
			Kind:     SubjectKindUser,
			ID:       "user-1",
		},
		Authenticated: true,
	}).Validate()

	require.Error(t, err)
	require.Contains(t, err.Error(), "tenant_id")
}

func TestExternalIdentityValidate(t *testing.T) {
	identity := ExternalIdentity{
		TenantID:   "tenant-1",
		Provider:   ExternalProviderDiscord,
		ExternalID: "discord-user-1",
		Subject: SubjectRef{
			TenantID: "tenant-1",
			Kind:     SubjectKindUser,
			ID:       "user-1",
		},
	}

	require.NoError(t, identity.Validate())
}

func TestNodeEnrollmentValidate(t *testing.T) {
	now := time.Now().UTC()
	enrollment := NodeEnrollment{
		TenantID:   "tenant-1",
		NodeID:     "node-1",
		TrustClass: TrustClassWorkspaceTrusted,
		Owner: SubjectRef{
			TenantID: "tenant-1",
			Kind:     SubjectKindNode,
			ID:       "node-1",
		},
		PairedAt:       now,
		LastVerifiedAt: now.Add(time.Minute),
		AuthMethod:     AuthMethodNodeChallenge,
	}

	require.NoError(t, enrollment.Validate())
}

func TestSessionBoundaryOwnerMatchesOwnerRef(t *testing.T) {
	boundary := SessionBoundary{
		TenantID: "tenant-1",
		Owner: SubjectRef{
			TenantID: "tenant-1",
			Kind:     SubjectKindUser,
			ID:       "user-1",
		},
	}
	actor := EventActor{
		ID:          "user-1",
		TenantID:    "tenant-1",
		SubjectKind: SubjectKindUser,
	}

	require.True(t, boundary.OwnerMatches(actor))
}
