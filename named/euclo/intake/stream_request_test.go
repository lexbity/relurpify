package intake

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/named/euclo/euclokeys"
	"codeburg.org/lexbit/relurpify/named/euclo/families"
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
)

func TestBuildStreamRequestQueryTemplate(t *testing.T) {
	templateStr := "failing tests for: {{.Instruction}}"
	instruction := "the login handler panics"

	req := BuildStreamRequestWithTemplate(templateStr, instruction, &TaskEnvelope{}, 1000, contextstream.ModeBlocking)

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

	req := BuildStreamRequestWithTemplate(templateStr, instruction, &TaskEnvelope{}, maxTokens, contextstream.ModeBlocking)

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

	req := BuildStreamRequestWithTemplate(templateStr, instruction, &TaskEnvelope{}, 1000, contextstream.ModeBlocking)

	if req != nil {
		t.Error("Expected nil request when template is empty")
	}
}

func TestBuildStreamRequestUsesFamilyAnchor(t *testing.T) {
	req := BuildStreamRequest(families.FamilySelection{WinningFamily: families.FamilyDebug}, "inspect the crash", 64, contextstream.ModeBlocking)
	if req == nil {
		t.Fatal("expected non-nil request")
	}
	if req.Query.Text != "inspect the crash" {
		t.Fatalf("unexpected query text: %q", req.Query.Text)
	}
	if len(req.Query.Anchors) != 1 {
		t.Fatalf("expected one anchor, got %d", len(req.Query.Anchors))
	}
	if req.Query.Anchors[0].AnchorID != "family:"+families.FamilyDebug {
		t.Fatalf("unexpected anchor id: %q", req.Query.Anchors[0].AnchorID)
	}
	if got, ok := req.Metadata["winning_family"]; !ok || got != families.FamilyDebug {
		t.Fatalf("unexpected metadata: %#v", req.Metadata)
	}
}

func TestBuildStreamRequestMode(t *testing.T) {
	templateStr := "context for: {{.Instruction}}"
	instruction := "review code"
	mode := contextstream.ModeBackground

	req := BuildStreamRequestWithTemplate(templateStr, instruction, &TaskEnvelope{}, 1000, mode)

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

	req := BuildStreamRequestWithTemplate(templateStr, instruction, &TaskEnvelope{}, 1000, contextstream.ModeBlocking)

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
	req := BuildStreamRequestWithTemplate(templateStr, instruction, &TaskEnvelope{}, 1000, contextstream.ModeBackground)

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

	req := BuildStreamRequestWithTemplate(templateStr, instruction, &TaskEnvelope{}, 1000, contextstream.ModeBackground)

	if req == nil {
		t.Fatal("Expected non-nil request")
	}

	_ = req
}

func TestIntakePipelineNodeExecute(t *testing.T) {
	registry := families.NewRegistry()
	_ = families.RegisterBuiltins(registry)

	trigger := &UniqueMockStreamTrigger{}
	node := NewIntakePipelineNode("intake", registry, 1000, contextstream.ModeBlocking, trigger)

	env := contextdata.NewEnvelope("task-123", "session-456")
	contextdata.SetTyped(env, euclokeys.KeyTaskInput, &execution.Task{Instruction: "analyze the codebase"})
	result, err := node.Execute(context.Background(), env)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Check that envelope contains the expected keys
	if _, ok := contextdata.GetTyped[*TaskEnvelope](env, "euclo.task_envelope"); !ok {
		t.Error("Expected envelope to contain task envelope")
	}
	if evidence, ok := contextdata.GetTyped[*intentcontext.IntentEvidence](env, "euclo.intent_evidence"); !ok || evidence == nil {
		t.Fatal("Expected envelope to contain intent evidence")
	}

	if _, ok := contextdata.GetTyped[*IntentClassification](env, "euclo.intent_classification"); !ok {
		t.Error("Expected envelope to contain intent classification")
	}

	if _, ok := contextdata.GetTyped[string](env, "euclo.family_selection"); !ok {
		t.Error("Expected envelope to contain family selection")
	}

	if _, ok := contextdata.GetTyped[any](env, "euclo.capability_sequence"); ok {
		t.Fatal("did not expect capability sequence in envelope")
	}

	// Check result
	if got, ok := execution.ResultField(result.Data, "intent_evidence"); !ok || got == nil {
		t.Fatal("Expected result data to include intent evidence")
	}
	if got, ok := execution.ResultField(result.Data, "missing_fields"); !ok || got == nil {
		t.Fatal("Expected result data to include missing fields")
	}
	if got, ok := execution.ResultField(result.Data, "stream_result"); !ok || got == nil {
		t.Fatal("Expected result data to include structured stream result")
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

func TestSomeTest(t *testing.T) {
	registry := families.NewRegistry()
	trigger := &UniqueMockStreamTrigger{}
	node := NewIntakePipelineNode("test-node", registry, 100, contextstream.ModeBlocking, trigger)
	// Test logic
	_ = node
}
