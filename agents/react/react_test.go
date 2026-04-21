package react

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextmgr"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/graph"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/memory/db"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	frameworksearch "codeburg.org/lexbit/relurpify/framework/search"
	"github.com/stretchr/testify/assert"
)

type stubLLM struct {
	responses      []*core.LLMResponse
	idx            int
	generateCalls  int
	withToolsCalls int
	toolMessages   [][]core.Message
}

type recordingTelemetry struct {
	events []core.Event
}

func boolPtr(value bool) *bool { return &value }

func (r *recordingTelemetry) Emit(event core.Event) {
	r.events = append(r.events, event)
}

// Generate returns the next queued LLM response for deterministic tests.
func (s *stubLLM) Generate(ctx context.Context, prompt string, options *core.LLMOptions) (*core.LLMResponse, error) {
	s.generateCalls++
	return s.nextResponse()
}

// GenerateStream is unused in tests.
func (s *stubLLM) GenerateStream(ctx context.Context, prompt string, options *core.LLMOptions) (<-chan string, error) {
	return nil, errors.New("not implemented")
}

// Chat is unused in tests.
func (s *stubLLM) Chat(ctx context.Context, messages []core.Message, options *core.LLMOptions) (*core.LLMResponse, error) {
	return nil, errors.New("not implemented")
}

// ChatWithTools returns the next queued response and increments instrumentation.
func (s *stubLLM) ChatWithTools(ctx context.Context, messages []core.Message, tools []core.LLMToolSpec, options *core.LLMOptions) (*core.LLMResponse, error) {
	s.withToolsCalls++
	copyMessages := make([]core.Message, len(messages))
	copy(copyMessages, messages)
	s.toolMessages = append(s.toolMessages, copyMessages)
	return s.nextResponse()
}

// nextResponse pops the next canned response or returns an error when empty.
func (s *stubLLM) nextResponse() (*core.LLMResponse, error) {
	if s.idx >= len(s.responses) {
		return nil, errors.New("no response")
	}
	resp := s.responses[s.idx]
	s.idx++
	return resp, nil
}

type stubTool struct {
	name   string
	tags   []string
	params []core.ToolParameter
}

type fileWriteStubTool struct{}

// Name returns the tool identifier used in tool calls.
func (t stubTool) Name() string { return t.name }

// Description provides a friendly label for CLI output.
func (t stubTool) Description() string { return "stub tool" }

// Category groups the tool in mock registries.
func (t stubTool) Category() string { return "test" }

// Parameters exposes the single optional argument accepted by the stub.
func (t stubTool) Parameters() []core.ToolParameter {
	if t.params != nil {
		return t.params
	}
	return []core.ToolParameter{
		{Name: "value", Type: "string", Required: false},
	}
}

// Execute echoes the provided "value" argument to simulate tool output.
func (t stubTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	return &core.ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"echo": args["value"],
		},
	}, nil
}

// IsAvailable always returns true for simplicity.
func (t stubTool) IsAvailable(ctx context.Context, state *core.Context) bool { return true }

// Permissions returns a read-only filesystem grant.
func (t stubTool) Permissions() core.ToolPermissions {
	return core.ToolPermissions{Permissions: &core.PermissionSet{
		FileSystem: []core.FileSystemPermission{
			{Action: core.FileSystemRead, Path: "**"},
		},
	}}
}

// Tags returns nil as the stub tool has no tags.
func (t stubTool) Tags() []string { return t.tags }

func (fileWriteStubTool) Name() string        { return "file_write" }
func (fileWriteStubTool) Description() string { return "writes a file" }
func (fileWriteStubTool) Category() string    { return "file" }
func (fileWriteStubTool) Parameters() []core.ToolParameter {
	return []core.ToolParameter{
		{Name: "path", Type: "string", Required: true},
		{Name: "content", Type: "string", Required: true},
	}
}
func (fileWriteStubTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	path := fmt.Sprint(args["path"])
	content := fmt.Sprint(args["content"])
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return nil, err
	}
	return &core.ToolResult{Success: true, Data: map[string]interface{}{"path": path}}, nil
}
func (fileWriteStubTool) IsAvailable(ctx context.Context, state *core.Context) bool { return true }
func (fileWriteStubTool) Permissions() core.ToolPermissions                         { return core.ToolPermissions{} }
func (fileWriteStubTool) Tags() []string                                            { return []string{core.TagDestructive, "file", "edit"} }

func makeRecoveryRegistry(t *testing.T, tools ...stubTool) *capability.Registry {
	t.Helper()
	registry := capability.NewRegistry()
	for _, tool := range tools {
		assert.NoError(t, registry.Register(tool))
	}
	return registry
}

func TestReactExecuteUsesExplicitRetrievalSummarizeAndCheckpointNodes(t *testing.T) {
	mem, err := memory.NewHybridMemory(t.TempDir())
	assert.NoError(t, err)
	assert.NoError(t, mem.Remember(context.Background(), "fact-1", map[string]interface{}{
		"memory_class": string(core.MemoryClassDeclarative),
		"summary":      "project fact",
	}, memory.MemoryScopeProject))

	agent := &ReActAgent{
		Model: &stubLLM{responses: []*core.LLMResponse{
			{Text: `{"thought":"done","action":"complete","tool":"","arguments":{},"complete":true,"summary":"finished"}`},
		}},
		Tools:          capability.NewRegistry(),
		Memory:         mem,
		CheckpointPath: t.TempDir(),
	}
	err = agent.Initialize(&core.Config{Name: "react-test"})
	assert.NoError(t, err)

	task := &core.Task{ID: "react-phase4", Instruction: "Summarize the current state."}
	state := core.NewContext()
	state.Set("task.id", task.ID)
	state.Set("task.instruction", task.Instruction)

	result, err := agent.Execute(context.Background(), task, state)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Success)

	_, ok := state.Get("graph.declarative_memory")
	assert.True(t, ok)
	_, ok = state.Get("graph.summary")
	assert.True(t, ok)
	_, ok = state.Get("graph.checkpoint")
	assert.True(t, ok)
	rawCheckpointRef, ok := state.Get("react.checkpoint_ref")
	assert.True(t, ok)
	checkpointRef, ok := rawCheckpointRef.(core.ArtifactReference)
	assert.True(t, ok)
	assert.NotEmpty(t, checkpointRef.ArtifactID)

	store := memory.NewCheckpointStore(agent.CheckpointPath)
	ids, err := store.List(task.ID)
	assert.NoError(t, err)
	assert.NotEmpty(t, ids)
}

func TestReactExecutePersistsCompletionSummaryThroughRuntimeWriter(t *testing.T) {
	mem, err := db.NewSQLiteRuntimeMemoryStore(filepath.Join(t.TempDir(), "runtime_memory.db"))
	assert.NoError(t, err)
	defer mem.Close()
	assert.NoError(t, mem.PutDeclarative(context.Background(), memory.DeclarativeMemoryRecord{
		RecordID: "fact-1",
		Scope:    memory.MemoryScopeProject,
		Kind:     memory.DeclarativeMemoryKindFact,
		Summary:  "project fact",
		Title:    "fact",
	}))

	agent := &ReActAgent{
		Model: &stubLLM{responses: []*core.LLMResponse{
			{Text: `{"thought":"done","action":"complete","tool":"","arguments":{},"complete":true,"summary":"finished"}`},
		}},
		Tools:  capability.NewRegistry(),
		Memory: mem,
	}
	err = agent.Initialize(&core.Config{Name: "react-phase6"})
	assert.NoError(t, err)

	task := &core.Task{ID: "react-phase6", Instruction: "Summarize the current state."}
	state := core.NewContext()
	state.Set("task.id", task.ID)
	state.Set("task.instruction", task.Instruction)

	result, err := agent.Execute(context.Background(), task, state)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Success)

	records, err := mem.SearchDeclarative(context.Background(), memory.DeclarativeMemoryQuery{
		TaskID: task.ID,
		Scope:  memory.MemoryScopeProject,
		Limit:  10,
	})
	assert.NoError(t, err)
	assert.NotEmpty(t, records)

	raw, ok := state.Get("graph.persistence")
	assert.True(t, ok)
	payload, ok := raw.(map[string]any)
	assert.True(t, ok)
	audits, ok := payload["records"].([]graph.PersistenceAuditRecord)
	assert.True(t, ok)
	assert.NotEmpty(t, audits)
	assert.Equal(t, graph.PersistenceActionCreated, audits[0].Action)
}

func TestReactExecuteCanUseLegacyCheckpointCallbackWhenExplicitNodesDisabled(t *testing.T) {
	agent := &ReActAgent{
		Model: &stubLLM{responses: []*core.LLMResponse{
			{Text: `{"thought":"done","action":"complete","tool":"","arguments":{},"complete":true,"summary":"finished"}`},
		}},
		Tools:          capability.NewRegistry(),
		CheckpointPath: t.TempDir(),
	}
	err := agent.Initialize(&core.Config{
		Name:                       "react-legacy-checkpoint",
		UseExplicitCheckpointNodes: boolPtr(false),
		UseDeclarativeRetrieval:    boolPtr(false),
		UseStructuredPersistence:   boolPtr(false),
	})
	assert.NoError(t, err)

	task := &core.Task{ID: "react-legacy", Instruction: "Finish quickly."}
	state := core.NewContext()
	state.Set("task.id", task.ID)

	result, err := agent.Execute(context.Background(), task, state)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Success)

	_, ok := state.Get("graph.checkpoint")
	assert.False(t, ok)

	store := memory.NewCheckpointStore(agent.CheckpointPath)
	ids, err := store.List(task.ID)
	assert.NoError(t, err)
	assert.NotEmpty(t, ids)
}

func TestPromptAssemblerFormatsWorkflowRetrievalEvidence(t *testing.T) {
	task := &core.Task{
		Instruction: "Use workflow retrieval evidence",
		Context: map[string]any{
			"workflow_retrieval": map[string]any{
				"query":      "find evidence",
				"scope":      "workflow:wf-1",
				"cache_tier": "l3_main",
				"results": []map[string]any{
					{
						"text": "retrieved workflow evidence",
						"citations": []retrieval.PackedCitation{{
							ChunkID:      "chunk:1",
							CanonicalURI: "memory://workflow/1",
						}},
					},
				},
			},
		},
	}
	assembler := newPromptContextAssembler(&ReActAgent{}, task)
	prompt := assembler.buildPrompt(core.NewContext(), []core.Tool{stubTool{name: "echo"}}, true)

	assert.Contains(t, prompt, "Workflow Retrieval:")
	assert.Contains(t, prompt, "Query: find evidence")
	assert.Contains(t, prompt, "Sources: memory://workflow/1")
}

