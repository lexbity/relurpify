package provider

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentspec "codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/governance/classification"
	"codeburg.org/lexbit/relurpify/governance/policy"
)

func validProviderDescriptor() ProviderDescriptor {
	return ProviderDescriptor{
		ID:       "prov-1",
		Kind:     agentspec.ProviderKindBuiltin,
		Security: ProviderSecurityProfile{Origin: agentspec.ProviderOriginLocal},
	}
}

func TestProviderDescriptorValidate(t *testing.T) {
	t.Run("valid minimal descriptor", func(t *testing.T) {
		assert.NoError(t, validProviderDescriptor().Validate())
	})

	t.Run("id required", func(t *testing.T) {
		desc := validProviderDescriptor()
		desc.ID = "   "
		err := desc.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "provider id required")
	})

	t.Run("kind vocabulary", func(t *testing.T) {
		for _, kind := range []agentspec.ProviderKind{
			agentspec.ProviderKindBuiltin,
			agentspec.ProviderKindPlugin,
			agentspec.ProviderKindAgentRuntime,
			agentspec.ProviderKindLSP,
			agentspec.ProviderKindNodeDevice,
		} {
			desc := validProviderDescriptor()
			desc.Kind = kind
			assert.NoError(t, desc.Validate(), "kind %s must be valid", kind)
		}

		desc := validProviderDescriptor()
		desc.Kind = agentspec.ProviderKind("warp-drive")
		err := desc.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "provider kind warp-drive invalid")
	})

	t.Run("trust baseline vocabulary", func(t *testing.T) {
		for _, trust := range []agentspec.TrustClass{
			"",
			agentspec.TrustClassBuiltinTrusted,
			agentspec.TrustClassWorkspaceTrusted,
			agentspec.TrustClassLLMGenerated,
			agentspec.TrustClassToolResult,
			agentspec.TrustClassProviderLocalUntrusted,
			agentspec.TrustClassRemoteDeclared,
			agentspec.TrustClassRemoteApproved,
		} {
			desc := validProviderDescriptor()
			desc.TrustBaseline = trust
			assert.NoError(t, desc.Validate(), "trust baseline %q must be valid", trust)
		}

		desc := validProviderDescriptor()
		desc.TrustBaseline = agentspec.TrustClass("totally-trusted")
		err := desc.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "trust baseline totally-trusted invalid")
	})

	t.Run("recoverability vocabulary", func(t *testing.T) {
		for _, mode := range []policy.RecoverabilityMode{
			"",
			policy.RecoverabilityEphemeral,
			policy.RecoverabilityInProcess,
			policy.RecoverabilityPersistedRestore,
		} {
			desc := validProviderDescriptor()
			desc.RecoverabilityMode = mode
			assert.NoError(t, desc.Validate(), "recoverability %q must be valid", mode)
		}

		desc := validProviderDescriptor()
		desc.RecoverabilityMode = policy.RecoverabilityMode("sometimes")
		err := desc.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "recoverability mode sometimes invalid")
	})

	t.Run("delegates to security profile", func(t *testing.T) {
		desc := validProviderDescriptor()
		desc.Security = ProviderSecurityProfile{Origin: agentspec.ProviderOriginKind("outer-space")}
		err := desc.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "provider security invalid")
	})
}

func TestProviderConfigValidate(t *testing.T) {
	valid := func() ProviderConfig {
		return ProviderConfig{ID: "prov-1", Kind: agentspec.ProviderKindPlugin}
	}

	t.Run("valid minimal config", func(t *testing.T) {
		assert.NoError(t, valid().Validate())
	})

	t.Run("id required", func(t *testing.T) {
		cfg := valid()
		cfg.ID = ""
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "provider id required")
	})

	t.Run("kind required from vocabulary", func(t *testing.T) {
		cfg := valid()
		cfg.Kind = agentspec.ProviderKind("not-a-kind")
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "provider kind not-a-kind invalid")
	})

	t.Run("trust baseline and recoverability validated", func(t *testing.T) {
		cfg := valid()
		cfg.TrustBaseline = agentspec.TrustClass("over-trusted")
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "trust baseline over-trusted invalid")

		cfg = valid()
		cfg.Recoverability = policy.RecoverabilityMode("unrecoverable")
		err = cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "recoverability mode unrecoverable invalid")
	})
}

