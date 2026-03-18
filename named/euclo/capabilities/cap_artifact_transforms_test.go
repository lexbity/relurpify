package capabilities

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/stretchr/testify/require"
)

func TestDiffSummaryExecuteProducesStructuredArtifact(t *testing.T) {
	env := testEnv(t)
	cap := &diffSummaryCapability{env: env}
	state := core.NewContext()
	state.Set("euclo.artifacts", []euclotypes.Artifact{{
		ID:      "edit_1",
		Kind:    euclotypes.ArtifactKindEditIntent,
		Summary: "Rename helper and adjust tests",
		Payload: map[string]any{
			"summary": "Rename helper and adjust tests",
			"files":   []any{"service.go", "service_test.go"},
		},
	}})

	result := cap.Execute(context.Background(), euclotypes.ExecutionEnvelope{
		State:       state,
		Registry:    env.Registry,
		Memory:      env.Memory,
		Environment: env,
	})

	require.Equal(t, euclotypes.ExecutionStatusCompleted, result.Status)
	require.Len(t, result.Artifacts, 1)
	require.Equal(t, euclotypes.ArtifactKindDiffSummary, result.Artifacts[0].Kind)

	payload, ok := result.Artifacts[0].Payload.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "moderate", payload["scope_assessment"])
	require.Equal(t, "medium", payload["risk_level"])
	files := payload["files"].([]map[string]any)
	require.Len(t, files, 2)
}

func TestTraceToRootCauseExecuteProducesRankedCandidates(t *testing.T) {
	env := testEnv(t)
	cap := &traceToRootCauseCapability{env: env}
	state := core.NewContext()
	state.Set("euclo.artifacts", []euclotypes.Artifact{{
		ID:      "trace_1",
		Kind:    euclotypes.ArtifactKindTrace,
		Summary: "panic trace",
		Payload: map[string]any{
			"frames": []any{
				map[string]any{"function": "service.Run", "detail": "unexpected nil error path"},
				map[string]any{"function": "service.load", "detail": "error returned before state initialized"},
			},
		},
	}})

	result := cap.Execute(context.Background(), euclotypes.ExecutionEnvelope{
		State:       state,
		Registry:    env.Registry,
		Memory:      env.Memory,
		Environment: env,
	})

	require.Equal(t, euclotypes.ExecutionStatusCompleted, result.Status)
	require.Len(t, result.Artifacts, 1)
	require.Equal(t, euclotypes.ArtifactKindRootCauseCandidates, result.Artifacts[0].Kind)

	payload, ok := result.Artifacts[0].Payload.(map[string]any)
	require.True(t, ok)
	candidates := payload["candidates"].([]map[string]any)
	require.Len(t, candidates, 2)
	top := payload["top_candidate"].(map[string]any)
	require.Equal(t, "service.Run", top["location"])
}

func TestVerificationSummaryExecuteProducesNormalizedResult(t *testing.T) {
	env := testEnv(t)
	cap := &verificationSummaryCapability{env: env}
	state := core.NewContext()
	state.Set("euclo.artifacts", []euclotypes.Artifact{{
		ID:      "verify_1",
		Kind:    euclotypes.ArtifactKindVerification,
		Summary: "Tests failed in package service",
		Payload: map[string]any{
			"summary": "Tests failed in package service",
			"gaps":    []any{"lint not run"},
		},
	}})

	result := cap.Execute(context.Background(), euclotypes.ExecutionEnvelope{
		State:       state,
		Registry:    env.Registry,
		Memory:      env.Memory,
		Environment: env,
	})

	require.Equal(t, euclotypes.ExecutionStatusCompleted, result.Status)
	require.Len(t, result.Artifacts, 1)
	require.Equal(t, euclotypes.ArtifactKindVerificationSummary, result.Artifacts[0].Kind)

	payload, ok := result.Artifacts[0].Payload.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "fail", payload["overall_status"])
	require.Equal(t, "reject", payload["recommendation"])
	require.NotEmpty(t, payload["checks"])
}
