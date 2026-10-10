package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewRootCmdRegistersVersionFlag(t *testing.T) {
	root := NewRootCmd()
	require.Equal(t, "dev-agent", root.Use)
	require.Equal(t, "dev", root.Version)
}

// TestNewRootCmdVersionFlagReportsBuildMetadata drives the --version flag with
// the link-time build metadata overridden, which is exactly what GoReleaser's
// ldflags injection produces in a release build.
func TestNewRootCmdVersionFlagReportsBuildMetadata(t *testing.T) {
	origVersion, origCommit, origDate := version, commit, date
	version, commit, date = "v9.9.9", "deadbeef", "2026-10-08T00:00:00Z"
	t.Cleanup(func() { version, commit, date = origVersion, origCommit, origDate })

	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--version"})

	require.NoError(t, root.Execute())
	require.Equal(t, "dev-agent v9.9.9 (commit deadbeef, built 2026-10-08T00:00:00Z)\n", out.String())
}

// TestNewRootCmdVersionDefaultsToLocalBuild pins the values a plain
// `go build` from source reports.
func TestNewRootCmdVersionDefaultsToLocalBuild(t *testing.T) {
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--version"})

	require.NoError(t, root.Execute())
	require.Equal(t, "dev-agent dev (commit none, built unknown)\n", out.String())
}
