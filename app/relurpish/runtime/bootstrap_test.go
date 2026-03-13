package runtime

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/capability"
	contractpkg "github.com/lexcodex/relurpify/framework/contract"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/memory"
	fsandbox "github.com/lexcodex/relurpify/framework/sandbox"
	"gopkg.in/yaml.v3"
)

func TestBootstrapAgentRuntimeUsesManifestModelAndRegistersAgentCapabilities(t *testing.T) {
	workspace := t.TempDir()
	store, err := memory.NewHybridMemory(t.TempDir())
	if err != nil {
		t.Fatalf("NewHybridMemory: %v", err)
	}
	embedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		embeddings := make([][]float32, 0, len(req.Input))
		for range req.Input {
			embeddings = append(embeddings, []float32{1, 2})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": embeddings})
	}))
	defer embedServer.Close()

	boot, err := BootstrapAgentRuntime(workspace, AgentBootstrapOptions{
		AgentID:    "coding",
		AgentName:  "coding",
		ConfigName: "coding",
		Manifest: &manifest.AgentManifest{
			Metadata: manifest.ManifestMetadata{Name: "coding"},
			Spec: manifest.ManifestSpec{
				Agent: &core.AgentRuntimeSpec{
					Model: core.AgentModelConfig{Name: "manifest-model"},
				},
			},
		},
		Runner:         fsandbox.NewLocalCommandRunner(workspace, nil),
		Memory:         store,
		OllamaEndpoint: embedServer.URL,
	})
	if err != nil {
		t.Fatalf("BootstrapAgentRuntime: %v", err)
	}

	if boot.AgentConfig.Model != "manifest-model" {
		t.Fatalf("expected manifest model, got %q", boot.AgentConfig.Model)
	}
	if boot.AgentSpec == nil {
		t.Fatal("expected agent spec")
	}
	if boot.Registry == nil {
		t.Fatal("expected capability registry")
	}
	if !boot.Registry.HasCapability("agent:planner") {
		t.Fatal("expected shared agent capabilities to be registered")
	}
}

func TestBootstrapAgentRuntimeRequiresManifestAgentSpec(t *testing.T) {
	workspace := t.TempDir()

	_, err := BootstrapAgentRuntime(workspace, AgentBootstrapOptions{
		AgentID:   "coding",
		AgentName: "coding",
		Manifest: &manifest.AgentManifest{
			Metadata: manifest.ManifestMetadata{Name: "coding"},
		},
		Runner: fsandbox.NewLocalCommandRunner(workspace, nil),
	})
	if err == nil {
		t.Fatal("expected missing spec.agent to fail")
	}
}

func TestBootstrapAgentRuntimeAdmitsSkillCapabilitiesUsingFinalResolvedSelectors(t *testing.T) {
	workspace := t.TempDir()
	skillRoot := filepath.Join(workspace, "relurpify_cfg", "skills", "reviewer")
	for _, dir := range []string{"scripts", "resources", "templates"} {
		requireNoError(t, os.MkdirAll(filepath.Join(skillRoot, dir), 0o755))
	}
	skill := manifest.SkillManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "SkillManifest",
		Metadata:   manifest.ManifestMetadata{Name: "reviewer", Version: "1.0.0"},
		Spec: manifest.SkillSpec{
			PromptSnippets: []string{"Review the change carefully."},
			AllowedCapabilities: []core.CapabilitySelector{{
				Name: "reviewer.prompt.1",
				Kind: core.CapabilityKindPrompt,
			}},
		},
	}
	data, err := yaml.Marshal(skill)
	requireNoError(t, err)
	requireNoError(t, os.WriteFile(filepath.Join(skillRoot, "skill.manifest.yaml"), data, 0o644))
	store, err := memory.NewHybridMemory(t.TempDir())
	requireNoError(t, err)

	boot, err := BootstrapAgentRuntime(workspace, AgentBootstrapOptions{
		AgentID:    "coding",
		AgentName:  "coding",
		ConfigName: "coding",
		Manifest: &manifest.AgentManifest{
			Metadata: manifest.ManifestMetadata{Name: "coding"},
			Spec: manifest.ManifestSpec{
				Agent: &core.AgentRuntimeSpec{
					Model: core.AgentModelConfig{Name: "manifest-model"},
					AllowedCapabilities: []core.CapabilitySelector{{
						Name: "file_read",
						Kind: core.CapabilityKindTool,
					}},
				},
				Skills: []string{"reviewer"},
			},
		},
		Runner:       fsandbox.NewLocalCommandRunner(workspace, nil),
		Memory:       store,
		SkipASTIndex: true,
	})
	requireNoError(t, err)
	if !boot.Registry.HasCapability("reviewer.prompt.1") {
		t.Fatal("expected reviewer prompt capability to be admitted")
	}
	admittedPrompt := false
	for _, admission := range boot.CapabilityAdmissions {
		if admission.CapabilityID == "prompt:reviewer:1" && admission.Admitted {
			admittedPrompt = true
			break
		}
	}
	if !admittedPrompt {
		t.Fatalf("expected admitted reviewer prompt capability, got %+v", boot.CapabilityAdmissions)
	}
}

