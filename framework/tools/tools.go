// Package tools contains the local-only tool runtime surface for built-in framework tools.
package tools

import (
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/runtime"
	"github.com/lexcodex/relurpify/framework/search"
)

type Tool = core.Tool
type Parameter = core.ToolParameter
type ToolParameter = core.ToolParameter
type Permissions = core.ToolPermissions
type ToolPermissions = core.ToolPermissions
type Result = core.ToolResult
type ToolResult = core.ToolResult
type Policy = core.ToolPolicy
type ToolPolicy = core.ToolPolicy
type Call = core.ToolCall
type ToolCall = core.ToolCall
type PermissionDescriptor = core.PermissionDescriptor
type PermissionDeniedError = core.PermissionDeniedError
type PermissionSet = core.PermissionSet
type AgentPermissionLevel = core.AgentPermissionLevel
type PermissionManager = runtime.PermissionManager
type PermissionAware = capability.PermissionAware
type AgentSpecAware = capability.AgentSpecAware

const (
	AgentPermissionAllow = core.AgentPermissionAllow
	AgentPermissionDeny  = core.AgentPermissionDeny
	AgentPermissionAsk   = core.AgentPermissionAsk
)

const (
	FileSystemRead    = core.FileSystemRead
	FileSystemWrite   = core.FileSystemWrite
	FileSystemExecute = core.FileSystemExecute
	FileSystemList    = core.FileSystemList
)

var MatchGlob = search.MatchGlob
