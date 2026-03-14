package agents

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestApplyManifestDefaultsForAgentMigratesCodingReactToEucloByDefault(t *testing.T) {
	spec := ApplyManifestDefaultsForAgent("coding", &core.AgentRuntimeSpec{
		Implementation: "react",
		Model:          core.AgentModelConfig{Name: "stub"},
	}, nil)
	require.Equal(t, "coding", spec.Implementation)
}

func TestApplyManifestDefaultsForAgentPreservesLegacyReactWhenCompatEnabled(t *testing.T) {
	t.Setenv(codingRuntimeCompatEnv, "legacy-react")
	spec := ApplyManifestDefaultsForAgent("coding", &core.AgentRuntimeSpec{
		Implementation: "react",
		Model:          core.AgentModelConfig{Name: "stub"},
	}, nil)
	require.Equal(t, "react", spec.Implementation)
}

func TestApplyManifestDefaultsForAgentDefaultsEmptyCodingImplementationToCoding(t *testing.T) {
	spec := ApplyManifestDefaultsForAgent("coding", &core.AgentRuntimeSpec{
		Model: core.AgentModelConfig{Name: "stub"},
	}, nil)
	require.Equal(t, "coding", spec.Implementation)
}
