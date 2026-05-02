package server

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	relmanifest "codeburg.org/lexbit/relurpify/framework/manifest"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/named/rex"
	rexcontrolplane "codeburg.org/lexbit/relurpify/named/rex/controlplane"
	rexnexus "codeburg.org/lexbit/relurpify/named/rex/nexus"
	rexreconcile "codeburg.org/lexbit/relurpify/named/rex/reconcile"
	rexctx "codeburg.org/lexbit/relurpify/named/rex/rexctx"
	"codeburg.org/lexbit/relurpify/named/rex/rexkeys"
	rexruntime "codeburg.org/lexbit/relurpify/named/rex/runtime"
	rexstore "codeburg.org/lexbit/relurpify/named/rex/store"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	fwfmp "codeburg.org/lexbit/relurpify/relurpnet/fmp"
)

const rexCapabilityID = "nexus.runtime.rex.execute"

// WorkflowStore is a stub for the deleted framework/memory/db type
type WorkflowStore struct{}

func (s *WorkflowStore) Close() error { return nil }
func (s *WorkflowStore) GetWorkflow(ctx context.Context, id string) (memory.WorkflowRecord, bool, error) {
	return memory.WorkflowRecord{}, false, nil
}
func (s *WorkflowStore) GetRun(ctx context.Context, id string) (memory.WorkflowRunRecord, bool, error) {
	return memory.WorkflowRunRecord{}, false, nil
}

// FindLineageBindingsByLineageID implements nexus.LineageBindingStore
func (s *WorkflowStore) FindLineageBindingsByLineageID(ctx context.Context, lineageID string) ([]rexstore.LineageBindingRecord, error) {
	return nil, nil
}

// FindLineageBindingsByAttemptID implements nexus.LineageBindingStore
func (s *WorkflowStore) FindLineageBindingsByAttemptID(ctx context.Context, attemptID string) ([]rexstore.LineageBindingRecord, error) {
	return nil, nil
}

// UpsertLineageBinding implements nexus.LineageBindingStore
func (s *WorkflowStore) UpsertLineageBinding(ctx context.Context, record rexstore.LineageBindingRecord) error {
	return nil
}

// CreateWorkflow creates a new workflow record (stub).
func (s *WorkflowStore) CreateWorkflow(ctx context.Context, workflow *memory.WorkflowRecord) error {
	return nil
}

// CreateRun creates a new run record (stub).
func (s *WorkflowStore) CreateRun(ctx context.Context, run *memory.WorkflowRunRecord) error {
	return nil
}

// ListWorkflows lists all workflows (stub).
func (s *WorkflowStore) ListWorkflows(ctx context.Context) ([]memory.WorkflowRecord, error) {
	return nil, nil
}

// ListWorkflowArtifacts lists artifacts for a workflow (stub).
func (s *WorkflowStore) ListWorkflowArtifacts(ctx context.Context, workflowID, runID string) ([]memory.WorkflowArtifactRecord, error) {
	return nil, nil
}

// AppendEvent appends an event to a run (stub).
func (s *WorkflowStore) AppendEvent(ctx context.Context, workflowID, runID string, event *rexstore.WorkflowEventRecord) error {
	return nil
}

// GetLineageBinding gets a lineage binding (stub).
func (s *WorkflowStore) GetLineageBinding(ctx context.Context, workflowID, runID string) (*rexstore.LineageBindingRecord, bool, error) {
	return nil, false, nil
}

// ListEvents lists events for a run (stub).
func (s *WorkflowStore) ListEvents(ctx context.Context, workflowID, runID string) ([]rexstore.WorkflowEventRecord, error) {
	return nil, nil
}

// CheckpointStore is a stub for the deleted framework/memory/db type
type CheckpointStore struct{}

type RexRuntimeProvider struct {
	Agent           *rex.Agent
	Adapter         *rexnexus.Adapter
	SnapshotStore   *rexnexus.SnapshotStore
	LineageBridge   *rexnexus.LineageBridge
	RuntimeEndpoint *rexnexus.RuntimeEndpoint
	Packager        fwfmp.ContextPackager
	WorkflowStore   *rexstore.SQLiteWorkflowStore
	CheckpointStore *CheckpointStore
	Bundle          *CapabilityBundle
	TrustedResolver rexctx.TrustedContextResolver
	EventBridge     interface{ Health() (bool, string) }
	// Phase 7.1: Admission control for gateway routing
	Admission         rexcontrolplane.AdmissionController
	AdmissionAudit    *rexcontrolplane.AuditLog
	PartitionDetector fwfmp.PartitionDetector
	sloMu             sync.Mutex
	sloSignals        rexcontrolplane.SLOSignals
	sloCachedAt       time.Time
	sloTTL            time.Duration
}

