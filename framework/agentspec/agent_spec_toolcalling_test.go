package agentspec

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestNativeToolCallingEnabled_NewField(t *testing.T) {
	var spec AgentRuntimeSpec

	require.NoError(t, yaml.Unmarshal([]byte(`
mode: primary
model:
  provider: ollama
  name: test-model
native_tool_calling: false
`), &spec))

	require.False(t, spec.NativeToolCallingEnabled())
}

func TestNativeToolCallingEnabled_IntentField(t *testing.T) {
	var spec AgentRuntimeSpec

	require.NoError(t, yaml.Unmarshal([]byte(`
mode: primary
model:
  provider: ollama
  name: test-model
tool_calling_intent: prefer_prompt
`), &spec))

	require.Equal(t, ToolCallingIntentPreferPrompt, spec.ToolCallingIntent)
	require.False(t, spec.NativeToolCallingEnabled())
}

func TestNativeToolCallingEnabled_Default(t *testing.T) {
	spec := &AgentRuntimeSpec{}

	require.True(t, spec.NativeToolCallingEnabled())
}

func TestNativeToolCallingEnabled_ExplicitTrueWins(t *testing.T) {
	enabled := true
	spec := &AgentRuntimeSpec{
		NativeToolCalling: &enabled,
	}

	require.True(t, spec.NativeToolCallingEnabled())
}

func TestMergeAgentSpecsToolCallingOverlay(t *testing.T) {
	base := &AgentRuntimeSpec{
		Mode: AgentModePrimary,
		Model: AgentModelConfig{
			Provider: "ollama",
			Name:     "test-model",
		},
	}
	enabled := false

	merged := MergeAgentSpecs(base, AgentSpecOverlay{
		NativeToolCalling: &enabled,
	})

	require.False(t, merged.NativeToolCallingEnabled())
	require.NotNil(t, merged.NativeToolCalling)
	require.False(t, *merged.NativeToolCalling)
}

func TestMergeAgentSpecsToolCallingIntentWinsOverLegacyBool(t *testing.T) {
	base := &AgentRuntimeSpec{
		Mode: AgentModePrimary,
		Model: AgentModelConfig{
			Provider: "ollama",
			Name:     "test-model",
		},
	}
	intent := ToolCallingIntentPreferPrompt
	enabled := true

	merged := MergeAgentSpecs(base, AgentSpecOverlay{
		ToolCallingIntent: &intent,
		NativeToolCalling: &enabled,
	})

	require.Equal(t, ToolCallingIntentPreferPrompt, merged.ToolCallingIntent)
	require.False(t, merged.NativeToolCallingEnabled())
}
