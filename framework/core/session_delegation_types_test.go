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
		Grantee: SubjectRef{
			TenantID: "tenant-1",
			Kind:     SubjectKindServiceAccount,
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
		Grantee: SubjectRef{
			TenantID: "tenant-1",
			Kind:     SubjectKindServiceAccount,
			ID:       "operator-1",
		},
		Operations: []SessionOperation{SessionOperationSend},
		CreatedAt:  time.Now().UTC(),
	}

	require.True(t, record.Allows(EventActor{
		ID:          "operator-1",
		TenantID:    "tenant-1",
		SubjectKind: SubjectKindServiceAccount,
	}, SessionOperationSend, time.Now().UTC()))

	require.False(t, record.Allows(EventActor{
		ID:          "operator-1",
		TenantID:    "tenant-1",
		SubjectKind: SubjectKindServiceAccount,
	}, SessionOperationResume, time.Now().UTC()))
}
