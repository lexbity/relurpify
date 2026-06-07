package agentspec

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
			"remote-plugin": {
				Activate:     AgentPermissionAsk,
				DefaultTrust: TrustClassRemoteDeclared,
			},
		},
		Providers: []ProviderConfig{
			{
				ID:             "remote-plugin",
				Kind:           ProviderKindPlugin,
				Enabled:        true,
				Target:         "https://plugin.example.test",
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
	err := ValidateCapabilitySelector(CapabilitySelector{
		CoordinationRoles:          []CoordinationRole{CoordinationRoleReviewer},
		CoordinationTaskTypes:      []string{"review"},
		CoordinationExecutionModes: []CoordinationExecutionMode{CoordinationExecutionModeBackgroundAgent},
		CoordinationLongRunning:    EnabledStateEnabled,
	})
	require.NoError(t, err)
}

func TestValidateCapabilitySelectorAcceptsRuntimeFamilyField(t *testing.T) {
	err := ValidateCapabilitySelector(CapabilitySelector{
		RuntimeFamilies: []CapabilityRuntimeFamily{CapabilityRuntimeFamilyRelurpic},
	})
	require.NoError(t, err)
}

func TestAgentRuntimeSpecValidateAcceptsRelurpicCapabilities(t *testing.T) {
	spec := &AgentRuntimeSpec{
		Mode: AgentModePrimary,
		Model: AgentModelConfig{
			Provider: "ollama",
			Name:     "test",
		},
		Capabilities: AgentCapabilitiesSpec{
			Relurpic: []string{
				"euclo:cap.test_run",
				"euclo:cap.ast_query",
			},
		},
	}

	require.NoError(t, spec.Validate())
}

func TestAgentRuntimeSpecValidateRejectsInvalidRelurpicCapabilityID(t *testing.T) {
	spec := &AgentRuntimeSpec{
		Mode: AgentModePrimary,
		Model: AgentModelConfig{
			Provider: "ollama",
			Name:     "test",
		},
		Capabilities: AgentCapabilitiesSpec{
			Relurpic: []string{" invalid"},
		},
	}

	err := spec.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "capabilities.relurpic")
}

