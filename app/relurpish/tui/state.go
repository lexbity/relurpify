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

// CapabilityDetail is the richer capability inspection model used by the Tasks pane.
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
