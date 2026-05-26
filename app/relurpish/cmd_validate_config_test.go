package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	runtimesvc "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"github.com/stretchr/testify/require"
)

func TestValidateConfigValidWorkspace(t *testing.T) {
	originalCfg := cfg
	t.Cleanup(func() { cfg = originalCfg })

	cfg = runtimesvc.DefaultConfig()
	cfg.Workspace = repoRoot(t)
	cfg.EnvOverrides = nil

	cmd := newValidateCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	require.NoError(t, cmd.RunE(cmd, nil))
	require.Contains(t, out.String(), "Config valid.")
}

func TestValidateConfigBrokenWorkspace(t *testing.T) {
	originalCfg := cfg
	t.Cleanup(func() { cfg = originalCfg })

	ws := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(ws, "relurpify_cfg"), 0o755))

	cfg = runtimesvc.DefaultConfig()
	cfg.Workspace = ws
	cfg.EnvOverrides = nil

	cmd := newValidateCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	require.Contains(t, out.String(), "Validating config for workspace:")
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}
