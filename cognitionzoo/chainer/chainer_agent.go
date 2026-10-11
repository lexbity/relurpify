package chainer

import (
	"context"
	"errors"
	"fmt"
	"strings"

	capability "codeburg.org/lexbit/relurpify/capability/registry"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/model"
)

// ChainerAgent executes a deterministic chain of isolated LLM links.
//
// Execute() uses the envelope-native chainRunner (runner.go) for direct
// working-memory operations. Each link reads its declared InputKeys from
// the envelope, runs the LLM call, parses the response, and writes the result
// to its OutputKey.
type ChainerAgent struct {
	Model           model.LanguageModel
	Tools           *capability.CapabilityRegistry
	Config          *execution.Config
	Chain           *Chain
	ChainBuilder    func(*execution.Task) (*Chain, error)
	PromptRegistry  any // prompt.Registry; injected into the chain runner for PromptID links
	StreamMode      contextstream.Mode
	StreamQuery     string
	StreamMaxTokens int
	initialised     bool
}

func (a *ChainerAgent) Initialize(cfg *execution.Config) error {
	a.Config = cfg
	if a.Tools == nil {
		a.Tools = capability.NewRegistry()
	}
	a.initialised = true
	return nil
}

func (a *ChainerAgent) Capabilities() []string {
	return []string{"plan", "execute", "explain"}
}

func (a *ChainerAgent) BuildGraph(ctx context.Context, task *execution.Task) (*agentgraph.Graph, error) {
	chain, err := a.resolveChain(task)
	if err != nil {
		return nil, err
	}
	g := agentgraph.NewGraph()
	nodes := make([]agentgraph.Node, 0, len(chain.Links)+2)
	stream := a.streamTriggerNode(task)
	if stream != nil {
		nodes = append(nodes, stream)
	}
	for i, link := range chain.Links {
		nodes = append(nodes, &chainerLinkNode{id: fmt.Sprintf("chainer_link_%02d_%s", i, sanitizeLinkName(link.Name)), name: link.Name})
	}
	nodes = append(nodes, agentgraph.NewTerminalNode("chainer_done"))
	for _, node := range nodes {
		if err := g.AddNode(node); err != nil {
			return nil, err
		}
	}
	if err := g.SetStart(nodes[0].ID()); err != nil {
		return nil, err
	}
	for i := 0; i < len(nodes)-1; i++ {
		if err := g.AddEdge(nodes[i].ID(), nodes[i+1].ID(), nil, false); err != nil {
			return nil, err
		}
	}
	return g, nil
}

func (a *ChainerAgent) Execute(ctx context.Context, task *execution.Task, env *contextdata.Envelope) (*execution.Result, error) {
	if !a.initialised {
		if err := a.Initialize(a.Config); err != nil {
			return nil, err
		}
	}
	if env == nil {
		env = contextdata.NewEnvelope("chainer", "session")
	}
	if err := a.executeStreamingTrigger(ctx, task, env); err != nil {
		return nil, err
	}

	chain, err := a.resolveChain(task)
	if err != nil {
		return nil, err
	}
	if err := chain.Validate(); err != nil {
		return nil, err
	}

	return a.executeChain(ctx, task, env, chain)
}

// executeChain runs the chain using the envelope-native chainRunner.
func (a *ChainerAgent) executeChain(ctx context.Context, task *execution.Task, env *contextdata.Envelope, chain *Chain) (*execution.Result, error) {
	if err := (&chainRunner{Model: a.Model, Registry: a.PromptRegistry}).Run(ctx, task, chain, env); err != nil {
		return nil, err
	}
	env.SetWorkingValueWithClass("chainer.links_executed", len(chain.Links), contextdata.MemoryClassTask)
	data := map[string]any{"links_executed": len(chain.Links)}
	for _, link := range chain.Links {
		if value, ok := contextdata.GetTyped[any](env, link.OutputKey); ok {
			data[link.OutputKey] = value
		}
	}
	return &execution.Result{Success: true, Data: execution.NewToolResultPayload(data)}, nil
}

