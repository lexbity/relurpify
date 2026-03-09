package server

import "context"

type InspectableMeta struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Title         string `json:"title,omitempty"`
	RuntimeFamily string `json:"runtime_family,omitempty"`
	TrustClass    string `json:"trust_class,omitempty"`
	Scope         string `json:"scope,omitempty"`
	Source        string `json:"source,omitempty"`
	State         string `json:"state,omitempty"`
	CapturedAt    string `json:"captured_at,omitempty"`
}

type CapabilityPayload struct {
	Description           string   `json:"description,omitempty"`
	Category              string   `json:"category,omitempty"`
	Exposure              string   `json:"exposure,omitempty"`
	Callable              bool     `json:"callable"`
	ProviderID            string   `json:"provider_id,omitempty"`
	SessionAffinity       string   `json:"session_affinity,omitempty"`
	Availability          string   `json:"availability,omitempty"`
	RiskClasses           []string `json:"risk_classes,omitempty"`
	EffectClasses         []string `json:"effect_classes,omitempty"`
	Tags                  []string `json:"tags,omitempty"`
	CoordinationRole      string   `json:"coordination_role,omitempty"`
	CoordinationTaskTypes []string `json:"coordination_task_types,omitempty"`
}

type CapabilityResource struct {
	Meta       InspectableMeta   `json:"meta"`
	Capability CapabilityPayload `json:"capability"`
}

type ProviderPayload struct {
	ProviderID     string   `json:"provider_id"`
	ProviderKind   string   `json:"provider_kind"`
	TrustBaseline  string   `json:"trust_baseline,omitempty"`
	Recoverability string   `json:"recoverability,omitempty"`
	ConfiguredFrom string   `json:"configured_from,omitempty"`
	CapabilityIDs  []string `json:"capability_ids,omitempty"`
	Metadata       []string `json:"metadata,omitempty"`
}

type ProviderResource struct {
	Meta     InspectableMeta `json:"meta"`
	Provider ProviderPayload `json:"provider"`
}

type SessionPayload struct {
	SessionID       string   `json:"session_id"`
	ProviderID      string   `json:"provider_id"`
	WorkflowID      string   `json:"workflow_id,omitempty"`
	TaskID          string   `json:"task_id,omitempty"`
	Recoverability  string   `json:"recoverability,omitempty"`
	CapabilityIDs   []string `json:"capability_ids,omitempty"`
	LastActivityAt  string   `json:"last_activity_at,omitempty"`
	MetadataSummary []string `json:"metadata_summary,omitempty"`
}

type SessionResource struct {
	Meta    InspectableMeta `json:"meta"`
	Session SessionPayload  `json:"session"`
}

type ApprovalPayload struct {
	ID             string            `json:"id"`
	Kind           string            `json:"kind"`
	PermissionType string            `json:"permission_type,omitempty"`
	Action         string            `json:"action"`
	Resource       string            `json:"resource,omitempty"`
	Risk           string            `json:"risk,omitempty"`
	Scope          string            `json:"scope,omitempty"`
	Justification  string            `json:"justification,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

type ApprovalResource struct {
	Meta     InspectableMeta `json:"meta"`
	Approval ApprovalPayload `json:"approval"`
}

type PromptMessagePayload struct {
	Role    string                   `json:"role,omitempty"`
	Content []map[string]interface{} `json:"content,omitempty"`
}

type PromptPayload struct {
	PromptID    string                 `json:"prompt_id"`
	ProviderID  string                 `json:"provider_id,omitempty"`
	Description string                 `json:"description,omitempty"`
	Messages    []PromptMessagePayload `json:"messages,omitempty"`
	Metadata    []string               `json:"metadata,omitempty"`
}

type PromptResource struct {
	Meta   InspectableMeta `json:"meta"`
	Prompt PromptPayload   `json:"prompt"`
}

type ReadableResourcePayload struct {
	ResourceID       string                   `json:"resource_id"`
	ProviderID       string                   `json:"provider_id,omitempty"`
	Description      string                   `json:"description,omitempty"`
	WorkflowResource bool                     `json:"workflow_resource,omitempty"`
	WorkflowURI      string                   `json:"workflow_uri,omitempty"`
	Contents         []map[string]interface{} `json:"contents,omitempty"`
	Metadata         []string                 `json:"metadata,omitempty"`
}

type ReadableResource struct {
	Meta     InspectableMeta         `json:"meta"`
	Resource ReadableResourcePayload `json:"resource"`
}

type DelegationPayload struct {
	DelegationID       string   `json:"delegation_id"`
	RunID              string   `json:"run_id,omitempty"`
	TaskID             string   `json:"task_id,omitempty"`
	State              string   `json:"state"`
	TargetCapabilityID string   `json:"target_capability_id,omitempty"`
	TargetProviderID   string   `json:"target_provider_id,omitempty"`
	TargetSessionID    string   `json:"target_session_id,omitempty"`
	Recoverability     string   `json:"recoverability,omitempty"`
	InsertionAction    string   `json:"insertion_action,omitempty"`
	ResourceRefs       []string `json:"resource_refs,omitempty"`
}

type DelegationResource struct {
	Meta       InspectableMeta   `json:"meta"`
	Delegation DelegationPayload `json:"delegation"`
}

type ArtifactPayload struct {
	ArtifactID  string `json:"artifact_id"`
	RunID       string `json:"run_id,omitempty"`
	Kind        string `json:"kind"`
	ContentType string `json:"content_type,omitempty"`
	SummaryText string `json:"summary_text,omitempty"`
}

type ArtifactResource struct {
	Meta     InspectableMeta `json:"meta"`
	Artifact ArtifactPayload `json:"artifact"`
}

type Inspector interface {
	ListCapabilities(ctx context.Context) ([]CapabilityResource, error)
	GetCapability(ctx context.Context, id string) (*CapabilityResource, error)
	ListPrompts(ctx context.Context) ([]PromptResource, error)
	GetPrompt(ctx context.Context, id string) (*PromptResource, error)
	ListProviders(ctx context.Context) ([]ProviderResource, error)
	GetProvider(ctx context.Context, id string) (*ProviderResource, error)
	ListResources(ctx context.Context) ([]ReadableResource, error)
	GetResource(ctx context.Context, id string) (*ReadableResource, error)
	GetWorkflowResource(ctx context.Context, uri string) (*ReadableResource, error)
	ListSessions(ctx context.Context) ([]SessionResource, error)
	GetSession(ctx context.Context, id string) (*SessionResource, error)
	ListApprovals(ctx context.Context) ([]ApprovalResource, error)
	GetApproval(ctx context.Context, id string) (*ApprovalResource, error)
}
