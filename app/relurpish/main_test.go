package main

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	runtimesvc "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
)

func TestNewRootCmdRegistersCoreEntryPoints(t *testing.T) {
	root := newRootCmd()
	require.Equal(t, "relurpish", root.Use)
	require.Equal(t, "Bubble Tea shell for the Relurpify agent runtime", root.Short)

	want := map[string]bool{
		"doctor": true,
		"status": true,
		"chat":   true,
	}
	for _, cmd := range root.Commands() {
		delete(want, cmd.Name())
	}
	require.Empty(t, want)
}

func TestNewRootCmdPersistentPreRunNormalizesConfig(t *testing.T) {
	originalCfg := cfg
	t.Cleanup(func() {
		cfg = originalCfg
	})

	workspace := t.TempDir()
	cfg = runtimesvc.Config{
		Workspace:     workspace,
		AgentsDir:     "agents",
		MemoryPath:    "memory",
		LogPath:       "logs/relurpish.log",
		TelemetryPath: "telemetry/telemetry.jsonl",
		EventsPath:    "events.db",
		ConfigPath:    "manifest.yaml",
		AgentName:     "",
		AuditLimit:    0,
		HITLTimeout:   0,
	}

	root := newRootCmd()
	require.NoError(t, root.PersistentPreRunE(root, nil))

	require.True(t, filepath.IsAbs(cfg.Workspace))
	require.Equal(t, "euclo", cfg.AgentName)
	require.Equal(t, "ollama", cfg.InferenceProvider)
	require.Equal(t, "http://localhost:11434", cfg.InferenceEndpoint)
	require.Equal(t, 256, cfg.AuditLimit)
	require.Equal(t, 30*time.Second, cfg.HITLTimeout)
	require.True(t, filepath.IsAbs(cfg.AgentsDir))
	require.True(t, filepath.IsAbs(cfg.MemoryPath))
	require.True(t, filepath.IsAbs(cfg.LogPath))
	require.True(t, filepath.IsAbs(cfg.TelemetryPath))
	require.True(t, filepath.IsAbs(cfg.EventsPath))
	require.True(t, filepath.IsAbs(cfg.ConfigPath))
}

// TestNewRootCmdVersionFlagReportsBuildMetadata drives the --version flag with
// the link-time build metadata overridden, which is exactly what GoReleaser's
// ldflags injection produces in a release build.
func TestNewRootCmdVersionFlagReportsBuildMetadata(t *testing.T) {
	origVersion, origCommit, origDate := version, commit, date
	version, commit, date = "v9.9.9", "deadbeef", "2026-10-08T00:00:00Z"
	t.Cleanup(func() { version, commit, date = origVersion, origCommit, origDate })

	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--version"})

	require.NoError(t, root.Execute())
	require.Equal(t, "relurpish v9.9.9 (commit deadbeef, built 2026-10-08T00:00:00Z)\n", out.String())
}

// TestNewRootCmdVersionDefaultsToLocalBuild pins the values a plain
// `go build` from source reports.
func TestNewRootCmdVersionDefaultsToLocalBuild(t *testing.T) {
	root := newRootCmd()
	require.Equal(t, "dev", root.Version)

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--version"})

	require.NoError(t, root.Execute())
	require.Equal(t, "relurpish dev (commit none, built unknown)\n", out.String())
}
