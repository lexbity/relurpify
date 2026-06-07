package toolcapabilities

import (
	"codeburg.org/lexbit/relurpify/capability/ports"
)

// Type aliases that live canonically in ports.
type (
	ToolBackend                 = ports.ToolBackend
	ToolRateLimit               = ports.ToolRateLimit
	ToolManifest                = ports.ToolManifest
	ToolManifestExecution       = ports.ToolManifestExecution
	ToolManifestCommand         = ports.ToolManifestCommand
	ToolManifestFlag            = ports.ToolManifestFlag
	ToolManifestSandbox         = ports.ToolManifestSandbox
	ToolManifestComposition     = ports.ToolManifestComposition
	ToolManifestCompositionStep = ports.ToolManifestCompositionStep
	ToolManifestReturns         = ports.ToolManifestReturns
	ToolManifestReturnsChunking = ports.ToolManifestReturnsChunking
	ToolManifestCapability      = ports.ToolManifestCapability
)

const (
	ToolBackendSubprocess = ports.ToolBackendSubprocess
	ToolBackendGoNative   = ports.ToolBackendGoNative
	ToolBackendComposite  = ports.ToolBackendComposite
)

type (
	ToolManifestGuidance  = ports.ToolManifestGuidance
	ToolManifestTelemetry = ports.ToolManifestTelemetry
)

const (
	FlagStyleEquals      = ports.FlagStyleEquals
	FlagStyleSeparate    = ports.FlagStyleSeparate
	ChunkingModeWhole    = ports.ChunkingModeWhole
	ChunkingModePerItem  = ports.ChunkingModePerItem
	ChunkingModePerField = ports.ChunkingModePerField
)

// NormalizeToolName canonicalizes tool identifiers for lookups.
func NormalizeToolName(name string) string {
	return ports.NormalizeToolName(name)
}
