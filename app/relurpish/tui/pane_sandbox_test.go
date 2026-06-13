package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

type sandboxPaneRuntimeFake struct {
	workspace    string
	document     *config.Document
	documentPath string
	configPath   string
	backend      string
	reloads      int
}

func (f *sandboxPaneRuntimeFake) SessionInfo() SessionInfo {
	return SessionInfo{Workspace: f.workspace}
}

func (f *sandboxPaneRuntimeFake) LoadSandboxDocument() (*config.Document, error) {
	if f.document == nil {
		return nil, errors.New("document not set")
	}
	return cloneTestDocument(f.document)
}

func (f *sandboxPaneRuntimeFake) SaveSandboxDocument(doc *config.Document) (string, error) {
	if f.documentPath == "" {
		return "", os.ErrInvalid
	}
	backup, err := config.SaveDocumentWithBackup(f.documentPath, doc)
	if err != nil {
		return "", err
	}
	f.document = doc
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
	return nil
}

func testSandboxDocument(t *testing.T) *config.Document {
	t.Helper()
	doc := &config.Document{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentManifest",
		Metadata: config.DocumentMetadata{
			Name: "sandbox-agent",
		},
		Spec: make(map[string]yaml.Node),
	}
	permissionsSpec := permissions.PermissionSet{
		FileSystem: []permissions.FileSystemPermission{
			{Action: permissions.FileSystemRead, Path: "/workspace/**"},
			{Action: permissions.FileSystemWrite, Path: "/workspace/**"},
		},
		Network: []permissions.NetworkPermission{
			{Direction: "egress", Protocol: "tcp", Host: "localhost", Port: 11434},
		},
	}
	agentSpec := &agentspec.AgentRuntimeSpec{
		Implementation: "coding",
		Mode:           agentspec.AgentModePrimary,
		Model: agentspec.AgentModelConfig{
			Provider: RuntimeOllama,
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
	}
	doc.Spec["permissions"] = nodeForTest(t, permissionsSpec)
	doc.Spec["agent"] = nodeForTest(t, agentSpec)
	return doc
}

func nodeForTest(t *testing.T, value any) yaml.Node {
	t.Helper()
	var node yaml.Node
	if err := node.Encode(value); err != nil {
		t.Fatalf("encode node: %v", err)
	}
	return node
}

func cloneTestDocument(doc *config.Document) (*config.Document, error) {
	var cloned config.Document
	data, err := yaml.Marshal(doc)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, &cloned); err != nil {
		return nil, err
	}
	return &cloned, nil
}

func TestSandboxPaneCyclesAndPersists(t *testing.T) {
	dir := t.TempDir()
	documentPath := filepath.Join(dir, "relurpify_cfg", "agent.yaml")
	configPath := filepath.Join(dir, "relurpify_cfg", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(documentPath), fs.PublicDirMode); err != nil { // public: test dir
		t.Fatalf("mkdir document dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), fs.PublicDirMode); err != nil { // public: test dir
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := config.SaveYAML(documentPath, testSandboxDocument(t)); err != nil {
		t.Fatalf("seed document: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("sandbox_backend: gvisor\n"), fs.PublicFileMode); err != nil { // public: test fixture
		t.Fatalf("seed config: %v", err)
	}

	rt := &sandboxPaneRuntimeFake{
		workspace:    dir,
		document:     testSandboxDocument(t),
		documentPath: documentPath,
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
	clone, err := pane.buildSavedDocument()
	if err != nil {
		t.Fatalf("build saved document: %v", err)
	}
	permNode, ok := clone.Section("permissions")
	if !ok {
		t.Fatalf("permissions section missing from serialized document")
	}
	var ps permissions.PermissionSet
	if err := permNode.Decode(&ps); err != nil {
		t.Fatalf("decode permissions section: %v", err)
	}
	if len(ps.FileSystem) == 0 || !ps.FileSystem[0].HITLRequired {
		t.Fatalf("serialized permissions = %#v, want HITLRequired=true", ps)
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
		t.Fatal("expected document backup path")
	}
	if _, err := os.Stat(saved.Backup); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	written, err := os.ReadFile(filepath.Clean(documentPath))
	if err != nil {
		t.Fatalf("read written document: %v", err)
	}
	if !strings.Contains(string(written), "hitl_required: true") {
		t.Fatalf("written document missing ask flag:\n%s", string(written))
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
	_, _ = pane.Update(switched)
	if rt.backend != "docker" {
		t.Fatalf("backend = %q, want docker", rt.backend)
	}
	if rt.reloads < 2 {
		t.Fatalf("reload count = %d, want at least 2", rt.reloads)
	}
}
