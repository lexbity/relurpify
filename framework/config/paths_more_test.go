package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPathsAllHelpers(t *testing.T) {
	paths := New("")
	require.Equal(t, ".", paths.Workspace)
	require.Equal(t, filepath.Join(".", DirName), paths.ConfigRoot())
	require.Equal(t, filepath.Join(".", DirName, "config.yaml"), paths.ConfigFile())
	require.Equal(t, filepath.Join(".", DirName, "agents"), paths.AgentsDir())
	require.Equal(t, filepath.Join(".", DirName, "skills"), paths.SkillsDir())
	require.Equal(t, filepath.Join(".", DirName, "logs"), paths.LogsDir())
	require.Equal(t, filepath.Join(".", DirName, "logs", "relurpish.log"), paths.LogFile(""))
	require.Equal(t, filepath.Join(".", DirName, "telemetry"), paths.TelemetryDir())
	require.Equal(t, filepath.Join(".", DirName, "telemetry", "telemetry.jsonl"), paths.TelemetryFile(""))
	require.Equal(t, filepath.Join(".", DirName, "sessions"), paths.SessionsDir())
	require.Equal(t, filepath.Join(".", DirName, "sessions", "checkpoints"), paths.CheckpointsDir())
	require.Equal(t, filepath.Join(".", DirName, "exports"), paths.ExportsDir())
	require.Equal(t, filepath.Join(".", DirName, "testsuites"), paths.TestsuitesDir())
	require.Equal(t, filepath.Join(".", DirName, "events.db"), paths.EventsFile())
	require.Equal(t, filepath.Join(".", DirName, "nodes.db"), paths.NodesFile())
	require.Equal(t, filepath.Join(".", DirName, "sessions.db"), paths.SessionStoreFile())
	require.Equal(t, filepath.Join(".", DirName, "test_runs", "a", "b"), paths.TestRunDir("a", "b"))
	require.Equal(t, filepath.Join(".", DirName, "test_runs", "a", "b", "logs"), paths.TestRunLogsDir("a", "b"))
	require.Equal(t, filepath.Join(".", DirName, "test_runs", "a", "b", "telemetry"), paths.TestRunTelemetryDir("a", "b"))
	require.Equal(t, filepath.Join(".", DirName, "test_runs", "a", "b", "artifacts"), paths.TestRunArtifactsDir("a", "b"))
	require.Equal(t, filepath.Join(".", DirName, "test_runs", "a", "b", "tmp"), paths.TestRunTmpDir("a", "b"))
	require.Equal(t, filepath.Join(".", DirName, "memory"), paths.MemoryDir())
	require.Equal(t, filepath.Join(".", DirName, "memory", "ast_index"), paths.ASTIndexDir())
	require.Equal(t, filepath.Join(".", DirName, "memory", "ast_index", "index.db"), paths.ASTIndexDB())
	require.Equal(t, filepath.Join(".", DirName, "memory", "retrieval.db"), paths.RetrievalDB())
	require.Equal(t, filepath.Join(".", DirName, "shell_blacklist.yaml"), paths.ShellBlacklistFile())
	require.Equal(t, filepath.Join(".", DirName, "model_profiles"), paths.ModelProfilesDir())
}