func NewRexRuntimeProvider(ctx context.Context, workspace string) (*RexRuntimeProvider, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil, fmt.Errorf("workspace required")
	}
	paths := relmanifest.New(workspace)
	if err := os.MkdirAll(paths.MemoryDir(), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(paths.SessionsDir(), 0o755); err != nil {
		return nil, err
	}
	workflowStore, err := rexstore.NewSQLiteWorkflowStore(filepath.Join(paths.MemoryDir(), "rex_workflow.db"))
	if err != nil {
		return nil, err
	}
	runner := sandbox.NewLocalCommandRunner(workspace, nil)
	bundle, err := BuildBuiltinCapabilityBundle(workspace, runner, CapabilityRegistryOptions{
		Context: ctx,
	})
	if err != nil {
		_ = workflowStore.Close()
		return nil, err
	}
	agent := rex.NewWithWorkspace(&agentenv.WorkspaceEnvironment{
		Registry:      bundle.Registry,
		IndexManager:  bundle.IndexManager,
		SearchEngine:  bundle.SearchEngine,
		WorkingMemory: memory.NewWorkingMemoryStore(),
		Config:        &core.Config{Name: "rex"},
	}, workspace)
	agent.Runtime.Start(ctx)
	provider := &RexRuntimeProvider{
		Agent:           agent,
		Adapter:         agent.ManagedAdapter(),
		SnapshotStore:   &rexnexus.SnapshotStore{WorkflowStore: workflowStore},
		WorkflowStore:   workflowStore,
		CheckpointStore: &CheckpointStore{},
		Bundle:          bundle,
		TrustedResolver: &rexctx.DefaultTrustedContextResolver{},
		// Phase 7.1: Initialize admission controller with default capacity and fairness quotas
		Admission: &rexcontrolplane.LoadController{
			Capacity: 100, // Default max concurrent workflows
			InFlight: 0,
			Fairness: &rexcontrolplane.FairnessController{
				Limits: map[string]int{}, // Per-tenant quotas, defaults to 1 per tenant
				Usage:  map[string]int{},
			},
		},
		AdmissionAudit: &rexcontrolplane.AuditLog{},
		sloTTL:         10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		provider.Close()
	}()
	return provider, nil
}

func (p *RexRuntimeProvider) Close() {
	if p == nil {
		return
	}
	if p.Agent != nil && p.Agent.Runtime != nil {
		p.Agent.Runtime.Stop()
	}
	if p.Bundle != nil && p.Bundle.IndexManager != nil {
		_ = p.Bundle.IndexManager.Close()
	}
	if p.WorkflowStore != nil {
		_ = p.WorkflowStore.Close()
	}
}

func (p *RexRuntimeProvider) Registration() rexnexus.Registration {
	if p == nil || p.Adapter == nil {
		return rexnexus.Registration{}
	}
	return p.Adapter.Registration()
}

func (p *RexRuntimeProvider) RuntimeProjection() rexnexus.Projection {
	if p == nil || p.Adapter == nil {
		return rexnexus.Projection{}
	}
	projection := p.Agent.RuntimeProjection()
	if p.WorkflowStore != nil && strings.TrimSpace(projection.WorkflowID) != "" {
		ctx := context.Background()
		workflow, ok, err := p.WorkflowStore.GetWorkflow(ctx, projection.WorkflowID)
		if err == nil && ok {
			var run *memory.WorkflowRunRecord
			if strings.TrimSpace(projection.RunID) != "" {
				candidate, ok, err := p.WorkflowStore.GetRun(ctx, projection.RunID)
				if err == nil && ok {
					run = &candidate
				}
			}
			metadata := rexcontrolplane.BuildDRMetadata(workflow, run)
			projection.FailoverReady = metadata.FailoverReady
			projection.RecoveryState = metadata.RecoveryState
			projection.RuntimeVersion = metadata.RuntimeVersion
			projection.LastCheckpoint = metadata.LastCheckpoint
		}
	}
	if p.PartitionDetector != nil && p.PartitionDetector.IsPartitioned() {
		projection.Health = rexruntime.HealthDegraded
		if projection.LastError == "" {
			projection.LastError = "ownership store partitioned"
		}
	}
	if p.EventBridge != nil {
		healthy, lastError := p.EventBridge.Health()
		if !healthy {
			projection.Health = rexruntime.HealthDegraded
			if strings.TrimSpace(lastError) != "" {
				projection.LastError = lastError
			} else if projection.LastError == "" {
				projection.LastError = "event bridge unhealthy"
			}
		}
	}
	return projection
}

