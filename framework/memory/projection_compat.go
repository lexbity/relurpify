package memory

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
)

type WorkflowProjectionRole = core.CoordinationRole

const (
	WorkflowProjectionRoleExecutor  WorkflowProjectionRole = core.CoordinationRoleExecutor
	WorkflowProjectionRoleReviewer  WorkflowProjectionRole = core.CoordinationRoleReviewer
	WorkflowProjectionRoleVerifier  WorkflowProjectionRole = core.CoordinationRoleVerifier
	WorkflowProjectionRoleArchitect WorkflowProjectionRole = core.CoordinationRoleArchitect
)

type WorkflowProjectionTier = string

const (
	WorkflowProjectionTierHot  WorkflowProjectionTier = "hot"
	WorkflowProjectionTierWarm WorkflowProjectionTier = "warm"
)

type WorkflowResourceRef struct {
	WorkflowID string
	RunID      string
	StepID     string
	Role       WorkflowProjectionRole
	Tier       string
	Kind       string
}

func ParseWorkflowResourceURI(uri string) (WorkflowResourceRef, error) {
	trimmed := strings.TrimSpace(uri)
	if trimmed == "" {
		return WorkflowResourceRef{}, fmt.Errorf("workflow resource uri required")
	}
	return WorkflowResourceRef{WorkflowID: trimmed, Tier: "summary"}, nil
}

func DefaultWorkflowProjectionRefs(workflowID, runID, stepID string, role WorkflowProjectionRole) []string {
	workflowID = strings.TrimSpace(workflowID)
	if workflowID == "" {
		return nil
	}
	return []string{workflowID}
}

type WorkflowProjectionService struct {
	Store WorkflowStateStore
}

func (s WorkflowProjectionService) Project(ctx context.Context, ref WorkflowResourceRef) (*core.ResourceReadResult, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(ref.WorkflowID) == "" {
		return &core.ResourceReadResult{}, nil
	}
	payload := map[string]any{
		"workflow_id": ref.WorkflowID,
		"role":        string(ref.Role),
		"tier":        ref.Tier,
	}
	return &core.ResourceReadResult{
		Contents: []core.ContentBlock{core.StructuredContentBlock{Data: payload}},
		Metadata: payload,
	}, nil
}
