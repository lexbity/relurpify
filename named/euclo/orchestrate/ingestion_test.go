package orchestrate

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
)

func TestIngestionNodeIngestsUserFiles(t *testing.T) {
	node := NewIngestionNode("ingestion1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	taskEnvelope := &intake.TaskEnvelope{
		TaskID:      "task-123",
		SessionID:   "session-456",
		Instruction: "fix the bug",
		UserFiles:   []string{"main.go", "utils.go"},
	}

	state.SetTaskEnvelope(env, taskEnvelope)

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if got, ok := core.ResultField(result.Data, "user_files_ingested"); !ok || got != 2 {
		t.Errorf("Expected user_files_ingested 2, got %v", got)
	}

	// Check that files were ingested to envelope
	_, ok := contextdata.GetTyped[string](env, state.KeyIngestedFilePrefix+"main.go")
	if !ok {
		t.Error("Expected main.go to be ingested")
	}

	_, ok = contextdata.GetTyped[string](env, state.KeyIngestedFilePrefix+"utils.go")
	if !ok {
		t.Error("Expected utils.go to be ingested")
	}
}

func TestIngestionNodeIngestsSessionPins(t *testing.T) {
	node := NewIngestionNode("ingestion1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	taskEnvelope := &intake.TaskEnvelope{
		TaskID:      "task-123",
		SessionID:   "session-456",
		Instruction: "fix the bug",
		SessionPins: []string{"config.yaml", "README.md"},
	}

	state.SetTaskEnvelope(env, taskEnvelope)

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if got, ok := core.ResultField(result.Data, "session_pins_ingested"); !ok || got != 2 {
		t.Errorf("Expected session_pins_ingested 2, got %v", got)
	}

	// Check that pins were ingested to envelope
	_, ok := contextdata.GetTyped[string](env, state.KeyIngestedPinPrefix+"config.yaml")
	if !ok {
		t.Error("Expected config.yaml to be ingested")
	}

	_, ok = contextdata.GetTyped[string](env, state.KeyIngestedPinPrefix+"README.md")
	if !ok {
		t.Error("Expected README.md to be ingested")
	}
}

func TestIngestionNodeHandlesEmptyLists(t *testing.T) {
	node := NewIngestionNode("ingestion1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	taskEnvelope := &intake.TaskEnvelope{
		TaskID:      "task-123",
		SessionID:   "session-456",
		Instruction: "fix the bug",
		UserFiles:   []string{},
		SessionPins: []string{},
	}

	state.SetTaskEnvelope(env, taskEnvelope)

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if got, ok := core.ResultField(result.Data, "user_files_ingested"); !ok || got != 0 {
		t.Errorf("Expected user_files_ingested 0, got %v", got)
	}

	if got, ok := core.ResultField(result.Data, "session_pins_ingested"); !ok || got != 0 {
		t.Errorf("Expected session_pins_ingested 0, got %v", got)
	}
}

func TestIngestionNodeWritesToEnvelope(t *testing.T) {
	node := NewIngestionNode("ingestion1")

	env := contextdata.NewEnvelope("task-123", "session-456")
	taskEnvelope := &intake.TaskEnvelope{
		TaskID:      "task-123",
		SessionID:   "session-456",
		Instruction: "fix the bug",
		UserFiles:   []string{"main.go"},
		SessionPins: []string{"config.yaml"},
	}

	state.SetTaskEnvelope(env, taskEnvelope)

	_, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Check ingestion metadata
	count, ok := contextdata.GetTyped[int](env, state.KeyIngestionUserFilesCount)
	if !ok {
		t.Error("Expected user_files_count in envelope")
	}

	if count != 1 {
		t.Errorf("Expected user_files_count 1, got %v", count)
	}

	count, ok = contextdata.GetTyped[int](env, state.KeyIngestionSessionPinsCount)
	if !ok {
		t.Error("Expected session_pins_count in envelope")
	}

	if count != 1 {
		t.Errorf("Expected session_pins_count 1, got %v", count)
	}

	// Check that file content is in correct format
	content, ok := contextdata.GetTyped[string](env, state.KeyIngestedFilePrefix+"main.go")
	if !ok {
		t.Error("Expected file content in envelope")
	}

	if content != "stub_ingested_content_for_main.go" {
		t.Errorf("Expected stub_ingested_content_for_main.go, got %v", content)
	}
}

func TestIngestionNodeNoTaskEnvelope(t *testing.T) {
	node := NewIngestionNode("ingestion1")

	env := contextdata.NewEnvelope("task-123", "session-456")

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Should handle missing task envelope gracefully by returning nil
	if result == nil {
		t.Fatal("Expected non-nil result when no task envelope")
	}

	if got, ok := core.ResultField(result.Data, "skipped"); !ok || got != true {
		t.Errorf("Expected skipped result when no task envelope, got %v", got)
	}
}

func TestIngestionNodeID(t *testing.T) {
	node := NewIngestionNode("ingestion1")

	if node.ID() != "ingestion1" {
		t.Errorf("Expected ID ingestion1, got %s", node.ID())
	}
}

func TestIngestionNodeType(t *testing.T) {
	node := NewIngestionNode("ingestion1")

	if node.Type() != agentgraph.NodeTypeTool {
		t.Errorf("Expected Type tool, got %s", node.Type())
	}
}
