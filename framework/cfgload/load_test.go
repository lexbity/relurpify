package cfgload

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type mockFlagSet struct {
	flags map[string]string
}

func (m *mockFlagSet) GetString(name string) (string, error) {
	if val, ok := m.flags[name]; ok {
		return val, nil
	}
	return "", fmt.Errorf("flag %q not found", name)
}

func (m *mockFlagSet) GetBool(name string) (bool, error) {
	return false, nil
}

func (m *mockFlagSet) GetInt(name string) (int, error) {
	return 0, nil
}

func TestLoadConsolidatedConfig(t *testing.T) {
	tmp, err := os.MkdirTemp("", "relurpify-load-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmp)

	// Create relurpify_cfg structure
	cfgDir := filepath.Join(tmp, "relurpify_cfg")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))

	// 1. workspace.yaml
	wsYAML := `schema: relurpify/workspace/v1
paths:
  state_dir: .relurpify_state
model:
  default_name: test-model
sandbox:
  backend: gvisor
logging:
  level: debug
  format: json
audit:
  retention_days: 14
telemetry:
  enabled: false
`
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "workspace.yaml"), []byte(wsYAML), 0o644))

	// 2. security policy files
	secDir := filepath.Join(cfgDir, "security")
	require.NoError(t, os.MkdirAll(secDir, 0o755))
	
	sandboxYAML := `schema: relurpify/policy/sandbox/v1
read_only_root: true
protected_paths:
  - protected/path
no_new_privileges: true
allowed_env_keys:
  - PATH
`
	require.NoError(t, os.WriteFile(filepath.Join(secDir, "sandbox.policy.yaml"), []byte(sandboxYAML), 0o644))

	shellYAML := `schema: relurpify/policy/shell/v1
rules:
  - id: prohibit_rm
    pattern: "rm -rf"
    reason: "prohibit rm -rf"
    action: block
`
	require.NoError(t, os.WriteFile(filepath.Join(secDir, "shell.policy.yaml"), []byte(shellYAML), 0o644))

	localToolYAML := `schema: relurpify/policy/localtool/v1
tools:
  test_tool:
    execute: allow
`
	require.NoError(t, os.WriteFile(filepath.Join(secDir, "localtool.policy.yaml"), []byte(localToolYAML), 0o644))

	ingestionYAML := `schema: relurpify/policy/ingestion/v1
rules:
  - id: deny_env
    name: Deny Env Files
    enabled: true
    conditions:
      export_names:
        - "**/.env"
    effect:
      action: deny
`
	require.NoError(t, os.WriteFile(filepath.Join(secDir, "workspaceingestion.policy.yaml"), []byte(ingestionYAML), 0o644))

	// 3. model provider and profiles
	modelDir := filepath.Join(cfgDir, "model")
	providerDir := filepath.Join(modelDir, "provider")
	profilesDir := filepath.Join(modelDir, "profiles")
	require.NoError(t, os.MkdirAll(providerDir, 0o755))
	require.NoError(t, os.MkdirAll(profilesDir, 0o755))

	providerYAML := `schema: relurpify/model/provider/v1
name: ollama
endpoint: http://localhost:11434
kind: ollama
`
	require.NoError(t, os.WriteFile(filepath.Join(providerDir, "ollama.provider.yaml"), []byte(providerYAML), 0o644))

	profileYAML := `schema: relurpify/model/profile/v1
pattern: "*"
tool_calling:
  intent: native
  max_concurrent_tools: 4
context:
  max_tokens: 8192
generation:
  temperature: 0.2
  top_p: 0.95
`
	require.NoError(t, os.WriteFile(filepath.Join(profilesDir, "default.llm.yaml"), []byte(profileYAML), 0o644))

	// 4. tools definitions
	toolsDir := filepath.Join(cfgDir, "tools")
	require.NoError(t, os.MkdirAll(toolsDir, 0o755))
	toolYAML := `schema: relurpify/tool/v1
name: test_tool
family: test
intent: [test_intent]
description: A mock test tool
parameters:
  - name: input
    type: string
    required: true
execution:
  backend: go_native
  implementation: test_tool_impl
capability:
  trust_class: workspace_trusted
  risk_class: [read_only]
  effect_class: [filesystem_read]
`
	require.NoError(t, os.WriteFile(filepath.Join(toolsDir, "test.tool.yaml"), []byte(toolYAML), 0o644))

	// 5. agents definitions
	agentsDir := filepath.Join(cfgDir, "agents")
	require.NoError(t, os.MkdirAll(agentsDir, 0o755))

	baseAgentYAML := `schema: relurpify/agent/v1
kind: base
sandbox:
  runtime: gvisor
  limits:
    cpu: "2"
    memory: 4Gi
execution:
  max_iterations: 16
`
	require.NoError(t, os.WriteFile(filepath.Join(agentsDir, "_base.agent.yaml"), []byte(baseAgentYAML), 0o644))

	eucloAgentYAML := `schema: relurpify/agent/v1
kind: agent
name: euclo
sandbox:
  limits:
    memory: 8Gi
model:
  name: "${RELURPIFY_MODEL}"
filesystem:
  - action: [fs:read]
    path: "${workspace}/src/**"
capabilities:
  tools:
    - test_tool
execution:
  max_iterations: 24
`
	require.NoError(t, os.WriteFile(filepath.Join(agentsDir, "euclo.yaml"), []byte(eucloAgentYAML), 0o644))

	// Run Load consolidated config
	opts := LoadOptions{
		WorkspaceRoot: tmp,
		EnvOverrides: []string{
			"RELURPIFY_MODEL=override-model",
			"RELURPIFY_STRICT=true",
			"RELURPIFY_LLM_API_KEY=llm-key",
			"RELURPIFY_NEXUS_TOKEN=nexus-token",
		},
		CLIFlags: &mockFlagSet{
			flags: map[string]string{
				"log-level": "warn",
			},
		},
	}

	appConfig, secrets, err := Load(opts)
	require.NoError(t, err)
	require.NotNil(t, appConfig)
	require.NotNil(t, secrets)

	// Verify workspace configs and overrides
	require.Equal(t, "override-model", *appConfig.Workspace.Model.DefaultName)
	require.Equal(t, "warn", *appConfig.Workspace.Logging.Level)

	// Verify model providers and profiles
	require.Len(t, appConfig.Model.Providers, 1)
	require.Equal(t, "ollama", appConfig.Model.Providers[0].Name)
	require.Len(t, appConfig.Model.Profiles, 1)
	require.Equal(t, "*", appConfig.Model.Profiles[0].Pattern)

	// Verify secrets
	require.Equal(t, "llm-key", secrets.LLMAPIKey)
	require.Equal(t, "nexus-token", secrets.NexusToken)

	// Verify tools
	tool, ok := appConfig.Tools.LookupTool("test_tool")
	require.True(t, ok)
	require.Equal(t, "test_tool", tool.Name)

	// Verify agent merging and variable resolution
	agent, exists := appConfig.Agents.Agents["euclo"]
	require.True(t, exists)
	require.Equal(t, "euclo", agent.Name)
	require.Equal(t, "8Gi", *agent.Sandbox.Limits.Memory)     // Named overridden base (base was 4Gi)
	require.Equal(t, "2", *agent.Sandbox.Limits.CPU)         // Inherited from base
	require.Equal(t, "override-model", *agent.Model.Name)     // Resolved from ${RELURPIFY_MODEL}
	require.Equal(t, 24, *agent.Execution.MaxIterations)      // Overridden (base was 16)

	// Filesystem permissions resolved ${workspace}
	require.Len(t, agent.Filesystem, 1)
	require.Equal(t, filepath.Join(tmp, "src")+"/**", agent.Filesystem[0].Path)

	// Excludes are appended with the security invariant
	require.Contains(t, agent.Filesystem[0].Exclude, filepath.Join(tmp, "relurpify_cfg")+"/**")
}