func TestPromptAssemblerFormatsReferenceOnlyWorkflowRetrievalEvidence(t *testing.T) {
	task := &core.Task{
		Instruction: "Use workflow retrieval evidence",
		Context: map[string]any{
			"workflow_retrieval": map[string]any{
				"query": "find evidence",
				"results": []map[string]any{
					{
						"summary": "retrieved workflow evidence summary",
						"reference": map[string]any{
							"kind":   string(core.ContextReferenceRetrievalEvidence),
							"uri":    "memory://workflow/2",
							"detail": "packed",
						},
					},
				},
			},
		},
	}
	assembler := newPromptContextAssembler(&ReActAgent{}, task)
	prompt := assembler.buildPrompt(core.NewContext(), []core.Tool{stubTool{name: "echo"}}, true)

	assert.Contains(t, prompt, "retrieved workflow evidence summary")
	assert.Contains(t, prompt, "Reference: memory://workflow/2")
}

func TestPromptAssemblerPrefersWorkflowRetrievalPayload(t *testing.T) {
	task := &core.Task{
		Instruction: "Use workflow retrieval evidence",
		Context: map[string]any{
			"workflow_retrieval": "legacy summary text",
			"workflow_retrieval_payload": map[string]any{
				"query": "find evidence",
				"results": []map[string]any{
					{
						"summary": "structured workflow evidence",
						"reference": map[string]any{
							"kind": string(core.ContextReferenceRetrievalEvidence),
							"uri":  "memory://workflow/3",
						},
					},
				},
			},
		},
	}

	assembler := newPromptContextAssembler(&ReActAgent{}, task)
	prompt := assembler.buildPrompt(core.NewContext(), []core.Tool{stubTool{name: "echo"}}, true)

	assert.Contains(t, prompt, "structured workflow evidence")
	assert.Contains(t, prompt, "Reference: memory://workflow/3")
	assert.NotContains(t, prompt, "\"legacy summary text\"")
}

func TestPromptAssemblerPrefersGraphDeclarativeMemoryPayload(t *testing.T) {
	task := &core.Task{Instruction: "Use memory context"}
	state := core.NewContext()
	state.Set("graph.declarative_memory", map[string]any{
		"results": []core.MemoryRecordEnvelope{{
			Key:     "legacy-1",
			Summary: "legacy memory summary",
		}},
	})
	state.Set("graph.declarative_memory_payload", map[string]any{
		"results": []map[string]any{
			{
				"summary": "mixed evidence memory summary",
				"source":  "retrieval",
			},
		},
	})

	assembler := newPromptContextAssembler(&ReActAgent{}, task)
	prompt := assembler.buildPrompt(state, []core.Tool{stubTool{name: "echo"}}, true)

	assert.Contains(t, prompt, "mixed evidence memory summary [retrieval]")
	assert.NotContains(t, prompt, "legacy memory summary")
}

func TestPromptAssemblerFallsBackToGraphDeclarativeMemoryRefs(t *testing.T) {
	task := &core.Task{Instruction: "Use memory context"}
	state := core.NewContext()
	state.Set("graph.declarative_memory_payload", map[string]any{
		"results": []map[string]any{
			{
				"reference": map[string]any{
					"kind": string(core.ContextReferenceRetrievalEvidence),
					"uri":  "memory://runtime/chunk-1",
				},
			},
		},
	})
	state.Set("graph.declarative_memory_refs", []core.ContextReference{{
		Kind: core.ContextReferenceRetrievalEvidence,
		URI:  "memory://runtime/chunk-1",
	}})

	assembler := newPromptContextAssembler(&ReActAgent{}, task)
	prompt := assembler.buildPrompt(state, []core.Tool{stubTool{name: "echo"}}, true)

	assert.Contains(t, prompt, "Reference: memory://runtime/chunk-1")
}

func TestRenderContextFilesUsesSummaryAndReferenceWithoutHydration(t *testing.T) {
	task := &core.Task{
		Instruction: "inspect context",
		Context: map[string]any{
			"context_file_contents": []core.ContextFileContent{{
				Path:    "src/lib.rs",
				Summary: "Rust library entrypoint",
				Reference: &core.ContextReference{
					Kind:   core.ContextReferenceFile,
					ID:     "src/lib.rs",
					URI:    "src/lib.rs",
					Detail: "summary",
				},
			}},
		},
	}

	rendered := renderContextFiles(task, 512)
	assert.Contains(t, rendered, "src/lib.rs [detail=summary]")
	assert.Contains(t, rendered, "Rust library entrypoint")
	assert.NotContains(t, rendered, "```")
}

// TestReActAgentExecute validates a minimal think-act-observe pass.
func TestReActAgentExecute(t *testing.T) {
	llm := &stubLLM{
		responses: []*core.LLMResponse{
			{Text: `{"thought":"finished","tool":"none","arguments":{},"complete":true}`},
		},
	}
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(stubTool{name: "echo"}))
	agent := &ReActAgent{
		Model: llm,
		Tools: registry,
	}
	cfg := &core.Config{Model: "test-model", MaxIterations: 2, NativeToolCalling: true}
	assert.NoError(t, agent.Initialize(cfg))

	task := &core.Task{ID: "task-1", Instruction: "do something"}
	state := core.NewContext()
	state.Set("task.id", task.ID)

	think := &reactThinkNode{id: "think", agent: agent, task: task}
	act := &reactActNode{id: "act", agent: agent}
	observe := &reactObserveNode{id: "observe", agent: agent, task: task}
	terminal := graph.NewTerminalNode("done")

	graph := graph.NewGraph()
	assert.NoError(t, graph.AddNode(think))
	assert.NoError(t, graph.AddNode(act))
	assert.NoError(t, graph.AddNode(observe))
	assert.NoError(t, graph.AddNode(terminal))
	assert.NoError(t, graph.SetStart("think"))
	assert.NoError(t, graph.AddEdge("think", "act", nil, false))
	assert.NoError(t, graph.AddEdge("act", "observe", nil, false))
	assert.NoError(t, graph.AddEdge("observe", "done", nil, false))

	// run single pass (no loop) to validate node behavior
	result, err := graph.Execute(context.Background(), state)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "done", result.NodeID)

	final, ok := state.Get("react.final_output")
	assert.True(t, ok, "final output should be stored in context")
	assert.Contains(t, final.(map[string]interface{})["summary"], "Iteration")
	assert.Equal(t, 1, llm.withToolsCalls)
	assert.Equal(t, 0, llm.generateCalls)
}

// TestReActAgentToolCallingDisabled ensures the agent falls back to plain text.
func TestReActAgentToolCallingDisabled(t *testing.T) {
	llm := &stubLLM{
		responses: []*core.LLMResponse{
			{Text: `{"thought":"finished","tool":"none","arguments":{},"complete":true}`},
		},
	}
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(stubTool{name: "echo"}))
	agent := &ReActAgent{
		Model: llm,
		Tools: registry,
	}
	cfg := &core.Config{Model: "test-model", MaxIterations: 2, NativeToolCalling: false}
	assert.NoError(t, agent.Initialize(cfg))

	task := &core.Task{ID: "task-2", Instruction: "do something"}
	state := core.NewContext()
	state.Set("task.id", task.ID)

	think := &reactThinkNode{id: "think", agent: agent, task: task}
	act := &reactActNode{id: "act", agent: agent}
	observe := &reactObserveNode{id: "observe", agent: agent, task: task}
	terminal := graph.NewTerminalNode("done")

	graph := graph.NewGraph()
	assert.NoError(t, graph.AddNode(think))
	assert.NoError(t, graph.AddNode(act))
	assert.NoError(t, graph.AddNode(observe))
	assert.NoError(t, graph.AddNode(terminal))
	assert.NoError(t, graph.SetStart("think"))
	assert.NoError(t, graph.AddEdge("think", "act", nil, false))
	assert.NoError(t, graph.AddEdge("act", "observe", nil, false))
	assert.NoError(t, graph.AddEdge("observe", "done", nil, false))

	result, err := graph.Execute(context.Background(), state)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "done", result.NodeID)
	assert.Equal(t, 0, llm.withToolsCalls)
	assert.Equal(t, 1, llm.generateCalls)
}

// TestReActAgentToolCallingFallbackExecutesParsedToolCalls ensures that
// tool calls parsed from plain text are still executed when tool calling is off.
func TestReActAgentToolCallingFallbackExecutesParsedToolCalls(t *testing.T) {
	llm := &stubLLM{
		responses: []*core.LLMResponse{
			{Text: `{"tool":"echo","arguments":{"value":"hi"}}`},
		},
	}
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(stubTool{name: "echo"}))
	agent := &ReActAgent{
		Model: llm,
		Tools: registry,
	}
	cfg := &core.Config{Model: "test-model", MaxIterations: 2, NativeToolCalling: false}
	assert.NoError(t, agent.Initialize(cfg))

	task := &core.Task{ID: "task-3", Instruction: "use tool via fallback"}
	state := core.NewContext()
	state.Set("task.id", task.ID)

	think := &reactThinkNode{id: "think", agent: agent, task: task}
	act := &reactActNode{id: "act", agent: agent}
	observe := &reactObserveNode{id: "observe", agent: agent, task: task}
	terminal := graph.NewTerminalNode("done")

	graph := graph.NewGraph()
	assert.NoError(t, graph.AddNode(think))
	assert.NoError(t, graph.AddNode(act))
	assert.NoError(t, graph.AddNode(observe))
	assert.NoError(t, graph.AddNode(terminal))
	assert.NoError(t, graph.SetStart("think"))
	assert.NoError(t, graph.AddEdge("think", "act", nil, false))
	assert.NoError(t, graph.AddEdge("act", "observe", nil, false))
	assert.NoError(t, graph.AddEdge("observe", "done", nil, false))

	result, err := graph.Execute(context.Background(), state)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "done", result.NodeID)
	assert.Equal(t, 0, llm.withToolsCalls)
	assert.Equal(t, 1, llm.generateCalls)

	lastToolRes, ok := state.Get("react.last_tool_result")
	assert.True(t, ok)
	assert.Contains(t, fmt.Sprint(lastToolRes.(map[string]interface{})["echo"]), "hi")
}

func TestParseDecisionNormalizesMalformedToolAction(t *testing.T) {
	parsed, err := parseDecision(`{"thought":"write file","action":"tool|complete","tool":"file_write","arguments":{"path":"hello.txt"},"complete":true}`)
	assert.NoError(t, err)
	assert.Equal(t, "tool", parsed.Action)
	assert.Equal(t, "file_write", parsed.Tool)
	assert.False(t, parsed.Complete)
	assert.Equal(t, "hello.txt", parsed.Arguments["path"])
}

