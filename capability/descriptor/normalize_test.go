package descriptor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/governance/classification"
	"codeburg.org/lexbit/relurpify/model"
)

func TestEnabledState(t *testing.T) {
	assert.False(t, EnabledStateUnset.IsSet())
	assert.False(t, EnabledStateUnset.IsEnabled())
	assert.True(t, EnabledStateEnabled.IsSet())
	assert.True(t, EnabledStateEnabled.IsEnabled())
	assert.True(t, EnabledStateDisabled.IsSet())
	assert.False(t, EnabledStateDisabled.IsEnabled())
	assert.True(t, EnabledState(99).IsSet(), "any value beyond the vocabulary counts as set")
	assert.False(t, EnabledState(99).IsEnabled())
}

func TestNormalizeCapabilityDescriptor_EmptyDescriptorGetsDefaults(t *testing.T) {
	desc := NormalizeCapabilityDescriptor(CapabilityDescriptor{})

	assert.Equal(t, agentspec.CapabilityKindTool, desc.Kind)
	assert.Equal(t, agentspec.CapabilityRuntimeFamilyLocalTool, desc.RuntimeFamily)
	assert.Equal(t, classification.CapabilityScopeBuiltin, desc.Source.Scope)
	assert.Nil(t, desc.Tags)
	assert.Nil(t, desc.EffectClasses)
	assert.Nil(t, desc.Coordination)
}

func TestNormalizeCapabilityDescriptor_DefaultRuntimeFamily(t *testing.T) {
	cases := []struct {
		name string
		desc CapabilityDescriptor
		want agentspec.CapabilityRuntimeFamily
	}{
		{
			"builtin prompt is relurpic",
			CapabilityDescriptor{Kind: agentspec.CapabilityKindPrompt},
			agentspec.CapabilityRuntimeFamilyRelurpic,
		},
		{
			"provider-backed prompt is provider family",
			CapabilityDescriptor{Kind: agentspec.CapabilityKindPrompt, Source: CapabilitySource{ProviderID: "p1"}},
			agentspec.CapabilityRuntimeFamilyProvider,
		},
		{
			"remote resource is provider family",
			CapabilityDescriptor{Kind: agentspec.CapabilityKindResource, Source: CapabilitySource{Scope: classification.CapabilityScopeRemote}},
			agentspec.CapabilityRuntimeFamilyProvider,
		},
		{
			"builtin session is relurpic",
			CapabilityDescriptor{Kind: agentspec.CapabilityKindSession},
			agentspec.CapabilityRuntimeFamilyRelurpic,
		},
		{
			"provider-scoped subscription is provider family",
			CapabilityDescriptor{Kind: agentspec.CapabilityKindSubscription, Source: CapabilitySource{Scope: classification.CapabilityScopeProvider}},
			agentspec.CapabilityRuntimeFamilyProvider,
		},
		{
			"builtin tool is local",
			CapabilityDescriptor{Kind: agentspec.CapabilityKindTool},
			agentspec.CapabilityRuntimeFamilyLocalTool,
		},
		{
			"provider-backed tool is provider family",
			CapabilityDescriptor{Kind: agentspec.CapabilityKindTool, Source: CapabilitySource{Scope: classification.CapabilityScopeProvider}},
			agentspec.CapabilityRuntimeFamilyProvider,
		},
		{
			"unknown kind with provider id is provider family",
			CapabilityDescriptor{Kind: agentspec.CapabilityKind("custom"), Source: CapabilitySource{ProviderID: "p1"}},
			agentspec.CapabilityRuntimeFamilyProvider,
		},
		{
			"unknown kind builtin is relurpic",
			CapabilityDescriptor{Kind: agentspec.CapabilityKind("custom")},
			agentspec.CapabilityRuntimeFamilyRelurpic,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			desc := NormalizeCapabilityDescriptor(tc.desc)
			assert.Equal(t, tc.want, desc.RuntimeFamily)
		})
	}
}