func (p *RexRuntimeProvider) AdminSnapshot(ctx context.Context) (rexnexus.AdminSnapshot, error) {
	if p == nil || p.Adapter == nil {
		return rexnexus.AdminSnapshot{}, fmt.Errorf("rex runtime unavailable")
	}
	return p.Adapter.AdminSnapshot(ctx)
}

func (p *RexRuntimeProvider) AttachFMPService(service *fwfmp.Service) {
	if p == nil || service == nil {
		return
	}
	if service.PartitionDetector == nil {
		service.PartitionDetector = &fwfmp.AtomicPartitionState{}
	}
	p.PartitionDetector = service.PartitionDetector
	if p.Agent != nil && p.Agent.Runtime != nil {
		p.Agent.Runtime.SetPartitionDetector(service.PartitionDetector)
	}
	recipient := p.runtimeRecipient()
	key := sha256.Sum256([]byte(recipient))
	mediationRecipient := fwfmp.QualifiedGatewayRecipient(p.runtimeDescriptor().TrustDomain, p.runtimeDescriptor().NodeID)
	mediationKey := sha256.Sum256([]byte(mediationRecipient))
	keyResolver := &fwfmp.TrustBundleRecipientKeyResolver{
		Trust: service.Trust,
		Static: map[string][][]byte{
			recipient:          {key[:]},
			mediationRecipient: {mediationKey[:]},
		},
	}
	packager := fwfmp.JSONPackager{
		RuntimeStore:      p.SnapshotStore,
		KeyResolver:       keyResolver,
		Signer:            service.Signer,
		DefaultRecipients: []string{recipient},
		LocalRecipient:    recipient,
	}
	p.Packager = packager
	service.Packager = p.Packager
	service.Mediator = &fwfmp.MediationController{
		Packager:       packager,
		LocalRecipient: mediationRecipient,
	}
	p.LineageBridge = &rexnexus.LineageBridge{
		Service:        service,
		LifecycleRepo:  p.Agent.Environment.AgentLifecycle,
		RuntimeID:      p.runtimeDescriptor().RuntimeID,
		PolicyResolver: p.TrustedResolver,
	}
	p.Agent.Observer = p.LineageBridge
	p.Agent.Reconciler = &rexreconcile.FMPBackedReconciler{
		Base:           &rexreconcile.InMemoryReconciler{},
		ResolveBinding: p.LineageBridge.ResolveReconciliationBinding,
		ResolveAttempt: func(ctx context.Context, lineageID, attemptID string) (*rexreconcile.AttemptView, error) {
			attempt, ok, err := service.Ownership.GetAttempt(ctx, attemptID)
			if err != nil || !ok || attempt == nil || attempt.LineageID != lineageID {
				return nil, err
			}
			return &rexreconcile.AttemptView{
				State:  rexreconcile.AttemptState(attempt.State),
				Fenced: attempt.Fenced,
			}, nil
		},
		ApplyOutcome: p.LineageBridge.ApplyReconciliationOutcome,
	}
	p.RuntimeEndpoint = &rexnexus.RuntimeEndpoint{
		DescriptorValue:     p.runtimeDescriptor(),
		Packager:            packager,
		WorkflowStore:       p.WorkflowStore,
		LineageBindingStore: p.WorkflowStore,
		Schedule: func(ctx context.Context, workflowID, runID string, task *core.Task, state *contextdata.Envelope) error {
			item := rexnexusWorkItem(workflowID, runID, task, state, p.Agent)
			if !p.Agent.Runtime.Enqueue(item) {
				return fmt.Errorf("rex runtime queue full")
			}
			return nil
		},
	}
	service.Runtime = p.RuntimeEndpoint
}