func TestReactActNodeStoresCapabilityEnvelope(t *testing.T) {
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(stubTool{name: "echo"}))

	agent := &ReActAgent{
		Tools: registry,
		contextPolicy: contextmgr.NewContextPolicy(contextmgr.ContextPolicyConfig{
			Strategy: contextmgr.NewAdaptiveStrategy(),
		}, nil),
	}

	node := &reactActNode{id: "act", agent: agent}
	state := core.NewContext()
	state.Set("task.id", "task-1")
	state.Set("react.decision", decisionPayload{
		Tool:      "echo",
		Arguments: map[string]interface{}{"value": "hi"},
	})

	result, err := node.Execute(context.Background(), state)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	rawEnvelope, ok := state.Get("react.last_tool_result_envelope")
	assert.True(t, ok)
	envelope, ok := rawEnvelope.(*core.CapabilityResultEnvelope)
	assert.True(t, ok)
	assert.Equal(t, "tool:echo", envelope.Descriptor.ID)
	assert.Equal(t, "task-1", envelope.Approval.TaskID)
	assert.NotNil(t, envelope.Policy)
	assert.Equal(t, envelope.Policy.ID, envelope.Insertion.PolicySnapshotID)

	capabilityEnvelope, ok := result.Metadata["capability_result"].(*core.CapabilityResultEnvelope)
	assert.True(t, ok)
	assert.Equal(t, envelope.Descriptor.ID, capabilityEnvelope.Descriptor.ID)

	items := agent.contextPolicy.ContextManager.GetItemsByType(core.ContextTypeToolResult)
	if assert.Len(t, items, 1) {
		item, ok := items[0].(*core.ToolResultContextItem)
		assert.True(t, ok)
		assert.NotNil(t, item.Envelope)
		assert.Equal(t, core.ContentDispositionSummarized, item.Envelope.Disposition)
		assert.Equal(t, core.InsertionActionSummarized, item.Envelope.Insertion.Action)
	}
}

func TestReactActNodeRefreshesSearchIndexAfterFileWrite(t *testing.T) {
	dir := t.TempDir()
	index, err := memory.NewCodeIndex(dir, filepath.Join(dir, "code_index.json"))
	assert.NoError(t, err)
	assert.NoError(t, index.BuildIndex(context.Background()))
	searchEngine := frameworksearch.NewSearchEngine(nil, index)

	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(fileWriteStubTool{}))

	agent := &ReActAgent{
		Tools:        registry,
		SearchEngine: searchEngine,
		contextPolicy: contextmgr.NewContextPolicy(contextmgr.ContextPolicyConfig{
			Strategy: contextmgr.NewAdaptiveStrategy(),
		}, nil),
	}

	node := &reactActNode{id: "act", agent: agent}
	state := core.NewContext()
	target := filepath.Join(dir, "fresh.go")
	state.Set("react.decision", decisionPayload{
		Tool: "file_write",
		Arguments: map[string]interface{}{
			"path":    target,
			"content": "package sample\nfunc Fresh() string { return \"needle\" }\n",
		},
	})

	result, err := node.Execute(context.Background(), state)
	assert.NoError(t, err)
	if assert.NotNil(t, result) {
		assert.True(t, result.Success)
	}

	results, err := searchEngine.Search(frameworksearch.SearchQuery{
		Text:       "needle",
		Mode:       frameworksearch.SearchKeyword,
		MaxResults: 5,
	})
	assert.NoError(t, err)
	if assert.NotEmpty(t, results) {
		assert.Contains(t, results[0].File, "fresh.go")
	}
}

func TestReactActNodeRefreshesASTIndexAfterFileWrite(t *testing.T) {
	dir := t.TempDir()
	astStore, err := ast.NewSQLiteStore(filepath.Join(dir, "ast.db"))
	assert.NoError(t, err)
	defer astStore.Close()
	indexManager := ast.NewIndexManager(astStore, ast.IndexConfig{WorkspacePath: dir})

	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(fileWriteStubTool{}))

	agent := &ReActAgent{
		Tools:        registry,
		IndexManager: indexManager,
		contextPolicy: contextmgr.NewContextPolicy(contextmgr.ContextPolicyConfig{
			Strategy: contextmgr.NewAdaptiveStrategy(),
		}, nil),
	}

	node := &reactActNode{id: "act", agent: agent}
	state := core.NewContext()
	target := filepath.Join(dir, "fresh.go")
	state.Set("react.decision", decisionPayload{
		Tool: "file_write",
		Arguments: map[string]interface{}{
			"path":    target,
			"content": "package sample\nfunc Fresh() string { return \"needle\" }\n",
		},
	})

	result, err := node.Execute(context.Background(), state)
	assert.NoError(t, err)
	if assert.NotNil(t, result) {
		assert.True(t, result.Success)
	}

	nodes, err := indexManager.QuerySymbol("Fresh")
	assert.NoError(t, err)
	if assert.NotEmpty(t, nodes) {
		assert.Equal(t, "Fresh", nodes[0].Name)
	}
}

func TestReactCapabilityEnvelopeEmitsInsertionTelemetry(t *testing.T) {
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(stubTool{name: "echo"}))
	telemetry := &recordingTelemetry{}

	agent := &ReActAgent{
		Tools: registry,
		Config: &core.Config{
			Telemetry: telemetry,
		},
	}

	node := &reactActNode{id: "act", agent: agent}
	state := core.NewContext()
	state.Set("task.id", "task-telemetry")
	res := &core.ToolResult{Success: true, Data: map[string]interface{}{"echo": "hi"}}
	_ = node.capabilityEnvelope(context.Background(), state, stubTool{name: "echo"}, core.ToolCall{Name: "echo"}, res)

	found := false
	for _, event := range telemetry.events {
		if event.Metadata["security_event"] == "insertion_decision" {
			found = true
			assert.Equal(t, "tool:echo", event.Metadata["capability_id"])
		}
	}
	assert.True(t, found)
}

func TestRenderInsertionFilteredSummaryMetadataOnly(t *testing.T) {
	agent := &ReActAgent{
		Config: &core.Config{
			AgentSpec: &core.AgentRuntimeSpec{
				InsertionPolicies: []core.CapabilityInsertionPolicy{
					{
						Selector: core.CapabilitySelector{
							Name: "echo",
						},
						Action: core.InsertionActionMetadataOnly,
					},
				},
			},
		},
	}
	payload := &core.ToolResult{
		Success: true,
		Data:    map[string]interface{}{"echo": "secret"},
	}
	envelope := core.NewCapabilityResultEnvelope(
		core.CapabilityDescriptor{
			ID:         "tool:echo",
			Kind:       core.CapabilityKindTool,
			Name:       "echo",
			TrustClass: core.TrustClassBuiltinTrusted,
		},
		payload,
		core.ContentDispositionRaw,
		nil,
		nil,
	)

	text, ok := renderInsertionFilteredSummary(agent, nil, "echo", payload, envelope)
	assert.True(t, ok)
	assert.Contains(t, text, "metadata-only")
	assert.NotContains(t, text, "secret")
}

func TestReactCapabilityEnvelopePersistsEffectiveInsertionDecision(t *testing.T) {
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(stubTool{name: "echo"}))
	registry.UseAgentSpec("agent-1", &core.AgentRuntimeSpec{
		InsertionPolicies: []core.CapabilityInsertionPolicy{
			{
				Selector: core.CapabilitySelector{Name: "echo"},
				Action:   core.InsertionActionMetadataOnly,
			},
		},
	})

	agent := &ReActAgent{
		Tools: registry,
		Config: &core.Config{
			AgentSpec: &core.AgentRuntimeSpec{
				InsertionPolicies: []core.CapabilityInsertionPolicy{
					{
						Selector: core.CapabilitySelector{Name: "echo"},
						Action:   core.InsertionActionMetadataOnly,
					},
				},
			},
		},
	}

	node := &reactActNode{id: "act", agent: agent}
	res := &core.ToolResult{Success: true, Data: map[string]interface{}{"echo": "secret"}}
	envelope := node.capabilityEnvelope(context.Background(), core.NewContext(), stubTool{name: "echo"}, core.ToolCall{Name: "echo"}, res)

	assert.Equal(t, core.InsertionActionMetadataOnly, envelope.Insertion.Action)
	assert.NotEmpty(t, envelope.BlockInsertions)
	assert.Equal(t, core.InsertionActionMetadataOnly, envelope.BlockInsertions[0].Decision.Action)

	rawDecision, ok := res.Metadata["insertion_decision"].(core.InsertionDecision)
	assert.True(t, ok)
	assert.Equal(t, core.InsertionActionMetadataOnly, rawDecision.Action)
}

func TestRenderInsertionFilteredSummaryUsesVisibleBlocksOnly(t *testing.T) {
	payload := &core.ToolResult{
		Success: true,
		Data:    map[string]interface{}{"summary": "visible secret"},
	}
	envelope := &core.CapabilityResultEnvelope{
		Descriptor: core.CapabilityDescriptor{
			ID:         "tool:echo",
			Kind:       core.CapabilityKindTool,
			Name:       "echo",
			TrustClass: core.TrustClassBuiltinTrusted,
		},
		ContentBlocks: []core.ContentBlock{
			core.TextContentBlock{Text: "visible"},
			core.ResourceLinkContentBlock{URI: "file:///tmp/secret"},
		},
		BlockInsertions: []core.ContentBlockInsertion{
			{ContentType: "text", Decision: core.InsertionDecision{Action: core.InsertionActionDirect}},
			{ContentType: "resource-link", Decision: core.InsertionDecision{Action: core.InsertionActionMetadataOnly}},
		},
		Insertion: core.InsertionDecision{Action: core.InsertionActionDirect},
	}

	text, ok := renderInsertionFilteredSummary(nil, nil, "echo", payload, envelope)
	assert.True(t, ok)
	assert.Contains(t, text, "visible")
	assert.NotContains(t, text, "/tmp/secret")
}

func TestReactObservationHistoryRespectsDeniedInsertionPolicy(t *testing.T) {
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(stubTool{name: "echo"}))

	agent := &ReActAgent{
		Tools: registry,
		Config: &core.Config{
			AgentSpec: &core.AgentRuntimeSpec{
				InsertionPolicies: []core.CapabilityInsertionPolicy{
					{
						Selector: core.CapabilitySelector{Name: "echo"},
						Action:   core.InsertionActionDenied,
					},
				},
			},
		},
		contextPolicy: contextmgr.NewContextPolicy(contextmgr.ContextPolicyConfig{
			Strategy: contextmgr.NewAdaptiveStrategy(),
		}, nil),
	}

	node := &reactActNode{id: "act", agent: agent}
	state := core.NewContext()
	state.Set("task.id", "task-1")
	state.Set("react.messages", []core.Message{{Role: "assistant", Content: "use tool"}})
	state.Set("react.decision", decisionPayload{
		Tool:      "echo",
		Arguments: map[string]interface{}{"value": "secret"},
	})

	_, err := node.Execute(context.Background(), state)
	assert.NoError(t, err)

	rawObs, ok := state.Get("react.tool_observations")
	assert.True(t, ok)
	assert.Empty(t, rawObs.([]ToolObservation))

	messages := getReactMessages(state)
	assert.Len(t, messages, 1)

	assembler := newPromptContextAssembler(agent, &core.Task{Instruction: "test"})
	assert.Empty(t, assembler.recentToolObservations(state))
	assert.Empty(t, assembler.contextFiles(state))
}

