package nexus

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	memdb "github.com/lexcodex/relurpify/framework/memory/db"
	fwfmp "github.com/lexcodex/relurpify/framework/middleware/fmp"
	"github.com/lexcodex/relurpify/named/rex/rexkeys"
)

var _ fwfmp.RuntimeEndpoint = (*RuntimeEndpoint)(nil)

const maxRuntimeEndpointProjections = 256

type RuntimeEndpoint struct {
	DescriptorValue     core.RuntimeDescriptor
	Packager            fwfmp.ContextPackager
	WorkflowStore       *memdb.SQLiteWorkflowStateStore
	LineageBindingStore memdb.LineageBindingStore
	Schedule            func(context.Context, string, string, *core.Task, *core.Context) error
	Now                 func() time.Time
	mu                  sync.Mutex
	projections         map[string]core.CapabilityEnvelope
	projectionOrder     []string
}

func (e *RuntimeEndpoint) Descriptor(context.Context) (core.RuntimeDescriptor, error) {
	return e.DescriptorValue, nil
}

func (e *RuntimeEndpoint) ExportContext(ctx context.Context, lineage core.LineageRecord, attempt core.AttemptRecord) (*fwfmp.PortableContextPackage, error) {
	if e == nil || e.Packager == nil {
		return nil, fmt.Errorf("packager unavailable")
	}
	return e.Packager.BuildPackage(ctx, lineage, attempt, fwfmp.RuntimeQuery{
		WorkflowID: lineage.LineageID,
		RunID:      attempt.AttemptID,
	})
}

func (e *RuntimeEndpoint) ValidateContext(_ context.Context, manifest core.ContextManifest, sealed core.SealedContext) error {
	if strings.TrimSpace(manifest.SchemaVersion) == "" {
		return fmt.Errorf("schema version required")
	}
	if strings.TrimSpace(sealed.CipherSuite) == "" {
		return fmt.Errorf("cipher suite required")
	}
	return nil
}

func (e *RuntimeEndpoint) ImportContext(ctx context.Context, _ core.LineageRecord, manifest core.ContextManifest, sealed core.SealedContext) (*fwfmp.PortableContextPackage, error) {
	if e == nil || e.Packager == nil {
		return nil, fmt.Errorf("packager unavailable")
	}
	pkg := &fwfmp.PortableContextPackage{Manifest: manifest}
	if err := e.Packager.UnsealPackage(ctx, sealed, pkg); err != nil {
		return nil, err
	}
	return pkg, nil
}

func (e *RuntimeEndpoint) CreateAttempt(ctx context.Context, lineage core.LineageRecord, accept core.HandoffAccept, pkg *fwfmp.PortableContextPackage) (*core.AttemptRecord, error) {
	if pkg == nil {
		return nil, fmt.Errorf("portable package required")
	}
	task, state, workflowID, runID, err := e.rehydrateTask(pkg, lineage, accept)
	if err != nil {
		return nil, err
	}
	state.Set(rexkeys.FMPLineageID, lineage.LineageID)
	state.Set(rexkeys.FMPAttemptID, accept.ProvisionalAttemptID)
	state.Set("fmp.capability_projection", accept.AcceptedCapabilityProjection)
	state.Set(rexkeys.RexFMPLineageID, lineage.LineageID)
	state.Set(rexkeys.RexFMPAttemptID, accept.ProvisionalAttemptID)
	if e.WorkflowStore != nil {
		if err := e.ensureImportedWorkflow(ctx, workflowID, runID, task); err != nil {
			return nil, err
		}
		if err := e.persistImport(ctx, workflowID, runID, lineage.LineageID, task, state, accept); err != nil {
			return nil, err
		}
	}
	e.rememberProjection(accept.ProvisionalAttemptID, accept.AcceptedCapabilityProjection)
	if e.Schedule != nil {
		if err := e.Schedule(ctx, workflowID, runID, task, state); err != nil {
			return nil, err
		}
	}
	now := e.nowUTC()
	return &core.AttemptRecord{
		AttemptID:        accept.ProvisionalAttemptID,
		LineageID:        lineage.LineageID,
		RuntimeID:        e.DescriptorValue.RuntimeID,
		State:            core.AttemptStateRunning,
		StartTime:        now,
		LastProgressTime: now,
	}, nil
}

