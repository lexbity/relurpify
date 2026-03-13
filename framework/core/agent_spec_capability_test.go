package core

import "testing"

import "github.com/stretchr/testify/require"

func TestAgentRuntimeSpecValidateCapabilityPolicies(t *testing.T) {
	spec := &AgentRuntimeSpec{
		Mode: AgentModePrimary,
		Model: AgentModelConfig{
			Provider: "ollama",
			Name:     "test",
		},
		CapabilityPolicies: []CapabilityPolicy{
			{
				Selector: CapabilitySelector{
					Kind:        CapabilityKindTool,
					RiskClasses: []RiskClass{RiskClassExecute},
				},
				Execute: AgentPermissionAsk,
			},
		},
		ExposurePolicies: []CapabilityExposurePolicy{
			{
				Selector: CapabilitySelector{
					TrustClasses: []TrustClass{TrustClassRemoteDeclared},
				},
				Access: CapabilityExposureInspectable,
			},
		},
		InsertionPolicies: []CapabilityInsertionPolicy{
			{
				Selector: CapabilitySelector{
					TrustClasses: []TrustClass{TrustClassRemoteDeclared},
				},
				Action: InsertionActionMetadataOnly,
			},
		},
		GlobalPolicies: map[string]AgentPermissionLevel{
			string(RiskClassNetwork): AgentPermissionDeny,
		},
		ProviderPolicies: map[string]ProviderPolicy{
			"remote-mcp": {
				Activate:     AgentPermissionAsk,
				DefaultTrust: TrustClassRemoteDeclared,
			},
		},
		Providers: []ProviderConfig{
			{
				ID:             "remote-mcp",
				Kind:           ProviderKindMCPClient,
				Enabled:        true,
				Target:         "https://mcp.example.test",
				Recoverability: RecoverabilityPersistedRestore,
			},
		},
	}

	require.NoError(t, spec.Validate())
}

