package framework

import (
	"context"
	"testing"

	graph "codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	execution "codeburg.org/lexbit/relurpify/execution"
)

// TestGraphExecutionStatePropagation validates that node execution can update
// envelope state and that the resulting state is visible to the next node.
func TestGraphExecutionStatePropagation(t *testing.T) {
	// Create a graph with multiple nodes that pass state through the envelope
	g := graph.NewGraph()

	// First node sets a value
	node1 := &stateSetterNode{
		id:    "node1",
		key:   "propagated-key",
		value: "initial-value",
	}

	// Second node reads the value set by the first node
	node2 := &stateReaderNode{
		id:          "node2",
		key:         "propagated-key",
		expectValue: "initial-value",
	}

	// Third node modifies the value
	node3 := &stateModifierNode{
		id:          "node3",
		key:         "propagated-key",
		modifyValue: "modified-value",
	}

	// Fourth node validates the modified value
	node4 := &stateReaderNode{
		id:          "node4",
		key:         "propagated-key",
		expectValue: "modified-value",
	}

	terminal := graph.NewTerminalNode("done")

	// Add all nodes
	for _, node := range []graph.Node{node1, node2, node3, node4, terminal} {
		if err := g.AddNode(node); err != nil {
			t.Fatalf("failed to add node %s: %v", node.ID(), err)
		}
	}

	// Set up edges
	if err := g.SetStart(node1.ID()); err != nil {
		t.Fatalf("failed to set start node: %v", err)
	}
	if err := g.AddEdge(node1.ID(), node2.ID(), nil, false); err != nil {
		t.Fatalf("failed to add edge node1->node2: %v", err)
	}
	if err := g.AddEdge(node2.ID(), node3.ID(), nil, false); err != nil {
		t.Fatalf("failed to add edge node2->node3: %v", err)
	}
	if err := g.AddEdge(node3.ID(), node4.ID(), nil, false); err != nil {
		t.Fatalf("failed to add edge node3->node4: %v", err)
	}
	if err := g.AddEdge(node4.ID(), terminal.ID(), nil, false); err != nil {
		t.Fatalf("failed to add edge node4->done: %v", err)
	}

	// Create initial envelope
	env := contextdata.NewEnvelope("propagation-task", "propagation-session")

	// Execute the graph
	ctx := context.Background()
	_, err := g.Execute(ctx, env)
	if err != nil {
		t.Fatalf("graph execution failed: %v", err)
	}

	// Validate final state
	finalValue, ok := env.GetWorkingValue("propagated-key")
	if !ok {
		t.Fatal("expected propagated-key to be in envelope")
	}
	if finalValue != "modified-value" {
		t.Errorf("expected final value 'modified-value', got %v", finalValue)
	}
}

// TestNodeToEnvelopeStateTransfer validates that individual nodes can
// transfer state to and from the envelope correctly.
func TestNodeToEnvelopeStateTransfer(t *testing.T) {
	// Create a simple graph with one node that transfers state
	g := graph.NewGraph()

	transferNode := &stateTransferNode{
		id:        "transfer",
		inputKey:  "input-data",
		outputKey: "output-data",
		transform: func(val any) any {
			if s, ok := val.(string); ok {
				return s + "-processed"
			}
			return val
		},
	}

	terminal := graph.NewTerminalNode("done")

	if err := g.AddNode(transferNode); err != nil {
		t.Fatalf("failed to add transfer node: %v", err)
	}
	if err := g.AddNode(terminal); err != nil {
		t.Fatalf("failed to add terminal node: %v", err)
	}

	if err := g.SetStart(transferNode.ID()); err != nil {
		t.Fatalf("failed to set start node: %v", err)
	}
	if err := g.AddEdge(transferNode.ID(), terminal.ID(), nil, false); err != nil {
		t.Fatalf("failed to add edge transfer->done: %v", err)
	}

	// Create envelope with initial data
	env := contextdata.NewEnvelope("transfer-task", "transfer-session")
	env.SetWorkingValue("input-data", "test-input", contextdata.MemoryClassTask)

	// Execute the graph
	ctx := context.Background()
	_, err := g.Execute(ctx, env)
	if err != nil {
		t.Fatalf("graph execution failed: %v", err)
	}

	// Validate transfer happened
	outputValue, ok := env.GetWorkingValue("output-data")
	if !ok {
		t.Fatal("expected output-data to be in envelope")
	}
	if outputValue != "test-input-processed" {
		t.Errorf("expected 'test-input-processed', got %v", outputValue)
	}

	// Validate input data is still present (transfer is additive)
	inputValue, ok := env.GetWorkingValue("input-data")
	if !ok {
		t.Fatal("expected input-data to still be in envelope")
	}
	if inputValue != "test-input" {
		t.Errorf("expected input 'test-input', got %v", inputValue)
	}
}

