package agents

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
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

	skill := manifest.SkillManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "SkillManifest",
		Metadata:   manifest.ManifestMetadata{Name: skillName, Version: "1.0.0"},
		Spec: manifest.SkillSpec{
			PromptSnippets: []string{"snippet one"},
		},
	}
	data, err := yaml.Marshal(skill)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(skillPath, data, 0o644))

	registry := capability.NewRegistry()
	base := &core.AgentRuntimeSpec{Prompt: "base prompt"}
	updated, results := ApplySkills(ws, base, []string{skillName}, registry, nil, "agent-1")
	require.Len(t, results, 1)
	require.True(t, results[0].Applied)
	require.Contains(t, updated.Prompt, "base prompt")
	require.Contains(t, updated.Prompt, "snippet one")
}

func TestApplySkillsMissingTool(t *testing.T) {
	ws := t.TempDir()
	skillName := "missing-bin"
	skillPath := SkillManifestPath(ws, skillName)
	require.NoError(t, os.MkdirAll(filepath.Dir(skillPath), 0o755))
	require.NoError(t, createSkillDirs(SkillRoot(ws, skillName)))

	skill := manifest.SkillManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "SkillManifest",
		Metadata:   manifest.ManifestMetadata{Name: skillName, Version: "1.0.0"},
		Spec: manifest.SkillSpec{
			Requires: manifest.SkillRequiresSpec{
				Bins: []string{"__nonexistent_binary_xyzzy__"},
			},
		},
	}
	data, err := yaml.Marshal(skill)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(skillPath, data, 0o644))

	registry := capability.NewRegistry()
	_, results := ApplySkills(ws, &core.AgentRuntimeSpec{}, []string{skillName}, registry, nil, "agent-1")
	require.Len(t, results, 1)
	require.False(t, results[0].Applied)
	require.Contains(t, results[0].Error, "__nonexistent_binary_xyzzy__")
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

	registry := capability.NewRegistry()
	_, results := ApplySkills(ws, &core.AgentRuntimeSpec{}, []string{skillName}, registry, nil, "agent-1")
	require.Len(t, results, 1)
	require.False(t, results[0].Applied)
	require.Contains(t, results[0].Error, "missing skill resources")
}

// TestApplySkillsFlat verifies two flat skills are both applied without inheritance.
func TestApplySkillsFlat(t *testing.T) {
	ws := t.TempDir()

	for _, name := range []string{"skill-a", "skill-b"} {
		skillPath := SkillManifestPath(ws, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(skillPath), 0o755))
		require.NoError(t, createSkillDirs(SkillRoot(ws, name)))
		skill := manifest.SkillManifest{
			APIVersion: "relurpify/v1alpha1",
			Kind:       "SkillManifest",
			Metadata:   manifest.ManifestMetadata{Name: name, Version: "1.0.0"},
			Spec: manifest.SkillSpec{
				AllowedCapabilities: []core.CapabilitySelector{{Name: "tool_" + name, Kind: core.CapabilityKindTool}},
			},
		}
		data, err := yaml.Marshal(skill)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(skillPath, data, 0o644))
	}

	registry := capability.NewRegistry()
	spec, results := ApplySkills(ws, &core.AgentRuntimeSpec{}, []string{"skill-a", "skill-b"}, registry, nil, "agent-1")
	require.Len(t, results, 2)
	require.True(t, results[0].Applied)
	require.True(t, results[1].Applied)
	require.Contains(t, spec.AllowedCapabilities, core.CapabilitySelector{Name: "tool_skill-a", Kind: core.CapabilityKindTool})
	require.Contains(t, spec.AllowedCapabilities, core.CapabilitySelector{Name: "tool_skill-b", Kind: core.CapabilityKindTool})
}

