package shell

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

func TestCompositeToolName(t *testing.T) {
	tool := &CompositeTool{ToolName: "build_and_test", ToolDescription: "Build then test"}
	if tool.Name() != "build_and_test" {
		t.Fatalf("expected Name 'build_and_test', got %q", tool.Name())
	}
	if tool.Description() != "Build then test" {
		t.Fatalf("expected Description 'Build then test', got %q", tool.Description())
	}
}

func TestCompositeToolSteps(t *testing.T) {
	called := make([]string, 0)
	lookup := func(name string) (contracts.Tool, bool) {
		return &recordingTool{name: name, calls: &called}, true
	}
	tool := &CompositeTool{
		ToolName: "ci_pipeline",
		Steps: []contracts.ToolManifestCompositionStep{
			{Tool: "lint", Alias: "linter"},
			{Tool: "test", Alias: "tester"},
		},
		Lookup: lookup,
	}
	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got %v", result.Error)
	}
	if len(called) != 2 || called[0] != "lint" || called[1] != "test" {
		t.Fatalf("expected [lint test], got %v", called)
	}
}

func TestCompositeToolFailsOnFirstError(t *testing.T) {
	called := make([]string, 0)
	lookup := func(name string) (contracts.Tool, bool) {
		return &recordingTool{
			name:   name,
			result: &contracts.ToolResult{Success: false, Error: "step failed"},
			calls:  &called,
		}, true
	}
	tool := &CompositeTool{
		ToolName: "failing",
		Steps: []contracts.ToolManifestCompositionStep{
			{Tool: "first"},
			{Tool: "second"},
		},
		Lookup: lookup,
	}
	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute should not return error for non-error step result: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure result")
	}
	if result.Error != "step failed" {
		t.Fatalf("expected error 'step failed', got %q", result.Error)
	}
}

func TestCompositeToolMissingStepTool(t *testing.T) {
	tool := &CompositeTool{
		ToolName: "missing",
		Steps:  []contracts.ToolManifestCompositionStep{{Tool: "nonexistent"}},
		Lookup: func(name string) (contracts.Tool, bool) { return nil, false },
	}
	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute should not return error for missing tool: %v", err)
	}
	if result.Success {
		t.Fatal("expected failure for missing tool")
	}
}

func TestStreamingToolInterface(t *testing.T) {
	// Verify that a tool can implement the optional StreamingTool interface
	tool := &streamingEchoTool{}
	var st contracts.StreamingTool = tool
	if st == nil {
		t.Fatal("streamingEchoTool should implement StreamingTool")
	}
	ch, err := st.ExecuteStream(context.Background(), map[string]interface{}{"msg": "hello"})
	if err != nil {
		t.Fatalf("ExecuteStream: %v", err)
	}
	var results []contracts.ToolChunk
	for chunk := range ch {
		results = append(results, chunk)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one chunk")
	}
	last := results[len(results)-1]
	if !last.Done {
		t.Fatal("expected final chunk to have Done=true")
	}
}

// recordingTool records calls for testing.
type recordingTool struct {
	name   string
	result *contracts.ToolResult
	calls  *[]string
}

func (r *recordingTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	if r.calls != nil {
		*r.calls = append(*r.calls, r.name)
	}
	if r.result != nil {
		return r.result, nil
	}
	return &contracts.ToolResult{Success: true, Data: map[string]interface{}{"stdout": r.name + "_output"}}, nil
}

func (r *recordingTool) Name() string                           { return r.name }
func (r *recordingTool) Description() string                    { return "recording tool" }
func (r *recordingTool) Category() string                       { return "test" }
func (r *recordingTool) Parameters() []contracts.ToolParameter  { return nil }
func (r *recordingTool) IsAvailable(ctx context.Context) bool    { return true }
func (r *recordingTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{Permissions: &contracts.PermissionSet{
		Executables: []contracts.ExecutablePermission{{Binary: "echo"}},
	}}
}
func (r *recordingTool) Tags() []string { return nil }

// streamingEchoTool implements StreamingTool for testing.
type streamingEchoTool struct{}

func (s *streamingEchoTool) Name() string        { return "stream_echo" }
func (s *streamingEchoTool) Description() string { return "Echo with streaming" }
func (s *streamingEchoTool) Category() string    { return "test" }
func (s *streamingEchoTool) Parameters() []contracts.ToolParameter {
	return []contracts.ToolParameter{{Name: "msg", Type: contracts.ToolParamString}}
}
func (s *streamingEchoTool) Execute(ctx context.Context, args map[string]interface{}) (*contracts.ToolResult, error) {
	msg, _ := args["msg"].(string)
	return &contracts.ToolResult{Success: true, Data: map[string]interface{}{"output": msg}}, nil
}
func (s *streamingEchoTool) ExecuteStream(ctx context.Context, args map[string]interface{}) (<-chan contracts.ToolChunk, error) {
	ch := make(chan contracts.ToolChunk)
	go func() {
		defer close(ch)
		msg, _ := args["msg"].(string)
		for i, r := range msg {
			ch <- contracts.ToolChunk{
				Data:   map[string]interface{}{"char": string(r)},
				SeqNum: i,
			}
		}
		ch <- contracts.ToolChunk{Done: true, SeqNum: len(msg)}
	}()
	return ch, nil
}
func (s *streamingEchoTool) IsAvailable(ctx context.Context) bool { return true }
func (s *streamingEchoTool) Permissions() contracts.ToolPermissions {
	return contracts.ToolPermissions{Permissions: &contracts.PermissionSet{
		Executables: []contracts.ExecutablePermission{{Binary: "echo"}},
	}}
}
func (s *streamingEchoTool) Tags() []string { return nil }
