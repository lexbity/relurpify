package interaction

import (
	"fmt"
	"time"
)

// generateID creates a simple unique ID without external dependencies.
func generateID() string {
	return fmt.Sprintf("frame-%d", time.Now().UnixNano())
}

// NewScopeConfirmationFrame creates a scope confirmation frame for ingestion.
func NewScopeConfirmationFrame(taskID, sessionID string) *InteractionFrame {
	return &InteractionFrame{
		ID:        generateID(),
		Type:      FrameScopeConfirmation,
		TaskID:    taskID,
		SessionID: sessionID,
		Seq:       0, // Will be set by EmitFrame
		Slots: []ActionSlot{
			{
				ID:      "use_selected_files",
				Label:   "Use my selected files",
				Action:  "files_only",
				Risk:    "low",
				Default: true,
			},
			{
				ID:      "scan_changed",
				Label:   "Scan changed files (incremental)",
				Action:  "incremental",
				Risk:    "medium",
				Default: false,
			},
			{
				ID:      "scan_full",
				Label:   "Scan full workspace",
				Action:  "full",
				Risk:    "high",
				Default: false,
			},
		},
		DefaultSlot: "use_selected_files",
		Payload: map[string]any{
			"mode": "files_only",
		},
		CreatedAt: time.Now(),
		Timeout:   5 * time.Minute,
	}
}

// NewHITLApprovalFrame creates a HITL approval frame.
func NewHITLApprovalFrame(taskID, sessionID string, permission string, risk string) *InteractionFrame {
	return &InteractionFrame{
		ID:        generateID(),
		Type:      FrameHITLApproval,
		TaskID:    taskID,
		SessionID: sessionID,
		Seq:       0,
		Slots: []ActionSlot{
			{
				ID:      "approve",
				Label:   "Approve",
				Action:  "approve",
				Risk:    risk,
				Default: false,
			},
			{
				ID:      "reject",
				Label:   "Reject",
				Action:  "reject",
				Risk:    "low",
				Default: false,
			},
		},
		DefaultSlot: "",
		Payload: map[string]any{
			"permission": permission,
			"risk":       risk,
		},
		CreatedAt: time.Now(),
		Timeout:   5 * time.Minute,
	}
}

// NewCandidateSelectionFrame creates a candidate selection frame for ambiguous classification.
func NewCandidateSelectionFrame(taskID, sessionID string, candidates []string) *InteractionFrame {
	slots := make([]ActionSlot, len(candidates))
	for i, candidate := range candidates {
		slots[i] = ActionSlot{
			ID:      candidate,
			Label:   candidate,
			Action:  candidate,
			Risk:    "low",
			Default: i == 0,
		}
	}

	return &InteractionFrame{
		ID:          generateID(),
		Type:        FrameCandidateSelection,
		TaskID:      taskID,
		SessionID:   sessionID,
		Seq:         0,
		Slots:       slots,
		DefaultSlot: candidates[0],
		Payload: map[string]any{
			"candidates": candidates,
		},
		CreatedAt: time.Now(),
		Timeout:   5 * time.Minute,
	}
}

// NewOutcomeFeedbackFrame creates an outcome feedback frame.
func NewOutcomeFeedbackFrame(taskID, sessionID string, outcome string) *InteractionFrame {
	return &InteractionFrame{
		ID:        generateID(),
		Type:      FrameOutcomeFeedback,
		TaskID:    taskID,
		SessionID: sessionID,
		Seq:       0,
		Slots: []ActionSlot{
			{
				ID:      "positive",
				Label:   "Yes, solved my problem",
				Action:  "positive",
				Risk:    "low",
				Default: true,
			},
			{
				ID:      "partial",
				Label:   "Partially helpful",
				Action:  "partial",
				Risk:    "low",
				Default: false,
			},
			{
				ID:      "negative",
				Label:   "No, not helpful",
				Action:  "negative",
				Risk:    "low",
				Default: false,
			},
		},
		DefaultSlot: "positive",
		Payload: map[string]any{
			"outcome": outcome,
		},
		CreatedAt: time.Now(),
		Timeout:   30 * time.Second,
	}
}
