package capabilities

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestInvestigateRegressionDescriptor(t *testing.T) {
	cap := &investigateRegressionCapability{env: testEnv(t)}
	desc := cap.Descriptor()
	require.Equal(t, "euclo:debug.investigate_regression", desc.ID)
	require.Contains(t, desc.Tags, "regression")
	profiles, ok := desc.Annotations["supported_profiles"].([]string)
	require.True(t, ok)
	require.Contains(t, profiles, "reproduce_localize_patch")
}

func TestInvestigateRegressionEligibleOnlyForRegressionShapedIntake(t *testing.T) {
	cap := &investigateRegressionCapability{env: testEnv(t)}
	snapshot := euclotypes.CapabilitySnapshot{HasExecuteTools: true}

	eligible := cap.Eligible(euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindIntake, Payload: map[string]any{"instruction": "This used to work but now fails after recent changes"}},
	}), snapshot)
	require.True(t, eligible.Eligible)

	ineligible := cap.Eligible(euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindIntake, Payload: map[string]any{"instruction": "Debug why the current test is failing"}},
	}), snapshot)
	require.False(t, ineligible.Eligible)
	require.Contains(t, ineligible.Reason, "regression")
}

func TestInvestigateRegressionExecuteProducesRegressionArtifactsAndDelegatesDebugFlow(t *testing.T) {
	env := testEnv(t)
	env.Model = &regressionQueueModel{
		responses: []string{
			`{"facts":[{"key":"regression:suspect_changes","value":[{"commit":"abc123","file":"calc.go","function":"Multiply","relevance_score":0.94,"change_description":"changed multiply to addition"}]}],"summary":"suspect changes found"}`,
			`{"facts":[{"key":"regression:correlations","value":[{"change":"abc123","test":"TestMultiply","result":"failed","correlation_strength":0.91}]}],"summary":"test correlation found"}`,
			`{"facts":[{"key":"regression:reproduction","value":{"reproduced":true,"method":"go test ./testsuite/fixtures","error":"expected 12 got 7","root_change":{"commit":"abc123","file":"calc.go","function":"Multiply","change_description":"changed multiply to addition"},"confidence":0.93}}],"summary":"reproduced regression"}`,
		},
		fallback: testutil.StubModel{},
	}

	cap := &investigateRegressionCapability{env: env}
	state := core.NewContext()
	state.Set("euclo.envelope", map[string]any{"instruction": "TestMultiply used to work but now fails after recent changes"})

	result := cap.Execute(context.Background(), euclotypes.ExecutionEnvelope{
		Task:        &core.Task{ID: "regression-1", Instruction: "TestMultiply used to work but now fails after recent changes"},
		Mode:        euclotypes.ModeResolution{ModeID: "debug"},
		Profile:     euclotypes.ExecutionProfileSelection{ProfileID: "reproduce_localize_patch"},
		Registry:    env.Registry,
		State:       state,
		Memory:      env.Memory,
		Environment: env,
	})

	require.Equal(t, euclotypes.ExecutionStatusCompleted, result.Status)
	require.NotNil(t, result.RecoveryHint)
	require.Equal(t, "euclo:reproduce_localize_patch", result.RecoveryHint.SuggestedCapability)

	kinds := map[euclotypes.ArtifactKind]bool{}
	for _, artifact := range result.Artifacts {
		kinds[artifact.Kind] = true
	}
	require.True(t, kinds[euclotypes.ArtifactKindRegressionAnalysis])
	require.True(t, kinds[euclotypes.ArtifactKindReproduction])
	require.True(t, kinds[euclotypes.ArtifactKindRootCause])
	require.True(t, kinds[euclotypes.ArtifactKindEditIntent])
	require.True(t, kinds[euclotypes.ArtifactKindVerification])

	rootCause, ok := state.Get("euclo.root_cause")
	require.True(t, ok)
	rootCauseMap, ok := rootCause.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "calc.go", rootCauseMap["file"])
}

type regressionQueueModel struct {
	responses []string
	fallback  core.LanguageModel
}

func (m *regressionQueueModel) Generate(ctx context.Context, prompt string, opts *core.LLMOptions) (*core.LLMResponse, error) {
	if len(m.responses) > 0 {
		response := m.responses[0]
		m.responses = m.responses[1:]
		return &core.LLMResponse{Text: response}, nil
	}
	return m.fallback.Generate(ctx, prompt, opts)
}

func (m *regressionQueueModel) GenerateStream(ctx context.Context, prompt string, opts *core.LLMOptions) (<-chan string, error) {
	return m.fallback.GenerateStream(ctx, prompt, opts)
}

func (m *regressionQueueModel) Chat(ctx context.Context, messages []core.Message, opts *core.LLMOptions) (*core.LLMResponse, error) {
	return m.fallback.Chat(ctx, messages, opts)
}

func (m *regressionQueueModel) ChatWithTools(ctx context.Context, messages []core.Message, tools []core.LLMToolSpec, opts *core.LLMOptions) (*core.LLMResponse, error) {
	return m.fallback.ChatWithTools(ctx, messages, tools, opts)
}
