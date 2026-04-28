package capability

import (
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// Types from platform/contracts (tool infrastructure)
type Tool = contracts.Tool
type ToolParameter = contracts.ToolParameter
type ToolPermissions = contracts.ToolPermissions
type ToolResult = contracts.ToolResult
type CapabilityExecutionResult = contracts.CapabilityExecutionResult
type PermissionSet = contracts.PermissionSet
type PermissionDescriptor = contracts.PermissionDescriptor
type PermissionDeniedError = contracts.PermissionDeniedError
type FileSystemAction = contracts.FileSystemAction

// Types from framework/core (capability system)
type CapabilityKind = core.CapabilityKind
type CapabilityDescriptor = core.CapabilityDescriptor
type CapabilityRuntimeFamily = core.CapabilityRuntimeFamily
type CapabilitySource = core.CapabilitySource
type TrustClass = core.TrustClass
type RiskClass = core.RiskClass
type EffectClass = core.EffectClass
type Telemetry = core.Telemetry
type ToolCall = core.ToolCall
type Event = core.Event
type ProviderDescriptor = core.ProviderDescriptor
type ProviderSession = core.ProviderSession
type ProviderHealthSnapshot = core.ProviderHealthSnapshot

// Types from framework/agentspec
type AgentRuntimeSpec = agentspec.AgentRuntimeSpec
type ToolPolicy = agentspec.ToolPolicy
type AgentPermissionLevel = agentspec.AgentPermissionLevel

// Types from framework/authorization
type PermissionManager = authorization.PermissionManager

// Constants from platform/contracts
type Tag = string

const (
	TagReadOnly    Tag = contracts.TagReadOnly
	TagExecute     Tag = contracts.TagExecute
	TagDestructive Tag = contracts.TagDestructive
	TagNetwork     Tag = contracts.TagNetwork
)

const (
	FileSystemRead    FileSystemAction = contracts.FileSystemRead
	FileSystemWrite   FileSystemAction = contracts.FileSystemWrite
	FileSystemExecute FileSystemAction = contracts.FileSystemExecute
	FileSystemList    FileSystemAction = contracts.FileSystemList
)

// Constants from framework/core
const (
	CapabilityKindTool = core.CapabilityKindTool
)

const (
	CapabilityRuntimeFamilyLocalTool = core.CapabilityRuntimeFamilyLocalTool
	CapabilityRuntimeFamilyProvider  = core.CapabilityRuntimeFamilyProvider
	CapabilityRuntimeFamilyRelurpic  = core.CapabilityRuntimeFamilyRelurpic
)

const (
	TrustClassBuiltinTrusted         = core.TrustClassBuiltinTrusted
	TrustClassWorkspaceTrusted       = core.TrustClassWorkspaceTrusted
	TrustClassProviderLocalUntrusted = core.TrustClassProviderLocalUntrusted
	TrustClassRemoteDeclared         = core.TrustClassRemoteDeclared
	TrustClassRemoteApproved         = core.TrustClassRemoteApproved
)

const (
	RiskClassReadOnly     = core.RiskClassReadOnly
	RiskClassDestructive  = core.RiskClassDestructive
	RiskClassExecute      = core.RiskClassExecute
	RiskClassNetwork      = core.RiskClassNetwork
	RiskClassCredentialed = core.RiskClassCredentialed
	RiskClassExfiltration = core.RiskClassExfiltration
	RiskClassSessioned    = core.RiskClassSessioned
)

const (
	EffectClassFilesystemMutation = core.EffectClassFilesystemMutation
	EffectClassProcessSpawn       = core.EffectClassProcessSpawn
	EffectClassNetworkEgress      = core.EffectClassNetworkEgress
	EffectClassCredentialUse      = core.EffectClassCredentialUse
	EffectClassExternalState      = core.EffectClassExternalState
	EffectClassSessionCreation    = core.EffectClassSessionCreation
	EffectClassContextInsertion   = core.EffectClassContextInsertion
)

const (
	PermissionTypeHITL = contracts.PermissionTypeHITL
)

const (
	EventCapabilityCall   = core.EventCapabilityCall
	EventCapabilityResult = core.EventCapabilityResult
	EventToolCall         = core.EventToolCall
	EventToolResult       = core.EventToolResult
)

const (
	GrantScopeOneTime = authorization.GrantScopeOneTime
	RiskLevelMedium   = authorization.RiskLevelMedium
)