func TestReactObservationHistoryMetadataOnlyRedactsSummary(t *testing.T) {
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(stubTool{name: "echo"}))

	agent := &ReActAgent{
		Tools: registry,
		Config: &core.Config{
			AgentSpec: &core.AgentRuntimeSpec{
				InsertionPolicies: []core.CapabilityInsertionPolicy{
					{
						Selector: core.CapabilitySelector{Name: "echo"},
						Action:   core.InsertionActionMetadataOnly,
					},
				},
			},
		},
	}

	node := &reactActNode{id: "act", agent: agent}
	state := core.NewContext()
	state.Set("task.id", "task-1")
	state.Set("react.messages", []core.Message{{Role: "assistant", Content: "use tool"}})
	state.Set("react.decision", decisionPayload{
		Tool:      "echo",
		Arguments: map[string]interface{}{"value": "secret"},
	})

	_, err := node.Execute(context.Background(), state)
	assert.NoError(t, err)

	rawObs, ok := state.Get("react.tool_observations")
	assert.True(t, ok)
	observations := rawObs.([]ToolObservation)
	if assert.Len(t, observations, 1) {
		assert.Contains(t, observations[0].Summary, "metadata-only")
		assert.NotContains(t, observations[0].Summary, "secret")
		assert.Nil(t, observations[0].Data)
	}

	assembler := newPromptContextAssembler(agent, &core.Task{Instruction: "test"})
	obsText := assembler.recentToolObservations(state)
	assert.Contains(t, obsText, "metadata-only")
	assert.NotContains(t, obsText, "secret")
}

func TestPromptBuilderIncludesNonToolCapabilityCatalog(t *testing.T) {
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(stubTool{name: "echo"}))
	assert.NoError(t, registry.RegisterCapability(core.CapabilityDescriptor{
		ID:          "prompt:catalog:1",
		Kind:        core.CapabilityKindPrompt,
		Name:        "catalog.prompt.1",
		Description: "Use the catalog prompt",
		Source: core.CapabilitySource{
			Scope: core.CapabilityScopeWorkspace,
		},
		TrustClass: core.TrustClassWorkspaceTrusted,
	}))
	assert.NoError(t, registry.RegisterCapability(core.CapabilityDescriptor{
		ID:          "resource:catalog:guide",
		Kind:        core.CapabilityKindResource,
		Name:        "catalog.resource.guide",
		Description: "Guide resource",
		Source: core.CapabilitySource{
			Scope: core.CapabilityScopeWorkspace,
		},
		TrustClass: core.TrustClassWorkspaceTrusted,
	}))

	agent := &ReActAgent{Tools: registry}
	assembler := newPromptContextAssembler(agent, &core.Task{Instruction: "test"})

	prompt := assembler.buildPrompt(core.NewContext(), []core.Tool{stubTool{name: "echo"}}, true)

	assert.Contains(t, prompt, "Capability Catalog:")
	assert.Contains(t, prompt, "catalog.prompt.1 [prompt]")
	assert.Contains(t, prompt, "catalog.resource.guide [resource]")
}

// TestReActAgentToolCalling verifies tool call handling and transcript storage.
func TestReActAgentToolCalling(t *testing.T) {
	llm := &stubLLM{
		responses: []*core.LLMResponse{
			{Text: "", ToolCalls: []core.ToolCall{{Name: "echo", Args: map[string]interface{}{"value": "hi"}}}},
			{Text: "all done"},
		},
	}
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(stubTool{name: "echo"}))
	agent := &ReActAgent{
		Model: llm,
		Tools: registry,
	}
	cfg := &core.Config{Model: "test-model", MaxIterations: 3, NativeToolCalling: true}
	assert.NoError(t, agent.Initialize(cfg))

	task := &core.Task{ID: "task-2", Instruction: "use tool"}
	state := core.NewContext()
	state.Set("task.id", task.ID)

	think := &reactThinkNode{id: "think", agent: agent, task: task}
	act := &reactActNode{id: "act", agent: agent}
	observe := &reactObserveNode{id: "observe", agent: agent, task: task}
	terminal := graph.NewTerminalNode("done")

	graph := graph.NewGraph()
	assert.NoError(t, graph.AddNode(think))
	assert.NoError(t, graph.AddNode(act))
	assert.NoError(t, graph.AddNode(observe))
	assert.NoError(t, graph.AddNode(terminal))
	assert.NoError(t, graph.SetStart("think"))
	assert.NoError(t, graph.AddEdge("think", "act", nil, false))
	assert.NoError(t, graph.AddEdge("act", "observe", nil, false))
	assert.NoError(t, graph.AddEdge("observe", "done", nil, false))

	result, err := graph.Execute(context.Background(), state)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "done", result.NodeID)
	assert.Equal(t, 1, llm.withToolsCalls)

	lastToolRes, ok := state.Get("react.last_tool_result")
	assert.True(t, ok)
	assert.Contains(t, fmt.Sprint(lastToolRes.(map[string]interface{})["echo"]), "hi")

	messagesVal, ok := state.Get("react.messages")
	assert.True(t, ok)
	messages, ok := messagesVal.([]core.Message)
	assert.True(t, ok)
	var toolMessages int
	for _, msg := range messages {
		if msg.Role == "tool" {
			toolMessages++
			assert.Equal(t, "echo", msg.Name)
			assert.Contains(t, msg.Content, "success")
		}
	}
	assert.Equal(t, 1, toolMessages)
}

func TestReActAgentToolCallingPreservesTranscriptAcrossTurns(t *testing.T) {
	llm := &stubLLM{
		responses: []*core.LLMResponse{
			{Text: "", ToolCalls: []core.ToolCall{{ID: "call-1", Name: "echo", Args: map[string]interface{}{"value": "hi"}}}},
			{Text: `{"thought":"finished","tool":"","arguments":{},"complete":true,"summary":"done"}`},
		},
	}
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(stubTool{name: "echo"}))
	agent := &ReActAgent{
		Model: llm,
		Tools: registry,
	}
	cfg := &core.Config{Model: "test-model", MaxIterations: 3, NativeToolCalling: true}
	assert.NoError(t, agent.Initialize(cfg))

	task := &core.Task{ID: "task-transcript", Instruction: "use the echo tool and finish"}
	state := core.NewContext()
	state.Set("task.id", task.ID)

	result, err := agent.Execute(context.Background(), task, state)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.GreaterOrEqual(t, len(llm.toolMessages), 2)

	secondTurn := llm.toolMessages[1]
	var sawAssistantToolCall bool
	var sawToolResult bool
	for _, msg := range secondTurn {
		if msg.Role == "assistant" && len(msg.ToolCalls) == 1 && msg.ToolCalls[0].Name == "echo" {
			sawAssistantToolCall = true
		}
		if msg.Role == "tool" && msg.Name == "echo" {
			sawToolResult = true
		}
	}
	assert.True(t, sawAssistantToolCall, "expected prior assistant tool call to be preserved")
	assert.True(t, sawToolResult, "expected prior tool result to be preserved")
}

func TestReactObserveReturnsToEditAfterFailedVerificationRead(t *testing.T) {
	agent := &ReActAgent{}
	task := &core.Task{Instruction: "fix the Rust module and rerun tests"}
	state := core.NewContext()
	state.Set("react.phase", contextmgrPhaseVerify)
	state.Set("react.last_tool_result", map[string]interface{}{
		"cli_cargo": map[string]interface{}{
			"success": false,
			"error":   "compile failed",
		},
	})
	state.Set("react.tool_observations", []ToolObservation{
		{Tool: "cli_cargo", Phase: contextmgrPhaseVerify, Success: false},
		{Tool: "file_read", Phase: contextmgrPhaseVerify, Success: true},
	})

	node := &reactObserveNode{agent: agent, task: task}
	node.advancePhase(state, decisionPayload{}, map[string]interface{}{})

	assert.Equal(t, contextmgrPhaseEdit, state.GetString("react.phase"))
}

func TestCompactToolDataPreservesMeaningfulStderr(t *testing.T) {
	call := core.ToolCall{Name: "cli_cargo"}
	res := &core.ToolResult{
		Success: false,
		Error:   "exit status 101",
		Data: map[string]interface{}{
			"stderr": "error: failed to parse manifest at /tmp/demo/Cargo.toml\nCaused by:\n  bad workspace\n",
		},
	}

	summary, data := compactToolData(call, res)
	assert.Contains(t, summary, "failed to parse manifest")
	assert.Contains(t, fmt.Sprint(data["stderr"]), "failed to parse manifest")
}

func TestInitializePhaseStartsDebugCargoTaskInVerify(t *testing.T) {
	agent := &ReActAgent{Mode: "debug"}
	state := core.NewContext()
	task := &core.Task{
		ID:          "debug-verify",
		Instruction: "Run cargo test and explain why it fails",
		Context:     map[string]any{"mode": "debug"},
	}

	agent.initializePhase(state, task)
	assert.Equal(t, contextmgrPhaseVerify, state.GetString("react.phase"))
}

func TestToolAllowedForPhasePermitsRustExecutionInEdit(t *testing.T) {
	tool := stubTool{name: "cli_cargo", tags: []string{core.TagExecute}}
	task := &core.Task{
		Instruction: "Fix the failing Rust tests and rerun cargo test",
		Context:     map[string]any{"mode": "code"},
	}

	assert.True(t, toolAllowedForPhase(tool, contextmgrPhaseEdit, task))
}

func TestObserveEntersEditPhaseAfterSingleRead(t *testing.T) {
	agent := &ReActAgent{}
	state := core.NewContext()
	state.Set("react.phase", contextmgrPhaseExplore)
	state.Set("react.tool_observations", []ToolObservation{
		{Tool: "file_read", Args: map[string]interface{}{"path": "src/lib.rs"}, Data: map[string]interface{}{"snippet": "fn add() {}"}, Success: true},
	})
	task := &core.Task{Instruction: "Fix the bug in src/lib.rs"}
	node := &reactObserveNode{id: "observe", agent: agent, task: task}

	node.advancePhase(state, decisionPayload{}, map[string]interface{}{"content": "fn add() {}"})
	assert.Equal(t, contextmgrPhaseEdit, state.GetString("react.phase"))
}

func TestObserveKeepsExplorePhaseAfterFailedSingleRead(t *testing.T) {
	agent := &ReActAgent{}
	state := core.NewContext()
	state.Set("react.phase", contextmgrPhaseExplore)
	state.Set("react.tool_observations", []ToolObservation{
		{Tool: "file_read", Args: map[string]interface{}{"path": "src/lib.rs"}, Data: map[string]interface{}{"error": "permission denied"}, Success: false},
	})
	task := &core.Task{Instruction: "Fix the bug in src/lib.rs"}
	node := &reactObserveNode{id: "observe", agent: agent, task: task}

	node.advancePhase(state, decisionPayload{}, map[string]interface{}{"error": "permission denied"})
	assert.Equal(t, contextmgrPhaseExplore, state.GetString("react.phase"))
}

