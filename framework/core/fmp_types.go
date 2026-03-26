package core

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type SensitivityClass string

const (
	SensitivityClassLow        SensitivityClass = "low"
	SensitivityClassModerate   SensitivityClass = "moderate"
	SensitivityClassHigh       SensitivityClass = "high"
	SensitivityClassRestricted SensitivityClass = "restricted"
)

type RouteMode string

const (
	RouteModeDirect   RouteMode = "direct"
	RouteModeGateway  RouteMode = "gateway"
	RouteModeMediated RouteMode = "mediated"
)

type EncryptionMode string

const (
	EncryptionModeLinkOnly     EncryptionMode = "link-only"
	EncryptionModeObject       EncryptionMode = "object"
	EncryptionModeEndToEnd     EncryptionMode = "end-to-end"
	EncryptionModeEndToEndWrap EncryptionMode = "end-to-end-wrap"
)

type TransferMode string

const (
	TransferModeInline   TransferMode = "inline"
	TransferModeChunked  TransferMode = "chunked"
	TransferModeExternal TransferMode = "external"
)

type ReceiptStatus string

const (
	ReceiptStatusRunning  ReceiptStatus = "running"
	ReceiptStatusRejected ReceiptStatus = "rejected"
	ReceiptStatusFailed   ReceiptStatus = "failed"
)

type AttemptState string

const (
	AttemptStateCreated         AttemptState = "CREATED"
	AttemptStateAdmitted        AttemptState = "ADMITTED"
	AttemptStateRunning         AttemptState = "RUNNING"
	AttemptStateCheckpointing   AttemptState = "CHECKPOINTING"
	AttemptStateHandoffOffered  AttemptState = "HANDOFF_OFFERED"
	AttemptStateHandoffAccepted AttemptState = "HANDOFF_ACCEPTED"
	AttemptStateResumePending   AttemptState = "RESUME_PENDING"
	AttemptStateCommittedRemote AttemptState = "COMMITTED_REMOTE"
	AttemptStateFenced          AttemptState = "FENCED"
	AttemptStateCompleted       AttemptState = "COMPLETED"
	AttemptStateFailed          AttemptState = "FAILED"
	AttemptStateOrphaned        AttemptState = "ORPHANED"
)

type RefusalReasonCode string

const (
	RefusalUnauthorized        RefusalReasonCode = "unauthorized"
	RefusalIncompatibleRuntime RefusalReasonCode = "incompatible_runtime"
	RefusalUnsupportedContext  RefusalReasonCode = "unsupported_context_class"
	RefusalContextTooLarge     RefusalReasonCode = "context_too_large"
	RefusalAdmissionClosed     RefusalReasonCode = "admission_closed"
	RefusalSensitivityDenied   RefusalReasonCode = "sensitivity_not_allowed"
	RefusalDestinationBusy     RefusalReasonCode = "destination_overloaded"
	RefusalInvalidLease        RefusalReasonCode = "invalid_handoff_token"
	RefusalExpiredOffer        RefusalReasonCode = "expired_offer"
	RefusalUntrustedPeer       RefusalReasonCode = "untrusted_peer"
	RefusalTransferBudget      RefusalReasonCode = "transfer_budget_exceeded"
	RefusalDuplicateHandoff    RefusalReasonCode = "duplicate_handoff"
	RefusalStaleEpoch          RefusalReasonCode = "stale_epoch"
)

