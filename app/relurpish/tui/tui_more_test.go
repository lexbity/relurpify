package tui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestParseExportArgs(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		format string
		path   string
	}{
		{name: "default", args: nil, format: "md"},
		{name: "markdown alias", args: []string{"markdown", "out.md"}, format: "md", path: "out.md"},
		{name: "json alias", args: []string{"json", "out.json"}, format: "json", path: "out.json"},
		{name: "explicit md file", args: []string{"report.markdown"}, format: "md", path: "report.markdown"},
		{name: "explicit json file", args: []string{"report.json"}, format: "json", path: "report.json"},
		{name: "unsupported", args: []string{"txt"}, format: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format, path := parseExportArgs(tt.args)
			require.Equal(t, tt.format, format)
			require.Equal(t, tt.path, path)
		})
	}
}

func TestTelemetryAndExportSanitizers(t *testing.T) {
	dir := t.TempDir()
	telemetryPath := filepath.Join(dir, "telemetry.log")
	lines := []string{
		`{"type":"tool_call","message":"ok","timestamp":"2026-04-08T12:00:00Z","metadata":{"secret":"token-123","safe":"value"}}`,
		`{invalid-json`,
		`{"type":"tool_result","message":"done","timestamp":"2026-04-08T12:01:00Z","metadata":{"secret":"token-456","safe":"value-2"}}`,
	}
	require.NoError(t, os.WriteFile(telemetryPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644))

	events, truncated, err := loadTelemetryEvents(telemetryPath, 1)
	require.NoError(t, err)
	require.True(t, truncated)
	require.Len(t, events, 1)
	require.Equal(t, "tool_result", string(events[0].Type))
	require.Equal(t, "value-2", events[0].Metadata["safe"])

	sanitizedTelemetry := sanitizeTelemetryExport(TelemetryExport{Path: telemetryPath, Events: events})
	require.Len(t, sanitizedTelemetry.Events, 1)
	require.Equal(t, "[REDACTED]", sanitizedTelemetry.Events[0].Metadata["secret"])

	messages := []Message{{
		ID:        "msg-1",
		Timestamp: time.Date(2026, 4, 8, 12, 2, 0, 0, time.UTC),
		Role:      RoleAgent,
		Content: MessageContent{
			Text: "token-123 should be redacted",
			Thinking: []ThinkingStep{{
				Type:        StepCoding,
				Description: "secret token-456",
				Details:     []string{"keep", "token-789"},
			}},
			Changes: []FileChange{{
				Path:   "app.go",
				Status: StatusPending,
				Type:   ChangeModify,
				Diff:   "token-abc",
			}},
			Plan: &TaskPlan{Tasks: []Task{{Description: "do the thing", Status: TaskCompleted}}},
		},
	}}
	sanitizedMessages := sanitizeMessagesForExport(messages)
	require.Equal(t, 1, len(sanitizedMessages))
	require.Equal(t, "token-123 should be redacted", sanitizedMessages[0].Content.Text)
	require.Equal(t, "secret token-456", sanitizedMessages[0].Content.Thinking[0].Description)
	require.Equal(t, "token-789", sanitizedMessages[0].Content.Thinking[0].Details[1])
	require.Equal(t, "token-abc", sanitizedMessages[0].Content.Changes[0].Diff)

	ctx := &AgentContext{Files: []string{"app.go"}}
	session := &Session{
		ID:            "sess-1",
		StartTime:     time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC),
		Workspace:     dir,
		Model:         "model-a",
		Agent:         "agent-a",
		Mode:          "debug",
		Strategy:      "react",
		TotalTokens:   1234,
		TotalDuration: 5 * time.Minute,
	}

	mdPath := filepath.Join(dir, "export.md")
	outPath, err := WriteSessionExport(messages, session, ctx, ExportOptions{
		Format:        "md",
		Path:          mdPath,
		TelemetryPath: telemetryPath,
		Limit:         1,
		LogPath:       filepath.Join(dir, "session.log"),
	})
	require.NoError(t, err)
	require.Equal(t, mdPath, outPath)

	mdData, err := os.ReadFile(mdPath)
	require.NoError(t, err)
	md := string(mdData)
	require.Contains(t, md, "# Relurpish Session Export")
	require.Contains(t, md, "## Session")
	require.Contains(t, md, "- ID: sess-1")
	require.Contains(t, md, "## Context")
	require.Contains(t, md, "- Files:")
	require.Contains(t, md, "## Messages")
	require.Contains(t, md, "do the thing")
	require.Contains(t, md, "## Telemetry")
	require.Contains(t, md, "telemetry.log")
	require.Contains(t, md, "Log Path:")

	jsonPath := filepath.Join(dir, "export.json")
	outPath, err = WriteSessionExport(messages, session, ctx, ExportOptions{
		Format: "json",
		Path:   jsonPath,
	})
	require.NoError(t, err)
	require.Equal(t, jsonPath, outPath)

	jsonData, err := os.ReadFile(jsonPath)
	require.NoError(t, err)
	var payload SessionExport
	require.NoError(t, json.Unmarshal(jsonData, &payload))
	require.Equal(t, "sess-1", payload.Session.ID)
	require.Equal(t, "app.go", payload.Context.Files[0])
	require.Len(t, payload.Messages, 1)
	require.Equal(t, "token-123 should be redacted", payload.Messages[0].Content.Text)
}

