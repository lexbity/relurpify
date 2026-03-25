package nexus

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/named/rex/proof"
	"github.com/lexcodex/relurpify/named/rex/runtime"
)

// Projection is the Nexus-facing runtime snapshot for rex.
type Projection struct {
	Health         runtime.Health     `json:"health"`
	ActiveWork     int                `json:"active_work"`
	QueueDepth     int                `json:"queue_depth"`
	RecoveryCount  int                `json:"recovery_count"`
	LastError      string             `json:"last_error,omitempty"`
	LastProof      proof.ProofSurface `json:"last_proof"`
	WorkflowID     string             `json:"workflow_id,omitempty"`
	RunID          string             `json:"run_id,omitempty"`
	// Phase 7.3: DR metadata for federation health summaries
	FailoverReady  bool               `json:"failover_ready,omitempty"`
	RecoveryState  string             `json:"recovery_state,omitempty"`
	RuntimeVersion string             `json:"runtime_version,omitempty"`
}

type Registration struct {
	Name            string              `json:"name"`
	RuntimeType     string              `json:"runtime_type"`
	Managed         bool                `json:"managed"`
	Capabilities    []core.Capability   `json:"capabilities,omitempty"`
	ProjectionTiers []string            `json:"projection_tiers,omitempty"`
}

type AdminSnapshot struct {
	Runtime        Projection         `json:"runtime"`
	WorkflowRefURI []string           `json:"workflow_ref_uri,omitempty"`
	HotState       map[string]any     `json:"hot_state,omitempty"`
	WarmState      map[string]any     `json:"warm_state,omitempty"`
}

// ManagedRuntime is the Nexus-facing contract exposed by rex.
type ManagedRuntime interface {
	Execute(context.Context, *core.Task, *core.Context) (*core.Result, error)
	RuntimeProjection() Projection
}

type CallableRuntime interface {
	ManagedRuntime
	Capabilities() []core.Capability
}

type Adapter struct {
	name    string
	runtime CallableRuntime
	store   memory.WorkflowStateStore
}

func NewAdapter(name string, runtime CallableRuntime, store memory.WorkflowStateStore) *Adapter {
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
		Capabilities:    append([]core.Capability{}, a.runtime.Capabilities()...),
		ProjectionTiers: []string{"hot", "warm"},
	}
}

func (a *Adapter) Invoke(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	if a == nil || a.runtime == nil {
		return nil, fmt.Errorf("managed runtime unavailable")
	}
	return a.runtime.Execute(ctx, task, state)
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
	refBase := memory.DefaultWorkflowProjectionRefs(projection.WorkflowID, projection.RunID, "", core.CoordinationRoleBackgroundAgent)
	snapshot.WorkflowRefURI = append([]string{}, refBase...)
	service := memory.WorkflowProjectionService{Store: a.store}
	hot, err := service.Project(ctx, memory.WorkflowResourceRef{
		WorkflowID: projection.WorkflowID,
		RunID:      projection.RunID,
		Tier:       memory.WorkflowProjectionTierHot,
		Role:       core.CoordinationRoleBackgroundAgent,
	})
	if err != nil {
		return AdminSnapshot{}, err
	}
	warm, err := service.Project(ctx, memory.WorkflowResourceRef{
		WorkflowID: projection.WorkflowID,
		RunID:      projection.RunID,
		Tier:       memory.WorkflowProjectionTierWarm,
		Role:       core.CoordinationRoleBackgroundAgent,
	})
	if err != nil {
		return AdminSnapshot{}, err
	}
	snapshot.HotState = firstStructuredContent(hot)
	snapshot.WarmState = firstStructuredContent(warm)
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

func firstStructuredContent(result *core.ResourceReadResult) map[string]any {
	if result == nil {
		return nil
	}
	for _, block := range result.Contents {
		if typed, ok := block.(core.StructuredContentBlock); ok {
			if payload, ok := typed.Data.(map[string]any); ok {
				return payload
			}
		}
	}
	return nil
}
