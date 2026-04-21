package authorization

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
)

var ErrDelegationNotFound = errors.New("delegation not found")

type DelegationCapabilityRegistry interface {
	CapturePolicySnapshot() *core.PolicySnapshot
	GetCoordinationTarget(idOrName string) (core.CapabilityDescriptor, bool)
	CoordinationTargets(selectors ...core.CapabilitySelector) []core.CapabilityDescriptor
	InvokeCapability(ctx context.Context, state *core.Context, idOrName string, args map[string]interface{}) (*core.ToolResult, error)
}

type BackgroundDelegationOutcome struct {
	Result *core.ToolResult
	Error  error
}

type BackgroundDelegationHandle struct {
	ProviderID     string
	SessionID      string
	Recoverability core.RecoverabilityMode
	Results        <-chan BackgroundDelegationOutcome
	Cancel         func(context.Context, core.DelegationSnapshot) error
}

type DelegationBackgroundRunner interface {
	StartBackgroundDelegation(ctx context.Context, request core.DelegationRequest, target core.CapabilityDescriptor, args map[string]any, opts DelegationExecutionOptions) (*BackgroundDelegationHandle, error)
}

type DelegationExecutionOptions struct {
	Registry         DelegationCapabilityRegistry
	BackgroundRunner DelegationBackgroundRunner
	AgentSpec        *core.AgentRuntimeSpec
	State            *core.Context
	WorkflowStore    memory.WorkflowStateStore
	WorkflowRunID    string
	WorkflowStepID   string
	CallerAgentID    string
	CallerTrust      core.TrustClass
	Recoverability   core.RecoverabilityMode
	Background       bool
	Metadata         map[string]any
}

type DelegationStartOptions struct {
	TrustClass     core.TrustClass
	Recoverability core.RecoverabilityMode
	Background     bool
	PolicySnapshot *core.PolicySnapshot
	Metadata       map[string]any
	OnCancel       func(context.Context, core.DelegationSnapshot) error
}

type DelegationManager struct {
	mu          sync.RWMutex
	delegations map[string]*delegationRecord
	observer    func(core.DelegationSnapshot)
}

type delegationRecord struct {
	snapshot core.DelegationSnapshot
	cancel   func(context.Context, core.DelegationSnapshot) error
}

func NewDelegationManager() *DelegationManager {
	return &DelegationManager{
		delegations: map[string]*delegationRecord{},
	}
}

func (m *DelegationManager) SetObserver(observer func(core.DelegationSnapshot)) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observer = observer
}

