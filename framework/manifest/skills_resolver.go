package manifest

import (
	"fmt"
	"path/filepath"
	"strings"
)

const skillsDirName = "relurpify_cfg/skills"

// ResolveSkillChain returns the inheritance-ordered chain for a skill name.
func ResolveSkillChain(workspace, name string) ([]*SkillManifest, error) {
	var ordered []*SkillManifest
	visiting := make(map[string]bool)
	visited := make(map[string]bool)

	var visit func(string) error
	visit = func(skill string) error {
		skill = strings.TrimSpace(skill)
		if skill == "" {
			return nil
		}
		if visited[skill] {
			return nil
		}
		if visiting[skill] {
			return fmt.Errorf("skill inheritance cycle detected at %s", skill)
		}
		visiting[skill] = true
		manifestPath := filepath.Join(workspace, skillsDirName, skill, "skill.manifest.yaml")
		skillManifest, err := LoadSkillManifest(manifestPath)
		if err != nil {
			return err
		}
		for _, parent := range skillManifest.Spec.Inherits {
			if err := visit(parent); err != nil {
				return err
			}
		}
		visiting[skill] = false
		visited[skill] = true
		ordered = append(ordered, skillManifest)
		return nil
	}

	if err := visit(name); err != nil {
		return nil, err
	}
	return ordered, nil
}

// ResolveSkillList expands a list of skills into a de-duplicated, ordered list.
func ResolveSkillList(workspace string, names []string) ([]*SkillManifest, error) {
	var ordered []*SkillManifest
	seen := make(map[string]bool)
	for _, name := range names {
		chain, err := ResolveSkillChain(workspace, name)
		if err != nil {
			return nil, err
		}
		for _, skill := range chain {
			if skill == nil || seen[skill.Metadata.Name] {
				continue
			}
			seen[skill.Metadata.Name] = true
			ordered = append(ordered, skill)
		}
	}
	return ordered, nil
}
