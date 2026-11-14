package tui

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/userconfig/config"
)

type workspaceGuardReader struct{ workspace string }

// NewWorkspaceGuardReader supplies confined inspection independently of model
// readiness. It exposes no writes, runtime reload, or authorization operation.
func NewWorkspaceGuardReader(workspace string) GuardPolicyPort {
	return &workspaceGuardReader{workspace: workspace}
}

func (r *workspaceGuardReader) GuardPolicy(ctx context.Context, section config.GuardSection) (GuardPolicySnapshot, error) {
	if r.workspace == "" {
		return GuardPolicySnapshot{}, fmt.Errorf("workspace is not selected")
	}
	document, err := config.ReadGuardPolicy(ctx, r.workspace, section)
	if err != nil {
		return GuardPolicySnapshot{}, err
	}
	return GuardPolicySnapshot{Workspace: r.workspace, Document: document, Notice: "Disk source inspected; this does not establish live shell or ingestion enforcement."}, nil
}
