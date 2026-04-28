package skills

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/manifest"
	"github.com/stretchr/testify/require"
)

func TestSkillPolicyHelpersAndClones(t *testing.T) {
	toolPolicy := map[string]agentspec.ToolPolicy{"tool-a": {Execute: agentspec.AgentPermissionAsk}}
	globalPolicy := map[string]agentspec.AgentPermissionLevel{"fs": agentspec.AgentPermissionAllow}
	providerPolicy := map[string]agentspec.ProviderPolicy{"provider-a": {Activate: agentspec.AgentPermissionAllow, DefaultTrust: agentspec.TrustClassWorkspaceTrusted, AllowCredentialSharing: true}}
	providers := []agentspec.ProviderConfig{{ID: "provider-a", Config: map[string]any{"x": 1}}}
	capPolicies := []agentspec.CapabilityPolicy{{Selector: agentspec.CapabilitySelector{SourceScopes: []agentspec.CapabilityScope{agentspec.CapabilityScopeWorkspace}}}}
	insertPolicies := []agentspec.CapabilityInsertionPolicy{{Selector: agentspec.CapabilitySelector{TrustClasses: []agentspec.TrustClass{agentspec.TrustClassWorkspaceTrusted}}}}
	sessionPolicies := []agentspec.SessionPolicy{{ID: "sess-1", Selector: agentspec.SessionSelector{ActorKinds: []string{"user"}, ActorIDs: []string{"a"}}}}

	clonedTool := cloneToolPolicies(toolPolicy)
	clonedGlobal := cloneGlobalPolicies(globalPolicy)
	clonedProvider := cloneProviderPolicies(providerPolicy)
	clonedProviders := cloneProviderConfigs(providers)
	clonedCap := cloneCapabilityPolicies(capPolicies)
	clonedInsert := cloneInsertionPolicies(insertPolicies)
	clonedSession := cloneSessionPolicies(sessionPolicies)

	require.Equal(t, providers[0].Config, clonedProviders[0].Config)
	require.Len(t, clonedCap, 1)
	require.Equal(t, capPolicies[0].Selector.SourceScopes, clonedCap[0].Selector.SourceScopes)
	require.Len(t, clonedInsert, 1)
	require.Equal(t, insertPolicies[0].Selector.TrustClasses, clonedInsert[0].Selector.TrustClasses)
	require.Len(t, clonedSession, 1)
	require.Equal(t, sessionPolicies[0].Selector.ActorKinds, clonedSession[0].Selector.ActorKinds)

	toolPolicy["tool-a"] = agentspec.ToolPolicy{Execute: agentspec.AgentPermissionDeny}
	globalPolicy["fs"] = agentspec.AgentPermissionDeny
	providerPolicy["provider-a"] = agentspec.ProviderPolicy{Activate: agentspec.AgentPermissionDeny}
	providers[0].Config["x"] = 2
	require.Equal(t, agentspec.AgentPermissionAsk, clonedTool["tool-a"].Execute)
	require.Equal(t, agentspec.AgentPermissionAllow, clonedGlobal["fs"])
	require.Equal(t, agentspec.AgentPermissionAllow, clonedProvider["provider-a"].Activate)
	require.Equal(t, 1, clonedProviders[0].Config["x"])

	merged := mergeStringList([]string{"alpha", "beta"}, []string{" beta ", "", "gamma", "alpha"})
	require.Equal(t, []string{"alpha", "beta", "gamma"}, merged)

	dstTool := map[string]agentspec.ToolPolicy{}
	mergeToolExecutionPolicies(&dstTool, map[string]agentspec.ToolPolicy{"tool-b": {Execute: agentspec.AgentPermissionDeny}})
	require.Equal(t, agentspec.AgentPermissionDeny, dstTool["tool-b"].Execute)

	dstGlobal := map[string]agentspec.AgentPermissionLevel{}
	mergeGlobalPolicies(&dstGlobal, map[string]agentspec.AgentPermissionLevel{"net": agentspec.AgentPermissionAsk})
	require.Equal(t, agentspec.AgentPermissionAsk, dstGlobal["net"])

	dstProvider := map[string]agentspec.ProviderPolicy{}
	mergeProviderPolicies(&dstProvider, map[string]agentspec.ProviderPolicy{"provider-b": {Activate: agentspec.AgentPermissionAllow}})
	require.Equal(t, agentspec.AgentPermissionAllow, dstProvider["provider-b"].Activate)

	mergedProviders := mergeProviderConfigs(providers, []agentspec.ProviderConfig{{ID: "provider-b", Config: map[string]any{"y": 2}}})
	require.Len(t, mergedProviders, 2)
	require.Equal(t, "provider-b", mergedProviders[1].ID)

	mergedCap := appendCapabilityPolicies(capPolicies, []agentspec.CapabilityPolicy{{Selector: agentspec.CapabilitySelector{Name: "extra"}}})
	require.Len(t, mergedCap, 2)
	mergedInsert := appendInsertionPolicies(insertPolicies, []agentspec.CapabilityInsertionPolicy{{Selector: agentspec.CapabilitySelector{Name: "extra"}}})
	require.Len(t, mergedInsert, 2)
	mergedSession := appendSessionPolicies(sessionPolicies, []agentspec.SessionPolicy{{ID: "sess-2", Name: "two", Enabled: true, Effect: agentspec.AgentPermissionAllow, Selector: agentspec.SessionSelector{ActorKinds: []string{"user"}}}})
	require.Len(t, mergedSession, 2)
}

