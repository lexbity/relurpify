package policy

import (
	"fmt"
	"strings"
	"time"
)

type RecoverabilityMode string

const (
	RecoverabilityEphemeral        RecoverabilityMode = "ephemeral"
	RecoverabilityInProcess        RecoverabilityMode = "recoverable-in-process"
	RecoverabilityPersistedRestore RecoverabilityMode = "recoverable-from-persisted-state"
)

type PolicySnapshot struct {
	ID string `json:"id"`
}

type ApprovalBinding struct {
	CapabilityID string `json:"capability_id,omitempty"`
	TaskID       string `json:"task_id,omitempty"`
	WorkflowID   string `json:"workflow_id,omitempty"`
}

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
	ID                 string               `json:"id"`
	WorkflowID         string               `json:"workflow_id,omitempty"`
	TaskID             string               `json:"task_id,omitempty"`
	CallerAgentID      string               `json:"caller_agent_id,omitempty"`
	CallerCapabilityID string               `json:"caller_capability_id,omitempty"`
	TargetCapabilityID string               `json:"target_capability_id"`
	TargetProviderID   string               `json:"target_provider_id,omitempty"`
	TargetSessionID    string               `json:"target_session_id,omitempty"`
	TaskType           string               `json:"task_type"`
	Instruction        string               `json:"instruction"`
	ResourceRefs       []string             `json:"resource_refs,omitempty"`
	ExpectedResult     *any `json:"expected_result,omitempty"`
	Depth              int                  `json:"depth,omitempty"`
	PolicySnapshotID   string               `json:"policy_snapshot_id,omitempty"`
	ApprovalRequired   bool                 `json:"approval_required,omitempty"`
	Metadata           map[string]any       `json:"metadata,omitempty"`
	CreatedAt          time.Time            `json:"created_at,omitempty"`
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

type DelegationFilter struct {
	WorkflowID         string            `json:"workflow_id,omitempty"`
	TaskID             string            `json:"task_id,omitempty"`
	TargetCapabilityID string            `json:"target_capability_id,omitempty"`
	TargetProviderID   string            `json:"target_provider_id,omitempty"`
	TargetSessionID    string            `json:"target_session_id,omitempty"`
	States             []DelegationState `json:"states,omitempty"`
}

type DelegationResult struct {
	DelegationID       string          `json:"delegation_id"`
	TargetCapabilityID string          `json:"target_capability_id,omitempty"`
	ProviderID         string          `json:"provider_id,omitempty"`
	SessionID          string          `json:"session_id,omitempty"`
	State              DelegationState `json:"state"`
	Success            bool            `json:"success"`
	Data               map[string]any  `json:"data,omitempty"`
	ResourceRefs       []string        `json:"resource_refs,omitempty"`
	Diagnostics        []string        `json:"diagnostics,omitempty"`
	Provenance         any             `json:"provenance,omitempty"`
	Disposition        any             `json:"disposition,omitempty"`
	Insertion          any             `json:"insertion,omitempty"`
	Metadata           map[string]any  `json:"metadata,omitempty"`
	RecordedAt         time.Time       `json:"recorded_at,omitempty"`
	CompletedAt        time.Time       `json:"completed_at,omitempty"`
}

type DelegationSnapshot struct {
	Request        DelegationRequest `json:"request"`
	Result         *DelegationResult `json:"result,omitempty"`
	State          DelegationState   `json:"state"`
	TrustClass     string            `json:"trust_class,omitempty"`
	Recoverability string            `json:"recoverability,omitempty"`
	Background     bool              `json:"background,omitempty"`
	Metadata       map[string]any    `json:"metadata,omitempty"`
	StartedAt      time.Time         `json:"started_at,omitempty"`
	UpdatedAt      time.Time         `json:"updated_at,omitempty"`
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

func NewDelegationResult(request DelegationRequest, targetCapabilityID, providerID, sessionID string, state DelegationState, success bool, data map[string]any) *DelegationResult {
	now := time.Now().UTC()
	result := &DelegationResult{
		DelegationID:       strings.TrimSpace(request.ID),
		TargetCapabilityID: strings.TrimSpace(firstNonEmpty(targetCapabilityID, request.TargetCapabilityID)),
		ProviderID:         strings.TrimSpace(firstNonEmpty(providerID, request.TargetProviderID)),
		SessionID:          strings.TrimSpace(firstNonEmpty(sessionID, request.TargetSessionID)),
		State:              state,
		Success:            success,
		Data:               cloneMap(data),
		ResourceRefs:       append([]string(nil), request.ResourceRefs...),
		RecordedAt:         now,
	}
	if state == DelegationStateSucceeded || state == DelegationStateFailed || state == DelegationStateCancelled {
		result.CompletedAt = now
	}
	return result
}

func ApplyDelegationInsertionDecision(result *DelegationResult, decision any) *DelegationResult {
	if result == nil {
		return nil
	}
	if decision != nil {
		result.Insertion = decision
	}
	return result
}

func ApprovalBindingFromDelegation(request DelegationRequest, result *DelegationResult) *ApprovalBinding {
	binding := &ApprovalBinding{
		CapabilityID: firstNonEmpty(request.TargetCapabilityID, delegationTargetCapability(result)),
		TaskID:       strings.TrimSpace(request.TaskID),
		WorkflowID:   strings.TrimSpace(request.WorkflowID),
	}
	if binding.CapabilityID == "" && binding.TaskID == "" && binding.WorkflowID == "" {
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}
