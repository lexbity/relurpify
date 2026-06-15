package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealWorkspaceConfigV1_Parses(t *testing.T) {
	_, err := LoadRuntimeWorkspaceConfigV1("../../relurpify_cfg/workspace.yaml")
	require.NoError(t, err, "relurpify_cfg/workspace.yaml must be parseable as V1")
}

func TestTemplateWorkspaceConfigV1_Parses(t *testing.T) {
	_, err := LoadRuntimeWorkspaceConfigV1("../../templates/workspace/workspace.yaml")
	require.NoError(t, err, "templates/workspace/workspace.yaml must be parseable as V1")
}

func TestRealEucloManifest_Parses(t *testing.T) {
	snap, err := LoadDocument("../../relurpify_cfg/agents/euclo.yaml")
	require.NoError(t, err, "relurpify_cfg/agents/euclo.yaml must be parseable")
	require.Equal(t, "euclo", snap.Document.Metadata.Name)
	require.Equal(t, "AgentManifest", snap.Document.Kind)
}

func TestTemplateEucloManifest_Parses(t *testing.T) {
	snap, err := LoadDocument("../../templates/workspace/agent.yaml")
	require.NoError(t, err, "templates/workspace/agent.yaml must be parseable")
	require.Equal(t, "euclo", snap.Document.Metadata.Name)
	require.Equal(t, "AgentManifest", snap.Document.Kind)
}
