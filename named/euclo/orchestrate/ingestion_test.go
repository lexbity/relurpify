package orchestrate

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/named/euclo/intake"
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

	env.SetWorkingValue("euclo.task.envelope", taskEnvelope, contextdata.MemoryClassTask)

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result["user_files_ingested"] != 2 {
		t.Errorf("Expected user_files_ingested 2, got %v", result["user_files_ingested"])
	}

	// Check that files were ingested to envelope
	_, ok := env.GetWorkingValue("euclo.ingested.file.main.go")
	if !ok {
		t.Error("Expected main.go to be ingested")
	}

	_, ok = env.GetWorkingValue("euclo.ingested.file.utils.go")
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

	env.SetWorkingValue("euclo.task.envelope", taskEnvelope, contextdata.MemoryClassTask)

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result["session_pins_ingested"] != 2 {
		t.Errorf("Expected session_pins_ingested 2, got %v", result["session_pins_ingested"])
	}

	// Check that pins were ingested to envelope
	_, ok := env.GetWorkingValue("euclo.ingested.pin.config.yaml")
	if !ok {
		t.Error("Expected config.yaml to be ingested")
	}

	_, ok = env.GetWorkingValue("euclo.ingested.pin.README.md")
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

	env.SetWorkingValue("euclo.task.envelope", taskEnvelope, contextdata.MemoryClassTask)

	result, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result["user_files_ingested"] != 0 {
		t.Errorf("Expected user_files_ingested 0, got %v", result["user_files_ingested"])
	}

	if result["session_pins_ingested"] != 0 {
		t.Errorf("Expected session_pins_ingested 0, got %v", result["session_pins_ingested"])
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

	env.SetWorkingValue("euclo.task.envelope", taskEnvelope, contextdata.MemoryClassTask)

	_, err := node.Execute(context.Background(), env)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Check ingestion metadata
	count, ok := env.GetWorkingValue("euclo.ingestion.user_files_count")
	if !ok {
		t.Error("Expected user_files_count in envelope")
	}

	if count != 1 {
		t.Errorf("Expected user_files_count 1, got %v", count)
	}

	count, ok = env.GetWorkingValue("euclo.ingestion.session_pins_count")
	if !ok {
		t.Error("Expected session_pins_count in envelope")
	}

	if count != 1 {
		t.Errorf("Expected session_pins_count 1, got %v", count)
	}

	// Check that file content is in correct format
	content, ok := env.GetWorkingValue("euclo.ingested.file.main.go")
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
	if result != nil {
		t.Error("Expected nil result when no task envelope")
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

	if node.Type() != "ingestion" {
		t.Errorf("Expected Type ingestion, got %s", node.Type())
	}
}