func TestProviderSnapshotValidate(t *testing.T) {
	valid := func() ProviderSnapshot {
		return ProviderSnapshot{
			ProviderID: "prov-1",
			Descriptor: validProviderDescriptor(),
		}
	}

	t.Run("valid snapshot", func(t *testing.T) {
		assert.NoError(t, valid().Validate())
	})

	t.Run("provider id required", func(t *testing.T) {
		snapshot := valid()
		snapshot.ProviderID = "  "
		err := snapshot.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "provider id required")
	})

	t.Run("descriptor must be valid", func(t *testing.T) {
		snapshot := valid()
		snapshot.Descriptor.Kind = agentspec.ProviderKind("bogus")
		err := snapshot.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "descriptor invalid")
	})

	t.Run("descriptor id must match snapshot id", func(t *testing.T) {
		snapshot := valid()
		snapshot.Descriptor.ID = "prov-other"
		err := snapshot.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not match snapshot provider id prov-1")
	})

	t.Run("recoverability validated", func(t *testing.T) {
		snapshot := valid()
		snapshot.Recoverability = policy.RecoverabilityMode("maybe")
		err := snapshot.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "recoverability mode maybe invalid")
	})
}

func TestProviderSessionSnapshotValidate(t *testing.T) {
	valid := func() ProviderSessionSnapshot {
		return ProviderSessionSnapshot{
			Session: ProviderSession{ID: "sess-1", ProviderID: "prov-1"},
		}
	}

	t.Run("valid snapshot", func(t *testing.T) {
		assert.NoError(t, valid().Validate())
	})

	t.Run("session id required", func(t *testing.T) {
		snapshot := valid()
		snapshot.Session.ID = ""
		err := snapshot.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "session id required")
	})

	t.Run("provider id required", func(t *testing.T) {
		snapshot := valid()
		snapshot.Session.ProviderID = " "
		err := snapshot.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "provider id required")
	})

	t.Run("recoverability validated", func(t *testing.T) {
		snapshot := valid()
		snapshot.Session.Recoverability = policy.RecoverabilityMode("resurrect")
		err := snapshot.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "recoverability mode resurrect invalid")
	})
}

func TestProviderSecurityProfileValidate(t *testing.T) {
	t.Run("empty profile is valid", func(t *testing.T) {
		assert.NoError(t, ProviderSecurityProfile{}.Validate())
	})

	t.Run("origin vocabulary", func(t *testing.T) {
		assert.NoError(t, ProviderSecurityProfile{Origin: agentspec.ProviderOriginLocal}.Validate())
		assert.NoError(t, ProviderSecurityProfile{Origin: agentspec.ProviderOriginRemote}.Validate())

		err := ProviderSecurityProfile{Origin: agentspec.ProviderOriginKind("cloud")}.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "origin cloud invalid")
	})

	t.Run("credential domains require holds_credentials", func(t *testing.T) {
		err := ProviderSecurityProfile{
			Origin:            agentspec.ProviderOriginLocal,
			CredentialDomains: []string{"example.com"},
		}.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "credential_domains requires holds_credentials=true")
	})

	t.Run("credential domains with empty entry rejected", func(t *testing.T) {
		err := ProviderSecurityProfile{
			Origin:            agentspec.ProviderOriginLocal,
			HoldsCredentials:  true,
			CredentialDomains: []string{"example.com", "  "},
		}.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "credential_domains contains empty value")
	})

	t.Run("credential domains with holds_credentials accepted", func(t *testing.T) {
		assert.NoError(t, ProviderSecurityProfile{
			Origin:            agentspec.ProviderOriginLocal,
			HoldsCredentials:  true,
			CredentialDomains: []string{"example.com"},
		}.Validate())
	})

	t.Run("holds_credentials without domains accepted", func(t *testing.T) {
		assert.NoError(t, ProviderSecurityProfile{
			Origin:           agentspec.ProviderOriginLocal,
			HoldsCredentials: true,
		}.Validate())
	})
}

