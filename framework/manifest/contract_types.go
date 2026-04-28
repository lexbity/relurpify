package manifest

import (
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// EffectiveAgentContract captures the resolved runtime-facing contract derived
// from the manifest, skill set, and any later overlays.
type EffectiveAgentContract struct {
	AgentID        string
	Manifest       *AgentManifest
	AgentSpec      *agentspec.AgentRuntimeSpec
	Permissions    core.PermissionSet
	Resources      ResourceSpec
	ResolvedSkills []ResolvedSkill
	SkillResults   []SkillResolution
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

// ResolveOptions provides optional inputs layered on top of the raw
type ResolveOptions struct {
	GlobalConfig  *GlobalConfig
	AgentOverlays []agentspec.AgentSpecOverlay
}
