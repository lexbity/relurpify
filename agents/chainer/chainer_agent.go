package chainer

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
)

// ChainerAgent executes a deterministic chain of isolated LLM links.
//
// Execute() uses the envelope-native chainRunner (runner.go) for direct
// working-memory operations. Each link reads its declared InputKeys from
// the envelope, runs the LLM call, parses the response, and writes the result
// to its OutputKey.
type ChainerAgent struct {
	Model           core.LanguageModel
	Tools           *capability.Registry
	Config          *core.Config
	Chain           *Chain
	ChainBuilder    func(*core.Task) (*Chain, error)
	StreamTrigger   *contextstream.Trigger
	StreamMode      contextstream.Mode
	StreamQuery     string
	StreamMaxTokens int
	initialised     bool
}

func (a *ChainerAgent) Initialize(cfg *core.Config) error {
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

func (a *ChainerAgent) BuildGraph(task *core.Task) (*agentgraph.Graph, error) {
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

func envGetString(env *contextdata.Envelope, key string) string {
	val, _ := env.GetWorkingValue(key)
	if s, ok := val.(string); ok {
		return s
	}
	return ""
}

func (a *ChainerAgent) Execute(ctx context.Context, task *core.Task, env *contextdata.Envelope) (*core.Result, error) {
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
func (a *ChainerAgent) executeChain(ctx context.Context, task *core.Task, env *contextdata.Envelope, chain *Chain) (*core.Result, error) {
	if err := (&chainRunner{Model: a.Model}).Run(ctx, task, chain, env); err != nil {
		return nil, err
	}
	env.SetWorkingValue("chainer.links_executed", len(chain.Links), contextdata.MemoryClassTask)
	data := map[string]any{"links_executed": len(chain.Links)}
	for _, link := range chain.Links {
		if value, ok := env.GetWorkingValue(link.OutputKey); ok {
			data[link.OutputKey] = value
		}
	}
	return &core.Result{Success: true, Data: data}, nil
}

func (a *ChainerAgent) resolveChain(task *core.Task) (*Chain, error) {
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

func (a *ChainerAgent) streamQuery(task *core.Task) string {
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

func (a *ChainerAgent) streamTriggerNode(task *core.Task) agentgraph.Node {
	if a.StreamTrigger == nil {
		return nil
	}
	query := a.streamQuery(task)
	if strings.TrimSpace(query) == "" {
		return nil
	}
	node := agentgraph.NewContextStreamNode("chainer_stream", a.StreamTrigger, retrieval.RetrievalQuery{Text: query}, a.streamMaxTokens())
	node.Mode = a.streamMode()
	node.BudgetShortfallPolicy = "emit_partial"
	node.Metadata = map[string]any{"agent": "chainer"}
	return node
}

func (a *ChainerAgent) executeStreamingTrigger(ctx context.Context, task *core.Task, env *contextdata.Envelope) error {
	if a.StreamTrigger == nil {
		return nil
	}
	query := a.streamQuery(task)
	if strings.TrimSpace(query) == "" {
		return nil
	}
	req := contextstream.Request{
		ID:        "chainer_stream",
		MaxTokens: a.streamMaxTokens(),
		Mode:      a.streamMode(),
		Query:     retrieval.RetrievalQuery{Text: query},
	}
	result, err := a.StreamTrigger.RequestBlocking(ctx, req)
	if err != nil {
		return err
	}
	if env != nil {
		env.SetWorkingValue("chainer.stream.result", result, contextdata.MemoryClassTask)
		env.SetWorkingValue("chainer.stream.query", query, contextdata.MemoryClassTask)
	}
	return nil
}

type chainerLinkNode struct {
	id   string
	name string
}

func (n *chainerLinkNode) ID() string                { return n.id }
func (n *chainerLinkNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeSystem }
func (n *chainerLinkNode) Execute(_ context.Context, env *contextdata.Envelope) (*core.Result, error) {
	if env != nil {
		env.SetWorkingValue("chainer.inspect_link", n.name, contextdata.MemoryClassTask)
	}
	return &core.Result{NodeID: n.id, Success: true}, nil
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
