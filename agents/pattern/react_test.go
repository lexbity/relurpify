package pattern

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/contextmgr"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
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
func (s *stubLLM) ChatWithTools(ctx context.Context, messages []core.Message, tools []core.Tool, options *core.LLMOptions) (*core.LLMResponse, error) {
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

func makeRecoveryRegistry(t *testing.T, tools ...stubTool) *capability.Registry {
	t.Helper()
	registry := capability.NewRegistry()
	for _, tool := range tools {
		assert.NoError(t, registry.Register(tool))
	}
	return registry
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
	cfg := &core.Config{Model: "test-model", MaxIterations: 2, OllamaToolCalling: true}
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
	cfg := &core.Config{Model: "test-model", MaxIterations: 2, OllamaToolCalling: false}
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
	cfg := &core.Config{Model: "test-model", MaxIterations: 2, OllamaToolCalling: false}
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
	cfg := &core.Config{Model: "test-model", MaxIterations: 3, OllamaToolCalling: true}
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
	cfg := &core.Config{Model: "test-model", MaxIterations: 3, OllamaToolCalling: true}
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
	cfg := &core.Config{Model: "test-model", MaxIterations: 5, OllamaToolCalling: false}
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

func TestPlannerSkillHintsIncludePlanningPolicy(t *testing.T) {
	agent := &PlannerAgent{
		Tools: capability.NewRegistry(),
		Config: &core.Config{
			AgentSpec: &core.AgentRuntimeSpec{
				SkillConfig: core.AgentSkillConfig{
					Planning: core.AgentPlanningPolicy{
						RequiredBeforeEdit:          []core.SkillCapabilitySelector{{Capability: "file_read"}},
						PreferredVerifyCapabilities: []core.SkillCapabilitySelector{{Capability: "cli_go"}},
						StepTemplates:               []core.SkillStepTemplate{{Kind: "verify", Description: "Run tests"}},
						RequireVerificationStep:     true,
					},
				},
			},
		},
	}
	assert.NoError(t, agent.Tools.Register(stubTool{name: "file_read"}))
	assert.NoError(t, agent.Tools.Register(stubTool{name: "cli_go"}))

	hints := plannerSkillHints(agent)
	assert.Contains(t, hints, "Required before edit: file_read")
	assert.Contains(t, hints, "Preferred verify capabilities: cli_go")
	assert.Contains(t, hints, "Plans must include an explicit verification step.")
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

func TestDirectCompletionSummaryHandlesReadOnlySummary(t *testing.T) {
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

func TestFinalResultFallbackSummaryHandlesReadOnlySummary(t *testing.T) {
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

	summary, ok := finalResultFallbackSummary(task, state)

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

func TestIsLanguageExecutionToolAllowsExplicitSQLiteTool(t *testing.T) {
	task := &core.Task{
		Instruction: "Verify by running cli_sqlite3 args [\":memory:\"]",
	}

	assert.True(t, isLanguageExecutionTool("cli_sqlite3", task))
}
