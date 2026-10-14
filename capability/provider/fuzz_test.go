package provider

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentspec "codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/governance/classification"
)

// FuzzNormalizeProviderCapability drives provider admission with arbitrary
// untrusted provider ids, trust classes, scopes, and origins. Invariants: on
// success the admitted descriptor is always provider-owned (provider id set,
// provider runtime family, non-empty trust class), remote providers always
// force remote scope and admission annotations, and non-tool remote
// capabilities never carry effect classes. Invalid inputs must be rejected,
// never panic.
func FuzzNormalizeProviderCapability(f *testing.F) {
	f.Add("prov-1", "cap-1", "builtin-trusted", "", "provider", "local", true)
	f.Add("", "", "", "", "", "remote", false)
	f.Add("prov-2", "  ", "remote-declared-untrusted", "workspace", "remote-declared-untrusted", "bogus-origin", false)
	f.Add("prov-3", "cap-3", "made-up-trust", "made-up-scope", "", "", true)

	f.Fuzz(func(t *testing.T, providerID, descID, providerTrust, scope, policyTrust, origin string, kindIsTool bool) {
		provider := ProviderDescriptor{
			ID:            providerID,
			Kind:          agentspec.ProviderKindBuiltin,
			TrustBaseline: agentspec.TrustClass(providerTrust),
			Security:      ProviderSecurityProfile{Origin: agentspec.ProviderOriginKind(origin)},
		}
		desc := descriptor.CapabilityDescriptor{
			ID:     descID,
			Kind:   agentspec.CapabilityKindTool,
			Source: descriptor.CapabilitySource{Scope: classification.CapabilityScope(scope)},
		}
		if !kindIsTool {
			desc.Kind = agentspec.CapabilityKindPrompt
		}
		pol := agentspec.ProviderPolicy{DefaultTrust: agentspec.TrustClass(policyTrust)}

		normalized, err := NormalizeProviderCapability(desc, provider, pol)
		if err != nil {
			// Rejection is only acceptable when an input is invalid.
			if providerErr := provider.Validate(); providerErr == nil {
				if policyErr := agentspec.ValidateProviderPolicy(pol); policyErr == nil {
					if strings.TrimSpace(descID) != "" {
						t.Fatalf("NormalizeProviderCapability rejected valid inputs: %v", err)
					}
				}
			}
			return
		}

		require.NotEmpty(t, provider.ID)
		assert.Equal(t, provider.ID, normalized.Source.ProviderID)
		assert.Equal(t, agentspec.CapabilityRuntimeFamilyProvider, normalized.RuntimeFamily)
		assert.NotEmpty(t, normalized.TrustClass, "admitted capability must carry a trust class")

		if provider.Security.Origin == agentspec.ProviderOriginRemote {
			assert.Equal(t, classification.CapabilityScopeRemote, normalized.Source.Scope)
			require.NotNil(t, normalized.Annotations)
			assert.Equal(t, true, normalized.Annotations["remote_metadata_advisory"])
			assert.Equal(t, true, normalized.Annotations["requires_insertion_policy"])
			assert.Equal(t, provider.ID, normalized.Annotations["admitted_by_provider"])
			if !kindIsTool {
				assert.Nil(t, normalized.EffectClasses, "remote non-tool capabilities must drop effect classes")
			}
		}
	})
}
