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
	Provider      string
	BackendState  string
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
	Workspace    string
	Provider     string
	BackendState string
	Model        string
	Agent        string
	Role         string
	Mode         string
	Strategy     string
	MaxTokens    int
}

// SessionArtifacts provides file paths for logs/telemetry.
type SessionArtifacts struct {
	TelemetryPath string
	LogPath       string
}

// ContractSummary exposes the resolved runtime contract for inspection.
type ContractSummary struct {
	AgentID         string
	ManifestName    string
	ManifestVersion string
	Workspace       string
	AppliedSkills   []string
	FailedSkills    []string
	CapabilityCount int
	AdmissionCount  int
	RejectedCount   int
	PolicyRuleCount int
}

// CapabilityAdmissionInfo exposes explicit admission decisions captured during bootstrap.
type CapabilityAdmissionInfo struct {
	CapabilityID   string
	CapabilityName string
	Kind           string
	Admitted       bool
	Reason         string
}

// InspectableMeta is the shared metadata subset used by TUI inspection models.
type InspectableMeta struct {
	ID            string
	Kind          string
	Title         string
	RuntimeFamily string
	TrustClass    string
	Scope         string
	Source        string
	State         string
	CapturedAt    string
}

// LiveProviderInfo exposes current runtime provider state.
type LiveProviderInfo struct {
	Meta           InspectableMeta
	ProviderID     string
	Kind           string
	TrustBaseline  string
	Recoverability string
	ConfiguredFrom string
	CapabilityIDs  []string
}

// LiveProviderSessionInfo exposes current runtime provider-session state.
type LiveProviderSessionInfo struct {
	Meta            InspectableMeta
	SessionID       string
	ProviderID      string
	WorkflowID      string
	TaskID          string
	TrustClass      string
	Recoverability  string
	Health          string
	CapabilityIDs   []string
	LastActivityAt  string
	MetadataSummary []string
}

// ApprovalInfo exposes pending HITL approvals through a unified UI model.
type ApprovalInfo struct {
	Meta           InspectableMeta
	ID             string
	Kind           string
	PermissionType string
	Action         string
	Resource       string
	Risk           string
	Scope          string
	Justification  string
	RequestedAt    time.Time
	Metadata       map[string]string
}

// CapabilityDetail is the richer capability inspection model used by the config pane.
type CapabilityDetail struct {
	Meta                  InspectableMeta
	Description           string
	Category              string
	Exposure              string
	Callable              bool
	ProviderID            string
	SessionAffinity       string
	Availability          string
	RiskClasses           []string
	EffectClasses         []string
	Tags                  []string
	CoordinationRole      string
	CoordinationTaskTypes []string
}

// PromptInfo exposes prompt capabilities through a dedicated browser.
type PromptInfo struct {
	Meta       InspectableMeta
	PromptID   string
	ProviderID string
}

// PromptDetail is the richer prompt inspection model.
type PromptDetail struct {
	Meta        InspectableMeta
	PromptID    string
	ProviderID  string
	Description string
	Messages    []StructuredPromptMessage
	Metadata    []string
}

// StructuredPromptMessage is a rendered prompt message for browsing.
type StructuredPromptMessage struct {
	Role    string
	Content []StructuredContentBlock
}

// ResourceInfo exposes resource capabilities and workflow resources.
type ResourceInfo struct {
	Meta             InspectableMeta
	ResourceID       string
	ProviderID       string
	WorkflowResource bool
	WorkflowURI      string
}

// ResourceDetail is the richer resource inspection model.
type ResourceDetail struct {
	Meta             InspectableMeta
	ResourceID       string
	ProviderID       string
	Description      string
	WorkflowResource bool
	WorkflowURI      string
	Contents         []StructuredContentBlock
	Metadata         []string
}

// LiveProviderDetail is the richer live-provider inspection model.
type LiveProviderDetail struct {
	Meta           InspectableMeta
	ProviderID     string
	Kind           string
	TrustBaseline  string
	Recoverability string
	ConfiguredFrom string
	CapabilityIDs  []string
	Metadata       []string
}

// LiveProviderSessionDetail is the richer live-session inspection model.
type LiveProviderSessionDetail struct {
	Meta            InspectableMeta
	SessionID       string
	ProviderID      string
	WorkflowID      string
	TaskID          string
	Recoverability  string
	CapabilityIDs   []string
	LastActivityAt  string
	MetadataSummary []string
}