func TestEffectiveAllowedCapabilitySelectorsClonesSelectors(t *testing.T) {
	spec := &AgentRuntimeSpec{
		AllowedCapabilities: []CapabilitySelector{{
			RuntimeFamilies:             []CapabilityRuntimeFamily{CapabilityRuntimeFamilyProvider},
			Tags:                        []string{"lang:go"},
			CoordinationRoles:           []CoordinationRole{CoordinationRolePlanner},
			CoordinationTaskTypes:       []string{"plan"},
			CoordinationExecutionModes:  []CoordinationExecutionMode{CoordinationExecutionModeSessionBacked},
			CoordinationLongRunning:     EnabledStateEnabled,
			CoordinationDirectInsertion: EnabledStateDisabled,
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
	selectors[0].CoordinationLongRunning = EnabledStateDisabled
	require.Equal(t, []CapabilityRuntimeFamily{CapabilityRuntimeFamilyProvider}, spec.AllowedCapabilities[0].RuntimeFamilies)
	require.Equal(t, []string{"lang:go"}, spec.AllowedCapabilities[0].Tags)
	require.Equal(t, []CoordinationRole{CoordinationRolePlanner}, spec.AllowedCapabilities[0].CoordinationRoles)
	require.Equal(t, []string{"plan"}, spec.AllowedCapabilities[0].CoordinationTaskTypes)
	require.Equal(t, EnabledStateEnabled, spec.AllowedCapabilities[0].CoordinationLongRunning)
}

func TestCloneCapabilitySelectorsDeepCopiesAllSelectorFields(t *testing.T) {
	input := []CapabilitySelector{{
		ID:                          "selector-1",
		Name:                        "reviewer",
		Kind:                        CapabilityKindTool,
		RuntimeFamilies:             []CapabilityRuntimeFamily{CapabilityRuntimeFamilyRelurpic},
		Tags:                        []string{"lang:go"},
		ExcludeTags:                 []string{"unsafe"},
		SourceScopes:                []CapabilityScope{CapabilityScopeProvider},
		TrustClasses:                []TrustClass{TrustClassRemoteDeclared},
		RiskClasses:                 []RiskClass{RiskClassExecute},
		EffectClasses:               []EffectClass{EffectClassProcessSpawn},
		CoordinationRoles:           []CoordinationRole{CoordinationRoleReviewer},
		CoordinationTaskTypes:       []string{"review"},
		CoordinationExecutionModes:  []CoordinationExecutionMode{CoordinationExecutionModeBackgroundAgent},
		CoordinationLongRunning:     EnabledStateEnabled,
		CoordinationDirectInsertion: EnabledStateDisabled,
	}}

	cloned := CloneCapabilitySelectors(input)
	require.Len(t, cloned, 1)

	cloned[0].RuntimeFamilies[0] = CapabilityRuntimeFamilyProvider
	cloned[0].Tags[0] = "lang:rust"
	cloned[0].ExcludeTags[0] = "mutated"
	cloned[0].SourceScopes[0] = CapabilityScopeWorkspace
	cloned[0].TrustClasses[0] = TrustClassWorkspaceTrusted
	cloned[0].RiskClasses[0] = RiskClassNetwork
	cloned[0].EffectClasses[0] = EffectClassNetworkEgress
	cloned[0].CoordinationRoles[0] = CoordinationRolePlanner
	cloned[0].CoordinationTaskTypes[0] = "plan"
	cloned[0].CoordinationExecutionModes[0] = CoordinationExecutionModeSessionBacked
	cloned[0].CoordinationLongRunning = EnabledStateDisabled
	cloned[0].CoordinationDirectInsertion = EnabledStateEnabled

	require.Equal(t, CapabilityRuntimeFamilyRelurpic, input[0].RuntimeFamilies[0])
	require.Equal(t, "lang:go", input[0].Tags[0])
	require.Equal(t, "unsafe", input[0].ExcludeTags[0])
	require.Equal(t, CapabilityScopeProvider, input[0].SourceScopes[0])
	require.Equal(t, TrustClassRemoteDeclared, input[0].TrustClasses[0])
	require.Equal(t, RiskClassExecute, input[0].RiskClasses[0])
	require.Equal(t, EffectClassProcessSpawn, input[0].EffectClasses[0])
	require.Equal(t, CoordinationRoleReviewer, input[0].CoordinationRoles[0])
	require.Equal(t, "review", input[0].CoordinationTaskTypes[0])
	require.Equal(t, CoordinationExecutionModeBackgroundAgent, input[0].CoordinationExecutionModes[0])
	require.Equal(t, EnabledStateEnabled, input[0].CoordinationLongRunning)
	require.Equal(t, EnabledStateDisabled, input[0].CoordinationDirectInsertion)
}

func TestMergeCapabilitySelectorsDeduplicatesAndDeepCopies(t *testing.T) {
	base := []CapabilitySelector{{
		Name:                        "reviewer",
		RuntimeFamilies:             []CapabilityRuntimeFamily{CapabilityRuntimeFamilyRelurpic},
		Tags:                        []string{"lang:go"},
		CoordinationRoles:           []CoordinationRole{CoordinationRoleReviewer},
		CoordinationLongRunning:     EnabledStateEnabled,
		CoordinationDirectInsertion: EnabledStateDisabled,
	}}
	extra := []CapabilitySelector{
		{
			Name:                        "reviewer",
			RuntimeFamilies:             []CapabilityRuntimeFamily{CapabilityRuntimeFamilyRelurpic},
			Tags:                        []string{"lang:go"},
			CoordinationRoles:           []CoordinationRole{CoordinationRoleReviewer},
			CoordinationLongRunning:     EnabledStateEnabled,
			CoordinationDirectInsertion: EnabledStateDisabled,
		},
		{
			Name:            "planner",
			RuntimeFamilies: []CapabilityRuntimeFamily{CapabilityRuntimeFamilyProvider},
			Tags:            []string{"lang:rust"},
		},
	}

	merged := MergeCapabilitySelectors(base, extra)
	require.Len(t, merged, 2)
	require.Equal(t, "reviewer", merged[0].Name)
	require.Equal(t, "planner", merged[1].Name)

	merged[0].Tags[0] = "mutated"
	merged[1].RuntimeFamilies[0] = CapabilityRuntimeFamilyLocalTool

	require.Equal(t, "lang:go", base[0].Tags[0])
	require.Equal(t, CapabilityRuntimeFamilyProvider, extra[1].RuntimeFamilies[0])
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