func TestReActAgentFailsWhenIterationBudgetIsExhaustedWithoutEdits(t *testing.T) {
	llm := &stubLLM{
		responses: []*core.LLMResponse{
			{Text: `{"tool":"echo","arguments":{"value":"same"}}`},
			{Text: `{"tool":"echo","arguments":{"value":"same"}}`},
			{Text: `{"tool":"echo","arguments":{"value":"same"}}`},
		},
	}
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(stubTool{name: "echo"}))
	agent := &ReActAgent{
		Model: llm,
		Tools: registry,
	}
	cfg := &core.Config{Model: "test-model", MaxIterations: 5, NativeToolCalling: false}
	assert.NoError(t, agent.Initialize(cfg))

	task := &core.Task{ID: "task-loop", Instruction: "Fix the bug and run tests"}
	state := core.NewContext()
	state.Set("task.id", task.ID)

	result, err := agent.Execute(context.Background(), task, state)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error.Error(), "stuck repeating")
	iterVal, ok := state.Get("react.iteration")
	assert.True(t, ok)
	assert.Equal(t, 3, iterVal)
}

func TestTaskNeedsEditingIgnoresNegativeModifyInstructions(t *testing.T) {
	task := &core.Task{Instruction: "Run cargo test and explain the failure. Do not modify any files."}
	assert.False(t, taskNeedsEditing(task))
}

func TestVerificationSummaryFromSuccessCompletesAfterPassingCargo(t *testing.T) {
	task := &core.Task{
		Instruction: "Implement mul and run cargo test",
		Context: map[string]any{
			"agent_spec": &core.AgentRuntimeSpec{
				SkillConfig: core.AgentSkillConfig{
					Verification: core.AgentVerificationPolicy{
						SuccessTools:  []string{"cli_cargo"},
						StopOnSuccess: true,
					},
				},
			},
		},
	}
	state := core.NewContext()
	state.Set("react.tool_observations", []ToolObservation{
		{Tool: "file_write", Success: true},
		{Tool: "cli_cargo", Success: true, Data: map[string]interface{}{"stdout": "test result: ok"}},
	})

	agent := &ReActAgent{
		Config: &core.Config{
			AgentSpec: task.Context["agent_spec"].(*core.AgentRuntimeSpec),
		},
	}
	summary, ok := verificationSummaryFromSuccess(agent, task, state, map[string]interface{}{
		"cli_cargo": map[string]interface{}{
			"success": true,
			"data":    map[string]interface{}{"stdout": "test result: ok"},
		},
	})
	assert.True(t, ok)
	assert.Contains(t, summary, "cli_cargo succeeded")
}

func TestAvailableToolsForPhaseRespectsSkillPhaseCapabilities(t *testing.T) {
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(stubTool{name: "cli_cargo", tags: []string{core.TagExecute}}))
	assert.NoError(t, registry.Register(stubTool{name: "cli_rustfmt", tags: []string{core.TagExecute}}))

	agent := &ReActAgent{
		Tools: registry,
		Config: &core.Config{
			AgentSpec: &core.AgentRuntimeSpec{
				SkillConfig: core.AgentSkillConfig{
					PhaseCapabilities: map[string][]string{
						contextmgrPhaseVerify: {"cli_cargo"},
					},
				},
			},
		},
	}
	state := core.NewContext()
	state.Set("react.phase", contextmgrPhaseVerify)
	task := &core.Task{Instruction: "Run cargo test", Context: map[string]any{"mode": "debug"}}

	tools := agent.availableToolsForPhase(state, task)
	assert.Len(t, tools, 1)
	assert.Equal(t, "cli_cargo", tools[0].Name())
}

func TestAvailableToolsForPhaseRespectsSkillPhaseCapabilitySelectors(t *testing.T) {
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(stubTool{name: "go_test", tags: []string{core.TagExecute, "lang:go", "test"}}))
	assert.NoError(t, registry.Register(stubTool{name: "go_build", tags: []string{core.TagExecute, "lang:go", "build"}}))

	agent := &ReActAgent{
		Tools: registry,
		Config: &core.Config{
			AgentSpec: &core.AgentRuntimeSpec{
				SkillConfig: core.AgentSkillConfig{
					PhaseCapabilitySelectors: map[string][]core.SkillCapabilitySelector{
						contextmgrPhaseVerify: {{Tags: []string{"lang:go", "test"}}},
					},
				},
			},
		},
	}
	state := core.NewContext()
	state.Set("react.phase", contextmgrPhaseVerify)
	task := &core.Task{Instruction: "Run go test", Context: map[string]any{"mode": "debug"}}

	tools := agent.availableToolsForPhase(state, task)
	assert.Len(t, tools, 1)
	assert.Equal(t, "go_test", tools[0].Name())
}

func TestAvailableToolsForPhaseIncludesConfiguredRecoveryToolsOnFailure(t *testing.T) {
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(stubTool{name: "cli_cargo", tags: []string{core.TagExecute}}))
	assert.NoError(t, registry.Register(stubTool{name: "search_grep", tags: []string{core.TagReadOnly}}))

	agent := &ReActAgent{
		Tools: registry,
		Config: &core.Config{
			AgentSpec: &core.AgentRuntimeSpec{
				SkillConfig: core.AgentSkillConfig{
					PhaseCapabilities: map[string][]string{
						contextmgrPhaseVerify: {"cli_cargo", "search_grep"},
					},
					Recovery: core.AgentRecoveryPolicy{
						FailureProbeTools: []string{"search_grep"},
					},
				},
			},
		},
	}
	state := core.NewContext()
	state.Set("react.phase", contextmgrPhaseVerify)
	state.Set("react.last_tool_result", map[string]interface{}{"error": "cargo failed"})
	task := &core.Task{Instruction: "Run cargo test and explain the failure", Context: map[string]any{"mode": "debug"}}

	tools := agent.availableToolsForPhase(state, task)
	assert.Len(t, tools, 2)
}

func TestAvailableToolsForPhaseExcludesInspectableOnlyTools(t *testing.T) {
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(stubTool{name: "cli_cargo", tags: []string{core.TagExecute}}))
	registry.UseAgentSpec("agent", &core.AgentRuntimeSpec{
		ExposurePolicies: []core.CapabilityExposurePolicy{
			{
				Selector: core.CapabilitySelector{Name: "cli_cargo"},
				Access:   core.CapabilityExposureInspectable,
			},
		},
	})

	agent := &ReActAgent{Tools: registry}
	state := core.NewContext()
	state.Set("react.phase", contextmgrPhaseVerify)
	task := &core.Task{Instruction: "Run cargo test", Context: map[string]any{"mode": "debug"}}

	tools := agent.availableToolsForPhase(state, task)
	assert.Empty(t, tools)
}

func TestAvailableToolsForPhaseUsesExecutionCatalogSnapshot(t *testing.T) {
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(stubTool{name: "cli_cargo", tags: []string{core.TagExecute}}))

	agent := &ReActAgent{Tools: registry}
	agent.executionCatalog = registry.CaptureExecutionCatalogSnapshot()
	defer func() { agent.executionCatalog = nil }()

	registry.UseAgentSpec("agent", &core.AgentRuntimeSpec{
		ExposurePolicies: []core.CapabilityExposurePolicy{
			{
				Selector: core.CapabilitySelector{Name: "cli_cargo"},
				Access:   core.CapabilityExposureInspectable,
			},
		},
	})

	state := core.NewContext()
	state.Set("react.phase", contextmgrPhaseVerify)
	task := &core.Task{Instruction: "Run cargo test", Context: map[string]any{"mode": "debug"}}

	tools := agent.availableToolsForPhase(state, task)
	assert.Len(t, tools, 1)
	assert.Equal(t, "cli_cargo", tools[0].Name())
}

func TestCapabilityEnvelopeUsesExecutionCatalogSnapshot(t *testing.T) {
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(stubTool{name: "cli_cargo", tags: []string{core.TagExecute}}))
	registry.UseAgentSpec("agent", &core.AgentRuntimeSpec{
		ToolExecutionPolicy: map[string]core.ToolPolicy{
			"cli_cargo": {Execute: core.AgentPermissionAsk},
		},
	})

	agent := &ReActAgent{Tools: registry}
	agent.executionCatalog = registry.CaptureExecutionCatalogSnapshot()
	defer func() { agent.executionCatalog = nil }()

	registry.UpdateToolPolicy("cli_cargo", core.ToolPolicy{Execute: core.AgentPermissionDeny})

	node := &reactActNode{id: "act", agent: agent}
	state := core.NewContext()
	envelope := node.capabilityEnvelope(context.Background(), state, nil, core.ToolCall{Name: "cli_cargo"}, &core.ToolResult{Success: true})
	if envelope == nil || envelope.Policy == nil {
		t.Fatalf("expected capability envelope with policy snapshot")
	}
	assert.Equal(t, core.AgentPermissionAsk, envelope.Policy.ToolPolicies["cli_cargo"].Execute)
	assert.Equal(t, "tool:cli_cargo", envelope.Descriptor.ID)
}

func TestScheduleRecoveryProbeUsesSkillOrder(t *testing.T) {
	agent := &ReActAgent{Tools: makeRecoveryRegistry(t,
		stubTool{name: "rust_workspace_detect", params: []core.ToolParameter{{Name: "path", Type: "string", Required: false}}},
		stubTool{name: "rust_cargo_metadata", params: []core.ToolParameter{{Name: "working_directory", Type: "string", Required: false}}},
		stubTool{name: "rust_cargo_check", params: []core.ToolParameter{{Name: "working_directory", Type: "string", Required: false}}},
	)}
	task := &core.Task{
		Instruction: "Run cargo test and explain the failure. Do not modify any files.",
		Context: map[string]any{
			"agent_spec": &core.AgentRuntimeSpec{
				SkillConfig: core.AgentSkillConfig{
					Recovery: core.AgentRecoveryPolicy{
						FailureProbeTools: []string{"rust_workspace_detect", "rust_cargo_metadata", "rust_cargo_check"},
					},
				},
			},
		},
	}
	state := core.NewContext()
	state.Set("react.last_tool_result", map[string]interface{}{
		"rust_cargo_test": map[string]interface{}{
			"error": "exit status 101",
			"data": map[string]interface{}{
				"stderr": "error: failed to parse manifest at `/tmp/demo/Cargo.toml`",
			},
			"success": false,
		},
	})
	node := &reactObserveNode{id: "observe", agent: agent, task: task}

	scheduled := node.scheduleRecoveryProbe(state, map[string]interface{}{
		"rust_cargo_test": map[string]interface{}{
			"error": "exit status 101",
			"data": map[string]interface{}{
				"stderr": "error: failed to parse manifest at `/tmp/demo/Cargo.toml`",
			},
			"success": false,
		},
	})
	assert.True(t, scheduled)
	raw, ok := state.Get("react.tool_calls")
	assert.True(t, ok)
	calls := raw.([]core.ToolCall)
	assert.Len(t, calls, 1)
	assert.Equal(t, "rust_workspace_detect", calls[0].Name)
}

