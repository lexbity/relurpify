package cfgload

import (
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/cfgload/model"
	"codeburg.org/lexbit/relurpify/framework/cfgload/security"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"github.com/stretchr/testify/require"
)

func TestLoadRegistry_BaseRequired(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, "relurpify_cfg", "agents")
	require.NoError(t, os.MkdirAll(agentsDir, 0o755))

	writeAgentTestFile(t, filepath.Join(agentsDir, "euclo.yaml"), `schema: relurpify/agent/v1
kind: agent
name: euclo
filesystem:
  - action: [fs:read]
    path: "${workspace}/**"
`)

	registry, err := LoadAgentRegistry(agentsDir, root, nil, model.ModelRef{Provider: "ollama", Name: "gemma4:e4b"}, testProviders(), testToolRegistry(t, "cli_git"), StrictDecode)
	require.Error(t, err)
	require.Nil(t, registry)
	require.Contains(t, err.Error(), "_base.agent.yaml")
}

func TestLoadRegistry_ModelResolution_BothFields(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, "relurpify_cfg", "agents")
	require.NoError(t, os.MkdirAll(agentsDir, 0o755))
	writeAgentTestFile(t, filepath.Join(agentsDir, "_base.agent.yaml"), `schema: relurpify/agent/v1
kind: base
filesystem:
  - action: [fs:read]
    path: "${workspace}/**"
`)
	writeAgentTestFile(t, filepath.Join(agentsDir, "euclo.yaml"), `schema: relurpify/agent/v1
kind: agent
name: euclo
model:
  provider: ollama
  name: gemma4:e4b
filesystem:
  - action: [fs:read]
    path: "${workspace}/**"
capabilities:
  tools:
    - cli_git
`)

	registry, err := LoadAgentRegistry(agentsDir, root, nil, model.ModelRef{Provider: "ollama", Name: "gemma4:e4b"}, testProviders(), testToolRegistry(t, "cli_git"), StrictDecode)
	require.NoError(t, err)
	agent := registry.Agents["euclo"]
	require.NotNil(t, agent)
	require.NotNil(t, agent.ResolvedModel)
	require.Equal(t, "ollama", agent.ResolvedModel.Provider.Name)
	require.Equal(t, "gemma4:e4b", agent.ResolvedModel.Name)
}

func TestLoadRegistry_ModelResolution_InheritsWorkspace(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, "relurpify_cfg", "agents")
	require.NoError(t, os.MkdirAll(agentsDir, 0o755))
	writeAgentTestFile(t, filepath.Join(agentsDir, "_base.agent.yaml"), `schema: relurpify/agent/v1
kind: base
filesystem:
  - action: [fs:read]
    path: "${workspace}/**"
`)
	writeAgentTestFile(t, filepath.Join(agentsDir, "rex.yaml"), `schema: relurpify/agent/v1
kind: agent
name: rex
filesystem:
  - action: [fs:read]
    path: "${workspace}/**"
capabilities:
  tools:
    - cli_git
`)

	registry, err := LoadAgentRegistry(agentsDir, root, nil, model.ModelRef{Provider: "ollama", Name: "gemma4:e4b"}, testProviders(), testToolRegistry(t, "cli_git"), StrictDecode)
	require.NoError(t, err)
	agent := registry.Agents["rex"]
	require.NotNil(t, agent.ResolvedModel)
	require.Equal(t, "ollama", agent.ResolvedModel.Provider.Name)
	require.Equal(t, "gemma4:e4b", agent.ResolvedModel.Name)
}

func TestLoadRegistry_ModelResolution_UnknownProvider(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, "relurpify_cfg", "agents")
	require.NoError(t, os.MkdirAll(agentsDir, 0o755))
	writeAgentTestFile(t, filepath.Join(agentsDir, "_base.agent.yaml"), `schema: relurpify/agent/v1
kind: base
filesystem:
  - action: [fs:read]
    path: "${workspace}/**"
`)
	writeAgentTestFile(t, filepath.Join(agentsDir, "euclo.yaml"), `schema: relurpify/agent/v1
kind: agent
name: euclo
model:
  provider: missing
  name: gemma4:e4b
filesystem:
  - action: [fs:read]
    path: "${workspace}/**"
capabilities:
  tools:
    - cli_git
`)

	_, err := LoadAgentRegistry(agentsDir, root, nil, model.ModelRef{}, testProviders(), testToolRegistry(t, "cli_git"), StrictDecode)
	require.Error(t, err)
	require.Contains(t, err.Error(), "euclo")
	require.Contains(t, err.Error(), "provider \"missing\" not found")
}

func TestLoadRegistry_CapabilityValidation_UnknownTool(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, "relurpify_cfg", "agents")
	require.NoError(t, os.MkdirAll(agentsDir, 0o755))
	writeAgentTestFile(t, filepath.Join(agentsDir, "_base.agent.yaml"), `schema: relurpify/agent/v1
kind: base
filesystem:
  - action: [fs:read]
    path: "${workspace}/**"
`)
	writeAgentTestFile(t, filepath.Join(agentsDir, "euclo.yaml"), `schema: relurpify/agent/v1
kind: agent
name: euclo
filesystem:
  - action: [fs:read]
    path: "${workspace}/**"
capabilities:
  tools:
    - missing_tool
`)

	_, err := LoadAgentRegistry(agentsDir, root, nil, model.ModelRef{Provider: "ollama", Name: "gemma4:e4b"}, testProviders(), testToolRegistry(t, "cli_git"), StrictDecode)
	require.Error(t, err)
	require.Contains(t, err.Error(), "euclo")
	require.Contains(t, err.Error(), "missing_tool")
}

