package testsuite

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"

	"codeburg.org/lexbit/relurpify/app/relurpish/euclotui"
	runtimesvc "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

var update = flag.Bool("update", false, "update golden files")

var goldenDir = func() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("testdata", "golden")
	}
	return filepath.Join(filepath.Dir(file), "testdata", "golden")
}()

type mockOllamaBackend struct{}

func (b *mockOllamaBackend) Model() model.LanguageModel { return nil }
func (b *mockOllamaBackend) Embedder() llm.Embedder     { return nil }
func (b *mockOllamaBackend) Capabilities() model.BackendCapabilities {
	return model.BackendCapabilities{}
}
func (b *mockOllamaBackend) ModelContextSize(ctx context.Context) (int, error) { return 2048, nil }
func (b *mockOllamaBackend) Health(ctx context.Context) (*llm.HealthReport, error) {
	return &llm.HealthReport{State: llm.BackendHealthReady}, nil
}
func (b *mockOllamaBackend) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{
		{Name: "gemma4:12b", Family: "qwen"},
		{Name: "llama3:latest", Family: "llama"},
	}, nil
}
func (b *mockOllamaBackend) Warm(ctx context.Context) error                   { return nil }
func (b *mockOllamaBackend) Close() error                                     { return nil }
func (b *mockOllamaBackend) SetDebugLogging(enabled bool)                     {}
func (b *mockOllamaBackend) SetProfile(profile *model.ModelProfile)           {}
func (b *mockOllamaBackend) Reset(ctx context.Context, strategy string) error { return nil }

func init() {
	llm.RegisterKind("mock-ollama", func(cfg llm.ProviderConfig, secrets llm.ProviderSecrets) (llm.ManagedBackend, error) {
		_ = cfg
		_ = secrets
		return &mockOllamaBackend{}, nil
	})
}

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
	if err := os.MkdirAll(filepath.Dir(path), fs.PublicDirMode); err != nil { // public: golden dir
		t.Fatalf("failed to create golden dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), fs.PublicFileMode); err != nil { // public: golden fixture
		t.Fatalf("failed to write golden file %s: %v", name, err)
	}
}

func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	got = normalizeGoldenText(got)
	if *update {
		writeGolden(t, name, got)
		return
	}
	want := normalizeGoldenText(readGolden(t, name))
	if got != want {
		t.Errorf("golden mismatch for %s\n--- got:\n%s\n--- want:\n%s", name, got, want)
	}
}

var workdirRegex = regexp.MustCompile(`/tmp/.+-\s+[^/]+/TestPanelGoldenViews\d+/001/[^\s]+`)

func normalizeSnapshot(view, root string) string {
	normalized := normalizeGoldenText(view)
	if root != "" {
		normalized = strings.ReplaceAll(normalized, root, "<WORKDIR>")
	}
	return workdirRegex.ReplaceAllString(normalized, "<WORKDIR>")
}

func normalizeGoldenText(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\r\n", "\n")), " ")
}

type sandboxFixtureRuntime struct {
	workspace string
	document  *config.Document
	backend   string
}

func (r *sandboxFixtureRuntime) SessionInfo() tui.SessionInfo {
	return tui.SessionInfo{Workspace: r.workspace}
}

func (r *sandboxFixtureRuntime) LoadSandboxDocument() (*config.Document, error) {
	if r.document == nil {
		return nil, errors.New("document not set")
	}
	return cloneDocumentFixture(r.document), nil
}

