package runtime

import (
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/manifest"
)

type PermissionType = core.PermissionType
type FileSystemAction = core.FileSystemAction
type FileSystemPermission = core.FileSystemPermission
type ExecutablePermission = core.ExecutablePermission
type NetworkPermission = core.NetworkPermission
type CapabilityPermission = core.CapabilityPermission
type IPCPermission = core.IPCPermission
type PermissionSet = core.PermissionSet
type PermissionDescriptor = core.PermissionDescriptor
type PermissionDeniedError = core.PermissionDeniedError
type ToolPermissions = core.ToolPermissions
type AgentManifest = manifest.AgentManifest
type Agent = graph.Agent
type Context = core.Context
type AgentPermissionLevel = core.AgentPermissionLevel
type Result = core.Result
type Task = core.Task
type Config = core.Config
type AuditLogger = core.AuditLogger
type AuditQuery = core.AuditQuery
type AuditRecord = core.AuditRecord
type Tool = core.Tool

const (
	PermissionTypeFilesystem = core.PermissionTypeFilesystem
	PermissionTypeExecutable = core.PermissionTypeExecutable
	PermissionTypeNetwork    = core.PermissionTypeNetwork
	PermissionTypeCapability = core.PermissionTypeCapability
	PermissionTypeIPC        = core.PermissionTypeIPC
	PermissionTypeHITL       = core.PermissionTypeHITL
)

const (
	FileSystemRead    = core.FileSystemRead
	FileSystemWrite   = core.FileSystemWrite
	FileSystemExecute = core.FileSystemExecute
	FileSystemList    = core.FileSystemList
)

const (
	AgentPermissionAllow = core.AgentPermissionAllow
	AgentPermissionAsk   = core.AgentPermissionAsk
	AgentPermissionDeny  = core.AgentPermissionDeny
)

var LoadAgentManifest = manifest.LoadAgentManifest
var SaveAgentManifest = manifest.SaveAgentManifest
var NewInMemoryAuditLogger = core.NewInMemoryAuditLogger
