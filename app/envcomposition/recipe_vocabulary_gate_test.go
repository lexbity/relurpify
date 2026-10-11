package envcomposition

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	fsandbox "codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
	"codeburg.org/lexbit/relurpify/named/euclo/services"
	thoughtrecipe "codeburg.org/lexbit/relurpify/named/euclo/thoughtrecipes"
	"codeburg.org/lexbit/relurpify/userconfig/templates"
)

// composeShippedRuntime composes the capability runtime against a freshly
// materialized embedded template workspace exactly as production does:
// Build → Register (BuildCapabilityRuntime) → relurpic capabilities
// (mirroring agent.Initialize order) — then loads every shipped recipe
// through the production loader backed by the SAME registry instance (FR-5).
func composeShippedRuntime(t *testing.T) (*CapabilityRuntime, string) {
	t.Helper()
	workspace := t.TempDir()
	require.NoError(t, templates.GenerateConfig(filepath.Join(workspace, "relurpify_cfg")))
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "hello.txt"), []byte("composition gate"), 0o600))

	runner, err := fsandbox.NewAuthorizedRunner(noopCommandRunner{}, fsandbox.CommandPolicyFunc(func(context.Context, fsandbox.CommandRequest) error {
		return nil
	}))
	require.NoError(t, err)

	rt, err := BuildCapabilityRuntime(context.Background(), workspace, runner, CapabilityRuntimeOptions{
		SkipASTIndex: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		if rt.IndexManager != nil {
			_ = rt.IndexManager.Close(context.Background())
		}
	})

	if err := relurpicabilities.RegisterAll(relurpicabilities.RegistrationDeps{
		Registry:  rt.Registry,
		Declared:  services.EucloCapabilityIDs(),
		Workspace: workspace,
	}); err != nil {
		t.Fatalf("register relurpic capabilities: %v", err)
	}
	return rt, workspace
}

// TestCompositionRootRecipeVocabularyGate is the standing D8 gate: every
// shipped recipe's every declared `may invoke` name resolves against the
// composed registry, and the stamped file tools enforce the workspace scope
// through the allowlist child — the composition that let F1/F2 survive every
// hermetic suite is now proven on every commit.
func TestCompositionRootRecipeVocabularyGate(t *testing.T) {
	rt, workspace := composeShippedRuntime(t)
	ctx := context.Background()

	loader := thoughtrecipe.NewLoader().WithCapabilityRegistry(services.CapabilityLookup(rt.Registry))
	loaded, err := loader.LoadWorkspace(workspace)
	require.NoError(t, err, "every shipped recipe must load against the composed registry")
	require.NotEmpty(t, loaded.Registry.Entries(), "shipped recipe corpus must be non-empty")

	for _, entry := range loaded.Registry.Entries() {
		require.NotNil(t, entry.Plan, "recipe %s has no compiled plan", entry.Name)
		var walk func(steps []thoughtrecipe.ExecutionStep)
		walk = func(steps []thoughtrecipe.ExecutionStep) {
			for _, step := range steps {
				for _, name := range step.Scope.AllowedToolNames() {
					_, ok := rt.Registry.GetCapability(name)
					require.True(t, ok, "recipe %s step %s: declared %q does not resolve in the composed registry", entry.Name, step.ID, name)
				}
			}
		}
		walk(entry.Plan.Steps)
		for _, group := range entry.Plan.Routes {
			for _, branch := range group.Branches {
				walk(branch.Steps)
			}
		}
		for _, pipeline := range entry.Plan.Pipelines {
			for _, stage := range pipeline.Stages {
				walk(stage.Steps)
			}
		}
	}

	// FR-7 invocation leg through a WithAllowlist child of the composed
	// registry: the stamped scope admits the workspace and denies outside.
	desc, ok := rt.Registry.GetCapability("file_read")
	require.True(t, ok)
	child := rt.Registry.WithAllowlist([]string{desc.ID})
	inside, err := child.InvokeCapability(ctx, nil, desc.ID, map[string]any{"path": "hello.txt"})
	require.NoError(t, err)
	require.True(t, inside.Success, "workspace read must succeed: %s", inside.Error)
	require.Equal(t, "composition gate", inside.Data["content"])

	_, err = child.InvokeCapability(ctx, nil, desc.ID, map[string]any{"path": filepath.Join("relurpify_cfg", "workspace.yaml")})
	require.Error(t, err, "protected-path read must deny through the stamped scope")
}

