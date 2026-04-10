package command

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/authorization"
	"github.com/stretchr/testify/require"
)

func TestCommandToolSettersAndTags(t *testing.T) {
	tool := NewCommandTool(t.TempDir(), CommandToolConfig{
		Name:        "cli_echo",
		Description: "echo",
		Command:     "echo",
		Tags:        []string{core.TagExecute, "example"},
	})

	require.Equal(t, []string{core.TagExecute, "example"}, tool.Tags())

	var manager *authorization.PermissionManager
	tool.SetPermissionManager(manager, "agent-1")
	require.Nil(t, tool.manager)
	require.Equal(t, "agent-1", tool.agentID)

	spec := &core.AgentRuntimeSpec{}
	tool.SetAgentSpec(spec, "agent-2")
	require.Equal(t, spec, tool.spec)
	require.Equal(t, "agent-2", tool.agentID)
}
