package contract

import (
	frameworkconfig "codeburg.org/lexbit/relurpify/framework/config"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/manifest"
	frameworkskills "codeburg.org/lexbit/relurpify/framework/skills"
)

// EffectiveAgentContract captures the resolved runtime-facing contract derived
// from the manifest, skill set, and any later overlays.
type EffectiveAgentContract struct {
	AgentID        string
	Manifest       *manifest.AgentManifest
	AgentSpec      *core.AgentRuntimeSpec
	Permissions    core.PermissionSet
	Resources      manifest.ResourceSpec
	ResolvedSkills []frameworkskills.ResolvedSkill
	SkillResults   []frameworkskills.SkillResolution
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
	GlobalConfig  *frameworkconfig.GlobalConfig
	AgentOverlays []core.AgentSpecOverlay
}
