package core

import "testing"

func TestProviderSnapshotValidate(t *testing.T) {
	snapshot := ProviderSnapshot{
		ProviderID:     "browser",
		Recoverability: RecoverabilityInProcess,
		Descriptor: ProviderDescriptor{
			ID:                 "browser",
			Kind:               ProviderKindAgentRuntime,
			TrustBaseline:      TrustClassProviderLocalUntrusted,
			RecoverabilityMode: RecoverabilityInProcess,
			Security: ProviderSecurityProfile{
				Origin: ProviderOriginLocal,
			},
		},
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("expected snapshot to validate: %v", err)
	}
}

func TestProviderSessionSnapshotValidate(t *testing.T) {
	snapshot := ProviderSessionSnapshot{
		Session: ProviderSession{
			ID:             "session-1",
			ProviderID:     "browser",
			Recoverability: RecoverabilityInProcess,
		},
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("expected session snapshot to validate: %v", err)
	}
}