func (p *RexRuntimeProvider) PublishFMPTrustBundle(ctx context.Context, service *fwfmp.Service) error {
	if p == nil || service == nil {
		return nil
	}
	recipient := p.runtimeRecipient()
	runtimeKey := sha256.Sum256([]byte(recipient))
	mediationRecipient := fwfmp.QualifiedGatewayRecipient(p.runtimeDescriptor().TrustDomain, p.runtimeDescriptor().NodeID)
	mediationKey := sha256.Sum256([]byte(mediationRecipient))
	return service.PublishLocalTrustBundle(ctx, p.runtimeDescriptor().TrustDomain, p.runtimeDescriptor().TrustDomain+":nexus", []fwfmp.RecipientKeyAdvertisement{
		{
			Recipient: recipient,
			KeyID:     "runtime",
			Version:   "v1",
			PublicKey: runtimeKey[:],
			Active:    true,
		},
		{
			Recipient: mediationRecipient,
			KeyID:     "gateway-mediation",
			Version:   "v1",
			PublicKey: mediationKey[:],
			Active:    true,
		},
	})
}

func (p *RexRuntimeProvider) ReadSLOSignals(ctx context.Context) (rexcontrolplane.SLOSignals, int64, error) {
	if p == nil || p.WorkflowStore == nil {
		return rexcontrolplane.SLOSignals{}, 0, fmt.Errorf("rex workflow store unavailable")
	}
	p.sloMu.Lock()
	defer p.sloMu.Unlock()
	now := time.Now().UTC()
	ttl := p.sloTTL
	if ttl <= 0 {
		ttl = 10 * time.Second
	}
	if !p.sloCachedAt.IsZero() && now.Sub(p.sloCachedAt) < ttl {
		return p.sloSignals, p.sloCachedAt.UnixNano(), nil
	}
	signals, err := rexcontrolplane.CollectSLOSignals(ctx, p.WorkflowStore, 1000)
	if err != nil {
		return rexcontrolplane.SLOSignals{}, 0, err
	}
	p.sloSignals = signals
	p.sloCachedAt = now
	return signals, now.UnixNano(), nil
}

func (p *RexRuntimeProvider) CapabilityDescriptor() core.CapabilityDescriptor {
	return core.NormalizeCapabilityDescriptor(core.CapabilityDescriptor{
		ID:            rexCapabilityID,
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Name:          "rex.execute",
		Version:       "v1alpha1",
		Description:   "Execute a task on the Nexus-managed Rex runtime",
		Category:      "runtime",
		Tags:          []string{"rex", "managed-runtime", "execute"},
		TrustClass:    core.TrustClassWorkspaceTrusted,
		InputSchema: &core.Schema{
			Type: "object",
			Properties: map[string]*core.Schema{
				"instruction":      {Type: "string", Description: "Task instruction for Rex"},
				"task_type":        {Type: "string", Description: "Optional core task type"},
				"task_id":          {Type: "string"},
				rexkeys.WorkflowID: {Type: "string"},
				rexkeys.RunID:      {Type: "string"},
				"context":          {Type: "object", Description: "Initial context state"},
				"metadata":         {Type: "object", Description: "String metadata for the task"},
			},
			Required: []string{"instruction"},
		},
		Annotations: map[string]any{
			"runtime_name": p.Registration().Name,
		},
	})
}

func (p *RexRuntimeProvider) InvokeCapability(ctx context.Context, sessionKey string, principalTenantID string, args map[string]any) (*contracts.CapabilityExecutionResult, error) {
	if p == nil || p.Adapter == nil {
		return nil, fmt.Errorf("rex runtime unavailable")
	}
	task, state, err := rexTaskFromArgs(args)
	if err != nil {
		return nil, err
	}
	if sessionKey != "" {
		state.SetWorkingValue(rexkeys.GatewaySessionID, sessionKey, contextdata.MemoryClassTask)
	}
	if principalTenantID != "" {
		state.SetWorkingValue(rexkeys.GatewayTenantID, principalTenantID, contextdata.MemoryClassTask)
	}
	result, err := p.Adapter.Invoke(ctx, task, state)
	out := &contracts.CapabilityExecutionResult{Success: err == nil, Data: map[string]any{}}
	if result != nil {
		out.Success = result.Success
		out.Data = result.Data
		out.Metadata = result.Metadata
		if strings.TrimSpace(result.Error) != "" {
			out.Error = result.Error
		}
	}
	if err != nil {
		out.Error = err.Error()
	}
	return out, err
}

