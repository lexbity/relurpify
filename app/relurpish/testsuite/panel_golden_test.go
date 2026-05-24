package testsuite

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/app/relurpish/euclotui"
	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	runtimesvc "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/manifest"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	tea "github.com/charmbracelet/bubbletea"
)

var update = flag.Bool("update", false, "update golden files")

var goldenDir = func() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("testdata", "golden")
	}
	return filepath.Join(filepath.Dir(file), "testdata", "golden")
}()

func goldenPath(name string) string {
	return filepath.Join(goldenDir, name)
}

func readGolden(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(goldenPath(name))
	if err != nil {
		t.Fatalf("failed to read golden file %s: %v", name, err)
	}
	return string(data)
}

func writeGolden(t *testing.T, name, content string) {
	t.Helper()
	path := goldenPath(name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create golden dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write golden file %s: %v", name, err)
	}
}

func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	if *update {
		writeGolden(t, name, got)
		return
	}
	want := readGolden(t, name)
	if got != want {
		t.Errorf("golden mismatch for %s\n--- got:\n%s\n--- want:\n%s", name, got, want)
	}
}

func normalizeSnapshot(view, root string) string {
	if root == "" {
		return view
	}
	return strings.ReplaceAll(view, root, "<WORKDIR>")
}

type sandboxFixtureRuntime struct {
	workspace string
	manifest  *manifest.AgentManifest
	backend   string
}

func (r *sandboxFixtureRuntime) SessionInfo() tui.SessionInfo {
	return tui.SessionInfo{Workspace: r.workspace}
}

func (r *sandboxFixtureRuntime) LoadSandboxManifest() (*manifest.AgentManifest, error) {
	return manifest.CloneAgentManifest(r.manifest)
}

func (r *sandboxFixtureRuntime) SaveSandboxManifest(m *manifest.AgentManifest) (string, error) {
	r.manifest = m
	return filepath.Join(r.workspace, "relurpify_cfg", "agent.yaml.bak"), nil
}

func (r *sandboxFixtureRuntime) SandboxBackend() string {
	return r.backend
}

func (r *sandboxFixtureRuntime) SaveSandboxBackend(backend string) (string, error) {
	r.backend = strings.TrimSpace(backend)
	return filepath.Join(r.workspace, "relurpify_cfg", "config.yaml.bak"), nil
}

func (r *sandboxFixtureRuntime) ReloadWorkspace(context.Context, string) error {
	return nil
}

type sessionInfoFixture struct {
	workspace string
	provider  string
	model     string
	agent     string
}

func (f *sessionInfoFixture) SessionInfo() tui.SessionInfo {
	return tui.SessionInfo{
		Workspace: f.workspace,
		Provider:  f.provider,
		Model:     f.model,
		Agent:     f.agent,
	}
}

func (f *sessionInfoFixture) ReloadWorkspace(context.Context, string) error {
	return nil
}

func testManifest() *manifest.AgentManifest {
	return &manifest.AgentManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentManifest",
		Metadata: manifest.ManifestMetadata{
			Name:        "coding",
			Version:     "1.0.0",
			Description: "sandbox test manifest",
		},
		Spec: manifest.ManifestSpec{
			Image:   "ghcr.io/example/runtime:latest",
			Runtime: "gvisor",
			Permissions: contracts.PermissionSet{
				FileSystem: []contracts.FileSystemPermission{
					{Action: contracts.FileSystemRead, Path: "/workspace/**"},
					{Action: contracts.FileSystemWrite, Path: "/workspace/**"},
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
					Default:       agentspec.AgentPermissionAsk,
				},
			},
		},
	}
}

func withWorkingDir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(old)
	})
}

