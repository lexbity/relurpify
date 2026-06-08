package permissions

import (
	"context"
)

// PermissionType enumerates the supported permission families.
type PermissionType string

const (
	PermissionTypeFilesystem PermissionType = "filesystem"
	PermissionTypeExecutable PermissionType = "executable"
	PermissionTypeNetwork    PermissionType = "network"
	PermissionTypeCapability PermissionType = "capability"
	PermissionTypeIPC        PermissionType = "ipc"
	PermissionTypeHITL       PermissionType = "hitl"
)

// AgentPermissionLevel enumerates allow/deny/ask for permission decisions.
type AgentPermissionLevel string

const (
	AgentPermissionAllow AgentPermissionLevel = "allow"
	AgentPermissionDeny  AgentPermissionLevel = "deny"
	AgentPermissionAsk   AgentPermissionLevel = "ask"
)

// FileSystemAction enumerates filesystem operations.
type FileSystemAction string

const (
	FileSystemRead    FileSystemAction = "fs:read"
	FileSystemWrite   FileSystemAction = "fs:write"
	FileSystemDelete  FileSystemAction = "fs:delete"
	FileSystemRename  FileSystemAction = "fs:rename"
	FileSystemMove    FileSystemAction = "fs:move"
	FileSystemExecute FileSystemAction = "fs:execute"
	FileSystemList    FileSystemAction = "fs:list"
)

// FileSystemPermission scopes access to a portion of the workspace.
type FileSystemPermission struct {
	Action        FileSystemAction `json:"action" yaml:"action"`
	Path          string           `json:"path" yaml:"path"`
	Justification string           `json:"justification,omitempty" yaml:"justification,omitempty"`
	HITLRequired  bool             `json:"hitl_required,omitempty" yaml:"hitl_required,omitempty"`
	ReadOnlyMount bool             `json:"read_only_mount,omitempty" yaml:"read_only_mount,omitempty"`
}

// ExecutablePermission restricts binary execution.
type ExecutablePermission struct {
	Binary        string   `json:"binary" yaml:"binary"`
	Args          []string `json:"args,omitempty" yaml:"args,omitempty"`
	Env           []string `json:"env,omitempty" yaml:"env,omitempty"`
	Checksum      string   `json:"checksum,omitempty" yaml:"checksum,omitempty"`
	HITLRequired  bool     `json:"hitl_required,omitempty" yaml:"hitl_required,omitempty"`
	ProxyRequired bool     `json:"proxy_required,omitempty" yaml:"proxy_required,omitempty"`
}

// NetworkPermission describes network access.
type NetworkPermission struct {
	Direction    string `json:"direction" yaml:"direction"`
	Protocol     string `json:"protocol" yaml:"protocol"`
	Host         string `json:"host,omitempty" yaml:"host,omitempty"`
	Port         int    `json:"port,omitempty" yaml:"port,omitempty"`
	Description  string `json:"description,omitempty" yaml:"description,omitempty"`
	HITLRequired bool   `json:"hitl_required,omitempty" yaml:"hitl_required,omitempty"`
}

// CapabilityPermission enumerates Linux capability requirements.
type CapabilityPermission struct {
	Capability    string `json:"capability" yaml:"capability"`
	Justification string `json:"justification,omitempty" yaml:"justification,omitempty"`
}

// IPCPermission restricts inter-process communication.
type IPCPermission struct {
	Kind         string `json:"kind" yaml:"kind"`
	Target       string `json:"target" yaml:"target"`
	Description  string `json:"description,omitempty" yaml:"description,omitempty"`
	HITLRequired bool   `json:"hitl_required,omitempty" yaml:"hitl_required,omitempty"`
}

// PermissionSet aggregates the permissions declared by an agent manifest.
type PermissionSet struct {
	FileSystem   []FileSystemPermission `json:"filesystem,omitempty" yaml:"filesystem,omitempty"`
	Executables  []ExecutablePermission `json:"executables,omitempty" yaml:"executables,omitempty"`
	Network      []NetworkPermission    `json:"network,omitempty" yaml:"network,omitempty"`
	Capabilities []CapabilityPermission `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	IPC          []IPCPermission        `json:"ipc,omitempty" yaml:"ipc,omitempty"`
	HITLRequired []string               `json:"hitl_required,omitempty" yaml:"hitl_required,omitempty"`
}

// PermissionDescriptor identifies a specific permission for policy decisions.
type PermissionDescriptor struct {
	Type         PermissionType    `json:"type"`
	Action       string            `json:"action"`
	Resource     string            `json:"resource"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	RequiresHITL bool              `json:"requires_hitl,omitempty"`
}

// PermissionDeniedError records a denied permission with context.
type PermissionDeniedError struct {
	Descriptor PermissionDescriptor `json:"descriptor"`
	Message    string               `json:"message"`
}

func (e *PermissionDeniedError) Error() string { return e.Message }

// FilePermissionChecker checks filesystem access.
type FilePermissionChecker interface {
	CheckFilePermission(ctx context.Context, agentID, basePath, action, absPath string, matrix any) error
}

// NetworkPermissionChecker checks network access.
type NetworkPermissionChecker interface {
	CheckNetwork(ctx context.Context, agentID, direction, protocol, host string, port int) error
}

// CapabilityChecker checks capability access.
type CapabilityChecker interface {
	CheckCapability(ctx context.Context, agentID, capability string) error
}

// AgentReviewApprovalRules configures approval rules for agent review.
type AgentReviewApprovalRules struct {
	RequireVerificationEvidence bool `yaml:"require_verification_evidence,omitempty" json:"require_verification_evidence,omitempty"`
	RejectOnUnresolvedErrors    bool `yaml:"reject_on_unresolved_errors,omitempty" json:"reject_on_unresolved_errors,omitempty"`
}