// ApprovalDetail is the richer approval inspection model.
type ApprovalDetail struct {
	Meta           InspectableMeta
	ID             string
	Kind           string
	PermissionType string
	Action         string
	Resource       string
	Risk           string
	Scope          string
	Justification  string
	RequestedAt    time.Time
	Metadata       map[string]string
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
	Workflow          WorkflowInfo
	Steps             []WorkflowStepInfo
	Events            []WorkflowEventInfo
	Facts             []WorkflowKnowledgeInfo
	Issues            []WorkflowKnowledgeInfo
	Decisions         []WorkflowKnowledgeInfo
	Delegations       []WorkflowDelegationInfo
	Transitions       []WorkflowDelegationTransitionInfo
	WorkflowArtifacts []WorkflowArtifactInfo
	LinkedResources   []string
	Providers         []WorkflowProviderInfo
	ProviderSessions  []WorkflowProviderSessionInfo
	ResourceDetails   []WorkflowLinkedResourceInfo
}

type WorkflowLinkedResourceInfo struct {
	URI     string
	Tier    string
	Role    string
	RunID   string
	StepID  string
	Summary string
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

type WorkflowDelegationInfo struct {
	DelegationID       string
	RunID              string
	TaskID             string
	State              string
	TargetCapabilityID string
	TargetProviderID   string
	TargetSessionID    string
	TrustClass         string
	Recoverability     string
	Background         bool
	StartedAt          time.Time
	UpdatedAt          time.Time
	InsertionAction    string
	ResourceRefs       []string
}

type WorkflowDelegationTransitionInfo struct {
	DelegationID string
	TransitionID string
	RunID        string
	FromState    string
	ToState      string
	CreatedAt    time.Time
}

type WorkflowArtifactInfo struct {
	ArtifactID  string
	RunID       string
	Kind        string
	ContentType string
	SummaryText string
	CreatedAt   time.Time
}

type WorkflowProviderInfo struct {
	SnapshotID     string
	RunID          string
	ProviderID     string
	Kind           string
	Recoverability string
	Health         string
	CapturedAt     time.Time
}

type WorkflowProviderSessionInfo struct {
	SnapshotID     string
	RunID          string
	SessionID      string
	ProviderID     string
	Health         string
	Recoverability string
	CapturedAt     time.Time
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

// ContextSidebarEntry represents a file in the chat context sidebar
type ContextSidebarEntry struct {
	Path            string
	InsertionAction string // "direct" | "summarized" | "metadata-only"
	IsPin           bool   // session pin vs per-turn confirmed
}

// ServiceInfo represents a service in the ayenitd service manager
type ServiceInfo struct {
	ID     string
	Status ServiceStatus // Running | Stopped | Error
}

type ServiceStatus string

const (
	ServiceStatusRunning ServiceStatus = "running"
	ServiceStatusStopped ServiceStatus = "stopped"
	ServiceStatusError   ServiceStatus = "error"
)

// ActivePlanView represents a view of the active living plan
type ActivePlanView struct {
	WorkflowID string
	Title      string
	Steps      []PlanStepInfo
	UpdatedAt  time.Time
}

// BlobEntry represents a tension, pattern, or learning item
type BlobEntry struct {
	ID          string
	Kind        BlobKind // BlobTension | BlobPattern | BlobLearning
	Title       string
	Description string
	Severity    string // for tensions: high/med/low
	Status      string // for tensions: active/accepted/resolved
	InPlan      bool
	AnchorRefs  []string
	StepID      string // set when InPlan is true
}

type BlobKind string

const (
	BlobTension  BlobKind = "tension"
	BlobPattern  BlobKind = "pattern"
	BlobLearning BlobKind = "learning"
)

// ─── TUI-safe data types added for the rework (Part 10 / plan-06) ────────────

// CommentRef is a lightweight reference to a pattern comment surfaced in the
// guidance panel alongside ambiguity questions.
type CommentRef struct {
	PatternTitle string
	IntentType   string
	Body         string
	CreatedAt    string
}

// PatternRef is a compact reference to a known pattern.
type PatternRef struct {
	ID    string
	Title string
	Scope string
}

// AnchorRef is a compact reference to an anchor dependency on a plan step.
type AnchorRef struct {
	Name   string
	Class  string // commitment | policy | technical
	Status string // active | drifted | proposed
}

// SymbolChange describes a symbol modification within a simulation or plan diff.
type SymbolChange struct {
	SymbolID   string
	FilePath   string
	ChangeKind string // add | modify | remove
}

// TensionSite describes one location where two patterns conflict.
type TensionSite struct {
	FilePath string
	Line     int
	PatternA string // how pattern A manifests here
	PatternB string // how pattern B manifests here
}

// TensionInfo describes a structural tension between two confirmed patterns.
type TensionInfo struct {
	ID                 string
	PatternAID         string
	PatternBID         string
	TitleA             string
	TitleB             string
	Sites              []TensionSite
	ResolutionPatterns []PatternRef
}

// SimulationResult captures the blast radius of adopting a pattern.
type SimulationResult struct {
	PatternID        string
	DirectImpact     int
	IndirectImpact   int
	FilesAffected    int
	SymbolChanges    []SymbolChange
	PatternsAdapting []PatternRef
}

// PlanStepInfo describes one step in the living plan.
type PlanStepInfo struct {
	ID          string
	Title       string
	Status      string // ready | blocked | running | done | failed | pending
	SymbolScope []string
	Anchors     []AnchorRef
	DependsOn   []string
	Notes       []string
	Attempts    int
}

// LivePlanInfo is the TUI-safe representation of the persisted living plan.
type LivePlanInfo struct {
	WorkflowID string
	Title      string
	Confidence float64
	Steps      []PlanStepInfo
	ModifiedAt time.Time
}

// DiagnosticsInfo is a snapshot of runtime resource and agent state for the
// session → live subtab.
type DiagnosticsInfo struct {
	ContextTokensUsed int
	ContextTokensMax  int
	ActiveWorkflows   int
	PatternEntries    int
	ActiveProfile     string
	ActiveMode        string
	ActivePhase       string
	DoomLoopState     string
	CapabilitiesTotal int
	PendingApprovals  int
	LiveProviders     int
	ContextStrategy   string
	PruningEvents     int
}

// MapNodeInfo describes a node in the symbol graph (explore → structure subtab).
type MapNodeInfo struct {
	ID       string
	Label    string
	FilePath string
	Line     int
	Kind     string // func | method | type | var | const
}

// MapEdgeInfo describes an edge in the symbol graph.
type MapEdgeInfo struct {
	FromID   string
	ToID     string
	EdgeKind string // calls | implements | embeds | references
}

// PatternProposalInfo is a proposed pattern surfaced by relurpic gap detection.
type PatternProposalInfo struct {
	ID          string
	Title       string
	Scope       string
	Description string
	Confidence  float64
	CreatedAt   time.Time
}

// PatternRecordInfo is a confirmed pattern stored in the pattern registry.
type PatternRecordInfo struct {
	ID          string
	Title       string
	Scope       string
	Description string
	IntentType  string
	CreatedAt   time.Time
	ModifiedAt  time.Time
}

// IntentGapInfo describes a detected gap between stated intent and
// implementation.
type IntentGapInfo struct {
	FilePath    string
	Line        int
	AnchorName  string
	AnchorClass string
	Description string
	Severity    string
}

// PatternMatchInfo describes a prospective pattern match result.
type PatternMatchInfo struct {
	PatternID   string
	Title       string
	Score       float64
	Description string
	Scope       string
}

// TraceInfo holds a parsed execution trace for the debug pane.
type TraceInfo struct {
	Description string
	Frames      []TraceFrame
}

// TraceFrame is one entry in an execution trace.
type TraceFrame struct {
	FuncName string
	FilePath string
	Line     int
	Duration string
	IsError  bool
	ErrorMsg string
	Children []TraceFrame
}

// PlanDiffInfo holds divergence between the living plan and current
// implementation state.
type PlanDiffInfo struct {
	WorkflowID   string
	Steps        []PlanStepInfo
	AnchorDrifts []AnchorDriftInfo
}

// AnchorDriftInfo describes a single anchor drift event.
type AnchorDriftInfo struct {
	AnchorName string
	FilePath   string
	Line       int
	Reason     string
}
