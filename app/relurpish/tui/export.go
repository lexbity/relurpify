package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"codeburg.org/lexbit/relurpify/capability/runtime"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/platform/llm"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

type ExportOptions struct {
	Format        string
	Path          string
	WorkspaceRoot string
	TelemetryPath string
	LogPath       string
	Limit         int
}

type TelemetryExport struct {
	Path      string            `json:"path,omitempty"`
	Events    []telemetry.Event `json:"events,omitempty"`
	Truncated bool              `json:"truncated,omitempty"`
	Error     string            `json:"error,omitempty"`
}

type SessionExport struct {
	ExportedAt time.Time       `json:"exported_at"`
	Session    *Session        `json:"session,omitempty"`
	Context    *AgentContext   `json:"context,omitempty"`
	Messages   []Message       `json:"messages,omitempty"`
	LogPath    string          `json:"log_path,omitempty"`
	Telemetry  TelemetryExport `json:"telemetry,omitempty"`
}

func parseExportArgs(args []string) (string, string) {
	if len(args) == 0 {
		return "md", ""
	}
	first := strings.ToLower(strings.TrimSpace(args[0]))
	if first == "md" || first == "markdown" {
		if len(args) > 1 {
			return "md", args[1]
		}
		return "md", ""
	}
	if first == "json" {
		if len(args) > 1 {
			return "json", args[1]
		}
		return "json", ""
	}
	ext := strings.ToLower(filepath.Ext(first))
	if ext == ".md" || ext == ".markdown" {
		return "md", args[0]
	}
	if ext == ".json" {
		return "json", args[0]
	}
	return "", ""
}

// WriteSessionExport writes a session export to disk. The caller provides
// messages, session, and context directly to avoid coupling to a specific model type.
func WriteSessionExport(messages []Message, session *Session, ctx *AgentContext, opts ExportOptions) (string, error) {
	if opts.Format == "" {
		return "", fmt.Errorf("export format required")
	}
	format := strings.ToLower(opts.Format)
	outPath := opts.Path
	if outPath == "" {
		root := opts.WorkspaceRoot
		if root == "" {
			root = "."
		}
		base := "session-" + time.Now().Format("20060102-150405")
		outPath = filepath.Join(config.New(root).ExportsDir(), base+"."+format)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), fs.PublicDirMode); err != nil { // public: export output dir
		return "", err
	}

	telemetry := TelemetryExport{Path: opts.TelemetryPath}
	if opts.TelemetryPath != "" {
		events, truncated, err := loadTelemetryEvents(opts.TelemetryPath, opts.Limit)
		if err != nil {
			telemetry.Error = err.Error()
		} else {
			telemetry.Events = events
			telemetry.Truncated = truncated
		}
	}

	payload := SessionExport{
		ExportedAt: time.Now(),
		Session:    session,
		Context:    ctx,
		Messages:   sanitizeMessagesForExport(messages),
		LogPath:    opts.LogPath,
		Telemetry:  sanitizeTelemetryExport(telemetry),
	}

	switch format {
	case "md", "markdown":
		return outPath, writeMarkdownExport(outPath, payload)
	case "json":
		return outPath, writeJSONExport(outPath, payload)
	default:
		return "", fmt.Errorf("unsupported export format: %s", format)
	}
}

func writeJSONExport(path string, payload SessionExport) error {
	data, err := json.MarshalIndent(runtime.RedactAny(payload), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, fs.PublicFileMode) // public: JSON export
}

