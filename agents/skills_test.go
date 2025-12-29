package agents

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/toolsys"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSkillManifestValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skill.manifest.yaml")
	err := os.WriteFile(path, []byte("kind: SkillManifest\n"), 0o644)
	require.NoError(t, err)

	_, err = manifest.LoadSkillManifest(path)
	require.Error(t, err)
}

func TestApplySkillsMergesPromptSnippets(t *testing.T) {
	ws := t.TempDir()
	skillName := "prompt-skill"
	skillPath := SkillManifestPath(ws, skillName)
	require.NoError(t, os.MkdirAll(filepath.Dir(skillPath), 0o755))
	require.NoError(t, createSkillDirs(SkillRoot(ws, skillName)))

	overlayPrompt := "overlay prompt"
	skill := manifest.SkillManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "SkillManifest",
		Metadata:   manifest.ManifestMetadata{Name: skillName, Version: "1.0.0"},
		Spec: manifest.SkillSpec{
			PromptSnippets: []string{"snippet one"},
			AgentOverlay:   &core.AgentSpecOverlay{Prompt: &overlayPrompt},
		},
	}
	data, err := yaml.Marshal(skill)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(skillPath, data, 0o644))

	registry := toolsys.NewToolRegistry()
	base := &core.AgentRuntimeSpec{Prompt: "base prompt"}
	updated, results := ApplySkills(ws, base, []string{skillName}, nil, registry, nil, "agent-1")
	require.Len(t, results, 1)
	require.True(t, results[0].Applied)
	require.Contains(t, updated.Prompt, "overlay prompt")
	require.Contains(t, updated.Prompt, "snippet one")
	require.NotContains(t, updated.Prompt, "base prompt")
}

func TestApplySkillsMissingTool(t *testing.T) {
	ws := t.TempDir()
	skillName := "missing-tool"
	skillPath := SkillManifestPath(ws, skillName)
	require.NoError(t, os.MkdirAll(filepath.Dir(skillPath), 0o755))
	require.NoError(t, createSkillDirs(SkillRoot(ws, skillName)))

	skill := manifest.SkillManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "SkillManifest",
		Metadata:   manifest.ManifestMetadata{Name: skillName, Version: "1.0.0"},
		Spec: manifest.SkillSpec{
			RequiredTools: []string{"missing_tool"},
		},
	}
	data, err := yaml.Marshal(skill)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(skillPath, data, 0o644))

	registry := toolsys.NewToolRegistry()
	_, results := ApplySkills(ws, &core.AgentRuntimeSpec{}, []string{skillName}, nil, registry, nil, "agent-1")
	require.Len(t, results, 1)
	require.False(t, results[0].Applied)
	require.Contains(t, results[0].Error, "missing_tool")
}

func createSkillDirs(root string) error {
	for _, name := range []string{"scripts", "resources", "templates"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func TestApplySkillsMissingResources(t *testing.T) {
	ws := t.TempDir()
	skillName := "missing-resources"
	skillPath := SkillManifestPath(ws, skillName)
	require.NoError(t, os.MkdirAll(filepath.Dir(skillPath), 0o755))

	skill := manifest.SkillManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "SkillManifest",
		Metadata:   manifest.ManifestMetadata{Name: skillName, Version: "1.0.0"},
		Spec: manifest.SkillSpec{
			ResourcePaths: manifest.SkillResourceSpec{
				Scripts: []string{"scripts"},
			},
		},
	}
	data, err := yaml.Marshal(skill)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(skillPath, data, 0o644))

	registry := toolsys.NewToolRegistry()
	_, results := ApplySkills(ws, &core.AgentRuntimeSpec{}, []string{skillName}, nil, registry, nil, "agent-1")
	require.Len(t, results, 1)
	require.False(t, results[0].Applied)
	require.Contains(t, results[0].Error, "missing skill resources")
}
