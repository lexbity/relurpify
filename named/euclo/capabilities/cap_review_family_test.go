package capabilities

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/stretchr/testify/require"
)

func TestReviewFindingsExecuteProducesStructuredArtifact(t *testing.T) {
	env := testEnv(t)
	cap := &reviewFindingsCapability{env: env}
	state := core.NewContext()
	state.Set("euclo.envelope", map[string]any{"instruction": "Review this code for correctness"})

	result := cap.Execute(context.Background(), euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			ID:          "review-findings",
			Instruction: "Review this code for correctness",
			Context: map[string]any{
				"context_file_contents": []any{
					map[string]any{"path": "service.go", "content": "package main\n\nfunc Run() {\n\tpanic(\"boom\")\n}\n"},
				},
			},
		},
		State:       state,
		Registry:    env.Registry,
		Memory:      env.Memory,
		Environment: env,
	})

	require.Equal(t, euclotypes.ExecutionStatusCompleted, result.Status)
	require.Len(t, result.Artifacts, 1)
	require.Equal(t, euclotypes.ArtifactKindReviewFindings, result.Artifacts[0].Kind)

	payload, ok := result.Artifacts[0].Payload.(map[string]any)
	require.True(t, ok)
	stats, ok := payload["stats"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, 1, intValue(stats["critical_count"]))
}

func TestReviewCompatibilityExecuteProducesAssessment(t *testing.T) {
	env := testEnv(t)
	cap := &reviewCompatibilityCapability{env: env}
	state := core.NewContext()
	state.Set("euclo.envelope", map[string]any{"instruction": "Check compatibility for these changes"})

	result := cap.Execute(context.Background(), euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			ID:          "review-compat",
			Instruction: "Check compatibility for these changes",
			Context: map[string]any{
				"context_file_contents": []any{
					map[string]any{"path": "api.go", "content": "package api\n\nfunc Exported() {}\n"},
				},
			},
		},
		State:       state,
		Registry:    env.Registry,
		Memory:      env.Memory,
		Environment: env,
	})

	require.Equal(t, euclotypes.ExecutionStatusCompleted, result.Status)
	require.Len(t, result.Artifacts, 1)
	require.Equal(t, euclotypes.ArtifactKindCompatibilityAssessment, result.Artifacts[0].Kind)
}

func TestReviewImplementIfSafeStopsOnCriticalAndImplementsWarnings(t *testing.T) {
	env := testEnv(t)
	cap := &reviewImplementIfSafeCapability{env: env}

	criticalState := core.NewContext()
	criticalState.Set("euclo.envelope", map[string]any{"instruction": "Fix all critical findings"})
	criticalResult := cap.Execute(context.Background(), euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			ID:          "review-fix-critical",
			Instruction: "Fix all critical findings",
			Context: map[string]any{
				"context_file_contents": []any{
					map[string]any{"path": "service.go", "content": "package main\n\nfunc Run() {\n\tpanic(\"boom\")\n}\n"},
				},
			},
		},
		State:       criticalState,
		Registry:    env.Registry,
		Memory:      env.Memory,
		Environment: env,
	})
	require.Equal(t, euclotypes.ExecutionStatusCompleted, criticalResult.Status)
	require.Len(t, criticalResult.Artifacts, 1)
	require.Equal(t, euclotypes.ArtifactKindReviewFindings, criticalResult.Artifacts[0].Kind)

	warningState := core.NewContext()
	warningState.Set("euclo.envelope", map[string]any{"instruction": "Implement if safe and fix findings"})
	warningResult := cap.Execute(context.Background(), euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			ID:          "review-fix-warning",
			Instruction: "Implement if safe and fix findings",
			Context: map[string]any{
				"context_file_contents": []any{
					map[string]any{"path": "service.go", "content": "package main\n\n// TODO remove debug\nfunc Run() {}\n"},
				},
			},
		},
		Mode:        euclotypes.ModeResolution{ModeID: "review"},
		Profile:     euclotypes.ExecutionProfileSelection{ProfileID: "review_suggest_implement"},
		State:       warningState,
		Registry:    env.Registry,
		Memory:      env.Memory,
		Environment: env,
	})
	require.Equal(t, euclotypes.ExecutionStatusCompleted, warningResult.Status)
	require.GreaterOrEqual(t, len(warningResult.Artifacts), 3)
	hasEditIntent := false
	for _, artifact := range warningResult.Artifacts {
		if artifact.Kind == euclotypes.ArtifactKindEditIntent {
			hasEditIntent = true
		}
	}
	require.True(t, hasEditIntent)
}
