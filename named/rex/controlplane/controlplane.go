package controlplane

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lexcodex/relurpify/framework/memory"
)

type WorkloadClass string

const (
	WorkloadCritical   WorkloadClass = "critical"
	WorkloadImportant  WorkloadClass = "important"
	WorkloadBestEffort WorkloadClass = "best_effort"
)

type AdmissionRequest struct {
	TenantID string
	Class    WorkloadClass
}

type AdmissionDecision struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

type AdmissionController interface {
	Admit(AdmissionRequest) bool
	Decide(AdmissionRequest) AdmissionDecision
	Release(AdmissionRequest)
}

type FairnessController struct {
	Limits map[string]int
	Usage  map[string]int
}

func (f *FairnessController) Admit(req AdmissionRequest) bool {
	return f.Decide(req).Allowed
}

func (f *FairnessController) Decide(req AdmissionRequest) AdmissionDecision {
	tenantID := strings.TrimSpace(req.TenantID)
	if req.Class == WorkloadCritical {
		return AdmissionDecision{Allowed: true, Reason: "critical_bypass"}
	}
	limit := 1
	if f != nil && f.Limits != nil && f.Limits[tenantID] > 0 {
		limit = f.Limits[tenantID]
	}
	if f != nil && f.Usage == nil {
		f.Usage = map[string]int{}
	}
	if f != nil && f.Usage[tenantID] >= limit {
		return AdmissionDecision{Allowed: false, Reason: "tenant_quota_exceeded"}
	}
	if f != nil {
		f.Usage[tenantID]++
	}
	return AdmissionDecision{Allowed: true, Reason: "tenant_quota_available"}
}

func (f *FairnessController) Release(req AdmissionRequest) {
	if f == nil || f.Usage == nil {
		return
	}
	tenantID := strings.TrimSpace(req.TenantID)
	if f.Usage[tenantID] > 0 {
		f.Usage[tenantID]--
	}
}

type LoadController struct {
	Capacity int
	InFlight int
	Fairness *FairnessController
}

func (c *LoadController) Admit(req AdmissionRequest) bool {
	return c.Decide(req).Allowed
}

func (c *LoadController) Decide(req AdmissionRequest) AdmissionDecision {
	if req.Class == WorkloadCritical {
		c.InFlight++
		if c.Fairness != nil {
			_ = c.Fairness.Decide(req)
		}
		return AdmissionDecision{Allowed: true, Reason: "critical_bypass"}
	}
	capacity := c.Capacity
	if capacity <= 0 {
		capacity = 1
	}
	if c.InFlight >= capacity {
		return AdmissionDecision{Allowed: false, Reason: "over_capacity"}
	}
	if c.Fairness != nil {
		decision := c.Fairness.Decide(req)
		if !decision.Allowed {
			return decision
		}
	}
	c.InFlight++
	return AdmissionDecision{Allowed: true, Reason: "capacity_available"}
}

func (c *LoadController) Release(req AdmissionRequest) {
	if c.InFlight > 0 {
		c.InFlight--
	}
	if c.Fairness != nil {
		c.Fairness.Release(req)
	}
}

type OperatorAction struct {
	Action   string `json:"action"`
	Role     string `json:"role"`
	TenantID string `json:"tenant_id,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type AuditRecord struct {
	Action    string    `json:"action"`
	Role      string    `json:"role"`
	TenantID  string    `json:"tenant_id,omitempty"`
	Allowed   bool      `json:"allowed"`
	Reason    string    `json:"reason,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type AuditLog struct {
	mu      sync.RWMutex
	records []AuditRecord
}

func (l *AuditLog) Append(record AuditRecord) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = append(l.records, record)
}

func (l *AuditLog) Records() []AuditRecord {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]AuditRecord, len(l.records))
	copy(out, l.records)
	return out
}

func ValidateOperatorAction(action OperatorAction) error {
	if strings.TrimSpace(action.Action) == "" {
		return errors.New("operator action required")
	}
	if strings.TrimSpace(action.Role) == "" {
		return errors.New("operator role required")
	}
	return nil
}

