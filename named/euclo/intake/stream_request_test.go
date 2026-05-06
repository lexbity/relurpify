package intake

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/families"
)

func TestBuildStreamRequestQueryTemplate(t *testing.T) {
	templateStr := "failing tests for: {{.Instruction}}"
	instruction := "the login handler panics"

	req := BuildStreamRequestWithTemplate(templateStr, instruction, nil, 1000, contextstream.ModeBlocking)

	if req == nil {
		t.Fatal("Expected non-nil request")
	}

	expectedQuery := "failing tests for: the login handler panics"
	if req.Query.Text != expectedQuery {
		t.Errorf("Expected query %q, got %q", expectedQuery, req.Query.Text)
	}
}

func TestBuildStreamRequestMaxTokens(t *testing.T) {
	templateStr := "context for: {{.Instruction}}"
	instruction := "fix the bug"
	maxTokens := 5000

	req := BuildStreamRequestWithTemplate(templateStr, instruction, nil, maxTokens, contextstream.ModeBlocking)

	if req == nil {
		t.Fatal("Expected non-nil request")
	}

	if req.MaxTokens != maxTokens {
		t.Errorf("Expected MaxTokens %d, got %d", maxTokens, req.MaxTokens)
	}
}

func TestBuildStreamRequestNoTemplate(t *testing.T) {
	templateStr := ""
	instruction := "do something"

	req := BuildStreamRequestWithTemplate(templateStr, instruction, nil, 1000, contextstream.ModeBlocking)

	if req != nil {
		t.Error("Expected nil request when template is empty")
	}
}

func TestBuildStreamRequestMode(t *testing.T) {
	templateStr := "context for: {{.Instruction}}"
	instruction := "review code"
	mode := contextstream.ModeBackground

	req := BuildStreamRequestWithTemplate(templateStr, instruction, nil, 1000, mode)

	if req == nil {
		t.Fatal("Expected non-nil request")
	}

	if req.Mode != mode {
		t.Errorf("Expected mode %q, got %q", mode, req.Mode)
	}
}

func TestBuildStreamRequestNoFileAnchors(t *testing.T) {
	templateStr := "context for: {{.Instruction}}"
	instruction := "fix the bug"

	req := BuildStreamRequestWithTemplate(templateStr, instruction, nil, 1000, contextstream.ModeBlocking)

	if req == nil {
		t.Fatal("Expected non-nil request")
	}

	// Verify no file parameters in Query
	if len(req.Query.Scope) > 0 {
		t.Error("Expected no file scope in query")
	}
	if len(req.Query.SourceTypes) > 0 {
		t.Error("Expected no source types in query")
	}
	if len(req.Query.Anchors) > 0 {
		t.Error("Expected no anchors in query")
	}
}

func TestBuildStreamRequestIncludesEnvelopeAnchors(t *testing.T) {
	templateStr := "context for: {{.Instruction}}"
	instruction := "fix the bug"
	env := &TaskEnvelope{
		UserFiles:   []string{"src/main.go"},
		SessionPins: []string{"README.md"},
	}

	req := BuildStreamRequestWithTemplate(templateStr, instruction, env, 1000, contextstream.ModeBlocking)
	if req == nil {
		t.Fatal("Expected non-nil request")
	}
	if len(req.Query.Anchors) != 2 {
		t.Fatalf("Expected 2 anchors, got %d", len(req.Query.Anchors))
	}
	if req.Query.Anchors[0].AnchorID != "file:src/main.go" {
		t.Fatalf("Expected first anchor to be file anchor, got %q", req.Query.Anchors[0].AnchorID)
	}
	if req.Query.Anchors[1].AnchorID != "pin:README.md" {
		t.Fatalf("Expected second anchor to be session pin anchor, got %q", req.Query.Anchors[1].AnchorID)
	}
}

