package capabilities

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/internal/testutil"
	"github.com/stretchr/testify/require"
)

type refactorTestRunnerTool struct{}

func (refactorTestRunnerTool) Name() string        { return "go_test" }
func (refactorTestRunnerTool) Description() string { return "runs go tests" }
func (refactorTestRunnerTool) Category() string    { return "exec" }
func (refactorTestRunnerTool) Parameters() []core.ToolParameter {
	return nil
}
func (refactorTestRunnerTool) Execute(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true}, nil
}
func (refactorTestRunnerTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (refactorTestRunnerTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: &core.PermissionSet{
		Executables: []core.ExecutablePermission{{Binary: "go"}},
	}}
}
func (refactorTestRunnerTool) Tags() []string { return []string{"test"} }

func TestRefactorDecomposeExtractAndRenameProducesExpectedSequence(t *testing.T) {
	plan, methodName, err := decomposeRefactorPlan(&core.Task{
		ID:          "refactor-1",
		Type:        core.TaskTypeCodeModification,
		Instruction: "Extract function and rename helper while keeping the public API stable",
	})
	require.NoError(t, err)
	require.Equal(t, "api_compatible_extract_and_rename", methodName)
	require.Len(t, plan.Steps, 3)
	require.Equal(t, "api_compatible_extract_and_rename.extract_function", plan.Steps[0].ID)
	require.Equal(t, "api_compatible_extract_and_rename.rename_symbol", plan.Steps[1].ID)
	require.Equal(t, "api_compatible_extract_and_rename.verify", plan.Steps[2].ID)
	require.Equal(t, []string{"api_compatible_extract_and_rename.extract_function"}, plan.Dependencies[plan.Steps[1].ID])
}

func TestRefactorAPICompatibleBlocksBreakingExportedRename(t *testing.T) {
	env := testEnv(t)
	cap := &refactorAPICompatibleCapability{env: env}
	state := core.NewContext()
	state.Set("euclo.envelope", map[string]any{"instruction": "Refactor by renaming Exported to BetterExported while keeping the public API stable"})

	result := cap.Execute(context.Background(), euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			ID:          "refactor-exported",
			Instruction: "Refactor by renaming Exported to BetterExported while keeping the public API stable",
			Type:        core.TaskTypeCodeModification,
			Context: map[string]any{
				"context_file_contents": []any{
					map[string]any{"path": "api.go", "content": "package api\n\nfunc Exported() {}\n"},
				},
			},
		},
		Mode:        euclotypes.ModeResolution{ModeID: "code"},
		Profile:     euclotypes.ExecutionProfileSelection{ProfileID: "edit_verify_repair"},
		State:       state,
		Registry:    env.Registry,
		Memory:      env.Memory,
		Environment: env,
	})

	require.Equal(t, euclotypes.ExecutionStatusCompleted, result.Status)
	require.Contains(t, result.Summary, "blocked refactor step")
	require.Len(t, result.Artifacts, 2)
	require.Equal(t, euclotypes.ArtifactKindPlan, result.Artifacts[0].Kind)
	require.Equal(t, euclotypes.ArtifactKindCompatibilityAssessment, result.Artifacts[1].Kind)
	payload, ok := result.Artifacts[1].Payload.(map[string]any)
	require.True(t, ok)
	require.False(t, payload["overall_compatible"].(bool))
	require.NotEmpty(t, payload["breaking_changes"])
}

func TestRefactorAPICompatibleExecutesInternalRefactor(t *testing.T) {
	env := testEnv(t)
	require.NoError(t, env.Registry.Register(testutil.FileWriteTool{}))
	require.NoError(t, env.Registry.Register(refactorTestRunnerTool{}))

	cap := &refactorAPICompatibleCapability{env: env}
	state := core.NewContext()
	state.Set("euclo.envelope", map[string]any{"instruction": "Refactor by extracting a helper and renaming helper to worker while keeping the public API stable"})

	result := cap.Execute(context.Background(), euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			ID:          "refactor-internal",
			Instruction: "Refactor by extracting a helper and renaming helper to worker while keeping the public API stable",
			Type:        core.TaskTypeCodeModification,
			Context: map[string]any{
				"context_file_contents": []any{
					map[string]any{"path": "service.go", "content": "package service\n\nfunc Exported() { helper() }\n\nfunc helper() {}\n"},
				},
			},
		},
		Mode:        euclotypes.ModeResolution{ModeID: "code"},
		Profile:     euclotypes.ExecutionProfileSelection{ProfileID: "edit_verify_repair"},
		State:       state,
		Registry:    env.Registry,
		Memory:      env.Memory,
		Environment: env,
	})

	require.Equal(t, euclotypes.ExecutionStatusCompleted, result.Status)
	hasEditIntent := false
	hasVerification := false
	hasCompatibility := false
	for _, artifact := range result.Artifacts {
		switch artifact.Kind {
		case euclotypes.ArtifactKindEditIntent:
			hasEditIntent = true
		case euclotypes.ArtifactKindVerification:
			hasVerification = true
		case euclotypes.ArtifactKindCompatibilityAssessment:
			hasCompatibility = true
			payload, ok := artifact.Payload.(map[string]any)
			require.True(t, ok)
			require.True(t, payload["overall_compatible"].(bool))
		}
	}
	require.True(t, hasEditIntent)
	require.True(t, hasVerification)
	require.True(t, hasCompatibility)
}
