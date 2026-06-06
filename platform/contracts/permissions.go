package contracts

import (
	"context"

	"codeburg.org/lexbit/relurpify/governance/permissions"
)

// Phase 9 bridges.
type PermissionType = permissions.PermissionType

const (
	PermissionTypeFilesystem = permissions.PermissionTypeFilesystem
	PermissionTypeExecutable = permissions.PermissionTypeExecutable
	PermissionTypeNetwork    = permissions.PermissionTypeNetwork
	PermissionTypeCapability = permissions.PermissionTypeCapability
	PermissionTypeIPC        = permissions.PermissionTypeIPC
	PermissionTypeHITL       = permissions.PermissionTypeHITL
)

type AgentPermissionLevel = permissions.AgentPermissionLevel

const (
	AgentPermissionAllow = permissions.AgentPermissionAllow
	AgentPermissionDeny  = permissions.AgentPermissionDeny
	AgentPermissionAsk   = permissions.AgentPermissionAsk
)

type FileSystemAction = permissions.FileSystemAction

const (
	FileSystemRead    = permissions.FileSystemRead
	FileSystemWrite   = permissions.FileSystemWrite
	FileSystemDelete  = permissions.FileSystemDelete
	FileSystemRename  = permissions.FileSystemRename
	FileSystemMove    = permissions.FileSystemMove
	FileSystemExecute = permissions.FileSystemExecute
	FileSystemList    = permissions.FileSystemList
)

type FileSystemPermission = permissions.FileSystemPermission
type ExecutablePermission = permissions.ExecutablePermission
type NetworkPermission = permissions.NetworkPermission
type CapabilityPermission = permissions.CapabilityPermission
type IPCPermission = permissions.IPCPermission
type PermissionSet = permissions.PermissionSet

// PermissionDescriptor identifies a specific permission for policy decisions.
type PermissionDescriptor struct {
	Type        PermissionType    `json:"type"`
	Action      string            `json:"action"`
	Resource    string            `json:"resource"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	RequiresHITL bool             `json:"requires_hitl,omitempty"`
}

// PermissionDeniedError records a denied permission with context.
type PermissionDeniedError struct {
	Descriptor PermissionDescriptor
	Message    string
}

func (e *PermissionDeniedError) Error() string { return e.Message }

// FilePermissionChecker is the interface for checking filesystem access.
type FilePermissionChecker interface {
	CheckFilePermission(ctx context.Context, agentID, basePath, action, absPath string, matrix AgentFileMatrix) error
}

// NetworkPermissionChecker is the interface for checking network access.
type NetworkPermissionChecker interface {
	CheckNetwork(ctx context.Context, agentID, direction, protocol, host string, port int) error
}

// CapabilityChecker is the interface for checking capability usage permissions.
type CapabilityChecker interface {
	CheckCapability(ctx context.Context, agentID string, capability string) error
}
