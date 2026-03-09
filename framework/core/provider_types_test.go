package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProviderDescriptorValidate(t *testing.T) {
	desc := ProviderDescriptor{
		ID:                 "remote-mcp",
		Kind:               ProviderKindMCPClient,
		TrustBaseline:      TrustClassRemoteDeclared,
		RecoverabilityMode: RecoverabilityInProcess,
		Security: ProviderSecurityProfile{
			Origin:                     ProviderOriginRemote,
			HoldsCredentials:           true,
			CredentialDomains:          []string{"github.com"},
			RequiresFrameworkMediation: true,
		},
	}

	require.NoError(t, desc.Validate())
}

func TestProviderSecurityProfileValidateRequiresCredentialFlag(t *testing.T) {
	err := (ProviderSecurityProfile{
		CredentialDomains: []string{"db-prod"},
	}).Validate()

	require.Error(t, err)
	require.Contains(t, err.Error(), "holds_credentials")
}

func TestNormalizeProviderCapabilityAppliesRemoteProviderDefaults(t *testing.T) {
	provider := ProviderDescriptor{
		ID:            "remote-mcp",
		Kind:          ProviderKindMCPClient,
		TrustBaseline: TrustClassRemoteDeclared,
		Security: ProviderSecurityProfile{
			Origin:                     ProviderOriginRemote,
			RequiresFrameworkMediation: true,
		},
	}

	desc, err := NormalizeProviderCapability(CapabilityDescriptor{
		ID:   "tool:remote.echo",
		Kind: CapabilityKindTool,
		Name: "remote.echo",
		Source: CapabilitySource{
			Scope: CapabilityScopeWorkspace,
		},
		TrustClass: TrustClassWorkspaceTrusted,
	}, provider, ProviderPolicy{})

	require.NoError(t, err)
	require.Equal(t, CapabilityScopeRemote, desc.Source.Scope)
	require.Equal(t, "remote-mcp", desc.Source.ProviderID)
	require.Equal(t, TrustClassRemoteDeclared, desc.TrustClass)
	require.Empty(t, desc.RiskClasses)
	require.True(t, desc.Annotations["remote_metadata_advisory"].(bool))
	require.True(t, desc.Annotations["requires_insertion_policy"].(bool))
}

func TestNormalizeProviderCapabilityUsesPolicyDefaultTrustWithoutAllowingSelfElevation(t *testing.T) {
	provider := ProviderDescriptor{
		ID:            "remote-mcp",
		Kind:          ProviderKindMCPClient,
		TrustBaseline: TrustClassRemoteDeclared,
		Security: ProviderSecurityProfile{
			Origin:                     ProviderOriginRemote,
			RequiresFrameworkMediation: true,
		},
	}

	desc, err := NormalizeProviderCapability(CapabilityDescriptor{
		ID:         "prompt:remote.summary",
		Kind:       CapabilityKindPrompt,
		Name:       "remote.summary",
		TrustClass: TrustClassRemoteDeclared,
	}, provider, ProviderPolicy{DefaultTrust: TrustClassRemoteApproved})

	require.NoError(t, err)
	require.Equal(t, TrustClassRemoteDeclared, desc.TrustClass)

	desc, err = NormalizeProviderCapability(CapabilityDescriptor{
		ID:            "prompt:remote.summary.approved",
		Kind:          CapabilityKindPrompt,
		Name:          "remote.summary.approved",
		EffectClasses: []EffectClass{EffectClassContextInsertion},
	}, provider, ProviderPolicy{DefaultTrust: TrustClassRemoteApproved})

	require.NoError(t, err)
	require.Equal(t, TrustClassRemoteApproved, desc.TrustClass)
	require.Empty(t, desc.EffectClasses)
}

func TestNormalizeProviderCapabilityRejectsProviderMismatch(t *testing.T) {
	provider := ProviderDescriptor{
		ID:   "browser",
		Kind: ProviderKindAgentRuntime,
		Security: ProviderSecurityProfile{
			Origin:                     ProviderOriginLocal,
			RequiresFrameworkMediation: true,
		},
	}

	_, err := NormalizeProviderCapability(CapabilityDescriptor{
		ID:   "tool:browser",
		Kind: CapabilityKindTool,
		Name: "browser",
		Source: CapabilitySource{
			ProviderID: "other-provider",
		},
	}, provider, ProviderPolicy{})

	require.Error(t, err)
	require.Contains(t, err.Error(), "does not match provider")
}
