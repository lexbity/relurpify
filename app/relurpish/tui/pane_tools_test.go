package tui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToolRuntimeLabelIncludesFamilyAndScope(t *testing.T) {
	require.Equal(t, "local-tool/builtin", toolRuntimeLabel(ToolInfo{
		RuntimeFamily: "local-tool",
		Scope:         "builtin",
	}))
	require.Equal(t, "provider/remote", toolRuntimeLabel(ToolInfo{
		RuntimeFamily: "provider",
		Scope:         "remote",
	}))
}

func TestToolsPaneEmptyStateMentionsLocalTools(t *testing.T) {
	pane := &ToolsPane{}
	view := pane.View()
	require.Contains(t, view, "Local Tools & Permissions")
	require.Contains(t, view, "No local tools registered.")
}