func TestBuildStreamRequestBackgroundModeEnforcement(t *testing.T) {
	// This test requires BuildStreamRequest with classification source
	// For Phase 5, we'll test the mode enforcement logic separately
	// The actual enforcement will be in the pipeline node
	templateStr := "context for: {{.Instruction}}"
	instruction := "fix the bug"

	// Default mode is background
	req := BuildStreamRequestWithTemplate(templateStr, instruction, nil, 1000, contextstream.ModeBackground)

	if req == nil {
		t.Fatal("Expected non-nil request")
	}

	// Mode enforcement will be in pipeline node based on classification source
	_ = req
}

func TestBuildStreamRequestBackgroundModeSafeWithHint(t *testing.T) {
	// When ClassificationSource == "override", background mode is preserved
	// This will be tested in the pipeline node
	templateStr := "context for: {{.Instruction}}"
	instruction := "fix the bug"

	req := BuildStreamRequestWithTemplate(templateStr, instruction, nil, 1000, contextstream.ModeBackground)

	if req == nil {
		t.Fatal("Expected non-nil request")
	}

	_ = req
}

func TestIntakePipelineNodeExecute(t *testing.T) {
	registry := families.NewRegistry()
	families.RegisterBuiltins(registry)

	trigger := &UniqueMockStreamTrigger{}
	classifier := &UniqueMockCapabilityClassifier{}
	node := NewIntakePipelineNode("intake", registry, 1000, contextstream.ModeBackground, trigger, classifier)

	env := contextdata.NewEnvelope("task-123", "session-456")
	env.SetWorkingValue("task.input", &core.Task{Instruction: "analyze the codebase"}, contextdata.MemoryClassTask)
	result, err := node.Execute(context.Background(), env)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Check that envelope contains the expected keys
	if _, ok := env.GetWorkingValue("euclo.task_envelope"); !ok {
		t.Error("Expected envelope to contain task envelope")
	}

	if _, ok := env.GetWorkingValue("euclo.intent_classification"); !ok {
		t.Error("Expected envelope to contain intent classification")
	}

	if _, ok := env.GetWorkingValue("euclo.family_selection"); !ok {
		t.Error("Expected envelope to contain family selection")
	}

	// Check result
	if result.Data["winning_family"] != families.FamilyInvestigation {
		t.Errorf("Expected winning_family %q, got %q", families.FamilyInvestigation, result.Data["winning_family"])
	}
}

func TestIntakePipelineNodeWritesToTelemetry(t *testing.T) {
	// For Phase 5, telemetry emission is not yet implemented
	// This test will be added in Phase 13 when telemetry is implemented
	t.Skip("Telemetry emission will be implemented in Phase 13")
}

func TestRootGraphContainsStreamNode(t *testing.T) {
	// Root graph wiring will be implemented in Phase 14
	// This test will be added then
	t.Skip("Root graph wiring will be implemented in Phase 14")
}

func TestRootGraphSkipsStreamNodeWhenNoTemplate(t *testing.T) {
	// Root graph wiring will be implemented in Phase 14
	// This test will be added then
	t.Skip("Root graph wiring will be implemented in Phase 14")
}

type UniqueMockStreamTrigger struct{}

func (m *UniqueMockStreamTrigger) Request(ctx context.Context, req *contextstream.Request) (*contextstream.Result, error) {
	return &contextstream.Result{}, nil
}

func (m *UniqueMockStreamTrigger) RequestBlocking(ctx context.Context, req contextstream.Request) (*contextstream.Result, error) {
	return &contextstream.Result{}, nil
}

func (m *UniqueMockStreamTrigger) RequestBackground(ctx context.Context, req contextstream.Request) (*contextstream.Job, error) {
	return &contextstream.Job{}, nil
}

type UniqueMockCapabilityClassifier struct{}

func (m *UniqueMockCapabilityClassifier) Classify(ctx context.Context, instruction, familyID, streamedContext string, negativeConstraints []string) ([]string, string, error) {
	return []string{}, "", nil
}

func TestSomeTest(t *testing.T) {
	registry := families.NewRegistry()
	trigger := &UniqueMockStreamTrigger{}
	classifier := &UniqueMockCapabilityClassifier{}
	node := NewIntakePipelineNode("test-node", registry, 100, contextstream.ModeBlocking, trigger, classifier)
	// Test logic
	_ = node
}