func (m *DelegationManager) ExecuteDelegation(ctx context.Context, request core.DelegationRequest, opts DelegationExecutionOptions) (*core.DelegationSnapshot, error) {
	if m == nil {
		return nil, fmt.Errorf("delegation manager unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.Registry == nil {
		return nil, fmt.Errorf("delegation registry required")
	}
	coordination := core.EffectiveCoordination(opts.AgentSpec)
	target, err := resolveDelegationTarget(request, opts.Registry, coordination)
	if err != nil {
		return nil, err
	}
	if err := validateDelegationTargetPolicy(request, target, coordination); err != nil {
		return nil, err
	}
	request = cloneDelegationRequest(request)
	request.CallerAgentID = firstNonEmpty(request.CallerAgentID, opts.CallerAgentID)
	request.TargetCapabilityID = target.ID
	request.TargetProviderID = firstNonEmpty(request.TargetProviderID, target.Source.ProviderID)
	request.TargetSessionID = firstNonEmpty(request.TargetSessionID, target.Source.SessionID)
	request.ResourceRefs = resolveDelegationResourceRefs(request, target, opts)
	args, err := buildDelegationInvocationArgs(ctx, request, target, opts)
	if err != nil {
		return nil, err
	}

	policySnapshot := opts.Registry.CapturePolicySnapshot()
	runBackground := shouldRunDelegationInBackground(target, opts.Background)
	if runBackground {
		if opts.BackgroundRunner == nil {
			return nil, fmt.Errorf("background delegation runner required for %s", target.Name)
		}
		handle, err := opts.BackgroundRunner.StartBackgroundDelegation(ctx, request, target, args, opts)
		if err != nil {
			return nil, err
		}
		request.TargetProviderID = firstNonEmpty(request.TargetProviderID, handle.ProviderID)
		request.TargetSessionID = firstNonEmpty(request.TargetSessionID, handle.SessionID)
		started, err := m.StartDelegation(ctx, request, DelegationStartOptions{
			TrustClass:     target.TrustClass,
			Recoverability: effectiveDelegationRecoverability(firstRecoverability(handle.Recoverability, opts.Recoverability)),
			Background:     true,
			PolicySnapshot: policySnapshot,
			Metadata: mergeAnyMaps(opts.Metadata, map[string]any{
				"target_role":   string(target.Coordination.Role),
				"task_type":     request.TaskType,
				"resource_refs": append([]string{}, request.ResourceRefs...),
			}),
			OnCancel: handle.Cancel,
		})
		if err != nil {
			return nil, err
		}
		go m.awaitBackgroundDelegation(started.Request.ID, request, target, handle, policySnapshot, opts)
		return started, nil
	}
	started, err := m.StartDelegation(ctx, request, DelegationStartOptions{
		TrustClass:     target.TrustClass,
		Recoverability: effectiveDelegationRecoverability(opts.Recoverability),
		Background:     opts.Background,
		PolicySnapshot: policySnapshot,
		Metadata: mergeAnyMaps(opts.Metadata, map[string]any{
			"target_role":   string(target.Coordination.Role),
			"task_type":     request.TaskType,
			"resource_refs": append([]string{}, request.ResourceRefs...),
		}),
	})
	if err != nil {
		return nil, err
	}
	result, invokeErr := opts.Registry.InvokeCapability(ctx, effectiveDelegationState(opts.State), target.ID, args)
	delegationResult := buildDelegationResult(request, target, result, invokeErr, policySnapshot, opts.AgentSpec, opts.CallerTrust)
	completed, completeErr := m.CompleteDelegation(started.Request.ID, delegationResult)
	if completeErr != nil {
		return nil, completeErr
	}
	if invokeErr != nil {
		return completed, invokeErr
	}
	return completed, nil
}

func (m *DelegationManager) StartDelegation(ctx context.Context, request core.DelegationRequest, opts DelegationStartOptions) (*core.DelegationSnapshot, error) {
	if m == nil {
		return nil, fmt.Errorf("delegation manager unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	request = cloneDelegationRequest(request)
	if request.CreatedAt.IsZero() {
		request.CreatedAt = time.Now().UTC()
	}
	if request.PolicySnapshotID == "" && opts.PolicySnapshot != nil {
		request.PolicySnapshotID = opts.PolicySnapshot.ID
	}
	snapshot := core.DelegationSnapshot{
		Request:        request,
		State:          core.DelegationStateRunning,
		TrustClass:     opts.TrustClass,
		Recoverability: opts.Recoverability,
		Background:     opts.Background,
		Metadata:       cloneAnyMap(opts.Metadata),
		StartedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	if err := snapshot.Validate(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	if _, exists := m.delegations[snapshot.Request.ID]; exists {
		m.mu.Unlock()
		return nil, fmt.Errorf("delegation %s already exists", snapshot.Request.ID)
	}
	m.delegations[snapshot.Request.ID] = &delegationRecord{
		snapshot: snapshot,
		cancel:   opts.OnCancel,
	}
	observer := m.observer
	m.mu.Unlock()
	out := cloneDelegationSnapshot(snapshot)
	if observer != nil {
		observer(out)
	}
	return &out, nil
}

func (m *DelegationManager) CompleteDelegation(id string, result *core.DelegationResult) (*core.DelegationSnapshot, error) {
	if m == nil {
		return nil, fmt.Errorf("delegation manager unavailable")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("delegation id required")
	}
	if result == nil {
		return nil, fmt.Errorf("delegation result required")
	}
	candidate := cloneDelegationResult(result)
	if candidate.DelegationID == "" {
		candidate.DelegationID = id
	}
	if err := candidate.Validate(); err != nil {
		return nil, err
	}
	switch candidate.State {
	case core.DelegationStateSucceeded, core.DelegationStateFailed, core.DelegationStateCancelled:
	default:
		return nil, fmt.Errorf("delegation result state %s not terminal", candidate.State)
	}
	m.mu.Lock()
	record, ok := m.delegations[id]
	if !ok {
		m.mu.Unlock()
		return nil, ErrDelegationNotFound
	}
	if record.snapshot.State == core.DelegationStateSucceeded || record.snapshot.State == core.DelegationStateFailed || record.snapshot.State == core.DelegationStateCancelled {
		out := cloneDelegationSnapshot(record.snapshot)
		m.mu.Unlock()
		return &out, nil
	}
	record.snapshot.Result = &candidate
	record.snapshot.State = candidate.State
	record.snapshot.UpdatedAt = time.Now().UTC()
	record.cancel = nil
	observer := m.observer
	m.mu.Unlock()
	out := cloneDelegationSnapshot(record.snapshot)
	if observer != nil {
		observer(out)
	}
	return &out, nil
}

func (m *DelegationManager) CancelDelegation(ctx context.Context, id, reason string) (*core.DelegationSnapshot, error) {
	if m == nil {
		return nil, fmt.Errorf("delegation manager unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("delegation id required")
	}
	m.mu.Lock()
	record, ok := m.delegations[id]
	if !ok {
		m.mu.Unlock()
		return nil, ErrDelegationNotFound
	}
	snapshot := cloneDelegationSnapshot(record.snapshot)
	cancelHook := record.cancel
	m.mu.Unlock()

	if cancelHook != nil {
		if err := cancelHook(ctx, snapshot); err != nil {
			return nil, err
		}
	}

	result := core.NewDelegationResult(
		snapshot.Request,
		snapshot.Request.TargetCapabilityID,
		snapshot.Request.TargetProviderID,
		snapshot.Request.TargetSessionID,
		snapshot.TrustClass,
		core.DelegationStateCancelled,
		false,
		map[string]any{"reason": strings.TrimSpace(reason)},
		&core.PolicySnapshot{ID: snapshot.Request.PolicySnapshotID},
	)
	if trimmed := strings.TrimSpace(reason); trimmed != "" {
		result.Diagnostics = []string{trimmed}
	}
	return m.CompleteDelegation(id, result)
}

func (m *DelegationManager) GetDelegation(id string) (*core.DelegationSnapshot, error) {
	if m == nil {
		return nil, fmt.Errorf("delegation manager unavailable")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("delegation id required")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	record, ok := m.delegations[id]
	if !ok {
		return nil, ErrDelegationNotFound
	}
	out := cloneDelegationSnapshot(record.snapshot)
	return &out, nil
}

func (m *DelegationManager) ListDelegations(filter core.DelegationFilter) []core.DelegationSnapshot {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]core.DelegationSnapshot, 0, len(m.delegations))
	for _, record := range m.delegations {
		if !delegationMatchesFilter(record.snapshot, filter) {
			continue
		}
		out = append(out, cloneDelegationSnapshot(record.snapshot))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].Request.ID < out[j].Request.ID
		}
		return out[i].StartedAt.Before(out[j].StartedAt)
	})
	return out
}

func (m *DelegationManager) SnapshotDelegations() []core.DelegationSnapshot {
	return m.ListDelegations(core.DelegationFilter{})
}

func (m *DelegationManager) PersistDelegations(ctx context.Context, store memory.WorkflowStateStore, workflowID, runID string) error {
	if m == nil {
		return fmt.Errorf("delegation manager unavailable")
	}
	if store == nil {
		return fmt.Errorf("workflow state store required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for _, snapshot := range m.SnapshotDelegations() {
		if strings.TrimSpace(workflowID) != "" && snapshot.Request.WorkflowID != "" && snapshot.Request.WorkflowID != workflowID {
			continue
		}
		record := memory.WorkflowDelegationRecord{
			DelegationID:   snapshot.Request.ID,
			WorkflowID:     firstNonEmpty(snapshot.Request.WorkflowID, workflowID),
			RunID:          strings.TrimSpace(runID),
			TaskID:         snapshot.Request.TaskID,
			State:          snapshot.State,
			TrustClass:     snapshot.TrustClass,
			Recoverability: snapshot.Recoverability,
			Background:     snapshot.Background,
			Request:        snapshot.Request,
			Result:         snapshot.Result,
			Metadata:       cloneAnyMap(snapshot.Metadata),
			StartedAt:      snapshot.StartedAt,
			UpdatedAt:      snapshot.UpdatedAt,
		}
		if err := store.UpsertDelegation(ctx, record); err != nil {
			return err
		}
		transition := memory.WorkflowDelegationTransitionRecord{
			TransitionID: delegationTransitionID(snapshot),
			DelegationID: snapshot.Request.ID,
			WorkflowID:   record.WorkflowID,
			RunID:        record.RunID,
			ToState:      snapshot.State,
			Metadata: map[string]any{
				"target_capability_id": snapshot.Request.TargetCapabilityID,
				"target_provider_id":   snapshot.Request.TargetProviderID,
				"target_session_id":    snapshot.Request.TargetSessionID,
			},
			CreatedAt: snapshot.UpdatedAt,
		}
		if snapshot.Result != nil {
			transition.Metadata["has_result"] = true
		}
		if err := store.AppendDelegationTransition(ctx, transition); err != nil {
			return err
		}
		if artifact := promotedDelegationArtifact(snapshot, record.WorkflowID, runID); artifact != nil {
			if err := store.UpsertWorkflowArtifact(ctx, *artifact); err != nil {
				return err
			}
		}
	}
	return nil
}

func resolveDelegationTarget(request core.DelegationRequest, registry DelegationCapabilityRegistry, coordination core.AgentCoordinationSpec) (core.CapabilityDescriptor, error) {
	if registry == nil {
		return core.CapabilityDescriptor{}, fmt.Errorf("delegation registry required")
	}
	targetID := strings.TrimSpace(request.TargetCapabilityID)
	if targetID != "" {
		target, ok := registry.GetCoordinationTarget(targetID)
		if !ok {
			return core.CapabilityDescriptor{}, fmt.Errorf("delegation target %s not admitted", targetID)
		}
		return target, nil
	}
	candidates := registry.CoordinationTargets(coordination.DelegationTargetSelectors...)
	for _, candidate := range candidates {
		if !core.SelectorMatchesDescriptor(core.CapabilitySelector{
			CoordinationTaskTypes: []string{request.TaskType},
		}, candidate) {
			continue
		}
		if role := delegationRequestedRole(request); role != "" && (candidate.Coordination == nil || candidate.Coordination.Role != role) {
			continue
		}
		return candidate, nil
	}
	if role := delegationRequestedRole(request); role != "" {
		return core.CapabilityDescriptor{}, fmt.Errorf("no admitted delegation target for task type %s and role %s", request.TaskType, role)
	}
	return core.CapabilityDescriptor{}, fmt.Errorf("no admitted delegation target for task type %s", request.TaskType)
}

func validateDelegationTargetPolicy(request core.DelegationRequest, target core.CapabilityDescriptor, coordination core.AgentCoordinationSpec) error {
	if target.Coordination == nil || !target.Coordination.Target {
		return fmt.Errorf("capability %s is not a coordination target", target.ID)
	}
	if coordination.MaxDelegationDepth > 0 && request.Depth > coordination.MaxDelegationDepth {
		return fmt.Errorf("delegation depth %d exceeds max %d", request.Depth, coordination.MaxDelegationDepth)
	}
	if target.RuntimeFamily == core.CapabilityRuntimeFamilyProvider && target.Source.Scope == core.CapabilityScopeRemote && !coordination.AllowRemoteDelegation {
		return fmt.Errorf("remote delegation to %s is not allowed", target.Name)
	}
	if target.Coordination.LongRunning && !coordination.AllowBackgroundDelegation {
		return fmt.Errorf("background delegation to %s is not allowed", target.Name)
	}
	if requestPrefersBackground(request) && !containsBackgroundExecutionMode(target.Coordination.ExecutionModes) && !target.Coordination.LongRunning {
		return fmt.Errorf("delegation target %s is not background-capable", target.Name)
	}
	if requestPrefersBackground(request) && !coordination.AllowBackgroundDelegation {
		return fmt.Errorf("session-backed or background delegation to %s is not allowed", target.Name)
	}
	return nil
}

func resolveDelegationResourceRefs(request core.DelegationRequest, target core.CapabilityDescriptor, opts DelegationExecutionOptions) []string {
	if len(request.ResourceRefs) > 0 {
		return dedupeStringSlice(request.ResourceRefs)
	}
	if opts.WorkflowStore == nil || strings.TrimSpace(request.WorkflowID) == "" || target.Coordination == nil {
		return nil
	}
	return memory.DefaultWorkflowProjectionRefs(request.WorkflowID, opts.WorkflowRunID, opts.WorkflowStepID, memory.WorkflowProjectionRole(target.Coordination.Role))
}

func buildDelegationInvocationArgs(ctx context.Context, request core.DelegationRequest, target core.CapabilityDescriptor, opts DelegationExecutionOptions) (map[string]any, error) {
	args := map[string]any{
		"instruction":   request.Instruction,
		"task_id":       request.TaskID,
		"workflow_id":   request.WorkflowID,
		"resource_refs": append([]string{}, request.ResourceRefs...),
	}
	for key, value := range request.Metadata {
		args[key] = value
	}
	summaries, err := projectDelegationResources(ctx, request.ResourceRefs, opts.WorkflowStore)
	if err != nil {
		return nil, err
	}
	role := core.CoordinationRole("")
	if target.Coordination != nil {
		role = target.Coordination.Role
	}
	switch role {
	case core.CoordinationRoleArchitect:
		args["context_summary"] = strings.Join(summaries, "\n\n")
	case core.CoordinationRoleReviewer:
		args["artifact_summary"] = strings.Join(summaries, "\n\n")
		args["acceptance_criteria"] = normalizeStringArray(args["acceptance_criteria"])
	case core.CoordinationRoleVerifier:
		args["artifact_summary"] = strings.Join(summaries, "\n\n")
		if criteria, ok := args["verification_criteria"]; ok {
			args["verification_criteria"] = normalizeStringArray(criteria)
		} else {
			args["verification_criteria"] = normalizeStringArray(args["acceptance_criteria"])
		}
	case core.CoordinationRoleExecutor:
		args["args"] = normalizeArgumentMap(args["args"])
	}
	return args, nil
}

func buildDelegationResult(request core.DelegationRequest, target core.CapabilityDescriptor, result *core.ToolResult, invokeErr error, snapshot *core.PolicySnapshot, spec *core.AgentRuntimeSpec, callerTrust core.TrustClass) *core.DelegationResult {
	if invokeErr != nil {
		failed := core.NewDelegationResult(request, target.ID, target.Source.ProviderID, target.Source.SessionID, target.TrustClass, core.DelegationStateFailed, false, nil, snapshot)
		failed.Diagnostics = []string{invokeErr.Error()}
		return failed
	}
	if result == nil {
		result = &core.ToolResult{Success: true}
	}
	state := core.DelegationStateSucceeded
	if !result.Success {
		state = core.DelegationStateFailed
	}
	delegationResult := core.NewDelegationResult(request, target.ID, target.Source.ProviderID, target.Source.SessionID, target.TrustClass, state, result.Success, result.Data, snapshot)
	if result.Error != "" {
		delegationResult.Diagnostics = append(delegationResult.Diagnostics, result.Error)
	}
	envelope := core.NewCapabilityResultEnvelope(target, result, core.ContentDispositionRaw, snapshot, core.ApprovalBindingFromDelegation(request, delegationResult))
	decision := core.EffectiveInsertionDecision(spec, envelope)
	if target.Coordination != nil && !target.Coordination.DirectInsertionAllowed && decision.Action == core.InsertionActionDirect {
		decision.Action = core.InsertionActionSummarized
		decision.Reason = "coordination target requires summarized insertion"
	}
	if requiresCrossTrustApproval(callerTrust, target.TrustClass, spec, request) {
		decision.Action = core.InsertionActionHITLRequired
		decision.RequiresHITL = true
		decision.Reason = "cross-trust delegation requires approval"
	}
	delegationResult.Provenance = envelope.Provenance
	delegationResult.Disposition = envelope.Disposition
	core.ApplyDelegationInsertionDecision(delegationResult, decision)
	return delegationResult
}

func (m *DelegationManager) awaitBackgroundDelegation(id string, request core.DelegationRequest, target core.CapabilityDescriptor, handle *BackgroundDelegationHandle, snapshot *core.PolicySnapshot, opts DelegationExecutionOptions) {
	if m == nil || handle == nil || handle.Results == nil {
		return
	}
	outcome, ok := <-handle.Results
	if !ok {
		outcome.Error = fmt.Errorf("background delegation session %s closed without result", handle.SessionID)
	}
	result := buildDelegationResult(request, target, outcome.Result, outcome.Error, snapshot, opts.AgentSpec, opts.CallerTrust)
	_, _ = m.CompleteDelegation(id, result)
}

func delegationMatchesFilter(snapshot core.DelegationSnapshot, filter core.DelegationFilter) bool {
	if filter.WorkflowID != "" && !strings.EqualFold(strings.TrimSpace(filter.WorkflowID), snapshot.Request.WorkflowID) {
		return false
	}
	if filter.TaskID != "" && !strings.EqualFold(strings.TrimSpace(filter.TaskID), snapshot.Request.TaskID) {
		return false
	}
	if filter.TargetCapabilityID != "" && !strings.EqualFold(strings.TrimSpace(filter.TargetCapabilityID), snapshot.Request.TargetCapabilityID) {
		return false
	}
	if filter.TargetProviderID != "" && !strings.EqualFold(strings.TrimSpace(filter.TargetProviderID), snapshot.Request.TargetProviderID) {
		return false
	}
	if filter.TargetSessionID != "" && !strings.EqualFold(strings.TrimSpace(filter.TargetSessionID), snapshot.Request.TargetSessionID) {
		return false
	}
	if len(filter.States) > 0 {
		match := false
		for _, state := range filter.States {
			if state == snapshot.State {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}
	return true
}

func projectDelegationResources(ctx context.Context, refs []string, store memory.WorkflowStateStore) ([]string, error) {
	if len(refs) == 0 || store == nil {
		return nil, nil
	}
	service := memory.WorkflowProjectionService{Store: store}
	summaries := make([]string, 0, len(refs))
	for _, ref := range refs {
		parsed, err := memory.ParseWorkflowResourceURI(ref)
		if err != nil {
			continue
		}
		resource, err := service.Project(ctx, parsed)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, summarizeResourceRead(resource))
	}
	return summaries, nil
}

func summarizeResourceRead(result *core.ResourceReadResult) string {
	if result == nil || len(result.Contents) == 0 {
		return ""
	}
	parts := make([]string, 0, len(result.Contents))
	for _, block := range result.Contents {
		switch typed := block.(type) {
		case core.TextContentBlock:
			text := strings.TrimSpace(typed.Text)
			if text != "" {
				parts = append(parts, text)
			}
		case core.StructuredContentBlock:
			encoded, err := json.Marshal(typed.Data)
			if err == nil && len(encoded) > 0 {
				parts = append(parts, string(encoded))
			}
		}
	}
	return strings.Join(parts, "\n")
}

func effectiveDelegationRecoverability(mode core.RecoverabilityMode) core.RecoverabilityMode {
	switch mode {
	case core.RecoverabilityEphemeral, core.RecoverabilityInProcess, core.RecoverabilityPersistedRestore:
		return mode
	default:
		return core.RecoverabilityInProcess
	}
}

func firstRecoverability(values ...core.RecoverabilityMode) core.RecoverabilityMode {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func effectiveDelegationState(state *core.Context) *core.Context {
	if state != nil {
		return state
	}
	return core.NewContext()
}

func delegationRequestedRole(request core.DelegationRequest) core.CoordinationRole {
	if request.Metadata == nil {
		return ""
	}
	value, ok := request.Metadata["target_role"]
	if !ok || value == nil {
		return ""
	}
	role := strings.TrimSpace(fmt.Sprint(value))
	if strings.EqualFold(role, "<nil>") {
		return ""
	}
	return core.CoordinationRole(role)
}

func containsBackgroundExecutionMode(modes []core.CoordinationExecutionMode) bool {
	for _, mode := range modes {
		switch mode {
		case core.CoordinationExecutionModeBackgroundAgent, core.CoordinationExecutionModeSessionBacked:
			return true
		}
	}
	return false
}

func requestPrefersBackground(request core.DelegationRequest) bool {
	if request.Metadata == nil {
		return false
	}
	value, ok := request.Metadata["background"]
	if !ok || value == nil {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func shouldRunDelegationInBackground(target core.CapabilityDescriptor, requested bool) bool {
	if target.Coordination == nil {
		return false
	}
	if target.Coordination.LongRunning {
		return true
	}
	return requested && containsBackgroundExecutionMode(target.Coordination.ExecutionModes)
}

func dedupeStringSlice(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(input))
	for _, item := range input {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func normalizeStringArray(value any) []any {
	switch typed := value.(type) {
	case nil:
		return nil
	case []any:
		return typed
	case []string:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			if strings.TrimSpace(item) == "" {
				continue
			}
			out = append(out, item)
		}
		return out
	default:
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" {
			return nil
		}
		return []any{text}
	}
}

func normalizeArgumentMap(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	if typed, ok := value.(map[string]interface{}); ok {
		return typed
	}
	return map[string]any{}
}

func requiresCrossTrustApproval(callerTrust, targetTrust core.TrustClass, spec *core.AgentRuntimeSpec, request core.DelegationRequest) bool {
	if request.ApprovalRequired {
		return true
	}
	if callerTrust == "" || callerTrust == targetTrust {
		return false
	}
	return core.EffectiveCoordination(spec).RequireApprovalCrossTrust
}

func mergeAnyMaps(parts ...map[string]any) map[string]any {
	var total int
	for _, part := range parts {
		total += len(part)
	}
	if total == 0 {
		return nil
	}
	out := make(map[string]any, total)
	for _, part := range parts {
		for key, value := range part {
			out[key] = value
		}
	}
	return out
}

func cloneDelegationSnapshot(input core.DelegationSnapshot) core.DelegationSnapshot {
	out := input
	out.Request = cloneDelegationRequest(input.Request)
	out.Metadata = cloneAnyMap(input.Metadata)
	if input.Result != nil {
		result := cloneDelegationResult(input.Result)
		out.Result = &result
	}
	return out
}

func cloneDelegationRequest(input core.DelegationRequest) core.DelegationRequest {
	out := input
	out.ResourceRefs = append([]string{}, input.ResourceRefs...)
	out.Metadata = cloneAnyMap(input.Metadata)
	return out
}

func cloneDelegationResult(input *core.DelegationResult) core.DelegationResult {
	if input == nil {
		return core.DelegationResult{}
	}
	out := *input
	out.Data = cloneAnyMap(input.Data)
	out.ResourceRefs = append([]string{}, input.ResourceRefs...)
	out.Diagnostics = append([]string{}, input.Diagnostics...)
	out.Metadata = cloneAnyMap(input.Metadata)
	return out
}

func cloneAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func delegationTransitionID(snapshot core.DelegationSnapshot) string {
	when := snapshot.UpdatedAt
	if when.IsZero() {
		when = snapshot.StartedAt
	}
	return fmt.Sprintf("%s:%s:%d", snapshot.Request.ID, snapshot.State, when.UTC().UnixNano())
}

func promotedDelegationArtifact(snapshot core.DelegationSnapshot, workflowID, runID string) *memory.WorkflowArtifactRecord {
	if snapshot.Result == nil || (snapshot.State != core.DelegationStateSucceeded && snapshot.State != core.DelegationStateFailed && snapshot.State != core.DelegationStateCancelled) {
		return nil
	}
	return &memory.WorkflowArtifactRecord{
		ArtifactID:        "delegation-result:" + snapshot.Request.ID,
		WorkflowID:        workflowID,
		RunID:             strings.TrimSpace(runID),
		Kind:              "delegation_result",
		ContentType:       "application/json",
		StorageKind:       memory.ArtifactStorageInline,
		SummaryText:       delegationSummary(snapshot),
		SummaryMetadata:   delegationArtifactMetadata(snapshot),
		InlineRawText:     marshalDelegationArtifact(snapshot),
		RawSizeBytes:      int64(len(marshalDelegationArtifact(snapshot))),
		CompressionMethod: "none",
		CreatedAt:         snapshot.UpdatedAt,
	}
}

func delegationSummary(snapshot core.DelegationSnapshot) string {
	target := firstNonEmpty(snapshot.Request.TargetCapabilityID, delegationResultTarget(snapshot.Result))
	switch snapshot.State {
	case core.DelegationStateSucceeded:
		return fmt.Sprintf("delegation %s to %s succeeded", snapshot.Request.ID, target)
	case core.DelegationStateFailed:
		return fmt.Sprintf("delegation %s to %s failed", snapshot.Request.ID, target)
	case core.DelegationStateCancelled:
		return fmt.Sprintf("delegation %s to %s cancelled", snapshot.Request.ID, target)
	default:
		return fmt.Sprintf("delegation %s to %s updated", snapshot.Request.ID, target)
	}
}

func delegationArtifactMetadata(snapshot core.DelegationSnapshot) map[string]any {
	metadata := map[string]any{
		"delegation_id":        snapshot.Request.ID,
		"state":                snapshot.State,
		"target_capability_id": snapshot.Request.TargetCapabilityID,
		"target_provider_id":   snapshot.Request.TargetProviderID,
		"target_session_id":    snapshot.Request.TargetSessionID,
		"trust_class":          snapshot.TrustClass,
		"background":           snapshot.Background,
	}
	if snapshot.Result != nil {
		metadata["success"] = snapshot.Result.Success
	}
	return metadata
}

func marshalDelegationArtifact(snapshot core.DelegationSnapshot) string {
	payload := map[string]any{
		"request": snapshot.Request,
		"state":   snapshot.State,
		"result":  snapshot.Result,
	}
	data, err := json.Marshal(core.RedactAny(payload))
	if err != nil {
		return "{}"
	}
	return string(data)
}

func delegationResultTarget(result *core.DelegationResult) string {
	if result == nil {
		return ""
	}
	return result.TargetCapabilityID
}