func TestScheduleRecoveryProbeUsesPythonSkillOrder(t *testing.T) {
	agent := &ReActAgent{Tools: makeRecoveryRegistry(t,
		stubTool{name: "python_workspace_detect", params: []core.ToolParameter{{Name: "path", Type: "string", Required: false}}},
		stubTool{name: "python_project_metadata", params: []core.ToolParameter{{Name: "path", Type: "string", Required: false}}},
		stubTool{name: "python_compile_check", params: []core.ToolParameter{{Name: "working_directory", Type: "string", Required: false}}},
	)}
	task := &core.Task{
		Instruction: "Run python tests and explain the failure. Do not modify any files.",
		Context: map[string]any{
			"agent_spec": &core.AgentRuntimeSpec{
				SkillConfig: core.AgentSkillConfig{
					Recovery: core.AgentRecoveryPolicy{
						FailureProbeTools: []string{"python_workspace_detect", "python_project_metadata", "python_compile_check"},
					},
				},
			},
		},
	}
	state := core.NewContext()
	state.Set("react.last_tool_result", map[string]interface{}{
		"python_pytest": map[string]interface{}{
			"error": "exit status 1",
			"data": map[string]interface{}{
				"stderr": "FAILED tests/test_math.py::test_add - assert 1 == 2",
			},
			"success": false,
		},
	})
	node := &reactObserveNode{id: "observe", agent: agent, task: task}

	scheduled := node.scheduleRecoveryProbe(state, map[string]interface{}{
		"python_pytest": map[string]interface{}{
			"error": "exit status 1",
			"data": map[string]interface{}{
				"stderr": "FAILED tests/test_math.py::test_add - assert 1 == 2",
			},
			"success": false,
		},
	})
	assert.True(t, scheduled)
	raw, ok := state.Get("react.tool_calls")
	assert.True(t, ok)
	calls := raw.([]core.ToolCall)
	assert.Len(t, calls, 1)
	assert.Equal(t, "python_workspace_detect", calls[0].Name)
}

func TestScheduleRecoveryProbeUsesNodeSkillOrder(t *testing.T) {
	agent := &ReActAgent{Tools: makeRecoveryRegistry(t,
		stubTool{name: "node_workspace_detect", params: []core.ToolParameter{{Name: "path", Type: "string", Required: false}}},
		stubTool{name: "node_project_metadata", params: []core.ToolParameter{{Name: "path", Type: "string", Required: false}}},
		stubTool{name: "node_syntax_check", params: []core.ToolParameter{
			{Name: "working_directory", Type: "string", Required: false},
			{Name: "path", Type: "string", Required: true},
		}},
	)}
	task := &core.Task{
		Instruction: "Run npm test and explain the failure. Do not modify any files.",
		Context: map[string]any{
			"agent_spec": &core.AgentRuntimeSpec{
				SkillConfig: core.AgentSkillConfig{
					Recovery: core.AgentRecoveryPolicy{
						FailureProbeTools: []string{"node_workspace_detect", "node_project_metadata", "node_syntax_check"},
					},
				},
			},
		},
	}
	state := core.NewContext()
	state.Set("react.failure_path", "src/app.js")
	state.Set("react.last_tool_result", map[string]interface{}{
		"node_npm_test": map[string]interface{}{
			"error": "exit status 1",
			"data": map[string]interface{}{
				"stderr": "FAIL src/math.test.js expected 2, received 1",
			},
			"success": false,
		},
	})
	node := &reactObserveNode{id: "observe", agent: agent, task: task}

	scheduled := node.scheduleRecoveryProbe(state, map[string]interface{}{
		"node_npm_test": map[string]interface{}{
			"error": "exit status 1",
			"data": map[string]interface{}{
				"stderr": "FAIL src/math.test.js expected 2, received 1",
			},
			"success": false,
		},
	})
	assert.True(t, scheduled)
	raw, ok := state.Get("react.tool_calls")
	assert.True(t, ok)
	calls := raw.([]core.ToolCall)
	assert.Len(t, calls, 1)
	assert.Equal(t, "node_workspace_detect", calls[0].Name)
}

func TestScheduleRecoveryProbeUsesGoSkillOrder(t *testing.T) {
	agent := &ReActAgent{Tools: makeRecoveryRegistry(t,
		stubTool{name: "go_workspace_detect", params: []core.ToolParameter{{Name: "path", Type: "string", Required: false}}},
		stubTool{name: "go_module_metadata", params: []core.ToolParameter{{Name: "working_directory", Type: "string", Required: false}}},
		stubTool{name: "go_build", params: []core.ToolParameter{{Name: "working_directory", Type: "string", Required: false}}},
	)}
	task := &core.Task{
		Instruction: "Run go test and explain the failure. Do not modify any files.",
		Context: map[string]any{
			"agent_spec": &core.AgentRuntimeSpec{
				SkillConfig: core.AgentSkillConfig{
					Recovery: core.AgentRecoveryPolicy{
						FailureProbeTools: []string{"go_workspace_detect", "go_module_metadata", "go_build"},
					},
				},
			},
		},
	}
	state := core.NewContext()
	state.Set("react.last_tool_result", map[string]interface{}{
		"go_test": map[string]interface{}{
			"error": "exit status 1",
			"data": map[string]interface{}{
				"stderr": "./main.go:12:2: undefined: missingSymbol",
			},
			"success": false,
		},
	})
	node := &reactObserveNode{id: "observe", agent: agent, task: task}

	scheduled := node.scheduleRecoveryProbe(state, map[string]interface{}{
		"go_test": map[string]interface{}{
			"error": "exit status 1",
			"data": map[string]interface{}{
				"stderr": "./main.go:12:2: undefined: missingSymbol",
			},
			"success": false,
		},
	})
	assert.True(t, scheduled)
	raw, ok := state.Get("react.tool_calls")
	assert.True(t, ok)
	calls := raw.([]core.ToolCall)
	assert.Len(t, calls, 1)
	assert.Equal(t, "go_workspace_detect", calls[0].Name)
}

func TestScheduleRecoveryProbeUsesSQLiteSkillOrder(t *testing.T) {
	agent := &ReActAgent{Tools: makeRecoveryRegistry(t,
		stubTool{name: "sqlite_database_detect", params: []core.ToolParameter{{Name: "path", Type: "string", Required: false}}},
		stubTool{name: "sqlite_schema_inspect", params: []core.ToolParameter{{Name: "database_path", Type: "string", Required: true}}},
		stubTool{name: "sqlite_integrity_check", params: []core.ToolParameter{{Name: "database_path", Type: "string", Required: true}}},
	)}
	task := &core.Task{
		Instruction: "Run sqlite validation and explain the failure. Do not modify any files.",
		Context: map[string]any{
			"agent_spec": &core.AgentRuntimeSpec{
				SkillConfig: core.AgentSkillConfig{
					Recovery: core.AgentRecoveryPolicy{
						FailureProbeTools: []string{"sqlite_database_detect", "sqlite_schema_inspect", "sqlite_integrity_check"},
					},
				},
			},
		},
	}
	state := core.NewContext()
	state.Set("react.failure_path", "data/app.db")
	state.Set("react.last_tool_result", map[string]interface{}{
		"sqlite_integrity_check": map[string]interface{}{
			"error": "exit status 1",
			"data": map[string]interface{}{
				"stdout": "row 1 missing from index idx_users_email",
			},
			"success": false,
		},
	})
	node := &reactObserveNode{id: "observe", agent: agent, task: task}

	scheduled := node.scheduleRecoveryProbe(state, map[string]interface{}{
		"sqlite_integrity_check": map[string]interface{}{
			"error": "exit status 1",
			"data": map[string]interface{}{
				"stdout": "row 1 missing from index idx_users_email",
			},
			"success": false,
		},
	})
	assert.True(t, scheduled)
	raw, ok := state.Get("react.tool_calls")
	assert.True(t, ok)
	calls := raw.([]core.ToolCall)
	assert.Len(t, calls, 1)
	assert.Equal(t, "sqlite_database_detect", calls[0].Name)
}

func TestReActResolvedSkillPolicyUsesTaskAgentSpecOverride(t *testing.T) {
	registry := capability.NewRegistry()
	assert.NoError(t, registry.Register(stubTool{name: "cli_go"}))
	assert.NoError(t, registry.Register(stubTool{name: "go_test", tags: []string{"lang:go", "test"}}))
	agent := &ReActAgent{
		Tools: registry,
		Config: &core.Config{
			AgentSpec: &core.AgentRuntimeSpec{
				SkillConfig: core.AgentSkillConfig{
					Verification: core.AgentVerificationPolicy{
						SuccessTools: []string{"cli_go"},
					},
				},
			},
		},
	}
	task := &core.Task{
		Context: map[string]any{
			"agent_spec": &core.AgentRuntimeSpec{
				SkillConfig: core.AgentSkillConfig{
					Verification: core.AgentVerificationPolicy{
						SuccessCapabilitySelectors: []core.SkillCapabilitySelector{{Tags: []string{"lang:go", "test"}}},
					},
				},
			},
		},
	}

	policy := agent.resolvedSkillPolicy(task)

	assert.Equal(t, []string{"go_test"}, policy.VerificationSuccessCapabilities)
}

func TestReactObserveCompletesWhenVerificationLatchIsSet(t *testing.T) {
	agent := &ReActAgent{
		Config: &core.Config{},
	}
	task := &core.Task{
		Instruction: "Fix the bug and run tests",
		Context: map[string]any{
			"agent_spec": &core.AgentRuntimeSpec{
				SkillConfig: core.AgentSkillConfig{
					Verification: core.AgentVerificationPolicy{StopOnSuccess: true},
				},
			},
		},
	}
	state := core.NewContext()
	state.Set("task.id", "react-latch")
	state.Set("react.decision", decisionPayload{})
	state.Set("react.last_tool_result", map[string]interface{}{"stdout": "ok"})
	state.Set("react.verification_latched_summary", "go_test succeeded after applying changes")

	node := &reactObserveNode{id: "observe", agent: agent, task: task}
	result, err := node.Execute(context.Background(), state)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Success)
	done, _ := state.Get("react.done")
	assert.Equal(t, true, done)
	assert.Equal(t, "go_test succeeded after applying changes", state.GetString("react.verification_latched_summary"))
	final, ok := state.Get("react.final_output")
	assert.True(t, ok)
	assert.Contains(t, fmt.Sprint(final), "go_test succeeded after applying changes")
}

func TestMirrorReactFinalOutputReferenceUsesGraphSummaryArtifact(t *testing.T) {
	state := core.NewContext()
	state.Set("react.final_output", map[string]any{
		"summary": "done",
		"result":  map[string]any{"ok": true},
	})
	state.Set("graph.summary_ref", core.ArtifactReference{
		ArtifactID:  "artifact-1",
		WorkflowID:  "workflow-1",
		Kind:        "summary",
		ContentType: "text/plain",
	})
	state.Set("graph.summary", "react summary artifact")

	mirrorReactFinalOutputReference(state)

	rawRef, ok := state.Get("react.final_output_ref")
	assert.True(t, ok)
	ref, ok := rawRef.(core.ArtifactReference)
	assert.True(t, ok)
	assert.Equal(t, "artifact-1", ref.ArtifactID)
	assert.Equal(t, "summary", ref.Kind)
	assert.Equal(t, "react summary artifact", state.GetString("react.final_output_summary"))
}

