package testsuite

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/framework/persistence"
	eucloingestion "codeburg.org/lexbit/relurpify/named/euclo/ingestion"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	recipepkg "codeburg.org/lexbit/relurpify/named/euclo/recipes"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type recordingTelemetry struct {
	mu     sync.Mutex
	events []core.Event
}

func (r *recordingTelemetry) Emit(event core.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *recordingTelemetry) types() []core.EventType {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]core.EventType, 0, len(r.events))
	for _, event := range r.events {
		out = append(out, event.Type)
	}
	return out
}

type mockTier2Classifier struct {
	mu        sync.Mutex
	responses map[string]tier2Response
	calls     []tier2Call
}

type tier2Call struct {
	Instruction string
	FamilyID    string
}

type tier2Response struct {
	Sequence []string
	Operator string
}

func (m *mockTier2Classifier) Classify(ctx context.Context, instruction, familyID, streamedContext string, negativeConstraints []string) ([]string, string, error) {
	_ = ctx
	_ = streamedContext
	_ = negativeConstraints
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, tier2Call{Instruction: instruction, FamilyID: familyID})
	if resp, ok := m.responses[familyID]; ok {
		return append([]string(nil), resp.Sequence...), resp.Operator, nil
	}
	if resp, ok := m.responses["*"]; ok {
		return append([]string(nil), resp.Sequence...), resp.Operator, nil
	}
	return []string{"euclo:cap.ast_query"}, "OR", nil
}

func (m *mockTier2Classifier) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

type testCapabilityHandler struct {
	descriptor core.CapabilityDescriptor
	invoke     func(context.Context, *contextdata.Envelope, map[string]any) (*contracts.CapabilityExecutionResult, error)
}

func (h *testCapabilityHandler) Descriptor(context.Context, *contextdata.Envelope) core.CapabilityDescriptor {
	return h.descriptor
}

func (h *testCapabilityHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*contracts.CapabilityExecutionResult, error) {
	if h != nil && h.invoke != nil {
		return h.invoke(ctx, env, args)
	}
	return &contracts.CapabilityExecutionResult{
		Success: true,
		Data: map[string]any{
			"capability_id": h.descriptor.ID,
		},
	}, nil
}

func newCapabilityRegistry(t *testing.T, ids ...string) *capability.CapabilityRegistry {
	t.Helper()
	reg := capability.NewCapabilityRegistry()
	for _, id := range ids {
		if err := reg.RegisterInvocableCapability(&testCapabilityHandler{
			descriptor: core.CapabilityDescriptor{
				ID:            id,
				Name:          id,
				Kind:          core.CapabilityKindTool,
				RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
				Availability:  core.AvailabilitySpec{Available: true},
			},
			invoke: func(id string) func(context.Context, *contextdata.Envelope, map[string]any) (*contracts.CapabilityExecutionResult, error) {
				return func(context.Context, *contextdata.Envelope, map[string]any) (*contracts.CapabilityExecutionResult, error) {
					return &contracts.CapabilityExecutionResult{
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

func newRecipeRegistry(t *testing.T, recipe *recipepkg.ThoughtRecipe) *recipepkg.RecipeRegistry {
	t.Helper()
	reg := recipepkg.NewRecipeRegistry()
	if err := reg.Register(recipe); err != nil {
		t.Fatalf("register recipe: %v", err)
	}
	return reg
}

func seedTask(env *contextdata.Envelope, instruction string, userFiles ...string) *core.Task {
	task := &core.Task{
		ID:          env.TaskID,
		Type:        "euclo",
		Instruction: instruction,
		Data:        map[string]any{},
		Context:     map[string]any{},
		Metadata:    map[string]any{},
	}
	env.SetWorkingValue("task.input", task, contextdata.MemoryClassTask)
	taskEnvelope := &intake.TaskEnvelope{
		TaskID:    env.TaskID,
		SessionID: env.SessionID,
		UserFiles: append([]string(nil), userFiles...),
	}
	euclostate.SetTaskEnvelope(env, taskEnvelope)
	env.SetWorkingValue("euclo.task_envelope", taskEnvelope, contextdata.MemoryClassTask)
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
	value, ok := env.GetWorkingValue(key)
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
	value, ok := env.GetWorkingValue(key)
	if !ok {
		t.Fatalf("missing envelope value %q", key)
	}
	b, ok := value.(bool)
	if !ok {
		t.Fatalf("envelope value %q is %T, want bool", key, value)
	}
	return b
}

func assertEventOrder(t *testing.T, got []core.EventType, want []core.EventType) {
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

func workspaceEnv(reg *capability.CapabilityRegistry) agentenv.WorkspaceEnvironment {
	return agentenv.WorkspaceEnvironment{Registry: reg}
}

func writeWorkspaceFile(t *testing.T, dir, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0600); err != nil {
		t.Fatalf("write workspace file: %v", err)
	}
}

func newPersistenceWriter(t *testing.T) *persistence.Writer {
	t.Helper()
	dir := t.TempDir()
	engine, err := graphdb.Open(graphdb.DefaultOptions(filepath.Join(dir, "graphdb")))
	if err != nil {
		t.Fatalf("open graphdb: %v", err)
	}
	t.Cleanup(func() {
		_ = engine.Close()
	})
	return persistence.NewWriter(&knowledge.ChunkStore{Graph: engine}, nil, nil)
}