func TestResolveSkillsAndCapabilityHelpers(t *testing.T) {
	registry := capability.NewRegistry()
	require.NoError(t, registry.Register(resolverStubTool{name: "go_test", tags: []string{"execute", "lang:go", "test"}}))
	require.NoError(t, registry.Register(resolverStubTool{name: "go_build", tags: []string{"execute", "lang:go", "build"}}))

	policy := ResolveSkillPolicy(registry, agentspec.AgentSkillConfig{
		PhaseCapabilities: map[string][]string{"verify": {"go_test"}},
		PhaseCapabilitySelectors: map[string][]agentspec.SkillCapabilitySelector{
			"verify": {{Tags: []string{"lang:go", "build"}}},
		},
		Verification: agentspec.AgentVerificationPolicy{
			SuccessTools: []string{"go_test"},
		},
		Recovery: agentspec.AgentRecoveryPolicy{
			FailureProbeTools: []string{"go_build"},
		},
		Planning: agentspec.AgentPlanningPolicy{
			RequiredBeforeEdit:        []agentspec.SkillCapabilitySelector{{Capability: "go_test"}},
			PreferredEditCapabilities: []agentspec.SkillCapabilitySelector{{Tags: []string{"lang:go", "build"}}},
		},
		Review: agentspec.AgentReviewPolicy{
			SeverityWeights: map[string]float64{"high": 0.9},
		},
	})
	require.Equal(t, []string{"go_test", "go_build"}, policy.PhaseCapabilities["verify"])
	require.Equal(t, []string{"go_test"}, policy.VerificationSuccessCapabilities)
	require.Equal(t, []string{"go_build"}, policy.RecoveryProbeCapabilities)
	require.Equal(t, []string{"go_test"}, policy.Planning.RequiredBeforeEdit)
	require.Equal(t, []string{"go_build"}, policy.Planning.PreferredEditCapabilities)
	require.Equal(t, 0.9, policy.Review.SeverityWeights["high"])

	spec := agentspec.AgentSkillConfig{
		Verification: agentspec.AgentVerificationPolicy{SuccessTools: []string{"go_test"}},
	}
	require.Equal(t, []string{"go_test"}, resolveCapabilityNames(registry, spec.Verification.SuccessTools, nil))
	require.Equal(t, []string{"go_build"}, resolveCapabilityNames(registry, nil, []agentspec.SkillCapabilitySelector{{Capability: "go_build"}}))
	require.True(t, matchesAnyCapabilitySelector([]agentspec.CapabilitySelector{{Tags: []string{"lang:go"}}}, core.CapabilityDescriptor{Name: "go_build", Tags: []string{"lang:go"}}))
	require.False(t, matchesAnyCapabilitySelector([]agentspec.CapabilitySelector{{Tags: []string{"python"}}}, core.CapabilityDescriptor{Name: "go_build", Tags: []string{"lang:go"}}))

	allowed := []agentspec.CapabilitySelector{{Name: "go_test"}}
	require.Equal(t, []string{"go_test"}, mergeResolvedNames(nil, []string{"go_test", "", "go_test"}))
	require.True(t, reflect.DeepEqual(allowed, skillAllowedCapabilities(manifest.SkillSpec{AllowedCapabilities: allowed})))
}

