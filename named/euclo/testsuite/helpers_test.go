package testsuite

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/descriptor"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	registry "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
	"codeburg.org/lexbit/relurpify/context/persistence"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/model"
	eucloingestion "codeburg.org/lexbit/relurpify/named/euclo/ingestion"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	"codeburg.org/lexbit/relurpify/named/euclo/orchestrate"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
	thoughtrecipepkg "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

type stubLanguageModel struct{}

func (stubLanguageModel) Generate(context.Context, string, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}

func (stubLanguageModel) GenerateStream(context.Context, string, *model.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (stubLanguageModel) Chat(context.Context, []model.Message, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}

func (stubLanguageModel) ChatWithTools(context.Context, []model.Message, []model.LLMToolSpec, *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: `{"thought":"done","action":"complete","complete":true,"summary":"ok"}`}, nil
}

type noopCompiler struct{}

func (noopCompiler) Compile(_ context.Context, _ contextports.CompilationRequest) (*contextports.CompilationResult, error) {
	return &contextports.CompilationResult{}, nil
}

type recordingTelemetry struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (r *recordingTelemetry) Emit(event telemetry.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *recordingTelemetry) types() []telemetry.EventType {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]telemetry.EventType, 0, len(r.events))
	for _, event := range r.events {
		out = append(out, event.Type)
	}
	return out
}

type testCapabilityHandler struct {
	descriptor descriptor.CapabilityDescriptor
	invoke     func(context.Context, *contextdata.Envelope, map[string]any) (*ports.ToolResult, error)
}

func (h *testCapabilityHandler) Descriptor(context.Context, ports.State) descriptor.CapabilityDescriptor {
	return h.descriptor
}

func (h *testCapabilityHandler) Invoke(ctx context.Context, st ports.State, args map[string]any) (*ports.ToolResult, error) {
	if h != nil && h.invoke != nil {
		return h.invoke(ctx, contextdata.EnvelopeFromState(st), args)
	}
	return &ports.ToolResult{
		Success: true,
		Data: map[string]any{
			"capability_id": h.descriptor.ID,
		},
	}, nil
}

func newCapabilityRegistry(t *testing.T, ids ...string) *registry.CapabilityRegistry {
	t.Helper()
	reg := registry.NewRegistry()
	for _, id := range ids {
		if err := reg.RegisterInvocableCapability(context.Background(), &testCapabilityHandler{
			descriptor: descriptor.CapabilityDescriptor{
				ID:            id,
				Name:          id,
				Kind:          agentspec.CapabilityKindTool,
				RuntimeFamily: agentspec.CapabilityRuntimeFamilyProvider,
				Availability:  descriptor.AvailabilitySpec{Available: true},
			},
			invoke: func(id string) func(context.Context, *contextdata.Envelope, map[string]any) (*ports.ToolResult, error) {
				return func(context.Context, *contextdata.Envelope, map[string]any) (*ports.ToolResult, error) {
					return &ports.ToolResult{
						Success: true,
						Data: map[string]any{
							"capability_id": id,
							"result":        id + ":ok",
						},
					}, nil
				}
			}(id),
		}); err != nil {
			t.Fatalf("register capability %s: %v", id, err)
		}
	}
	return reg
}

func newThoughtRecipeRegistry(t *testing.T, thoughtrecipe *surface.ThoughtRecipe) *thoughtrecipepkg.ThoughtRecipeRegistry {
	t.Helper()
	reg := thoughtrecipepkg.NewThoughtRecipeRegistry()
	if thoughtrecipe == nil {
		t.Fatal("thoughtrecipe is nil")
	}
	stepID := thoughtrecipe.ID + ".step0"
	plan := &thoughtrecipepkg.ExecutionPlan{
		ThoughtRecipe: thoughtrecipe,
		Steps: []thoughtrecipepkg.ExecutionStep{{
			ID:       stepID,
			Kind:     thoughtrecipepkg.StepKindRun,
			Paradigm: "goalcon",
			Goal:     "Continue the thoughtrecipe.",
			Prompt:   "Continue the thoughtrecipe.",
			Config:   map[string]any{},
		}},
	}
	if err := reg.RegisterCompiled(thoughtrecipe, plan, "test"); err != nil {
		t.Fatalf("register compiled thoughtrecipe: %v", err)
	}
	return reg
}

func seedTask(env *contextdata.Envelope, instruction string, userFiles ...string) *execution.Task {
	task := &execution.Task{
		ID:          env.TaskID,
		Type:        "euclo",
		Instruction: instruction,
		Data:        map[string]any{},
		Context:     map[string]any{},
		Metadata:    map[string]any{},
	}
	contextdata.SetTyped(env, euclostate.KeyTaskInput, task)
	taskEnvelope := &intake.TaskEnvelope{
		TaskID:    env.TaskID,
		SessionID: env.SessionID,
		UserFiles: append([]string(nil), userFiles...),
	}
	euclostate.SetTaskEnvelope(env, taskEnvelope)
	return task
}

