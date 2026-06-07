package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	runtimesvc "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

type sandboxPaneRuntimeFake struct {
	workspace    string
	manifest     *config.AgentManifest
	manifestPath string
	configPath   string
	backend      string
	reloads      int
}

func (f *sandboxPaneRuntimeFake) SessionInfo() SessionInfo {
	return SessionInfo{Workspace: f.workspace}
}

func (f *sandboxPaneRuntimeFake) LoadSandboxManifest() (*config.AgentManifest, error) {
	if f.manifest == nil && f.manifestPath != "" {
		loaded, err := config.LoadAgentManifest(f.manifestPath)
		if err != nil {
			return nil, err
		}
		f.manifest = loaded
	}
	return config.CloneAgentManifest(f.manifest)
}

func (f *sandboxPaneRuntimeFake) SaveSandboxManifest(m *config.AgentManifest) (string, error) {
	if f.manifestPath == "" {
		return "", os.ErrInvalid
	}
	backup, err := runtimesvc.SaveAgentManifestWithBackup(f.manifestPath, m)
	if err != nil {
		return "", err
	}
	f.manifest = m
	return backup, nil
}

func (f *sandboxPaneRuntimeFake) SandboxBackend() string {
	return f.backend
}

func (f *sandboxPaneRuntimeFake) SaveSandboxBackend(backend string) (string, error) {
	if f.configPath == "" {
		return "", os.ErrInvalid
	}
	f.backend = strings.TrimSpace(backend)
	backup, err := config.SaveRuntimeWorkspaceConfigWithBackup(f.configPath, config.RuntimeWorkspaceConfig{
		SandboxBackend: f.backend,
		LastUpdated:    1,
	})
	if err != nil {
		return "", err
	}
	return backup, nil
}

func (f *sandboxPaneRuntimeFake) ReloadWorkspace(ctx context.Context, workspace string) error {
	_ = ctx
	f.workspace = workspace
	f.reloads++
	if f.manifestPath != "" {
		loaded, err := config.LoadAgentManifest(f.manifestPath)
		if err != nil {
			return err
		}
		f.manifest = loaded
	}
	return nil
}

func testSandboxManifest() *config.AgentManifest {
	return &config.AgentManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentManifest",
		Metadata: config.ManifestMetadata{
			Name:        "coding",
			Version:     "1.0.0",
			Description: "sandbox test manifest",
		},
		Spec: config.ManifestSpec{
			Image:   "ghcr.io/example/runtime:0.4.1",
			Runtime: "gvisor",
			Permissions: permissions.PermissionSet{
				FileSystem: []permissions.FileSystemPermission{
					{Action: permissions.FileSystemRead, Path: "/workspace/**"},
					{Action: permissions.FileSystemWrite, Path: "/workspace/**"},
				},
				Network: []permissions.NetworkPermission{
					{Direction: "egress", Protocol: "tcp", Host: "localhost", Port: 11434},
				},
			},
			Agent: &agentspec.AgentRuntimeSpec{
				Implementation: "coding",
				Mode:           agentspec.AgentModePrimary,
				Model: agentspec.AgentModelConfig{
					Provider: "ollama",
					Name:     "qwen2.5-coder:14b",
				},
				Bash: agentspec.AgentBashPermissions{
					AllowPatterns: []string{"git status"},
					DenyPatterns:  []string{"rm -rf .*"},
					Default:       agentspec.AgentPermissionAsk,
				},
				ProviderPolicies: map[string]agentspec.ProviderPolicy{
					"remote-plugin": {Activate: agentspec.AgentPermissionAsk, DefaultTrust: "remote-declared-untrusted"},
				},
				ToolExecutionPolicy: map[string]agentspec.ToolPolicy{
					"cli_mkdir": {Execute: agentspec.AgentPermissionAllow},
				},
			},
		},
	}
}

func TestSandboxPaneCyclesAndPersists(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "relurpify_cfg", "agent.yaml")
	configPath := filepath.Join(dir, "relurpify_cfg", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := config.SaveAgentManifest(manifestPath, testSandboxManifest()); err != nil {
		t.Fatalf("seed manifest: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("sandbox_backend: gvisor\n"), 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	rt := &sandboxPaneRuntimeFake{
		workspace:    dir,
		manifestPath: manifestPath,
		configPath:   configPath,
		backend:      "gvisor",
	}
	pane := NewSandboxPane(rt)
	pane.SetSize(120, 40)

	view := pane.View()
	if !strings.Contains(view, "File Scopes") || !strings.Contains(view, "/workspace/**") {
		t.Fatalf("expected file scopes in initial view, got:\n%s", view)
	}

	pane.FocusFilescopes()
	pane, _ = pane.Update(tea.KeyMsg{Type: tea.KeyDown})
	node := pane.selectedNode()
	if node == nil || node.Kind != sandboxNodeFileScope {
		t.Fatalf("selected node = %#v, want file scope", node)
	}
	pane, _ = pane.Update(tea.KeyMsg{Type: tea.KeySpace})
	node = pane.selectedNode()
	if node.State != agentspec.AgentPermissionAsk {
		t.Fatalf("file state = %s, want ask", node.State)
	}
	clone, err := pane.buildSavedManifest()
	if err != nil {
		t.Fatalf("build saved manifest: %v", err)
	}
	if clone.Spec.Policy == nil || len(clone.Spec.Policy.Permissions.FileSystem) == 0 || !clone.Spec.Policy.Permissions.FileSystem[0].HITLRequired {
		t.Fatalf("serialized policy permissions = %#v, want HITLRequired=true", clone.Spec.Policy)
	}

	pane, cmd := pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if cmd == nil {
		t.Fatal("expected save command from s")
	}
	msg := cmd()
	saved, ok := msg.(sandboxPersistedMsg)
	if !ok {
		t.Fatalf("save message type = %T, want sandboxPersistedMsg", msg)
	}
	if saved.Backup == "" {
		t.Fatal("expected manifest backup path")
	}
	if _, err := os.Stat(saved.Backup); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	written, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read written manifest: %v", err)
	}
	if !strings.Contains(string(written), "hitl_required: true") {
		t.Fatalf("written manifest missing ask flag:\n%s", string(written))
	}
	pane, _ = pane.Update(saved)
	if rt.reloads == 0 {
		t.Fatal("expected runtime reload after save")
	}
	node = pane.selectedNode()
	if node == nil || node.State != agentspec.AgentPermissionAsk {
		t.Fatalf("reloaded file state = %#v, want ask", node)
	}
	if got := pane.renderTreeLines(); !strings.Contains(got, "ask") {
		t.Fatalf("expected tree render to reflect ask state, got:\n%s", got)
	}

	pane, _ = pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if !pane.confirmBackend || pane.pendingBackend != "docker" {
		t.Fatalf("backend prompt state = confirm=%v pending=%q, want docker prompt", pane.confirmBackend, pane.pendingBackend)
	}
	pane, cmd = pane.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected backend save command from enter")
	}
	msg = cmd()
	switched, ok := msg.(sandboxPersistedMsg)
	if !ok {
		t.Fatalf("backend message type = %T, want sandboxPersistedMsg", msg)
	}
	if switched.Err != nil {
		t.Fatalf("backend toggle failed: %v", switched.Err)
	}
	pane, _ = pane.Update(switched)
	if rt.backend != "docker" {
		t.Fatalf("backend = %q, want docker", rt.backend)
	}
	if rt.reloads < 2 {
		t.Fatalf("reload count = %d, want at least 2", rt.reloads)
	}
}
