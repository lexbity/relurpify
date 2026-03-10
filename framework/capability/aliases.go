package capability

import (
	"github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/core"
)

type Context = core.Context
type CapabilityKind = core.CapabilityKind
type CapabilityDescriptor = core.CapabilityDescriptor
type CapabilityRuntimeFamily = core.CapabilityRuntimeFamily
type CapabilitySource = core.CapabilitySource
type TrustClass = core.TrustClass
type RiskClass = core.RiskClass
type EffectClass = core.EffectClass
type Tool = core.Tool
type ToolParameter = core.ToolParameter
type ToolPermissions = core.ToolPermissions
type ToolResult = core.ToolResult
type CapabilityExecutionResult = core.CapabilityExecutionResult
type Telemetry = core.Telemetry
type AgentRuntimeSpec = core.AgentRuntimeSpec
type ToolPolicy = core.ToolPolicy
type PermissionManager = authorization.PermissionManager
type PermissionSet = core.PermissionSet
type AgentPermissionLevel = core.AgentPermissionLevel
type ToolCall = core.ToolCall
type PermissionDescriptor = core.PermissionDescriptor
type PermissionDeniedError = core.PermissionDeniedError
type Event = core.Event
type ProviderDescriptor = core.ProviderDescriptor
type ProviderSession = core.ProviderSession
type ProviderHealthSnapshot = core.ProviderHealthSnapshot

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
	FileSystemRead    = core.FileSystemRead
	FileSystemWrite   = core.FileSystemWrite
	FileSystemExecute = core.FileSystemExecute
	FileSystemList    = core.FileSystemList
)

const (
	AgentPermissionAllow = core.AgentPermissionAllow
	AgentPermissionDeny  = core.AgentPermissionDeny
	AgentPermissionAsk   = core.AgentPermissionAsk
)

const (
	PermissionTypeHITL = core.PermissionTypeHITL
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