// TestApplySkillsMissingBinarySkipped verifies that a skill with a missing binary is skipped
// while other skills continue to be applied.
func TestApplySkillsMissingBinarySkipped(t *testing.T) {
	ws := t.TempDir()

	badSkill := "bad-skill"
	badPath := SkillManifestPath(ws, badSkill)
	require.NoError(t, os.MkdirAll(filepath.Dir(badPath), 0o755))
	require.NoError(t, createSkillDirs(SkillRoot(ws, badSkill)))
	bad := manifest.SkillManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "SkillManifest",
		Metadata:   manifest.ManifestMetadata{Name: badSkill, Version: "1.0.0"},
		Spec:       manifest.SkillSpec{Requires: manifest.SkillRequiresSpec{Bins: []string{"__nonexistent__"}}},
	}
	data, _ := yaml.Marshal(bad)
	require.NoError(t, os.WriteFile(badPath, data, 0o644))

	goodSkill := "good-skill"
	goodPath := SkillManifestPath(ws, goodSkill)
	require.NoError(t, os.MkdirAll(filepath.Dir(goodPath), 0o755))
	require.NoError(t, createSkillDirs(SkillRoot(ws, goodSkill)))
	good := manifest.SkillManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "SkillManifest",
		Metadata:   manifest.ManifestMetadata{Name: goodSkill, Version: "1.0.0"},
		Spec:       manifest.SkillSpec{AllowedCapabilities: []core.CapabilitySelector{{Name: "tool_good", Kind: core.CapabilityKindTool}}},
	}
	data, _ = yaml.Marshal(good)
	require.NoError(t, os.WriteFile(goodPath, data, 0o644))

	registry := capability.NewRegistry()
	spec, results := ApplySkills(ws, &core.AgentRuntimeSpec{}, []string{badSkill, goodSkill}, registry, nil, "agent-1")
	require.Len(t, results, 2)
	require.False(t, results[0].Applied)
	require.True(t, results[1].Applied)
	require.Contains(t, spec.AllowedCapabilities, core.CapabilitySelector{Name: "tool_good", Kind: core.CapabilityKindTool})
}

// TestApplySkillsToolExecutionPolicy verifies that skill tool_execution_policy is merged.
func TestApplySkillsToolExecutionPolicy(t *testing.T) {
	ws := t.TempDir()
	skillName := "policy-skill"
	skillPath := SkillManifestPath(ws, skillName)
	require.NoError(t, os.MkdirAll(filepath.Dir(skillPath), 0o755))
	require.NoError(t, createSkillDirs(SkillRoot(ws, skillName)))

	skill := manifest.SkillManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "SkillManifest",
		Metadata:   manifest.ManifestMetadata{Name: skillName, Version: "1.0.0"},
		Spec: manifest.SkillSpec{
			ToolExecutionPolicy: map[string]core.ToolPolicy{
				"git_commit": {Execute: core.AgentPermissionAsk},
			},
		},
	}
	data, err := yaml.Marshal(skill)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(skillPath, data, 0o644))

	registry := capability.NewRegistry()
	spec, results := ApplySkills(ws, &core.AgentRuntimeSpec{}, []string{skillName}, registry, nil, "agent-1")
	require.Len(t, results, 1)
	require.True(t, results[0].Applied)
	require.NotNil(t, spec.ToolExecutionPolicy)
	require.Equal(t, core.AgentPermissionAsk, spec.ToolExecutionPolicy["git_commit"].Execute)
}

func TestApplySkillsMergesCapabilityAndProviderPolicies(t *testing.T) {
	ws := t.TempDir()
	skillName := "capability-policy-skill"
	skillPath := SkillManifestPath(ws, skillName)
	require.NoError(t, os.MkdirAll(filepath.Dir(skillPath), 0o755))
	require.NoError(t, createSkillDirs(SkillRoot(ws, skillName)))

	skill := manifest.SkillManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "SkillManifest",
		Metadata:   manifest.ManifestMetadata{Name: skillName, Version: "1.0.0"},
		Spec: manifest.SkillSpec{
			CapabilityPolicies: []core.CapabilityPolicy{
				{
					Selector: core.CapabilitySelector{
						Kind:        core.CapabilityKindTool,
						RiskClasses: []core.RiskClass{core.RiskClassExecute},
					},
					Execute: core.AgentPermissionAsk,
				},
			},
			GlobalPolicies: map[string]core.AgentPermissionLevel{
				string(core.TrustClassRemoteDeclared): core.AgentPermissionDeny,
			},
			ProviderPolicies: map[string]core.ProviderPolicy{
				"remote-mcp": {
					Activate:     core.AgentPermissionAsk,
					DefaultTrust: core.TrustClassRemoteDeclared,
				},
			},
			Providers: []core.ProviderConfig{
				{
					ID:      "remote-mcp",
					Kind:    core.ProviderKindMCPClient,
					Enabled: true,
					Target:  "https://mcp.example.test",
				},
			},
		},
	}
	data, err := yaml.Marshal(skill)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(skillPath, data, 0o644))

	registry := capability.NewRegistry()
	spec, results := ApplySkills(ws, &core.AgentRuntimeSpec{}, []string{skillName}, registry, nil, "agent-1")
	require.Len(t, results, 1)
	require.True(t, results[0].Applied)
	require.Len(t, spec.CapabilityPolicies, 1)
	require.Equal(t, core.AgentPermissionAsk, spec.CapabilityPolicies[0].Execute)
	require.Equal(t, core.AgentPermissionDeny, spec.GlobalPolicies[string(core.TrustClassRemoteDeclared)])
	require.Equal(t, core.AgentPermissionAsk, spec.ProviderPolicies["remote-mcp"].Activate)
	require.Len(t, spec.Providers, 1)
	require.Equal(t, "https://mcp.example.test", spec.Providers[0].Target)
}

