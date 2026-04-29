package nexus

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/named/rex/rexkeys"
	"codeburg.org/lexbit/relurpify/named/rex/store"
	fwfmp "codeburg.org/lexbit/relurpify/relurpnet/fmp"
)

var _ fwfmp.RuntimeEndpoint = (*RuntimeEndpoint)(nil)

const maxRuntimeEndpointProjections = 256

type RuntimeEndpoint struct {
	DescriptorValue     fwfmp.RuntimeDescriptor
	Packager            fwfmp.ContextPackager
	WorkflowStore       *store.SQLiteWorkflowStore
	LineageBindingStore store.LineageBindingStore
	Schedule            func(context.Context, string, string, *core.Task, *contextdata.Envelope) error
	Now                 func() time.Time
	mu                  sync.Mutex
	projections         map[string]fwfmp.CapabilityEnvelope
	projectionOrder     []string
}

func (e *RuntimeEndpoint) Descriptor(context.Context) (fwfmp.RuntimeDescriptor, error) {
	return e.DescriptorValue, nil
}

func (e *RuntimeEndpoint) ExportContext(ctx context.Context, lineage fwfmp.LineageRecord, attempt fwfmp.AttemptRecord) (*fwfmp.PortableContextPackage, error) {
	if e == nil || e.Packager == nil {
		return nil, fmt.Errorf("packager unavailable")
	}
	return e.Packager.BuildPackage(ctx, lineage, attempt, fwfmp.RuntimeQuery{
		WorkflowID: lineage.LineageID,
		RunID:      attempt.AttemptID,
	})
}

func (e *RuntimeEndpoint) ValidateContext(_ context.Context, manifest fwfmp.ContextManifest, sealed fwfmp.SealedContext) error {
	if strings.TrimSpace(manifest.SchemaVersion) == "" {
		return fmt.Errorf("schema version required")
	}
	if strings.TrimSpace(sealed.CipherSuite) == "" {
		return fmt.Errorf("cipher suite required")
	}
	return nil
}

func (e *RuntimeEndpoint) ImportContext(ctx context.Context, _ fwfmp.LineageRecord, manifest fwfmp.ContextManifest, sealed fwfmp.SealedContext) (*fwfmp.PortableContextPackage, error) {
	if e == nil || e.Packager == nil {
		return nil, fmt.Errorf("packager unavailable")
	}
	pkg := &fwfmp.PortableContextPackage{Manifest: manifest}
	if err := e.Packager.UnsealPackage(ctx, sealed, pkg); err != nil {
		return nil, err
	}
	return pkg, nil
}

func (e *RuntimeEndpoint) CreateAttempt(ctx context.Context, lineage fwfmp.LineageRecord, accept fwfmp.HandoffAccept, pkg *fwfmp.PortableContextPackage) (*fwfmp.AttemptRecord, error) {
	if pkg == nil {
		return nil, fmt.Errorf("portable package required")
	}
	task, env, workflowID, runID, err := e.rehydrateTask(pkg, lineage, accept)
	if err != nil {
		return nil, err
	}
	env.SetWorkingValue(rexkeys.FMPLineageID, lineage.LineageID, contextdata.MemoryClassTask)
	env.SetWorkingValue(rexkeys.FMPAttemptID, accept.ProvisionalAttemptID, contextdata.MemoryClassTask)
	env.SetWorkingValue("fmp.capability_projection", accept.AcceptedCapabilityProjection, contextdata.MemoryClassTask)
	env.SetWorkingValue(rexkeys.RexFMPLineageID, lineage.LineageID, contextdata.MemoryClassTask)
	env.SetWorkingValue(rexkeys.RexFMPAttemptID, accept.ProvisionalAttemptID, contextdata.MemoryClassTask)
	if e.WorkflowStore != nil {
		if err := e.ensureImportedWorkflow(ctx, workflowID, runID, task); err != nil {
			return nil, err
		}
		if err := e.persistImport(ctx, workflowID, runID, lineage.LineageID, task, env, accept); err != nil {
			return nil, err
		}
	}
	e.rememberProjection(accept.ProvisionalAttemptID, accept.AcceptedCapabilityProjection)
	if e.Schedule != nil {
		if err := e.Schedule(ctx, workflowID, runID, task, env); err != nil {
			return nil, err
		}
	}
	return &fwfmp.AttemptRecord{
		AttemptID:        accept.ProvisionalAttemptID,
		LineageID:        lineage.LineageID,
		RuntimeID:        e.DescriptorValue.RuntimeID,
		State:            fwfmp.AttemptStateRunning,
	}, nil
}

