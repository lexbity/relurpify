package agentgraph

import (
	"context"
	"sync"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/framework/compiler"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/graphdb"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/framework/persistence"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	"github.com/stretchr/testify/require"
)

type speculationCompilerStub struct {
	mu     sync.Mutex
	delays []time.Duration
	calls  int
	result *compiler.CompilationResult
	record *compiler.CompilationRecord
}

func (s *speculationCompilerStub) Compile(ctx context.Context, request compiler.CompilationRequest) (*compiler.CompilationResult, *compiler.CompilationRecord, error) {
	s.mu.Lock()
	call := s.calls
	s.calls++
	delay := time.Duration(0)
	if call < len(s.delays) {
		delay = s.delays[call]
	} else if len(s.delays) > 0 {
		delay = s.delays[len(s.delays)-1]
	}
	s.mu.Unlock()

	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-timer.C:
		}
	}
	return s.result, s.record, nil
}

func (s *speculationCompilerStub) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func TestSpeculationCache_HitPath(t *testing.T) {
	graph := NewGraph()
	stub := &speculationCompilerStub{
		delays: []time.Duration{20 * time.Millisecond},
		result: &compiler.CompilationResult{
			StreamedRefs: []contextdata.ChunkReference{{ChunkID: "chunk-b", Rank: 1}},
		},
		record: &compiler.CompilationRecord{
			AssemblyMetadata: contextdata.AssemblyMeta{CompilationID: "spec-b"},
		},
	}
	trigger := contextstream.NewTrigger(stub)
	a := &sleepNode{id: "a", delay: 80 * time.Millisecond}
	b := NewContextStreamNode("b", trigger, retrieval.RetrievalQuery{Text: "speculative"}, 64)
	done := NewTerminalNode("done")

	require.NoError(t, graph.AddNode(a))
	require.NoError(t, graph.AddNode(b))
	require.NoError(t, graph.AddNode(done))
	require.NoError(t, graph.AddEdge("a", "b", func(*Result, *contextdata.Envelope) bool { return false }, false))
	require.NoError(t, graph.AddEdge("a", "done", func(*Result, *contextdata.Envelope) bool { return true }, false))
	require.NoError(t, graph.SetStart("a"))

	env := contextdata.NewEnvelope("task", "session")
	start := time.Now()
	_, err := graph.Execute(context.Background(), env)
	require.NoError(t, err)
	require.GreaterOrEqual(t, stub.Calls(), 1)
	require.Less(t, time.Since(start), 200*time.Millisecond)

	_, ok := env.GetWorkingValue("contextstream.speculative_job.b")
	require.True(t, ok)
	_, ok = env.GetWorkingValue("contextstream.speculative.b")
	require.True(t, ok)

	bStart := time.Now()
	result, err := b.Execute(context.Background(), env)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Less(t, time.Since(bStart), 5*time.Millisecond)
	require.Equal(t, 1, stub.Calls())
	ids := env.StreamedChunkIDs()
	require.Contains(t, ids, contextdata.ChunkID("chunk-b"))
}

func TestSpeculationCache_MissPath(t *testing.T) {
	graph := NewGraph()
	stub := &speculationCompilerStub{result: &compiler.CompilationResult{}}
	trigger := contextstream.NewTrigger(stub)
	a := &sleepNode{id: "a", delay: 5 * time.Millisecond}
	noop := &noopNode{id: "noop"}
	done := NewTerminalNode("done")

	require.NoError(t, graph.AddNode(a))
	require.NoError(t, graph.AddNode(noop))
	require.NoError(t, graph.AddNode(done))
	require.NoError(t, graph.AddEdge("a", "noop", func(*Result, *contextdata.Envelope) bool { return true }, false))
	require.NoError(t, graph.AddEdge("noop", "done", func(*Result, *contextdata.Envelope) bool { return true }, false))
	require.NoError(t, graph.SetStart("a"))

	env := contextdata.NewEnvelope("task", "session")
	_, err := graph.Execute(context.Background(), env)
	require.NoError(t, err)
	require.Equal(t, 0, stub.Calls())
	_, ok := env.GetWorkingValue("contextstream.speculative_job.noop")
	require.False(t, ok)
	_ = trigger
}

