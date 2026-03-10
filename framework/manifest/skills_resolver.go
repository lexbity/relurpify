package manifest

import (
	"fmt"
	"path/filepath"

	"github.com/lexcodex/relurpify/framework/config"
)

// LoadSkill loads a single skill by name (flat, no inheritance chain).
func LoadSkill(workspace, name string) (*SkillManifest, error) {
	if name == "" {
		return nil, fmt.Errorf("skill name required")
	}
	manifestPath := filepath.Join(config.New(workspace).SkillsDir(), name, "skill.manifest.yaml")
	return LoadSkillManifest(manifestPath)
}

// LoadSkillList loads each named skill independently. Skills that fail to load
// are skipped; the caller is responsible for logging warnings via the returned
// error list.
func LoadSkillList(workspace string, names []string) []*SkillManifest {
	var loaded []*SkillManifest
	for _, name := range names {
		if name == "" {
			continue
		}
		skill, err := LoadSkill(workspace, name)
		if err != nil {
			continue
		}
		loaded = append(loaded, skill)
	}
	return loaded
}
