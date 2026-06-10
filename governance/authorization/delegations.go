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

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/runtime"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/capability/classification"
)

var ErrDelegationNotFound = errors.New("delegation not found")

type DelegationCapabilityRegistry interface {
	GetCoordinationTarget(idOrName string) (governanceports.DescriptorView, bool)
	CoordinationTargets(selectors ...governanceports.CapabilitySelectorView) []governanceports.DescriptorView
	InvokeCapability(ctx context.Context, state ports.State, idOrName string, args map[string]interface{}) (any, error)
	CapturePolicySnapshot() *policy.PolicySnapshot
	EffectiveCoordination(spec governanceports.SpecView) governanceports.CoordinationSpecView
	BuildDelegationResult(request policy.DelegationRequest, target governanceports.DescriptorView, result any, invokeErr error, snapshot *policy.PolicySnapshot, spec governanceports.SpecView, callerTrust string) *policy.DelegationResult
}

type BackgroundDelegationOutcome struct {
	Result any
	Error  error
}

type BackgroundDelegationHandle struct {
	ProviderID     string
	SessionID      string
	Recoverability policy.RecoverabilityMode
	Results        <-chan BackgroundDelegationOutcome
	Cancel         func(context.Context, policy.DelegationSnapshot) error
}

type DelegationBackgroundRunner interface {
	StartBackgroundDelegation(ctx context.Context, request policy.DelegationRequest, target governanceports.DescriptorView, args map[string]any, opts DelegationExecutionOptions) (*BackgroundDelegationHandle, error)
}

type DelegationExecutionOptions struct {
	Registry         DelegationCapabilityRegistry
	BackgroundRunner DelegationBackgroundRunner
	AgentSpec        governanceports.SpecView
	State            *contextdata.Envelope
	LifecycleRepo    governanceports.DelegationRepository
	WorkflowRunID    string
	WorkflowStepID   string
	CallerAgentID    string
	CallerTrust      string
	Recoverability   policy.RecoverabilityMode
	Background       bool
	Metadata         map[string]any
}

type DelegationStartOptions struct {
	TrustClass     string
	Recoverability policy.RecoverabilityMode
	Background     bool
	PolicySnapshot *policy.PolicySnapshot
	Metadata       map[string]any
	OnCancel       func(context.Context, policy.DelegationSnapshot) error
}

type DelegationManager struct {
	mu          sync.RWMutex
	delegations map[string]*delegationRecord
	observer    func(policy.DelegationSnapshot)
}

type delegationRecord struct {
	snapshot policy.DelegationSnapshot
	cancel   func(context.Context, policy.DelegationSnapshot) error
}

func NewDelegationManager() *DelegationManager {
	return &DelegationManager{
		delegations: map[string]*delegationRecord{},
	}
}

func (m *DelegationManager) SetObserver(observer func(policy.DelegationSnapshot)) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observer = observer
}

