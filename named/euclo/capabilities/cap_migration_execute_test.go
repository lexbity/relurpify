package capabilities

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	frameworkpipeline "github.com/lexcodex/relurpify/framework/pipeline"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/internal/testutil"
	"github.com/stretchr/testify/require"
)

type migrationTestRunnerTool struct{}

func (migrationTestRunnerTool) Name() string        { return "go_test" }
func (migrationTestRunnerTool) Description() string { return "runs go tests" }
func (migrationTestRunnerTool) Category() string    { return "exec" }
func (migrationTestRunnerTool) Parameters() []core.ToolParameter {
	return nil
}
func (migrationTestRunnerTool) Execute(context.Context, *core.Context, map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true}, nil
}
func (migrationTestRunnerTool) IsAvailable(context.Context, *core.Context) bool { return true }
func (migrationTestRunnerTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: &core.PermissionSet{
		Executables: []core.ExecutablePermission{{Binary: "go"}},
	}}
}
func (migrationTestRunnerTool) Tags() []string { return []string{"test"} }

func TestMigrationStagesRunInOrder(t *testing.T) {
	model := testutil.StubModel{}
	runner := &frameworkpipeline.Runner{Options: frameworkpipeline.RunnerOptions{Model: model}}
	state := core.NewContext()
	step := map[string]any{
		"id":             "apply_migration_changes",
		"description":    "Apply dependency migration",
		"preconditions":  []string{"scope confirmed"},
		"postconditions": []string{"tests pass"},
		"files":          []string{"go.mod"},
	}
	state.Set("migration.step", step)

	results, err := runner.Execute(context.Background(), &core.Task{ID: "mig-step"}, state, newMigrationStages(step))
	require.NoError(t, err)
	require.Len(t, results, 3)
	require.Equal(t, "migration_precheck", results[0].StageName)
	require.Equal(t, "migration_execute", results[1].StageName)
	require.Equal(t, "migration_postcheck", results[2].StageName)
	_, ok := state.Get("migration.postcheck_result")
	require.True(t, ok)
}

func TestMigrationRollbackRestoresSnapshotOnFailedPostCheck(t *testing.T) {
	env := testEnv(t)
	require.NoError(t, env.Registry.Register(testutil.FileWriteTool{}))
	require.NoError(t, env.Registry.Register(migrationTestRunnerTool{}))
	cap := &migrationExecuteCapability{env: env}
	state := core.NewContext()
	state.Set("euclo.envelope", map[string]any{"instruction": "Execute the dependency migration to v2"})

	result := cap.Execute(context.Background(), euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			ID:          "migration-fail",
			Instruction: "Execute the dependency migration to v2",
			Type:        core.TaskTypeCodeModification,
			Context: map[string]any{
				"migration_fail_postcheck_step": "apply_migration_changes",
				"context_file_contents": []any{
					map[string]any{"path": "go.mod", "content": "module example\n"},
				},
			},
		},
		Mode:        euclotypes.ModeResolution{ModeID: "code"},
		Profile:     euclotypes.ExecutionProfileSelection{ProfileID: "plan_stage_execute"},
		State:       state,
		Registry:    env.Registry,
		Memory:      env.Memory,
		Environment: env,
	})

	require.Equal(t, euclotypes.ExecutionStatusPartial, result.Status)
	require.NotNil(t, result.FailureInfo)
	require.Len(t, result.Artifacts, 4)
	verification := result.Artifacts[len(result.Artifacts)-1]
	require.Equal(t, euclotypes.ArtifactKindVerification, verification.Kind)
	payload, ok := verification.Payload.(map[string]any)
	require.True(t, ok)
	rollback, ok := payload["rollback"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, rollback["restored"])
	require.Equal(t, float64(1), float64(intValue(rollback["restored_files"])))
}

func TestMigrationExecuteProducesPlanEditAndVerification(t *testing.T) {
	env := testEnv(t)
	require.NoError(t, env.Registry.Register(testutil.FileWriteTool{}))
	require.NoError(t, env.Registry.Register(migrationTestRunnerTool{}))
	cap := &migrationExecuteCapability{env: env}
	state := core.NewContext()
	state.Set("euclo.envelope", map[string]any{"instruction": "Migrate the dependency update to the new SDK version"})

	result := cap.Execute(context.Background(), euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			ID:          "migration-success",
			Instruction: "Migrate the dependency update to the new SDK version",
			Type:        core.TaskTypeCodeModification,
			Context: map[string]any{
				"context_file_contents": []any{
					map[string]any{"path": "go.mod", "content": "module example\n"},
					map[string]any{"path": "service.go", "content": "package service\n"},
				},
			},
		},
		Mode:        euclotypes.ModeResolution{ModeID: "code"},
		Profile:     euclotypes.ExecutionProfileSelection{ProfileID: "plan_stage_execute"},
		State:       state,
		Registry:    env.Registry,
		Memory:      env.Memory,
		Environment: env,
	})

	require.Equal(t, euclotypes.ExecutionStatusCompleted, result.Status)
	require.GreaterOrEqual(t, len(result.Artifacts), 3)
	require.Equal(t, euclotypes.ArtifactKindMigrationPlan, result.Artifacts[0].Kind)
	require.Equal(t, euclotypes.ArtifactKindVerification, result.Artifacts[len(result.Artifacts)-1].Kind)
}
