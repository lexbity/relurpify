package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/telemetry"
	oteltest "codeburg.org/lexbit/relurpify/telemetry/otelleg/otlptest"
)

// exportTelemetryDir is the telemetry segment directory rendered into the export.
const exportTelemetryDir = "telemetry/otlp"

func TestWriteMarkdownExportTelemetrySections(t *testing.T) {
	tests := []struct {
		name      string
		telemetry TelemetryExport
		want      []string
		absent    []string
	}{
		{
			name:      "error takes precedence",
			telemetry: TelemetryExport{Path: exportTelemetryDir, Error: "probe failed"},
			want:      []string{"- Error: probe failed"},
			absent:    []string{"- Events:"},
		},
		{
			name: "events render counts and truncation",
			telemetry: TelemetryExport{
				Path: exportTelemetryDir,
				Events: []telemetry.Event{
					{Type: telemetry.EventType("first")},
					{Type: telemetry.EventType("second")},
				},
				Truncated: true,
			},
			want: []string{"- Events: 2", "- Note: telemetry truncated"},
		},
		{
			name:      "no events",
			telemetry: TelemetryExport{Path: exportTelemetryDir},
			want:      []string{"- Events: 0"},
			absent:    []string{"- Note: telemetry truncated"},
		},
		{
			name:      "missing telemetry path",
			telemetry: TelemetryExport{Error: "no telemetry file"},
			want:      []string{"- Path: (none)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "export.md")
			if err := writeMarkdownExport(path, SessionExport{
				ExportedAt: time.Now(),
				LogPath:    "logs/relurpish.log",
				Telemetry:  tt.telemetry,
			}); err != nil {
				t.Fatalf("writeMarkdownExport: %v", err)
			}

			body, err := os.ReadFile(path) //nolint:gosec // path is inside t.TempDir(), created by this test
			if err != nil {
				t.Fatalf("read export: %v", err)
			}
			out := string(body)
			if !strings.Contains(out, "## Telemetry") {
				t.Fatalf("export missing telemetry section:\n%s", out)
			}
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Errorf("export missing %q:\n%s", want, out)
				}
			}
			for _, absent := range tt.absent {
				if strings.Contains(out, absent) {
					t.Errorf("export unexpectedly contains %q:\n%s", absent, out)
				}
			}
		})
	}
}

func TestWriteMarkdownExportRendersSessionAndMessages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "export.md")
	payload := SessionExport{
		ExportedAt: time.Now(),
		Session: &Session{
			ID:            "session-1",
			StartTime:     time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC),
			Workspace:     "/workspace",
			Model:         "test-model",
			Agent:         SurfaceEuclo,
			Mode:          "test-mode",
			Strategy:      "default",
			TotalTokens:   42,
			TotalDuration: 3 * time.Second,
		},
		Context: &AgentContext{Files: []string{"a.go"}, Directories: nil, MaxTokens: 10, UsedTokens: 4},
		Messages: []Message{
			{
				Role:    RoleUser,
				Content: MessageContent{Text: "hello there"},
			},
			{
				Role: RoleAgent,
				Content: MessageContent{
					Text: "reply complete",
					Changes: []FileChange{{
						Path:   "a.go",
						Status: StatusApproved,
					}},
					Plan: &TaskPlan{Tasks: []Task{{Status: "succeeded", Description: "plan it"}}},
				},
			},
		},
		LogPath: "logs/relurpish.log",
	}
	if err := writeMarkdownExport(path, payload); err != nil {
		t.Fatalf("writeMarkdownExport: %v", err)
	}

	body, err := os.ReadFile(path) //nolint:gosec // path is inside t.TempDir(), created by this test
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	out := string(body)
	for _, want := range []string{
		"## Session",
		"- ID: session-1",
		"## Context",
		"  - a.go",
		"## Messages",
		"hello there",
		"reply complete",
		"Plan:",
		"plan it",
		"Changes:",
		"- a.go (",
		"- Log Path: logs/relurpish.log",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("export missing %q:\n%s", want, out)
		}
	}
}

func TestWriteMarkdownExportOmitsAbsentSections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "export.md")
	if err := writeMarkdownExport(path, SessionExport{ExportedAt: time.Now()}); err != nil {
		t.Fatalf("writeMarkdownExport: %v", err)
	}

	body, err := os.ReadFile(path) //nolint:gosec // path is inside t.TempDir(), created by this test
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	out := string(body)
	if !strings.Contains(out, "## Messages\n(no messages)\n") {
		t.Errorf("export should render the empty-messages placeholder:\n%s", out)
	}
	for _, absent := range []string{"## Session", "## Context"} {
		if strings.Contains(out, absent) {
			t.Errorf("export unexpectedly contains %q:\n%s", absent, out)
		}
	}
}

// TestLoadTelemetryEventsFromSegments proves the export's segment reader end
// to end: events written through a real OTel chain round-trip back to the
// display event shape with the correlation attributes hoisted to first-class
// fields (AC-11: the TUI's telemetry surface reads the new file leg while the
// bus feed is untouched).
func TestLoadTelemetryEventsFromSegments(t *testing.T) {
	dir := t.TempDir()
	oteltest.WriteSegments(t, dir, []telemetry.Event{
		{
			Type:      telemetry.EventType("tool_call"),
			Message:   "shell ls",
			SessionID: "sess-1",
			RunID:     "run-1",
			AgentID:   "euclo",
			Metadata:  map[string]any{"tool": "shell"},
		},
		{Type: telemetry.EventType("scheduler.tick")},
	})

	events, truncated, err := loadTelemetryEvents(dir, 10)
	if err != nil {
		t.Fatalf("loadTelemetryEvents: %v", err)
	}
	if truncated {
		t.Fatal("two events must not truncate at limit 10")
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	first := events[0]
	if first.Type != "tool_call" || first.Message != "shell ls" {
		t.Fatalf("first event = %+v", first)
	}
	if first.SessionID != "sess-1" || first.RunID != "run-1" || first.AgentID != "euclo" {
		t.Fatalf("correlation fields not hoisted: %+v", first)
	}
	if got, _ := first.Metadata["tool"].(string); got != "shell" {
		t.Fatalf("tool metadata = %q, want shell", got)
	}

	// A tight limit keeps only the tail and reports truncation.
	events, truncated, err = loadTelemetryEvents(dir, 1)
	if err != nil {
		t.Fatalf("loadTelemetryEvents: %v", err)
	}
	if !truncated || len(events) != 1 {
		t.Fatalf("limit-1 read: events=%d truncated=%v, want 1/true", len(events), truncated)
	}
}