func TestLoadRegistry_CollectsAllErrors(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, "relurpify_cfg", "agents")
	require.NoError(t, os.MkdirAll(agentsDir, 0o755))
	writeAgentTestFile(t, filepath.Join(agentsDir, "_base.agent.yaml"), `schema: relurpify/agent/v1
kind: base
filesystem:
  - action: [fs:read]
    path: "${workspace}/**"
`)
	writeAgentTestFile(t, filepath.Join(agentsDir, "alpha.yaml"), `schema: relurpify/agent/v1
kind: agent
name: alpha
model:
  provider: missing
  name: gemma4:e4b
capabilities:
  tools:
    - missing_tool_a
`)
	writeAgentTestFile(t, filepath.Join(agentsDir, "beta.yaml"), `schema: relurpify/agent/v1
kind: agent
name: beta
capabilities:
  tools:
    - missing_tool_b
`)

	_, err := LoadAgentRegistry(agentsDir, root, nil, model.ModelRef{}, testProviders(), testToolRegistry(t, "cli_git"), StrictDecode)
	require.Error(t, err)
	require.Contains(t, err.Error(), "alpha")
	require.Contains(t, err.Error(), "beta")
	require.Contains(t, err.Error(), "provider \"missing\" not found")
	require.Contains(t, err.Error(), "missing_tool_a")
	require.Contains(t, err.Error(), "missing_tool_b")
}

func TestLoadRegistry_IntegrationActualTree(t *testing.T) {
	root, err := repoRootPath()
	require.NoError(t, err)

	workspaceCfg, err := LoadWorkspaceConfig(filepath.Join(root, "relurpify_cfg", "workspace.yaml"), root, WorkspaceLoadOptions{})
	require.NoError(t, err)

	providers, err := model.LoadProviderDir(filepath.Join(root, "relurpify_cfg", "model", "provider"), StrictDecode)
	require.NoError(t, err)

	toolManifests, err := LoadToolManifests(filepath.Join(root, "relurpify_cfg", "tools"))
	require.NoError(t, err)
	policy, err := security.LoadLocalToolPolicy(filepath.Join(root, "relurpify_cfg", "security", "localtool.policy.yaml"), root, StrictDecode)
	require.NoError(t, err)
	toolRegistry, err := BuildRegistry(toolManifests, policy, nil)
	require.NoError(t, err)

	registry, err := LoadAgentRegistry(filepath.Join(root, "relurpify_cfg", "agents"), root, nil, workspaceCfg.Model, providers, toolRegistry, StrictDecode)
	require.NoError(t, err)
	require.Contains(t, registry.Agents, "euclo")
	require.Contains(t, registry.Agents, "rex")
	require.Contains(t, registry.Agents, "testfu")
	require.Contains(t, registry.Agents, "factory")
	for _, agent := range registry.Agents {
		require.NotNil(t, agent.ResolvedModel)
	}
}

func TestLoadAgentRegistryStrictRejectsUnknownFields(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, "relurpify_cfg", "agents")
	require.NoError(t, os.MkdirAll(agentsDir, 0o755))
	writeAgentTestFile(t, filepath.Join(agentsDir, "_base.agent.yaml"), `schema: relurpify/agent/v1
kind: base
filesystem:
  - action: [fs:read]
    path: "${workspace}/**"
`)
	writeAgentTestFile(t, filepath.Join(agentsDir, "euclo.yaml"), `schema: relurpify/agent/v1
kind: agent
name: euclo
unexpected: true
filesystem:
  - action: [fs:read]
    path: "${workspace}/**"
capabilities:
  tools:
    - cli_git
`)

	_, err := LoadAgentRegistry(agentsDir, root, nil, model.ModelRef{Provider: "ollama", Name: "gemma4:e4b"}, testProviders(), testToolRegistry(t, "cli_git"), StrictDecode)
	require.Error(t, err)
	require.Contains(t, err.Error(), "field unexpected not found")
}

func testProviders() []*model.ResolvedProvider {
	return []*model.ResolvedProvider{
		{
			Name:            "ollama",
			Kind:            "ollama",
			Endpoint:        "http://localhost:11434",
			AvailableModels: []string{"gemma4:e4b", "qwen2.5-coder:14b"},
			SourcePath:      "/tmp/ollama.provider.yaml",
		},
	}
}

func testToolRegistry(t *testing.T, names ...string) *ToolRegistry {
	t.Helper()
	defs := make([]*contracts.ToolManifest, 0, len(names))
	policy := make(map[string]agentspec.ToolPolicy, len(names))
	for _, name := range names {
		defs = append(defs, &contracts.ToolManifest{
			Name: name,
			Execution: contracts.ToolManifestExecution{
				Backend: contracts.ToolBackendSubprocess,
				Command: &contracts.ToolManifestCommand{Base: []string{name}},
			},
			Capability: contracts.ToolManifestCapability{TrustClass: "builtin_trusted"},
		})
		policy[name] = agentspec.ToolPolicy{Execute: agentspec.AgentPermissionAllow}
	}
	registry, err := BuildRegistry(defs, policy, nil)
	require.NoError(t, err)
	return registry
}

func writeAgentTestFile(t *testing.T, path, contents string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
}

func repoRootPath() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Join(wd, "..", ".."))
}