func TestNormalizeCapabilityDescriptor_NormalizesTagsAndEffectClasses(t *testing.T) {
	desc := NormalizeCapabilityDescriptor(CapabilityDescriptor{
		Tags:          []string{"Fast", "fast", "DESTRUCTIVE", "My_Tag"},
		EffectClasses: []classification.EffectClass{"network_egress", "NETWORK_EGRESS", ""},
	})

	assert.Equal(t, []string{"fast", "my-tag"}, desc.Tags)
	assert.Equal(t, []classification.EffectClass{classification.EffectClassNetworkEgress}, desc.EffectClasses)
}

func TestNormalizeCapabilityDescriptor_CoordinationNormalization(t *testing.T) {
	inputSchema := schemaFixture("in")
	outputSchema := schemaFixture("out")

	desc := NormalizeCapabilityDescriptor(CapabilityDescriptor{
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
		Coordination: &CoordinationTargetMetadata{
			Target:         true,
			Role:           agentspec.CoordinationRoleExecutor,
			TaskTypes:      []string{" review ", "", "review", "audit"},
			ExecutionModes: []agentspec.CoordinationExecutionMode{" sync ", "", agentspec.CoordinationExecutionModeSessionBacked},
		},
	})

	meta := desc.Coordination
	require.NotNil(t, meta)
	assert.Equal(t, []string{"audit", "review"}, meta.TaskTypes)
	assert.Equal(t, []agentspec.CoordinationExecutionMode{
		agentspec.CoordinationExecutionModeSessionBacked,
		agentspec.CoordinationExecutionModeSync,
	}, meta.ExecutionModes)

	// Expected schemas default from the descriptor's own schemas.
	require.NotNil(t, meta.ExpectedInput)
	assert.Equal(t, "in", meta.ExpectedInput.Title)
	require.NotNil(t, meta.ExpectedOutput)
	assert.Equal(t, "out", meta.ExpectedOutput.Title)
}

func TestNormalizeCapabilityDescriptor_ClonesCoordinationSchemas(t *testing.T) {
	shared := &CoordinationTargetMetadata{
		Target:         true,
		Role:           agentspec.CoordinationRoleExecutor,
		TaskTypes:      []string{"t"},
		ExpectedInput:  schemaFixture("declared-in"),
		ExpectedOutput: schemaFixture("declared-out"),
	}
	desc := NormalizeCapabilityDescriptor(CapabilityDescriptor{
		Coordination: shared,
	})
	meta := desc.Coordination
	require.NotNil(t, meta)

	// The stored metadata must be a clone: mutating it must not touch the
	// original coordination metadata handed to Normalize.
	require.NotNil(t, meta.ExpectedInput)
	require.NotNil(t, meta.ExpectedOutput)
	assert.Equal(t, "declared-in", meta.ExpectedInput.Title)
	assert.Equal(t, "declared-out", meta.ExpectedOutput.Title)
	meta.ExpectedInput.Title = "mutated"
	meta.ExpectedOutput.Title = "mutated"
	assert.Equal(t, "declared-in", shared.ExpectedInput.Title,
		"normalization must clone nested schemas, not alias them")
	assert.Equal(t, "declared-out", shared.ExpectedOutput.Title,
		"normalization must clone nested schemas, not alias them")
	meta.TaskTypes[0] = "mutated"
	assert.Equal(t, "t", shared.TaskTypes[0], "task types must be cloned into a fresh slice")
}

func TestNormalizeCapabilityDescriptor_BlankCoordinationListsCollapseToNil(t *testing.T) {
	desc := NormalizeCapabilityDescriptor(CapabilityDescriptor{
		Coordination: &CoordinationTargetMetadata{
			Target:         true,
			Role:           agentspec.CoordinationRoleExecutor,
			TaskTypes:      []string{"   ", ""},
			ExecutionModes: []agentspec.CoordinationExecutionMode{"  ", ""},
		},
	})
	meta := desc.Coordination
	require.NotNil(t, meta)
	assert.Nil(t, meta.TaskTypes, "task types that are entirely blank collapse to nil")
	assert.Nil(t, meta.ExecutionModes, "execution modes that are entirely blank collapse to nil")
}

