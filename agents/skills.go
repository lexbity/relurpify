package agents

import (
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
)

const skillManifestName = "skill.manifest.yaml"

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

// ResolveSkillPaths exposes the resolved resource paths for a skill.
func ResolveSkillPaths(skill *manifest.SkillManifest) SkillPaths {
	return resolveSkillPaths(skill)
}

// ValidateSkillPaths ensures resource entries exist on disk.
func ValidateSkillPaths(paths SkillPaths) error {
	return validateSkillPaths(paths)
}

// DeriveGVisorAllowlist returns the binary allowlist for the gVisor sandbox
// by walking the effective (allowed) tool set and collecting each tool's
// declared executable permissions.
func DeriveGVisorAllowlist(allowed []core.CapabilitySelector, registry toolDescriptorRegistry) []core.ExecutablePermission {
	return deriveGVisorAllowlist(allowed, registry)
}
