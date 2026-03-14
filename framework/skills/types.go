package skills

import (
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
)

// SkillPaths resolves standard paths for a skill package.
type SkillPaths struct {
	Root      string
	Scripts   []string
	Resources []string
	Templates []string
}

// SkillResolution captures skill loading outcomes.
type SkillResolution struct {
	Name    string
	Applied bool
	Error   string
	Paths   SkillPaths
}

// ResolvedSkill carries the validated skill manifest and resolved paths for
// later registration steps after pure config resolution has completed.
type ResolvedSkill struct {
	Manifest *manifest.SkillManifest
	Paths    SkillPaths
}

// SkillCapabilityCandidate represents a prompt/resource capability contributed
// by a resolved skill package.
type SkillCapabilityCandidate struct {
	Descriptor      core.CapabilityDescriptor
	PromptHandler   core.PromptCapabilityHandler
	ResourceHandler core.ResourceCapabilityHandler
}
