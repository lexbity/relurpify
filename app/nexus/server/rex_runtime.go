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

	relruntime "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/ayenitd"
	relconfig "codeburg.org/lexbit/relurpify/framework/config"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
	memdb "codeburg.org/lexbit/relurpify/framework/memory/db"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/named/rex"
	rexcontrolplane "codeburg.org/lexbit/relurpify/named/rex/controlplane"
	rexnexus "codeburg.org/lexbit/relurpify/named/rex/nexus"
	rexreconcile "codeburg.org/lexbit/relurpify/named/rex/reconcile"
	rexctx "codeburg.org/lexbit/relurpify/named/rex/rexctx"
	"codeburg.org/lexbit/relurpify/named/rex/rexkeys"
	rexruntime "codeburg.org/lexbit/relurpify/named/rex/runtime"
	fwfmp "codeburg.org/lexbit/relurpify/relurpnet/fmp"
)

const rexCapabilityID = "nexus.runtime.rex.execute"

type RexRuntimeProvider struct {
	Agent           *rex.Agent
	Adapter         *rexnexus.Adapter
	SnapshotStore   *rexnexus.SnapshotStore
	LineageBridge   *rexnexus.LineageBridge
	RuntimeEndpoint *rexnexus.RuntimeEndpoint
	Packager        fwfmp.ContextPackager
	WorkflowStore   *memdb.SQLiteWorkflowStateStore
	RuntimeStore    *memdb.SQLiteRuntimeMemoryStore
	CheckpointStore *memdb.SQLiteCheckpointStore
	Bundle          *relruntime.CapabilityBundle
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
	paths := relconfig.New(workspace)
	if err := os.MkdirAll(paths.MemoryDir(), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(paths.SessionsDir(), 0o755); err != nil {
		return nil, err
	}
	workflowStore, err := memdb.NewSQLiteWorkflowStateStore(paths.WorkflowStateFile())
	if err != nil {
		return nil, err
	}
	runtimeStore, err := memdb.NewSQLiteRuntimeMemoryStore(filepath.Join(paths.MemoryDir(), "runtime_memory.db"))
	if err != nil {
		_ = workflowStore.Close()
		return nil, err
	}
	checkpointStore := memdb.NewSQLiteCheckpointStoreWithEvents(workflowStore.DB(), workflowStore, "", "")
	runner := sandbox.NewLocalCommandRunner(workspace, nil)
	bundle, err := relruntime.BuildBuiltinCapabilityBundle(workspace, runner, relruntime.CapabilityRegistryOptions{
		Context: ctx,
	})
	if err != nil {
		_ = runtimeStore.Close()
		_ = workflowStore.Close()
		return nil, err
	}
	agent := rex.NewWithWorkspace(ayenitd.WorkspaceEnvironment{
		Registry:     bundle.Registry,
		IndexManager: bundle.IndexManager,
		SearchEngine: bundle.SearchEngine,
		Memory:       memory.NewCompositeRuntimeStore(workflowStore, runtimeStore, checkpointStore),
		Config:       &core.Config{Name: "rex"},
	}, workspace)
	agent.Runtime.Start(ctx)
	provider := &RexRuntimeProvider{
		Agent:           agent,
		Adapter:         agent.ManagedAdapter(),
		SnapshotStore:   &rexnexus.SnapshotStore{WorkflowStore: workflowStore},
		WorkflowStore:   workflowStore,
		RuntimeStore:    runtimeStore,
		CheckpointStore: checkpointStore,
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
	if p.RuntimeStore != nil {
		_ = p.RuntimeStore.Close()
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
		if err == nil && ok && workflow != nil {
			var run *memory.WorkflowRunRecord
			if strings.TrimSpace(projection.RunID) != "" {
				candidate, ok, err := p.WorkflowStore.GetRun(ctx, projection.RunID)
				if err == nil && ok {
					run = candidate
				}
			}
			metadata := rexcontrolplane.BuildDRMetadata(*workflow, run)
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
		Service:             service,
		WorkflowStore:       p.WorkflowStore,
		LineageBindingStore: p.WorkflowStore,
		RuntimeID:           p.runtimeDescriptor().RuntimeID,
		PolicyResolver:      p.TrustedResolver,
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
				State:  attempt.State,
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
		Schedule: func(ctx context.Context, workflowID, runID string, task *core.Task, state *core.Context) error {
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
	return service.PublishLocalTrustBundle(ctx, p.runtimeDescriptor().TrustDomain, p.runtimeDescriptor().TrustDomain+":nexus", []core.RecipientKeyAdvertisement{
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

func (p *RexRuntimeProvider) InvokeCapability(ctx context.Context, sessionKey string, principalTenantID string, args map[string]any) (*core.CapabilityExecutionResult, error) {
	if p == nil || p.Adapter == nil {
		return nil, fmt.Errorf("rex runtime unavailable")
	}
	task, state, err := rexTaskFromArgs(args)
	if err != nil {
		return nil, err
	}
	if sessionKey != "" {
		state.Set(rexkeys.GatewaySessionID, sessionKey)
	}
	if principalTenantID != "" {
		state.Set(rexkeys.GatewayTenantID, principalTenantID)
	}
	result, err := p.Adapter.Invoke(ctx, task, state)
	out := &core.CapabilityExecutionResult{Success: err == nil, Data: map[string]any{}}
	if result != nil {
		out.Success = result.Success
		out.Data = result.Data
		out.Metadata = result.Metadata
		if result.Error != nil {
			out.Error = result.Error.Error()
		}
	}
	if err != nil {
		out.Error = err.Error()
	}
	return out, err
}

func rexTaskFromArgs(args map[string]any) (*core.Task, *core.Context, error) {
	instruction := strings.TrimSpace(stringValue(args["instruction"]))
	if instruction == "" {
		return nil, nil, fmt.Errorf("instruction required")
	}
	task := &core.Task{
		ID:          strings.TrimSpace(stringValue(args["task_id"])),
		Type:        core.TaskType(strings.TrimSpace(stringValue(args["task_type"]))),
		Instruction: instruction,
		Context:     mapStringAny(args["context"]),
		Metadata:    mapStringString(args["metadata"]),
	}
	if task.Type == "" {
		task.Type = core.TaskTypeCodeGeneration
	}
	state := core.NewContext()
	for key, value := range task.Context {
		state.Set(key, value)
	}
	if workflowID := strings.TrimSpace(stringValue(args[rexkeys.WorkflowID])); workflowID != "" {
		state.Set(rexkeys.WorkflowID, workflowID)
		state.Set(rexkeys.RexWorkflowID, workflowID)
	}
	if runID := strings.TrimSpace(stringValue(args[rexkeys.RunID])); runID != "" {
		state.Set(rexkeys.RunID, runID)
		state.Set(rexkeys.RexRunID, runID)
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
	raw, ok := value.(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make(map[string]string, len(raw))
	for key, entry := range raw {
		if text := strings.TrimSpace(stringValue(entry)); text != "" {
			out[key] = text
		}
	}
	return out
}

func (p *RexRuntimeProvider) runtimeDescriptor() core.RuntimeDescriptor {
	return core.RuntimeDescriptor{
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

func (p *RexRuntimeProvider) RuntimeDescriptor(context.Context) (core.RuntimeDescriptor, error) {
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

func rexnexusWorkItem(workflowID, runID string, task *core.Task, state *core.Context, agent *rex.Agent) rexruntime.WorkItem {
	return rexruntime.WorkItem{
		WorkflowID: workflowID,
		RunID:      runID,
		Task:       task,
		State:      state,
		Execute: func(ctx context.Context, item rexruntime.WorkItem) error {
			_, err := agent.Execute(ctx, item.Task, item.State)
			return err
		},
	}
}
