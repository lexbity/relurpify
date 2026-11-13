package llm

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/model"
	"github.com/stretchr/testify/require"
)

// recordingModel captures the context it receives and returns canned results.
type recordingModel struct {
	mu       sync.Mutex
	contexts []context.Context
	err      error
	tokens   []string
	block    bool
}

func (m *recordingModel) record(ctx context.Context) {
	m.mu.Lock()
	m.contexts = append(m.contexts, ctx)
	m.mu.Unlock()
}

func (m *recordingModel) Generate(ctx context.Context, _ string, _ *model.LLMOptions) (*model.LLMResponse, error) {
	m.record(ctx)
	if m.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if m.err != nil {
		return nil, m.err
	}
	return &model.LLMResponse{Text: "ok"}, nil
}

func (m *recordingModel) GenerateStream(ctx context.Context, _ string, _ *model.LLMOptions) (<-chan string, error) {
	m.record(ctx)
	if m.err != nil {
		return nil, m.err
	}
	out := make(chan string, len(m.tokens))
	for _, token := range m.tokens {
		out <- token
	}
	close(out)
	return out, nil
}

func (m *recordingModel) Chat(ctx context.Context, _ []model.Message, _ *model.LLMOptions) (*model.LLMResponse, error) {
	return m.Generate(ctx, "", nil)
}

func (m *recordingModel) ChatWithTools(ctx context.Context, _ []model.Message, _ []model.LLMToolSpec, _ *model.LLMOptions) (*model.LLMResponse, error) {
	return m.Generate(ctx, "", nil)
}

func TestNewGatedModelNilInputs(t *testing.T) {
	t.Parallel()
	inner := &recordingModel{}
	require.Same(t, inner, NewGatedModel(inner, nil))
	require.Nil(t, NewGatedModel(nil, model.NewDefaultCallGate()))
}

func TestGatedModelMethodsUseLane(t *testing.T) {
	t.Parallel()
	gate := model.NewDefaultCallGate()
	inner := &recordingModel{}
	gated := NewGatedModel(inner, gate)

	ctx := context.Background()
	_, err := gated.Generate(ctx, "prompt", nil)
	require.NoError(t, err)
	_, err = gated.Chat(ctx, []model.Message{{Role: "user", Content: "hi"}}, nil)
	require.NoError(t, err)
	_, err = gated.ChatWithTools(ctx, []model.Message{{Role: "user", Content: "hi"}}, nil, nil)
	require.NoError(t, err)

	require.Equal(t, int64(3), gate.TotalGranted())
	require.Equal(t, int64(3), gate.TotalReclaimed())
	require.Len(t, inner.contexts, 3)
	for _, got := range inner.contexts {
		require.NotEqual(t, ctx, got, "the provider must receive the ticket context")
	}
}

func TestGatedModelStreamReleasesAfterClose(t *testing.T) {
	t.Parallel()
	gate := model.NewDefaultCallGate()
	inner := &recordingModel{tokens: []string{"a", "b"}}
	gated := NewGatedModel(inner, gate)

	stream, err := gated.GenerateStream(context.Background(), "prompt", nil)
	require.NoError(t, err)
	var got []string
	for token := range stream {
		got = append(got, token)
	}
	require.Equal(t, []string{"a", "b"}, got)
	require.Equal(t, int64(1), gate.TotalGranted())
	require.Equal(t, int64(1), gate.TotalReclaimed())
}

func TestGatedModelStreamOpenErrorReleases(t *testing.T) {
	t.Parallel()
	gate := model.NewDefaultCallGate()
	inner := &recordingModel{err: errors.New("open failed")}
	gated := NewGatedModel(inner, gate)

	_, err := gated.GenerateStream(context.Background(), "prompt", nil)
	require.ErrorContains(t, err, "open failed")
	require.Equal(t, int64(1), gate.TotalGranted())
	require.Equal(t, int64(1), gate.TotalReclaimed())
}

func TestGatedModelActiveTimeoutIsClassified(t *testing.T) {
	t.Parallel()
	gate := model.NewCallGate(model.GateConfig{
		QueueCapacity:           4,
		QueueTimeout:            time.Second,
		ActiveTimeout:           20 * time.Millisecond,
		CancellationGracePeriod: 20 * time.Millisecond,
	})
	inner := &recordingModel{block: true}
	gated := NewGatedModel(inner, gate)

	_, err := gated.Generate(context.Background(), "prompt", nil)
	require.ErrorIs(t, err, model.ErrActiveTimeout)
	require.Equal(t, int64(1), gate.TotalReclaimed())
}

