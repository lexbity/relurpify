package authorization

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPolicyEngineNilManagerAlwaysAllows(t *testing.T) {
	engine := &ManifestPolicyEngine{}
	dec, err := engine.Evaluate(context.Background(), core.PolicyRequest{
		CapabilityName: "some.tool",
		TrustClass:     core.TrustClassRemoteApproved,
	})
	require.NoError(t, err)
	assert.Equal(t, "allow", dec.Effect)
}

func TestPolicyEngineBuiltinTrustedAlwaysAllows(t *testing.T) {
	pm := minimalPermissionManager(t)
	engine := &ManifestPolicyEngine{agentID: "test", manager: pm}

	for _, tc := range []core.TrustClass{
		core.TrustClassBuiltinTrusted,
		core.TrustClassWorkspaceTrusted,
	} {
		dec, err := engine.Evaluate(context.Background(), core.PolicyRequest{
			CapabilityName: "file_read",
			TrustClass:     tc,
		})
		require.NoError(t, err)
		assert.Equal(t, "allow", dec.Effect, "trust class %s should always allow", tc)
	}
}

func TestPolicyEngineRemoteCapabilityRespectsDefaultPolicy(t *testing.T) {
	cases := []struct {
		defaultPolicy core.AgentPermissionLevel
		wantEffect    string
	}{
		{core.AgentPermissionAllow, "allow"},
		{core.AgentPermissionDeny, "deny"},
		{core.AgentPermissionAsk, "require_approval"},
	}

	for _, tc := range cases {
		pm := minimalPermissionManager(t)
		pm.SetDefaultPolicy(tc.defaultPolicy)
		engine := &ManifestPolicyEngine{agentID: "test", manager: pm}

		for _, trustClass := range []core.TrustClass{
			core.TrustClassRemoteApproved,
			core.TrustClassRemoteDeclared,
			core.TrustClassProviderLocalUntrusted,
		} {
			dec, err := engine.Evaluate(context.Background(), core.PolicyRequest{
				CapabilityName: "remote.tool",
				TrustClass:     trustClass,
			})
			require.NoError(t, err)
			assert.Equal(t, tc.wantEffect, dec.Effect,
				"default policy %s + trust class %s", tc.defaultPolicy, trustClass)
		}
	}
}

func TestFromManifestWithConfigUsesAgentID(t *testing.T) {
	engine, err := FromManifestWithConfig(nil, "my-agent", nil)
	require.NoError(t, err)
	assert.Equal(t, "my-agent", engine.agentID)
}

func TestPolicyEngineCompiledRuleOverridesDefaultPolicy(t *testing.T) {
	pm := minimalPermissionManager(t)
	pm.SetDefaultPolicy(core.AgentPermissionAllow)
	engine, err := FromManifestWithConfig(&manifest.AgentManifest{
		Metadata: manifest.ManifestMetadata{Name: "test-agent"},
		Spec: manifest.ManifestSpec{
			Agent: &core.AgentRuntimeSpec{
				Mode: core.AgentModePrimary,
				Model: core.AgentModelConfig{
					Provider: "ollama",
					Name:     "test",
				},
				ToolExecutionPolicy: map[string]core.ToolPolicy{
					"file_read": {Execute: core.AgentPermissionDeny},
				},
			},
		},
	}, "test-agent", pm)
	require.NoError(t, err)

	dec, err := engine.Evaluate(context.Background(), core.PolicyRequest{
		CapabilityName: "file_read",
		CapabilityKind: core.CapabilityKindTool,
		RuntimeFamily:  core.CapabilityRuntimeFamilyLocalTool,
		TrustClass:     core.TrustClassWorkspaceTrusted,
	})
	require.NoError(t, err)
	assert.Equal(t, "deny", dec.Effect)
	require.NotNil(t, dec.Rule)
	assert.Equal(t, "tool:file_read", dec.Rule.ID)
}