func TestNormalizeProviderCapabilityRejectsInvalidInputs(t *testing.T) {
	desc := descriptor.CapabilityDescriptor{ID: "cap-1"}

	t.Run("invalid provider", func(t *testing.T) {
		provider := validProviderDescriptor()
		provider.ID = ""
		_, err := NormalizeProviderCapability(desc, provider, agentspec.ProviderPolicy{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "provider invalid")
	})

	t.Run("invalid policy", func(t *testing.T) {
		pol := agentspec.ProviderPolicy{Activate: agentspec.AgentPermissionLevel("maybe")}
		_, err := NormalizeProviderCapability(desc, validProviderDescriptor(), pol)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "provider policy invalid")
	})

	t.Run("capability id required", func(t *testing.T) {
		_, err := NormalizeProviderCapability(descriptor.CapabilityDescriptor{ID: "   "}, validProviderDescriptor(), agentspec.ProviderPolicy{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "capability id required")
	})

	t.Run("provider mismatch rejected", func(t *testing.T) {
		desc := descriptor.CapabilityDescriptor{ID: "cap-1", Source: descriptor.CapabilitySource{ProviderID: "other"}}
		_, err := NormalizeProviderCapability(desc, validProviderDescriptor(), agentspec.ProviderPolicy{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not match provider prov-1")
	})
}

func TestNormalizeProviderCapabilityAssignsProviderAndRuntimeFamily(t *testing.T) {
	// The capability id is gated on being non-empty after trimming, but the
	// id itself is not trimmed by provider admission.
	desc := descriptor.CapabilityDescriptor{ID: "  cap-1  "}

	normalized, err := NormalizeProviderCapability(desc, validProviderDescriptor(), agentspec.ProviderPolicy{})
	require.NoError(t, err)
	assert.Equal(t, "  cap-1  ", normalized.ID)
	assert.Equal(t, "prov-1", normalized.Source.ProviderID)
	assert.Equal(t, agentspec.CapabilityRuntimeFamilyProvider, normalized.RuntimeFamily)

	// A pre-set matching provider id survives.
	desc.ID = "cap-1"
	desc.Source.ProviderID = "prov-1"
	normalized, err = NormalizeProviderCapability(desc, validProviderDescriptor(), agentspec.ProviderPolicy{})
	require.NoError(t, err)
	assert.Equal(t, "prov-1", normalized.Source.ProviderID)
}

func TestNormalizeProviderCapabilityScopePerProviderOrigin(t *testing.T) {
	cases := []struct {
		name       string
		origin     agentspec.ProviderOriginKind
		inputScope classification.CapabilityScope
		want       classification.CapabilityScope
	}{
		{"remote origin forces remote scope", agentspec.ProviderOriginRemote, classification.CapabilityScopeBuiltin, classification.CapabilityScopeRemote},
		{"local origin keeps remote scope", agentspec.ProviderOriginLocal, classification.CapabilityScopeRemote, classification.CapabilityScopeRemote},
		{"local origin defaults others to provider", agentspec.ProviderOriginLocal, classification.CapabilityScopeWorkspace, classification.CapabilityScopeProvider},
		{"unset origin keeps declared scope", "", classification.CapabilityScopeWorkspace, classification.CapabilityScopeWorkspace},
		// The empty-scope input is defaulted to builtin by descriptor
		// normalization before the provider scope logic sees it, so the
		// default-to-provider fallback only applies to origin kinds the
		// descriptor normalization has not touched.
		{"unset origin keeps builtin default", "", "", classification.CapabilityScopeBuiltin},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider := validProviderDescriptor()
			provider.Security.Origin = tc.origin
			desc := descriptor.CapabilityDescriptor{ID: "cap-1", Source: descriptor.CapabilitySource{Scope: tc.inputScope}}

			normalized, err := NormalizeProviderCapability(desc, provider, agentspec.ProviderPolicy{})
			require.NoError(t, err)
			assert.Equal(t, tc.want, normalized.Source.Scope)
		})
	}
}

func TestNormalizeProviderCapabilityTrustBaselinePrecedence(t *testing.T) {
	cases := []struct {
		name          string
		providerTrust agentspec.TrustClass
		policyTrust   agentspec.TrustClass
		scope         classification.CapabilityScope
		want          agentspec.TrustClass
	}{
		{"policy default trust wins", agentspec.TrustClassProviderLocalUntrusted, agentspec.TrustClassBuiltinTrusted, classification.CapabilityScopeRemote, agentspec.TrustClassBuiltinTrusted},
		{"provider baseline next", agentspec.TrustClassWorkspaceTrusted, "", classification.CapabilityScopeRemote, agentspec.TrustClassWorkspaceTrusted},
		{"remote scope falls back to remote declared", "", "", classification.CapabilityScopeRemote, agentspec.TrustClassRemoteDeclared},
		{"workspace scope falls back to workspace trusted", "", "", classification.CapabilityScopeWorkspace, agentspec.TrustClassWorkspaceTrusted},
		{"builtin scope falls back to builtin trusted", "", "", classification.CapabilityScopeBuiltin, agentspec.TrustClassBuiltinTrusted},
		{"unknown scope falls back to provider local untrusted", "", "", classification.CapabilityScopeProvider, agentspec.TrustClassProviderLocalUntrusted},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider := validProviderDescriptor()
			provider.TrustBaseline = tc.providerTrust
			provider.Security.Origin = "" // keep scope normalization off the baseline path
			desc := descriptor.CapabilityDescriptor{ID: "cap-1", Source: descriptor.CapabilitySource{Scope: tc.scope}}

			normalized, err := NormalizeProviderCapability(desc, provider, agentspec.ProviderPolicy{DefaultTrust: tc.policyTrust})
			require.NoError(t, err)
			assert.Equal(t, tc.want, normalized.TrustClass)
		})
	}
}