func (m *DelegationManager) ExecuteDelegation(ctx context.Context, request policy.DelegationRequest, opts DelegationExecutionOptions) (*policy.DelegationSnapshot, error) {
	if m == nil {
		return nil, fmt.Errorf("delegation manager unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.Registry == nil {
		return nil, fmt.Errorf("delegation registry required")
	}
	coordination := opts.Registry.EffectiveCoordination(opts.AgentSpec)
	target, err := resolveDelegationTarget(request, opts.Registry, coordination)
	if err != nil {
		return nil, err
	}
	if err := validateDelegationTargetPolicy(request, target, coordination); err != nil {
		return nil, err
	}
	request = cloneDelegationRequest(request)
	request.CallerAgentID = firstNonEmpty(request.CallerAgentID, opts.CallerAgentID)
	request.TargetCapabilityID = target.CapabilityID()
	request.TargetProviderID = firstNonEmpty(request.TargetProviderID, target.SourceProviderID())
	request.TargetSessionID = firstNonEmpty(request.TargetSessionID, target.SourceSessionID())
	request.ResourceRefs = resolveDelegationResourceRefs(request, target, opts)
	args, err := buildDelegationInvocationArgs(ctx, request, target, opts)
	if err != nil {
		return nil, err
	}

	policySnapshot := opts.Registry.CapturePolicySnapshot()
	runBackground := shouldRunDelegationInBackground(target, opts.Background)
	if runBackground {
		if opts.BackgroundRunner == nil {
			return nil, fmt.Errorf("background delegation runner required for %s", target.CapabilityName())
		}
		handle, err := opts.BackgroundRunner.StartBackgroundDelegation(ctx, request, target, args, opts)
		if err != nil {
			return nil, err
		}
		request.TargetProviderID = firstNonEmpty(request.TargetProviderID, handle.ProviderID)
		request.TargetSessionID = firstNonEmpty(request.TargetSessionID, handle.SessionID)
		started, err := m.StartDelegation(ctx, request, DelegationStartOptions{
			TrustClass:     target.TrustClass(),
			Recoverability: effectiveDelegationRecoverability(firstRecoverability(handle.Recoverability, opts.Recoverability)),
			Background:     true,
			PolicySnapshot: policySnapshot,
			Metadata: mergeAnyMaps(opts.Metadata, map[string]any{
				"target_role":   target.CoordinationRole(),
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
		TrustClass:     target.TrustClass(),
		Recoverability: effectiveDelegationRecoverability(opts.Recoverability),
		Background:     opts.Background,
		PolicySnapshot: policySnapshot,
		Metadata: mergeAnyMaps(opts.Metadata, map[string]any{
			"target_role":   target.CoordinationRole(),
			"task_type":     request.TaskType,
			"resource_refs": append([]string{}, request.ResourceRefs...),
		}),
	})
	if err != nil {
		return nil, err
	}
	result, invokeErr := opts.Registry.InvokeCapability(ctx, effectiveDelegationState(opts.State).State(), target.CapabilityID(), args)
	delegationResult := opts.Registry.BuildDelegationResult(request, target, result, invokeErr, policySnapshot, opts.AgentSpec, opts.CallerTrust)
	completed, completeErr := m.CompleteDelegation(started.Request.ID, delegationResult)
	if completeErr != nil {
		return nil, completeErr
	}
	if invokeErr != nil {
		return completed, invokeErr
	}
	return completed, nil
}

func (m *DelegationManager) StartDelegation(ctx context.Context, request policy.DelegationRequest, opts DelegationStartOptions) (*policy.DelegationSnapshot, error) {
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
	snapshot := policy.DelegationSnapshot{
		Request:        request,
		State:          policy.DelegationStateRunning,
		TrustClass:     string(opts.TrustClass),
		Recoverability: string(opts.Recoverability),
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

func (m *DelegationManager) CompleteDelegation(id string, result *policy.DelegationResult) (*policy.DelegationSnapshot, error) {
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
	case policy.DelegationStateSucceeded, policy.DelegationStateFailed, policy.DelegationStateCancelled:
	default:
		return nil, fmt.Errorf("delegation result state %s not terminal", candidate.State)
	}
	m.mu.Lock()
	record, ok := m.delegations[id]
	if !ok {
		m.mu.Unlock()
		return nil, ErrDelegationNotFound
	}
	if record.snapshot.State == policy.DelegationStateSucceeded || record.snapshot.State == policy.DelegationStateFailed || record.snapshot.State == policy.DelegationStateCancelled {
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

func (m *DelegationManager) CancelDelegation(ctx context.Context, id, reason string) (*policy.DelegationSnapshot, error) {
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

	result := policy.NewDelegationResult(
		snapshot.Request,
		snapshot.Request.TargetCapabilityID,
		snapshot.Request.TargetProviderID,
		snapshot.Request.TargetSessionID,
		policy.DelegationStateCancelled,
		false,
		map[string]any{"reason": strings.TrimSpace(reason)},
	)
	if trimmed := strings.TrimSpace(reason); trimmed != "" {
		result.Diagnostics = []string{trimmed}
	}
	return m.CompleteDelegation(id, result)
}

func (m *DelegationManager) GetDelegation(id string) (*policy.DelegationSnapshot, error) {
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

func (m *DelegationManager) ListDelegations(filter policy.DelegationFilter) []policy.DelegationSnapshot {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]policy.DelegationSnapshot, 0, len(m.delegations))
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

func (m *DelegationManager) SnapshotDelegations() []policy.DelegationSnapshot {
	return m.ListDelegations(policy.DelegationFilter{})
}

func (m *DelegationManager) PersistDelegations(ctx context.Context, repo governanceports.DelegationRepository, workflowID, runID string) error {
	if m == nil {
		return fmt.Errorf("delegation manager unavailable")
	}
	if repo == nil {
		return fmt.Errorf("lifecycle repository required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for _, snapshot := range m.SnapshotDelegations() {
		if strings.TrimSpace(workflowID) != "" && snapshot.Request.WorkflowID != "" && snapshot.Request.WorkflowID != workflowID {
			continue
		}
		record := governanceports.DelegationEntry{
			DelegationID:   snapshot.Request.ID,
			WorkflowID:     firstNonEmpty(snapshot.Request.WorkflowID, workflowID),
			RunID:          strings.TrimSpace(runID),
			TaskID:         snapshot.Request.TaskID,
			State:          string(snapshot.State),
			TrustClass:     string(snapshot.TrustClass),
			Recoverability: string(snapshot.Recoverability),
			Background:     snapshot.Background,
			Request:        snapshot.Request,
			Result:         snapshot.Result,
			Metadata:       cloneAnyMap(snapshot.Metadata),
			StartedAt:      snapshot.StartedAt,
			UpdatedAt:      snapshot.UpdatedAt,
		}
		if err := repo.UpsertDelegation(ctx, record); err != nil {
			return err
		}
		transition := governanceports.DelegationTransitionEntry{
			TransitionID: delegationTransitionID(snapshot),
			DelegationID: snapshot.Request.ID,
			WorkflowID:   record.WorkflowID,
			RunID:        record.RunID,
			ToState:      string(snapshot.State),
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
		if err := repo.AppendDelegationTransition(ctx, transition); err != nil {
			return err
		}
		if artifact := promotedDelegationArtifact(snapshot, record.WorkflowID, runID); artifact != nil {
			if err := repo.UpsertArtifact(ctx, *artifact); err != nil {
				return err
			}
		}
	}
	return nil
}

func resolveDelegationTarget(request policy.DelegationRequest, registry DelegationCapabilityRegistry, coordination governanceports.CoordinationSpecView) (governanceports.DescriptorView, error) {
	if registry == nil {
		return nil, fmt.Errorf("delegation registry required")
	}
	targetID := strings.TrimSpace(request.TargetCapabilityID)
	if targetID != "" {
		target, ok := registry.GetCoordinationTarget(targetID)
		if !ok {
			return nil, fmt.Errorf("delegation target %s not admitted", targetID)
		}
		return target, nil
	}
	selectors := make([]governanceports.CapabilitySelectorView, 0, len(coordination.DelegationTargetSelectors))
	for _, selector := range coordination.DelegationTargetSelectors {
		selectors = append(selectors, selector)
	}
	candidates := registry.CoordinationTargets(selectors...)
	for _, candidate := range candidates {
		if !delegationSelectorMatchesTaskType(request.TaskType, candidate) {
			continue
		}
		if role := delegationRequestedRole(request); role != "" && candidate.CoordinationRole() != role {
			continue
		}
		return candidate, nil
	}
	if role := delegationRequestedRole(request); role != "" {
		return nil, fmt.Errorf("no admitted delegation target for task type %s and role %s", request.TaskType, role)
	}
	return nil, fmt.Errorf("no admitted delegation target for task type %s", request.TaskType)
}

func delegationSelectorMatchesTaskType(taskType string, target governanceports.DescriptorView) bool {
	if taskType == "" {
		return true
	}
	for _, tt := range target.CoordinationTaskTypes() {
		if strings.EqualFold(strings.TrimSpace(tt), strings.TrimSpace(taskType)) {
			return true
		}
	}
	return false
}

func validateDelegationTargetPolicy(request policy.DelegationRequest, target governanceports.DescriptorView, coordination governanceports.CoordinationSpecView) error {
	if !target.CoordinationTarget() {
		return fmt.Errorf("capability %s is not a coordination target", target.CapabilityID())
	}
	if coordination.MaxDelegationDepth > 0 && request.Depth > coordination.MaxDelegationDepth {
		return fmt.Errorf("delegation depth %d exceeds max %d", request.Depth, coordination.MaxDelegationDepth)
	}
	if target.RuntimeFamily() == "provider" && target.SourceScope() == string(classification.CapabilityScopeRemote) && !coordination.AllowRemoteDelegation {
		return fmt.Errorf("remote delegation to %s is not allowed", target.CapabilityName())
	}
	if target.CoordinationLongRunning() == 1 && !coordination.AllowBackgroundDelegation {
		return fmt.Errorf("background delegation to %s is not allowed", target.CapabilityName())
	}
	if requestPrefersBackground(request) && !containsBackgroundExecutionMode(target.CoordinationExecutionModes()) && target.CoordinationLongRunning() != 1 {
		return fmt.Errorf("delegation target %s is not background-capable", target.CapabilityName())
	}
	if requestPrefersBackground(request) && !coordination.AllowBackgroundDelegation {
		return fmt.Errorf("session-backed or background delegation to %s is not allowed", target.CapabilityName())
	}
	return nil
}

func resolveDelegationResourceRefs(request policy.DelegationRequest, target governanceports.DescriptorView, opts DelegationExecutionOptions) []string {
	if len(request.ResourceRefs) > 0 {
		return dedupeStringSlice(request.ResourceRefs)
	}
	// TODO: Implement resource projection via lifecycle repository in Phase 4
	// For now, return nil if no explicit refs provided
	return nil
}

func buildDelegationInvocationArgs(ctx context.Context, request policy.DelegationRequest, target governanceports.DescriptorView, opts DelegationExecutionOptions) (map[string]any, error) {
	args := map[string]any{
		"instruction":   request.Instruction,
		"task_id":       request.TaskID,
		"workflow_id":   request.WorkflowID,
		"resource_refs": append([]string{}, request.ResourceRefs...),
	}
	for key, value := range request.Metadata {
		args[key] = value
	}
	// TODO: Implement resource projection via lifecycle repository in Phase 4
	// For now, skip resource summaries
	role := target.CoordinationRole()
	switch role {
	case "architect":
		args["context_summary"] = ""
	case "reviewer":
		args["artifact_summary"] = ""
		args["acceptance_criteria"] = normalizeStringArray(args["acceptance_criteria"])
	case "verifier":
		args["artifact_summary"] = ""
		if criteria, ok := args["verification_criteria"]; ok {
			args["verification_criteria"] = normalizeStringArray(criteria)
		} else {
			args["verification_criteria"] = normalizeStringArray(args["acceptance_criteria"])
		}
	case "executor":
		args["args"] = normalizeArgumentMap(args["args"])
	}
	return args, nil
}

func (m *DelegationManager) awaitBackgroundDelegation(id string, request policy.DelegationRequest, target governanceports.DescriptorView, handle *BackgroundDelegationHandle, snapshot *policy.PolicySnapshot, opts DelegationExecutionOptions) {
	if m == nil || handle == nil || handle.Results == nil {
		return
	}
	outcome, ok := <-handle.Results
	if !ok {
		outcome.Error = fmt.Errorf("background delegation session %s closed without result", handle.SessionID)
	}
	result := opts.Registry.BuildDelegationResult(request, target, outcome.Result, outcome.Error, snapshot, opts.AgentSpec, opts.CallerTrust)
	_, _ = m.CompleteDelegation(id, result)
}

func delegationMatchesFilter(snapshot policy.DelegationSnapshot, filter policy.DelegationFilter) bool {
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

func projectDelegationResources(ctx context.Context, refs []string, repo governanceports.DelegationRepository) ([]string, error) {
	// TODO: Implement resource projection via lifecycle repository in Phase 4
	// For now, return nil
	return nil, nil
}

func effectiveDelegationRecoverability(mode policy.RecoverabilityMode) policy.RecoverabilityMode {
	switch mode {
	case policy.RecoverabilityEphemeral, policy.RecoverabilityInProcess, policy.RecoverabilityPersistedRestore:
		return mode
	default:
		return policy.RecoverabilityInProcess
	}
}

func firstRecoverability(values ...policy.RecoverabilityMode) policy.RecoverabilityMode {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func effectiveDelegationState(env *contextdata.Envelope) *contextdata.Envelope {
	if env != nil {
		return env
	}
	// Return a new empty envelope if none provided
	return contextdata.NewEnvelope("default", "default")
}

func delegationRequestedRole(request policy.DelegationRequest) string {
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
	return role
}

func containsBackgroundExecutionMode(modes []string) bool {
	for _, mode := range modes {
		switch mode {
		case "background-service", "session-backed":
			return true
		}
	}
	return false
}

func requestPrefersBackground(request policy.DelegationRequest) bool {
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

func shouldRunDelegationInBackground(target governanceports.DescriptorView, requested bool) bool {
	if target.CoordinationLongRunning() == 1 {
		return true
	}
	return requested && containsBackgroundExecutionMode(target.CoordinationExecutionModes())
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

func cloneDelegationSnapshot(input policy.DelegationSnapshot) policy.DelegationSnapshot {
	out := input
	out.Request = cloneDelegationRequest(input.Request)
	out.Metadata = cloneAnyMap(input.Metadata)
	if input.Result != nil {
		result := cloneDelegationResult(input.Result)
		out.Result = &result
	}
	return out
}

func cloneDelegationRequest(input policy.DelegationRequest) policy.DelegationRequest {
	out := input
	out.ResourceRefs = append([]string{}, input.ResourceRefs...)
	out.Metadata = cloneAnyMap(input.Metadata)
	return out
}

func cloneDelegationResult(input *policy.DelegationResult) policy.DelegationResult {
	if input == nil {
		return policy.DelegationResult{}
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

func delegationTransitionID(snapshot policy.DelegationSnapshot) string {
	when := snapshot.UpdatedAt
	if when.IsZero() {
		when = snapshot.StartedAt
	}
	return fmt.Sprintf("%s:%s:%d", snapshot.Request.ID, snapshot.State, when.UTC().UnixNano())
}

func promotedDelegationArtifact(snapshot policy.DelegationSnapshot, workflowID, runID string) *governanceports.WorkflowArtifactRecord {
	if snapshot.Result == nil || (snapshot.State != policy.DelegationStateSucceeded && snapshot.State != policy.DelegationStateFailed && snapshot.State != policy.DelegationStateCancelled) {
		return nil
	}
	return &governanceports.WorkflowArtifactRecord{
		ArtifactID:        "delegation-result:" + snapshot.Request.ID,
		WorkflowID:        workflowID,
		RunID:             strings.TrimSpace(runID),
		Kind:              "delegation_result",
		ContentType:       "application/json",
		StorageKind:       governanceports.ArtifactStorageInline,
		SummaryText:       delegationSummary(snapshot),
		SummaryMetadata:   delegationArtifactMetadata(snapshot),
		InlineRawText:     marshalDelegationArtifact(snapshot),
		RawSizeBytes:      int64(len(marshalDelegationArtifact(snapshot))),
		CompressionMethod: "none",
		CreatedAt:         snapshot.UpdatedAt,
	}
}

func delegationSummary(snapshot policy.DelegationSnapshot) string {
	target := firstNonEmpty(snapshot.Request.TargetCapabilityID, delegationResultTarget(snapshot.Result))
	switch snapshot.State {
	case policy.DelegationStateSucceeded:
		return fmt.Sprintf("delegation %s to %s succeeded", snapshot.Request.ID, target)
	case policy.DelegationStateFailed:
		return fmt.Sprintf("delegation %s to %s failed", snapshot.Request.ID, target)
	case policy.DelegationStateCancelled:
		return fmt.Sprintf("delegation %s to %s cancelled", snapshot.Request.ID, target)
	default:
		return fmt.Sprintf("delegation %s to %s updated", snapshot.Request.ID, target)
	}
}

func delegationArtifactMetadata(snapshot policy.DelegationSnapshot) map[string]any {
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

func marshalDelegationArtifact(snapshot policy.DelegationSnapshot) string {
	payload := map[string]any{
		"request": snapshot.Request,
		"state":   snapshot.State,
		"result":  snapshot.Result,
	}
	data, err := json.Marshal(runtime.RedactAny(payload))
	if err != nil {
		return "{}"
	}
	return string(data)
}

func delegationResultTarget(result *policy.DelegationResult) string {
	if result == nil {
		return ""
	}
	return result.TargetCapabilityID
}
