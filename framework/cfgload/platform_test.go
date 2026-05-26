package cfgload

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadPlatformConfig(t *testing.T) {
	workspace := t.TempDir()
	toolsDir := DefaultToolManifestDir(workspace)
	require.NoError(t, os.MkdirAll(toolsDir, 0o755))
	secDir := filepath.Join(workspace, "relurpify_cfg", "security")
	require.NoError(t, os.MkdirAll(secDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(toolsDir, "demo.tool.yaml"), []byte(`schema: relurpify/tool/v1
name: demo
family: cli
intent: [demo]
description: Demo tool
guidance:
  use_when: [demo]
execution:
  backend: subprocess
  command:
    base: ["demo"]
parameters:
  - name: path
    type: string
capability:
  trust_class: local
  risk_class: [read-only]
  effect_class: [inspect]
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(secDir, "localtool.policy.yaml"), []byte(`schema: relurpify/policy/localtool/v1
tools:
  demo:
    execute: ask
`), 0o644))

	cfg, err := LoadPlatformConfig(workspace)
	require.NoError(t, err)
	require.Equal(t, filepath.Clean(workspace), cfg.Workspace)
	require.Len(t, cfg.ToolManifests, 1)
	require.Equal(t, "demo", cfg.ToolManifests[0].Name)
	require.NotNil(t, cfg.ToolRegistry)
	manifest, ok := cfg.ToolRegistry.LookupTool("demo")
	require.True(t, ok)
	require.Equal(t, "demo", manifest.Name)
}