func TestNormalizeProviderCapabilityMergesDeclaredTrustWithBaseline(t *testing.T) {
	cases := []struct {
		name     string
		declared agentspec.TrustClass
		baseline agentspec.TrustClass
		want     agentspec.TrustClass
	}{
		{"declared weaker than baseline yields baseline", agentspec.TrustClassBuiltinTrusted, agentspec.TrustClassRemoteDeclared, agentspec.TrustClassRemoteDeclared},
		{"declared stronger than baseline kept", agentspec.TrustClassRemoteDeclared, agentspec.TrustClassBuiltinTrusted, agentspec.TrustClassRemoteDeclared},
		{"equal declared kept", agentspec.TrustClassLLMGenerated, agentspec.TrustClassToolResult, agentspec.TrustClassLLMGenerated},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider := validProviderDescriptor()
			provider.TrustBaseline = tc.baseline
			desc := descriptor.CapabilityDescriptor{ID: "cap-1", TrustClass: tc.declared}

			normalized, err := NormalizeProviderCapability(desc, provider, agentspec.ProviderPolicy{})
			require.NoError(t, err)
			assert.Equal(t, tc.want, normalized.TrustClass)
		})
	}
}

func TestNormalizeProviderCapabilityRemoteOriginAnnotations(t *testing.T) {
	newRemoteProvider := func() ProviderDescriptor {
		provider := validProviderDescriptor()
		provider.Security.Origin = agentspec.ProviderOriginRemote
		return provider
	}

	t.Run("tool capabilities keep effect classes", func(t *testing.T) {
		desc := descriptor.CapabilityDescriptor{
			ID:            "cap-1",
			Kind:          agentspec.CapabilityKindTool,
			EffectClasses: []classification.EffectClass{classification.EffectClassNetworkEgress},
		}

		normalized, err := NormalizeProviderCapability(desc, newRemoteProvider(), agentspec.ProviderPolicy{})
		require.NoError(t, err)
		assert.Equal(t, []classification.EffectClass{classification.EffectClassNetworkEgress}, normalized.EffectClasses)
		assert.Equal(t, classification.CapabilityScopeRemote, normalized.Source.Scope)
	})

	t.Run("non-tool capabilities drop effect classes", func(t *testing.T) {
		desc := descriptor.CapabilityDescriptor{
			ID:            "cap-2",
			Kind:          agentspec.CapabilityKindPrompt,
			EffectClasses: []classification.EffectClass{classification.EffectClassNetworkEgress},
		}

		normalized, err := NormalizeProviderCapability(desc, newRemoteProvider(), agentspec.ProviderPolicy{})
		require.NoError(t, err)
		assert.Nil(t, normalized.EffectClasses)
	})

	t.Run("annotations and default description applied", func(t *testing.T) {
		normalized, err := NormalizeProviderCapability(
			descriptor.CapabilityDescriptor{ID: "cap-3", Kind: agentspec.CapabilityKindResource},
			newRemoteProvider(), agentspec.ProviderPolicy{},
		)
		require.NoError(t, err)
		require.NotNil(t, normalized.Annotations)
		assert.Equal(t, true, normalized.Annotations["remote_metadata_advisory"])
		assert.Equal(t, true, normalized.Annotations["requires_insertion_policy"])
		assert.Equal(t, "prov-1", normalized.Annotations["admitted_by_provider"])
		assert.Equal(t, "remote resource capability admitted via provider prov-1", normalized.Description)
	})

	t.Run("existing description preserved", func(t *testing.T) {
		normalized, err := NormalizeProviderCapability(
			descriptor.CapabilityDescriptor{ID: "cap-3", Kind: agentspec.CapabilityKindResource, Description: "mine"},
			newRemoteProvider(), agentspec.ProviderPolicy{},
		)
		require.NoError(t, err)
		assert.Equal(t, "mine", normalized.Description)
	})

	t.Run("local origin adds no annotations", func(t *testing.T) {
		normalized, err := NormalizeProviderCapability(
			descriptor.CapabilityDescriptor{ID: "cap-4"},
			validProviderDescriptor(), agentspec.ProviderPolicy{},
		)
		require.NoError(t, err)
		assert.Nil(t, normalized.Annotations)
		assert.Empty(t, normalized.Description)
	})
}

