package capabilities

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/stretchr/testify/require"
)

func TestTraceAnalyzeFeedsTraceToRootCauseTransform(t *testing.T) {
	env := testEnv(t)
	traceCap := &traceAnalyzeCapability{env: env}
	transformCap := &traceToRootCauseCapability{env: env}
	state := core.NewContext()
	state.Set("euclo.envelope", map[string]any{"instruction": "Trace this path and analyze it"})

	traceResult := traceCap.Execute(context.Background(), euclotypes.ExecutionEnvelope{
		Task:        &core.Task{ID: "trace-compose", Instruction: "Trace this path and analyze it"},
		Mode:        euclotypes.ModeResolution{ModeID: "debug"},
		Profile:     euclotypes.ExecutionProfileSelection{ProfileID: "trace_execute_analyze"},
		State:       state,
		Registry:    env.Registry,
		Memory:      env.Memory,
		Environment: env,
	})
	require.Equal(t, euclotypes.ExecutionStatusCompleted, traceResult.Status)
	mergeStateArtifactsToContext(state, traceResult.Artifacts)

	transformResult := transformCap.Execute(context.Background(), euclotypes.ExecutionEnvelope{
		Task:        &core.Task{ID: "trace-transform", Instruction: "Rank root-cause candidates from the trace"},
		Mode:        euclotypes.ModeResolution{ModeID: "debug"},
		Profile:     euclotypes.ExecutionProfileSelection{ProfileID: "trace_execute_analyze"},
		State:       state,
		Registry:    env.Registry,
		Memory:      env.Memory,
		Environment: env,
	})
	require.Equal(t, euclotypes.ExecutionStatusCompleted, transformResult.Status)
	require.Len(t, transformResult.Artifacts, 1)
	require.Equal(t, euclotypes.ArtifactKindRootCauseCandidates, transformResult.Artifacts[0].Kind)
}

func TestReviewImplementIfSafeComposesFindingsAndEditFlow(t *testing.T) {
	env := testEnv(t)
	cap := &reviewImplementIfSafeCapability{env: env}
	state := core.NewContext()
	state.Set("euclo.envelope", map[string]any{"instruction": "Review and implement if safe"})

	result := cap.Execute(context.Background(), euclotypes.ExecutionEnvelope{
		Task: &core.Task{
			ID:          "review-compose",
			Instruction: "Review and implement if safe",
			Context: map[string]any{
				"context_file_contents": []any{
					map[string]any{"path": "service.go", "content": "package main\n\n// TODO remove debug\nfunc Run() {}\n"},
				},
			},
		},
		Mode:        euclotypes.ModeResolution{ModeID: "review"},
		Profile:     euclotypes.ExecutionProfileSelection{ProfileID: "review_suggest_implement"},
		State:       state,
		Registry:    env.Registry,
		Memory:      env.Memory,
		Environment: env,
	})

	require.Equal(t, euclotypes.ExecutionStatusCompleted, result.Status)
	hasFindings := false
	hasEdit := false
	for _, artifact := range result.Artifacts {
		if artifact.Kind == euclotypes.ArtifactKindReviewFindings {
			hasFindings = true
		}
		if artifact.Kind == euclotypes.ArtifactKindEditIntent {
			hasEdit = true
		}
	}
	require.True(t, hasFindings)
	require.True(t, hasEdit)
}
