package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestWriteSessionExportJSON(t *testing.T) {
	dir := t.TempDir()
	telemetryPath := filepath.Join(dir, "telemetry.jsonl")
	event := core.Event{Type: core.EventGraphStart, TaskID: "task-1", Timestamp: time.Now()}
	if err := writeTelemetryLine(telemetryPath, event); err != nil {
		t.Fatalf("write telemetry: %v", err)
	}

	sess := &Session{ID: "sess-1", StartTime: time.Now(), Workspace: dir}
	ctx := &AgentContext{Files: []string{"README.md"}}
	msgs := []Message{{
		Role:      RoleUser,
		Timestamp: time.Now(),
		Content:   MessageContent{Text: "hello"},
	}}

	out, err := WriteSessionExport(msgs, sess, ctx, ExportOptions{
		Format:        "json",
		Path:          filepath.Join(dir, "export.json"),
		WorkspaceRoot: dir,
		TelemetryPath: telemetryPath,
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("WriteSessionExport: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if !strings.Contains(string(data), "\"sess-1\"") {
		t.Fatalf("expected session id in export")
	}
	if !strings.Contains(string(data), "\"telemetry\"") {
		t.Fatalf("expected telemetry in export")
	}
}

func TestWriteSessionExportMarkdown(t *testing.T) {
	dir := t.TempDir()

	sess := &Session{ID: "sess-2", StartTime: time.Now(), Workspace: dir, Model: "llama"}
	ctx := &AgentContext{}

	out, err := WriteSessionExport(nil, sess, ctx, ExportOptions{
		Format:        "md",
		Path:          filepath.Join(dir, "export.md"),
		WorkspaceRoot: dir,
	})
	if err != nil {
		t.Fatalf("WriteSessionExport: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if !strings.Contains(string(data), "Relurpish Session Export") {
		t.Fatalf("expected markdown title")
	}
	if !strings.Contains(string(data), "sess-2") {
		t.Fatalf("expected session id in markdown")
	}
}

func TestWriteSessionExportRedactsSensitiveTelemetryAndMessageContent(t *testing.T) {
	dir := t.TempDir()
	telemetryPath := filepath.Join(dir, "telemetry.jsonl")
	event := core.Event{
		Type:      core.EventToolCall,
		TaskID:    "task-1",
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"token": "super-secret",
		},
	}
	if err := writeTelemetryLine(telemetryPath, event); err != nil {
		t.Fatalf("write telemetry: %v", err)
	}

	out, err := WriteSessionExport([]Message{{
		Role:      RoleAgent,
		Timestamp: time.Now(),
		Content:   MessageContent{Text: "Bearer secret"},
	}}, &Session{ID: "sess-3", StartTime: time.Now(), Workspace: dir}, &AgentContext{}, ExportOptions{
		Format:        "json",
		Path:          filepath.Join(dir, "export-redacted.json"),
		WorkspaceRoot: dir,
		TelemetryPath: telemetryPath,
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("WriteSessionExport: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if !strings.Contains(string(data), "[REDACTED]") {
		t.Fatalf("expected redacted marker in export")
	}
}

func writeTelemetryLine(path string, event core.Event) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	return err
}
