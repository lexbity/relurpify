package contract

import (
	"github.com/lexcodex/relurpify/agents"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
)

// EffectiveAgentContract captures the resolved runtime-facing contract derived
// from the manifest, skill set, and any later overlays.
type EffectiveAgentContract struct {
	AgentID        string
	Manifest       *manifest.AgentManifest
	AgentSpec      *core.AgentRuntimeSpec
	Permissions    core.PermissionSet
	Resources      manifest.ResourceSpec
	ResolvedSkills []agents.ResolvedSkill
	SkillResults   []agents.SkillResolution
	Sources        SourceSummary
}

// SourceSummary records which inputs contributed to the effective contract so
// callers can inspect how a runtime was resolved.
type SourceSummary struct {
	ManifestName     string
	ManifestVersion  string
	Workspace        string
	RequestedSkills  []string
	AppliedSkills    []string
	FailedSkills     []string
	GlobalDefaults   bool
	OverlayCount     int
	RuntimeOverrides int
}

// ResolveOptions provides optional inputs layered on top of the raw manifest.
type ResolveOptions struct {
	GlobalConfig  *agents.GlobalConfig
	AgentOverlays []core.AgentSpecOverlay
}