func TestResolveAndApplySkillsWithTempManifest(t *testing.T) {
	workspace := t.TempDir()
	skillRoot := filepath.Join(manifest.New(workspace).SkillsDir(), "gocoder")
	require.NoError(t, os.MkdirAll(filepath.Join(skillRoot, "scripts"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(skillRoot, "resources"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(skillRoot, "templates"), 0o755))
	data, err := os.ReadFile(filepath.Join("..", "..", "relurpify_cfg", "skills", "gocoder", "skill.manifest.yaml"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(skillRoot, "skill.yaml"), data, 0o644))

	registry := capability.NewRegistry()
	base := &agentspec.AgentRuntimeSpec{
		Prompt: "base",
		SkillConfig: agentspec.AgentSkillConfig{
			Verification: agentspec.AgentVerificationPolicy{SuccessTools: []string{"base_tool"}},
		},
	}
	spec, resolved, results := ResolveSkills(workspace, base, []string{"gocoder"})
	require.NotNil(t, spec)
	require.Len(t, resolved, 1)
	require.Len(t, results, 1)
	require.True(t, results[0].Applied)
	require.Contains(t, spec.Prompt, "For Go, detect the nearest go.mod or go.work")

	updated, resolutions := ApplySkills(workspace, base, []string{"gocoder"}, registry, nil, "agent-1")
	require.NotNil(t, updated)
	require.Len(t, resolutions, 1)
	require.True(t, resolutions[0].Applied)
}

func TestSkillPathAndCapabilityRenderHelpers(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ws")
	require.Equal(t, filepath.Join(root, manifest.DirName, "skills", "demo"), SkillRoot(root, "demo"))
	require.Equal(t, filepath.Join(root, manifest.DirName, "skills", "demo", skillManifestName), SkillManifestPath(root, "demo"))
	require.Equal(t, "application/json", inferSkillResourceMIMEType("demo.json"))
	require.Equal(t, "text/plain", inferSkillResourceMIMEType("README.txt"))
	require.Equal(t, "hello", truncateSkillCapabilityDescription("hello"))
	require.True(t, strings.HasSuffix(truncateSkillCapabilityDescription("hello world hello world hello world hello world hello world hello world hello world hello world hello world"), "..."))

	skillManifest := &manifest.SkillManifest{
		Metadata: manifest.ManifestMetadata{Name: "demo", Version: "1.0.0"},
		Spec: manifest.SkillSpec{
			PromptSnippets: []string{"  hello {name}  "},
			ResourcePaths: manifest.SkillResourceSpec{
				Resources: []string{filepath.Join(root, "snippet.txt")},
			},
		},
		SourcePath: filepath.Join(root, manifest.DirName, "skills", "demo", "skill.manifest.yaml"),
	}
	paths := ResolveSkillPaths(skillManifest)
	require.Equal(t, filepath.Join(root, manifest.DirName, "skills", "demo"), paths.Root)
	require.Equal(t, []string{filepath.Join(root, manifest.DirName, "skills", "demo", "scripts")}, paths.Scripts)

	candidates := EnumerateSkillCapabilities([]ResolvedSkill{{Manifest: skillManifest, Paths: paths}})
	require.NotEmpty(t, candidates)
	require.NotEmpty(t, skillPromptCapabilities(skillManifest))
	require.NotEmpty(t, skillResourceCapabilities(skillManifest, paths))

	rendered := RenderPlanningPolicy(ResolvedSkillPolicy{
		Planning: ResolvedPlanningPolicy{
			RequiredBeforeEdit:      []string{"a"},
			StepTemplates:           []agentspec.SkillStepTemplate{{Kind: "verify", Description: "Run"}},
			RequireVerificationStep: true,
		},
	}, PlanningRenderOptions{IncludePhaseCapabilities: true, IncludeVerificationSuccess: true})
	require.Contains(t, rendered, "Required before edit: a")
	require.Contains(t, rendered, "Plans must include an explicit verification step.")
	require.NotEmpty(t, RenderReviewPolicy(ResolvedSkillPolicy{}))
	require.NotEmpty(t, RenderExecutionPolicy(&ResolvedSkillPolicy{}, true))
}