func TestGatedModelRejectsNestedCall(t *testing.T) {
	t.Parallel()
	gate := model.NewDefaultCallGate()
	nested := &nestedModel{}
	gated := NewGatedModel(nested, gate)
	nested.inner = gated

	_, err := gated.Generate(context.Background(), "prompt", nil)
	require.ErrorIs(t, err, model.ErrNestedModelCall)
	require.Equal(t, int64(1), gate.TotalGranted())
	require.Equal(t, int64(1), gate.TotalReclaimed())
}

// nestedModel calls another model from inside a generation, as a repair or
// routing path would.
type nestedModel struct {
	inner model.LanguageModel
}

func (m *nestedModel) Generate(ctx context.Context, _ string, _ *model.LLMOptions) (*model.LLMResponse, error) {
	_, err := m.inner.Generate(ctx, "nested", nil)
	return nil, err
}

func (m *nestedModel) GenerateStream(ctx context.Context, _ string, _ *model.LLMOptions) (<-chan string, error) {
	return nil, errors.New("unused")
}

func (m *nestedModel) Chat(context.Context, []model.Message, *model.LLMOptions) (*model.LLMResponse, error) {
	return nil, errors.New("unused")
}

func (m *nestedModel) ChatWithTools(context.Context, []model.Message, []model.LLMToolSpec, *model.LLMOptions) (*model.LLMResponse, error) {
	return nil, errors.New("unused")
}

type controlledStreamModel struct {
	recordingModel
	source chan string
}

func (m *controlledStreamModel) GenerateStream(context.Context, string, *model.LLMOptions) (<-chan string, error) {
	return m.source, nil
}

func TestV2GateStreamCancellationWaitsForProducerReclamation(t *testing.T) {
	gate := model.NewCallGate(model.GateConfig{CancellationGracePeriod: 100 * time.Millisecond})
	inner := &controlledStreamModel{source: make(chan string)}
	ctx, cancel := context.WithCancel(context.Background())
	out, err := NewGatedModel(inner, gate).GenerateStream(ctx, "request", nil)
	require.NoError(t, err)
	cancel()
	require.True(t, gate.State().Active)
	close(inner.source)
	select {
	case _, ok := <-out:
		require.False(t, ok)
	case <-time.After(time.Second):
		t.Fatal("stream failed to close")
	}
	require.False(t, gate.State().Active)
	require.False(t, gate.State().Poisoned)
	require.EqualValues(t, 1, gate.TotalReclaimed())
}

func TestV2GateStreamUnreclaimedProducerPoisonsLane(t *testing.T) {
	gate := model.NewCallGate(model.GateConfig{CancellationGracePeriod: 10 * time.Millisecond})
	inner := &controlledStreamModel{source: make(chan string)}
	defer close(inner.source)
	ctx, cancel := context.WithCancel(context.Background())
	out, err := NewGatedModel(inner, gate).GenerateStream(ctx, "request", nil)
	require.NoError(t, err)
	cancel()
	select {
	case _, ok := <-out:
		require.False(t, ok)
	case <-time.After(time.Second):
		t.Fatal("stream cancellation hung")
	}
	require.True(t, gate.State().Poisoned)
	require.True(t, gate.State().Active)
	require.EqualValues(t, 0, gate.TotalReclaimed(), "abandoned stream was falsely acknowledged as reclaimed")
	require.True(t, gate.Recover().Poisoned)
	_, err = gate.Acquire(context.Background(), "next", "attempt")
	require.ErrorIs(t, err, model.ErrGatePoisoned)
}

type profiledRecordingModel struct {
	recordingModel
	profile *model.ModelProfile
}

func (m *profiledRecordingModel) SetProfile(profile *model.ModelProfile) { m.profile = profile }
func (m *profiledRecordingModel) ToolRepairStrategy() string             { return "native-only" }
func (m *profiledRecordingModel) MaxToolsPerCall() int                   { return 7 }
func (m *profiledRecordingModel) UsesNativeToolCalling() bool            { return true }

func TestV2GatePreservesFullProfileContract(t *testing.T) {
	inner := &profiledRecordingModel{}
	gated := NewGatedModel(inner, model.NewDefaultCallGate())
	profiled, ok := gated.(model.ProfiledModel)
	require.True(t, ok)
	require.Equal(t, "native-only", profiled.ToolRepairStrategy())
	require.Equal(t, 7, profiled.MaxToolsPerCall())
	require.True(t, profiled.UsesNativeToolCalling())
	profile := &model.ModelProfile{}
	require.True(t, ApplyProfile(gated, profile))
	require.Equal(t, profile, inner.profile)
}