func (e *RuntimeEndpoint) FenceAttempt(context.Context, core.FenceNotice) error {
	return nil
}

func (e *RuntimeEndpoint) IssueReceipt(_ context.Context, lineage core.LineageRecord, attempt core.AttemptRecord, _ *fwfmp.PortableContextPackage) (*core.ResumeReceipt, error) {
	return &core.ResumeReceipt{
		ReceiptID:                   attempt.AttemptID + ":receipt",
		LineageID:                   lineage.LineageID,
		AttemptID:                   attempt.AttemptID,
		RuntimeID:                   attempt.RuntimeID,
		StartTime:                   e.nowUTC(),
		CompatibilityVerified:       true,
		CapabilityProjectionApplied: e.projectionForAttempt(attempt.AttemptID),
		Status:                      core.ReceiptStatusRunning,
	}, nil
}

func (e *RuntimeEndpoint) rehydrateTask(pkg *fwfmp.PortableContextPackage, lineage core.LineageRecord, accept core.HandoffAccept) (*core.Task, *core.Context, string, string, error) {
	if len(pkg.ExecutionPayload) == 0 {
		return nil, nil, "", "", fmt.Errorf("execution payload required")
	}
	var payload map[string]any
	if err := json.Unmarshal(pkg.ExecutionPayload, &payload); err != nil {
		return nil, nil, "", "", err
	}
	taskMap, _ := payload["task"].(map[string]any)
	task := &core.Task{
		ID:          strings.TrimSpace(valueString(taskMap["id"])),
		Type:        core.TaskType(strings.TrimSpace(valueString(taskMap["type"]))),
		Instruction: strings.TrimSpace(valueString(taskMap["instruction"])),
		Context:     mapStringAny(taskMap["context"]),
		Metadata:    mapStringString(taskMap["metadata"]),
	}
	if task.Type == "" {
		task.Type = core.TaskTypeCodeGeneration
	}
	if task.Instruction == "" {
		return nil, nil, "", "", fmt.Errorf("imported task instruction required")
	}
	state := core.NewContext()
	stateMap, _ := payload["state"].(map[string]any)
	for key, value := range stateMap {
		state.Set(key, value)
	}
	for key, value := range task.Context {
		state.Set(key, value)
	}
	workflowID := strings.TrimSpace(valueString(stateMap[rexkeys.WorkflowID]))
	if workflowID == "" {
		workflowID = strings.TrimSpace(valueString(task.Context[rexkeys.WorkflowID]))
	}
	if workflowID == "" {
		workflowID = lineage.LineageID
	}
	runID := accept.ProvisionalAttemptID
	task.Context[rexkeys.WorkflowID] = workflowID
	task.Context[rexkeys.RunID] = runID
	state.Set(rexkeys.WorkflowID, workflowID)
	state.Set(rexkeys.RunID, runID)
	state.Set(rexkeys.RexWorkflowID, workflowID)
	state.Set(rexkeys.RexRunID, runID)
	return task, state, workflowID, runID, nil
}

func (e *RuntimeEndpoint) persistImport(ctx context.Context, workflowID, runID, lineageID string, task *core.Task, state *core.Context, accept core.HandoffAccept) error {
	if e.LineageBindingStore == nil {
		return nil
	}
	return e.LineageBindingStore.UpsertLineageBinding(ctx, memdb.LineageBindingRecord{
		WorkflowID: workflowID,
		RunID:      runID,
		LineageID:  lineageID,
		AttemptID:  accept.ProvisionalAttemptID,
		RuntimeID:  e.DescriptorValue.RuntimeID,
		SessionID:  importedSessionID(state, task),
		State:      string(core.AttemptStateRunning),
		UpdatedAt:  e.nowUTC(),
	})
}

