package planner

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/compiler"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"github.com/stretchr/testify/require"
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

func (m *plannerModelStub) Generate(ctx context.Context, prompt string, options *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	m.mu.Lock()
	m.prompts = append(m.prompts, prompt)
	m.mu.Unlock()
	return &contracts.LLMResponse{Text: m.response}, nil
}

func (m *plannerModelStub) GenerateStream(ctx context.Context, prompt string, options *contracts.LLMOptions) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (m *plannerModelStub) Chat(ctx context.Context, messages []contracts.Message, options *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	return &contracts.LLMResponse{Text: m.response}, nil
}

func (m *plannerModelStub) ChatWithTools(ctx context.Context, messages []contracts.Message, tools []contracts.LLMToolSpec, options *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	return &contracts.LLMResponse{Text: m.response}, nil
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
	trigger := contextstream.NewTrigger(compilerStub)
	agent := &PlannerAgent{
		Model:           model,
		Tools:           nil,
		Config:          &core.Config{},
		StreamTrigger:   trigger,
		StreamMode:      contextstream.ModeBlocking,
		StreamMaxTokens: 128,
		StreamQuery:     "workspace query",
	}

	env := contextdata.NewEnvelope("task-1", "session-1")
	task := &core.Task{ID: "task-1", Instruction: "build a plan"}

	result, err := agent.Execute(context.Background(), task, env)
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
		Config:          &core.Config{},
		StreamTrigger:   contextstream.NewTrigger(compilerStub),
		StreamMode:      contextstream.ModeBackground,
		StreamMaxTokens: 64,
		StreamQuery:     "background query",
	}

	env := contextdata.NewEnvelope("task-2", "session-2")
	task := &core.Task{ID: "task-2", Instruction: "build a plan"}

	result, err := agent.Execute(context.Background(), task, env)
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
