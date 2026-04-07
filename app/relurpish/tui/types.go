package tui

import "time"

// TabID identifies one of the main TUI tabs. String-based to support
// agent-declared tabs without a central enum.
type TabID string

// Universal tab IDs — present for all agents.
const (
	TabConfig  TabID = "config"
	TabSession TabID = "session"
)

// Euclo tab IDs — registered by the euclo agent on init.
const (
	TabChat    TabID = "chat"
	TabPlanner TabID = "planner"
	TabDebug   TabID = "debug"
)

// SubTabID identifies a subtab within a main tab. Alias so string literals are
// accepted without casting in switch statements.
type SubTabID = string

// Euclo subtab IDs.
const (
	// Chat subtabs — map to euclo capability/edit policy.
	SubTabChatLocalRead  SubTabID = "local-read-only"
	SubTabChatLocalEdit  SubTabID = "local-edit-on"
	SubTabChatOnlineRead SubTabID = "online-read-on" // TODO: wire to Nexus MCP runtime
	SubTabChatOnlineEdit SubTabID = "online-edit-on" // TODO: wire to Nexus MCP runtime

	// Planner subtabs.
	SubTabPlannerExplore  SubTabID = "explore"
	SubTabPlannerAnalyze  SubTabID = "analyze"
	SubTabPlannerFinalize SubTabID = "finalize"

	// Debug subtabs.
	SubTabDebugTest      SubTabID = "test"
	SubTabDebugBenchmark SubTabID = "benchmark"
	SubTabDebugTrace     SubTabID = "trace"
	SubTabDebugPlanDiff  SubTabID = "live-plan-diff"

	// Session subtabs — universal.
	SubTabSessionLive     SubTabID = "live"
	SubTabSessionTasks    SubTabID = "tasks"
	SubTabSessionServices SubTabID = "services"
	SubTabSessionSettings SubTabID = "settings"
)

// TabDefinition declares a main tab and its subtabs.
type TabDefinition struct {
	ID          TabID
	Label       string
	SubTabs     []SubTabDefinition
	AgentFilter []string // empty = all agents; non-empty = restrict to named agents
}

// SubTabDefinition declares a subtab within a main tab.
type SubTabDefinition struct {
	ID    SubTabID
	Label string
}

// TabRegistry holds the ordered set of registered tabs and tracks active state.
// Agents register their tabs on init; the registry drives the tab bar and
// subtab bar rendering.
type TabRegistry struct {
	tabs      []TabDefinition
	active    TabID
	subActive map[TabID]SubTabID
}

// NewTabRegistry returns an empty registry.
func NewTabRegistry() *TabRegistry {
	return &TabRegistry{subActive: make(map[TabID]SubTabID)}
}

// Register appends a tab definition. The first subtab becomes the default
// active subtab for that tab if none has been set yet.
func (r *TabRegistry) Register(def TabDefinition) {
	r.tabs = append(r.tabs, def)
	if len(def.SubTabs) > 0 {
		if _, ok := r.subActive[def.ID]; !ok {
			r.subActive[def.ID] = def.SubTabs[0].ID
		}
	}
}

// ActiveTab returns the definition for the currently active tab. Returns the
// first registered tab if the active ID is not found.
func (r *TabRegistry) ActiveTab() TabDefinition {
	for _, t := range r.tabs {
		if t.ID == r.active {
			return t
		}
	}
	if len(r.tabs) > 0 {
		return r.tabs[0]
	}
	return TabDefinition{}
}

// ActiveSubTab returns the active subtab ID for the current main tab.
func (r *TabRegistry) ActiveSubTab() SubTabID {
	return r.subActive[r.active]
}

// SetActive sets the active tab.
func (r *TabRegistry) SetActive(id TabID) {
	r.active = id
}

// SetSubActive sets the active subtab for the given main tab.
func (r *TabRegistry) SetSubActive(tabID TabID, subID SubTabID) {
	r.subActive[tabID] = subID
}

// TabsForAgent returns tabs visible for the given agent name. Universal tabs
// (AgentFilter empty) are always included.
func (r *TabRegistry) TabsForAgent(agentName string) []TabDefinition {
	var out []TabDefinition
	for _, t := range r.tabs {
		if len(t.AgentFilter) == 0 {
			out = append(out, t)
			continue
		}
		for _, a := range t.AgentFilter {
			if a == agentName {
				out = append(out, t)
				break
			}
		}
	}
	return out
}

// CycleNext advances to the next tab and returns its ID.
func (r *TabRegistry) CycleNext() TabID {
	if len(r.tabs) == 0 {
		return r.active
	}
	for i, t := range r.tabs {
		if t.ID == r.active {
			next := r.tabs[(i+1)%len(r.tabs)]
			r.active = next.ID
			return next.ID
		}
	}
	r.active = r.tabs[0].ID
	return r.tabs[0].ID
}

// CyclePrev retreats to the previous tab and returns its ID.
func (r *TabRegistry) CyclePrev() TabID {
	if len(r.tabs) == 0 {
		return r.active
	}
	for i, t := range r.tabs {
		if t.ID == r.active {
			prev := r.tabs[(i-1+len(r.tabs))%len(r.tabs)]
			r.active = prev.ID
			return prev.ID
		}
	}
	r.active = r.tabs[len(r.tabs)-1].ID
	return r.tabs[len(r.tabs)-1].ID
}