func TestCompactReactFinalOutputStateWhenArtifactRefExists(t *testing.T) {
	state := core.NewContext()
	state.Set("react.final_output", map[string]any{
		"summary": "done",
		"result":  map[string]any{"large": "payload"},
	})
	state.Set("react.final_output_ref", core.ArtifactReference{
		ArtifactID: "artifact-1",
		Kind:       "summary",
	})

	compactReactFinalOutputState(state)

	raw, ok := state.Get("react.final_output")
	assert.True(t, ok)
	payload, ok := raw.(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "done", payload["summary"])
	_, hasResult := payload["result"]
	assert.False(t, hasResult)
}

func TestCompactReactToolObservations(t *testing.T) {
	compacted := compactReactToolObservations([]ToolObservation{
		{Tool: "file_read", Success: true},
		{Tool: "cli_go", Success: false},
	})

	assert.Equal(t, 2, compacted["observation_count"])
	assert.Equal(t, "cli_go", compacted["last_tool"])
	assert.Equal(t, false, compacted["last_success"])
	assert.Equal(t, []string{"file_read", "cli_go"}, compacted["recent_tools"])
}

func TestCompactReactToolObservationsStateWhenArtifactRefExists(t *testing.T) {
	state := core.NewContext()
	state.Set("react.tool_observations", []ToolObservation{
		{Tool: "file_read", Success: true},
		{Tool: "cli_go", Success: false},
	})
	state.Set("react.final_output_ref", core.ArtifactReference{
		ArtifactID: "artifact-1",
		Kind:       "summary",
	})

	compactReactToolObservationsState(state)

	raw, ok := state.Get("react.tool_observations")
	assert.True(t, ok)
	payload, ok := raw.(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, 2, payload["observation_count"])
	assert.Equal(t, "cli_go", payload["last_tool"])
}

func TestCompactReactLastToolResult(t *testing.T) {
	compacted := compactReactLastToolResult(map[string]interface{}{
		"stdout":  "ok",
		"results": []string{"a", "b"},
		"success": true,
	})

	assert.Equal(t, 3, compacted["key_count"])
	assert.Equal(t, []string{"results", "stdout", "success"}, compacted["keys"])
	assert.Equal(t, true, compacted["success"])
}

func TestCompactReactLastToolResultStateWhenArtifactRefExists(t *testing.T) {
	state := core.NewContext()
	state.Set("react.last_tool_result", map[string]interface{}{
		"stdout":  "ok",
		"results": []string{"a", "b"},
		"success": true,
	})
	state.Set("react.final_output_ref", core.ArtifactReference{
		ArtifactID: "artifact-1",
		Kind:       "summary",
	})

	compactReactLastToolResultState(state)

	raw, ok := state.Get("react.last_tool_result")
	assert.True(t, ok)
	payload, ok := raw.(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, 3, payload["key_count"])
	assert.Equal(t, true, payload["success"])
}

func TestCompactReactLoopStateWhenArtifactRefExists(t *testing.T) {
	state := core.NewContext()
	state.Set("react.final_output_ref", core.ArtifactReference{
		ArtifactID: "artifact-1",
		Kind:       "summary",
	})
	state.Set("react.decision", decisionPayload{Tool: "echo"})
	state.Set("react.tool_calls", []core.ToolCall{{Name: "echo"}})
	state.Set("react.last_tool_result_envelope", &core.CapabilityResultEnvelope{})
	state.Set("react.last_tool_result_envelopes", []*core.CapabilityResultEnvelope{{}})

	compactReactLoopState(state)

	rawDecision, ok := state.Get("react.decision")
	assert.True(t, ok)
	assert.Equal(t, map[string]any{"present": true}, rawDecision)

	rawCalls, ok := state.Get("react.tool_calls")
	assert.True(t, ok)
	assert.Equal(t, map[string]any{"count": 1}, rawCalls)

	rawEnvelope, ok := state.Get("react.last_tool_result_envelope")
	assert.True(t, ok)
	assert.Equal(t, map[string]any{"present": true}, rawEnvelope)

	rawEnvelopes, ok := state.Get("react.last_tool_result_envelopes")
	assert.True(t, ok)
	assert.Equal(t, map[string]any{"count": 1}, rawEnvelopes)
}

func TestMirrorReactCheckpointReferenceUsesGraphCheckpointArtifact(t *testing.T) {
	state := core.NewContext()
	state.Set("graph.checkpoint_ref", core.ArtifactReference{
		ArtifactID:  "checkpoint-1",
		WorkflowID:  "workflow-1",
		Kind:        "checkpoint",
		ContentType: "application/json",
	})

	mirrorReactCheckpointReference(state)

	rawRef, ok := state.Get("react.checkpoint_ref")
	assert.True(t, ok)
	ref, ok := rawRef.(core.ArtifactReference)
	assert.True(t, ok)
	assert.Equal(t, "checkpoint-1", ref.ArtifactID)
	assert.Equal(t, "checkpoint", ref.Kind)
}

func TestReadOnlySummaryFromStateUsesLatestFileRead(t *testing.T) {
	task := &core.Task{Instruction: "Summarize README.md in 5 bullets."}
	state := core.NewContext()
	state.Set("react.tool_observations", []ToolObservation{
		{
			Tool:    "file_read",
			Success: true,
			Args:    map[string]interface{}{"path": "README.md"},
			Data:    map[string]interface{}{"snippet": "Relurpify is a local agentic automation framework."},
		},
	})

	summary, ok := readOnlySummaryFromState(task, state, map[string]interface{}{"content": "ignored"})

	assert.True(t, ok)
	assert.Contains(t, summary, "README.md")
	assert.Contains(t, summary, "Relurpify")
}

func TestEditSummaryFromSuccessAppliesWithoutVerificationRequirement(t *testing.T) {
	task := &core.Task{Instruction: "Edit foo.txt by appending DONE."}
	state := core.NewContext()
	state.Set("react.tool_observations", []ToolObservation{
		{Tool: "file_write", Success: true},
	})

	summary, ok := editSummaryFromSuccess(task, state, map[string]interface{}{})

	assert.True(t, ok)
	assert.Contains(t, summary, "file_write")
}

func TestEditSummaryFromSuccessRequiresVerificationWhenPromptAsks(t *testing.T) {
	task := &core.Task{Instruction: "Edit foo.txt and run tests to verify it."}
	state := core.NewContext()
	state.Set("react.tool_observations", []ToolObservation{
		{Tool: "file_write", Success: true},
	})

	_, ok := editSummaryFromSuccess(task, state, map[string]interface{}{})

	assert.False(t, ok)
}

func TestCompletionSummaryFromStateFallsBackForRepeatedReadOnlyTask(t *testing.T) {
	task := &core.Task{Instruction: "Summarize README.md in 5 bullets."}
	state := core.NewContext()
	state.Set("react.tool_observations", []ToolObservation{
		{
			Tool:    "file_read",
			Success: true,
			Args:    map[string]interface{}{"path": "README.md"},
			Data:    map[string]interface{}{"snippet": "Relurpify is a local agentic automation framework."},
		},
		{
			Tool:    "file_read",
			Success: true,
			Args:    map[string]interface{}{"path": "README.md"},
			Data:    map[string]interface{}{"snippet": "Relurpify is a local agentic automation framework."},
		},
		{
			Tool:    "file_read",
			Success: true,
			Args:    map[string]interface{}{"path": "README.md"},
			Data:    map[string]interface{}{"snippet": "Relurpify is a local agentic automation framework."},
		},
	})

	summary, ok := completionSummaryFromState(nil, task, state, map[string]interface{}{
		"file_read": map[string]interface{}{
			"data":    map[string]interface{}{"content": "ignored"},
			"error":   "",
			"success": true,
		},
	})

	assert.True(t, ok)
	assert.Contains(t, summary, "README.md")
}

func TestVerificationSummaryFromSuccessAllowsPromptDrivenVerificationStop(t *testing.T) {
	task := &core.Task{Instruction: "Fix the bug and run tests."}
	state := core.NewContext()
	state.Set("react.tool_observations", []ToolObservation{
		{Tool: "file_write", Success: true},
		{Tool: "cli_cargo", Success: true},
	})
	agent := &ReActAgent{}

	summary, ok := verificationSummaryFromSuccess(agent, task, state, map[string]interface{}{
		"cli_cargo": map[string]interface{}{
			"data": map[string]interface{}{
				"stdout": "test result: ok",
			},
			"error":   "",
			"success": true,
		},
	})

	assert.True(t, ok)
	assert.Contains(t, summary, "cli_cargo")
}

func TestLatchVerificationSuccessAllowsPromptDrivenVerificationStop(t *testing.T) {
	state := core.NewContext()
	state.Set("react.tool_observations", []ToolObservation{
		{Tool: "file_write", Success: true},
	})
	task := &core.Task{Instruction: "Fix the bug and run tests."}
	node := &reactActNode{
		agent: &ReActAgent{},
		task:  task,
	}

	node.latchVerificationSuccess(state, "cli_cargo", &core.ToolResult{Success: true})

	assert.Equal(t, "cli_cargo succeeded after applying changes", state.GetString("react.verification_latched_summary"))
	assert.Equal(t, "cli_cargo succeeded after applying changes", state.GetString("react.synthetic_summary"))
}

func TestDirectCompletionSummaryHandlesReadOnlySummaryLegacy(t *testing.T) {
	task := &core.Task{Instruction: "Summarize README.md in 5 bullets."}
	state := core.NewContext()
	state.Set("react.tool_observations", []ToolObservation{
		{
			Tool:    "file_read",
			Success: true,
			Args:    map[string]interface{}{"path": "README.md"},
			Data:    map[string]interface{}{"snippet": "Relurpify is a local agentic automation framework."},
		},
	})

	summary, ok := directCompletionSummary(task, state)

	assert.True(t, ok)
	assert.Contains(t, summary, "README.md")
	assert.Contains(t, summary, "Relurpify")
}

func TestToolAllowedByExecutionContextBlocksVerifyToolsBeforeEditAfterFailure(t *testing.T) {
	state := core.NewContext()
	state.Set("react.last_tool_result", map[string]interface{}{
		"cli_cargo": map[string]interface{}{
			"success": false,
			"error":   "exit status 101",
		},
	})
	agent := &ReActAgent{}
	task := &core.Task{Instruction: "Fix the failing tests and run tests again."}

	assert.False(t, agent.toolAllowedByExecutionContext(state, task, contextmgrPhaseEdit, stubTool{name: "cli_rustfmt"}))
	assert.False(t, agent.toolAllowedByExecutionContext(state, task, contextmgrPhaseEdit, stubTool{name: "cli_cargo"}))
	assert.True(t, agent.toolAllowedByExecutionContext(state, task, contextmgrPhaseEdit, stubTool{name: "file_read"}))
	assert.True(t, agent.toolAllowedByExecutionContext(state, task, contextmgrPhaseEdit, stubTool{name: "file_write"}))
}

