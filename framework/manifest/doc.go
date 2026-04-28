// Package manifest parses, validates, and serialises agent security contracts
// expressed as YAML documents under the relurpify/v1alpha1 API version.
//
// # AgentManifest
//
// AgentManifest is the primary type. It declares everything an agent is
// allowed to do before it starts:
//
//   - Filesystem paths it may read, write, or execute.
//   - Binaries it may run (go, git, bash, …).
//   - Network endpoints it may reach.
//   - Container image to run tools inside (gVisor required for production).
//   - Default policy for actions not explicitly declared (ask / allow / deny).
//   - Skill references resolved at startup.
//
// # SkillManifest
//
// SkillManifest defines a reusable skill package — a named set of
// capabilities and prompt templates that can be composed into an agent
// 
//
// # Composition
//
// merge.go overlays workspace-local manifest overrides onto the base template.
// resolve.go resolves relative paths against the workspace root. skills_resolver.go
// expands skill references into their full CapabilityDescriptor sets before
// the agent initialises.
package manifest