func TestSpeculationCache_ConditionalEdge_UnusedBranch(t *testing.T) {
	graph := NewGraph()
	stub := &speculationCompilerStub{
		delays: []time.Duration{10 * time.Millisecond},
		result: &compiler.CompilationResult{
			StreamedRefs: []contextdata.ChunkReference{{ChunkID: "chunk-b", Rank: 1}},
		},
		record: &compiler.CompilationRecord{
			AssemblyMetadata: contextdata.AssemblyMeta{CompilationID: "spec-b"},
		},
	}
	trigger := contextstream.NewTrigger(stub)
	a := &sleepNode{id: "a", delay: 30 * time.Millisecond}
	b := NewContextStreamNode("b", trigger, retrieval.RetrievalQuery{Text: "unused"}, 32)
	done := NewTerminalNode("done")

	require.NoError(t, graph.AddNode(a))
	require.NoError(t, graph.AddNode(b))
	require.NoError(t, graph.AddNode(done))
	require.NoError(t, graph.AddEdge("a", "b", func(*Result, *contextdata.Envelope) bool { return false }, false))
	require.NoError(t, graph.AddEdge("a", "done", func(*Result, *contextdata.Envelope) bool { return true }, false))
	require.NoError(t, graph.SetStart("a"))

	env := contextdata.NewEnvelope("task", "session")
	_, err := graph.Execute(context.Background(), env)
	require.NoError(t, err)
	require.Equal(t, 1, stub.Calls())
	_, ok := env.GetWorkingValue("contextstream.job_id")
	require.True(t, ok)
	_, ok = env.GetWorkingValue("contextstream.speculative_job.b")
	require.True(t, ok)
	ids := env.StreamedChunkIDs()
	require.NotContains(t, ids, contextdata.ChunkID("chunk-b"))
}

func TestSpeculationCache_TTLEviction(t *testing.T) {
	cache := NewSpeculationCache(100 * time.Millisecond)
	job := contextstream.NewJob(contextstream.Request{ID: "job-1"})
	cache.Store("node-1", job)
	time.Sleep(200 * time.Millisecond)
	_, ok := cache.Get("node-1")
	require.False(t, ok)
}

func TestSpeculationCache_FallbackOnTimeout(t *testing.T) {
	graph := NewGraph()
	stub := &speculationCompilerStub{
		delays: []time.Duration{300 * time.Millisecond, 0},
		result: &compiler.CompilationResult{
			StreamedRefs: []contextdata.ChunkReference{{ChunkID: "chunk-b", Rank: 1}},
		},
		record: &compiler.CompilationRecord{
			AssemblyMetadata: contextdata.AssemblyMeta{CompilationID: "spec-b"},
		},
	}
	trigger := contextstream.NewTrigger(stub)
	a := &sleepNode{id: "a", delay: 10 * time.Millisecond}
	b := NewContextStreamNode("b", trigger, retrieval.RetrievalQuery{Text: "timeout"}, 32)
	done := NewTerminalNode("done")

	require.NoError(t, graph.AddNode(a))
	require.NoError(t, graph.AddNode(b))
	require.NoError(t, graph.AddNode(done))
	require.NoError(t, graph.AddEdge("a", "b", func(*Result, *contextdata.Envelope) bool { return false }, false))
	require.NoError(t, graph.AddEdge("a", "done", func(*Result, *contextdata.Envelope) bool { return true }, false))
	require.NoError(t, graph.SetStart("a"))

	env := contextdata.NewEnvelope("task", "session")
	_, err := graph.Execute(context.Background(), env)
	require.NoError(t, err)

	start := time.Now()
	result, err := b.Execute(context.Background(), env)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.GreaterOrEqual(t, time.Since(start), 200*time.Millisecond)
	require.Equal(t, 2, stub.Calls())
}

