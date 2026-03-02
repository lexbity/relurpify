package agents

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/toolsys"
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

// ApplySkills merges skill contributions (flat, no inheritance) into baseSpec
// and returns the updated spec plus per-skill resolution results.
func ApplySkills(workspace string, baseSpec *core.AgentRuntimeSpec, skillNames []string,
	registry *toolsys.ToolRegistry, permissions *toolsys.PermissionManager, agentID string,
) (*core.AgentRuntimeSpec, []SkillResolution) {
	spec := core.MergeAgentSpecs(baseSpec)
	results := make([]SkillResolution, 0, len(skillNames))
	allowedTools := append([]string{}, spec.AllowedTools...)
	toolPolicies := cloneToolPolicies(spec.ToolExecutionPolicy)

	for _, name := range skillNames {
		skillName := strings.TrimSpace(name)
		if skillName == "" {
			continue
		}
		skillManifest, err := manifest.LoadSkill(workspace, skillName)
		if err != nil {
			results = append(results, logSkillError(workspace, skillName, "load_failed", err,
				SkillPaths{Root: SkillRoot(workspace, skillName)}))
			continue
		}

		// Check binary prerequisites.
		if missingBin := findMissingBin(skillManifest.Spec.Requires.Bins); missingBin != "" {
			binErr := fmt.Errorf("required binary %q not found in PATH", missingBin)
			paths := resolveSkillPaths(skillManifest)
			results = append(results, logSkillError(workspace, skillName, "missing_binary", binErr, paths))
			continue
		}

		// Check resource paths.
		paths := resolveSkillPaths(skillManifest)
		if err := validateSkillPaths(paths); err != nil {
			results = append(results, logSkillError(workspace, skillName, "missing_resources", err, paths))
			continue
		}

		// Merge contributions.
		if len(skillManifest.Spec.AllowedTools) > 0 {
			allowedTools = mergeStringList(allowedTools, skillManifest.Spec.AllowedTools)
		}
		for toolName, policy := range skillManifest.Spec.ToolExecutionPolicy {
			if toolPolicies == nil {
				toolPolicies = make(map[string]core.ToolPolicy)
			}
			toolPolicies[toolName] = policy
		}
		if len(skillManifest.Spec.PromptSnippets) > 0 {
			spec.Prompt = mergePromptSnippets(spec.Prompt, skillManifest.Spec.PromptSnippets)
		}

		results = append(results, SkillResolution{
			Name:    skillManifest.Metadata.Name,
			Applied: true,
			Paths:   paths,
		})
	}

	spec.AllowedTools = allowedTools
	spec.ToolExecutionPolicy = toolPolicies
	return spec, results
}

// DeriveGVisorAllowlist returns the binary allowlist for the gVisor sandbox
// by walking the effective (allowed) tool set and collecting each tool's
// declared executable permissions.
func DeriveGVisorAllowlist(allowedToolNames []string, registry *toolsys.ToolRegistry) []core.ExecutablePermission {
	if registry == nil {
		return nil
	}
	seen := make(map[string]bool)
	var result []core.ExecutablePermission
	for _, name := range allowedToolNames {
		tool, ok := registry.Get(name)
		if !ok {
			continue
		}
		perms := tool.Permissions()
		for _, ep := range perms.Permissions.Executables {
			if seen[ep.Binary] {
				continue
			}
			seen[ep.Binary] = true
			result = append(result, ep)
		}
	}
	return result
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

func findMissingBin(bins []string) string {
	for _, bin := range bins {
		bin = strings.TrimSpace(bin)
		if bin == "" {
			continue
		}
		if _, err := exec.LookPath(bin); err != nil {
			return bin
		}
	}
	return ""
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
	entry := fmt.Sprintf("[WARNING] %s skill %s (%s): %s", time.Now().UTC().Format(time.RFC3339), name, reason, err.Error())
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
