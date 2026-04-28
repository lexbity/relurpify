package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSessionDelegationRecordValidate(t *testing.T) {
	record := SessionDelegationRecord{
		TenantID:  "tenant-1",
		SessionID: "sess_1",
		Grantee: DelegationSubjectRef{
			TenantID: "tenant-1",
			Kind:     "service_account",
			ID:       "operator-1",
		},
		Operations: []SessionOperation{SessionOperationSend},
		CreatedAt:  time.Now().UTC(),
	}

	require.NoError(t, record.Validate())
}

func TestSessionDelegationRecordAllowsActorAndOperation(t *testing.T) {
	record := SessionDelegationRecord{
		TenantID:  "tenant-1",
		SessionID: "sess_1",
		Grantee: DelegationSubjectRef{
			TenantID: "tenant-1",
			Kind:     "service_account",
			ID:       "operator-1",
		},
		Operations: []SessionOperation{SessionOperationSend},
		CreatedAt:  time.Now().UTC(),
	}

	require.True(t, record.Allows(EventActor{
		ID:          "operator-1",
		TenantID:    "tenant-1",
		SubjectKind: "service_account",
	}, SessionOperationSend, time.Now().UTC()))

	require.False(t, record.Allows(EventActor{
		ID:          "operator-1",
		TenantID:    "tenant-1",
		SubjectKind: "service_account",
	}, SessionOperationResume, time.Now().UTC()))
}

func TestDelegationSubjectRefMatches_KindAndIDMatch(t *testing.T) {
	subject := DelegationSubjectRef{
		Kind:     "service_account",
		ID:       "operator-1",
		TenantID: "tenant-1",
	}
	actor := EventActor{
		Kind:     "service_account",
		ID:       "operator-1",
		TenantID: "tenant-1",
	}
	require.True(t, subject.Matches(actor))
}

func TestDelegationSubjectRefMatches_KindMismatch(t *testing.T) {
	subject := DelegationSubjectRef{
		Kind:     "service_account",
		ID:       "operator-1",
		TenantID: "tenant-1",
	}
	actor := EventActor{
		Kind:     "user",
		ID:       "operator-1",
		TenantID: "tenant-1",
	}
	require.False(t, subject.Matches(actor))
}

func TestDelegationSubjectRefMatches_IDMismatch(t *testing.T) {
	subject := DelegationSubjectRef{
		Kind:     "service_account",
		ID:       "operator-1",
		TenantID: "tenant-1",
	}
	actor := EventActor{
		Kind:     "service_account",
		ID:       "operator-2",
		TenantID: "tenant-1",
	}
	require.False(t, subject.Matches(actor))
}

func TestDelegationSubjectRefMatches_TenantMismatch(t *testing.T) {
	subject := DelegationSubjectRef{
		Kind:     "service_account",
		ID:       "operator-1",
		TenantID: "tenant-1",
	}
	actor := EventActor{
		Kind:     "service_account",
		ID:       "operator-1",
		TenantID: "tenant-2",
	}
	require.False(t, subject.Matches(actor))
}

func TestDelegationSubjectRefMatches_EmptyTenant(t *testing.T) {
	subject := DelegationSubjectRef{
		Kind:     "service_account",
		ID:       "operator-1",
		TenantID: "",
	}
	actor := EventActor{
		Kind:     "service_account",
		ID:       "operator-1",
		TenantID: "any-tenant",
	}
	require.True(t, subject.Matches(actor))
}

func TestDelegationSubjectRefMatches_CaseInsensitiveKind(t *testing.T) {
	subject := DelegationSubjectRef{
		Kind:     "Service_Account",
		ID:       "operator-1",
		TenantID: "tenant-1",
	}
	actor := EventActor{
		Kind:     "service_account",
		ID:       "operator-1",
		TenantID: "tenant-1",
	}
	require.True(t, subject.Matches(actor))
}

func TestDelegationSubjectRefMatches_UsesSubjectKindFallback(t *testing.T) {
	subject := DelegationSubjectRef{
		Kind:     "service_account",
		ID:       "operator-1",
		TenantID: "tenant-1",
	}
	actor := EventActor{
		SubjectKind: "service_account",
		ID:          "operator-1",
		TenantID:    "tenant-1",
	}
	require.True(t, subject.Matches(actor))
}
