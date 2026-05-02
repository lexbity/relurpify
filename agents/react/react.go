package react

import (
	"context"

	graph "codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/framework/memory"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	"codeburg.org/lexbit/relurpify/framework/search"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// ReActAgent implements the ReAct (Reason + Act) paradigm. It iteratively
// thinks about the task, acts using tools, and observes results until completion.
type ReActAgent struct {
	Model           contracts.LanguageModel
	Tools           *capability.Registry
	Memory          *memory.WorkingMemoryStore
	Config          *core.Config
	IndexManager    *ast.IndexManager
	SearchEngine    *search.SearchEngine
	StreamMode      contextstream.Mode
	StreamQuery     string
	StreamMaxTokens int
	OutputIngester  *knowledge.OutputIngester
	IngestOutputs   bool

	// Internal state
	executionCatalog *capability.ExecutionCapabilityCatalogSnapshot
	initialized      bool
	Mode             string
	maxIterations    int
}

// Initialize configures the agent.
func (a *ReActAgent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	if a.Tools == nil {
		a.Tools = capability.NewRegistry()
	}
	a.initialized = true
	return nil
}

// Capabilities returns the capability identifiers this agent provides.
func (a *ReActAgent) Capabilities() []string {
	return []string{"react"}
}

// Execute runs the react workflow.
func (a *ReActAgent) Execute(ctx context.Context, task *core.Task, env *contextdata.Envelope) (*core.Result, error) {
	if !a.initialized {
		if err := a.Initialize(a.Config); err != nil {
			return nil, err
		}
	}
	graph, err := a.BuildGraph(task)
	if err != nil {
		return nil, err
	}
	if cfg := a.Config; cfg != nil && cfg.Telemetry != nil {
		graph.SetTelemetry(cfg.Telemetry)
	}
	result, err := graph.Execute(ctx, env)
	return result, err
}

// debugf is a helper for debug logging when telemetry is available.
func (a *ReActAgent) debugf(format string, args ...interface{}) {
	_ = format
	_ = args
}

// streamMode returns the streaming mode, defaulting to blocking.
func (a *ReActAgent) streamMode() contextstream.Mode {
	if a.StreamMode != "" {
		return a.StreamMode
	}
	return contextstream.ModeBlocking
}

// streamQuery returns the query for streaming, defaulting to task instruction.
func (a *ReActAgent) streamQuery(task *core.Task) string {
	if a.StreamQuery != "" {
		return a.StreamQuery
	}
	if task != nil {
		return task.Instruction
	}
	return ""
}

// streamMaxTokens returns the max tokens for streaming, defaulting to 256.
func (a *ReActAgent) streamMaxTokens() int {
	if a.StreamMaxTokens > 0 {
		return a.StreamMaxTokens
	}
	return 256
}

func (a *ReActAgent) outputIngestionEnabled() bool {
	return a != nil && a.OutputIngester != nil && a.IngestOutputs
}

// streamTriggerNode creates a streaming trigger node for the react agent.
func (a *ReActAgent) streamTriggerNode(task *core.Task) graph.Node {
	query := a.streamQuery(task)
	node := graph.NewContextStreamNode("react_stream", retrieval.RetrievalQuery{Text: query}, a.streamMaxTokens())
	node.Mode = a.streamMode()
	return node
}
