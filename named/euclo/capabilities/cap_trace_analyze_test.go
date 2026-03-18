package capabilities

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestTraceAnalyzeDescriptorAndEligibility(t *testing.T) {
	cap := &traceAnalyzeCapability{env: testEnv(t)}
	desc := cap.Descriptor()
	require.Equal(t, "euclo:trace.analyze", desc.ID)
	require.Contains(t, desc.Tags, "trace")

	eligible := cap.Eligible(euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindIntake, Payload: map[string]any{"instruction": "Show trace for this failing test"}},
	}), euclotypes.CapabilitySnapshot{HasExecuteTools: true})
	require.True(t, eligible.Eligible)

	ineligible := cap.Eligible(euclotypes.NewArtifactState([]euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindIntake, Payload: map[string]any{"instruction": "Debug this failing test"}},
	}), euclotypes.CapabilitySnapshot{HasExecuteTools: true})
	require.False(t, ineligible.Eligible)
}

func TestTraceAnalyzeExecuteProducesTraceAndAnalysisArtifacts(t *testing.T) {
	env := testEnv(t)
	env.Model = &traceQueueModel{
		responses: []string{
			`{"facts":[{"key":"trace:raw_output","value":"TRACE start\nTRACE call Multiply\nTRACE end"}],"summary":"trace collected"}`,
			`{"facts":[{"key":"trace:analysis","value":{"call_chain":[{"function":"Multiply","location":"calc.go:3"}],"hot_paths":[{"path":"Multiply","count":1}],"anomalies":[{"description":"unexpected add branch","severity":"high"}],"timing":{"slowest_path":"Multiply"}}}],"summary":"trace analyzed"}`,
			`{"facts":[{"key":"trace:correlations","value":[{"location":"calc.go:3","finding":"unexpected add branch","assessment":"likely regression in multiply logic","severity":"high"}]}],"summary":"trace correlated"}`,
		},
		fallback: testutil.StubModel{},
	}
	cap := &traceAnalyzeCapability{env: env}
	state := core.NewContext()
	state.Set("euclo.envelope", map[string]any{"instruction": "Show trace for this failing test"})

	result := cap.Execute(context.Background(), euclotypes.ExecutionEnvelope{
		Task:        &core.Task{ID: "trace-1", Instruction: "Show trace for this failing test"},
		Mode:        euclotypes.ModeResolution{ModeID: "debug"},
		Profile:     euclotypes.ExecutionProfileSelection{ProfileID: "trace_execute_analyze"},
		Registry:    env.Registry,
		State:       state,
		Memory:      env.Memory,
		Environment: env,
	})

	require.Equal(t, euclotypes.ExecutionStatusCompleted, result.Status)
	require.Len(t, result.Artifacts, 2)
	require.Equal(t, euclotypes.ArtifactKindTrace, result.Artifacts[0].Kind)
	require.Equal(t, euclotypes.ArtifactKindAnalyze, result.Artifacts[1].Kind)

	tracePayload, ok := result.Artifacts[0].Payload.(map[string]any)
	require.True(t, ok)
	require.Contains(t, tracePayload["raw_output"], "TRACE call Multiply")

	analysisPayload, ok := result.Artifacts[1].Payload.(map[string]any)
	require.True(t, ok)
	require.NotEmpty(t, analysisPayload["correlations"])
}

type traceQueueModel struct {
	responses []string
	fallback  core.LanguageModel
}

func (m *traceQueueModel) Generate(ctx context.Context, prompt string, opts *core.LLMOptions) (*core.LLMResponse, error) {
	if len(m.responses) > 0 {
		response := m.responses[0]
		m.responses = m.responses[1:]
		return &core.LLMResponse{Text: response}, nil
	}
	return m.fallback.Generate(ctx, prompt, opts)
}

func (m *traceQueueModel) GenerateStream(ctx context.Context, prompt string, opts *core.LLMOptions) (<-chan string, error) {
	return m.fallback.GenerateStream(ctx, prompt, opts)
}

func (m *traceQueueModel) Chat(ctx context.Context, messages []core.Message, opts *core.LLMOptions) (*core.LLMResponse, error) {
	return m.fallback.Chat(ctx, messages, opts)
}

func (m *traceQueueModel) ChatWithTools(ctx context.Context, messages []core.Message, tools []core.LLMToolSpec, opts *core.LLMOptions) (*core.LLMResponse, error) {
	return m.fallback.ChatWithTools(ctx, messages, tools, opts)
}