func (e *RuntimeEndpoint) FenceAttempt(context.Context, fwfmp.FenceNotice) error {
	return nil
}

func (e *RuntimeEndpoint) IssueReceipt(_ context.Context, lineage fwfmp.LineageRecord, attempt fwfmp.AttemptRecord, _ *fwfmp.PortableContextPackage) (*fwfmp.ResumeReceipt, error) {
	return &fwfmp.ResumeReceipt{
		ReceiptID:                   attempt.AttemptID + ":receipt",
		LineageID:                   lineage.LineageID,
		AttemptID:                   attempt.AttemptID,
		RuntimeID:                   attempt.RuntimeID,
		StartTime:                   e.nowUTC(),
		CompatibilityVerified:       true,
		CapabilityProjectionApplied: e.projectionForAttempt(attempt.AttemptID),
		Status:                      fwfmp.ReceiptStatusRunning,
	}, nil
}

func (e *RuntimeEndpoint) rehydrateTask(pkg *fwfmp.PortableContextPackage, lineage fwfmp.LineageRecord, accept fwfmp.HandoffAccept) (*core.Task, *contextdata.Envelope, string, string, error) {
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
		Type:        strings.TrimSpace(valueString(taskMap["type"])),
		Instruction: strings.TrimSpace(valueString(taskMap["instruction"])),
		Context:     mapStringAny(taskMap["context"]),
		Metadata:    mapStringAny(taskMap["metadata"]),
	}
	if task.Type == "" {
		task.Type = "code-generation"
	}
	if task.Instruction == "" {
		return nil, nil, "", "", fmt.Errorf("imported task instruction required")
	}
	env := contextdata.NewEnvelope(task.ID, "")
	stateMap, _ := payload["state"].(map[string]any)
	for key, value := range stateMap {
		env.SetWorkingValue(key, value, contextdata.MemoryClassTask)
	}
	for key, value := range task.Context {
		env.SetWorkingValue(key, value, contextdata.MemoryClassTask)
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
	env.SetWorkingValue(rexkeys.WorkflowID, workflowID, contextdata.MemoryClassTask)
	env.SetWorkingValue(rexkeys.RunID, runID, contextdata.MemoryClassTask)
	env.SetWorkingValue(rexkeys.RexWorkflowID, workflowID, contextdata.MemoryClassTask)
	env.SetWorkingValue(rexkeys.RexRunID, runID, contextdata.MemoryClassTask)
	return task, env, workflowID, runID, nil
}

func (e *RuntimeEndpoint) persistImport(ctx context.Context, workflowID, runID, lineageID string, task *core.Task, env *contextdata.Envelope, accept fwfmp.HandoffAccept) error {
	if e.LineageBindingStore == nil {
		return nil
	}
	return e.LineageBindingStore.UpsertLineageBinding(ctx, store.LineageBindingRecord{
		WorkflowID: workflowID,
		RunID:      runID,
		LineageID:  lineageID,
		AttemptID:  accept.ProvisionalAttemptID,
		RuntimeID:  e.DescriptorValue.RuntimeID,
		SessionID:  importedSessionID(env, task),
		State:      string(fwfmp.AttemptStateRunning),
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

func (e *RuntimeEndpoint) rememberProjection(attemptID string, projection fwfmp.CapabilityEnvelope) {
	if e == nil || strings.TrimSpace(attemptID) == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.projections == nil {
		e.projections = map[string]fwfmp.CapabilityEnvelope{}
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

func (e *RuntimeEndpoint) projectionForAttempt(attemptID string) fwfmp.CapabilityEnvelope {
	if e == nil || strings.TrimSpace(attemptID) == "" {
		return fwfmp.CapabilityEnvelope{}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.projections == nil {
		return fwfmp.CapabilityEnvelope{}
	}
	return e.projections[attemptID]
}

func importedSessionID(env *contextdata.Envelope, task *core.Task) string {
	if env != nil {
		if val, ok := env.GetWorkingValue(rexkeys.GatewaySessionID); ok {
			if value := strings.TrimSpace(fmt.Sprint(val)); value != "" {
				return value
			}
		}
		if val, ok := env.GetWorkingValue(rexkeys.SessionID); ok {
			if value := strings.TrimSpace(fmt.Sprint(val)); value != "" {
				return value
			}
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