func TestApplySkillsMergesInsertionPolicies(t *testing.T) {
	ws := t.TempDir()
	skillName := "insertion-policies"
	skillPath := SkillManifestPath(ws, skillName)
	require.NoError(t, os.MkdirAll(filepath.Dir(skillPath), 0o755))
	require.NoError(t, createSkillDirs(SkillRoot(ws, skillName)))

	skill := manifest.SkillManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "SkillManifest",
		Metadata:   manifest.ManifestMetadata{Name: skillName, Version: "1.0.0"},
		Spec: manifest.SkillSpec{
			InsertionPolicies: []core.CapabilityInsertionPolicy{
				{
					Selector: core.CapabilitySelector{
						TrustClasses: []core.TrustClass{core.TrustClassRemoteDeclared},
					},
					Action: core.InsertionActionMetadataOnly,
				},
			},
		},
	}
	data, err := yaml.Marshal(skill)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(skillPath, data, 0o644))

	registry := capability.NewRegistry()
	spec, results := ApplySkills(ws, &core.AgentRuntimeSpec{}, []string{skillName}, registry, nil, "agent-1")
	require.Len(t, results, 1)
	require.True(t, results[0].Applied)
	require.Len(t, spec.InsertionPolicies, 1)
	require.Equal(t, core.InsertionActionMetadataOnly, spec.InsertionPolicies[0].Action)
	require.Equal(t, core.TrustClassRemoteDeclared, spec.InsertionPolicies[0].Selector.TrustClasses[0])
}

func TestApplySkillsRegistersPromptAndResourceCapabilities(t *testing.T) {
	ws := t.TempDir()
	skillName := "catalog"
	skillPath := SkillManifestPath(ws, skillName)
	require.NoError(t, os.MkdirAll(filepath.Dir(skillPath), 0o755))
	require.NoError(t, createSkillDirs(SkillRoot(ws, skillName)))
	resourceFile := filepath.Join(SkillRoot(ws, skillName), "resources", "guide.md")
	require.NoError(t, os.WriteFile(resourceFile, []byte("guide"), 0o644))

	skill := manifest.SkillManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "SkillManifest",
		Metadata:   manifest.ManifestMetadata{Name: skillName, Version: "1.0.0"},
		Spec: manifest.SkillSpec{
			PromptSnippets: []string{"Prefer the workspace-specific guide."},
			ResourcePaths: manifest.SkillResourceSpec{
				Resources: []string{"resources/guide.md"},
			},
		},
	}
	data, err := yaml.Marshal(skill)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(skillPath, data, 0o644))

	registry := capability.NewRegistry()
	_, results := ApplySkills(ws, &core.AgentRuntimeSpec{}, []string{skillName}, registry, nil, "agent-1")
	require.Len(t, results, 1)
	require.True(t, results[0].Applied)

	capabilities := registry.AllCapabilities()
	require.Len(t, capabilities, 2)
	kinds := map[core.CapabilityKind]bool{}
	for _, capability := range capabilities {
		kinds[capability.Kind] = true
	}
	require.True(t, kinds[core.CapabilityKindPrompt])
	require.True(t, kinds[core.CapabilityKindResource])
}

