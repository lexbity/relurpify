package runtime

import (
	"context"
	"os"
	"testing"

	"codeburg.org/lexbit/relurpify/userconfig/config"
)

func TestBootstrapStartupStateInitializesWorkspaceTemplates(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Workspace = dir
	// Pin the inference endpoint to a guaranteed-unreachable address so the
	// doctor probe deterministically reports an unhealthy backend, regardless
	// of whether a local ollama happens to be running on the dev machine.
	cfg.InferenceEndpoint = "http://127.0.0.1:1"
	if err := cfg.Normalize(); err != nil {
		t.Fatalf("normalize config: %v", err)
	}

	state, err := BootstrapStartupState(context.Background(), cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("bootstrap startup state: %v", err)
	}
	if _, err := os.Stat(cfg.ConfigPath); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
	if state.Report.NeedsInitialization() {
		t.Fatalf("report still needs initialization: %#v", state.Report)
	}
	if !state.Locked {
		t.Fatalf("expected startup to remain locked without a healthy backend")
	}
	if state.ActiveAgent != "none" {
		t.Fatalf("active agent = %q, want none", state.ActiveAgent)
	}
	if state.ActiveTab != "doctor" {
		t.Fatalf("active tab = %q, want doctor", state.ActiveTab)
	}
}

func TestBootstrapStartupStatePreservesInitializedWorkspace(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Workspace = dir
	if err := cfg.Normalize(); err != nil {
		t.Fatalf("normalize config: %v", err)
	}
	if err := InitializeWorkspaceFromTemplates(cfg, false); err != nil {
		t.Fatalf("initialize workspace: %v", err)
	}

	state, err := BootstrapStartupState(context.Background(), cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("bootstrap startup state: %v", err)
	}
	if !state.Report.ConfigExists {
		t.Fatalf("expected initialized report, got %#v", state.Report)
	}
	if _, err := os.Stat(cfg.ConfigPath); err != nil {
		t.Fatalf("config file missing after bootstrap: %v", err)
	}
}
