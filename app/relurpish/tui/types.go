package tui

import "time"

// TabID identifies one of the four main TUI tabs.
type TabID int

const (
	TabChat     TabID = 1
	TabTasks    TabID = 2
	TabSession  TabID = 3
	TabSettings TabID = 4
	TabTools    TabID = 5
)

// NotificationKind describes what kind of notification is being shown.
type NotificationKind string

const (
	NotifKindInfo     NotificationKind = "info"
	NotifKindHITL     NotificationKind = "hitl"
	NotifKindTaskDone NotificationKind = "task_done"
	NotifKindRestore  NotificationKind = "restore"
	NotifKindError    NotificationKind = "error"
)

// NotificationItem is a single item in the notification queue.
type NotificationItem struct {
	ID        string
	Kind      NotificationKind
	Msg       string
	Extra     map[string]string
	CreatedAt time.Time
}

// TaskItem describes a queued agent task.
type TaskItem struct {
	ID          string
	Description string
	Agent       string
	Model       string
	Status      TaskStatus
	RunID       string
}

// InputHistory provides ↑/↓ recall of submitted values.
type InputHistory struct {
	entries []string
	cursor  int
}

// Push adds an entry to the history (ignoring blanks or duplicates).
func (h *InputHistory) Push(entry string) {
	if entry == "" {
		return
	}
	if len(h.entries) > 0 && h.entries[len(h.entries)-1] == entry {
		h.cursor = len(h.entries)
		return
	}
	h.entries = append(h.entries, entry)
	h.cursor = len(h.entries)
}

// Prev moves back through history; returns empty string when exhausted.
func (h *InputHistory) Prev() string {
	if len(h.entries) == 0 {
		return ""
	}
	if h.cursor > 0 {
		h.cursor--
	}
	return h.entries[h.cursor]
}

// Next moves forward through history; returns empty string at the end.
func (h *InputHistory) Next() string {
	if h.cursor >= len(h.entries)-1 {
		h.cursor = len(h.entries)
		return ""
	}
	h.cursor++
	return h.entries[h.cursor]
}

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
	Result   *StructuredResult
	Expanded map[string]bool
}

// StructuredResult captures capability-aware output for richer rendering.
type StructuredResult struct {
	NodeID    string
	Success   bool
	Envelope  *StructuredResultEnvelope
	ErrorText string
}

// StructuredResultEnvelope stores the TUI-safe subset of a capability result envelope.
type StructuredResultEnvelope struct {
	CapabilityID   string
	CapabilityName string
	TrustClass     string
	Disposition    string
	Insertion      StructuredInsertion
	Approval       *StructuredApprovalBinding
	Blocks         []StructuredContentBlock
}

// StructuredInsertion summarizes insertion policy for display.
type StructuredInsertion struct {
	Action       string
	Reason       string
	RequiresHITL bool
}

// StructuredApprovalBinding summarizes approval linkage for display.
type StructuredApprovalBinding struct {
	CapabilityID   string
	CapabilityName string
	ProviderID     string
	SessionID      string
	TargetResource string
	TaskID         string
	WorkflowID     string
	EffectClasses  []string
}

// StructuredContentBlock is a TUI-friendly rendering block.
type StructuredContentBlock struct {
	Type       string
	Summary    string
	Body       string
	Provenance map[string]string
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
