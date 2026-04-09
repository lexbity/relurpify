package main

import (
	"path/filepath"
	"testing"
	"time"

	runtimesvc "github.com/lexcodex/relurpify/app/relurpish/runtime"
	"github.com/stretchr/testify/require"
)

func TestNewRootCmdRegistersCoreEntryPoints(t *testing.T) {
	root := newRootCmd()
	require.Equal(t, "relurpish", root.Use)
	require.Equal(t, "Bubble Tea shell for the Relurpify agent runtime", root.Short)

	want := map[string]bool{
		"doctor": true,
		"status": true,
		"chat":   true,
		"serve":  true,
	}
	for _, cmd := range root.Commands() {
		delete(want, cmd.Name())
	}
	require.Empty(t, want)
}

func TestNewRootCmdPersistentPreRunNormalizesConfig(t *testing.T) {
	originalCfg := cfg
	originalStartServer := startServer
	t.Cleanup(func() {
		cfg = originalCfg
		startServer = originalStartServer
	})

	workspace := t.TempDir()
	cfg = runtimesvc.Config{
		Workspace:     workspace,
		ManifestPath:  "agent.manifest.yaml",
		AgentsDir:     "agents",
		MemoryPath:    "memory",
		LogPath:       "logs/relurpish.log",
		TelemetryPath: "telemetry/telemetry.jsonl",
		EventsPath:    "events.db",
		ConfigPath:    "config.yaml",
		AgentName:     "",
		ServerAddr:    "",
		AuditLimit:    0,
		HITLTimeout:   0,
	}

	root := newRootCmd()
	require.NoError(t, root.PersistentPreRunE(root, nil))

	require.True(t, filepath.IsAbs(cfg.Workspace))
	require.Equal(t, "coding", cfg.AgentName)
	require.Equal(t, "ollama", cfg.InferenceProvider)
	require.Equal(t, "http://localhost:11434", cfg.InferenceEndpoint)
	require.Equal(t, ":8080", cfg.ServerAddr)
	require.Equal(t, 256, cfg.AuditLimit)
	require.Equal(t, 30*time.Second, cfg.HITLTimeout)
	require.True(t, filepath.IsAbs(cfg.ManifestPath))
	require.True(t, filepath.IsAbs(cfg.AgentsDir))
	require.True(t, filepath.IsAbs(cfg.MemoryPath))
	require.True(t, filepath.IsAbs(cfg.LogPath))
	require.True(t, filepath.IsAbs(cfg.TelemetryPath))
	require.True(t, filepath.IsAbs(cfg.EventsPath))
	require.True(t, filepath.IsAbs(cfg.ConfigPath))
}
