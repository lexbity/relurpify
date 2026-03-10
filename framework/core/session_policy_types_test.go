package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateSessionPolicyAcceptsValidPolicy(t *testing.T) {
	ownerOnly := true
	err := ValidateSessionPolicy(SessionPolicy{
		ID:      "owner-send",
		Name:    "Owner Send",
		Enabled: true,
		Selector: SessionSelector{
			Operations:       []SessionOperation{SessionOperationSend, SessionOperationInvoke},
			Scopes:           []SessionScope{SessionScopePerChannelPeer},
			RequireOwnership: &ownerOnly,
		},
		Effect:      AgentPermissionAllow,
		Approvers:   []string{"ops"},
		ApprovalTTL: "30m",
	})
	require.NoError(t, err)
}

func TestValidateSessionPolicyRejectsEmptySelector(t *testing.T) {
	err := ValidateSessionPolicy(SessionPolicy{
		ID:      "bad",
		Name:    "Bad",
		Enabled: true,
		Effect:  AgentPermissionAllow,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "selector invalid")
}

func TestValidateSessionSelectorRejectsInvalidOperation(t *testing.T) {
	err := ValidateSessionSelector(SessionSelector{
		Operations: []SessionOperation{"teleport"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "operation")
}
