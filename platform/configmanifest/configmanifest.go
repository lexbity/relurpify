package configmanifest

import "codeburg.org/lexbit/relurpify/capability/ports"

type ToolManifest = ports.ToolManifest
type ToolManifestBackend = ports.ToolBackend
type ToolManifestCommand = ports.ToolManifestCommand
type ToolManifestComposition = ports.ToolManifestComposition
type ToolManifestCompositionStep = ports.ToolManifestCompositionStep
type ToolManifestCapability = ports.ToolManifestCapability
type ToolManifestExecution = ports.ToolManifestExecution
type ToolManifestFlag = ports.ToolManifestFlag
type ToolManifestGuidance = ports.ToolManifestGuidance
type ToolManifestReturns = ports.ToolManifestReturns
type ToolManifestReturnsChunking = ports.ToolManifestReturnsChunking
type ToolManifestSandbox = ports.ToolManifestSandbox
type ToolManifestTelemetry = ports.ToolManifestTelemetry
type ToolParameter = ports.ToolParameter
type ToolPermissions = ports.ToolPermissions
type ToolResult = ports.ToolResult
type CommandRequest = ports.CommandRequest
type CommandResult = ports.CommandResult
type ToolBackend = ports.ToolBackend

const (
	ToolBackendSubprocess = ports.ToolBackendSubprocess
	ToolBackendGoNative   = ports.ToolBackendGoNative
	ToolBackendComposite  = ports.ToolBackendComposite

	FlagStyleEquals   = ports.FlagStyleEquals
	FlagStyleSeparate = ports.FlagStyleSeparate

	ChunkingModeWhole    = ports.ChunkingModeWhole
	ChunkingModePerItem  = ports.ChunkingModePerItem
	ChunkingModePerField = ports.ChunkingModePerField
)

func NormalizeToolName(name string) string {
	return ports.NormalizeToolName(name)
}
