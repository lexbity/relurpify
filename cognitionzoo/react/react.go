package react

import (
	"context"

	capability "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	"codeburg.org/lexbit/relurpify/context/knowledge/memory"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	"codeburg.org/lexbit/relurpify/context/knowledge/search"
	execution "codeburg.org/lexbit/relurpify/execution"
	graph "codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/execution/prompt"
	"codeburg.org/lexbit/relurpify/model"
)

// ReActAgent implements the ReAct (Reason + Act) paradigm. It iteratively
// thinks about the task, acts using tools, and observes results until completion.
type ReActAgent struct {
	Model           model.LanguageModel
	Tools           *capability.CapabilityRegistry
	Memory          *memory.WorkingMemoryStore
	Config          *execution.Config
	IndexManager    *ast.IndexManager
	SearchEngine    *search.SearchEngine
	StreamMode      contextstream.Mode
	StreamQuery     string
	StreamMaxTokens int
	StreamTrigger   *contextstream.Trigger
	OutputIngester  *knowledge.OutputIngester
	IngestOutputs   bool
	PromptRegistry  prompt.Registry

	// Internal state
	executionCatalog *capability.ExecutionCapabilityCatalogSnapshot
	initialized      bool
	Mode             string
	maxIterations    int
}

// Initialize configures the agent.
func (a *ReActAgent) Initialize(cfg *execution.Config) error {
	a.Config = cfg
	if a.Tools == nil {
		a.Tools = capability.NewRegistry()
	}
	if cfg != nil && cfg.MaxIterations > 0 {
		a.maxIterations = cfg.MaxIterations
	} else if a.maxIterations <= 0 {
		a.maxIterations = 8
	}
	a.initialized = true
	return nil
}

// Capabilities returns the capability identifiers this agent provides.
func (a *ReActAgent) Capabilities() []string {
	return []string{"react"}
}

// Execute runs the react workflow.
func (a *ReActAgent) Execute(ctx context.Context, task *execution.Task, env *contextdata.Envelope) (*execution.Result, error) {
	if !a.initialized {
		if err := a.Initialize(a.Config); err != nil {
			return nil, err
		}
	}
	if a.StreamTrigger != nil {
		ctx = contextstream.WithTrigger(ctx, a.StreamTrigger)
	}
	graph, err := a.BuildGraph(ctx, task)
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
func (a *ReActAgent) debugf(format string, args ...any) {
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
func (a *ReActAgent) streamQuery(task *execution.Task) string {
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
func (a *ReActAgent) streamTriggerNode(task *execution.Task) graph.Node {
	query := a.streamQuery(task)
	node := graph.NewContextStreamNode("react_stream", retrieval.RetrievalQuery{Text: query}, a.streamMaxTokens())
	node.Mode = a.streamMode()
	return node
}