// Compile-time contract checks for the provider-facing interfaces in this
// package, mirroring the fixture pattern from capability/descriptor.
type fakeProvider struct{}

func (fakeProvider) Descriptor() ProviderDescriptor { return validProviderDescriptor() }
func (fakeProvider) Initialize(_ context.Context, _ ProviderRuntime) error {
	return nil
}
func (fakeProvider) RegisterCapabilities(_ context.Context, _ CapabilityRegistrar) error {
	return nil
}
func (fakeProvider) ListSessions(_ context.Context) ([]ProviderSession, error) {
	return nil, nil
}
func (fakeProvider) HealthSnapshot(_ context.Context) (ProviderHealthSnapshot, error) {
	return ProviderHealthSnapshot{Status: "healthy"}, nil
}
func (fakeProvider) Close(_ context.Context) error { return nil }

type fakeRegistrar struct {
	registered []descriptor.CapabilityDescriptor
}

func (r *fakeRegistrar) RegisterCapability(_ context.Context, desc descriptor.CapabilityDescriptor) error {
	r.registered = append(r.registered, desc)
	return nil
}

type fakeRuntime struct{ state map[string]any }

func (r fakeRuntime) State() map[string]any { return r.state }

type fakeSnapshotter struct{}

func (fakeSnapshotter) SnapshotProvider(_ context.Context) (*ProviderSnapshot, error) {
	return &ProviderSnapshot{ProviderID: "prov-1", Descriptor: validProviderDescriptor()}, nil
}

type fakeSessionSnapshotter struct{}

func (fakeSessionSnapshotter) SnapshotSessions(_ context.Context) ([]ProviderSessionSnapshot, error) {
	return nil, nil
}

type fakeRestorer struct{ restored int }

func (r *fakeRestorer) RestoreProvider(_ context.Context, _ ProviderSnapshot) error {
	r.restored++
	return nil
}

type fakeSessionRestorer struct{ restored int }

func (r *fakeSessionRestorer) RestoreSession(_ context.Context, _ ProviderSessionSnapshot) error {
	r.restored++
	return nil
}

var (
	_ Provider                   = fakeProvider{}
	_ CapabilityRegistrar        = (*fakeRegistrar)(nil)
	_ ProviderRuntime            = fakeRuntime{}
	_ ProviderSnapshotter        = fakeSnapshotter{}
	_ ProviderSessionSnapshotter = fakeSessionSnapshotter{}
	_ ProviderRestorer           = (*fakeRestorer)(nil)
	_ ProviderSessionRestorer    = (*fakeSessionRestorer)(nil)
)