func TestApplySkillsMergesSkillConfig(t *testing.T) {
	ws := t.TempDir()
	skillName := "skill-config"
	skillPath := SkillManifestPath(ws, skillName)
	require.NoError(t, os.MkdirAll(filepath.Dir(skillPath), 0o755))
	require.NoError(t, createSkillDirs(SkillRoot(ws, skillName)))

	skill := manifest.SkillManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "SkillManifest",
		Metadata:   manifest.ManifestMetadata{Name: skillName, Version: "1.0.0"},
		Spec: manifest.SkillSpec{
			PhaseCapabilities: map[string][]string{
				"verify": {"cli_cargo"},
			},
			PhaseCapabilitySelectors: map[string][]core.SkillCapabilitySelector{
				"verify": {
					{Tags: []string{"lang:rust", "test"}},
				},
			},
			Verification: manifest.SkillVerificationSpec{
				SuccessTools:               []string{"cli_cargo"},
				SuccessCapabilitySelectors: []core.SkillCapabilitySelector{{Tags: []string{"lang:rust", "build"}}},
				StopOnSuccess:              true,
			},
			Recovery: manifest.SkillRecoverySpec{
				FailureProbeTools:               []string{"file_read", "search_grep"},
				FailureProbeCapabilitySelectors: []core.SkillCapabilitySelector{{Tags: []string{"recovery"}}},
			},
			Planning: manifest.SkillPlanningSpec{
				RequiredBeforeEdit:          []core.SkillCapabilitySelector{{Tags: []string{"workspace-detect"}}},
				PreferredVerifyCapabilities: []core.SkillCapabilitySelector{{Tags: []string{"test"}}},
				StepTemplates:               []core.SkillStepTemplate{{Kind: "verify", Description: "Run tests"}},
				RequireVerificationStep:     true,
			},
			Review: manifest.SkillReviewSpec{
				Criteria:  []string{"correctness"},
				FocusTags: []string{"verification"},
				ApprovalRules: core.AgentReviewApprovalRules{
					RequireVerificationEvidence: true,
				},
				SeverityWeights: map[string]float64{"high": 1},
			},
			ContextHints: manifest.SkillContextHintsSpec{
				PreferredDetailLevel: "concise",
				ProtectPatterns:      []string{"**/Cargo.toml"},
			},
		},
	}
	data, err := yaml.Marshal(skill)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(skillPath, data, 0o644))

	registry := capability.NewRegistry()
	spec, results := ApplySkills(ws, &core.AgentRuntimeSpec{}, []string{skillName}, registry, nil, "agent-1")
	require.Len(t, results, 1)
	require.True(t, results[0].Applied)
	require.Equal(t, []string{"cli_cargo"}, spec.SkillConfig.PhaseCapabilities["verify"])
	require.Len(t, spec.SkillConfig.PhaseCapabilitySelectors["verify"], 1)
	require.True(t, spec.SkillConfig.Verification.StopOnSuccess)
	require.Equal(t, []string{"cli_cargo"}, spec.SkillConfig.Verification.SuccessTools)
	require.Len(t, spec.SkillConfig.Verification.SuccessCapabilitySelectors, 1)
	require.Equal(t, []string{"file_read", "search_grep"}, spec.SkillConfig.Recovery.FailureProbeTools)
	require.Len(t, spec.SkillConfig.Recovery.FailureProbeCapabilitySelectors, 1)
	require.Len(t, spec.SkillConfig.Planning.RequiredBeforeEdit, 1)
	require.True(t, spec.SkillConfig.Planning.RequireVerificationStep)
	require.Equal(t, []string{"correctness"}, spec.SkillConfig.Review.Criteria)
	require.True(t, spec.SkillConfig.Review.ApprovalRules.RequireVerificationEvidence)
	require.Equal(t, "concise", spec.SkillConfig.ContextHints.PreferredDetailLevel)
	require.Equal(t, []string{"**/Cargo.toml"}, spec.SkillConfig.ContextHints.ProtectPatterns)
}

// stubTagTool is a minimal Tool implementation for DeriveGVisorAllowlist tests.
type stubTagTool struct {
	name  string
	perms core.ToolPermissions
}

func (t stubTagTool) Name() string                     { return t.name }
func (t stubTagTool) Description() string              { return "" }
func (t stubTagTool) Category() string                 { return "test" }
func (t stubTagTool) Parameters() []core.ToolParameter { return nil }
func (t stubTagTool) Execute(_ context.Context, _ *core.Context, _ map[string]interface{}) (*core.ToolResult, error) {
	return nil, nil
}
func (t stubTagTool) IsAvailable(_ context.Context, _ *core.Context) bool { return true }
func (t stubTagTool) Permissions() core.ToolPermissions                   { return t.perms }
func (t stubTagTool) Tags() []string                                      { return nil }

// TestDeriveGVisorAllowlist verifies that the allowlist is derived from tool permissions.
func TestDeriveGVisorAllowlist(t *testing.T) {
	registry := capability.NewRegistry()

	goBinary := core.ExecutablePermission{Binary: "go", Args: []string{"*"}}
	permSet := core.PermissionSet{Executables: []core.ExecutablePermission{goBinary}}
	perms := core.ToolPermissions{Permissions: &permSet}
	require.NoError(t, registry.Register(stubTagTool{name: "cli_go", perms: perms}))

	allowlist := DeriveGVisorAllowlist([]core.CapabilitySelector{{Name: "cli_go", Kind: core.CapabilityKindTool}}, registry)
	require.Len(t, allowlist, 1)
	require.Equal(t, "go", allowlist[0].Binary)
}
