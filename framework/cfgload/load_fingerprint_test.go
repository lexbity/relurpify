package cfgload

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"github.com/stretchr/testify/require"
)

func TestConfigFingerprintDeterministic(t *testing.T) {
	left := &AppConfig{
		Workspace: WorkspaceConfig{WorkspaceAbs: "/tmp/workspace"},
		Tools: &ToolRegistry{
			manifests: map[string]contracts.ToolManifest{
				"beta":  {Name: "beta"},
				"alpha": {Name: "alpha"},
			},
			policies: map[string]agentspec.ToolPolicy{
				"beta":  {Execute: agentspec.AgentPermissionAllow},
				"alpha": {Execute: agentspec.AgentPermissionAsk},
			},
			ordered: []string{"alpha", "beta"},
		},
		Agents: AgentRegistry{
			Agents: map[string]*AgentConfig{
				"beta":  {Name: "beta"},
				"alpha": {Name: "alpha"},
			},
		},
		Editor:     "vim",
		SharedRoot: "/tmp/shared",
	}
	right := &AppConfig{
		Workspace: WorkspaceConfig{WorkspaceAbs: "/tmp/workspace"},
		Tools: &ToolRegistry{
			manifests: map[string]contracts.ToolManifest{
				"alpha": {Name: "alpha"},
				"beta":  {Name: "beta"},
			},
			policies: map[string]agentspec.ToolPolicy{
				"alpha": {Execute: agentspec.AgentPermissionAsk},
				"beta":  {Execute: agentspec.AgentPermissionAllow},
			},
			ordered: []string{"alpha", "beta"},
		},
		Agents: AgentRegistry{
			Agents: map[string]*AgentConfig{
				"alpha": {Name: "alpha"},
				"beta":  {Name: "beta"},
			},
		},
		Editor:     "vim",
		SharedRoot: "/tmp/shared",
	}

	leftFP, err := ConfigFingerprint(left)
	require.NoError(t, err)
	rightFP, err := ConfigFingerprint(right)
	require.NoError(t, err)

	require.Equal(t, leftFP, rightFP)
}

func TestConfigFingerprintChangesOnEdit(t *testing.T) {
	base := &AppConfig{
		Workspace: WorkspaceConfig{WorkspaceAbs: "/tmp/workspace"},
		Agents: AgentRegistry{
			Agents: map[string]*AgentConfig{
				"alpha": {Name: "alpha"},
			},
		},
		Editor:     "vim",
		SharedRoot: "/tmp/shared",
	}
	changed := &AppConfig{
		Workspace: WorkspaceConfig{WorkspaceAbs: "/tmp/workspace"},
		Agents: AgentRegistry{
			Agents: map[string]*AgentConfig{
				"alpha": {Name: "alpha"},
			},
		},
		Editor:     "nvim",
		SharedRoot: "/tmp/shared",
	}

	baseFP, err := ConfigFingerprint(base)
	require.NoError(t, err)
	changedFP, err := ConfigFingerprint(changed)
	require.NoError(t, err)

	require.NotEqual(t, baseFP, changedFP)
}