func writeMarkdownExport(path string, payload SessionExport) error {
	var b strings.Builder
	b.WriteString("# Relurpish Session Export\n\n")
	if payload.Session != nil {
		b.WriteString("## Session\n")
		fmt.Fprintf(&b, "- ID: %s\n", payload.Session.ID)
		fmt.Fprintf(&b, "- Start: %s\n", payload.Session.StartTime.Format(time.RFC3339))
		fmt.Fprintf(&b, "- Workspace: %s\n", payload.Session.Workspace)
		fmt.Fprintf(&b, "- Model: %s\n", payload.Session.Model)
		fmt.Fprintf(&b, "- Agent: %s\n", payload.Session.Agent)
		if payload.Session.Mode != "" {
			fmt.Fprintf(&b, "- Mode: %s\n", payload.Session.Mode)
		}
		if payload.Session.Strategy != "" {
			fmt.Fprintf(&b, "- Strategy: %s\n", payload.Session.Strategy)
		}
		fmt.Fprintf(&b, "- Tokens: %d\n", payload.Session.TotalTokens)
		fmt.Fprintf(&b, "- Duration: %s\n", payload.Session.TotalDuration)
		b.WriteString("\n")
	}
	if payload.Context != nil {
		b.WriteString("## Context\n")
		if len(payload.Context.Files) == 0 {
			b.WriteString("- Files: (none)\n\n")
		} else {
			b.WriteString("- Files:\n")
			for _, file := range payload.Context.Files {
				fmt.Fprintf(&b, "  - %s\n", file)
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("## Messages\n")
	if len(payload.Messages) == 0 {
		b.WriteString("(no messages)\n")
	} else {
		for _, msg := range payload.Messages {
			fmt.Fprintf(&b, "### [%s] %s\n", msg.Timestamp.Format("15:04:05"), strings.ToUpper(string(msg.Role)))
			if msg.Content.Text != "" {
				b.WriteString(msg.Content.Text + "\n")
			}
			if msg.Content.Plan != nil && len(msg.Content.Plan.Tasks) > 0 {
				b.WriteString("\nPlan:\n")
				for _, task := range msg.Content.Plan.Tasks {
					status := string(task.Status)
					fmt.Fprintf(&b, "- [%s] %s\n", status, task.Description)
				}
			}
			if len(msg.Content.Changes) > 0 {
				b.WriteString("\nChanges:\n")
				for _, change := range msg.Content.Changes {
					fmt.Fprintf(&b, "- %s (%s)\n", change.Path, change.Status)
				}
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("## Telemetry\n")
	if payload.Telemetry.Path != "" {
		fmt.Fprintf(&b, "- Path: %s\n", payload.Telemetry.Path)
	} else {
		b.WriteString("- Path: (none)\n")
	}
	if payload.Telemetry.Error != "" {
		fmt.Fprintf(&b, "- Error: %s\n", payload.Telemetry.Error)
	} else if len(payload.Telemetry.Events) > 0 {
		fmt.Fprintf(&b, "- Events: %d\n", len(payload.Telemetry.Events))
		if payload.Telemetry.Truncated {
			b.WriteString("- Note: telemetry truncated\n")
		}
	} else {
		b.WriteString("- Events: 0\n")
	}
	if payload.LogPath != "" {
		fmt.Fprintf(&b, "- Log Path: %s\n", payload.LogPath)
	}

	return os.WriteFile(path, []byte(b.String()), fs.PublicFileMode) // public: markdown export
}

func loadTelemetryEvents(path string, limit int) ([]telemetry.Event, bool, error) {
	if limit <= 0 {
		limit = 200
	}
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	var events []telemetry.Event
	total := 0
	for scanner.Scan() {
		total++
		var event telemetry.Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		events = append(events, event)
		if len(events) > limit {
			events = events[1:]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, false, err
	}
	truncated := total > limit
	return events, truncated, nil
}

func sanitizeTelemetryExport(in TelemetryExport) TelemetryExport {
	out := in
	if len(in.Events) == 0 {
		return out
	}
	out.Events = make([]telemetry.Event, 0, len(in.Events))
	for _, event := range in.Events {
		clone := event
		clone.Metadata = runtime.RedactMetadataMap(clone.Metadata)
		out.Events = append(out.Events, clone)
	}
	return out
}

func sanitizeMessagesForExport(messages []Message) []Message {
	if len(messages) == 0 {
		return nil
	}
	out := make([]Message, 0, len(messages))
	for _, message := range messages {
		clone := message
		clone.Content.Text = redactExportString(clone.Content.Text)
		for i := range clone.Content.Thinking {
			clone.Content.Thinking[i].Description = redactExportString(clone.Content.Thinking[i].Description)
			for j := range clone.Content.Thinking[i].Details {
				clone.Content.Thinking[i].Details[j] = redactExportString(clone.Content.Thinking[i].Details[j])
			}
		}
		for i := range clone.Content.Changes {
			clone.Content.Changes[i].Diff = redactExportString(clone.Content.Changes[i].Diff)
		}
		out = append(out, clone)
	}
	return out
}

func redactExportString(value string) string {
	redacted, ok := runtime.RedactAny(value).(string)
	if !ok {
		return value
	}
	return redacted
}

// NewTestRootModel exposes the internal newRootModel constructor for integration and golden view testing in the testsuite package.
func NewTestRootModel(rt RuntimeAdapter, factory SurfaceFactory) RootModel {
	return newRootModel(rt, factory)
}

// SetActiveTabForTest sets the active tab on RootModel for testing purposes.
func (m *RootModel) SetActiveTabForTest(id TabID) {
	m.setActiveTab(id)
}

// SetWidthHeightForTest updates the dimensions on RootModel and propagates resizing.
func (m *RootModel) SetWidthHeightForTest(w, h int) {
	updated, _ := m.handleResize(tea.WindowSizeMsg{Width: w, Height: h})
	if rm, ok := updated.(RootModel); ok {
		*m = rm
	}
}

// OpenInteractionGuidanceForTest exposes the internal openInteractionGuidance method to simulate a HITL row for golden view tests.
func (m *RootModel) OpenInteractionGuidanceForTest(notificationID string, frame interaction.InteractionFrame) {
	m.openInteractionGuidance(notificationID, frame)
}

// SetAIProviderModelsForTest sets the models list on AIProviderPane for deterministic golden tests.
func SetAIProviderModelsForTest(p *AIProviderPane, models []llm.ModelInfo) {
	p.models = models
}

// SetAIProviderStatusForTest sets the status string on AIProviderPane.
func SetAIProviderStatusForTest(p *AIProviderPane, status string) {
	p.status = status
}

// ActiveAgentNameForTest returns the active agent name.
func (m RootModel) ActiveAgentNameForTest() string {
	return m.activeAgentName()
}

// SetFocusRegion1ForTest sets focus to Region 1 for testing.
func (m *RootModel) SetFocusRegion1ForTest() {
	m.setFocus(FocusRegionRegion1)
}

// SetFocusInputForTest sets focus to Region 3 (Input Bar) for testing.
func (m *RootModel) SetFocusInputForTest() {
	m.setFocus(FocusRegionInput)
}

// IsFocusInRegion1ForTest returns true if focus is in Region 1.
func (m RootModel) IsFocusInRegion1ForTest() bool {
	return m.focus.State().InRegion1()
}

// IsFocusInInputForTest returns true if focus is in Region 3 (Input Bar).
func (m RootModel) IsFocusInInputForTest() bool {
	return m.focus.State().InInput()
}

// InputBarValueForTest returns the current value inside the universal input bar.
func (m RootModel) InputBarValueForTest() string {
	if m.inputBar != nil {
		return m.inputBar.Value()
	}
	return ""
}

// OverlaysForTest returns the internal overlays stack.
func (m RootModel) OverlaysForTest() *OverlayStack {
	return m.overlays
}

// SwitchActiveAgentForTest switches the active agent on RootModel for testing purposes.
func (m *RootModel) SwitchActiveAgentForTest(agentName string) error {
	return m.switchActiveAgent(agentName)
}