// TestCompositionRootRecipeVocabularyGateFailsOnUnknownDeclaration proves the
// gate bites: a workspace recipe declaring an unresolvable tool fails the
// load loudly naming the tool (D2) — the gate's red state is reachable.
func TestCompositionRootRecipeVocabularyGateFailsOnUnknownDeclaration(t *testing.T) {
	rt, workspace := composeShippedRuntime(t)

	badRecipe := filepath.Join(workspace, "relurpify_cfg", "euclo", "zz_gate_probe.erpe")
	require.NoError(t, os.WriteFile(badRecipe, []byte(`thoughtrecipe gate_probe
"Declares a tool the registry does not have."

trigger as capability:
  may read workspace
  may invoke ["no_such_tool_xyz"]

input workspace: "**/*"
agent reviewer uses react

run reviewer:
  from input.workspace
  goal "probe"
`), 0o600))

	loader := thoughtrecipe.NewLoader().WithCapabilityRegistry(services.CapabilityLookup(rt.Registry))
	_, err := loader.LoadWorkspace(workspace)
	require.Error(t, err, "an unresolvable declared tool must fail the load")
	require.Contains(t, err.Error(), "no_such_tool_xyz")
}

// TestReactStepToolScopeIsPureFunctionOfScope is the O1 probe (D6): the
// debug_tdd_repair repairer step's scoped child must expose the full declared
// scope — file_read, file_search (the grep alias), and file_write — as
// callable tools. The live scratch turn ran with 2 of 4 because a layer
// between scope and prompt dropped the rest; this pins scope ∩ callable ∩
// available as the only filter chain.
func TestReactStepToolScopeIsPureFunctionOfScope(t *testing.T) {
	rt, workspace := composeShippedRuntime(t)

	loader := thoughtrecipe.NewLoader().WithCapabilityRegistry(services.CapabilityLookup(rt.Registry))
	loaded, err := loader.LoadWorkspace(workspace)
	require.NoError(t, err)
	plan, ok := loaded.Registry.GetPlan("euclo.thoughtrecipe.debug_tdd_repair")
	require.True(t, ok, "debug_tdd_repair must be loaded")

	var repairer *thoughtrecipe.ExecutionStep
	var walk func(steps []thoughtrecipe.ExecutionStep)
	walk = func(steps []thoughtrecipe.ExecutionStep) {
		for i, step := range steps {
			if step.Kind == thoughtrecipe.StepKindRun && step.Paradigm == "react" && repairer == nil {
				repairer = &steps[i]
			}
		}
	}
	walk(plan.Steps)
	require.NotNil(t, repairer, "debug_tdd_repair must contain a react run step")

	// Exactly what stepCore.scopedRegistry does: resolve every declared name
	// through the registry, scope a child to the resolved IDs.
	allowed := make([]string, 0, len(repairer.Scope.AllowedToolNames()))
	for _, name := range repairer.Scope.AllowedToolNames() {
		desc, ok := rt.Registry.GetCapability(name)
		require.True(t, ok, "declared %q must resolve", name)
		allowed = append(allowed, desc.ID)
	}
	child := rt.Registry.WithAllowlist(allowed)

	exposed := map[string]bool{}
	for _, tool := range child.CallableTools(context.Background()) {
		exposed[tool.Name()] = true
	}
	t.Logf("repairer step %q declared=%v resolved=%v exposed=%v",
		repairer.ID, repairer.Scope.AllowedToolNames(), allowed, exposed)

	for _, required := range []string{"file_read", "file_search", "file_write"} {
		require.True(t, exposed[required], "repairer scoped registry must expose %s (declared scope: %v)", required, repairer.Scope.AllowedToolNames())
	}
}