func TestNormalizeCapabilityDescriptor_BackgroundAgentRoleIsForcedLongRunning(t *testing.T) {
	desc := NormalizeCapabilityDescriptor(CapabilityDescriptor{
		Coordination: &CoordinationTargetMetadata{
			Target: true,
			Role:   agentspec.CoordinationRoleBackgroundAgent,
		},
	})
	meta := desc.Coordination
	require.NotNil(t, meta)
	assert.Equal(t, EnabledStateEnabled, meta.LongRunning,
		"background-agent role forces long-running enabled")
	assert.Contains(t, meta.ExecutionModes, agentspec.CoordinationExecutionModeBackgroundAgent,
		"background-agent role gains the background-service execution mode")
}

func TestNormalizeCapabilityDescriptor_BackgroundAgentRoleKeepsExistingMode(t *testing.T) {
	modes := []agentspec.CoordinationExecutionMode{agentspec.CoordinationExecutionModeBackgroundAgent}
	desc := NormalizeCapabilityDescriptor(CapabilityDescriptor{
		Coordination: &CoordinationTargetMetadata{
			Target:         true,
			Role:           agentspec.CoordinationRoleBackgroundAgent,
			ExecutionModes: modes,
		},
	})
	meta := desc.Coordination
	require.NotNil(t, meta)
	assert.Equal(t, []agentspec.CoordinationExecutionMode{agentspec.CoordinationExecutionModeBackgroundAgent}, meta.ExecutionModes)
	assert.Len(t, meta.ExecutionModes, 1, "the mode must not be appended twice")
}

func TestValidateCoordinationTargetMetadata(t *testing.T) {
	valid := func() *CoordinationTargetMetadata {
		return &CoordinationTargetMetadata{
			Target:         true,
			Role:           agentspec.CoordinationRoleExecutor,
			TaskTypes:      []string{"coding"},
			ExecutionModes: []agentspec.CoordinationExecutionMode{agentspec.CoordinationExecutionModeSync},
		}
	}

	t.Run("nil metadata is valid", func(t *testing.T) {
		assert.NoError(t, ValidateCoordinationTargetMetadata(nil))
	})
	t.Run("minimal valid metadata", func(t *testing.T) {
		assert.NoError(t, ValidateCoordinationTargetMetadata(valid()))
	})
	t.Run("disabled target is rejected", func(t *testing.T) {
		meta := valid()
		meta.Target = false
		err := ValidateCoordinationTargetMetadata(meta)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be enabled")
	})

	for _, role := range []agentspec.CoordinationRole{
		agentspec.CoordinationRolePlanner,
		agentspec.CoordinationRoleArchitect,
		agentspec.CoordinationRoleReviewer,
		agentspec.CoordinationRoleVerifier,
		agentspec.CoordinationRoleExecutor,
		agentspec.CoordinationRoleDomainPack,
		agentspec.CoordinationRoleBackgroundAgent,
	} {
		t.Run("valid role "+string(role), func(t *testing.T) {
			meta := valid()
			meta.Role = role
			if role == agentspec.CoordinationRoleBackgroundAgent {
				meta.ExecutionModes = []agentspec.CoordinationExecutionMode{agentspec.CoordinationExecutionModeBackgroundAgent}
			}
			assert.NoError(t, ValidateCoordinationTargetMetadata(meta))
		})
	}

	t.Run("unknown role rejected", func(t *testing.T) {
		meta := valid()
		meta.Role = agentspec.CoordinationRole("overlord")
		err := ValidateCoordinationTargetMetadata(meta)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "coordination role")
	})
	t.Run("empty task types rejected", func(t *testing.T) {
		meta := valid()
		meta.TaskTypes = nil
		err := ValidateCoordinationTargetMetadata(meta)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "task_types required")
	})
	t.Run("blank task type rejected", func(t *testing.T) {
		meta := valid()
		meta.TaskTypes = []string{"coding", "  "}
		err := ValidateCoordinationTargetMetadata(meta)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot contain empty")
	})
	t.Run("empty execution modes rejected", func(t *testing.T) {
		meta := valid()
		meta.ExecutionModes = nil
		err := ValidateCoordinationTargetMetadata(meta)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "execution_modes required")
	})
	t.Run("unknown execution mode rejected", func(t *testing.T) {
		meta := valid()
		meta.ExecutionModes = []agentspec.CoordinationExecutionMode{"teleport"}
		err := ValidateCoordinationTargetMetadata(meta)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "execution mode")
	})
	t.Run("invalid long running rejected", func(t *testing.T) {
		meta := valid()
		meta.LongRunning = EnabledState(42)
		err := ValidateCoordinationTargetMetadata(meta)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "long_running")
	})
	t.Run("invalid direct insertion rejected", func(t *testing.T) {
		meta := valid()
		meta.DirectInsertionAllowed = EnabledState(-7)
		err := ValidateCoordinationTargetMetadata(meta)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "direct_insertion")
	})
	t.Run("negative max depth rejected", func(t *testing.T) {
		meta := valid()
		meta.MaxDepth = -1
		err := ValidateCoordinationTargetMetadata(meta)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max_depth")
	})
	t.Run("negative max runtime rejected", func(t *testing.T) {
		meta := valid()
		meta.MaxRuntimeSeconds = -5
		err := ValidateCoordinationTargetMetadata(meta)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max_runtime_seconds")
	})
	t.Run("long running requires session-backed or background mode", func(t *testing.T) {
		meta := valid()
		meta.LongRunning = EnabledStateEnabled
		meta.ExecutionModes = []agentspec.CoordinationExecutionMode{agentspec.CoordinationExecutionModeSync}
		err := ValidateCoordinationTargetMetadata(meta)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "session-backed or background-service")

		meta.ExecutionModes = []agentspec.CoordinationExecutionMode{agentspec.CoordinationExecutionModeSessionBacked}
		assert.NoError(t, ValidateCoordinationTargetMetadata(meta))
	})
	t.Run("background agent role requires background mode", func(t *testing.T) {
		meta := valid()
		meta.Role = agentspec.CoordinationRoleBackgroundAgent
		meta.ExecutionModes = []agentspec.CoordinationExecutionMode{agentspec.CoordinationExecutionModeSync}
		err := ValidateCoordinationTargetMetadata(meta)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "background-service execution mode")
	})
	t.Run("all enabled states accepted", func(t *testing.T) {
		for _, state := range []EnabledState{EnabledStateUnset, EnabledStateEnabled, EnabledStateDisabled} {
			meta := valid()
			meta.LongRunning = state
			meta.DirectInsertionAllowed = state
			// Session-backed mode keeps the long-running combination legal
			// for every state (the illegal combinations are covered above).
			meta.ExecutionModes = []agentspec.CoordinationExecutionMode{agentspec.CoordinationExecutionModeSessionBacked}
			assert.NoError(t, ValidateCoordinationTargetMetadata(meta))
		}
	})
}