func TestBuildFileIndexAndFiltering(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "cmd"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git", "objects"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "vendor"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "relurpify_cfg"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".hidden"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("readme"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "cmd", "main.go"), []byte("package main\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".git", "objects", "skip.txt"), []byte("skip"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "vendor", "skip.txt"), []byte("skip"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "relurpify_cfg", "skip.txt"), []byte("skip"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".hidden", "skip.txt"), []byte("skip"), 0o644))

	entries, err := buildFileIndex(root)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	require.Equal(t, "README.md", entries[0].DisplayPath)
	require.Equal(t, "cmd/main.go", entries[1].DisplayPath)
	require.Contains(t, renderFileEntryLine(entries[0]), "README.md")

	filtered := filterFileEntries(entries, "main", 1)
	require.Len(t, filtered, 1)
	require.Equal(t, "cmd/main.go", filtered[0].DisplayPath)

	limited := filterFileEntries(entries, "", 1)
	require.Len(t, limited, 1)
	require.Equal(t, "README.md", limited[0].DisplayPath)
}

func TestInputBarAndPaneHelpers(t *testing.T) {
	bar := NewInputBar()
	require.Equal(t, "> ", bar.prefix(""))
	require.Equal(t, "@ ", bar.prefix(TabSession))
	require.Equal(t, "? ", bar.prefix(TabConfig))
	require.Equal(t, "✎ ", bar.prefix(TabPlanner))
	require.Equal(t, "! ", bar.prefix(TabDebug))

	bar.SetWidth(64)
	require.NotZero(t, bar.input.Width)
	bar.SetSearchMode(true)
	require.Equal(t, "/ ", bar.prefix(TabSession))
	bar.SetSearchMode(false)
	bar.SetFilePickerMode(true)
	require.Equal(t, "@", strings.TrimSpace(bar.Value()))
	bar.SetFilePickerMode(false)
	require.Empty(t, bar.Value())

	name, args := parseSlashCommand("plain text")
	require.Empty(t, name)
	require.Nil(t, args)
	cmdName, cmdArgs := parseSlashCommand("/export json out.json")
	require.Equal(t, "export", cmdName)
	require.Equal(t, []string{"json", "out.json"}, cmdArgs)
	require.True(t, isWordChar('A'))
	require.False(t, isWordChar('-'))

	bar.palOpen = true
	bar.palette = []commandItem{{Name: "run", Usage: "run", Description: "run command"}}
	open, items, sel := bar.PaletteState()
	require.True(t, open)
	require.Len(t, items, 1)
	require.Equal(t, 0, sel)
	bar.autocomplete()
	require.Equal(t, "/run", strings.TrimSpace(bar.Value()))

	pickerCmd := filePickerQueryCmd(&recordingRuntimeAdapter{}, t.TempDir(), "@app")
	msg := pickerCmd()
	require.IsType(t, filePickerResultMsg{}, msg)

	debugPane := NewDebugPane()
	require.Equal(t, SubTabDebugTest, debugPane.activeSubTab)
	_, _ = debugPane.Update(DebugTestResultMsg{Package: "pkg", Passed: 1, Failed: 0, Skipped: 0, Output: []string{"ok pkg"}})
	_, _ = debugPane.Update(DebugBenchmarkResultMsg{Package: "pkg", Results: []BenchmarkEntry{{Name: "BenchmarkX", NsPerOp: 1.5}}})
	_, _ = debugPane.Update(DebugTraceMsg{Trace: TraceInfo{Description: "trace", Frames: []TraceFrame{{FuncName: "fn", FilePath: "file.go", Line: 10}}}})
	_, _ = debugPane.Update(DebugPlanDiffMsg{Diff: PlanDiffInfo{WorkflowID: "wf-1", Steps: []PlanStepInfo{{ID: "step-1", Title: "Step 1", Status: "running", Attempts: 2}}, AnchorDrifts: []AnchorDriftInfo{{AnchorName: "anchor-1", FilePath: "file.go", Line: 7, Reason: "moved"}}}})
	_, _ = debugPane.Update(tea.KeyMsg{Type: tea.KeyEnter})
	debugPane.SetSubTab(SubTabDebugTest)
	debugPane.HandleInputSubmit("bench ./...")
	debugPane.SetSubTab(SubTabDebugTrace)
	debugPane.HandleInputSubmit("trace filter")
	debugPane.SetSubTab(SubTabDebugPlanDiff)
	debugPane.HandleInputSubmit("refresh")
	debugView := debugPane.View()
	require.Contains(t, debugView, "Plan Diff: wf-1")
	require.Contains(t, debugView, "refreshing plan diff")
	debugPane.SetSubTab(SubTabDebugBenchmark)
	require.Contains(t, debugPane.View(), "Debug: Benchmarks")
	require.Contains(t, debugPane.View(), "BenchmarkX")
	debugPane.SetSubTab(SubTabDebugTest)
	require.Contains(t, debugPane.View(), "Debug: Tests")
	require.Contains(t, debugPane.View(), "pkg")

	planner := NewPlannerPane()
	require.Equal(t, SubTabPlannerExplore, planner.activeSubTab)
	planner.Update(PlannerPatternsMsg{
		Records:   []PatternRecordInfo{{ID: "rec-1", Title: "Pattern A", Scope: "scope-a", IntentType: "intent"}},
		Proposals: []PatternProposalInfo{{ID: "prop-1", Title: "Proposal B", Scope: "scope-b", Confidence: 0.75}},
	})
	planner.Update(PlannerTensionsMsg{
		Tensions: []TensionInfo{{ID: "ten-1", TitleA: "A", TitleB: "B", Sites: []TensionSite{{FilePath: "file.go", Line: 1}}}},
		Gaps:     []IntentGapInfo{{FilePath: "file.go", Line: 2, AnchorName: "anchor-1", Severity: "high"}},
	})
	planner.Update(PlannerPlanMsg{Plan: LivePlanInfo{WorkflowID: "wf-1", Title: "Plan", Steps: []PlanStepInfo{{ID: "step-1", Title: "Step 1", Status: "ready", Notes: []string{"note"}}}}})
	planner.SetSubTab(SubTabPlannerExplore)
	planner.HandleInputSubmit("pattern")
	exploreView := planner.View()
	require.Contains(t, exploreView, "Explore")
	require.Contains(t, exploreView, "Pattern A")
	planner.SetSubTab(SubTabPlannerFinalize)
	planner.HandleInputSubmit("note for step")
	planner.Update(plannerNoteAddedMsg{stepID: "step-1", note: "note for step"})
	planner.Update(tea.KeyMsg{Type: tea.KeyEnter})
	planner.SetSubTab(SubTabPlannerAnalyze)
	planner.HandleInputSubmit("analyze")
	analyzeView := planner.View()
	require.Contains(t, analyzeView, "Analyze")
	require.Contains(t, analyzeView, "tensions: 1")
	planner.SetSubTab(SubTabPlannerFinalize)
	require.Contains(t, planner.View(), "Finalize")
	require.Contains(t, planner.View(), "wf-1")
}

type recordingRuntimeAdapter struct {
	runtimeAdapter
}

func (r *recordingRuntimeAdapter) InvokeCapability(context.Context, string, map[string]any) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true, Data: map[string]any{"stdout": "/tmp/workspace/app.go\n"}}, nil
}