func TestSpeculationCache_NoSideEffects(t *testing.T) {
	engine, err := graphdb.Open(graphdb.DefaultOptions(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close()) })

	store := &knowledge.ChunkStore{Graph: engine}
	eventLog := &countingEventLog{}
	compiler := compiler.NewCompiler(nil, nil, store)
	compiler.SetPersistenceWriter(persistence.NewWriter(store, eventLog, nil))

	trigger := contextstream.NewTrigger(compiler)
	graph := NewGraph()
	a := &sleepNode{id: "a", delay: 30 * time.Millisecond}
	b := NewContextStreamNode("b", trigger, retrieval.RetrievalQuery{Text: "side effects"}, 32)
	done := NewTerminalNode("done")

	require.NoError(t, graph.AddNode(a))
	require.NoError(t, graph.AddNode(b))
	require.NoError(t, graph.AddNode(done))
	require.NoError(t, graph.AddEdge("a", "b", func(*Result, *contextdata.Envelope) bool { return false }, false))
	require.NoError(t, graph.AddEdge("a", "done", func(*Result, *contextdata.Envelope) bool { return true }, false))
	require.NoError(t, graph.SetStart("a"))

	before, err := store.FindAll()
	require.NoError(t, err)

	env := contextdata.NewEnvelope("task", "session")
	_, err = graph.Execute(context.Background(), env)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		after, err := store.FindAll()
		if err != nil {
			return false
		}
		return len(after) == len(before) && eventLog.Count() == 0
	}, 250*time.Millisecond, 10*time.Millisecond)
}

func TestGraph_SpeculativeLookaheadDepth2(t *testing.T) {
	graph := NewGraph()
	stub := &speculationCompilerStub{
		delays: []time.Duration{20 * time.Millisecond},
		result: &compiler.CompilationResult{
			StreamedRefs: []contextdata.ChunkReference{{ChunkID: "chunk-c", Rank: 1}},
		},
		record: &compiler.CompilationRecord{
			AssemblyMetadata: contextdata.AssemblyMeta{CompilationID: "spec-c"},
		},
	}
	trigger := contextstream.NewTrigger(stub)
	a := &sleepNode{id: "a", delay: 40 * time.Millisecond}
	b := &noopNode{id: "b"}
	c := NewContextStreamNode("c", trigger, retrieval.RetrievalQuery{Text: "depth-two"}, 32)

	require.NoError(t, graph.AddNode(a))
	require.NoError(t, graph.AddNode(b))
	require.NoError(t, graph.AddNode(c))
	require.NoError(t, graph.AddEdge("a", "b", func(*Result, *contextdata.Envelope) bool { return true }, false))
	require.NoError(t, graph.AddEdge("b", "c", func(*Result, *contextdata.Envelope) bool { return true }, false))
	require.NoError(t, graph.SetStart("a"))

	env := contextdata.NewEnvelope("task", "session")
	_, err := graph.Execute(context.Background(), env)
	require.NoError(t, err)
	require.Equal(t, 1, stub.Calls())
	require.Contains(t, env.StreamedChunkIDs(), contextdata.ChunkID("chunk-c"))
}

type sleepNode struct {
	id    string
	delay time.Duration
}

func (n *sleepNode) ID() string { return n.id }

func (n *sleepNode) Type() NodeType { return NodeTypeSystem }

func (n *sleepNode) Execute(ctx context.Context, env *contextdata.Envelope) (*Result, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(n.delay):
	}
	return &Result{NodeID: n.id, Success: true, Data: map[string]any{}}, nil
}

type noopNode struct {
	id string
}

func (n *noopNode) ID() string { return n.id }

func (n *noopNode) Type() NodeType { return NodeTypeSystem }

func (n *noopNode) Execute(ctx context.Context, env *contextdata.Envelope) (*Result, error) {
	_ = ctx
	_ = env
	return &Result{NodeID: n.id, Success: true, Data: map[string]any{}}, nil
}

type countingEventLog struct {
	mu    sync.Mutex
	count int
}

func (l *countingEventLog) Emit(eventType string, payload map[string]any) {
	_ = eventType
	_ = payload
	l.mu.Lock()
	l.count++
	l.mu.Unlock()
}

func (l *countingEventLog) Count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.count
}
