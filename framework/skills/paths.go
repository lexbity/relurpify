package skills

import (
	"path/filepath"

	"codeburg.org/lexbit/relurpify/framework/cfgload"
)

const skillFileName = "skill.yaml"

// SkillRoot returns the skill directory for a name.
func SkillRoot(workspace, name string) string {
	return filepath.Join(cfgload.New(workspace).ConfigRoot(), "skills", name)
}

// SkillPath returns the default skill path.
func SkillPath(workspace, name string) string {
	return filepath.Join(SkillRoot(workspace, name), skillFileName)
}
