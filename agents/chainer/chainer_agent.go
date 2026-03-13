package chainer

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/memory"
)

// ChainerAgent executes a deterministic chain of isolated LLM links.
type ChainerAgent struct {
	Model        core.LanguageModel
	Tools        *capability.Registry
	Memory       memory.MemoryStore
	Config       *core.Config
	Chain        *Chain
	ChainBuilder func(*core.Task) (*Chain, error)
	initialised  bool
}

func (a *ChainerAgent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	if a.Tools == nil {
		a.Tools = capability.NewRegistry()
	}
	a.initialised = true
	return nil
}

func (a *ChainerAgent) Capabilities() []core.Capability {
	return []core.Capability{
		core.CapabilityPlan,
		core.CapabilityExecute,
		core.CapabilityExplain,
	}
}

func (a *ChainerAgent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	chain, err := a.resolveChain(task)
	if err != nil {
		return nil, err
	}
	g := graph.NewGraph()
	nodes := make([]graph.Node, 0, len(chain.Links)+1)
	for i, link := range chain.Links {
		nodes = append(nodes, &chainerLinkNode{id: fmt.Sprintf("chainer_link_%02d_%s", i, sanitizeLinkName(link.Name)), name: link.Name})
	}
	nodes = append(nodes, graph.NewTerminalNode("chainer_done"))
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

func (a *ChainerAgent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	if !a.initialised {
		if err := a.Initialize(a.Config); err != nil {
			return nil, err
		}
	}
	if state == nil {
		state = core.NewContext()
	}
	chain, err := a.resolveChain(task)
	if err != nil {
		return nil, err
	}
	if err := chain.Validate(); err != nil {
		return nil, err
	}
	if err := (&chainRunner{Model: a.Model}).Run(ctx, task, chain, state); err != nil {
		return nil, err
	}
	state.Set("chainer.links_executed", len(chain.Links))
	data := map[string]any{"links_executed": len(chain.Links)}
	for _, link := range chain.Links {
		if value, ok := state.Get(link.OutputKey); ok {
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

type chainerLinkNode struct {
	id   string
	name string
}

func (n *chainerLinkNode) ID() string                { return n.id }
func (n *chainerLinkNode) Type() graph.NodeType      { return graph.NodeTypeSystem }
func (n *chainerLinkNode) Execute(_ context.Context, state *core.Context) (*core.Result, error) {
	if state != nil {
		state.Set("chainer.inspect_link", n.name)
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