func rexTaskFromArgs(args map[string]any) (*core.Task, *contextdata.Envelope, error) {
	instruction := strings.TrimSpace(stringValue(args["instruction"]))
	if instruction == "" {
		return nil, nil, fmt.Errorf("instruction required")
	}
	task := &core.Task{
		ID:          strings.TrimSpace(stringValue(args["task_id"])),
		Type:        strings.TrimSpace(stringValue(args["task_type"])),
		Instruction: instruction,
		Context:     mapStringAny(args["context"]),
		Metadata:    mapStringAny(args["metadata"]),
	}
	if task.Type == "" {
		task.Type = string(core.TaskTypeCodeGeneration)
	}
	state := contextdata.NewEnvelope(task.ID, "")
	for key, value := range task.Context {
		state.SetWorkingValue(key, value, contextdata.MemoryClassTask)
	}
	if workflowID := strings.TrimSpace(stringValue(args[rexkeys.WorkflowID])); workflowID != "" {
		state.SetWorkingValue(rexkeys.WorkflowID, workflowID, contextdata.MemoryClassTask)
		state.SetWorkingValue(rexkeys.RexWorkflowID, workflowID, contextdata.MemoryClassTask)
	}
	if runID := strings.TrimSpace(stringValue(args[rexkeys.RunID])); runID != "" {
		state.SetWorkingValue(rexkeys.RunID, runID, contextdata.MemoryClassTask)
		state.SetWorkingValue(rexkeys.RexRunID, runID, contextdata.MemoryClassTask)
	}
	return task, state, nil
}

func stringValue(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func mapStringAny(value any) map[string]any {
	raw, ok := value.(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make(map[string]any, len(raw))
	for key, entry := range raw {
		out[key] = entry
	}
	return out
}

func mapStringString(value any) map[string]string {
	raw, ok := value.(map[string]string)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make(map[string]string, len(raw))
	for key, entry := range raw {
		if text := strings.TrimSpace(entry); text != "" {
			out[key] = text
		}
	}
	return out
}

func (p *RexRuntimeProvider) runtimeDescriptor() fwfmp.RuntimeDescriptor {
	return fwfmp.RuntimeDescriptor{
		RuntimeID:                 "rex",
		NodeID:                    "nexus",
		TrustDomain:               "local",
		RuntimeVersion:            "rex.v1",
		SupportedContextClasses:   []string{"workflow-runtime"},
		SupportedEncryptionSuites: []string{"aes256-gcm+aes256-gcm-wrap.v1"},
		CompatibilityClass:        "rex.workflow.v1",
		MaxContextSize:            8 << 20,
		MaxConcurrentResumes:      8,
	}
}

func (p *RexRuntimeProvider) RuntimeDescriptor(context.Context) (fwfmp.RuntimeDescriptor, error) {
	if p != nil && p.RuntimeEndpoint != nil {
		if descriptor, err := p.RuntimeEndpoint.Descriptor(context.Background()); err == nil {
			return descriptor, nil
		}
	}
	return p.runtimeDescriptor(), nil
}

func (p *RexRuntimeProvider) runtimeRecipient() string {
	descriptor := p.runtimeDescriptor()
	return "runtime://" + descriptor.TrustDomain + "/" + descriptor.RuntimeID
}

func rexnexusWorkItem(workflowID, runID string, task *core.Task, state *contextdata.Envelope, agent *rex.Agent) rexruntime.WorkItem {
	return rexruntime.WorkItem{
		WorkflowID: workflowID,
		RunID:      runID,
		Task:       task,
		Envelope:   state,
		Execute: func(ctx context.Context, item rexruntime.WorkItem) error {
			_, err := agent.Execute(ctx, item.Task, item.Envelope)
			return err
		},
	}
}