func TestResolveVariables(t *testing.T) {
	env := []string{
		"RELURPIFY_MODEL=gemma4:e4b",
		"OLLAMA_HOST=http://remote-ollama",
		"CUSTOM_VAR=my-val",
	}

	tests := []struct {
		input    string
		expected string
		err      bool
	}{
		{"${workspace}/path", "/home/ws/path", false},
		{"${RELURPIFY_MODEL}", "gemma4:e4b", false},
		{"${OLLAMA_HOST:-localhost}", "http://remote-ollama", false},
		{"${MISSING_OLLAMA_HOST:-localhost}", "localhost", false},
		{"${CUSTOM_VAR}", "my-val", false},
		{"${MISSING_VAR}", "", true},
	}

	for _, tc := range tests {
		res, err := resolveVariables(tc.input, "/home/ws", env, "default-model")
		if tc.err {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.Equal(t, tc.expected, res)
		}
	}
}

func TestAgentFilesystemSecurityInvariant(t *testing.T) {
	agent := &AgentConfig{
		Filesystem: []AgentFilesystemPerm{
			{
				Action: []string{"fs:read"},
				Path:   "/workspace/src",
			},
			{
				Action:  []string{"fs:write"},
				Path:    "/workspace/dest",
				Exclude: []string{"/workspace/relurpify_cfg/**"},
			},
		},
	}

	applyFilesystemSecurityInvariant(agent, "/workspace")
	require.Len(t, agent.Filesystem, 2)
	require.Contains(t, agent.Filesystem[0].Exclude, "/workspace/relurpify_cfg/**")
	require.Contains(t, agent.Filesystem[1].Exclude, "/workspace/relurpify_cfg/**")
	require.Len(t, agent.Filesystem[1].Exclude, 1) // Ensure no duplicate excludes
}
