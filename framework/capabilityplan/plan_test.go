package capabilityplan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/agents"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/stretchr/testify/require"
)

func TestAdmitSkillCapabilitiesRecordsRejectedCandidates(t *testing.T) {
	workspace := t.TempDir()
	skillRoot := filepath.Join(workspace, "relurpify_cfg", "skills", "reviewer")
	for _, dir := range []string{"scripts", "resources", "templates"} {
		require.NoError(t, os.MkdirAll(filepath.Join(skillRoot, dir), 0o755))
	}
	skill := &manifest.SkillManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "SkillManifest",
		Metadata:   manifest.ManifestMetadata{Name: "reviewer", Version: "1.0.0"},
		Spec: manifest.SkillSpec{
			PromptSnippets: []string{"Review carefully."},
		},
		SourcePath: filepath.Join(skillRoot, "skill.manifest.yaml"),
	}
	resolved := []agents.ResolvedSkill{{
		Manifest: skill,
		Paths: agents.SkillPaths{
			Root:      skillRoot,
			Scripts:   []string{filepath.Join(skillRoot, "scripts")},
			Resources: []string{filepath.Join(skillRoot, "resources")},
			Templates: []string{filepath.Join(skillRoot, "templates")},
		},
	}}
	registry := capability.NewRegistry()

	results, err := AdmitSkillCapabilities(registry, resolved, []core.CapabilitySelector{{
		Name: "reviewer.prompt.1",
		Kind: core.CapabilityKindPrompt,
	}})
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.True(t, registry.HasCapability("reviewer.prompt.1"))

	var admitted, rejected bool
	for _, result := range results {
		switch result.CapabilityID {
		case "prompt:reviewer:1":
			admitted = result.Admitted
		case "resource:reviewer:resources":
			rejected = !result.Admitted && result.Reason == "filtered by allowed capabilities"
		}
	}
	require.True(t, admitted)
	require.True(t, rejected)
}

func TestEvaluateSkillCapabilitiesDoesNotRequireRegistry(t *testing.T) {
	resolved := []agents.ResolvedSkill{{
		Manifest: &manifest.SkillManifest{
			Metadata: manifest.ManifestMetadata{Name: "reviewer"},
			Spec: manifest.SkillSpec{
				PromptSnippets: []string{"Review."},
			},
		},
	}}

	results := EvaluateSkillCapabilities(resolved, []core.CapabilitySelector{{
		Name: "reviewer.prompt.1",
		Kind: core.CapabilityKindPrompt,
	}})

	require.Len(t, results, 1)
	require.True(t, results[0].Admitted)
	require.Equal(t, "admitted", results[0].Reason)
}