func TestValidateCapabilityExposurePolicyRejectsUnknownAccess(t *testing.T) {
	err := ValidateCapabilityExposurePolicy(CapabilityExposurePolicy{
		Selector: CapabilitySelector{Kind: CapabilityKindTool},
		Access:   CapabilityExposure("opaque"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid")
}

func TestValidateCapabilitySelectorRejectsEmptySelector(t *testing.T) {
	err := ValidateCapabilitySelector(CapabilitySelector{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least one")
}

func TestValidateCapabilitySelectorAcceptsTagMatching(t *testing.T) {
	err := ValidateCapabilitySelector(CapabilitySelector{
		Tags:        []string{"lang:go"},
		ExcludeTags: []string{"verification"},
	})
	require.NoError(t, err)
}

func TestValidateCapabilitySelectorAcceptsRuntimeFamilyMatching(t *testing.T) {
	err := ValidateCapabilitySelector(CapabilitySelector{
		RuntimeFamilies: []CapabilityRuntimeFamily{CapabilityRuntimeFamilyRelurpic},
	})
	require.NoError(t, err)
}

func TestValidateCapabilitySelectorAcceptsCoordinationFields(t *testing.T) {
	longRunning := true
	err := ValidateCapabilitySelector(CapabilitySelector{
		CoordinationRoles:          []CoordinationRole{CoordinationRoleReviewer},
		CoordinationTaskTypes:      []string{"review"},
		CoordinationExecutionModes: []CoordinationExecutionMode{CoordinationExecutionModeBackgroundAgent},
		CoordinationLongRunning:    &longRunning,
	})
	require.NoError(t, err)
}

func TestValidateSkillCapabilitySelectorAcceptsCapabilityField(t *testing.T) {
	err := ValidateSkillCapabilitySelector(SkillCapabilitySelector{Capability: "file_read"})
	require.NoError(t, err)
}

func TestValidateSkillCapabilitySelectorAcceptsRuntimeFamilyField(t *testing.T) {
	err := ValidateSkillCapabilitySelector(SkillCapabilitySelector{
		RuntimeFamilies: []CapabilityRuntimeFamily{CapabilityRuntimeFamilyRelurpic},
	})
	require.NoError(t, err)
}

func TestEffectiveAllowedCapabilitySelectorsClonesSelectors(t *testing.T) {
	longRunning := true
	directInsertion := false
	spec := &AgentRuntimeSpec{
		AllowedCapabilities: []CapabilitySelector{{
			RuntimeFamilies:             []CapabilityRuntimeFamily{CapabilityRuntimeFamilyProvider},
			Tags:                        []string{"lang:go"},
			CoordinationRoles:           []CoordinationRole{CoordinationRolePlanner},
			CoordinationTaskTypes:       []string{"plan"},
			CoordinationExecutionModes:  []CoordinationExecutionMode{CoordinationExecutionModeSessionBacked},
			CoordinationLongRunning:     &longRunning,
			CoordinationDirectInsertion: &directInsertion,
		}},
	}

	selectors := EffectiveAllowedCapabilitySelectors(spec)

	require.Len(t, selectors, 1)
	require.Equal(t, []CapabilityRuntimeFamily{CapabilityRuntimeFamilyProvider}, selectors[0].RuntimeFamilies)
	require.Equal(t, []string{"lang:go"}, selectors[0].Tags)
	require.Equal(t, []CoordinationRole{CoordinationRolePlanner}, selectors[0].CoordinationRoles)
	require.Equal(t, []string{"plan"}, selectors[0].CoordinationTaskTypes)
	selectors[0].RuntimeFamilies[0] = CapabilityRuntimeFamilyRelurpic
	selectors[0].Tags[0] = "mutated"
	selectors[0].CoordinationRoles[0] = CoordinationRoleReviewer
	selectors[0].CoordinationTaskTypes[0] = "review"
	*selectors[0].CoordinationLongRunning = false
	require.Equal(t, []CapabilityRuntimeFamily{CapabilityRuntimeFamilyProvider}, spec.AllowedCapabilities[0].RuntimeFamilies)
	require.Equal(t, []string{"lang:go"}, spec.AllowedCapabilities[0].Tags)
	require.Equal(t, []CoordinationRole{CoordinationRolePlanner}, spec.AllowedCapabilities[0].CoordinationRoles)
	require.Equal(t, []string{"plan"}, spec.AllowedCapabilities[0].CoordinationTaskTypes)
	require.True(t, *spec.AllowedCapabilities[0].CoordinationLongRunning)
}

func TestMergeAgentSpecsPreservesNilCapabilitySelectorSlices(t *testing.T) {
	spec := &AgentRuntimeSpec{
		Mode: AgentModePrimary,
		Model: AgentModelConfig{
			Provider: "ollama",
			Name:     "test",
		},
		CapabilityPolicies: []CapabilityPolicy{{
			Selector: CapabilitySelector{
				Kind:        CapabilityKindTool,
				RiskClasses: []RiskClass{RiskClassDestructive},
			},
			Execute: AgentPermissionAsk,
		}},
	}

	merged := MergeAgentSpecs(spec, AgentSpecOverlay{})

	require.Len(t, merged.CapabilityPolicies, 1)
	require.Nil(t, merged.CapabilityPolicies[0].Selector.Tags)
	require.Nil(t, merged.CapabilityPolicies[0].Selector.ExcludeTags)
	require.Nil(t, merged.CapabilityPolicies[0].Selector.SourceScopes)
	require.Nil(t, merged.CapabilityPolicies[0].Selector.CoordinationRoles)
	require.Nil(t, merged.CapabilityPolicies[0].Selector.CoordinationTaskTypes)
	require.Nil(t, merged.CapabilityPolicies[0].Selector.CoordinationExecutionModes)
	require.Equal(t, []RiskClass{RiskClassDestructive}, merged.CapabilityPolicies[0].Selector.RiskClasses)
}

func TestValidatePolicyClassKeyAcceptsCapabilityClasses(t *testing.T) {
	require.NoError(t, ValidatePolicyClassKey(string(RiskClassExecute)))
	require.NoError(t, ValidatePolicyClassKey(string(EffectClassNetworkEgress)))
	require.NoError(t, ValidatePolicyClassKey(string(TrustClassRemoteDeclared)))
	require.NoError(t, ValidatePolicyClassKey(string(CapabilityRuntimeFamilyRelurpic)))
}

func TestValidatePolicyClassKeyRejectsUnknownClass(t *testing.T) {
	err := ValidatePolicyClassKey("totally-custom")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown capability class")
}

func TestAgentRuntimeSpecValidateAcceptsCoordinationConfig(t *testing.T) {
	spec := &AgentRuntimeSpec{
		Mode: AgentModePrimary,
		Model: AgentModelConfig{
			Provider: "ollama",
			Name:     "test",
		},
		Coordination: AgentCoordinationSpec{
			Enabled: true,
			DelegationTargetSelectors: []CapabilitySelector{
				{
					CoordinationRoles:          []CoordinationRole{CoordinationRoleReviewer},
					CoordinationTaskTypes:      []string{"review"},
					CoordinationExecutionModes: []CoordinationExecutionMode{CoordinationExecutionModeBackgroundAgent},
				},
			},
			MaxDelegationDepth:        3,
			AllowBackgroundDelegation: true,
			RequireApprovalCrossTrust: true,
			Projection: AgentProjectionPolicy{
				Strategy: "balanced",
				Hot: AgentProjectionTier{
					MaxItems:       8,
					MaxTokens:      1024,
					ResourceScopes: []string{"workflow.current"},
				},
				Cold: AgentProjectionTier{
					Persist:        true,
					ResourceScopes: []string{"workflow.archive"},
				},
			},
			ScaleOut: AgentScaleOutPolicy{
				Mode:                "prefer-local",
				PreferredModelClass: "reasoning",
				PreferredProviders:  []string{"local-runtime"},
				Metadata:            map[string]string{"placement": "sticky"},
			},
		},
	}

	require.NoError(t, spec.Validate())
}

func TestEffectiveCoordinationIncludesLegacyInvocationCompatibility(t *testing.T) {
	spec := &AgentRuntimeSpec{
		Invocation: AgentInvocationSpec{
			CanInvokeSubagents: true,
			AllowedSubagents:   []string{"planner", "reviewer"},
			MaxDepth:           2,
		},
	}

	effective := EffectiveCoordination(spec)

	require.True(t, effective.Enabled)
	require.Equal(t, 2, effective.MaxDelegationDepth)
	require.Len(t, effective.DelegationTargetSelectors, 2)
	require.Equal(t, "planner", effective.DelegationTargetSelectors[0].Name)
	require.NotEmpty(t, effective.DelegationTargetSelectors[0].CoordinationRoles)
}

func TestAgentRuntimeSpecValidateRejectsInvalidProjectionTier(t *testing.T) {
	spec := &AgentRuntimeSpec{
		Mode: AgentModePrimary,
		Model: AgentModelConfig{
			Provider: "ollama",
			Name:     "test",
		},
		Coordination: AgentCoordinationSpec{
			Projection: AgentProjectionPolicy{
				Hot: AgentProjectionTier{
					MaxTokens: -1,
				},
			},
		},
	}

	err := spec.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "hot.max_tokens")
}

func TestMergeAgentSpecsPreservesInsertionPolicies(t *testing.T) {
	base := &AgentRuntimeSpec{
		InsertionPolicies: []CapabilityInsertionPolicy{
			{
				Selector: CapabilitySelector{Name: "echo"},
				Action:   InsertionActionSummarized,
			},
		},
	}
	overlay := AgentSpecOverlay{
		InsertionPolicies: []CapabilityInsertionPolicy{
			{
				Selector: CapabilitySelector{TrustClasses: []TrustClass{TrustClassRemoteDeclared}},
				Action:   InsertionActionMetadataOnly,
			},
		},
	}

	merged := MergeAgentSpecs(base, overlay)

	require.Len(t, merged.InsertionPolicies, 2)
	require.Equal(t, InsertionActionSummarized, merged.InsertionPolicies[0].Action)
	require.Equal(t, InsertionActionMetadataOnly, merged.InsertionPolicies[1].Action)
}

func TestMergeAgentSpecsPreservesSessionPolicies(t *testing.T) {
	base := &AgentRuntimeSpec{
		SessionPolicies: []SessionPolicy{{
			ID:      "owner-send",
			Name:    "Owner Send",
			Enabled: true,
			Selector: SessionSelector{
				Operations: []SessionOperation{SessionOperationSend},
			},
			Effect: AgentPermissionAllow,
		}},
	}
	overlay := AgentSpecOverlay{
		SessionPolicies: []SessionPolicy{{
			ID:      "inspect-ask",
			Name:    "Inspect Ask",
			Enabled: true,
			Selector: SessionSelector{
				Operations: []SessionOperation{SessionOperationInspect},
			},
			Effect: AgentPermissionAsk,
		}},
	}

	merged := MergeAgentSpecs(base, overlay)

	require.Len(t, merged.SessionPolicies, 2)
	require.Equal(t, "owner-send", merged.SessionPolicies[0].ID)
	require.Equal(t, "inspect-ask", merged.SessionPolicies[1].ID)
}

func TestAgentRuntimeSpecValidateRejectsDuplicateSessionPolicyIDs(t *testing.T) {
	spec := &AgentRuntimeSpec{
		Mode: AgentModePrimary,
		Model: AgentModelConfig{
			Provider: "ollama",
			Name:     "test",
		},
		SessionPolicies: []SessionPolicy{
			{
				ID:      "duplicate",
				Name:    "First",
				Enabled: true,
				Selector: SessionSelector{
					Operations: []SessionOperation{SessionOperationSend},
				},
				Effect: AgentPermissionAllow,
			},
			{
				ID:      "duplicate",
				Name:    "Second",
				Enabled: true,
				Selector: SessionSelector{
					Operations: []SessionOperation{SessionOperationInspect},
				},
				Effect: AgentPermissionAsk,
			},
		},
	}

	err := spec.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicates id")
}

func TestMergeAgentSpecsMergesProvidersByID(t *testing.T) {
	base := &AgentRuntimeSpec{
		Providers: []ProviderConfig{
			{ID: "mcp-client", Kind: ProviderKindMCPClient, Enabled: true, Target: "https://old.example"},
		},
	}
	overlay := AgentSpecOverlay{
		Providers: []ProviderConfig{
			{ID: "mcp-client", Kind: ProviderKindMCPClient, Enabled: true, Target: "https://new.example"},
			{ID: "mcp-server", Kind: ProviderKindMCPServer, Enabled: true, Target: "stdio://local"},
		},
	}

	merged := MergeAgentSpecs(base, overlay)

	require.Len(t, merged.Providers, 2)
	require.Equal(t, "https://new.example", merged.Providers[0].Target)
	require.Equal(t, "mcp-server", merged.Providers[1].ID)
}

func TestAgentRuntimeSpecValidateRejectsUnknownGlobalPolicyClass(t *testing.T) {
	spec := &AgentRuntimeSpec{
		Mode: AgentModePrimary,
		Model: AgentModelConfig{
			Provider: "ollama",
			Name:     "test",
		},
		GlobalPolicies: map[string]AgentPermissionLevel{
			"totally-custom": AgentPermissionAsk,
		},
	}

	err := spec.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown capability class")
}
