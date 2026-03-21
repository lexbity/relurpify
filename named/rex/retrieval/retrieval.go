package retrieval

import (
	"context"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory/db"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	"github.com/lexcodex/relurpify/named/rex/route"
)

type Policy = eucloruntime.RetrievalPolicy
type Expansion = eucloruntime.ContextExpansion

// ResolvePolicy maps rex route decisions to workflow-aware retrieval policy.
func ResolvePolicy(decision route.RouteDecision) Policy {
	mode := eucloruntime.ModeResolution{ModeID: decision.Mode}
	profile := eucloruntime.ExecutionProfileSelection{ProfileID: decision.Profile}
	policy := eucloruntime.ResolveRetrievalPolicy(mode, profile)
	if decision.RequireRetrieval {
		policy.WidenToWorkflow = true
	}
	return policy
}

// ExpandWithWorkflowStore uses the existing euclo retrieval helpers directly.
func ExpandWithWorkflowStore(ctx context.Context, store *db.SQLiteWorkflowStateStore, workflowID string, task *core.Task, state *core.Context, decision route.RouteDecision) (Expansion, error) {
	return eucloruntime.ExpandContext(ctx, store, workflowID, task, state, ResolvePolicy(decision))
}

// Apply persists expansion into state and task context.
func Apply(state *core.Context, task *core.Task, expansion Expansion) *core.Task {
	return eucloruntime.ApplyContextExpansion(state, task, expansion)
}
