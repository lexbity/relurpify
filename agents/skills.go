package agents

import (
	"context"
	"fmt"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/toolsys"
	"os"
	"path/filepath"
	"strings"
	"time"
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

// SkillRoot returns the skill directory for a name.
func SkillRoot(workspace, name string) string {
	return filepath.Join(ConfigDir(workspace), "skills", name)
}

// SkillManifestPath returns the default skill manifest path.
func SkillManifestPath(workspace, name string) string {
	return filepath.Join(SkillRoot(workspace, name), skillManifestName)
}

// ApplySkills merges skill overlays and returns the updated spec and results.
func ApplySkills(workspace string, baseSpec *core.AgentRuntimeSpec, skillNames []string, skillOverlays map[string]core.AgentSpecOverlay, registry *toolsys.ToolRegistry, permissions *toolsys.PermissionManager, agentID string) (*core.AgentRuntimeSpec, []SkillResolution) {
	spec := core.MergeAgentSpecs(baseSpec)
	results := make([]SkillResolution, 0, len(skillNames))
	allowedTools := append([]string{}, spec.AllowedTools...)
	toolPolicies := cloneToolPolicies(spec.ToolPolicies)

	seenSkills := make(map[string]bool)
	for _, name := range skillNames {
		skillName := strings.TrimSpace(name)
		if skillName == "" {
			continue
		}
		chain, err := manifest.ResolveSkillChain(workspace, skillName)
		if err != nil {
			results = append(results, logSkillError(workspace, skillName, "load_failed", err, SkillPaths{Root: SkillRoot(workspace, skillName)}))
			continue
		}
		for _, skillManifest := range chain {
			paths := resolveSkillPaths(skillManifest)
			if err := validateSkillPaths(paths); err != nil {
				results = append(results, logSkillError(workspace, skillName, "missing_resources", err, paths))
				goto skipSkill
			}
			if ok, err := skillToolsAvailable(skillManifest, registry, permissions, agentID); !ok {
				if err == nil {
					err = fmt.Errorf("required tools unavailable")
				}
				results = append(results, logSkillError(workspace, skillName, "missing_tool_access", err, paths))
				goto skipSkill
			}
			if len(skillManifest.Spec.AllowedTools) > 0 {
				allowedTools = mergeStringList(allowedTools, skillManifest.Spec.AllowedTools)
			}
			if skillManifest.Spec.ToolPolicies != nil {
				for name, policy := range skillManifest.Spec.ToolPolicies {
					toolPolicies[name] = policy
				}
			}
			if skillManifest.Spec.AgentOverlay != nil {
				spec = core.MergeAgentSpecs(spec, *skillManifest.Spec.AgentOverlay)
			}
			if overlay, ok := skillOverlays[skillManifest.Metadata.Name]; ok {
				spec = core.MergeAgentSpecs(spec, overlay)
			}
			if len(skillManifest.Spec.PromptSnippets) > 0 {
				spec.Prompt = mergePromptSnippets(spec.Prompt, skillManifest.Spec.PromptSnippets)
			}
			if !seenSkills[skillManifest.Metadata.Name] {
				results = append(results, SkillResolution{
					Name:    skillManifest.Metadata.Name,
					Applied: true,
					Paths:   paths,
				})
				seenSkills[skillManifest.Metadata.Name] = true
			}
		}
	skipSkill:
	}

	spec.AllowedTools = allowedTools
	spec.ToolPolicies = toolPolicies
	return spec, results
}

func skillToolsAvailable(skill *manifest.SkillManifest, registry *toolsys.ToolRegistry, permissions *toolsys.PermissionManager, agentID string) (bool, error) {
	if skill == nil {
		return false, fmt.Errorf("skill manifest missing")
	}
	if registry == nil {
		return false, fmt.Errorf("tool registry not available")
	}
	for _, toolName := range skill.Spec.RequiredTools {
		toolName = strings.TrimSpace(toolName)
		if toolName == "" {
			continue
		}
		tool, ok := registry.Get(toolName)
		if !ok {
			return false, fmt.Errorf("required tool %s not available", toolName)
		}
		if permissions != nil {
			if err := permissions.AuthorizeTool(context.Background(), agentID, tool, nil); err != nil {
				return false, fmt.Errorf("tool %s blocked: %w", toolName, err)
			}
		}
	}
	return true, nil
}

// CheckSkillToolsAvailable validates required tool availability and permissions.
func CheckSkillToolsAvailable(skill *manifest.SkillManifest, registry *toolsys.ToolRegistry, permissions *toolsys.PermissionManager, agentID string) (bool, error) {
	return skillToolsAvailable(skill, registry, permissions, agentID)
}

// ResolveSkillPaths exposes the resolved resource paths for a skill.
func ResolveSkillPaths(skill *manifest.SkillManifest) SkillPaths {
	return resolveSkillPaths(skill)
}

// ValidateSkillPaths ensures resource entries exist on disk.
func ValidateSkillPaths(paths SkillPaths) error {
	return validateSkillPaths(paths)
}

func resolveSkillPaths(skill *manifest.SkillManifest) SkillPaths {
	root := ""
	if skill != nil && skill.SourcePath != "" {
		root = filepath.Dir(skill.SourcePath)
	}
	paths := SkillPaths{Root: root}
	if skill == nil {
		return paths
	}
	paths.Scripts = resolveSkillList(root, skill.Spec.ResourcePaths.Scripts, filepath.Join(root, "scripts"))
	paths.Resources = resolveSkillList(root, skill.Spec.ResourcePaths.Resources, filepath.Join(root, "resources"))
	paths.Templates = resolveSkillList(root, skill.Spec.ResourcePaths.Templates, filepath.Join(root, "templates"))
	return paths
}

func resolveSkillList(root string, entries []string, fallback string) []string {
	if len(entries) == 0 {
		if fallback == "" {
			return nil
		}
		return []string{fallback}
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if filepath.IsAbs(entry) {
			paths = append(paths, entry)
			continue
		}
		paths = append(paths, filepath.Join(root, entry))
	}
	return paths
}

func validateSkillPaths(paths SkillPaths) error {
	var missing []string
	check := func(label string, entries []string) {
		for _, entry := range entries {
			if entry == "" {
				continue
			}
			if _, err := os.Stat(entry); err != nil {
				missing = append(missing, fmt.Sprintf("%s:%s", label, entry))
			}
		}
	}
	check("scripts", paths.Scripts)
	check("resources", paths.Resources)
	check("templates", paths.Templates)
	if len(missing) > 0 {
		return fmt.Errorf("missing skill resources: %s", strings.Join(missing, ", "))
	}
	return nil
}

func mergePromptSnippets(base string, snippets []string) string {
	builder := strings.Builder{}
	base = strings.TrimSpace(base)
	if base != "" {
		builder.WriteString(base)
	}
	for _, snippet := range snippets {
		snippet = strings.TrimSpace(snippet)
		if snippet == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(snippet)
	}
	return builder.String()
}

func logSkillError(workspace, name, reason string, err error, paths SkillPaths) SkillResolution {
	entry := fmt.Sprintf("%s skill %s (%s): %s", time.Now().UTC().Format(time.RFC3339), name, reason, err.Error())
	logSkillMessage(workspace, entry)
	return SkillResolution{
		Name:    name,
		Applied: false,
		Error:   err.Error(),
		Paths:   paths,
	}
}

func cloneToolPolicies(input map[string]core.ToolPolicy) map[string]core.ToolPolicy {
	if input == nil {
		return nil
	}
	clone := make(map[string]core.ToolPolicy, len(input))
	for name, policy := range input {
		clone[name] = policy
	}
	return clone
}

func mergeStringList(base, extra []string) []string {
	if len(extra) == 0 {
		return base
	}
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, entry := range append(base, extra...) {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		out = append(out, entry)
	}
	return out
}

func logSkillMessage(workspace, message string) {
	if workspace == "" {
		return
	}
	logDir := filepath.Join(ConfigDir(workspace), "logs")
	if mkErr := os.MkdirAll(logDir, 0o755); mkErr != nil {
		return
	}
	file := filepath.Join(logDir, "skills.log")
	entry := message + "\n"
	if f, openErr := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); openErr == nil {
		defer f.Close()
		_, _ = f.WriteString(entry)
	}
}
