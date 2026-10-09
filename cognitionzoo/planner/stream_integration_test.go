package planner

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/model"
)

type plannerStreamCompilerStub struct {
	mu      sync.Mutex
	request contextports.CompilationRequest
	result  *contextports.CompilationResult
}

func (s *plannerStreamCompilerStub) Compile(ctx context.Context, request contextports.CompilationRequest) (*contextports.CompilationResult, error) {
	s.mu.Lock()
	s.request = request
	s.mu.Unlock()
	return s.result, nil
}

type plannerModelStub struct {
	mu       sync.Mutex
	prompts  []string
	response string
}

func (m *plannerModelStub) Generate(ctx context.Context, prompt string, options *model.LLMOptions) (*model.LLMResponse, error) {
	m.mu.Lock()
	m.prompts = append(m.prompts, prompt)
	m.mu.Unlock()
	return &model.LLMResponse{Text: m.response}, nil
}

func (m *plannerModelStub) GenerateStream(ctx context.Context, prompt string, options *model.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (m *plannerModelStub) Chat(ctx context.Context, messages []model.Message, options *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: m.response}, nil
}

func (m *plannerModelStub) ChatWithTools(ctx context.Context, messages []model.Message, tools []model.LLMToolSpec, options *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: m.response}, nil
}

func TestPlannerExecuteBlockingContextStreamAppliesTrimmedMetadataBeforePlanning(t *testing.T) {
	compilerStub := &plannerStreamCompilerStub{
		result: &contextports.CompilationResult{
			StreamedRefs:    []string{"chunk-1"},
			ShortfallTokens: 7,
			Record: contextports.CompilationRecord{
				ID: "comp-1",
			},
		},
	}
	model := &plannerModelStub{
		response: `{"goal":"demo","steps":[{"id":"step-1","description":"collect context","tool":"","params":{}}],"dependencies":{},"files":[]}`,
	}
	agent := &PlannerAgent{
		Model:           model,
		Tools:           nil,
		Config:          &execution.Config{},
		StreamMode:      contextstream.ModeBlocking,
		StreamMaxTokens: 128,
		StreamQuery:     "workspace query",
	}

	env := contextdata.NewEnvelope("task-1", "session-1")
	task := &execution.Task{ID: "task-1", Instruction: "build a plan"}

	ctx := contextstream.WithTrigger(context.Background(), contextstream.NewTrigger(compilerStub))
	result, err := agent.Execute(ctx, task, env)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Equal(t, []contextdata.ChunkID{"chunk-1"}, env.StreamedChunkIDs())
	shortfall, ok := contextdata.GetTyped[int](env, "contextstream.shortfall_tokens")
	require.True(t, ok)
	require.Equal(t, 7, shortfall)
	trimmed, ok := contextdata.GetTyped[bool](env, "contextstream.trimmed")
	require.True(t, ok)
	require.True(t, trimmed)

	compilerStub.mu.Lock()
	request := compilerStub.request
	compilerStub.mu.Unlock()
	require.Equal(t, "workspace query", request.BaseContext)
	require.Equal(t, 128, request.BudgetTokens)

	model.mu.Lock()
	prompt := strings.Join(model.prompts, "\n")
	model.mu.Unlock()
	require.Contains(t, prompt, "Streaming note: context was trimmed to fit budget")
}

func TestPlannerExecuteBackgroundContextStreamPublishesJobMetadata(t *testing.T) {
	compilerStub := &plannerStreamCompilerStub{
		result: &contextports.CompilationResult{
			StreamedRefs: []string{"chunk-2"},
			Record: contextports.CompilationRecord{
				ID: "comp-2",
			},
		},
	}
	model := &plannerModelStub{
		response: `{"goal":"demo","steps":[{"id":"step-1","description":"collect context","tool":"","params":{}}],"dependencies":{},"files":[]}`,
	}
	agent := &PlannerAgent{
		Model:           model,
		Config:          &execution.Config{},
		StreamMode:      contextstream.ModeBackground,
		StreamMaxTokens: 64,
		StreamQuery:     "background query",
	}

	env := contextdata.NewEnvelope("task-2", "session-2")
	task := &execution.Task{ID: "task-2", Instruction: "build a plan"}

	ctx := contextstream.WithTrigger(context.Background(), contextstream.NewTrigger(compilerStub))
	// The epoch coordinator owns background stream jobs; the planner's graph
	// shares it and lands the job at its node barrier.
	runCtx := contextdata.WithEnvelope(ctx, env)
	ctx = agentgraph.WithEpochCoordinator(ctx, agentgraph.NewEpochCoordinator(runCtx, plannerNoopGrounder{}, nil))

	result, err := agent.Execute(ctx, task, env)
	require.NoError(t, err)
	require.NotNil(t, result)

	jobID, ok := contextdata.GetTyped[string](env, "contextstream.job_id")
	require.True(t, ok)
	require.NotEmpty(t, jobID)
	require.Equal(t, "background", envGetString(env, "contextstream.job_mode"))

	// The barrier lands the completed job before the run returns.
	require.Equal(t, []contextdata.ChunkID{"chunk-2"}, env.StreamedChunkIDs())
}

type plannerNoopGrounder struct{}

func (plannerNoopGrounder) Ground(_ context.Context, _ []knowledge.GroundingItem) (knowledge.GroundingReport, error) {
	return knowledge.GroundingReport{}, nil
}
