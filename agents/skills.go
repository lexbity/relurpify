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
	toolOverlays := make([]toolsys.ToolPolicyOverlay, 0)

	for _, name := range skillNames {
		skillName := strings.TrimSpace(name)
		if skillName == "" {
			continue
		}
		manifestPath := SkillManifestPath(workspace, skillName)
		skillManifest, err := manifest.LoadSkillManifest(manifestPath)
		if err != nil {
			results = append(results, logSkillError(workspace, skillName, "load_failed", err, SkillPaths{Root: SkillRoot(workspace, skillName)}))
			continue
		}
		paths := resolveSkillPaths(skillManifest)
		if ok, err := skillToolsAvailable(skillManifest, registry, permissions, agentID); !ok {
			if err == nil {
				err = fmt.Errorf("required tools unavailable")
			}
			results = append(results, logSkillError(workspace, skillName, "missing_tool_access", err, paths))
			continue
		}
		if skillManifest.Spec.ToolMatrixOverride != nil || skillManifest.Spec.ToolPolicies != nil {
			toolOverlays = append(toolOverlays, toolsys.ToolPolicyOverlay{
				MatrixOverride: skillManifest.Spec.ToolMatrixOverride,
				Policies:       skillManifest.Spec.ToolPolicies,
			})
		}
		if skillManifest.Spec.AgentOverlay != nil {
			spec = core.MergeAgentSpecs(spec, *skillManifest.Spec.AgentOverlay)
		}
		if overlay, ok := skillOverlays[skillName]; ok {
			spec = core.MergeAgentSpecs(spec, overlay)
		}
		if len(skillManifest.Spec.PromptSnippets) > 0 {
			spec.Prompt = mergePromptSnippets(spec.Prompt, skillManifest.Spec.PromptSnippets)
		}
		results = append(results, SkillResolution{
			Name:    skillName,
			Applied: true,
			Paths:   paths,
		})
	}

	if len(toolOverlays) > 0 {
		matrix, policies := toolsys.MergeToolConfig(spec.Tools, spec.ToolPolicies, toolOverlays...)
		spec.Tools = matrix
		spec.ToolPolicies = policies
	}
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

func resolveSkillPaths(skill *manifest.SkillManifest) SkillPaths {
	root := ""
	if skill != nil && skill.SourcePath != "" {
		root = filepath.Dir(skill.SourcePath)
	}
	paths := SkillPaths{Root: root}
	if skill == nil {
		return paths
	}
	paths.Scripts = resolveSkillList(root, skill.Spec.Resources.Scripts, filepath.Join(root, "scripts"))
	paths.Resources = resolveSkillList(root, skill.Spec.Resources.Resources, filepath.Join(root, "resources"))
	paths.Templates = resolveSkillList(root, skill.Spec.Resources.Templates, filepath.Join(root, "templates"))
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
