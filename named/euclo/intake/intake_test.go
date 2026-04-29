package intake

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// === Phase 3 Unit Tests ===

func TestHintParserParse(t *testing.T) {
	parser := NewHintParser()

	message := `context-hint: typescript-react
session-hint: continue-refactoring
follow-up: add-tests
mode: architect
workspace-scope: backend, frontend
Please refactor the AuthService to use the new pattern.
Also check file: src/auth/service.ts and path: src/utils/helpers.ts
@ingest: incremental
@since: abc1234`

	result := parser.Parse(message)

	if result.ContextHint != "typescript-react" {
		t.Errorf("ContextHint = %q, want %q", result.ContextHint, "typescript-react")
	}
	if result.SessionHint != "continue-refactoring" {
		t.Errorf("SessionHint = %q, want %q", result.SessionHint, "continue-refactoring")
	}
	if result.FollowUpHint != "add-tests" {
		t.Errorf("FollowUpHint = %q, want %q", result.FollowUpHint, "add-tests")
	}
	if result.AgentModeHint != "architect" {
		t.Errorf("AgentModeHint = %q, want %q", result.AgentModeHint, "architect")
	}
	if len(result.WorkspaceScopes) != 2 {
		t.Errorf("WorkspaceScopes length = %d, want 2", len(result.WorkspaceScopes))
	}
	if result.IngestPolicy != "incremental" {
		t.Errorf("IngestPolicy = %q, want %q", result.IngestPolicy, "incremental")
	}
	if result.IncrementalSince != "abc1234" {
		t.Errorf("IncrementalSince = %q, want %q", result.IncrementalSince, "abc1234")
	}
	if len(result.ExplicitFiles) == 0 {
		t.Error("Expected at least one explicit file")
	}
}

func TestHintParserStripHints(t *testing.T) {
	parser := NewHintParser()

	message := `context-hint: typescript-react
Please refactor the AuthService to use the new pattern.`

	clean := parser.StripHints(message)

	if strings.Contains(clean, "context-hint") {
		t.Error("Clean message should not contain context-hint")
	}
	if !strings.Contains(clean, "Please refactor") {
		t.Error("Clean message should contain the actual instruction")
	}
}

func TestTaskNormalizerNormalize(t *testing.T) {
	normalizer := NewTaskNormalizer()

	message := `context-hint: go-backend
Please implement a new handler for the /api/users endpoint.`

	result := normalizer.Normalize("task-123", "session-456", message)

	if result.TaskEnvelope.TaskID != "task-123" {
		t.Errorf("TaskID = %q, want %q", result.TaskEnvelope.TaskID, "task-123")
	}
	if result.TaskEnvelope.SessionID != "session-456" {
		t.Errorf("SessionID = %q, want %q", result.TaskEnvelope.SessionID, "session-456")
	}
	if result.TaskEnvelope.ContextHint != "go-backend" {
		t.Errorf("ContextHint = %q, want %q", result.TaskEnvelope.ContextHint, "go-backend")
	}
	if result.TaskEnvelope.IngestPolicy != "full" {
		t.Errorf("IngestPolicy = %q, want %q", result.TaskEnvelope.IngestPolicy, "full")
	}
	if result.TaskEnvelope.CleanMessage == "" {
		t.Error("CleanMessage should not be empty")
	}
	if strings.Contains(result.TaskEnvelope.CleanMessage, "context-hint") {
		t.Error("CleanMessage should not contain hints")
	}
}

func TestTaskNormalizerExplicitFiles(t *testing.T) {
	normalizer := NewTaskNormalizer()

	message := `Please fix the bug in file: src/components/Button.tsx and src/styles/main.css`

	result := normalizer.Normalize("task-1", "session-1", message)

	if len(result.TaskEnvelope.ExplicitFiles) == 0 {
		t.Error("Expected explicit files to be detected")
	}
	if result.TaskEnvelope.IngestPolicy != "files_only" {
		t.Errorf("IngestPolicy = %q, want %q (inferred from explicit files)",
			result.TaskEnvelope.IngestPolicy, "files_only")
	}
}