func TestPanelGoldenViews(t *testing.T) {
	workdir := t.TempDir()
	withWorkingDir(t, workdir)

	sessionStore := tui.NewSessionStore(workdir)
	now := time.Date(2020, 1, 2, 3, 4, 0, 0, time.UTC)
	for _, rec := range []tui.SessionRecord{
		{SessionMeta: tui.SessionMeta{ID: "a", Workspace: "/work/alpha", Agent: "none", Model: "m1", UpdatedAt: now.Add(-2 * time.Hour)}},
		{SessionMeta: tui.SessionMeta{ID: "b", Workspace: "/work/beta", Agent: "none", Model: "m2", UpdatedAt: now.Add(-1 * time.Hour)}},
	} {
		if err := sessionStore.Save(rec); err != nil {
			t.Fatalf("save session %q: %v", rec.ID, err)
		}
	}

	baseWelcome := tui.NewWelcomePane(&tui.Session{}, sessionStore)
	baseWelcome.SetSize(96, 18)
	assertGolden(t, "welcome_panel.txt", baseWelcome.View())

	sandboxDir := filepath.Join(workdir, "sandbox")
	if err := os.MkdirAll(filepath.Join(sandboxDir, "relurpify_cfg"), 0o755); err != nil {
		t.Fatalf("mkdir sandbox config: %v", err)
	}
	if err := manifest.SaveAgentManifest(filepath.Join(sandboxDir, "relurpify_cfg", "agent.yaml"), testManifest()); err != nil {
		t.Fatalf("seed sandbox manifest: %v", err)
	}
	sandboxPane := tui.NewSandboxPane(&sandboxFixtureRuntime{
		workspace: sandboxDir,
		manifest:  testManifest(),
		backend:   "gvisor",
	})
	sandboxPane.SetSize(96, 20)
	assertGolden(t, "sandbox_panel.txt", sandboxPane.View())

	sec := &tui.SecurityGuardPane{}
	sec.SetSize(96, 18)
	assertGolden(t, "securityguard_panel.txt", sec.View())

	keybinds := tui.NewKeybindingPane(&sessionInfoFixture{workspace: workdir})
	keybinds.SetSize(96, 18)
	assertGolden(t, "keybindings_panel.txt", keybinds.View())

	doctor := tui.NewDoctorPane(nil)
	doctor.SetSize(96, 18)
	doctor.SetReport(tui.DoctorReport{
		Workspace:            workdir,
		ConfigRoot:           filepath.Join(workdir, "relurpify_cfg"),
		WorkspacePresent:     true,
		ConfigExists:         true,
		ManifestExists:       true,
		ManifestFingerprint:  "sha256:demo",
		StarterTemplatesReady: true,
		Dependencies: []runtimesvc.DependencyStatus{
			{Name: "git", Available: true, Blocking: false, Details: "git available"},
			{Name: "runsc", Available: false, Blocking: true, Details: "runsc missing"},
		},
		CheckedAt: now,
	})
	assertGolden(t, "doctor_panel.txt", normalizeSnapshot(doctor.View(), workdir))

	chatRouter := euclotui.NewEucloEventRouter()
	chatFrame := interaction.NewClarificationFrame("task-1", "session-1", "Pick one", []string{"review", "implement"}, nil)
	chatFrame.ID = "frame-1"
	chatFrame.Metadata.Timestamp = now
	chatFrame.CreatedAt = now
	chatRouter.ApplyInteractionFrame(*chatFrame)
	chat := euclotui.NewChatPane(nil, &tui.AgentContext{}, &tui.Session{}, &tui.NotificationQueue{}, chatRouter)
	chat.SetSize(96, 18)
	chat.AppendMessage(tui.Message{
		ID:        "msg-1",
		Timestamp: now,
		Role:      tui.RoleSystem,
		Content:   tui.MessageContent{Text: "workspace ready"},
	})
	chat.AppendMessage(euclotui.RenderInteractionFrame(*chatFrame))
	assertGolden(t, "euclo_chat_panel.txt", chat.View())

	graph := euclotui.NewGraphPane(chatRouter)
	graph.SetSize(96, 18)
	graph.Update(tea.KeyMsg{Type: tea.KeyDown})
	assertGolden(t, "euclo_graph_panel.txt", graph.View())

	diff := euclotui.NewDiffPane(chatRouter, sandboxDir)
	diff.SetSize(96, 18)
	assertGolden(t, "euclo_diff_panel.txt", diff.View())

	if err := os.MkdirAll(filepath.Join(workdir, "relurpify_cfg", "euclo"), 0o755); err != nil {
		t.Fatalf("mkdir thoughtrecipe root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workdir, "relurpify_cfg", "euclo", "demo.euclo"), []byte(`thoughtrecipe demo_recipe
"Demo recipe."

trigger as capability:
  may read workspace
`), 0o644); err != nil {
		t.Fatalf("seed thoughtrecipe: %v", err)
	}
	library := euclotui.NewEucloLibraryPane(nil, chatRouter)
	library.SetSize(96, 18)
	assertGolden(t, "euclo_library_panel.txt", normalizeSnapshot(library.View(), workdir))
}
