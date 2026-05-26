package cfgload

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadAgentDefinitions(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, "relurpify_cfg", "agents")
	require.NoError(t, os.MkdirAll(agentsDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(agentsDir, "alpha.yaml"), []byte(`kind: AgentDefinition
name: alpha
spec:
  mode: primary
  model:
    provider: ollama
    name: gemma4:e4b
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(agentsDir, "skip.yaml"), []byte(`kind: base
name: skip
spec:
  mode: primary
  model:
    provider: ollama
    name: gemma4:e4b
`), 0o644))

	defs, err := LoadAgentDefinitions(root, agentsDir)
	require.NoError(t, err)
	require.Len(t, defs, 1)
	def := defs["alpha"]
	require.NotNil(t, def)
	require.Equal(t, "alpha", def.Name)
	require.Equal(t, "primary", string(def.Spec.Mode))
	require.Equal(t, "ollama", def.Spec.Model.Provider)
	require.Equal(t, "gemma4:e4b", def.Spec.Model.Name)
}