func TestPolicyEngineCompiledSessionRuleMatches(t *testing.T) {
	pm := minimalPermissionManager(t)
	ownerOnly := true
	engine, err := FromManifestWithConfig(&manifest.AgentManifest{
		Metadata: manifest.ManifestMetadata{Name: "test-agent"},
		Spec: manifest.ManifestSpec{
			Agent: &core.AgentRuntimeSpec{
				Mode: core.AgentModePrimary,
				Model: core.AgentModelConfig{
					Provider: "ollama",
					Name:     "test",
				},
				SessionPolicies: []core.SessionPolicy{{
					ID:      "owner-send",
					Name:    "Owner send",
					Enabled: true,
					Selector: core.SessionSelector{
						Operations:       []core.SessionOperation{core.SessionOperationSend},
						RequireOwnership: &ownerOnly,
					},
					Effect: core.AgentPermissionAllow,
				}},
			},
		},
	}, "test-agent", pm)
	require.NoError(t, err)

	dec, err := engine.Evaluate(context.Background(), core.PolicyRequest{
		Actor:            core.EventActor{Kind: "user", ID: "user-1"},
		SessionOperation: core.SessionOperationSend,
		IsOwner:          true,
		TrustClass:       core.TrustClassRemoteApproved,
	})
	require.NoError(t, err)
	assert.Equal(t, "allow", dec.Effect)
	require.NotNil(t, dec.Rule)
	assert.Equal(t, "owner-send", dec.Rule.ID)
}

func TestPolicyEngineProviderFallbackPreservesRemoteApprovalDefault(t *testing.T) {
	pm := minimalPermissionManager(t)
	engine := &ManifestPolicyEngine{agentID: "test", manager: pm}

	dec, err := engine.Evaluate(context.Background(), core.PolicyRequest{
		Target:         core.PolicyTargetProvider,
		CapabilityID:   "provider:remote-mcp:activate",
		CapabilityName: "provider:remote-mcp:activate",
		ProviderKind:   core.ProviderKindMCPClient,
		ProviderOrigin: core.ProviderOriginRemote,
		TrustClass:     core.TrustClassRemoteDeclared,
	})
	require.NoError(t, err)
	assert.Equal(t, "require_approval", dec.Effect)
}

func TestPolicyEngineProviderFallbackAllowsBuiltinProviders(t *testing.T) {
	pm := minimalPermissionManager(t)
	engine := &ManifestPolicyEngine{agentID: "test", manager: pm}

	dec, err := engine.Evaluate(context.Background(), core.PolicyRequest{
		Target:         core.PolicyTargetProvider,
		CapabilityID:   "provider:builtin:activate",
		CapabilityName: "provider:builtin:activate",
		ProviderKind:   core.ProviderKindBuiltin,
		ProviderOrigin: core.ProviderOriginLocal,
		TrustClass:     core.TrustClassBuiltinTrusted,
	})
	require.NoError(t, err)
	assert.Equal(t, "allow", dec.Effect)
}

func TestPolicyEngineEmitsPolicyEvaluationEvent(t *testing.T) {
	pm := minimalPermissionManager(t)
	var capturedEffect string
	var capturedReason string
	var capturedAction string
	var capturedRuleID any
	pm.SetEventLogger(func(_ context.Context, desc core.PermissionDescriptor, effect, reason string, fields map[string]interface{}) {
		capturedAction = desc.Action
		capturedEffect = effect
		capturedReason = reason
		capturedRuleID = fields["rule_id"]
	})
	engine, err := FromManifestWithConfig(&manifest.AgentManifest{
		Metadata: manifest.ManifestMetadata{Name: "test-agent"},
		Spec: manifest.ManifestSpec{
			Agent: &core.AgentRuntimeSpec{
				Mode: core.AgentModePrimary,
				Model: core.AgentModelConfig{
					Provider: "ollama",
					Name:     "test",
				},
				ToolExecutionPolicy: map[string]core.ToolPolicy{
					"file_read": {Execute: core.AgentPermissionDeny},
				},
			},
		},
	}, "test-agent", pm)
	require.NoError(t, err)

	_, err = engine.Evaluate(context.Background(), core.PolicyRequest{
		Target:         core.PolicyTargetCapability,
		CapabilityName: "file_read",
		CapabilityKind: core.CapabilityKindTool,
		RuntimeFamily:  core.CapabilityRuntimeFamilyLocalTool,
		TrustClass:     core.TrustClassWorkspaceTrusted,
	})
	require.NoError(t, err)
	assert.Equal(t, "file_read", capturedAction)
	assert.Equal(t, "deny", capturedEffect)
	assert.Equal(t, "tool:file_read", capturedRuleID)
	assert.Equal(t, "", capturedReason)
}

// minimalPermissionManager builds a PermissionManager with a minimal valid PermissionSet.
func minimalPermissionManager(t *testing.T) *PermissionManager {
	t.Helper()
	pm, err := NewPermissionManager(t.TempDir(), &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{
			{Action: core.FileSystemRead, Path: "**"},
		},
	}, nil, nil)
	require.NoError(t, err)
	return pm
}
