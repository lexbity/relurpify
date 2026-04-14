package session

import (
	"time"
)

// SessionRecord is a summarized view of a past Euclo session, used for
// session selection in the interaction trigger flow.
type SessionRecord struct {
	// WorkflowID is the stable identifier for this session.
	WorkflowID string `json:"workflow_id"`

	// Instruction is the original task instruction that started the session.
	Instruction string `json:"instruction"`

	// Mode is the last active mode (code, debug, planning, etc.).
	Mode string `json:"mode"`

	// Phase is the last active archaeology phase, if applicable.
	Phase string `json:"phase,omitempty"`

	// Status is the last workflow run status.
	Status string `json:"status"`

	// ActivePlanVersion is the version number of the current living plan,
	// zero if no plan exists.
	ActivePlanVersion int `json:"active_plan_version,omitempty"`

	// ActivePlanTitle is the short title of the active plan step or
	// overall plan, if available.
	ActivePlanTitle string `json:"active_plan_title,omitempty"`

	// HasBKCContext indicates that this session has anchored BKC root
	// chunk IDs and can be resumed with semantic warmup.
	HasBKCContext bool `json:"has_bkc_context"`

	// RootChunkIDs are the BKC root chunk IDs anchored to the active
	// plan version. Empty if no BKC context is available.
	RootChunkIDs []string `json:"root_chunk_ids,omitempty"`

	// LastActiveAt is when this session last had an active run.
	LastActiveAt time.Time `json:"last_active_at"`

	// WorkspaceRoot is the workspace path associated with this session.
	WorkspaceRoot string `json:"workspace_root,omitempty"`
}

// SessionList is an ordered list of sessions for selection.
// Order is most-recently-active first.
type SessionList struct {
	Sessions  []SessionRecord `json:"sessions"`
	Workspace string          `json:"workspace"`
}