// TestEnvelopeAdditiveMutation validates that envelope mutations are additive
// and don't erase existing state.
func TestEnvelopeAdditiveMutation(t *testing.T) {
	// Create envelope with initial state
	env := contextdata.NewEnvelope("mutation-task", "mutation-session")

	// Set initial values
	env.SetWorkingValue("key1", "value1", contextdata.MemoryClassTask)
	env.SetWorkingValue("key2", "value2", contextdata.MemoryClassTask)

	// Create a graph with two nodes that each add state
	g := graph.NewGraph()

	node1 := &stateSetterNode{
		id:    "node1",
		key:   "key3",
		value: "value3",
	}

	node2 := &stateReaderNode{
		id:          "node2",
		key:         "key1",
		expectValue: "value1",
	}

	terminal := graph.NewTerminalNode("done")

	if err := g.AddNode(node1); err != nil {
		t.Fatalf("failed to add node1: %v", err)
	}
	if err := g.AddNode(node2); err != nil {
		t.Fatalf("failed to add node2: %v", err)
	}
	if err := g.AddNode(terminal); err != nil {
		t.Fatalf("failed to add terminal: %v", err)
	}

	if err := g.SetStart(node1.ID()); err != nil {
		t.Fatalf("failed to set start: %v", err)
	}
	if err := g.AddEdge(node1.ID(), node2.ID(), nil, false); err != nil {
		t.Fatalf("failed to add edge node1->node2: %v", err)
	}
	if err := g.AddEdge(node2.ID(), terminal.ID(), nil, false); err != nil {
		t.Fatalf("failed to add edge node2->done: %v", err)
	}

	// Execute the graph
	ctx := context.Background()
	_, err := g.Execute(ctx, env)
	if err != nil {
		t.Fatalf("graph execution failed: %v", err)
	}

	// Validate all three keys are present
	val1, ok1 := env.GetWorkingValue("key1")
	if !ok1 || val1 != "value1" {
		t.Error("expected key1 to still be present with value1")
	}

	val2, ok2 := env.GetWorkingValue("key2")
	if !ok2 || val2 != "value2" {
		t.Error("expected key2 to still be present with value2")
	}

	val3, ok3 := env.GetWorkingValue("key3")
	if !ok3 || val3 != "value3" {
		t.Error("expected key3 to be present with value3")
	}

	// Validate the expected keys exist (don't check exact count as graph may add internal state)
	expectedKeys := map[string]bool{"key1": false, "key2": false, "key3": false}
	for key := range env.WorkingData {
		if _, ok := expectedKeys[key]; ok {
			expectedKeys[key] = true
		}
	}
	for key, found := range expectedKeys {
		if !found {
			t.Errorf("expected key %s to be present", key)
		}
	}
}

// stateSetterNode is a mock node that sets a value in the envelope.
type stateSetterNode struct {
	id    string
	key   string
	value any
}

