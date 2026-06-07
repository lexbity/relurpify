package planner

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/compiler"
	"codeburg.org/lexbit/relurpify/model"
)

type plannerStreamCompilerStub struct {
	mu      sync.Mutex
	request compiler.CompilationRequest
	result  *compiler.CompilationResult
	record  *compiler.CompilationRecord
}

func (s *plannerStreamCompilerStub) Compile(ctx context.Context, request compiler.CompilationRequest) (*compiler.CompilationResult, *compiler.CompilationRecord, error) {
	s.mu.Lock()
	s.request = request
	s.mu.Unlock()
	return s.result, s.record, nil
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

func (m *plannerModelStub) ChatWithTools(ctx context.Context, messages []model.Message, tools []ports.LLMToolSpec, options *model.LLMOptions) (*model.LLMResponse, error) {
	return &model.LLMResponse{Text: m.response}, nil
}

func TestPlannerExecuteBlockingContextStreamAppliesTrimmedMetadataBeforePlanning(t *testing.T) {
	compilerStub := &plannerStreamCompilerStub{
		result: &compiler.CompilationResult{
			StreamedRefs:    []contextdata.ChunkReference{{ChunkID: "chunk-1", Rank: 1}},
			ShortfallTokens: 7,
		},
		record: &compiler.CompilationRecord{
			AssemblyMetadata: contextdata.AssemblyMeta{CompilationID: "comp-1"},
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
	shortfall, ok := env.GetWorkingValue("contextstream.shortfall_tokens")
	require.True(t, ok)
	require.Equal(t, 7, shortfall)
	trimmed, ok := env.GetWorkingValue("contextstream.trimmed")
	require.True(t, ok)
	require.Equal(t, true, trimmed)

	compilerStub.mu.Lock()
	request := compilerStub.request
	compilerStub.mu.Unlock()
	require.Equal(t, "workspace query", request.Query.Text)
	require.Equal(t, 128, request.MaxTokens)

	model.mu.Lock()
	prompt := strings.Join(model.prompts, "\n")
	model.mu.Unlock()
	require.Contains(t, prompt, "Streaming note: context was trimmed to fit budget")
}

func TestPlannerExecuteBackgroundContextStreamPublishesJobMetadata(t *testing.T) {
	compilerStub := &plannerStreamCompilerStub{
		result: &compiler.CompilationResult{
			StreamedRefs: []contextdata.ChunkReference{{ChunkID: "chunk-2", Rank: 1}},
		},
		record: &compiler.CompilationRecord{
			AssemblyMetadata: contextdata.AssemblyMeta{CompilationID: "comp-2"},
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
	result, err := agent.Execute(ctx, task, env)
	require.NoError(t, err)
	require.NotNil(t, result)

	jobID, ok := env.GetWorkingValue("contextstream.job_id")
	require.True(t, ok)
	require.NotEmpty(t, jobID)
	require.Equal(t, "background", envGetString(env, "contextstream.job_mode"))

	require.Eventually(t, func() bool {
		ids := env.StreamedChunkIDs()
		return len(ids) == 1 && ids[0] == "chunk-2"
	}, time.Second, 10*time.Millisecond)
}
