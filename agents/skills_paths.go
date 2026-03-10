package agents

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lexcodex/relurpify/framework/manifest"
)

// SkillRoot returns the skill directory for a name.
func SkillRoot(workspace, name string) string {
	return filepath.Join(ConfigDir(workspace), "skills", name)
}

// SkillManifestPath returns the default skill manifest path.
func SkillManifestPath(workspace, name string) string {
	return filepath.Join(SkillRoot(workspace, name), skillManifestName)
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

	resourcePaths := skill.Spec.ResourcePaths
	paths.Scripts = resolveSkillList(root, resourcePaths.Scripts, filepath.Join(root, "scripts"))
	paths.Resources = resolveSkillList(root, resourcePaths.Resources, filepath.Join(root, "resources"))
	paths.Templates = resolveSkillList(root, resourcePaths.Templates, filepath.Join(root, "templates"))
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
