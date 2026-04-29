package nexus

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/rex/proof"
	"codeburg.org/lexbit/relurpify/named/rex/runtime"
	"codeburg.org/lexbit/relurpify/named/rex/store"
)

// Projection is the Nexus-facing runtime snapshot for rex.
type Projection struct {
	Health        runtime.Health     `json:"health"`
	ActiveWork    int                `json:"active_work"`
	QueueDepth    int                `json:"queue_depth"`
	RecoveryCount int                `json:"recovery_count"`
	LastError     string             `json:"last_error,omitempty"`
	LastProof     proof.ProofSurface `json:"last_proof"`
	WorkflowID    string             `json:"workflow_id,omitempty"`
	RunID         string             `json:"run_id,omitempty"`
	// Phase 7.3: DR metadata for federation health summaries
	FailoverReady  bool      `json:"failover_ready,omitempty"`
	RecoveryState  string    `json:"recovery_state,omitempty"`
	RuntimeVersion string    `json:"runtime_version,omitempty"`
	LastCheckpoint time.Time `json:"last_checkpoint,omitempty"`
}

type Registration struct {
	Name            string            `json:"name"`
	RuntimeType     string            `json:"runtime_type"`
	Managed         bool              `json:"managed"`
	Capabilities    []string          `json:"capabilities,omitempty"`
	ProjectionTiers []string          `json:"projection_tiers,omitempty"`
}

type AdminSnapshot struct {
	Runtime        Projection     `json:"runtime"`
	WorkflowRefURI []string       `json:"workflow_ref_uri,omitempty"`
	HotState       map[string]any `json:"hot_state,omitempty"`
	WarmState      map[string]any `json:"warm_state,omitempty"`
}

// ManagedRuntime is the Nexus-facing contract exposed by rex.
type ManagedRuntime interface {
	Execute(context.Context, *core.Task, *contextdata.Envelope) (*core.Result, error)
	RuntimeProjection() Projection
}

type CallableRuntime interface {
	ManagedRuntime
	Capabilities() []string
}

type Adapter struct {
	name    string
	runtime CallableRuntime
	store   *store.SQLiteWorkflowStore
}

func NewAdapter(name string, runtime CallableRuntime, store *store.SQLiteWorkflowStore) *Adapter {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		trimmed = "rex"
	}
	return &Adapter{name: trimmed, runtime: runtime, store: store}
}

func (a *Adapter) Registration() Registration {
	if a == nil || a.runtime == nil {
		return Registration{}
	}
	return Registration{
		Name:            a.name,
		RuntimeType:     "nexus-managed",
		Managed:         true,
		Capabilities:    append([]string{}, a.runtime.Capabilities()...),
		ProjectionTiers: []string{"hot", "warm"},
	}
}

func (a *Adapter) Invoke(ctx context.Context, task *core.Task, env *contextdata.Envelope) (*core.Result, error) {
	if a == nil || a.runtime == nil {
		return nil, fmt.Errorf("managed runtime unavailable")
	}
	return a.runtime.Execute(ctx, task, env)
}

func (a *Adapter) AdminSnapshot(ctx context.Context) (AdminSnapshot, error) {
	if a == nil || a.runtime == nil {
		return AdminSnapshot{}, fmt.Errorf("managed runtime unavailable")
	}
	projection := a.runtime.RuntimeProjection()
	snapshot := AdminSnapshot{Runtime: projection}
	if a.store == nil || strings.TrimSpace(projection.WorkflowID) == "" {
		return snapshot, nil
	}
	snapshot.WorkflowRefURI = []string{
		projection.WorkflowID + "/" + projection.RunID + "/context",
		projection.WorkflowID + "/" + projection.RunID + "/state",
	}
	if workflow, ok, err := a.store.GetWorkflow(ctx, projection.WorkflowID); err == nil && ok {
		snapshot.HotState = map[string]any{
			"workflow_id": workflow.WorkflowID,
			"task_id":     workflow.TaskID,
			"task_type":   workflow.TaskType,
			"instruction": workflow.Instruction,
		}
	}
	if run, ok, err := a.store.GetRun(ctx, projection.RunID); err == nil && ok {
		snapshot.WarmState = map[string]any{
			"run_id":      run.RunID,
			"workflow_id": run.WorkflowID,
			"status":      run.Status,
			"agent_mode":  run.AgentMode,
		}
	}
	return snapshot, nil
}

// BuildProjection creates a Nexus-managed runtime projection.
func BuildProjection(manager *runtime.Manager, surface proof.ProofSurface) Projection {
	details := manager.Details()
	projection := Projection{
		Health:        details.Health,
		ActiveWork:    details.ActiveWork,
		QueueDepth:    details.QueueDepth,
		RecoveryCount: details.RecoveryCount,
		LastError:     details.LastError,
		LastProof:     surface,
		WorkflowID:    details.LastWorkflowID,
		RunID:         details.LastRunID,
	}

	// Phase 7.3: Include DR metadata for federation health summaries
	// Mark as failover-ready if active work is running
	projection.FailoverReady = details.ActiveWork > 0
	projection.RecoveryState = strings.ToLower(strings.TrimSpace(string(details.Health)))

	return projection
}
