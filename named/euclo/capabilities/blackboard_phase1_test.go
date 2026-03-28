package capabilities

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/agents/blackboard"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/testutil/euclotestutil"
	"github.com/stretchr/testify/require"
)

func TestBlackboardArtifactBridgeRoundTrip(t *testing.T) {
	board := blackboard.NewBlackboard("debug the failing test")
	bridge := NewBlackboardArtifactBridge(board)

	artifacts := euclotypes.NewArtifactState([]euclotypes.Artifact{
		{
			ID:      "explore",
			Kind:    euclotypes.ArtifactKindExplore,
			Payload: map[string]any{"files": []string{"main.go"}, "status": "failing"},
		},
		{
			ID:      "verification",
			Kind:    euclotypes.ArtifactKindVerification,
			Payload: map[string]any{"passed": false, "command": "go test ./..."},
		},
	})

	require.NoError(t, bridge.SeedFromArtifacts(artifacts))

	explorePayload, ok := boardEntryValue(board, "explore:workspace_state")
	require.True(t, ok)
	require.Equal(t, "failing", explorePayload.(map[string]any)["status"])

	harvested := bridge.HarvestToArtifacts()
	require.Len(t, harvested, 2)
	require.Contains(t, artifactKinds(harvested), euclotypes.ArtifactKindExplore)
	require.Contains(t, artifactKinds(harvested), euclotypes.ArtifactKindVerification)
}

func TestKnowledgeSourceTemplatesExposeSelectorsAndPredicates(t *testing.T) {
	analysis := NewAnalysisKnowledgeSource("Diff Analyst", "always", []string{"cli_git", "file_read"}, "Goal: {{goal}}")
	resolved := blackboard.ResolveKnowledgeSource(analysis)
	require.Equal(t, "Diff Analyst", resolved.Spec.Name)
	require.Len(t, resolved.Spec.RequiredCapabilities, 2)
	require.Equal(t, "cli_git", resolved.Spec.RequiredCapabilities[0].ID)
	require.Equal(t, "file_read", resolved.Spec.RequiredCapabilities[1].ID)

	synthesis := NewSynthesisKnowledgeSource("Root Cause Synth", "", []string{"analysis:ready", "analysis:candidate"}, "Inputs: {{input_entries}}")
	board := blackboard.NewBlackboard("find root cause")
	require.False(t, synthesis.CanActivate(board))

	require.True(t, setBoardEntry(board, "analysis:ready", true, "test"))
	require.False(t, synthesis.CanActivate(board))

	require.True(t, setBoardEntry(board, "analysis:candidate", map[string]any{"file": "main.go"}, "test"))
	require.True(t, synthesis.CanActivate(board))

	mutation := NewMutationKnowledgeSource("Fix Draft", "analysis:ready exists", []string{"file_write"}, "Entries: {{entries}}")
	mutationResolved := blackboard.ResolveKnowledgeSource(mutation)
	require.Equal(t, "human", string(mutationResolved.Spec.Contract.SideEffectClass))
}

func TestExecuteBlackboardRunsKnowledgeSourcesInOrderAndHarvestsArtifacts(t *testing.T) {
	state := core.NewContext()
	state.Set("pipeline.explore", map[string]any{"files": []string{"buggy.go"}})

	model := &queueModel{
		responses: []string{
			`{"facts":[{"key":"analysis:ready","value":{"suspect":"buggy.go"}}],"summary":"analysis ready"}`,
			`{"artifacts":[{"id":"root-cause","kind":"debug:root_cause","content":{"file":"buggy.go","reason":"missing nil check"}}],"summary":"root cause found"}`,
			`{"facts":[{"key":"verify:result","value":{"passed":true}}],"summary":"verification complete"}`,
		},
	}
	env := testutil.EnvMinimal()
	env.Model = model

	result, err := ExecuteBlackboard(context.Background(), euclotypes.ExecutionEnvelope{
		Task:        &core.Task{ID: "bb-1", Instruction: "Investigate the regression"},
		State:       state,
		Registry:    env.Registry,
		Environment: env,
	}, []blackboard.KnowledgeSource{
		NewAnalysisKnowledgeSource("Analyze", "not analysis:ready exists", []string{"file_read"}, "Analyze {{entries}}"),
		NewSynthesisKnowledgeSource("RootCause", "not debug:root_cause exists", []string{"analysis:ready"}, "Synthesize {{input_entries}}"),
		NewSynthesisKnowledgeSource("Verify", "debug:root_cause exists", []string{"debug:root_cause"}, "Verify {{input_entries}}"),
	}, 6, func(bb *blackboard.Blackboard) bool {
		return boardHasEntry(bb, "verify:result")
	})

	require.NoError(t, err)
	require.Equal(t, "predicate_satisfied", result.Termination)
	require.Equal(t, 3, result.Cycles)
	require.Equal(t, "Verify", result.LastSource)
	require.Len(t, model.prompts, 3)

	kinds := artifactKinds(result.Artifacts)
	require.Contains(t, kinds, euclotypes.ArtifactKindExplore)
	require.Contains(t, kinds, euclotypes.ArtifactKindRootCause)
	require.Contains(t, kinds, euclotypes.ArtifactKindVerification)

	rootCause, ok := state.Get("euclo.root_cause")
	require.True(t, ok)
	rootCauseMap, ok := rootCause.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "buggy.go", rootCauseMap["file"])
}

func artifactKinds(artifacts []euclotypes.Artifact) []euclotypes.ArtifactKind {
	out := make([]euclotypes.ArtifactKind, 0, len(artifacts))
	for _, artifact := range artifacts {
		out = append(out, artifact.Kind)
	}
	return out
}

type queueModel struct {
	responses []string
	prompts   []string
}

func (m *queueModel) Generate(_ context.Context, prompt string, _ *core.LLMOptions) (*core.LLMResponse, error) {
	m.prompts = append(m.prompts, prompt)
	response := `{}`
	if len(m.responses) > 0 {
		response = m.responses[0]
		m.responses = m.responses[1:]
	}
	return &core.LLMResponse{Text: response}, nil
}

func (m *queueModel) GenerateStream(context.Context, string, *core.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (m *queueModel) Chat(context.Context, []core.Message, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: `{"summary":"ok"}`}, nil
}

func (m *queueModel) ChatWithTools(context.Context, []core.Message, []core.LLMToolSpec, *core.LLMOptions) (*core.LLMResponse, error) {
	return &core.LLMResponse{Text: `{"summary":"ok"}`}, nil
}
