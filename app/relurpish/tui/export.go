package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
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
	Path      string       `json:"path,omitempty"`
	Events    []core.Event `json:"events,omitempty"`
	Truncated bool         `json:"truncated,omitempty"`
	Error     string       `json:"error,omitempty"`
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

func WriteSessionExport(m Model, opts ExportOptions) (string, error) {
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
		outPath = filepath.Join(root, "relurpify_cfg", "exports", base+"."+format)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
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
		Session:    m.session,
		Context:    m.context,
		Messages:   append([]Message(nil), m.messages...),
		LogPath:    opts.LogPath,
		Telemetry:  telemetry,
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
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func writeMarkdownExport(path string, payload SessionExport) error {
	var b strings.Builder
	b.WriteString("# Relurpish Session Export\n\n")
	if payload.Session != nil {
		b.WriteString("## Session\n")
		b.WriteString(fmt.Sprintf("- ID: %s\n", payload.Session.ID))
		b.WriteString(fmt.Sprintf("- Start: %s\n", payload.Session.StartTime.Format(time.RFC3339)))
		b.WriteString(fmt.Sprintf("- Workspace: %s\n", payload.Session.Workspace))
		b.WriteString(fmt.Sprintf("- Model: %s\n", payload.Session.Model))
		b.WriteString(fmt.Sprintf("- Agent: %s\n", payload.Session.Agent))
		if payload.Session.Mode != "" {
			b.WriteString(fmt.Sprintf("- Mode: %s\n", payload.Session.Mode))
		}
		if payload.Session.Strategy != "" {
			b.WriteString(fmt.Sprintf("- Strategy: %s\n", payload.Session.Strategy))
		}
		b.WriteString(fmt.Sprintf("- Tokens: %d\n", payload.Session.TotalTokens))
		b.WriteString(fmt.Sprintf("- Duration: %s\n", payload.Session.TotalDuration))
		b.WriteString("\n")
	}
	if payload.Context != nil {
		b.WriteString("## Context\n")
		if len(payload.Context.Files) == 0 {
			b.WriteString("- Files: (none)\n\n")
		} else {
			b.WriteString("- Files:\n")
			for _, file := range payload.Context.Files {
				b.WriteString(fmt.Sprintf("  - %s\n", file))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("## Messages\n")
	if len(payload.Messages) == 0 {
		b.WriteString("(no messages)\n")
	} else {
		for _, msg := range payload.Messages {
			b.WriteString(fmt.Sprintf("### [%s] %s\n", msg.Timestamp.Format("15:04:05"), strings.ToUpper(string(msg.Role))))
			if msg.Content.Text != "" {
				b.WriteString(msg.Content.Text + "\n")
			}
			if msg.Content.Plan != nil && len(msg.Content.Plan.Tasks) > 0 {
				b.WriteString("\nPlan:\n")
				for _, task := range msg.Content.Plan.Tasks {
					status := string(task.Status)
					b.WriteString(fmt.Sprintf("- [%s] %s\n", status, task.Description))
				}
			}
			if len(msg.Content.Changes) > 0 {
				b.WriteString("\nChanges:\n")
				for _, change := range msg.Content.Changes {
					b.WriteString(fmt.Sprintf("- %s (%s)\n", change.Path, change.Status))
				}
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("## Telemetry\n")
	if payload.Telemetry.Path != "" {
		b.WriteString(fmt.Sprintf("- Path: %s\n", payload.Telemetry.Path))
	} else {
		b.WriteString("- Path: (none)\n")
	}
	if payload.Telemetry.Error != "" {
		b.WriteString(fmt.Sprintf("- Error: %s\n", payload.Telemetry.Error))
	} else if len(payload.Telemetry.Events) > 0 {
		b.WriteString(fmt.Sprintf("- Events: %d\n", len(payload.Telemetry.Events)))
		if payload.Telemetry.Truncated {
			b.WriteString("- Note: telemetry truncated\n")
		}
	} else {
		b.WriteString("- Events: 0\n")
	}
	if payload.LogPath != "" {
		b.WriteString(fmt.Sprintf("- Log Path: %s\n", payload.LogPath))
	}

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func loadTelemetryEvents(path string, limit int) ([]core.Event, bool, error) {
	if limit <= 0 {
		limit = 200
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	var events []core.Event
	total := 0
	for scanner.Scan() {
		total++
		var event core.Event
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
