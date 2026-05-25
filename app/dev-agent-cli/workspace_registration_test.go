package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/platform/llm"
)

func TestOpenWorkspaceForInspectionUsesEucloRegistrationFuncs(t *testing.T) {
	workspace := t.TempDir()
	agentDir := filepath.Join(workspace, "relurpify_cfg", "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(agentDir, "euclo.yaml")
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
`), 0o644); err != nil {
		t.Fatal(err)
	}

	originalOpen := workspaceOpenFn
	originalFuncs := workspaceRegistrationFuncsFn
	t.Cleanup(func() {
		workspaceOpenFn = originalOpen
		workspaceRegistrationFuncsFn = originalFuncs
	})

	var gotFuncs agentenv.AgentRegistrationFuncs
	workspaceRegistrationFuncsFn = func() agentenv.AgentRegistrationFuncs {
		return agentenv.AgentRegistrationFuncs{
			RegisterCapabilities:    func(env agentenv.WorkspaceEnvironment) error { return nil },
			RegisterPromptProviders: func(env agentenv.WorkspaceEnvironment) error { return nil },
			LoadThoughtRecipes:      func() (interface{}, error) { return nil, nil },
		}
	}
	workspaceOpenFn = func(ctx context.Context, cfg agentenv.WorkspaceConfig, secrets llm.ProviderSecrets, funcs agentenv.AgentRegistrationFuncs) (*agentenv.Workspace, error) {
		gotFuncs = funcs
		_ = secrets
		return &agentenv.Workspace{}, nil
	}

	if _, err := openWorkspaceForInspection(context.Background(), workspace); err != nil {
		t.Fatal(err)
	}
	if gotFuncs.RegisterCapabilities == nil || gotFuncs.RegisterPromptProviders == nil || gotFuncs.LoadThoughtRecipes == nil {
		t.Fatalf("expected euclo registration funcs to be supplied, got %+v", gotFuncs)
	}
}

func TestBuildInspectionTargetResolvesWorkspaceConfig(t *testing.T) {
	workspace := t.TempDir()
	agentDir := filepath.Join(workspace, "relurpify_cfg", "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "euclo.yaml"), []byte(`schema: relurpify/agent/v1
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
`), 0o644); err != nil {
		t.Fatal(err)
	}

	target, err := buildInspectionTarget(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if target == nil {
		t.Fatal("expected inspection target")
	}
	if target.cfg.Workspace != workspace {
		t.Fatalf("workspace = %q, want %q", target.cfg.Workspace, workspace)
	}
	if target.cfg.ManifestPath == "" {
		t.Fatal("expected manifest path to be populated")
	}
	if target.cfg.InferenceProvider != "ollama" {
		t.Fatalf("expected default provider ollama, got %q", target.cfg.InferenceProvider)
	}
	if target.cfg.ConfigPath == "" || !strings.HasSuffix(target.cfg.ConfigPath, filepath.Join("relurpify_cfg", "workspace.yaml")) {
		t.Fatalf("unexpected config path: %q", target.cfg.ConfigPath)
	}
}