// CycleSubNext advances to the next subtab of the active tab.
func (r *TabRegistry) CycleSubNext() SubTabID {
	def := r.ActiveTab()
	if len(def.SubTabs) == 0 {
		return ""
	}
	cur := r.subActive[r.active]
	for i, st := range def.SubTabs {
		if st.ID == cur {
			next := def.SubTabs[(i+1)%len(def.SubTabs)]
			r.subActive[r.active] = next.ID
			return next.ID
		}
	}
	r.subActive[r.active] = def.SubTabs[0].ID
	return def.SubTabs[0].ID
}

// CycleSubPrev retreats to the previous subtab of the active tab.
func (r *TabRegistry) CycleSubPrev() SubTabID {
	def := r.ActiveTab()
	if len(def.SubTabs) == 0 {
		return ""
	}
	cur := r.subActive[r.active]
	for i, st := range def.SubTabs {
		if st.ID == cur {
			prev := def.SubTabs[(i-1+len(def.SubTabs))%len(def.SubTabs)]
			r.subActive[r.active] = prev.ID
			return prev.ID
		}
	}
	r.subActive[r.active] = def.SubTabs[len(def.SubTabs)-1].ID
	return def.SubTabs[len(def.SubTabs)-1].ID
}

// TabAtIndex returns the tab ID at the given 0-based index. Returns the current
// active tab if the index is out of range.
func (r *TabRegistry) TabAtIndex(idx int) TabID {
	if idx < 0 || idx >= len(r.tabs) {
		return r.active
	}
	return r.tabs[idx].ID
}

// SubTabAtIndex returns the subtab ID at the given 0-based index within the
// active tab. Returns empty string if out of range.
func (r *TabRegistry) SubTabAtIndex(idx int) SubTabID {
	def := r.ActiveTab()
	if idx < 0 || idx >= len(def.SubTabs) {
		return ""
	}
	return def.SubTabs[idx].ID
}

// All returns all registered tab definitions.
func (r *TabRegistry) All() []TabDefinition {
	out := make([]TabDefinition, len(r.tabs))
	copy(out, r.tabs)
	return out
}

// Len returns the number of registered tabs.
func (r *TabRegistry) Len() int { return len(r.tabs) }

// registerEucloTabs adds the chat, planner, and debug tabs for the euclo agent.
// Called from newRootModel when the euclo agent is loaded.
func registerEucloTabs(reg *TabRegistry) {
	reg.Register(TabDefinition{
		ID: TabChat, Label: "chat", AgentFilter: []string{"euclo"},
		SubTabs: []SubTabDefinition{
			{SubTabChatLocalRead, "local-read-only"},
			{SubTabChatLocalEdit, "local-edit-on"},
			{SubTabChatOnlineRead, "online-read-on"}, // TODO: wire to Nexus MCP runtime
			{SubTabChatOnlineEdit, "online-edit-on"}, // TODO: wire to Nexus MCP runtime
		},
	})
	reg.Register(TabDefinition{
		ID: TabPlanner, Label: "planner", AgentFilter: []string{"euclo"},
		SubTabs: []SubTabDefinition{
			{SubTabPlannerExplore, "explore"},
			{SubTabPlannerAnalyze, "analyze"},
			{SubTabPlannerFinalize, "finalize"},
		},
	})
	reg.Register(TabDefinition{
		ID: TabDebug, Label: "debug", AgentFilter: []string{"euclo"},
		SubTabs: []SubTabDefinition{
			{SubTabDebugTest, "test"},
			{SubTabDebugBenchmark, "benchmark"},
			{SubTabDebugTrace, "trace"},
			{SubTabDebugPlanDiff, "live-plan-diff"},
		},
	})
	// Config and Session are registered universally in newRootModel.
}

// DiagnosticsUpdatedMsg delivers a fresh runtime diagnostics snapshot to the
// session pane's live subtab.
type DiagnosticsUpdatedMsg struct {
	Info DiagnosticsInfo
}

// SessionLiveSnapshotMsg delivers the richer runtime-backed snapshot for the
// session live subtab.
type SessionLiveSnapshotMsg struct {
	Info      DiagnosticsInfo
	Workflows []WorkflowInfo
	Providers []LiveProviderInfo
	Approvals []ApprovalInfo
}

// configRefreshMsg is an internal signal to reload config pane state from runtime.
type configRefreshMsg struct{}

// NotificationKind describes what kind of notification is being shown.
type NotificationKind string

const (
	NotifKindInfo     NotificationKind = "info"
	NotifKindDeferred NotificationKind = "deferred"
	NotifKindGuidance NotificationKind = "guidance"
	NotifKindHITL     NotificationKind = "hitl"
	NotifKindTaskDone NotificationKind = "task_done"
	NotifKindRestore  NotificationKind = "restore"
	NotifKindError    NotificationKind = "error"
	// NotifKindInteraction is declared in euclo_emitter.go to avoid a cycle.
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
	InputModeNormal      InputMode = iota // routes to active pane handler
	InputModeCommand                      // / prefix — command palette active
	InputModeFileRef                      // @ prefix — file picker overlay
	InputModeFilter                       // # prefix — in-pane filter
	InputModeHITL                         // HITL overlay has input focus
	InputModePlanNote                     // planner/explore: @step:N prefix
	InputModeCommentAuth                  // annotating a pattern; tab cycles intent type
)

// Backward-compatible aliases for code written against the old names.
const (
	ModeNormal     = InputModeNormal
	ModeCommand    = InputModeCommand
	ModeFilePicker = InputModeFileRef
	ModeHITL       = InputModeHITL
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