func (e *RuntimeEndpoint) ensureImportedWorkflow(ctx context.Context, workflowID, runID string, task *core.Task) error {
	if e.WorkflowStore == nil {
		return nil
	}
	if _, ok, err := e.WorkflowStore.GetWorkflow(ctx, workflowID); err != nil {
		return err
	} else if !ok {
		if err := e.WorkflowStore.CreateWorkflow(ctx, memory.WorkflowRecord{
			WorkflowID:  workflowID,
			TaskID:      task.ID,
			TaskType:    task.Type,
			Instruction: task.Instruction,
			Status:      memory.WorkflowRunStatusRunning,
			Metadata:    map[string]any{"agent": "rex", "imported": true},
			CreatedAt:   e.nowUTC(),
			UpdatedAt:   e.nowUTC(),
		}); err != nil {
			return err
		}
	}
	if _, ok, err := e.WorkflowStore.GetRun(ctx, runID); err != nil {
		return err
	} else if !ok {
		if err := e.WorkflowStore.CreateRun(ctx, memory.WorkflowRunRecord{
			RunID:      runID,
			WorkflowID: workflowID,
			Status:     memory.WorkflowRunStatusRunning,
			AgentName:  "rex",
			AgentMode:  "resume",
			StartedAt:  e.nowUTC(),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (e *RuntimeEndpoint) nowUTC() time.Time {
	if e != nil && e.Now != nil {
		return e.Now().UTC()
	}
	return time.Now().UTC()
}

func (e *RuntimeEndpoint) rememberProjection(attemptID string, projection core.CapabilityEnvelope) {
	if e == nil || strings.TrimSpace(attemptID) == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.projections == nil {
		e.projections = map[string]core.CapabilityEnvelope{}
	}
	if _, ok := e.projections[attemptID]; ok {
		e.projections[attemptID] = projection
		return
	}
	if len(e.projections) >= maxRuntimeEndpointProjections {
		evictID := ""
		for len(e.projectionOrder) > 0 {
			evictID = e.projectionOrder[0]
			e.projectionOrder = e.projectionOrder[1:]
			if _, ok := e.projections[evictID]; ok {
				break
			}
			evictID = ""
		}
		if evictID != "" {
			delete(e.projections, evictID)
		}
	}
	e.projectionOrder = append(e.projectionOrder, attemptID)
	e.projections[attemptID] = projection
}

func (e *RuntimeEndpoint) projectionForAttempt(attemptID string) core.CapabilityEnvelope {
	if e == nil || strings.TrimSpace(attemptID) == "" {
		return core.CapabilityEnvelope{}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.projections == nil {
		return core.CapabilityEnvelope{}
	}
	return e.projections[attemptID]
}

func importedSessionID(state *core.Context, task *core.Task) string {
	if state != nil {
		if value := strings.TrimSpace(state.GetString(rexkeys.GatewaySessionID)); value != "" {
			return value
		}
		if value := strings.TrimSpace(state.GetString(rexkeys.SessionID)); value != "" {
			return value
		}
	}
	if task != nil && task.Context != nil {
		for _, key := range []string{rexkeys.GatewaySessionID, rexkeys.SessionID} {
			if value := strings.TrimSpace(valueString(task.Context[key])); value != "" {
				return value
			}
		}
	}
	return ""
}

func valueString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func mapStringAny(value any) map[string]any {
	raw, ok := value.(map[string]any)
	if !ok || len(raw) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(raw))
	for key, entry := range raw {
		out[key] = entry
	}
	return out
}

func mapStringString(value any) map[string]string {
	raw, ok := value.(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make(map[string]string, len(raw))
	for key, entry := range raw {
		if text := strings.TrimSpace(valueString(entry)); text != "" {
			out[key] = text
		}
	}
	return out
}
