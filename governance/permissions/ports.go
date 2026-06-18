package permissions

import (
	"context"

	ucperms "codeburg.org/lexbit/relurpify/userconfig/permissions"
)

// The declarative permission vocabulary is owned by userconfig/permissions (the
// config domain decodes it). Governance owns enforcement behavior and
// re-exports the vocabulary so existing consumers reference it unchanged.
type (
	PermissionType           = ucperms.PermissionType
	AgentPermissionLevel     = ucperms.AgentPermissionLevel
	FileSystemAction         = ucperms.FileSystemAction
	FileSystemPermission     = ucperms.FileSystemPermission
	ExecutablePermission     = ucperms.ExecutablePermission
	NetworkPermission        = ucperms.NetworkPermission
	CapabilityPermission     = ucperms.CapabilityPermission
	IPCPermission            = ucperms.IPCPermission
	PermissionSet            = ucperms.PermissionSet
	PermissionDescriptor     = ucperms.PermissionDescriptor
	AgentReviewApprovalRules = ucperms.AgentReviewApprovalRules
)

const (
	PermissionTypeFilesystem = ucperms.PermissionTypeFilesystem
	PermissionTypeExecutable = ucperms.PermissionTypeExecutable
	PermissionTypeNetwork    = ucperms.PermissionTypeNetwork
	PermissionTypeCapability = ucperms.PermissionTypeCapability
	PermissionTypeIPC        = ucperms.PermissionTypeIPC
	PermissionTypeHITL       = ucperms.PermissionTypeHITL

	AgentPermissionAllow = ucperms.AgentPermissionAllow
	AgentPermissionDeny  = ucperms.AgentPermissionDeny
	AgentPermissionAsk   = ucperms.AgentPermissionAsk

	FileSystemRead    = ucperms.FileSystemRead
	FileSystemWrite   = ucperms.FileSystemWrite
	FileSystemDelete  = ucperms.FileSystemDelete
	FileSystemRename  = ucperms.FileSystemRename
	FileSystemMove    = ucperms.FileSystemMove
	FileSystemExecute = ucperms.FileSystemExecute
	FileSystemList    = ucperms.FileSystemList
)

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
