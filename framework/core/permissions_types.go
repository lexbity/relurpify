package core

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// Re-export permission contracts from platform/contracts for backward compatibility.

// PermissionType enumerates the supported permission families.
type PermissionType = contracts.PermissionType

const (
	PermissionTypeFilesystem = contracts.PermissionTypeFilesystem
	PermissionTypeExecutable = contracts.PermissionTypeExecutable
	PermissionTypeNetwork    = contracts.PermissionTypeNetwork
	PermissionTypeCapability = contracts.PermissionTypeCapability
	PermissionTypeIPC        = contracts.PermissionTypeIPC
	PermissionTypeHITL       = contracts.PermissionTypeHITL
)

const permissionMatchAll = "**"

// FileSystemAction enumerates filesystem operations.
type FileSystemAction = contracts.FileSystemAction

const (
	FileSystemRead    = contracts.FileSystemRead
	FileSystemWrite   = contracts.FileSystemWrite
	FileSystemDelete  = contracts.FileSystemDelete
	FileSystemRename  = contracts.FileSystemRename
	FileSystemMove    = contracts.FileSystemMove
	FileSystemExecute = contracts.FileSystemExecute
	FileSystemList    = contracts.FileSystemList
)

// FileSystemPermission scopes access to a portion of the workspace.
type FileSystemPermission = contracts.FileSystemPermission

// ExecutablePermission restricts binary execution.
type ExecutablePermission = contracts.ExecutablePermission

// NetworkPermission describes network access.
type NetworkPermission = contracts.NetworkPermission

// CapabilityPermission enumerates Linux capability requirements.
type CapabilityPermission = contracts.CapabilityPermission

// IPCPermission restricts inter-process communication.
type IPCPermission = contracts.IPCPermission

// PermissionSet aggregates the permissions declared by an agent manifest.
type PermissionSet = contracts.PermissionSet

// Validate ensures the permission declaration is consistent.
func ValidatePermissionSet(p *PermissionSet) error {
	return p.Validate()
}

// PermissionDescriptor describes a single permission decision.
type PermissionDescriptor = contracts.PermissionDescriptor

// PermissionDeniedError wraps denials with structured context.
type PermissionDeniedError = contracts.PermissionDeniedError

// ToolPermissions summarises tool requirements.
type ToolPermissions = contracts.ToolPermissions

// ValidateToolPermissions ensures tool permission manifests are well-formed.
func ValidateToolPermissions(t ToolPermissions) error {
	return t.Validate()
}
