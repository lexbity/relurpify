package testsuite

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/execution/agentenv"
	"codeburg.org/lexbit/relurpify/named/euclo"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	cfgsecurity "codeburg.org/lexbit/relurpify/userconfig/config/security"
)

func TestLiveWorkspaceHandshakeBuildsAgentContext(t *testing.T) {
	workspace := t.TempDir()
	relurpifyCfg := filepath.Join(workspace, "relurpify_cfg")
	if err := os.MkdirAll(filepath.Join(relurpifyCfg, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(relurpifyCfg, "security"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(relurpifyCfg, "agents", "euclo.yaml")
	if err := os.WriteFile(manifestPath, []byte(`schema: relurpify/agent/v1
apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: euclo
spec:
  image: ghcr.io/lexcodex/relurpify/runtime:latest
  runtime: gvisor
  permissions:
    filesystem:
      - action: fs:read
        path: ${workspace}/**
        justification: read workspace
  agent:
    implementation: euclo
    mode: primary
    model:
      provider: ollama
      name: qwen2.5-coder:14b
    capabilities:
      relurpic:
        - euclo:cap.test_run
        - euclo:cap.ast_query
        - euclo:cap.symbol_trace
        - euclo:cap.call_graph
        - euclo:cap.blame_trace
        - euclo:cap.bisect
        - euclo:cap.code_review
        - euclo:cap.diff_summary
        - euclo:cap.layer_check
        - euclo:cap.targeted_refactor
        - euclo:cap.rename_symbol
        - euclo:cap.api_compat
        - euclo:cap.boundary_report
        - euclo:cap.coverage_check
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(relurpifyCfg, "config.yaml"), []byte("provider: ollama\nmodel: qwen2.5-coder:14b\nsandbox_backend: gvisor\nagent: euclo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(relurpifyCfg, "security", "sandbox.policy.yaml"), []byte(`schema: relurpify/policy/sandbox/v1
read_only_root: false
protected_paths:
  - relurpify_cfg/agents
  - relurpify_cfg/config.yaml
  - relurpify_cfg/security
no_new_privileges: true
allowed_env_keys: []
denied_env_keys: []
network_rules: []
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(relurpifyCfg, "security", "shell.policy.yaml"), []byte(`schema: relurpify/policy/shell/v1
rules:
  - id: deny-git-reset-hard
    pattern: '(^|\s)git\s+reset\s+--hard(\s|$)'
    reason: "Destructive git reset is blocked"
    action: block
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(relurpifyCfg, "security", "localtool.policy.yaml"), []byte(`schema: relurpify/policy/localtool/v1
tools:
  cli_git:
    execute: ask
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(relurpifyCfg, "security", "workspaceingestion.policy.yaml"), []byte(`schema: relurpify/policy/ingestion/v1
rules:
  - id: allow-workspace-ingestion
    name: Workspace ingestion
    priority: 100
    enabled: true
    effect:
      action: allow
      reason: Allow workspace ingestion for configured sources
`), 0o644); err != nil {
		t.Fatal(err)
	}
	toolsDir := filepath.Join(relurpifyCfg, "tools", "shell", "fileops")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolsDir, "cli_git.tool.yaml"), []byte(`schema: relurpify/tool/v1
name: cli_git
family: fileops
intent: [inspect, repository]
description: Runs git with the provided arguments.
execution:
  backend: subprocess
  command:
    base: ["git"]
  sandbox:
    allowed_root: ${workspace}
    timeout_seconds: 30
  allow_stdin: true
  supports_workdir: true
capability:
  trust_class: builtin_trusted
  risk_class: [execute]
  effect_class: [filesystem_read]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	securityBundle, err := cfgsecurity.LoadBundle(workspace, config.StrictDecode)
	if err != nil {
		t.Fatal(err)
	}
	ws, err := agentenv.OpenWorkspace(context.Background(), agentenv.WorkspaceConfig{
		Workspace:         workspace,
		ManifestPath:      manifestPath,
		ConfigPath:        filepath.Join(relurpifyCfg, "config.yaml"),
		InferenceProvider: "ollama",
		InferenceModel:    "qwen2.5-coder:14b",
		AgentName:         "euclo",
		AgentsDir:         filepath.Join(relurpifyCfg, "agents"),
		SandboxBackend:    "gvisor",
		SecurityBundle:    securityBundle,
		AgentSpec: &agentspec.AgentRuntimeSpec{
			Mode: agentspec.AgentModePrimary,
			Model: agentspec.AgentModelConfig{
				Provider: "ollama",
				Name:     "qwen2.5-coder:14b",
			},
			Capabilities: agentspec.AgentCapabilitiesSpec{
				Relurpic: []string{
					"euclo:cap.test_run",
					"euclo:cap.ast_query",
					"euclo:cap.symbol_trace",
					"euclo:cap.call_graph",
					"euclo:cap.blame_trace",
					"euclo:cap.bisect",
					"euclo:cap.code_review",
					"euclo:cap.diff_summary",
					"euclo:cap.layer_check",
					"euclo:cap.targeted_refactor",
					"euclo:cap.rename_symbol",
					"euclo:cap.api_compat",
					"euclo:cap.boundary_report",
					"euclo:cap.coverage_check",
				},
			},
		},
	}, llm.ProviderSecrets{}, euclo.GetRegistrationFuncs())
	if err != nil {
		t.Fatal(err)
	}
	env := ws.Environment
	if env.Registry == nil {
		t.Fatal("expected capability registry")
	}
	if env.PromptRegistry == nil {
		t.Fatal("expected prompt registry")
	}
}
