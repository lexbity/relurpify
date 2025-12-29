package tui

import "time"

// InputMode tracks the role of the prompt bar.
type InputMode int

const (
	ModeNormal InputMode = iota
	ModeCommand
	ModeFilePicker
	ModeHITL
)

// Message structures mirror the specification for rendering rich agent output.
type Message struct {
	ID        string
	Timestamp time.Time
	Role      MessageRole
	Content   MessageContent
	Metadata  MessageMetadata
}

// MessageRole identifies the role of each entry in the feed.
type MessageRole string

const (
	RoleUser   MessageRole = "user"
	RoleAgent  MessageRole = "agent"
	RoleSystem MessageRole = "system"
)

// MessageContent stores the text, plan, and change information for a message.
type MessageContent struct {
	Text     string
	Thinking []ThinkingStep
	Changes  []FileChange
	Plan     *TaskPlan
	Expanded map[string]bool
}

// ThinkingStep captures an individual reasoning step emitted by the agent.
type ThinkingStep struct {
	Type        StepType
	Description string
	StartTime   time.Time
	EndTime     time.Time
	Details     []string
}

// StepType enumerates reasoning phases.
type StepType string

const (
	StepAnalyzing StepType = "analyzing"
	StepPlanning  StepType = "planning"
	StepCoding    StepType = "coding"
	StepTesting   StepType = "testing"
)

// FileChange represents a diff surfaced by the agent.
type FileChange struct {
	Path         string
	Status       ChangeStatus
	Type         ChangeType
	Diff         string
	LinesAdded   int
	LinesRemoved int
	Expanded     bool
}

// ChangeStatus tracks approval state for file changes.
type ChangeStatus string

const (
	StatusPending  ChangeStatus = "pending"
	StatusApproved ChangeStatus = "approved"
	StatusRejected ChangeStatus = "rejected"
)

// ChangeType identifies type of modification.
type ChangeType string

const (
	ChangeCreate ChangeType = "create"
	ChangeModify ChangeType = "modify"
	ChangeDelete ChangeType = "delete"
)

// TaskPlan mirrors the agent plan summary in the spec.
type TaskPlan struct {
	Tasks     []Task
	StartTime time.Time
}

// Task describes one actionable item in the plan.
type Task struct {
	Description string
	Status      TaskStatus
	StartTime   time.Time
	EndTime     time.Time
}

// TaskStatus enumerates plan state.
type TaskStatus string

const (
	TaskPending    TaskStatus = "pending"
	TaskInProgress TaskStatus = "in_progress"
	TaskCompleted  TaskStatus = "completed"
)

// MessageMetadata contains per-message metrics (duration, tokens).
type MessageMetadata struct {
	Duration    time.Duration
	TokensUsed  int
	TokensTotal int
}
