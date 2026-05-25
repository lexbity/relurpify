package contracts

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeToolName(t *testing.T) {
	require.Equal(t, "cli_git", NormalizeToolName("  CLI-Git  "))
	require.Equal(t, "shell_query", NormalizeToolName("shell.query"))
	require.Equal(t, "tool_name", NormalizeToolName("tool name"))
}

func TestToolManifestBackendConstants(t *testing.T) {
	require.Equal(t, ToolBackendSubprocess, ToolBackend("subprocess"))
	require.Equal(t, ToolBackendGoNative, ToolBackend("go_native"))
	require.Equal(t, ToolBackendMCP, ToolBackend("mcp"))
}