func TestCloneSchema_DeepIndependence(t *testing.T) {
	schema := &model.Schema{
		Type:       "array",
		Items:      &model.Schema{Type: "string", Title: "item"},
		Properties: map[string]*model.Schema{"a": {Type: "integer", Title: "prop"}},
		Required:   []string{"a"},
		Enum:       []any{"x", "y"},
	}

	clone := cloneSchema(schema)
	require.NotSame(t, schema, clone)
	require.NotSame(t, schema.Items, clone.Items)
	require.NotSame(t, schema.Properties["a"], clone.Properties["a"])

	clone.Items.Title = "mutated-items"
	clone.Properties["a"].Title = "mutated-prop"
	clone.Properties["extra"] = &model.Schema{Type: "boolean"}
	clone.Required[0] = "mutated"
	clone.Enum[0] = "mutated"
	clone.Type = "mutated"

	assert.Equal(t, "item", schema.Items.Title, "nested Items must be cloned")
	assert.Equal(t, "prop", schema.Properties["a"].Title, "nested properties must be cloned")
	assert.Len(t, schema.Properties, 1, "clone must not share the property map")
	assert.Equal(t, []string{"a"}, schema.Required, "required must be copied")
	assert.Equal(t, []any{"x", "y"}, schema.Enum, "enum must be copied")
	assert.Equal(t, "array", schema.Type)
}

func TestCloneSchema_NilYieldsNil(t *testing.T) {
	assert.Nil(t, cloneSchema(nil))
}

// schemaFixture builds a tiny schema tagged with a title, used to verify
// defaulting and cloning behavior.
func schemaFixture(title string) *model.Schema {
	return &model.Schema{Title: title}
}
