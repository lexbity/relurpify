package core

import (
	"fmt"
	"strings"
	"time"
)

type DelegationState string

const (
	DelegationStatePending   DelegationState = "pending"
	DelegationStateRunning   DelegationState = "running"
	DelegationStateSucceeded DelegationState = "succeeded"
	DelegationStateFailed    DelegationState = "failed"
	DelegationStateCancelled DelegationState = "cancelled"
)

// DelegationRequest is the framework-owned contract for handing work from one
// coordinated capability/agent target to another.
type DelegationRequest struct {
	ID                 string         `json:"id" yaml:"id"`
	WorkflowID         string         `json:"workflow_id,omitempty" yaml:"workflow_id,omitempty"`
	TaskID             string         `json:"task_id,omitempty" yaml:"task_id,omitempty"`
	CallerAgentID      string         `json:"caller_agent_id,omitempty" yaml:"caller_agent_id,omitempty"`
	CallerCapabilityID string         `json:"caller_capability_id,omitempty" yaml:"caller_capability_id,omitempty"`
	TargetCapabilityID string         `json:"target_capability_id" yaml:"target_capability_id"`
	TargetProviderID   string         `json:"target_provider_id,omitempty" yaml:"target_provider_id,omitempty"`
	TargetSessionID    string         `json:"target_session_id,omitempty" yaml:"target_session_id,omitempty"`
	TaskType           string         `json:"task_type" yaml:"task_type"`
	Instruction        string         `json:"instruction" yaml:"instruction"`
	ResourceRefs       []string       `json:"resource_refs,omitempty" yaml:"resource_refs,omitempty"`
	ExpectedResult     *Schema        `json:"expected_result,omitempty" yaml:"expected_result,omitempty"`
	Depth              int            `json:"depth,omitempty" yaml:"depth,omitempty"`
	PolicySnapshotID   string         `json:"policy_snapshot_id,omitempty" yaml:"policy_snapshot_id,omitempty"`
	ApprovalRequired   bool           `json:"approval_required,omitempty" yaml:"approval_required,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	CreatedAt          time.Time      `json:"created_at,omitempty" yaml:"created_at,omitempty"`
}

// DelegationResult is the framework-owned result record for delegated work.
type DelegationResult struct {
	DelegationID       string             `json:"delegation_id" yaml:"delegation_id"`
	TargetCapabilityID string             `json:"target_capability_id,omitempty" yaml:"target_capability_id,omitempty"`
	ProviderID         string             `json:"provider_id,omitempty" yaml:"provider_id,omitempty"`
	SessionID          string             `json:"session_id,omitempty" yaml:"session_id,omitempty"`
	State              DelegationState    `json:"state" yaml:"state"`
	Success            bool               `json:"success" yaml:"success"`
	Data               map[string]any     `json:"data,omitempty" yaml:"data,omitempty"`
	ResourceRefs       []string           `json:"resource_refs,omitempty" yaml:"resource_refs,omitempty"`
	Diagnostics        []string           `json:"diagnostics,omitempty" yaml:"diagnostics,omitempty"`
	Provenance         ContentProvenance  `json:"provenance,omitempty" yaml:"provenance,omitempty"`
	Disposition        ContentDisposition `json:"disposition,omitempty" yaml:"disposition,omitempty"`
	Insertion          InsertionDecision  `json:"insertion,omitempty" yaml:"insertion,omitempty"`
	Metadata           map[string]any     `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	RecordedAt         time.Time          `json:"recorded_at,omitempty" yaml:"recorded_at,omitempty"`
	CompletedAt        time.Time          `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
}

// DelegationSnapshot is the runtime-owned lifecycle record for an admitted
// delegation.
type DelegationSnapshot struct {
	Request        DelegationRequest  `json:"request" yaml:"request"`
	Result         *DelegationResult  `json:"result,omitempty" yaml:"result,omitempty"`
	State          DelegationState    `json:"state" yaml:"state"`
	TrustClass     TrustClass         `json:"trust_class,omitempty" yaml:"trust_class,omitempty"`
	Recoverability RecoverabilityMode `json:"recoverability,omitempty" yaml:"recoverability,omitempty"`
	Background     bool               `json:"background,omitempty" yaml:"background,omitempty"`
	Metadata       map[string]any     `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	StartedAt      time.Time          `json:"started_at,omitempty" yaml:"started_at,omitempty"`
	UpdatedAt      time.Time          `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
}

type DelegationFilter struct {
	WorkflowID         string            `json:"workflow_id,omitempty" yaml:"workflow_id,omitempty"`
	TaskID             string            `json:"task_id,omitempty" yaml:"task_id,omitempty"`
	TargetCapabilityID string            `json:"target_capability_id,omitempty" yaml:"target_capability_id,omitempty"`
	TargetProviderID   string            `json:"target_provider_id,omitempty" yaml:"target_provider_id,omitempty"`
	TargetSessionID    string            `json:"target_session_id,omitempty" yaml:"target_session_id,omitempty"`
	States             []DelegationState `json:"states,omitempty" yaml:"states,omitempty"`
}

func (r DelegationRequest) Validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("delegation id required")
	}
	if strings.TrimSpace(r.TargetCapabilityID) == "" {
		return fmt.Errorf("target capability id required")
	}
	if strings.TrimSpace(r.TaskType) == "" {
		return fmt.Errorf("task type required")
	}
	if strings.TrimSpace(r.Instruction) == "" {
		return fmt.Errorf("instruction required")
	}
	if r.Depth < 0 {
		return fmt.Errorf("depth cannot be negative")
	}
	for _, ref := range r.ResourceRefs {
		if strings.TrimSpace(ref) == "" {
			return fmt.Errorf("resource refs cannot contain empty values")
		}
	}
	return nil
}

func (r DelegationResult) Validate() error {
	if strings.TrimSpace(r.DelegationID) == "" {
		return fmt.Errorf("delegation id required")
	}
	switch r.State {
	case DelegationStatePending, DelegationStateRunning, DelegationStateSucceeded, DelegationStateFailed, DelegationStateCancelled:
	default:
		return fmt.Errorf("delegation state %s invalid", r.State)
	}
	if r.State == DelegationStateSucceeded && !r.Success {
		return fmt.Errorf("succeeded delegation result must be successful")
	}
	if r.State == DelegationStateFailed && r.Success {
		return fmt.Errorf("failed delegation result cannot be successful")
	}
	for _, ref := range r.ResourceRefs {
		if strings.TrimSpace(ref) == "" {
			return fmt.Errorf("resource refs cannot contain empty values")
		}
	}
	for _, diagnostic := range r.Diagnostics {
		if strings.TrimSpace(diagnostic) == "" {
			return fmt.Errorf("diagnostics cannot contain empty values")
		}
	}
	return nil
}

func (s DelegationSnapshot) Validate() error {
	if err := s.Request.Validate(); err != nil {
		return fmt.Errorf("delegation request invalid: %w", err)
	}
	switch s.State {
	case DelegationStatePending, DelegationStateRunning, DelegationStateSucceeded, DelegationStateFailed, DelegationStateCancelled:
	default:
		return fmt.Errorf("delegation state %s invalid", s.State)
	}
	switch s.Recoverability {
	case "", RecoverabilityEphemeral, RecoverabilityInProcess, RecoverabilityPersistedRestore:
	default:
		return fmt.Errorf("recoverability mode %s invalid", s.Recoverability)
	}
	if s.Result != nil {
		if err := s.Result.Validate(); err != nil {
			return fmt.Errorf("delegation result invalid: %w", err)
		}
		if s.Result.DelegationID != s.Request.ID {
			return fmt.Errorf("delegation result id %s does not match request id %s", s.Result.DelegationID, s.Request.ID)
		}
		if s.State != s.Result.State {
			return fmt.Errorf("delegation snapshot state %s does not match result state %s", s.State, s.Result.State)
		}
	}
	return nil
}

// NewDelegationResult constructs a result record with default provenance and
// insertion semantics derived from the delegated target's trust class.
func NewDelegationResult(request DelegationRequest, targetCapabilityID, providerID, sessionID string, trust TrustClass, state DelegationState, success bool, data map[string]any, snapshot *PolicySnapshot) *DelegationResult {
	now := time.Now().UTC()
	disposition := ContentDispositionRaw
	result := &DelegationResult{
		DelegationID:       strings.TrimSpace(request.ID),
		TargetCapabilityID: strings.TrimSpace(firstNonEmpty(targetCapabilityID, request.TargetCapabilityID)),
		ProviderID:         strings.TrimSpace(firstNonEmpty(providerID, request.TargetProviderID)),
		SessionID:          strings.TrimSpace(firstNonEmpty(sessionID, request.TargetSessionID)),
		State:              state,
		Success:            success,
		Data:               cloneInterfaceMap(data),
		ResourceRefs:       append([]string(nil), request.ResourceRefs...),
		Provenance: ContentProvenance{
			CapabilityID: firstNonEmpty(targetCapabilityID, request.TargetCapabilityID),
			ProviderID:   firstNonEmpty(providerID, request.TargetProviderID),
			TrustClass:   trust,
			Disposition:  disposition,
		},
		Disposition: disposition,
		Insertion:   DefaultInsertionDecision(CapabilityDescriptor{TrustClass: trust}, disposition),
		RecordedAt:  now,
	}
	if snapshot != nil {
		result.Insertion.PolicySnapshotID = snapshot.ID
	}
	if state == DelegationStateSucceeded || state == DelegationStateFailed || state == DelegationStateCancelled {
		result.CompletedAt = now
	}
	return result
}

func ApplyDelegationInsertionDecision(result *DelegationResult, decision InsertionDecision) *DelegationResult {
	if result == nil {
		return nil
	}
	decision.RequiresHITL = decision.Action == InsertionActionHITLRequired
	if decision.PolicySnapshotID == "" {
		decision.PolicySnapshotID = result.Insertion.PolicySnapshotID
	}
	result.Insertion = decision
	return result
}

func ApprovalBindingFromDelegation(request DelegationRequest, result *DelegationResult) *ApprovalBinding {
	if err := request.Validate(); err != nil {
		return nil
	}
	binding := &ApprovalBinding{
		CapabilityID: firstNonEmpty(request.TargetCapabilityID, delegationTargetCapability(result)),
		ProviderID:   firstNonEmpty(request.TargetProviderID, delegationProviderID(result)),
		SessionID:    firstNonEmpty(request.TargetSessionID, delegationSessionID(result)),
		TaskID:       strings.TrimSpace(request.TaskID),
		WorkflowID:   strings.TrimSpace(request.WorkflowID),
	}
	if len(request.ResourceRefs) > 0 {
		binding.TargetResource = strings.TrimSpace(request.ResourceRefs[0])
	}
	if binding.CapabilityID == "" && binding.ProviderID == "" && binding.SessionID == "" && binding.TargetResource == "" && binding.TaskID == "" && binding.WorkflowID == "" {
		return nil
	}
	return binding
}

func delegationTargetCapability(result *DelegationResult) string {
	if result == nil {
		return ""
	}
	return strings.TrimSpace(result.TargetCapabilityID)
}

func delegationProviderID(result *DelegationResult) string {
	if result == nil {
		return ""
	}
	return strings.TrimSpace(result.ProviderID)
}

func delegationSessionID(result *DelegationResult) string {
	if result == nil {
		return ""
	}
	return strings.TrimSpace(result.SessionID)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