func (r *sandboxFixtureRuntime) SaveSandboxDocument(doc *config.Document) (string, error) {
	r.document = doc
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

func testDocument() *config.Document {
	doc := &config.Document{
		APIVersion: "relurpify.io/v1",
		Kind:       "AgentManifest",
		Metadata:   config.DocumentMetadata{Name: "panel-agent"},
		Spec:       map[string]yaml.Node{},
	}
	var permissionsNode yaml.Node
	_ = permissionsNode.Encode(permissions.PermissionSet{
		FileSystem: []permissions.FileSystemPermission{
			{Action: permissions.FileSystemRead, Path: "/workspace/**"},
			{Action: permissions.FileSystemWrite, Path: "/workspace/**"},
		},
	})
	doc.Spec["permissions"] = permissionsNode
	var agentNode yaml.Node
	_ = agentNode.Encode(agentspec.AgentRuntimeSpec{
		Implementation: "coding",
		Mode:           agentspec.AgentModePrimary,
		Model: agentspec.AgentModelConfig{
			Provider: "mock-ollama",
			Name:     "gemma4:12b",
		},
		Bash: agentspec.AgentBashPermissions{
			AllowPatterns: []string{"git status"},
			Default:       agentspec.AgentPermissionAsk,
		},
	})
	doc.Spec["agent"] = agentNode
	return doc
}

func cloneDocumentFixture(doc *config.Document) *config.Document {
	if doc == nil {
		return nil
	}
	var cloned config.Document
	data, _ := yaml.Marshal(doc)
	_ = yaml.Unmarshal(data, &cloned)
	return &cloned
}

func withWorkingDir(t *testing.T, dir string) {
	t.Helper()
	t.Chdir(dir)
}

func deterministicTempDir(t *testing.T) string {
	baseDir := t.TempDir()
	targetLen := 120
	currentLen := len(baseDir)
	var workdir string
	if currentLen < targetLen-1 {
		subDirName := strings.Repeat("a", targetLen-currentLen-1)
		workdir = filepath.Join(baseDir, subDirName)
	} else {
		workdir = baseDir
	}
	if err := os.MkdirAll(workdir, fs.PublicDirMode); err != nil { // public: test workdir
		t.Fatalf("mkdir workdir: %v", err)
	}
	return workdir
}

func TestPanelGoldenViews(t *testing.T) {
	workdir := deterministicTempDir(t)
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

	baseWelcome := tui.NewWelcomePane(&tui.Session{}, sessionStore, nil)
	baseWelcome.SetSize(96, 18)
	assertGolden(t, "welcome_panel.txt", baseWelcome.View())

	sandboxDir := filepath.Join(workdir, "sandbox")
	if err := os.MkdirAll(filepath.Join(sandboxDir, "relurpify_cfg"), fs.PublicDirMode); err != nil { // public: sandbox config dir
		t.Fatalf("mkdir sandbox config: %v", err)
	}
	if err := config.SaveYAML(filepath.Join(sandboxDir, "relurpify_cfg", "agent.yaml"), testDocument()); err != nil {
		t.Fatalf("seed sandbox manifest: %v", err)
	}
	sandboxPane := tui.NewSandboxPane(&sandboxFixtureRuntime{
		workspace: sandboxDir,
		document:  testDocument(),
		backend:   "gvisor",
	})
	sandboxPane.SetSize(96, 20)
	assertGolden(t, "sandbox_panel.txt", sandboxPane.View())

	sec := &tui.SecurityGuardPane{}
	sec.SetTheme(theme.Default())
	sec.SetSize(96, 18)
	assertGolden(t, "securityguard_panel.txt", sec.View())

	aiProvider := tui.NewAIProviderPane(&sessionInfoFixture{workspace: workdir})
	aiProvider.SetSize(96, 18)
	providerPath := config.New(workdir).RuntimeProvidersFile()
	if err := fs.MkdirAllSecure(filepath.Dir(providerPath)); err != nil {
		t.Fatalf("mkdir provider dir: %v", err)
	}
	if err := fs.WriteFileSecure(providerPath, []byte("provider: mock-ollama\n")); err != nil {
		t.Fatalf("seed provider config: %v", err)
	}
	aiProvider.Refresh()
	tui.SetAIProviderModelsForTest(aiProvider, []llm.ModelInfo{
		{Name: "gemma4:12b", Family: "qwen"},
		{Name: "llama3:latest", Family: "llama"},
	})
	tui.SetAIProviderStatusForTest(aiProvider, "loaded 2 models")
	assertGolden(t, "aiprovider_panel.txt", aiProvider.View())

	keybinds := tui.NewKeybindingPane(&sessionInfoFixture{workspace: workdir})
	keybinds.SetSize(96, 18)
	assertGolden(t, "keybindings_panel.txt", keybinds.View())

	doctor := tui.NewDoctorPane(nil)
	doctor.SetSize(96, 18)
	doctor.SetReport(tui.DoctorReport{
		Workspace:             workdir,
		ConfigRoot:            filepath.Join(workdir, "relurpify_cfg"),
		WorkspacePresent:      true,
		ConfigExists:          true,
		ManifestExists:        true,
		ManifestFingerprint:   "sha256:demo",
		StarterTemplatesReady: true,
		Dependencies: []runtimesvc.DependencyStatus{
			{Name: "git", Available: true, Blocking: false, Details: "git available"},
			{Name: "runsc", Available: false, Blocking: true, Details: "runsc missing"},
		},
		CheckedAt: now,
	})

	chatRouter := euclotui.NewEucloEventRouter()
	chatFrame := interaction.NewClarificationFrame("task-1", "session-1", "Pick one", []string{"review", "implement"}, nil)
	chatFrame.ID = "frame-1"
	chatFrame.Metadata.Timestamp = now
	chatFrame.CreatedAt = now
	chatRouter.ApplyInteractionFrame(*chatFrame)
	chat := euclotui.NewChatPane(nil, &tui.AgentContext{}, &tui.Session{}, &tui.NotificationQueue{}, chatRouter, nil, nil)
	chat.SetSize(96, 18)
	chat.AppendMessage(tui.Message{
		ID:        "msg-1",
		Timestamp: now,
		Role:      tui.RoleSystem,
		Content:   tui.MessageContent{Text: "workspace ready"},
	})
	chat.AppendMessage(euclotui.RenderInteractionFrame(theme.Default(), *chatFrame))
	assertGolden(t, "euclo_chat_panel.txt", chat.View())

	diff := euclotui.NewDiffPane(chatRouter, sandboxDir, nil)
	diff.SetSize(96, 18)
	assertGolden(t, "euclo_diff_panel.txt", diff.View())

}

func TestRootTUIViews(t *testing.T) {
	workdir := deterministicTempDir(t)
	withWorkingDir(t, workdir)

	// Create necessary directories for runtime config
	if err := os.MkdirAll(filepath.Join(workdir, "relurpify_cfg"), fs.PublicDirMode); err != nil { // public: workdir config dir
		t.Fatalf("mkdir config: %v", err)
	}

	// Pin the AI provider to the in-process mock-ollama backend so the AI
	// provider tab renders a deterministic model list instead of probing a
	// real ollama on localhost:11434 (which makes the golden non-hermetic:
	// it passes only when no ollama is running on the dev machine).
	providerPath := config.New(workdir).RuntimeProvidersFile()
	if err := fs.MkdirAllSecure(filepath.Dir(providerPath)); err != nil {
		t.Fatalf("mkdir provider state dir: %v", err)
	}
	providerYAML := "provider: mock-ollama\nendpoint: http://mock-ollama\nmodel: gemma4:12b\ntimeout: 30s\nnative_tool_calling: true\n"
	if err := fs.WriteFileSecure(providerPath, []byte(providerYAML)); err != nil {
		t.Fatalf("write provider config: %v", err)
	}

	// 1. Integrated Welcome TUI Screen (no-agent welcome tab)
	m := tui.NewTestRootModel(nil, euclotui.NewSurfaceFactory())
	m.SetWidthHeightForTest(120, 40)
	m.SetActiveTabForTest(tui.TabWelcome)

	if m.ActiveAgentNameForTest() != "none" {
		t.Errorf("expected active agent none, got %s", m.ActiveAgentNameForTest())
	}
	assertGolden(t, "integrated_welcome_tui.txt", normalizeSnapshot(m.View(), workdir))

	// 2. Integrated Sandbox TUI Screen
	m.SetActiveTabForTest(tui.TabSandbox)
	assertGolden(t, "integrated_sandbox_tui.txt", normalizeSnapshot(m.View(), workdir))

	// 3. Integrated AI Provider TUI Screen
	m.SetActiveTabForTest(tui.TabAIProvider)
	assertGolden(t, "integrated_aiprovider_tui.txt", normalizeSnapshot(m.View(), workdir))

	// 4. Integrated Keybindings TUI Screen
	m.SetActiveTabForTest(tui.TabKeybindings)
	assertGolden(t, "integrated_keybindings_tui.txt", normalizeSnapshot(m.View(), workdir))

	// 5. Integrated HITL Row Screen (with simulated frame)
	m2 := tui.NewTestRootModel(nil, euclotui.NewSurfaceFactory())
	err := m2.SwitchActiveAgentForTest("euclo")
	if err != nil {
		t.Fatalf("switch agent: %v", err)
	}
	m2.SetWidthHeightForTest(120, 40)
	m2.SetActiveTabForTest("chat")

	now := time.Date(2020, 1, 2, 3, 4, 0, 0, time.UTC)
	frame := interaction.NewClarificationFrame("task-1", "session-1", "Clarification needed: which parser target?", []string{"parser.go", "types.go", "manual"}, nil)
	frame.ID = "frame-1"
	frame.Metadata.Timestamp = now
	frame.CreatedAt = now

	m2.OpenInteractionGuidanceForTest("frame-1", *frame)
	assertGolden(t, "integrated_hitl_row_tui.txt", normalizeSnapshot(m2.View(), workdir))
}

func TestFocusRedirectionAndAutocomplete(t *testing.T) {
	m := tui.NewTestRootModel(nil, euclotui.NewSurfaceFactory())
	m.SetWidthHeightForTest(120, 40)

	// Verify focus is initially in input
	if !m.IsFocusInInputForTest() {
		t.Fatalf("expected focus in input at startup")
	}

	// Focus Region 1
	m.SetFocusRegion1ForTest()
	if !m.IsFocusInRegion1ForTest() {
		t.Fatalf("expected focus in region 1")
	}

	// Send a printable character to trigger focus redirection
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = updated.(tui.RootModel)

	if !m.IsFocusInInputForTest() {
		t.Fatalf("expected focus to return to input on printable character")
	}
	if m.InputBarValueForTest() != "x" {
		t.Errorf("expected input value 'x', got %q", m.InputBarValueForTest())
	}

	// Test autocomplete trigger via prefix
	// Clear input first by backspacing
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = updated.(tui.RootModel)
	if m.InputBarValueForTest() != "" {
		t.Errorf("expected input to be empty after backspace, got %q", m.InputBarValueForTest())
	}

	// Type ':' to enter command mode and trigger palette sync
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	m = updated.(tui.RootModel)

	if m.OverlaysForTest().Len() != 1 {
		t.Errorf("expected 1 active overlay for autocomplete list, got %d", m.OverlaysForTest().Len())
	}
}