func AuthorizeOperatorAction(action OperatorAction, audit *AuditLog) error {
	err := ValidateOperatorAction(action)
	allowed := err == nil && isPrivilegedRole(action.Role)
	reason := strings.TrimSpace(action.Reason)
	if !allowed && err == nil {
		err = fmt.Errorf("operator role %q not authorized", action.Role)
	}
	if audit != nil {
		audit.Append(AuditRecord{
			Action:    action.Action,
			Role:      action.Role,
			TenantID:  action.TenantID,
			Allowed:   err == nil,
			Reason:    firstNonEmpty(reason, errorString(err), "authorized"),
			Timestamp: time.Now().UTC(),
		})
	}
	return err
}

type WorkflowLister interface {
	ListWorkflows(context.Context, int) ([]memory.WorkflowRecord, error)
	GetRun(context.Context, string) (*memory.WorkflowRunRecord, bool, error)
}

type SLOSignals struct {
	TotalWorkflows      int      `json:"total_workflows"`
	RunningWorkflows    int      `json:"running_workflows"`
	CompletedWorkflows  int      `json:"completed_workflows"`
	FailedWorkflows     int      `json:"failed_workflows"`
	RecoverySensitive   int      `json:"recovery_sensitive"`
	DegradedWorkflowIDs []string `json:"degraded_workflow_ids,omitempty"`
}

func CollectSLOSignals(ctx context.Context, store WorkflowLister, limit int) (SLOSignals, error) {
	if store == nil {
		return SLOSignals{}, nil
	}
	workflows, err := store.ListWorkflows(ctx, limit)
	if err != nil {
		return SLOSignals{}, err
	}
	signals := SLOSignals{TotalWorkflows: len(workflows)}
	for _, workflow := range workflows {
		switch workflow.Status {
		case memory.WorkflowRunStatusRunning, memory.WorkflowRunStatusNeedsReplan:
			signals.RunningWorkflows++
			signals.RecoverySensitive++
		case memory.WorkflowRunStatusCompleted:
			signals.CompletedWorkflows++
		case memory.WorkflowRunStatusFailed, memory.WorkflowRunStatusCanceled:
			signals.FailedWorkflows++
			signals.DegradedWorkflowIDs = append(signals.DegradedWorkflowIDs, workflow.WorkflowID)
		}
		runID := workflow.WorkflowID + ":run"
		if run, ok, err := store.GetRun(ctx, runID); err == nil && ok {
			if strings.Contains(strings.ToLower(run.AgentMode), "recover") {
				signals.RecoverySensitive++
			}
		}
	}
	return signals, nil
}

type DRMetadata struct {
	WorkflowID      string    `json:"workflow_id"`
	RunID           string    `json:"run_id,omitempty"`
	FailoverReady   bool      `json:"failover_ready"`
	RecoveryState   string    `json:"recovery_state,omitempty"`
	LastCheckpoint  time.Time `json:"last_checkpoint,omitempty"`
	RuntimeVersion  string    `json:"runtime_version,omitempty"`
}

func BuildDRMetadata(workflow memory.WorkflowRecord, run *memory.WorkflowRunRecord) DRMetadata {
	metadata := DRMetadata{
		WorkflowID:    workflow.WorkflowID,
		FailoverReady: workflow.Status == memory.WorkflowRunStatusRunning || workflow.Status == memory.WorkflowRunStatusNeedsReplan,
		RecoveryState: string(workflow.Status),
	}
	if run != nil {
		metadata.RunID = run.RunID
		metadata.RuntimeVersion = run.RuntimeVersion
		metadata.LastCheckpoint = run.StartedAt
	}
	return metadata
}

func isPrivilegedRole(role string) bool {
	role = strings.ToLower(strings.TrimSpace(role))
	return strings.Contains(role, "operator") || strings.Contains(role, "admin")
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