func TestBootstrapAgentRuntimeRegistersLocalToolsUsingFinalSkillExpandedSelectors(t *testing.T) {
	workspace := t.TempDir()
	skillRoot := filepath.Join(workspace, "relurpify_cfg", "skills", "tooling")
	for _, dir := range []string{"scripts", "resources", "templates"} {
		requireNoError(t, os.MkdirAll(filepath.Join(skillRoot, dir), 0o755))
	}
	skill := manifest.SkillManifest{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "SkillManifest",
		Metadata:   manifest.ManifestMetadata{Name: "tooling", Version: "1.0.0"},
		Spec: manifest.SkillSpec{
			AllowedCapabilities: []core.CapabilitySelector{{
				Name: "file_read",
				Kind: core.CapabilityKindTool,
			}},
		},
	}
	data, err := yaml.Marshal(skill)
	requireNoError(t, err)
	requireNoError(t, os.WriteFile(filepath.Join(skillRoot, "skill.manifest.yaml"), data, 0o644))
	store, err := memory.NewHybridMemory(t.TempDir())
	requireNoError(t, err)

	boot, err := BootstrapAgentRuntime(workspace, AgentBootstrapOptions{
		AgentID:    "coding",
		AgentName:  "coding",
		ConfigName: "coding",
		Manifest: &manifest.AgentManifest{
			Metadata: manifest.ManifestMetadata{Name: "coding"},
			Spec: manifest.ManifestSpec{
				Agent: &core.AgentRuntimeSpec{
					Model: core.AgentModelConfig{Name: "manifest-model"},
					AllowedCapabilities: []core.CapabilitySelector{{
						Name: "nonexistent_tool",
						Kind: core.CapabilityKindTool,
					}},
				},
				Skills: []string{"tooling"},
			},
		},
		Runner:       fsandbox.NewLocalCommandRunner(workspace, nil),
		Memory:       store,
		SkipASTIndex: true,
	})
	requireNoError(t, err)
	if _, ok := boot.Registry.Get("file_read"); !ok {
		t.Fatal("expected file_read to be registered under final skill-expanded selectors")
	}
}

