package cfgload

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeAgentConfig_ScalarFieldNamedAgentWins(t *testing.T) {
	base := &AgentConfig{
		Name: "base",
		Kind: "base",
		Sandbox: AgentSandboxConfig{
			Runtime: ptrString("gvisor"),
		},
	}
	named := &AgentConfig{
		Name: "euclo",
		Kind: "agent",
		Sandbox: AgentSandboxConfig{
			Runtime: ptrString("docker"),
		},
	}

	merged := MergeAgentConfig(base, named)
	require.Equal(t, "euclo", merged.Name)
	require.Equal(t, "agent", merged.Kind)
	require.Equal(t, "docker", *merged.Sandbox.Runtime)
	require.Equal(t, "base", base.Name)
	require.Equal(t, "base", base.Kind)
	require.Equal(t, "gvisor", *base.Sandbox.Runtime)
}

func TestMergeAgentConfig_StructFieldDeepMerge(t *testing.T) {
	base := &AgentConfig{
		Sandbox: AgentSandboxConfig{
			Runtime: ptrString("gvisor"),
			Limits: AgentSandboxLimits{
				CPU:    ptrString("2"),
				Memory: ptrString("4Gi"),
			},
			Security: AgentSandboxSec{
				RunAsUser:       ptrInt(1000),
				NoNewPrivileges: ptrBool(true),
			},
		},
		Execution: AgentExecutionConfig{
			MaxIterations: ptrInt(16),
		},
	}
	named := &AgentConfig{
		Sandbox: AgentSandboxConfig{
			Limits: AgentSandboxLimits{
				Memory: ptrString("8Gi"),
			},
			Security: AgentSandboxSec{
				ReadOnlyRoot: ptrBool(false),
			},
		},
		Execution: AgentExecutionConfig{
			CheckpointPolicy: ptrString("on_phase"),
		},
	}

	merged := MergeAgentConfig(base, named)
	require.Equal(t, "gvisor", *merged.Sandbox.Runtime)
	require.Equal(t, "2", *merged.Sandbox.Limits.CPU)
	require.Equal(t, "8Gi", *merged.Sandbox.Limits.Memory)
	require.Equal(t, 1000, *merged.Sandbox.Security.RunAsUser)
	require.True(t, *merged.Sandbox.Security.NoNewPrivileges)
	require.False(t, *merged.Sandbox.Security.ReadOnlyRoot)
	require.Equal(t, 16, *merged.Execution.MaxIterations)
	require.Equal(t, "on_phase", *merged.Execution.CheckpointPolicy)
}

func TestMergeAgentConfig_ListFieldReplacement(t *testing.T) {
	base := &AgentConfig{
		Filesystem: []AgentFilesystemPerm{
			{Path: "/base", Action: []string{"fs:read"}, Exclude: []string{"/base/ex"}},
		},
		Capabilities: AgentCapabilitiesConfig{
			Tools:    []string{"cli_git", "bash"},
			Relurpic: []string{"base.cap"},
			Prompts:  []string{"base.prompt"},
		},
		Network: AgentNetworkConfig{
			Allow: []AgentNetworkRule{{Host: "localhost", Port: 1, Protocol: "tcp"}},
		},
	}
	named := &AgentConfig{
		Filesystem: []AgentFilesystemPerm{
			{Path: "/named", Action: []string{"fs:write"}, Exclude: []string{"/named/ex"}},
		},
		Capabilities: AgentCapabilitiesConfig{
			Tools:   []string{"cli_git"},
			Prompts: []string{"named.prompt"},
		},
		Network: AgentNetworkConfig{
			Allow: []AgentNetworkRule{{Host: "example.com", Port: 443, Protocol: "tcp"}},
		},
	}

	merged := MergeAgentConfig(base, named)
	require.Len(t, merged.Filesystem, 1)
	require.Equal(t, "/named", merged.Filesystem[0].Path)
	require.Equal(t, []string{"cli_git"}, merged.Capabilities.Tools)
	require.Equal(t, []string{"base.cap"}, merged.Capabilities.Relurpic)
	require.Equal(t, []string{"named.prompt"}, merged.Capabilities.Prompts)
	require.Len(t, merged.Network.Allow, 1)
	require.Equal(t, "example.com", merged.Network.Allow[0].Host)
}

func TestMergeAgentConfig_AbsentFieldInheritsBase(t *testing.T) {
	base := &AgentConfig{
		Name: "base",
		Sandbox: AgentSandboxConfig{
			Image: ptrString("image-a"),
		},
		Audit: AgentAuditConfig{
			Level: ptrString("verbose"),
		},
	}
	named := &AgentConfig{}

	merged := MergeAgentConfig(base, named)
	require.Equal(t, "base", merged.Name)
	require.Equal(t, "image-a", *merged.Sandbox.Image)
	require.Equal(t, "verbose", *merged.Audit.Level)
}

func TestMergeAgentConfig_Idempotent(t *testing.T) {
	base := &AgentConfig{
		Name: "base",
		Filesystem: []AgentFilesystemPerm{
			{Path: "/base", Action: []string{"fs:read"}, Exclude: []string{"/base/ex"}},
		},
	}
	named := &AgentConfig{
		Name: "euclo",
		Filesystem: []AgentFilesystemPerm{
			{Path: "/named", Action: []string{"fs:write"}, Exclude: []string{"/named/ex"}},
		},
	}

	first := MergeAgentConfig(base, named)
	second := MergeAgentConfig(base, named)

	require.Equal(t, first, second)
	require.Equal(t, "/base", base.Filesystem[0].Path)
	require.Equal(t, "/named", named.Filesystem[0].Path)
}

func ptrString(v string) *string { return &v }
func ptrInt(v int) *int          { return &v }
func ptrBool(v bool) *bool       { return &v }
