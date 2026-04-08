package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewRootCmdWiresNexusishEntryPoint(t *testing.T) {
	root := newRootCmd()
	require.Equal(t, "nexusish", root.Use)
	require.Equal(t, "Terminal dashboard for the Nexus gateway", root.Short)
	require.NotNil(t, root.Flags().Lookup("workspace"))
	require.NotNil(t, root.Flags().Lookup("config"))
	require.Equal(t, ".", root.Flags().Lookup("workspace").DefValue)
	require.Equal(t, "", root.Flags().Lookup("config").DefValue)
}
