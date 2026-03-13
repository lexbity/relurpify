package contract

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/agents"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/stretchr/testify/require"
)

func TestResolveEffectiveAgentContractMergesInExpectedOrder(t *testing.T) {
	workspace := t.TempDir()
	writeSkillFixture(t, workspace, "reviewer", `
apiVersion: relurpify/v1alpha1
kind: SkillManifest
metadata:
  name: reviewer
  version: 1.0.0
spec:
  prompt_snippets:
    - "Review carefully."
  tool_execution_policy:
    file_read:
      execute: ask
  session_policies:
    - id: reviewer-send
      name: Reviewer Send
      enabled: true
      selector:
        operations: [send]
        scopes: [per-channel-peer]
      effect: allow
`)

	m := writeAgentManifestFixture(t, workspace, `
apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: coding
  version: 1.2.3
spec:
  image: relurpify:test
  runtime: gvisor
  permissions:
    filesystem:
      - action: fs:read
        path: src/**
  resources:
    limits:
      cpu: "1"
      memory: "1Gi"
      disk_io: "1Gi"
  agent:
    implementation: react
    mode: primary
    model:
      provider: manifest-provider
      name: manifest-model
    prompt: "Base prompt."
  skills: [reviewer]
`)
	global := &agents.GlobalConfig{
		DefaultModel: agents.ModelRef{
			Provider: "global-provider",
			Name:     "global-model",
		},
	}
	overlayModel := "overlay-model"
	overlay := core.AgentSpecOverlay{
		ModelOverlay: &core.AgentModelConfigOverlay{Name: &overlayModel},
	}

	contract, err := ResolveEffectiveAgentContract(workspace, m, ResolveOptions{
		GlobalConfig:  global,
		AgentOverlays: []core.AgentSpecOverlay{overlay},
	})
	require.NoError(t, err)
	require.NotNil(t, contract)
	require.Equal(t, "coding", contract.AgentID)
	require.Equal(t, "overlay-model", contract.AgentSpec.Model.Name)
	require.Equal(t, "manifest-provider", contract.AgentSpec.Model.Provider)
	require.Contains(t, contract.AgentSpec.Prompt, "Base prompt.")
	require.Contains(t, contract.AgentSpec.Prompt, "Review carefully.")
	require.Len(t, contract.AgentSpec.SessionPolicies, 1)
	require.Equal(t, "reviewer-send", contract.AgentSpec.SessionPolicies[0].ID)
	require.Equal(t, core.AgentPermissionAsk, contract.AgentSpec.ToolExecutionPolicy["file_read"].Execute)
	require.Equal(t, []string{"reviewer"}, contract.Sources.RequestedSkills)
	require.Equal(t, []string{"reviewer"}, contract.Sources.AppliedSkills)
	require.Empty(t, contract.Sources.FailedSkills)
	require.True(t, contract.Sources.GlobalDefaults)
	require.Equal(t, 1, contract.Sources.OverlayCount)
	require.Len(t, contract.SkillResults, 1)
	require.True(t, contract.SkillResults[0].Applied)
}

func TestResolveEffectiveAgentContractCarriesFailedSkillResults(t *testing.T) {
	workspace := t.TempDir()
	m := writeAgentManifestFixture(t, workspace, `
apiVersion: relurpify/v1alpha1
kind: AgentManifest
metadata:
  name: coding
  version: 1.0.0
spec:
  image: relurpify:test
  runtime: gvisor
  permissions:
    filesystem:
      - action: fs:read
        path: src/**
  resources:
    limits:
      cpu: "1"
      memory: "1Gi"
      disk_io: "1Gi"
  agent:
    implementation: react
    mode: primary
    model:
      provider: ollama
      name: qwen
  skills: [missing-skill]
`)

	contract, err := ResolveEffectiveAgentContract(workspace, m, ResolveOptions{})
	require.NoError(t, err)
	require.Len(t, contract.SkillResults, 1)
	require.False(t, contract.SkillResults[0].Applied)
	require.Equal(t, []string{"missing-skill"}, contract.Sources.FailedSkills)
}

func writeSkillFixture(t *testing.T, workspace, name, body string) {
	t.Helper()
	root := filepath.Join(workspace, "relurpify_cfg", "skills", name)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "scripts"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "resources"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "templates"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "skill.manifest.yaml"), []byte(body), 0o644))
}

func writeAgentManifestFixture(t *testing.T, workspace, body string) *manifest.AgentManifest {
	t.Helper()
	path := filepath.Join(workspace, "relurpify_cfg", "agent.manifest.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	m, err := manifest.LoadAgentManifest(path)
	require.NoError(t, err)
	return m
}
