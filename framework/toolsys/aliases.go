package toolsys

import (
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/runtime"
	"github.com/lexcodex/relurpify/framework/search"
)

type Context = core.Context
type Tool = core.Tool
type ToolParameter = core.ToolParameter
type ToolPermissions = core.ToolPermissions
type ToolResult = core.ToolResult
type Telemetry = core.Telemetry
type AgentRuntimeSpec = core.AgentRuntimeSpec
type ToolPolicy = core.ToolPolicy
type PermissionManager = runtime.PermissionManager
type AgentToolMatrix = core.AgentToolMatrix
type PermissionSet = core.PermissionSet
type AgentPermissionLevel = core.AgentPermissionLevel
type ToolCall = core.ToolCall
type PermissionDescriptor = core.PermissionDescriptor
type PermissionDeniedError = core.PermissionDeniedError
type Event = core.Event

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
	EventToolCall   = core.EventToolCall
	EventToolResult = core.EventToolResult
)

const (
	GrantScopeOneTime = runtime.GrantScopeOneTime
	RiskLevelMedium   = runtime.RiskLevelMedium
)

var MatchGlob = search.MatchGlob
