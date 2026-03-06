package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

// Session tracks high-level session metadata for the status bar.
type Session struct {
	ID            string
	StartTime     time.Time
	Workspace     string
	Model         string
	Agent         string
	Role          string
	Mode          string
	Strategy      string
	TotalTokens   int
	TotalDuration time.Duration
}

// SessionInfo is a compact snapshot returned by runtime adapters.
type SessionInfo struct {
	Workspace string
	Model     string
	Agent     string
	Role      string
	Mode      string
	Strategy  string
	MaxTokens int
}

// SessionArtifacts provides file paths for logs/telemetry.
type SessionArtifacts struct {
	TelemetryPath string
	LogPath       string
}

// WorkflowInfo is a compact workflow listing for inspect and resume flows.
type WorkflowInfo struct {
	WorkflowID   string
	TaskID       string
	Status       string
	CursorStepID string
	Instruction  string
	UpdatedAt    time.Time
}

// WorkflowDetails is the expanded workflow record used by TUI actions.
type WorkflowDetails struct {
	Workflow  WorkflowInfo
	Steps     []WorkflowStepInfo
	Events    []WorkflowEventInfo
	Facts     []WorkflowKnowledgeInfo
	Issues    []WorkflowKnowledgeInfo
	Decisions []WorkflowKnowledgeInfo
}

type WorkflowStepInfo struct {
	StepID       string
	Description  string
	Status       string
	Summary      string
	Dependencies []string
}

type WorkflowEventInfo struct {
	EventType string
	StepID    string
	Message   string
	CreatedAt time.Time
}

type WorkflowKnowledgeInfo struct {
	StepID    string
	Kind      string
	Title     string
	Content   string
	Status    string
	CreatedAt time.Time
}

// AgentContext records the active context files and token budget.
type AgentContext struct {
	Files       []string
	Directories []string
	MaxTokens   int
	UsedTokens  int
}

// AddFile registers a file path with de-duplication and budget validation.
func (ac *AgentContext) AddFile(path string) error {
	if ac == nil {
		return fmt.Errorf("context unavailable")
	}
	clean := filepath.Clean(path)
	for _, existing := range ac.Files {
		if existing == clean {
			return fmt.Errorf("%s already in context", clean)
		}
	}
	ac.Files = append(ac.Files, clean)
	return nil
}

// RemoveFile removes the file from the context list if present.
func (ac *AgentContext) RemoveFile(path string) {
	if ac == nil {
		return
	}
	clean := filepath.Clean(path)
	for i, existing := range ac.Files {
		if existing == clean {
			ac.Files = append(ac.Files[:i], ac.Files[i+1:]...)
			return
		}
	}
}

// List returns a snapshot of files currently in context.
func (ac *AgentContext) List() []string {
	if ac == nil {
		return nil
	}
	out := make([]string, len(ac.Files))
	copy(out, ac.Files)
	return out
}

// ContextFileResolution captures path validation and content loading results.
type ContextFileResolution struct {
	Allowed  []string
	Contents []core.ContextFileContent
	Denied   map[string]string
}

func (r ContextFileResolution) HasErrors() bool {
	return len(r.Denied) > 0
}

func (r ContextFileResolution) ErrorLines() []string {
	if len(r.Denied) == 0 {
		return nil
	}
	lines := make([]string, 0, len(r.Denied))
	for path, reason := range r.Denied {
		lines = append(lines, fmt.Sprintf("%s: %s", path, reason))
	}
	return lines
}

func normalizePaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		clean := filepath.Clean(strings.TrimSpace(path))
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}
