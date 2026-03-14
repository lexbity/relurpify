package skills

import (
	"github.com/lexcodex/relurpify/framework/capability"
	frameworkskills "github.com/lexcodex/relurpify/framework/skills"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
)

const skillManifestName = "skill.manifest.yaml"

// Deprecated: use framework/skills.SkillPaths.
// SkillPaths resolves standard paths for a skill package.
type SkillPaths = frameworkskills.SkillPaths

// Deprecated: use framework/skills.SkillResolution.
// SkillResolution captures skill loading outcomes.
type SkillResolution = frameworkskills.SkillResolution

// Deprecated: use framework/skills.ResolvedSkill.
// ResolvedSkill carries the validated skill manifest and resolved paths for
// later registration steps after pure config resolution has completed.
type ResolvedSkill = frameworkskills.ResolvedSkill

// Deprecated: use framework/skills.SkillCapabilityCandidate.
// SkillCapabilityCandidate represents a prompt/resource capability contributed
// by a resolved skill package.
type SkillCapabilityCandidate = frameworkskills.SkillCapabilityCandidate

// Deprecated: use framework/skills.ResolveSkillPaths.
// ResolveSkillPaths exposes the resolved resource paths for a skill.
func ResolveSkillPaths(skill *manifest.SkillManifest) SkillPaths {
	return frameworkskills.ResolveSkillPaths(skill)
}

// Deprecated: use framework/skills.ValidateSkillPaths.
// ValidateSkillPaths ensures resource entries exist on disk.
func ValidateSkillPaths(paths SkillPaths) error {
	return frameworkskills.ValidateSkillPaths(paths)
}

// Deprecated: use framework/skills.EnumerateSkillCapabilities.
// EnumerateSkillCapabilities expands resolved skills into prompt/resource
// capability candidates without mutating any registry state.
func EnumerateSkillCapabilities(resolved []ResolvedSkill) []SkillCapabilityCandidate {
	return frameworkskills.EnumerateSkillCapabilities(resolved)
}

// Deprecated: use framework/skills.DeriveGVisorAllowlist.
// DeriveGVisorAllowlist returns the binary allowlist for the gVisor sandbox
// by walking the effective (allowed) tool set and collecting each tool's
// declared executable permissions.
func DeriveGVisorAllowlist(allowed []core.CapabilitySelector, registry ToolDescriptorRegistry) []core.ExecutablePermission {
	return frameworkskills.DeriveGVisorAllowlist(allowed, registry)
}

// Deprecated: use framework/skills.ApplySkills.
func ApplySkills(workspace string, baseSpec *core.AgentRuntimeSpec, skillNames []string,
	registry *capability.Registry, permissions *capability.PermissionManager, agentID string,
) (*core.AgentRuntimeSpec, []SkillResolution) {
	return frameworkskills.ApplySkills(workspace, baseSpec, skillNames, registry, permissions, agentID)
}

// Deprecated: use framework/skills.ResolveSkills.
func ResolveSkills(workspace string, baseSpec *core.AgentRuntimeSpec, skillNames []string) (*core.AgentRuntimeSpec, []ResolvedSkill, []SkillResolution) {
	return frameworkskills.ResolveSkills(workspace, baseSpec, skillNames)
}

// Deprecated: use framework/skills.SkillRoot.
func SkillRoot(workspace, name string) string {
	return frameworkskills.SkillRoot(workspace, name)
}

// Deprecated: use framework/skills.SkillManifestPath.
func SkillManifestPath(workspace, name string) string {
	return frameworkskills.SkillManifestPath(workspace, name)
}