func TestBootstrapAgentRuntimeAppliesSelectedAgentDefinitionOverlayToEffectiveContract(t *testing.T) {
	workspace := t.TempDir()
	agentsDir := filepath.Join(workspace, "agents")
	requireNoError(t, os.MkdirAll(agentsDir, 0o755))
requireNoError(t, os.WriteFile(filepath.Join(agentsDir, "reviewer.yaml"), []byte(`
kind: AgentDefinition
name: reviewer
spec:
  implementation: react
  mode: primary
  model:
    provider: ollama
    name: manifest-model
  provider_policies:
    browser:
      activate: deny
`), 0o644))
	store, err := memory.NewHybridMemory(t.TempDir())
	requireNoError(t, err)

	boot, err := BootstrapAgentRuntime(workspace, AgentBootstrapOptions{
		AgentID:    "coding",
		AgentName:  "reviewer",
		ConfigName: "reviewer",
		AgentsDir:  agentsDir,
		Manifest: &manifest.AgentManifest{
			Metadata: manifest.ManifestMetadata{Name: "coding"},
			Spec: manifest.ManifestSpec{
				Agent: &core.AgentRuntimeSpec{
					Implementation: "react",
					Mode:           core.AgentModePrimary,
					Model:          core.AgentModelConfig{Provider: "ollama", Name: "manifest-model"},
				},
			},
		},
		Runner:       fsandbox.NewLocalCommandRunner(workspace, nil),
		Memory:       store,
		SkipASTIndex: true,
	})
	requireNoError(t, err)
	if boot.Contract == nil || boot.Contract.AgentSpec == nil {
		t.Fatal("expected effective contract")
	}
	if policy := boot.Contract.AgentSpec.ProviderPolicies["browser"]; policy.Activate != core.AgentPermissionDeny {
		t.Fatalf("expected definition overlay provider policy to be applied, got %+v", policy)
	}
	if boot.CompiledPolicy == nil || boot.CompiledPolicy.Engine == nil {
		t.Fatal("expected compiled policy from effective contract")
	}
}

func TestSwitchAgentUsesEffectiveContractOverlay(t *testing.T) {
	workspace := t.TempDir()
	agentsDir := filepath.Join(workspace, "agents")
	requireNoError(t, os.MkdirAll(agentsDir, 0o755))
requireNoError(t, os.WriteFile(filepath.Join(agentsDir, "reviewer.yaml"), []byte(`
kind: AgentDefinition
name: reviewer
spec:
  implementation: react
  mode: primary
  model:
    provider: ollama
    name: manifest-model
  provider_policies:
    browser:
      activate: deny
`), 0o644))
	permMgr, err := authorization.NewPermissionManager(workspace, &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{{Action: core.FileSystemRead, Path: "**"}},
	}, nil, nil)
	requireNoError(t, err)
	rt := &Runtime{
		Config: Config{
			Workspace:      workspace,
			AgentsDir:      agentsDir,
			AgentName:      "coding",
			OllamaModel:    "manifest-model",
			OllamaEndpoint: "http://localhost:11434",
		},
		Tools: capability.NewRegistry(),
		Model: nil,
		Memory: nil,
		IndexManager: nil,
		SearchEngine: nil,
		Registration: &authorization.AgentRegistration{
			ID:          "coding",
			Manifest:    &manifest.AgentManifest{Metadata: manifest.ManifestMetadata{Name: "coding"}, Spec: manifest.ManifestSpec{Agent: &core.AgentRuntimeSpec{Implementation: "react", Mode: core.AgentModePrimary, Model: core.AgentModelConfig{Provider: "ollama", Name: "manifest-model"}}}},
			Permissions: permMgr,
		},
		AgentDefinitions: map[string]*core.AgentDefinition{},
		AgentSpec: &core.AgentRuntimeSpec{
			Implementation: "react",
			Mode:           core.AgentModePrimary,
			Model:          core.AgentModelConfig{Provider: "ollama", Name: "manifest-model"},
		},
		EffectiveContract: &contractpkg.EffectiveAgentContract{
			AgentID: "coding",
			AgentSpec: &core.AgentRuntimeSpec{
				Implementation: "react",
				Mode:           core.AgentModePrimary,
				Model:          core.AgentModelConfig{Provider: "ollama", Name: "manifest-model"},
			},
		},
	}

	requireNoError(t, rt.SwitchAgent("reviewer"))
	if rt.EffectiveContract == nil || rt.EffectiveContract.AgentSpec == nil {
		t.Fatal("expected effective contract after switch")
	}
	if policy := rt.EffectiveContract.AgentSpec.ProviderPolicies["browser"]; policy.Activate != core.AgentPermissionDeny {
		t.Fatalf("expected switched runtime to use definition overlay provider policy, got %+v", policy)
	}
	if rt.CompiledPolicy == nil || rt.CompiledPolicy.Engine == nil {
		t.Fatal("expected compiled policy after switch")
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