func TestTaskRequiresVerificationIgnoresTestsuitePathText(t *testing.T) {
	task := &core.Task{Instruction: "Edit testsuite/agenttest_fixtures/hello.txt by appending DONE."}

	assert.False(t, taskRequiresVerification(task))
}

func TestTaskNeedsEditingDoesNotMisclassifySummaryPrompt(t *testing.T) {
	task := &core.Task{Instruction: "Summarize README.md in 5 bullets."}

	assert.False(t, taskNeedsEditing(task))
}

func TestTaskNeedsEditingUsesOriginalUserInstructionWhenDecorated(t *testing.T) {
	task := &core.Task{
		Instruction: "[Mode: Coding]\nDescription: General-purpose code work (read, write, debug, explain, plan)\n\nSummarize README.md in 5 bullets.",
		Context: map[string]interface{}{
			"user_instruction": "Summarize README.md in 5 bullets.",
		},
	}

	assert.False(t, taskNeedsEditing(task))
	assert.True(t, taskLooksLikeReadOnlySummary(task))
}

func TestInitializePhaseStartsVerifyForNoEditVerificationTask(t *testing.T) {
	agent := &ReActAgent{}
	state := core.NewContext()
	task := &core.Task{
		Instruction: "Run cli_cargo args [\"test\"] with working_directory \"testsuite/agenttest_fixtures/rustsuite\" and confirm whether the crate is already correct. Do not modify any files.",
	}

	agent.initializePhase(state, task)

	assert.Equal(t, contextmgrPhaseVerify, state.GetString("react.phase"))
}

func TestRepeatedReadCompletionSummaryHandlesReadOnlyTask(t *testing.T) {
	task := &core.Task{Instruction: "Summarize README.md in 5 bullets."}
	state := core.NewContext()
	state.Set("react.tool_observations", []ToolObservation{
		{Tool: "file_read", Success: true, Args: map[string]interface{}{"path": "README.md"}, Data: map[string]interface{}{"snippet": "Relurpify overview"}},
		{Tool: "file_read", Success: true, Args: map[string]interface{}{"path": "README.md"}, Data: map[string]interface{}{"snippet": "Relurpify overview"}},
		{Tool: "file_read", Success: true, Args: map[string]interface{}{"path": "README.md"}, Data: map[string]interface{}{"snippet": "Relurpify overview"}},
	})

	summary, ok := repeatedReadCompletionSummary(task, state)

	assert.True(t, ok)
	assert.Contains(t, summary, "README.md")
}

func TestToolAllowedByExecutionContextHonorsExplicitRequestedVerifyTool(t *testing.T) {
	agent := &ReActAgent{}
	task := &core.Task{Instruction: "Run cli_cargo args [\"test\"] after the fix."}

	assert.True(t, agent.toolAllowedByExecutionContext(core.NewContext(), task, contextmgrPhaseVerify, stubTool{name: "cli_cargo", tags: []string{core.TagExecute}}))
	assert.False(t, agent.toolAllowedByExecutionContext(core.NewContext(), task, contextmgrPhaseVerify, stubTool{name: "cli_rustfmt", tags: []string{core.TagExecute}}))
}

func TestVerificationToolMatchesTreatsGitAsVerification(t *testing.T) {
	assert.True(t, verificationToolMatches("cli_git", nil))
}

func TestToolAllowedByExecutionContextHonorsExplicitRequestedReadOnlyTools(t *testing.T) {
	agent := &ReActAgent{}
	task := &core.Task{Instruction: "Use search_semantic to find the retrieval docs, then use file_read on the result. Do not modify any files."}

	assert.True(t, agent.toolAllowedByExecutionContext(core.NewContext(), task, contextmgrPhaseExplore, stubTool{name: "search_semantic", tags: []string{core.TagReadOnly}}))
	assert.True(t, agent.toolAllowedByExecutionContext(core.NewContext(), task, contextmgrPhaseExplore, stubTool{name: "file_read", tags: []string{core.TagReadOnly}}))
	assert.False(t, agent.toolAllowedByExecutionContext(core.NewContext(), task, contextmgrPhaseExplore, stubTool{name: "file_search", tags: []string{core.TagReadOnly}}))
}

func TestRequestedReadOnlyToolsSatisfiedRequiresExplicitSearchAndRead(t *testing.T) {
	task := &core.Task{Instruction: "Use search_semantic to find the retrieval docs, then use file_read on the result. Do not modify any files."}
	state := core.NewContext()
	state.Set("react.tool_observations", []ToolObservation{
		{Tool: "search_semantic", Success: true},
	})

	assert.False(t, requestedReadOnlyToolsSatisfied(task, state))

	state.Set("react.tool_observations", []ToolObservation{
		{Tool: "search_semantic", Success: true},
		{Tool: "file_read", Success: true},
	})

	assert.True(t, requestedReadOnlyToolsSatisfied(task, state))
}

func TestToolAllowedByExecutionContextBlocksRepeatedFileReadBeforeEdit(t *testing.T) {
	agent := &ReActAgent{}
	task := &core.Task{Instruction: "Fix query.sql and then run cli_sqlite3."}
	state := core.NewContext()
	state.Set("react.tool_observations", []ToolObservation{
		{Tool: "file_read", Success: true, Args: map[string]interface{}{"path": "query.sql"}},
		{Tool: "file_read", Success: true, Args: map[string]interface{}{"path": "query.sql"}},
	})

	assert.False(t, agent.toolAllowedByExecutionContext(state, task, contextmgrPhaseEdit, stubTool{name: "file_read"}))
	assert.True(t, agent.toolAllowedByExecutionContext(state, task, contextmgrPhaseEdit, stubTool{name: "file_write"}))
}

func TestDirectCompletionSummaryHandlesReadOnlySummaryAgain(t *testing.T) {
	task := &core.Task{Instruction: "Summarize README.md in 5 bullets."}
	state := core.NewContext()
	state.Set("react.tool_observations", []ToolObservation{
		{
			Tool:    "file_read",
			Success: true,
			Args:    map[string]interface{}{"path": "README.md"},
			Data:    map[string]interface{}{"snippet": "Relurpify overview"},
		},
	})

	summary, ok := directCompletionSummary(task, state)

	assert.True(t, ok)
	assert.Contains(t, summary, "README.md")
}

func TestVerificationToolMatchesRecognizesCLIWrappers(t *testing.T) {
	assert.True(t, verificationToolMatches("cli_go", nil))
	assert.True(t, verificationToolMatches("cli_python", nil))
	assert.True(t, verificationToolMatches("cli_node", nil))
	assert.True(t, verificationToolMatches("cli_sqlite3", nil))
}

func TestVerificationSuccessSummaryUsesSQLiteStdout(t *testing.T) {
	summary := verificationSuccessSummary("cli_sqlite3", "alice|2\nbob|1\n")

	assert.Equal(t, "alice|2\nbob|1", summary)
}

func TestVerificationSummaryWithoutEditsUsesVerificationOutput(t *testing.T) {
	task := &core.Task{Instruction: "Run cli_go args [\"test\",\"./pkg\"] and confirm whether the package is already correct. Do not modify any files."}
	state := core.NewContext()
	state.Set("react.tool_observations", []ToolObservation{
		{
			Tool:    "cli_go",
			Success: true,
			Data: map[string]interface{}{
				"stdout": "ok\tgithub.com/example/pkg\t(cached)\n",
			},
		},
	})

	summary, ok := verificationSummaryWithoutEdits(&ReActAgent{}, task, state, map[string]interface{}{
		"cli_go": map[string]interface{}{
			"data":    map[string]interface{}{"stdout": "ok\tgithub.com/example/pkg\t(cached)\n"},
			"success": true,
			"error":   "",
		},
	})

	assert.True(t, ok)
	assert.Contains(t, summary, "ok")
}

// TestNormalizeDecisionRepairFailDoesNotSilentlyDropEmbeddedToolCall verifies
// that when repairDecision fails and the original response text contains a
// tool-like JSON fragment, the iteration is NOT silently marked complete.
// TestDocsModeDeniesWriteToolsViaToolAllowedByExecutionContext verifies that
// when Mode is "docs", file_write / file_create / file_delete are blocked at
// the tool-filter level regardless of phase.
func TestDocsModeDeniesWriteToolsViaToolAllowedByExecutionContext(t *testing.T) {
	agent := &ReActAgent{Mode: "docs"}
	assert.NoError(t, agent.Initialize(&core.Config{Name: "docs-test"}))
	state := core.NewContext()
	task := &core.Task{Instruction: "Update the package documentation."}

	for _, writeName := range []string{"file_write", "file_create", "file_delete"} {
		assert.False(t,
			agent.toolAllowedByExecutionContext(state, task, contextmgrPhaseEdit, stubTool{name: writeName, tags: []string{core.TagDestructive}}),
			"docs mode must block %s", writeName,
		)
	}
	// Read-only tools must still be allowed.
	assert.True(t,
		agent.toolAllowedByExecutionContext(state, task, contextmgrPhaseExplore, stubTool{name: "file_read", tags: []string{core.TagReadOnly}}),
		"docs mode must not block file_read",
	)
}

func TestNormalizeDecisionRepairFailDoesNotSilentlyDropEmbeddedToolCall(t *testing.T) {
	// stubLLM with empty queue → Generate returns error, simulating repair failure.
	agent := &ReActAgent{
		Model:  &stubLLM{responses: []*core.LLMResponse{}},
		Config: &core.Config{Model: "test"},
		Tools:  capability.NewRegistry(),
	}
	node := &reactThinkNode{id: "think", agent: agent, task: &core.Task{Instruction: "Read and report."}}

	// Broken JSON (unquoted key) so both ParseToolCallsFromText and parseDecision fail,
	// but the text clearly contains a "tool" key with a non-empty value.
	brokenToolJSON := `Thinking... {"tool": "file_read", arguments: {path: "main.go"}, complete: false}`
	resp := &core.LLMResponse{Text: brokenToolJSON}

	decision, _, err := node.normalizeDecision(context.Background(), core.NewContext(), resp, false, nil)

	assert.NoError(t, err)
	assert.False(t, decision.Complete, "tool call embedded in prose must not be silently dropped when repair fails")
}

func TestTextSuggestsPendingToolCallDetectsEmbeddedToolIntent(t *testing.T) {
	assert.True(t, textSuggestsPendingToolCall(`{"tool": "file_read", arguments: {path: "foo.go"}}`))
	assert.False(t, textSuggestsPendingToolCall(`{"tool": "", "complete": true}`))
	assert.False(t, textSuggestsPendingToolCall(`{"action":"complete","tool":"","complete":true}`))
	assert.False(t, textSuggestsPendingToolCall(`No JSON here, just prose.`))
	assert.True(t, textSuggestsPendingToolCall(`I will call {"tool": "file_write", "action":"tool"}`))
}

func TestIsLanguageExecutionToolAllowsExplicitSQLiteTool(t *testing.T) {
	task := &core.Task{
		Instruction: "Verify by running cli_sqlite3 args [\":memory:\"]",
	}

	assert.True(t, isLanguageExecutionTool("cli_sqlite3", task))
}