func (a *ChainerAgent) resolveChain(task *execution.Task) (*Chain, error) {
	switch {
	case a.ChainBuilder != nil:
		return a.ChainBuilder(task)
	case a.Chain != nil:
		return a.Chain, nil
	default:
		return nil, fmt.Errorf("chainer: chain not configured")
	}
}

func (a *ChainerAgent) streamMode() contextstream.Mode {
	if a.StreamMode != "" {
		return a.StreamMode
	}
	return contextstream.ModeBlocking
}

func (a *ChainerAgent) streamQuery(task *execution.Task) string {
	if strings.TrimSpace(a.StreamQuery) != "" {
		return strings.TrimSpace(a.StreamQuery)
	}
	if task != nil {
		return task.Instruction
	}
	return ""
}

func (a *ChainerAgent) streamMaxTokens() int {
	if a.StreamMaxTokens > 0 {
		return a.StreamMaxTokens
	}
	return 256
}

func (a *ChainerAgent) streamTriggerNode(task *execution.Task) agentgraph.Node {
	query := a.streamQuery(task)
	if strings.TrimSpace(query) == "" {
		return nil
	}
	node := agentgraph.NewContextStreamNode("chainer_stream", retrieval.RetrievalQuery{Text: query}, a.streamMaxTokens())
	node.Mode = a.streamMode()
	node.BudgetShortfallPolicy = "emit_partial"
	node.Metadata = map[string]any{"agent": "chainer"}
	return node
}

func (a *ChainerAgent) executeStreamingTrigger(ctx context.Context, task *execution.Task, env *contextdata.Envelope) error {
	query := a.streamQuery(task)
	if strings.TrimSpace(query) == "" {
		return nil
	}
	trigger := contextstream.TriggerFromContext(ctx)
	if trigger == nil {
		return nil
	}
	taskID := ""
	if task != nil {
		taskID = task.ID
	}
	req := contextstream.Request{
		ID:        "chainer." + taskID,
		MaxTokens: a.streamMaxTokens(),
		Mode:      a.streamMode(),
		Query:     retrieval.RetrievalQuery{Text: query},
	}
	// The chainer compile is part of the run's memory order: the request
	// carries the envelope's compile identity (epoch + run scope) so the
	// compiler's cache keys it correctly and can never serve it across runs.
	var epoch uint64
	if env != nil {
		meta := env.AssemblyMetadataSnapshot()
		epoch = meta.EpochID
		req.EpochID = meta.EpochID
		req.RunID = meta.RunID
		req.EventLogSeq = meta.EventLogSeq
	}
	result, err := trigger.RequestBlocking(ctx, req)
	if err != nil {
		return err
	}
	if env != nil {
		// Deliver through the substrate (the link prompts render the
		// envelope slice via the shared renderer seam); working state holds
		// the bounded summary only — never the compiled bodies.
		if err := contextstream.ApplyResult(env, result, epoch); err != nil {
			var dropped *contextstream.ErrStaleApplyDropped
			if !errors.As(err, &dropped) {
				return fmt.Errorf("chainer stream apply: %w", err)
			}
			// Designed drop: a newer compilation already owns the slice.
			contextstream.EmitStaleApplyDropped(ctx, dropped.CurrentEpoch, dropped.GotEpoch, dropped.RequestID)
		}
		env.SetWorkingValueWithClass("chainer.stream.result", contextstream.SummaryOf(result), contextdata.MemoryClassTask)
		env.SetWorkingValueWithClass("chainer.stream.query", query, contextdata.MemoryClassTask)
	}
	return nil
}

type chainerLinkNode struct {
	id   string
	name string
}

func (n *chainerLinkNode) ID() string                { return n.id }
func (n *chainerLinkNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeSystem }
func (n *chainerLinkNode) Execute(_ context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	if env != nil {
		env.SetWorkingValueWithClass("chainer.inspect_link", n.name, contextdata.MemoryClassTask)
	}
	return &execution.Result{NodeID: n.id, Success: true}, nil
}

func sanitizeLinkName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "-", "_")
	if name == "" {
		return "link"
	}
	return name
}
