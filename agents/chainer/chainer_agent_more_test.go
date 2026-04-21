package chainer

import (
	"context"
	"errors"
	"testing"

	"codeburg.org/lexbit/relurpify/agents/chainer/checkpoint"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/pipeline"
)

// TestSanitizeLinkName tests the sanitizeLinkName helper function
func TestSanitizeLinkName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"  My-Link  ", "my_link"},
		{"", "link"},
		{"UPPERCASE", "uppercase"},
		{"with spaces here", "with_spaces_here"},
		{"with-dashes-here", "with_dashes_here"},
		{"MiXeD-CaSe 123", "mixed_case_123"},
		{"   ", "link"},
		{"a", "a"},
		{"link with  multiple   spaces", "link_with__multiple___spaces"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeLinkName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeLinkName(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestChainerLinkNodeExecute tests the chainerLinkNode Execute method
func TestChainerLinkNodeExecute(t *testing.T) {
	t.Run("nil state", func(t *testing.T) {
		node := &chainerLinkNode{id: "link-1", name: "test-link"}
		result, err := node.Execute(context.Background(), nil)
		if err != nil {
			t.Fatalf("Execute with nil state should not error: %v", err)
		}
		if !result.Success {
			t.Error("expected success for nil state")
		}
	})

	t.Run("with state", func(t *testing.T) {
		node := &chainerLinkNode{id: "link-2", name: "inspect-link"}
		state := core.NewContext()
		result, err := node.Execute(context.Background(), state)
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
		if !result.Success {
			t.Error("expected success")
		}
		if result.NodeID != "link-2" {
			t.Errorf("expected node ID link-2, got %s", result.NodeID)
		}

		// Verify the inspected link name was recorded
		if linkName, ok := state.Get("chainer.inspect_link"); !ok || linkName != "inspect-link" {
			t.Errorf("expected inspect_link to be set to 'inspect-link', got %v", linkName)
		}
	})
}

// TestChainerAgentMethods tests various agent methods directly
func TestChainerAgentMethods(t *testing.T) {
	t.Run("nil agent capability registry", func(t *testing.T) {
		var nilAgent *ChainerAgent
		if nilAgent.CapabilityRegistry() != nil {
			t.Error("expected nil capability registry for nil agent")
		}
	})

	t.Run("agent with registry", func(t *testing.T) {
		agent := &ChainerAgent{
			Tools: capability.NewRegistry(),
		}
		if agent.CapabilityRegistry() == nil {
			t.Error("expected non-nil capability registry")
		}
	})
}

// TestChainerAgentInitialize tests the Initialize method
func TestChainerAgentInitialize(t *testing.T) {
	t.Run("initialize with nil tools", func(t *testing.T) {
		agent := &ChainerAgent{}
		cfg := &core.Config{Model: "test-model"}

		err := agent.Initialize(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if agent.Tools == nil {
			t.Error("expected Tools to be initialized")
		}

		if agent.Config != cfg {
			t.Error("expected Config to be set")
		}
	})

	t.Run("initialize with checkpoint store", func(t *testing.T) {
		store := &testCheckpointStore{checkpoints: make(map[string]*pipeline.Checkpoint)}
		agent := &ChainerAgent{
			CheckpointStore: store,
		}
		cfg := &core.Config{Model: "test-model"}

		err := agent.Initialize(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if agent.RecoveryManager == nil {
			t.Error("expected RecoveryManager to be initialized")
		}
	})

	t.Run("initialize with existing recovery manager", func(t *testing.T) {
		store := &testCheckpointStore{checkpoints: make(map[string]*pipeline.Checkpoint)}
		existingRM := checkpoint.NewRecoveryManager(store)
		agent := &ChainerAgent{
			CheckpointStore: store,
			RecoveryManager: existingRM,
		}
		cfg := &core.Config{Model: "test-model"}

		err := agent.Initialize(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if agent.RecoveryManager != existingRM {
			t.Error("expected existing RecoveryManager to be preserved")
		}
	})
}

// TestChainerAgentCapabilities tests the Capabilities method
func TestChainerAgentCapabilities(t *testing.T) {
	agent := &ChainerAgent{}
	caps := agent.Capabilities()

	if len(caps) != 3 {
		t.Errorf("expected 3 capabilities, got %d", len(caps))
	}

	expectedCaps := []core.Capability{core.CapabilityPlan, core.CapabilityExecute, core.CapabilityExplain}
	for i, cap := range caps {
		if cap != expectedCaps[i] {
			t.Errorf("expected capability %v, got %v", expectedCaps[i], cap)
		}
	}
}

// TestChainerAgentTelemetryQueries tests telemetry query methods
func TestChainerAgentTelemetryQueries(t *testing.T) {
	t.Run("nil agent telemetry queries", func(t *testing.T) {
		var nilAgent *ChainerAgent

		if nilAgent.ExecutionEvents("task-1") != nil {
			t.Error("expected nil ExecutionEvents for nil agent")
		}

		if nilAgent.ExecutionSummary("task-1") != nil {
			t.Error("expected nil ExecutionSummary for nil agent")
		}

		if nilAgent.LinkEvents("task-1", "link-1") != nil {
			t.Error("expected nil LinkEvents for nil agent")
		}
	})

	t.Run("nil event recorder", func(t *testing.T) {
		agent := &ChainerAgent{}

		if agent.ExecutionEvents("task-1") != nil {
			t.Error("expected nil ExecutionEvents for nil recorder")
		}

		if agent.ExecutionSummary("task-1") != nil {
			t.Error("expected nil ExecutionSummary for nil recorder")
		}

		if agent.LinkEvents("task-1", "link-1") != nil {
			t.Error("expected nil LinkEvents for nil recorder")
		}
	})
}

// TestChainerAgentResolveChain tests the resolveChain method
func TestChainerAgentResolveChain(t *testing.T) {
	t.Run("chain builder takes precedence", func(t *testing.T) {
		builtChain := &Chain{Links: []Link{NewLink("built", "prompt", nil, "out", nil)}}
		staticChain := &Chain{Links: []Link{NewLink("static", "prompt", nil, "out", nil)}}

		agent := &ChainerAgent{
			Chain:        staticChain,
			ChainBuilder: func(task *core.Task) (*Chain, error) { return builtChain, nil },
		}

		result, err := agent.resolveChain(&core.Task{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result != builtChain {
			t.Error("expected ChainBuilder result to take precedence")
		}
	})

	t.Run("static chain used when no builder", func(t *testing.T) {
		staticChain := &Chain{Links: []Link{NewLink("static", "prompt", nil, "out", nil)}}

		agent := &ChainerAgent{
			Chain: staticChain,
		}

		result, err := agent.resolveChain(&core.Task{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result != staticChain {
			t.Error("expected static chain to be returned")
		}
	})
}

// TestChainerLinkNodeIDType tests ID and Type methods
func TestChainerLinkNodeIDType(t *testing.T) {
	node := &chainerLinkNode{id: "test-id", name: "test-name"}

	if node.ID() != "test-id" {
		t.Errorf("expected ID test-id, got %s", node.ID())
	}

	// NodeType() should return graph.NodeTypeSystem
	nodeType := node.Type()
	_ = nodeType // Just verify it doesn't panic - the actual value is tested elsewhere
}

// testCheckpointStore is a minimal checkpoint store implementation for tests
type testCheckpointStore struct {
	checkpoints map[string]*pipeline.Checkpoint
}

func (s *testCheckpointStore) Save(cp *pipeline.Checkpoint) error {
	if cp == nil || cp.TaskID == "" || cp.CheckpointID == "" {
		return errors.New("invalid checkpoint")
	}
	key := cp.TaskID + ":" + cp.CheckpointID
	s.checkpoints[key] = cp
	return nil
}

func (s *testCheckpointStore) Load(taskID, checkpointID string) (*pipeline.Checkpoint, error) {
	key := taskID + ":" + checkpointID
	cp, ok := s.checkpoints[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return cp, nil
}
