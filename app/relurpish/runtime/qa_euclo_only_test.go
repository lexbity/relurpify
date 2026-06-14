package runtime

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/named/euclo"
	"codeburg.org/lexbit/relurpify/platform/fs"
	config "codeburg.org/lexbit/relurpify/userconfig/config"
)

type qaModel struct{}

func (qaModel) Generate(context.Context, string, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: "ok"}, nil
}

func (qaModel) GenerateStream(context.Context, string, *model.LLMOptions) (<-chan string, error) {
	ch := make(chan string, 1)
	ch <- "ok"
	close(ch)
	return ch, nil
}

func (qaModel) Chat(context.Context, []model.Message, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: "ok"}, nil
}

func (qaModel) ChatWithTools(context.Context, []model.Message, []model.LLMToolSpec, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: "ok"}, nil
}

func TestQA_StartupAgentIsEuclo(t *testing.T) {
	workspace := t.TempDir()
	copyTree(t, filepath.Join("..", "..", "..", "relurpify_cfg"), filepath.Join(workspace, "relurpify_cfg"))

	manifestPath := filepath.Join(workspace, "relurpify_cfg", "agents", "euclo.yaml")
	manifestData, err := config.ReadFileRaw(filepath.Join("..", "..", "..", "userconfig", "config", "testdata", "contracts", "document_current.yaml"))
	if err != nil {
		t.Fatalf("read manifest fixture: %v", err)
	}
	if err := fs.MkdirAllSecure(filepath.Dir(manifestPath)); err != nil {
		t.Fatalf("mkdir manifest: %v", err)
	}
	if err := fs.WriteFileSecure(manifestPath, manifestData); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	cfg := ConfigForWorkspace(Config{AgentName: "euclo"}, workspace)
	cfg.SecurityRunner = fakeCommandRunner{}
	cfg.SandboxBackendFactory = func(context.Context, string, governanceports.SandboxConfig, string, string) (governanceports.SandboxRuntime, error) {
		return &fakeSandboxRuntime{}, nil
	}

	rt, err := New(context.Background(), cfg, config.Secrets{})
	if err != nil {
		t.Fatalf("boot runtime: %v", err)
	}
	defer func() {
		if err := rt.Close(context.Background()); err != nil {
			t.Fatalf("close runtime: %v", err)
		}
	}()

	if rt.Agent == nil {
		t.Fatal("rt.Agent is nil")
	}
	if _, ok := rt.Agent.(*euclo.Agent); !ok {
		t.Fatalf("rt.Agent is %T, want *euclo.Agent", rt.Agent)
	}
}

func TestQA_NilRegistryFailsFast(t *testing.T) {
	deps := &paradigm.Deps{
		Registry: nil,
	}
	agent, err := instantiateAgent(deps)
	if err == nil {
		t.Fatal("expected error on nil registry")
	}
	if agent != nil {
		t.Fatalf("expected nil agent, got %T", agent)
	}
	if !strings.Contains(err.Error(), "registry") {
		t.Fatalf("expected error mentioning registry, got: %v", err)
	}
}

func TestQA_NilRegistryNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("instantiateAgent panicked: %v", r)
		}
	}()
	deps := &paradigm.Deps{
		Registry: nil,
	}
	_, _ = instantiateAgent(deps)
}

func TestQA_EucloExecutesEmptyRegistry(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)

	deps := &paradigm.Deps{
		Registry: registry.NewRegistry(),
		Model:    qaModel{},
	}
	agent, err := instantiateAgent(deps)
	if err != nil {
		t.Fatalf("instantiateAgent failed: %v", err)
	}

	task := &execution.Task{
		ID:          "qa-empty-reg-task",
		Instruction: "hi",
	}
	env := contextdata.NewEnvelope(task.ID, "qa-session")

	res, err := agent.Execute(context.Background(), task, env)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res == nil {
		t.Fatal("Execute returned nil result")
	}
}

func TestQA_MalformedRecipeErrorsNotSwallowed(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)

	sourceRoot := filepath.Join(tmp, "relurpify_cfg", "euclo")
	if err := fs.MkdirAllSecure(sourceRoot); err != nil {
		t.Fatalf("mkdir source root: %v", err)
	}
	if err := fs.WriteFileSecure(filepath.Join(sourceRoot, "bad.euclo"), []byte("thoughtrecipe \x00\x01 garbage\n")); err != nil {
		t.Fatalf("write malformed recipe: %v", err)
	}

	deps := &paradigm.Deps{
		Registry: registry.NewRegistry(),
		Model:    qaModel{},
	}
	agent, err := instantiateAgent(deps)
	if err != nil {
		t.Fatalf("instantiateAgent failed: %v", err)
	}

	task := &execution.Task{
		ID:          "qa-malformed-task",
		Instruction: "hi",
	}
	env := contextdata.NewEnvelope(task.ID, "qa-session")

	res, err := agent.Execute(context.Background(), task, env)
	if err == nil {
		t.Fatal("expected error with malformed recipe, but got nil")
	}
	if res != nil {
		t.Fatalf("expected nil result, got %#v", res)
	}
}