func runPreIngestion(t *testing.T, env *contextdata.Envelope, workspace string, userFiles []string) {
	t.Helper()
	if len(userFiles) == 0 {
		return
	}
	node := eucloingestion.NewIngestionNode("euclo.ingest", eucloingestion.IngestionSpec{
		Mode:          eucloingestion.IngestionModeFilesOnly,
		WorkspaceRoot: workspace,
	})
	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("pre-ingestion failed: %v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("pre-ingestion failed: %#v", result)
	}
}

func mustStringValue(t *testing.T, env *contextdata.Envelope, key string) string {
	t.Helper()
	value, ok := contextdata.GetTyped[any](env, key)
	if !ok {
		t.Fatalf("missing envelope value %q", key)
	}
	s, ok := value.(string)
	if !ok {
		t.Fatalf("envelope value %q is %T, want string", key, value)
	}
	return s
}

func mustBoolValue(t *testing.T, env *contextdata.Envelope, key string) bool {
	t.Helper()
	value, ok := contextdata.GetTyped[any](env, key)
	if !ok {
		t.Fatalf("missing envelope value %q", key)
	}
	b, ok := value.(bool)
	if !ok {
		t.Fatalf("envelope value %q is %T, want bool", key, value)
	}
	return b
}

func assertEventOrder(t *testing.T, got []telemetry.EventType, want []telemetry.EventType) {
	t.Helper()
	pos := 0
	for _, eventType := range got {
		if pos >= len(want) {
			break
		}
		if eventType == want[pos] {
			pos++
		}
	}
	if pos != len(want) {
		t.Fatalf("event order not observed: got=%v want=%v", got, want)
	}
}

// rootGraphDeps builds RootGraphDeps from a capability registry with a
// thoughtrecipe capability registry and a bare paradigm.Deps containing
// only the registry.
func rootGraphDeps(reg *registry.CapabilityRegistry) orchestrate.RootGraphDeps {
	return orchestrate.RootGraphDeps{
		DispatchCapabilities: reg,
		ThoughtRecipes:       thoughtrecipepkg.NewThoughtRecipeRegistry(),
		Paradigm: &paradigm.Deps{
			Registry: reg,
		},
	}
}

// rootGraphDepsWithModel builds RootGraphDeps from a registry and model,
// including a paradigm.Deps with Model and Config populated.
func rootGraphDepsWithModel(reg *registry.CapabilityRegistry, m model.LanguageModel) orchestrate.RootGraphDeps {
	return orchestrate.RootGraphDeps{
		DispatchCapabilities: reg,
		ThoughtRecipes:       thoughtrecipepkg.NewThoughtRecipeRegistry(),
		Paradigm: &paradigm.Deps{
			Model:    m,
			Registry: reg,
			Config:   &execution.Config{Name: "testsuite", Model: "stub"},
		},
	}
}

func writeWorkspaceFile(t *testing.T, dir, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
		t.Fatalf("write workspace file: %v", err)
	}
}

func newPersistenceWriter(t *testing.T) *persistence.Writer {
	t.Helper()
	dir := t.TempDir()
	engine, err := graphdb.Open(context.Background(), graphdb.DefaultOptions(filepath.Join(dir, "graphdb")))
	if err != nil {
		t.Fatalf("open graphdb: %v", err)
	}
	t.Cleanup(func() {
		_ = engine.Close(context.Background())
	})
	return persistence.NewWriter(&knowledge.ChunkStore{Graph: engine}, nil, nil, nil)
}

func ctxWithTrigger(ctx context.Context) context.Context {
	return contextstream.WithTrigger(ctx, contextstream.NewTrigger(noopCompiler{}))
}

func loadLatestCheckpointArtifact(ctx context.Context, repo *checkpointArtifactRepo, runID string) (*contextports.WorkflowArtifactRecord, error) {
	return persistence.LoadLatestCheckpointArtifact(ctx, func(runID string) ([]contextports.WorkflowArtifactRecord, error) {
		artifacts, err := repo.ListArtifactsByRun(ctx, runID)
		if err != nil {
			return nil, err
		}
		out := make([]contextports.WorkflowArtifactRecord, 0, len(artifacts))
		for _, artifact := range artifacts {
			out = append(out, contextports.WorkflowArtifactRecord{
				ArtifactID:  artifact.ArtifactID,
				WorkflowID:  artifact.WorkflowID,
				RunID:       artifact.RunID,
				ContentType: artifact.ContentType,
				StorageKind: string(artifact.StorageKind),
				Summary:     artifact.SummaryText,
				SizeBytes:   artifact.RawSizeBytes,
				CreatedAt:   artifact.CreatedAt,
				Metadata: map[string]any{
					"inline_raw": artifact.InlineRawText,
				},
			})
		}
		return out, nil
	}, runID)
}

func checkpointInlineRaw(t *testing.T, artifact *contextports.WorkflowArtifactRecord) string {
	t.Helper()
	if artifact == nil {
		t.Fatal("checkpoint artifact is nil")
	}
	raw, _ := artifact.Metadata["inline_raw"].(string)
	if raw == "" {
		t.Fatal("checkpoint artifact missing inline_raw metadata")
	}
	return raw
}