func (n *stateSetterNode) ID() string           { return n.id }
func (n *stateSetterNode) Type() graph.NodeType { return graph.NodeTypeTool }

func (n *stateSetterNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	env.SetWorkingValue(n.key, n.value, contextdata.MemoryClassTask)
	return &execution.Result{
		NodeID: n.id,
		Data: execution.NewToolResultPayload(map[string]any{
			"next": "continue",
		}),
	}, nil
}

func (n *stateSetterNode) Category() string                      { return "test" }
func (n *stateSetterNode) Description() string                   { return "sets a value in the envelope" }
func (n *stateSetterNode) Parameters() []contracts.ToolParameter { return nil }
func (n *stateSetterNode) RequiresExecution() bool               { return true }

// stateReaderNode is a mock node that reads and validates a value in the envelope.
type stateReaderNode struct {
	id          string
	key         string
	expectValue any
}

func (n *stateReaderNode) ID() string           { return n.id }
func (n *stateReaderNode) Type() graph.NodeType { return graph.NodeTypeConditional }

func (n *stateReaderNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	value, ok := env.GetWorkingValue(n.key)
	if !ok {
		return nil, &contracts.PermissionDeniedError{
			Message: "key not found in envelope",
		}
	}
	if value != n.expectValue {
		return nil, &contracts.PermissionDeniedError{
			Message: "value mismatch in envelope",
		}
	}
	return &execution.Result{
		NodeID: n.id,
		Data: execution.NewToolResultPayload(map[string]any{
			"next": "continue",
		}),
	}, nil
}

func (n *stateReaderNode) Category() string                      { return "test" }
func (n *stateReaderNode) Description() string                   { return "reads and validates a value in the envelope" }
func (n *stateReaderNode) Parameters() []contracts.ToolParameter { return nil }
func (n *stateReaderNode) RequiresExecution() bool               { return true }

// stateModifierNode is a mock node that modifies a value in the envelope.
type stateModifierNode struct {
	id          string
	key         string
	modifyValue any
}

func (n *stateModifierNode) ID() string           { return n.id }
func (n *stateModifierNode) Type() graph.NodeType { return graph.NodeTypeTool }

func (n *stateModifierNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	env.SetWorkingValue(n.key, n.modifyValue, contextdata.MemoryClassTask)
	return &execution.Result{
		NodeID: n.id,
		Data: execution.NewToolResultPayload(map[string]any{
			"next": "continue",
		}),
	}, nil
}

func (n *stateModifierNode) Category() string                      { return "test" }
func (n *stateModifierNode) Description() string                   { return "modifies a value in the envelope" }
func (n *stateModifierNode) Parameters() []contracts.ToolParameter { return nil }
func (n *stateModifierNode) RequiresExecution() bool               { return true }

// stateTransferNode is a mock node that transfers state from input to output.
type stateTransferNode struct {
	id        string
	inputKey  string
	outputKey string
	transform func(any) any
}

func (n *stateTransferNode) ID() string           { return n.id }
func (n *stateTransferNode) Type() graph.NodeType { return graph.NodeTypeTool }

func (n *stateTransferNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	inputValue, ok := env.GetWorkingValue(n.inputKey)
	if !ok {
		return nil, &contracts.PermissionDeniedError{
			Message: "input key not found in envelope",
		}
	}
	outputValue := n.transform(inputValue)
	env.SetWorkingValue(n.outputKey, outputValue, contextdata.MemoryClassTask)
	return &execution.Result{
		NodeID: n.id,
		Data: execution.NewToolResultPayload(map[string]any{
			"next": "continue",
		}),
	}, nil
}

func (n *stateTransferNode) Category() string                      { return "test" }
func (n *stateTransferNode) Description() string                   { return "transfers state from input to output" }
func (n *stateTransferNode) Parameters() []contracts.ToolParameter { return nil }
func (n *stateTransferNode) RequiresExecution() bool               { return true }