type CapabilityEnvelope struct {
	AllowedCapabilityIDs []string          `json:"allowed_capability_ids,omitempty" yaml:"allowed_capability_ids,omitempty"`
	AllowedTaskClasses   []string          `json:"allowed_task_classes,omitempty" yaml:"allowed_task_classes,omitempty"`
	SecretScopes         []string          `json:"secret_scopes,omitempty" yaml:"secret_scopes,omitempty"`
	NetworkEgressClass   string            `json:"network_egress_class,omitempty" yaml:"network_egress_class,omitempty"`
	StorageAccessClass   string            `json:"storage_access_class,omitempty" yaml:"storage_access_class,omitempty"`
	ObservabilityLevel   string            `json:"observability_level,omitempty" yaml:"observability_level,omitempty"`
	AllowChildTasks      bool              `json:"allow_child_tasks,omitempty" yaml:"allow_child_tasks,omitempty"`
	AllowOnwardExport    bool              `json:"allow_onward_export,omitempty" yaml:"allow_onward_export,omitempty"`
	MaxCPU               int               `json:"max_cpu,omitempty" yaml:"max_cpu,omitempty"`
	MaxMemoryMB          int               `json:"max_memory_mb,omitempty" yaml:"max_memory_mb,omitempty"`
	MaxRuntimeSeconds    int               `json:"max_runtime_seconds,omitempty" yaml:"max_runtime_seconds,omitempty"`
	Labels               map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

type RuntimeDescriptor struct {
	RuntimeID                 string            `json:"runtime_id" yaml:"runtime_id"`
	NodeID                    string            `json:"node_id" yaml:"node_id"`
	TrustDomain               string            `json:"trust_domain,omitempty" yaml:"trust_domain,omitempty"`
	RuntimeVersion            string            `json:"runtime_version" yaml:"runtime_version"`
	SandboxVersion            string            `json:"sandbox_version,omitempty" yaml:"sandbox_version,omitempty"`
	SupportedContextClasses   []string          `json:"supported_context_classes,omitempty" yaml:"supported_context_classes,omitempty"`
	SupportedEncryptionSuites []string          `json:"supported_encryption_suites,omitempty" yaml:"supported_encryption_suites,omitempty"`
	CompatibilityClass        string            `json:"compatibility_class,omitempty" yaml:"compatibility_class,omitempty"`
	MaxContextSize            int64             `json:"max_context_size,omitempty" yaml:"max_context_size,omitempty"`
	MaxConcurrentResumes      int               `json:"max_concurrent_resumes,omitempty" yaml:"max_concurrent_resumes,omitempty"`
	AttestationProfile        string            `json:"attestation_profile,omitempty" yaml:"attestation_profile,omitempty"`
	AttestationClaims         map[string]string `json:"attestation_claims,omitempty" yaml:"attestation_claims,omitempty"`
	ExpiresAt                 time.Time         `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
	SignatureAlgorithm        string            `json:"signature_algorithm,omitempty" yaml:"signature_algorithm,omitempty"`
	Signature                 string            `json:"signature,omitempty" yaml:"signature,omitempty"`
}

type ExportDescriptor struct {
	ExportName                   string           `json:"export_name" yaml:"export_name"`
	TrustDomain                  string           `json:"trust_domain,omitempty" yaml:"trust_domain,omitempty"`
	AcceptedSourceDomains        []string         `json:"accepted_source_domains,omitempty" yaml:"accepted_source_domains,omitempty"`
	AcceptedIdentities           []SubjectRef     `json:"accepted_identities,omitempty" yaml:"accepted_identities,omitempty"`
	AcceptedContextClasses       []string         `json:"accepted_context_classes,omitempty" yaml:"accepted_context_classes,omitempty"`
	AllowedCapabilityIDs         []string         `json:"allowed_capability_ids,omitempty" yaml:"allowed_capability_ids,omitempty"`
	AllowedTaskClasses           []string         `json:"allowed_task_classes,omitempty" yaml:"allowed_task_classes,omitempty"`
	AllowOnwardTransfer          *bool            `json:"allow_onward_transfer,omitempty" yaml:"allow_onward_transfer,omitempty"`
	MaxContextSize               int64            `json:"max_context_size,omitempty" yaml:"max_context_size,omitempty"`
	SensitivityLimit             SensitivityClass `json:"sensitivity_limit,omitempty" yaml:"sensitivity_limit,omitempty"`
	RequiredCompatibilityClasses []string         `json:"required_compatibility_classes,omitempty" yaml:"required_compatibility_classes,omitempty"`
	RouteMode                    RouteMode        `json:"route_mode,omitempty" yaml:"route_mode,omitempty"`
	AdmissionSummary             AvailabilitySpec `json:"admission_summary,omitempty" yaml:"admission_summary,omitempty"`
	AllowedTransportPaths        []RouteMode      `json:"allowed_transport_paths,omitempty" yaml:"allowed_transport_paths,omitempty"`
	SignatureAlgorithm           string           `json:"signature_algorithm,omitempty" yaml:"signature_algorithm,omitempty"`
	Signature                    string           `json:"signature,omitempty" yaml:"signature,omitempty"`
}

type LineageRecord struct {
	LineageID                string                    `json:"lineage_id" yaml:"lineage_id"`
	TenantID                 string                    `json:"tenant_id" yaml:"tenant_id"`
	ParentLineageID          string                    `json:"parent_lineage_id,omitempty" yaml:"parent_lineage_id,omitempty"`
	TaskClass                string                    `json:"task_class" yaml:"task_class"`
	ContextClass             string                    `json:"context_class" yaml:"context_class"`
	CurrentOwnerAttempt      string                    `json:"current_owner_attempt,omitempty" yaml:"current_owner_attempt,omitempty"`
	CurrentOwnerRuntime      string                    `json:"current_owner_runtime,omitempty" yaml:"current_owner_runtime,omitempty"`
	CapabilityEnvelope       CapabilityEnvelope        `json:"capability_envelope,omitempty" yaml:"capability_envelope,omitempty"`
	SensitivityClass         SensitivityClass          `json:"sensitivity_class,omitempty" yaml:"sensitivity_class,omitempty"`
	AllowedFederationTargets []string                  `json:"allowed_federation_targets,omitempty" yaml:"allowed_federation_targets,omitempty"`
	Owner                    SubjectRef                `json:"owner" yaml:"owner"`
	SessionID                string                    `json:"session_id,omitempty" yaml:"session_id,omitempty"`
	SessionBinding           *ExternalSessionBinding   `json:"session_binding,omitempty" yaml:"session_binding,omitempty"`
	Delegations              []SessionDelegationRecord `json:"delegations,omitempty" yaml:"delegations,omitempty"`
	TrustClass               TrustClass                `json:"trust_class,omitempty" yaml:"trust_class,omitempty"`
	CreatedAt                time.Time                 `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	UpdatedAt                time.Time                 `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
	LineageVersion           int64                     `json:"lineage_version,omitempty" yaml:"lineage_version,omitempty"`
}

type AttemptRecord struct {
	AttemptID         string       `json:"attempt_id" yaml:"attempt_id"`
	LineageID         string       `json:"lineage_id" yaml:"lineage_id"`
	RuntimeID         string       `json:"runtime_id" yaml:"runtime_id"`
	State             AttemptState `json:"state" yaml:"state"`
	LeaseID           string       `json:"lease_id,omitempty" yaml:"lease_id,omitempty"`
	LeaseExpiry       time.Time    `json:"lease_expiry,omitempty" yaml:"lease_expiry,omitempty"`
	StartTime         time.Time    `json:"start_time,omitempty" yaml:"start_time,omitempty"`
	LastProgressTime  time.Time    `json:"last_progress_time,omitempty" yaml:"last_progress_time,omitempty"`
	Fenced            bool         `json:"fenced,omitempty" yaml:"fenced,omitempty"`
	FencingEpoch      int64        `json:"fencing_epoch,omitempty" yaml:"fencing_epoch,omitempty"`
	PreviousAttemptID string       `json:"previous_attempt_id,omitempty" yaml:"previous_attempt_id,omitempty"`
}

type ContextManifest struct {
	ContextID          string           `json:"context_id" yaml:"context_id"`
	LineageID          string           `json:"lineage_id" yaml:"lineage_id"`
	AttemptID          string           `json:"attempt_id" yaml:"attempt_id"`
	ContextClass       string           `json:"context_class" yaml:"context_class"`
	SchemaVersion      string           `json:"schema_version" yaml:"schema_version"`
	SizeBytes          int64            `json:"size_bytes,omitempty" yaml:"size_bytes,omitempty"`
	ChunkCount         int              `json:"chunk_count,omitempty" yaml:"chunk_count,omitempty"`
	ContentHash        string           `json:"content_hash" yaml:"content_hash"`
	SensitivityClass   SensitivityClass `json:"sensitivity_class,omitempty" yaml:"sensitivity_class,omitempty"`
	TTL                time.Duration    `json:"ttl,omitempty" yaml:"ttl,omitempty"`
	ObjectRefs         []string         `json:"object_refs,omitempty" yaml:"object_refs,omitempty"`
	TransferMode       TransferMode     `json:"transfer_mode,omitempty" yaml:"transfer_mode,omitempty"`
	EncryptionMode     EncryptionMode   `json:"encryption_mode,omitempty" yaml:"encryption_mode,omitempty"`
	RecipientSet       []string         `json:"recipient_set,omitempty" yaml:"recipient_set,omitempty"`
	CreationTime       time.Time        `json:"creation_time,omitempty" yaml:"creation_time,omitempty"`
	SignatureAlgorithm string           `json:"signature_algorithm,omitempty" yaml:"signature_algorithm,omitempty"`
	Signature          string           `json:"signature,omitempty" yaml:"signature,omitempty"`
}

type SealedContext struct {
	EnvelopeVersion      string         `json:"envelope_version" yaml:"envelope_version"`
	ContextManifestRef   string         `json:"context_manifest_ref" yaml:"context_manifest_ref"`
	CipherSuite          string         `json:"cipher_suite" yaml:"cipher_suite"`
	RecipientBindings    []string       `json:"recipient_bindings,omitempty" yaml:"recipient_bindings,omitempty"`
	CiphertextChunks     [][]byte       `json:"ciphertext_chunks,omitempty" yaml:"ciphertext_chunks,omitempty"`
	ExternalObjectRefs   []string       `json:"external_object_refs,omitempty" yaml:"external_object_refs,omitempty"`
	IntegrityTag         string         `json:"integrity_tag" yaml:"integrity_tag"`
	ReplayProtectionData map[string]any `json:"replay_protection_data,omitempty" yaml:"replay_protection_data,omitempty"`
}

type LeaseToken struct {
	LeaseID            string    `json:"lease_id" yaml:"lease_id"`
	LineageID          string    `json:"lineage_id" yaml:"lineage_id"`
	AttemptID          string    `json:"attempt_id" yaml:"attempt_id"`
	Issuer             string    `json:"issuer" yaml:"issuer"`
	IssuedAt           time.Time `json:"issued_at" yaml:"issued_at"`
	Expiry             time.Time `json:"expiry" yaml:"expiry"`
	FencingEpoch       int64     `json:"fencing_epoch" yaml:"fencing_epoch"`
	SignatureAlgorithm string    `json:"signature_algorithm,omitempty" yaml:"signature_algorithm,omitempty"`
	Signature          string    `json:"signature,omitempty" yaml:"signature,omitempty"`
}

type TraceContext struct {
	TraceID string `json:"trace_id,omitempty" yaml:"trace_id,omitempty"`
	SpanID  string `json:"span_id,omitempty" yaml:"span_id,omitempty"`
}

type HandoffOffer struct {
	OfferID                       string             `json:"offer_id" yaml:"offer_id"`
	LineageID                     string             `json:"lineage_id" yaml:"lineage_id"`
	SourceAttemptID               string             `json:"source_attempt_id" yaml:"source_attempt_id"`
	SourceRuntimeID               string             `json:"source_runtime_id" yaml:"source_runtime_id"`
	SourceCompatibilityClass      string             `json:"source_compatibility_class,omitempty" yaml:"source_compatibility_class,omitempty"`
	DestinationExport             string             `json:"destination_export" yaml:"destination_export"`
	ContextManifestRef            string             `json:"context_manifest_ref" yaml:"context_manifest_ref"`
	ContextClass                  string             `json:"context_class" yaml:"context_class"`
	ContextSizeBytes              int64              `json:"context_size_bytes,omitempty" yaml:"context_size_bytes,omitempty"`
	SensitivityClass              SensitivityClass   `json:"sensitivity_class,omitempty" yaml:"sensitivity_class,omitempty"`
	RequestedCapabilityProjection CapabilityEnvelope `json:"requested_capability_projection,omitempty" yaml:"requested_capability_projection,omitempty"`
	LeaseToken                    LeaseToken         `json:"lease_token" yaml:"lease_token"`
	Expiry                        time.Time          `json:"expiry" yaml:"expiry"`
	TraceContext                  TraceContext       `json:"trace_context,omitempty" yaml:"trace_context,omitempty"`
	SignatureAlgorithm            string             `json:"signature_algorithm,omitempty" yaml:"signature_algorithm,omitempty"`
	Signature                     string             `json:"signature,omitempty" yaml:"signature,omitempty"`
}

type HandoffAccept struct {
	OfferID                      string             `json:"offer_id" yaml:"offer_id"`
	DestinationRuntimeID         string             `json:"destination_runtime_id" yaml:"destination_runtime_id"`
	AcceptedContextClass         string             `json:"accepted_context_class" yaml:"accepted_context_class"`
	AcceptedCapabilityProjection CapabilityEnvelope `json:"accepted_capability_projection,omitempty" yaml:"accepted_capability_projection,omitempty"`
	RewrapRequest                bool               `json:"rewrap_request,omitempty" yaml:"rewrap_request,omitempty"`
	ProvisionalAttemptID         string             `json:"provisional_attempt_id" yaml:"provisional_attempt_id"`
	Expiry                       time.Time          `json:"expiry" yaml:"expiry"`
	SignatureAlgorithm           string             `json:"signature_algorithm,omitempty" yaml:"signature_algorithm,omitempty"`
	Signature                    string             `json:"signature,omitempty" yaml:"signature,omitempty"`
}

type ResumeCommit struct {
	LineageID            string    `json:"lineage_id" yaml:"lineage_id"`
	OldAttemptID         string    `json:"old_attempt_id" yaml:"old_attempt_id"`
	NewAttemptID         string    `json:"new_attempt_id" yaml:"new_attempt_id"`
	DestinationRuntimeID string    `json:"destination_runtime_id" yaml:"destination_runtime_id"`
	ReceiptRef           string    `json:"receipt_ref" yaml:"receipt_ref"`
	CommitTime           time.Time `json:"commit_time" yaml:"commit_time"`
	SignatureAlgorithm   string    `json:"signature_algorithm,omitempty" yaml:"signature_algorithm,omitempty"`
	Signature            string    `json:"signature,omitempty" yaml:"signature,omitempty"`
}

type FenceNotice struct {
	LineageID          string    `json:"lineage_id" yaml:"lineage_id"`
	AttemptID          string    `json:"attempt_id" yaml:"attempt_id"`
	FencingEpoch       int64     `json:"fencing_epoch" yaml:"fencing_epoch"`
	Reason             string    `json:"reason,omitempty" yaml:"reason,omitempty"`
	Issuer             string    `json:"issuer" yaml:"issuer"`
	IssuedAt           time.Time `json:"issued_at,omitempty" yaml:"issued_at,omitempty"`
	SignatureAlgorithm string    `json:"signature_algorithm,omitempty" yaml:"signature_algorithm,omitempty"`
	Signature          string    `json:"signature,omitempty" yaml:"signature,omitempty"`
}

type ResumeReceipt struct {
	ReceiptID                   string             `json:"receipt_id" yaml:"receipt_id"`
	LineageID                   string             `json:"lineage_id" yaml:"lineage_id"`
	AttemptID                   string             `json:"attempt_id" yaml:"attempt_id"`
	RuntimeID                   string             `json:"runtime_id" yaml:"runtime_id"`
	ImportedContextID           string             `json:"imported_context_id" yaml:"imported_context_id"`
	StartTime                   time.Time          `json:"start_time,omitempty" yaml:"start_time,omitempty"`
	CompatibilityVerified       bool               `json:"compatibility_verified,omitempty" yaml:"compatibility_verified,omitempty"`
	CapabilityProjectionApplied CapabilityEnvelope `json:"capability_projection_applied,omitempty" yaml:"capability_projection_applied,omitempty"`
	Status                      ReceiptStatus      `json:"status" yaml:"status"`
	SignatureAlgorithm          string             `json:"signature_algorithm,omitempty" yaml:"signature_algorithm,omitempty"`
	Signature                   string             `json:"signature,omitempty" yaml:"signature,omitempty"`
}

type TransferRefusal struct {
	Code    RefusalReasonCode `json:"code" yaml:"code"`
	Message string            `json:"message,omitempty" yaml:"message,omitempty"`
	RetryAt time.Time         `json:"retry_at,omitempty" yaml:"retry_at,omitempty"`
}

type NodeAdvertisement struct {
	TrustDomain string         `json:"trust_domain" yaml:"trust_domain"`
	Node        NodeDescriptor `json:"node" yaml:"node"`
	Locality    string         `json:"locality,omitempty" yaml:"locality,omitempty"`
	Health      NodeHealth     `json:"health,omitempty" yaml:"health,omitempty"`
	ExpiresAt   time.Time      `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
	Signature   string         `json:"signature,omitempty" yaml:"signature,omitempty"`
}

type RuntimeAdvertisement struct {
	TrustDomain string            `json:"trust_domain" yaml:"trust_domain"`
	Runtime     RuntimeDescriptor `json:"runtime" yaml:"runtime"`
	ExpiresAt   time.Time         `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
	Signature   string            `json:"signature,omitempty" yaml:"signature,omitempty"`
}

type ExportAdvertisement struct {
	TrustDomain string           `json:"trust_domain" yaml:"trust_domain"`
	Export      ExportDescriptor `json:"export" yaml:"export"`
	RuntimeID   string           `json:"runtime_id,omitempty" yaml:"runtime_id,omitempty"`
	NodeID      string           `json:"node_id,omitempty" yaml:"node_id,omitempty"`
	Imported    bool             `json:"imported,omitempty" yaml:"imported,omitempty"`
	ExpiresAt   time.Time        `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
	Signature   string           `json:"signature,omitempty" yaml:"signature,omitempty"`
	// Phase 7.3: DR metadata for federation health summaries
	FailoverReady  bool      `json:"failover_ready,omitempty" yaml:"failover_ready,omitempty"`
	RecoveryState  string    `json:"recovery_state,omitempty" yaml:"recovery_state,omitempty"`
	RuntimeVersion string    `json:"runtime_version,omitempty" yaml:"runtime_version,omitempty"`
	LastCheckpoint time.Time `json:"last_checkpoint,omitempty" yaml:"last_checkpoint,omitempty"`
}

type RecipientKeyAdvertisement struct {
	Recipient string    `json:"recipient" yaml:"recipient"`
	KeyID     string    `json:"key_id,omitempty" yaml:"key_id,omitempty"`
	Version   string    `json:"version,omitempty" yaml:"version,omitempty"`
	PublicKey []byte    `json:"public_key" yaml:"public_key"`
	Active    bool      `json:"active,omitempty" yaml:"active,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
	RevokedAt time.Time `json:"revoked_at,omitempty" yaml:"revoked_at,omitempty"`
}

type TrustBundle struct {
	TrustDomain        string                      `json:"trust_domain" yaml:"trust_domain"`
	BundleID           string                      `json:"bundle_id" yaml:"bundle_id"`
	GatewayIdentities  []SubjectRef                `json:"gateway_identities,omitempty" yaml:"gateway_identities,omitempty"`
	TrustAnchors       []string                    `json:"trust_anchors,omitempty" yaml:"trust_anchors,omitempty"`
	RecipientKeys      []RecipientKeyAdvertisement `json:"recipient_keys,omitempty" yaml:"recipient_keys,omitempty"`
	IssuedAt           time.Time                   `json:"issued_at,omitempty" yaml:"issued_at,omitempty"`
	ExpiresAt          time.Time                   `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
	SignatureAlgorithm string                      `json:"signature_algorithm,omitempty" yaml:"signature_algorithm,omitempty"`
	Signature          string                      `json:"signature,omitempty" yaml:"signature,omitempty"`
}

type BoundaryPolicy struct {
	TrustDomain                  string       `json:"trust_domain" yaml:"trust_domain"`
	AcceptedSourceDomains        []string     `json:"accepted_source_domains,omitempty" yaml:"accepted_source_domains,omitempty"`
	AcceptedSourceIdentities     []SubjectRef `json:"accepted_source_identities,omitempty" yaml:"accepted_source_identities,omitempty"`
	AllowedRouteModes            []RouteMode  `json:"allowed_route_modes,omitempty" yaml:"allowed_route_modes,omitempty"`
	RequireGatewayAuthentication bool         `json:"require_gateway_authentication,omitempty" yaml:"require_gateway_authentication,omitempty"`
	AllowMediation               bool         `json:"allow_mediation,omitempty" yaml:"allow_mediation,omitempty"`
	MaxTransferBytes             int64        `json:"max_transfer_bytes,omitempty" yaml:"max_transfer_bytes,omitempty"`
	MaxRetries                   int          `json:"max_retries,omitempty" yaml:"max_retries,omitempty"`
	RetryBackoffSeconds          int          `json:"retry_backoff_seconds,omitempty" yaml:"retry_backoff_seconds,omitempty"`
}

type TenantFederationPolicy struct {
	TenantID            string      `json:"tenant_id" yaml:"tenant_id"`
	AllowedTrustDomains []string    `json:"allowed_trust_domains,omitempty" yaml:"allowed_trust_domains,omitempty"`
	AllowedRouteModes   []RouteMode `json:"allowed_route_modes,omitempty" yaml:"allowed_route_modes,omitempty"`
	AllowMediation      bool        `json:"allow_mediation,omitempty" yaml:"allow_mediation,omitempty"`
	MaxTransferBytes    int64       `json:"max_transfer_bytes,omitempty" yaml:"max_transfer_bytes,omitempty"`
	UpdatedAt           time.Time   `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
}

type GatewayForwardRequest struct {
	TenantID           string        `json:"tenant_id,omitempty" yaml:"tenant_id,omitempty"`
	LineageID          string        `json:"lineage_id,omitempty" yaml:"lineage_id,omitempty"`
	TrustDomain        string        `json:"trust_domain" yaml:"trust_domain"`
	SourceDomain       string        `json:"source_domain" yaml:"source_domain"`
	GatewayIdentity    SubjectRef    `json:"gateway_identity" yaml:"gateway_identity"`
	DestinationExport  string        `json:"destination_export" yaml:"destination_export"`
	OfferID            string        `json:"offer_id,omitempty" yaml:"offer_id,omitempty"`
	RouteMode          RouteMode     `json:"route_mode" yaml:"route_mode"`
	MediationRequested bool          `json:"mediation_requested,omitempty" yaml:"mediation_requested,omitempty"`
	SizeBytes          int64         `json:"size_bytes,omitempty" yaml:"size_bytes,omitempty"`
	ContextManifestRef string        `json:"context_manifest_ref" yaml:"context_manifest_ref"`
	SealedContext      SealedContext `json:"sealed_context" yaml:"sealed_context"`
}

type GatewayForwardResult struct {
	TrustDomain       string    `json:"trust_domain" yaml:"trust_domain"`
	DestinationExport string    `json:"destination_export" yaml:"destination_export"`
	RouteMode         RouteMode `json:"route_mode" yaml:"route_mode"`
	Opaque            bool      `json:"opaque" yaml:"opaque"`
	ForwardedAt       time.Time `json:"forwarded_at,omitempty" yaml:"forwarded_at,omitempty"`
}

type MessageEnvelope struct {
	ProtocolVersion   string         `json:"protocol_version" yaml:"protocol_version"`
	MessageType       string         `json:"message_type" yaml:"message_type"`
	SourceIdentity    string         `json:"source_identity,omitempty" yaml:"source_identity,omitempty"`
	DestinationTarget string         `json:"destination_target,omitempty" yaml:"destination_target,omitempty"`
	ObjectRef         string         `json:"object_ref,omitempty" yaml:"object_ref,omitempty"`
	CiphertextRef     string         `json:"ciphertext_ref,omitempty" yaml:"ciphertext_ref,omitempty"`
	PolicyLabels      []string       `json:"policy_labels,omitempty" yaml:"policy_labels,omitempty"`
	HandoffToken      string         `json:"handoff_token,omitempty" yaml:"handoff_token,omitempty"`
	SizeBytes         int64          `json:"size_bytes,omitempty" yaml:"size_bytes,omitempty"`
	ChunkCount        int            `json:"chunk_count,omitempty" yaml:"chunk_count,omitempty"`
	Expiry            time.Time      `json:"expiry,omitempty" yaml:"expiry,omitempty"`
	Nonce             string         `json:"nonce,omitempty" yaml:"nonce,omitempty"`
	CryptoSuite       string         `json:"crypto_suite,omitempty" yaml:"crypto_suite,omitempty"`
	TraceContext      TraceContext   `json:"trace_context,omitempty" yaml:"trace_context,omitempty"`
	Payload           map[string]any `json:"payload,omitempty" yaml:"payload,omitempty"`
}

func (s SensitivityClass) Validate() error {
	switch s {
	case "", SensitivityClassLow, SensitivityClassModerate, SensitivityClassHigh, SensitivityClassRestricted:
		return nil
	default:
		return fmt.Errorf("sensitivity class %s invalid", s)
	}
}

func (m RouteMode) Validate() error {
	switch m {
	case "", RouteModeDirect, RouteModeGateway, RouteModeMediated:
		return nil
	default:
		return fmt.Errorf("route mode %s invalid", m)
	}
}

func (m EncryptionMode) Validate() error {
	switch m {
	case "", EncryptionModeLinkOnly, EncryptionModeObject, EncryptionModeEndToEnd, EncryptionModeEndToEndWrap:
		return nil
	default:
		return fmt.Errorf("encryption mode %s invalid", m)
	}
}

func (m TransferMode) Validate() error {
	switch m {
	case "", TransferModeInline, TransferModeChunked, TransferModeExternal:
		return nil
	default:
		return fmt.Errorf("transfer mode %s invalid", m)
	}
}

func (r RefusalReasonCode) Validate() error {
	switch r {
	case RefusalUnauthorized, RefusalIncompatibleRuntime, RefusalUnsupportedContext,
		RefusalContextTooLarge, RefusalAdmissionClosed, RefusalSensitivityDenied,
		RefusalDestinationBusy, RefusalInvalidLease, RefusalExpiredOffer,
		RefusalUntrustedPeer, RefusalTransferBudget,
		RefusalDuplicateHandoff, RefusalStaleEpoch:
		return nil
	default:
		return fmt.Errorf("refusal reason code %s invalid", r)
	}
}

func (p TenantFederationPolicy) Validate() error {
	if strings.TrimSpace(p.TenantID) == "" {
		return errors.New("tenant federation policy tenant_id required")
	}
	for _, trustDomain := range p.AllowedTrustDomains {
		if strings.TrimSpace(trustDomain) == "" {
			return errors.New("tenant federation policy trust domain must not be empty")
		}
	}
	for _, mode := range p.AllowedRouteModes {
		if err := mode.Validate(); err != nil {
			return fmt.Errorf("tenant federation policy route mode invalid: %w", err)
		}
	}
	if p.MaxTransferBytes < 0 {
		return errors.New("tenant federation policy max transfer bytes must be >= 0")
	}
	return nil
}

func (s AttemptState) Validate() error {
	switch s {
	case AttemptStateCreated, AttemptStateAdmitted, AttemptStateRunning, AttemptStateCheckpointing,
		AttemptStateHandoffOffered, AttemptStateHandoffAccepted, AttemptStateResumePending,
		AttemptStateCommittedRemote, AttemptStateFenced, AttemptStateCompleted,
		AttemptStateFailed, AttemptStateOrphaned:
		return nil
	default:
		return fmt.Errorf("attempt state %s invalid", s)
	}
}

func (s ReceiptStatus) Validate() error {
	switch s {
	case ReceiptStatusRunning, ReceiptStatusRejected, ReceiptStatusFailed:
		return nil
	default:
		return fmt.Errorf("receipt status %s invalid", s)
	}
}

func (e CapabilityEnvelope) Validate() error {
	if e.MaxCPU < 0 || e.MaxMemoryMB < 0 || e.MaxRuntimeSeconds < 0 {
		return errors.New("capability envelope limits must be >= 0")
	}
	for _, value := range append(append([]string{}, e.AllowedCapabilityIDs...), e.AllowedTaskClasses...) {
		if strings.TrimSpace(value) == "" {
			return errors.New("capability envelope values must not be empty")
		}
	}
	return nil
}

func (d RuntimeDescriptor) Validate() error {
	if strings.TrimSpace(d.RuntimeID) == "" {
		return errors.New("runtime id required")
	}
	if strings.TrimSpace(d.NodeID) == "" {
		return errors.New("node id required")
	}
	if strings.TrimSpace(d.RuntimeVersion) == "" {
		return errors.New("runtime version required")
	}
	if d.MaxContextSize < 0 || d.MaxConcurrentResumes < 0 {
		return errors.New("runtime descriptor limits must be >= 0")
	}
	return nil
}

func (d ExportDescriptor) Validate() error {
	if strings.TrimSpace(d.ExportName) == "" {
		return errors.New("export name required")
	}
	if err := d.RouteMode.Validate(); err != nil {
		return err
	}
	if err := d.SensitivityLimit.Validate(); err != nil {
		return err
	}
	for _, route := range d.AllowedTransportPaths {
		if err := route.Validate(); err != nil {
			return err
		}
	}
	for _, capabilityID := range d.AllowedCapabilityIDs {
		if strings.TrimSpace(capabilityID) == "" {
			return errors.New("allowed capability ids must not contain empty values")
		}
	}
	for _, taskClass := range d.AllowedTaskClasses {
		if strings.TrimSpace(taskClass) == "" {
			return errors.New("allowed task classes must not contain empty values")
		}
	}
	for _, subject := range d.AcceptedIdentities {
		if err := subject.Validate(); err != nil {
			return fmt.Errorf("accepted identity invalid: %w", err)
		}
	}
	return nil
}

func (r LineageRecord) Validate() error {
	if strings.TrimSpace(r.LineageID) == "" {
		return errors.New("lineage id required")
	}
	if strings.TrimSpace(r.TenantID) == "" {
		return errors.New("tenant id required")
	}
	if strings.TrimSpace(r.TaskClass) == "" {
		return errors.New("task class required")
	}
	if strings.TrimSpace(r.ContextClass) == "" {
		return errors.New("context class required")
	}
	if err := r.Owner.Validate(); err != nil {
		return fmt.Errorf("owner invalid: %w", err)
	}
	if !strings.EqualFold(r.Owner.TenantID, r.TenantID) {
		return errors.New("lineage tenant_id must match owner tenant_id")
	}
	if err := r.CapabilityEnvelope.Validate(); err != nil {
		return err
	}
	if err := r.SensitivityClass.Validate(); err != nil {
		return err
	}
	if r.SessionBinding != nil {
		if err := r.SessionBinding.Validate(); err != nil {
			return fmt.Errorf("session binding invalid: %w", err)
		}
	}
	for _, delegation := range r.Delegations {
		if err := delegation.Validate(); err != nil {
			return fmt.Errorf("delegation invalid: %w", err)
		}
	}
	return nil
}

func (r AttemptRecord) Validate() error {
	if strings.TrimSpace(r.AttemptID) == "" {
		return errors.New("attempt id required")
	}
	if strings.TrimSpace(r.LineageID) == "" {
		return errors.New("lineage id required")
	}
	if strings.TrimSpace(r.RuntimeID) == "" {
		return errors.New("runtime id required")
	}
	if err := r.State.Validate(); err != nil {
		return err
	}
	if !r.LeaseExpiry.IsZero() && !r.StartTime.IsZero() && r.LeaseExpiry.Before(r.StartTime) {
		return errors.New("lease expiry must not be before start time")
	}
	return nil
}

func (b TrustBundle) Validate() error {
	if strings.TrimSpace(b.TrustDomain) == "" {
		return errors.New("trust domain required")
	}
	if strings.TrimSpace(b.BundleID) == "" {
		return errors.New("bundle id required")
	}
	if !b.ExpiresAt.IsZero() && !b.IssuedAt.IsZero() && b.ExpiresAt.Before(b.IssuedAt) {
		return errors.New("bundle expiry must not be before issued_at")
	}
	for _, subject := range b.GatewayIdentities {
		if err := subject.Validate(); err != nil {
			return fmt.Errorf("gateway identity invalid: %w", err)
		}
	}
	for _, anchor := range b.TrustAnchors {
		if strings.TrimSpace(anchor) == "" {
			return errors.New("trust anchors must not contain empty values")
		}
	}
	for _, key := range b.RecipientKeys {
		if strings.TrimSpace(key.Recipient) == "" {
			return errors.New("recipient keys require recipient")
		}
		if len(key.PublicKey) == 0 {
			return errors.New("recipient keys require public_key")
		}
	}
	return nil
}

func (p BoundaryPolicy) Validate() error {
	if strings.TrimSpace(p.TrustDomain) == "" {
		return errors.New("trust domain required")
	}
	if p.MaxTransferBytes < 0 {
		return errors.New("max transfer bytes must be >= 0")
	}
	if p.MaxRetries < 0 || p.RetryBackoffSeconds < 0 {
		return errors.New("retry controls must be >= 0")
	}
	for _, mode := range p.AllowedRouteModes {
		if err := mode.Validate(); err != nil {
			return err
		}
	}
	for _, domain := range p.AcceptedSourceDomains {
		if strings.TrimSpace(domain) == "" {
			return errors.New("accepted source domains must not contain empty values")
		}
	}
	for _, subject := range p.AcceptedSourceIdentities {
		if err := subject.Validate(); err != nil {
			return fmt.Errorf("accepted source identity invalid: %w", err)
		}
	}
	return nil
}

func (r GatewayForwardRequest) Validate() error {
	if strings.TrimSpace(r.TenantID) == "" && strings.TrimSpace(r.LineageID) == "" {
		return errors.New("tenant id or lineage id required")
	}
	if strings.TrimSpace(r.TrustDomain) == "" {
		return errors.New("trust domain required")
	}
	if strings.TrimSpace(r.SourceDomain) == "" {
		return errors.New("source domain required")
	}
	if err := r.GatewayIdentity.Validate(); err != nil {
		return fmt.Errorf("gateway identity invalid: %w", err)
	}
	if strings.TrimSpace(r.DestinationExport) == "" {
		return errors.New("destination export required")
	}
	if err := r.RouteMode.Validate(); err != nil {
		return err
	}
	if r.SizeBytes < 0 {
		return errors.New("size bytes must be >= 0")
	}
	if strings.TrimSpace(r.ContextManifestRef) == "" {
		return errors.New("context manifest ref required")
	}
	if err := r.SealedContext.Validate(); err != nil {
		return fmt.Errorf("sealed context invalid: %w", err)
	}
	return nil
}

func (r GatewayForwardResult) Validate() error {
	if strings.TrimSpace(r.TrustDomain) == "" {
		return errors.New("trust domain required")
	}
	if strings.TrimSpace(r.DestinationExport) == "" {
		return errors.New("destination export required")
	}
	return r.RouteMode.Validate()
}

func (m ContextManifest) Validate() error {
	if strings.TrimSpace(m.ContextID) == "" {
		return errors.New("context id required")
	}
	if strings.TrimSpace(m.LineageID) == "" {
		return errors.New("lineage id required")
	}
	if strings.TrimSpace(m.AttemptID) == "" {
		return errors.New("attempt id required")
	}
	if strings.TrimSpace(m.ContextClass) == "" {
		return errors.New("context class required")
	}
	if strings.TrimSpace(m.SchemaVersion) == "" {
		return errors.New("schema version required")
	}
	if strings.TrimSpace(m.ContentHash) == "" {
		return errors.New("content hash required")
	}
	if m.SizeBytes < 0 || m.ChunkCount < 0 {
		return errors.New("context manifest limits must be >= 0")
	}
	if err := m.SensitivityClass.Validate(); err != nil {
		return err
	}
	if err := m.TransferMode.Validate(); err != nil {
		return err
	}
	return m.EncryptionMode.Validate()
}

func (s SealedContext) Validate() error {
	if strings.TrimSpace(s.EnvelopeVersion) == "" {
		return errors.New("envelope version required")
	}
	if strings.TrimSpace(s.ContextManifestRef) == "" {
		return errors.New("context manifest ref required")
	}
	if strings.TrimSpace(s.CipherSuite) == "" {
		return errors.New("cipher suite required")
	}
	if strings.TrimSpace(s.IntegrityTag) == "" {
		return errors.New("integrity tag required")
	}
	if len(s.CiphertextChunks) == 0 && len(s.ExternalObjectRefs) == 0 {
		return errors.New("ciphertext or object refs required")
	}
	return nil
}

func (t LeaseToken) Validate() error {
	if strings.TrimSpace(t.LeaseID) == "" {
		return errors.New("lease id required")
	}
	if strings.TrimSpace(t.LineageID) == "" {
		return errors.New("lineage id required")
	}
	if strings.TrimSpace(t.AttemptID) == "" {
		return errors.New("attempt id required")
	}
	if strings.TrimSpace(t.Issuer) == "" {
		return errors.New("issuer required")
	}
	if t.IssuedAt.IsZero() || t.Expiry.IsZero() {
		return errors.New("lease issued_at and expiry required")
	}
	if !t.Expiry.After(t.IssuedAt) {
		return errors.New("lease expiry must be after issued_at")
	}
	return nil
}

func (o HandoffOffer) Validate() error {
	if strings.TrimSpace(o.OfferID) == "" {
		return errors.New("offer id required")
	}
	if strings.TrimSpace(o.LineageID) == "" {
		return errors.New("lineage id required")
	}
	if strings.TrimSpace(o.SourceAttemptID) == "" {
		return errors.New("source attempt id required")
	}
	if strings.TrimSpace(o.SourceRuntimeID) == "" {
		return errors.New("source runtime id required")
	}
	if strings.TrimSpace(o.DestinationExport) == "" {
		return errors.New("destination export required")
	}
	if strings.TrimSpace(o.ContextManifestRef) == "" {
		return errors.New("context manifest ref required")
	}
	if strings.TrimSpace(o.ContextClass) == "" {
		return errors.New("context class required")
	}
	if o.ContextSizeBytes < 0 {
		return errors.New("context size bytes must be >= 0")
	}
	if err := o.SensitivityClass.Validate(); err != nil {
		return err
	}
	if err := o.RequestedCapabilityProjection.Validate(); err != nil {
		return err
	}
	if err := o.LeaseToken.Validate(); err != nil {
		return fmt.Errorf("lease token invalid: %w", err)
	}
	if o.Expiry.IsZero() {
		return errors.New("offer expiry required")
	}
	return nil
}

func (a HandoffAccept) Validate() error {
	if strings.TrimSpace(a.OfferID) == "" {
		return errors.New("offer id required")
	}
	if strings.TrimSpace(a.DestinationRuntimeID) == "" {
		return errors.New("destination runtime id required")
	}
	if strings.TrimSpace(a.AcceptedContextClass) == "" {
		return errors.New("accepted context class required")
	}
	if strings.TrimSpace(a.ProvisionalAttemptID) == "" {
		return errors.New("provisional attempt id required")
	}
	if a.Expiry.IsZero() {
		return errors.New("accept expiry required")
	}
	return a.AcceptedCapabilityProjection.Validate()
}

func (c ResumeCommit) Validate() error {
	if strings.TrimSpace(c.LineageID) == "" {
		return errors.New("lineage id required")
	}
	if strings.TrimSpace(c.OldAttemptID) == "" {
		return errors.New("old attempt id required")
	}
	if strings.TrimSpace(c.NewAttemptID) == "" {
		return errors.New("new attempt id required")
	}
	if strings.TrimSpace(c.DestinationRuntimeID) == "" {
		return errors.New("destination runtime id required")
	}
	if strings.TrimSpace(c.ReceiptRef) == "" {
		return errors.New("receipt ref required")
	}
	if c.CommitTime.IsZero() {
		return errors.New("commit time required")
	}
	return nil
}

func (n FenceNotice) Validate() error {
	if strings.TrimSpace(n.LineageID) == "" {
		return errors.New("lineage id required")
	}
	if strings.TrimSpace(n.AttemptID) == "" {
		return errors.New("attempt id required")
	}
	if strings.TrimSpace(n.Issuer) == "" {
		return errors.New("issuer required")
	}
	if n.FencingEpoch < 0 {
		return errors.New("fencing epoch must be >= 0")
	}
	return nil
}

func (r ResumeReceipt) Validate() error {
	if strings.TrimSpace(r.ReceiptID) == "" {
		return errors.New("receipt id required")
	}
	if strings.TrimSpace(r.LineageID) == "" {
		return errors.New("lineage id required")
	}
	if strings.TrimSpace(r.AttemptID) == "" {
		return errors.New("attempt id required")
	}
	if strings.TrimSpace(r.RuntimeID) == "" {
		return errors.New("runtime id required")
	}
	if strings.TrimSpace(r.ImportedContextID) == "" {
		return errors.New("imported context id required")
	}
	if err := r.Status.Validate(); err != nil {
		return err
	}
	return r.CapabilityProjectionApplied.Validate()
}

func (r TransferRefusal) Validate() error {
	if strings.TrimSpace(string(r.Code)) == "" {
		return errors.New("refusal code required")
	}
	return nil
}

func (a NodeAdvertisement) Validate() error {
	if strings.TrimSpace(a.TrustDomain) == "" {
		return errors.New("trust domain required")
	}
	if err := a.Node.Validate(); err != nil {
		return fmt.Errorf("node invalid: %w", err)
	}
	return nil
}

func (a RuntimeAdvertisement) Validate() error {
	if strings.TrimSpace(a.TrustDomain) == "" {
		return errors.New("trust domain required")
	}
	if err := a.Runtime.Validate(); err != nil {
		return fmt.Errorf("runtime invalid: %w", err)
	}
	return nil
}

func (a ExportAdvertisement) Validate() error {
	if strings.TrimSpace(a.TrustDomain) == "" {
		return errors.New("trust domain required")
	}
	if err := a.Export.Validate(); err != nil {
		return fmt.Errorf("export invalid: %w", err)
	}
	return nil
}

func (e MessageEnvelope) Validate() error {
	if strings.TrimSpace(e.ProtocolVersion) == "" {
		return errors.New("protocol version required")
	}
	if strings.TrimSpace(e.MessageType) == "" {
		return errors.New("message type required")
	}
	if e.SizeBytes < 0 || e.ChunkCount < 0 {
		return errors.New("message envelope limits must be >= 0")
	}
	return nil
}