func TestEnvelopeBuilderBuild(t *testing.T) {
	envelope, err := NewEnvelopeBuilder().
		WithTaskID("task-789").
		WithSessionID("session-abc").
		WithInstruction("Implement user authentication").
		WithContextHint("go-backend").
		WithAgentModeHint("implementer").
		WithExplicitFiles([]string{"src/auth.go"}).
		WithIngestPolicy("files_only").
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if envelope.TaskID != "task-789" {
		t.Errorf("TaskID = %q, want %q", envelope.TaskID, "task-789")
	}
	if envelope.Instruction != "Implement user authentication" {
		t.Errorf("Instruction = %q, want %q", envelope.Instruction, "Implement user authentication")
	}
	if envelope.ContextHint != "go-backend" {
		t.Errorf("ContextHint = %q, want %q", envelope.ContextHint, "go-backend")
	}
	if envelope.IngestPolicy != "files_only" {
		t.Errorf("IngestPolicy = %q, want %q", envelope.IngestPolicy, "files_only")
	}
}

func TestEnvelopeBuilderBuildMissingTaskID(t *testing.T) {
	_, err := NewEnvelopeBuilder().
		WithInstruction("Test instruction").
		Build()

	if err == nil {
		t.Error("Expected error for missing task ID")
	}
}

func TestEnvelopeBuilderBuildMissingInstruction(t *testing.T) {
	_, err := NewEnvelopeBuilder().
		WithTaskID("task-123").
		Build()

	if err == nil {
		t.Error("Expected error for missing instruction")
	}
}

func TestBuildFromTask(t *testing.T) {
	task := &core.Task{
		ID: "task-999",
		Instruction: `context-hint: typescript
Please add type safety to the API client.`,
	}

	envelope, err := BuildFromTask(task)
	if err != nil {
		t.Fatalf("BuildFromTask failed: %v", err)
	}

	if envelope.TaskID != "task-999" {
		t.Errorf("TaskID = %q, want %q", envelope.TaskID, "task-999")
	}
	if envelope.SessionID != "" {
		t.Errorf("SessionID should be empty when not set, got %q", envelope.SessionID)
	}
	if envelope.ContextHint != "typescript" {
		t.Errorf("ContextHint = %q, want %q", envelope.ContextHint, "typescript")
	}
	if envelope.CleanMessage == "" {
		t.Error("CleanMessage should be populated")
	}
	if strings.Contains(envelope.CleanMessage, "context-hint") {
		t.Error("CleanMessage should have hints stripped")
	}
}

func TestSanitizeInstruction(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  multiple   spaces   here  ", "multiple spaces here"},
		{"\t\ttabs\t\tand\t\tspaces\t\t", "tabs and spaces"},
		{"\n\nnewlines\n\nand\n\nspaces\n\n", "newlines and spaces"},
		{"normal text", "normal text"},
	}

	for _, tt := range tests {
		result := SanitizeInstruction(tt.input)
		if result != tt.expected {
			t.Errorf("SanitizeInstruction(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestTruncateInstruction(t *testing.T) {
	longText := "This is a very long instruction that needs to be truncated because it exceeds the maximum length allowed"
	truncated := TruncateInstruction(longText, 20)

	if len(truncated) > 23 { // 20 + "..."
		t.Errorf("Truncated length = %d, want <= 23", len(truncated))
	}
	if !strings.HasSuffix(truncated, "...") {
		t.Error("Truncated text should end with ...")
	}
}

func TestExtractKeywords(t *testing.T) {
	text := "Implement user authentication with JWT tokens in the Go backend"
	keywords := ExtractKeywords(text)

	expected := []string{"implement", "user", "authentication", "with", "jwt", "tokens", "the", "backend"}
	if len(keywords) < len(expected) {
		t.Errorf("Expected at least %d keywords, got %d", len(expected), len(keywords))
	}

	// Check that common words are filtered (words <= 2 chars)
	for _, kw := range keywords {
		if len(kw) <= 2 {
			t.Errorf("Keyword %q is too short and should have been filtered", kw)
		}
	}
}

func TestHintParserEmptyMessage(t *testing.T) {
	parser := NewHintParser()

	result := parser.Parse("")

	if result.ContextHint != "" {
		t.Error("Empty message should produce empty context hint")
	}
	if result.SessionHint != "" {
		t.Error("Empty message should produce empty session hint")
	}
}
