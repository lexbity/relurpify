package pattern

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
)

// ReflectionAgent reviews outputs and triggers revisions when needed.
type ReflectionAgent struct {
	Reviewer      core.LanguageModel
	Delegate      graph.Agent
	Config        *core.Config
	maxIterations int
}

// Initialize configures the reviewer.
func (a *ReflectionAgent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	if cfg.MaxIterations <= 0 {
		a.maxIterations = 3
	} else {
		a.maxIterations = cfg.MaxIterations
	}
	return nil
}

// Execute runs the review workflow.
func (a *ReflectionAgent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	graph, err := a.BuildGraph(task)
	if err != nil {
		return nil, err
	}
	if cfg := a.Config; cfg != nil && cfg.Telemetry != nil {
		graph.SetTelemetry(cfg.Telemetry)
	}
	return graph.Execute(ctx, state)
}

// Capabilities returns capabilities.
func (a *ReflectionAgent) Capabilities() []core.Capability {
	return []core.Capability{core.CapabilityReview}
}

// BuildGraph builds the review workflow.
func (a *ReflectionAgent) BuildGraph(task *core.Task) (*graph.Graph, error) {
	if a.Delegate == nil {
		return nil, fmt.Errorf("reflection agent missing delegate")
	}
	if a.Reviewer == nil {
		return nil, fmt.Errorf("reflection agent missing reviewer model")
	}
	g := graph.NewGraph()
	run := &reflectionDelegateNode{id: "reflection_execute", agent: a, task: task}
	review := &reflectionReviewNode{id: "reflection_review", agent: a, task: task}
	decision := &reflectionDecisionNode{id: "reflection_decide", agent: a}
	done := graph.NewTerminalNode("reflection_done")
	for _, node := range []graph.Node{run, review, decision, done} {
		if err := g.AddNode(node); err != nil {
			return nil, err
		}
	}
	if err := g.SetStart(run.ID()); err != nil {
		return nil, err
	}
	if err := g.AddEdge(run.ID(), review.ID(), nil, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(review.ID(), decision.ID(), nil, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(decision.ID(), run.ID(), func(res *core.Result, ctx *core.Context) bool {
		revise, _ := ctx.Get("reflection.revise")
		return revise == true
	}, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(decision.ID(), done.ID(), func(res *core.Result, ctx *core.Context) bool {
		revise, _ := ctx.Get("reflection.revise")
		return revise != true
	}, false); err != nil {
		return nil, err
	}
	return g, nil
}

type reflectionDelegateNode struct {
	id    string
	agent *ReflectionAgent
	task  *core.Task
}

// ID returns the graph identifier for the delegate execution step.
func (n *reflectionDelegateNode) ID() string { return n.id }

// Type indicates this node executes system steps rather than tools.
func (n *reflectionDelegateNode) Type() graph.NodeType {
	return graph.NodeTypeSystem
}

// Execute runs the delegate agent while isolating state mutations until the
// child run succeeds.
func (n *reflectionDelegateNode) Execute(ctx context.Context, state *core.Context) (*core.Result, error) {
	state.SetExecutionPhase("executing")
	child := state.Clone()
	result, err := n.agent.Delegate.Execute(ctx, n.task, child)
	if err != nil {
		return nil, err
	}
	state.Merge(child)
	state.SetHandleScoped("reflection.last_result", result, taskScope(n.task, state))
	return result, nil
}

type reflectionReviewNode struct {
	id    string
	agent *ReflectionAgent
	task  *core.Task
}

// ID returns the review node identifier.
func (n *reflectionReviewNode) ID() string { return n.id }

// Type marks the node as an observation step since it inspects output.
func (n *reflectionReviewNode) Type() graph.NodeType {
	return graph.NodeTypeObservation
}

// Execute asks the reviewer model to evaluate the last result and captures the
// structured feedback in the shared state.
func (n *reflectionReviewNode) Execute(ctx context.Context, state *core.Context) (*core.Result, error) {
	lastResult := resolveResultHandle(state, "reflection.last_result")
	prompt := fmt.Sprintf(`Review the following result for task "%s".
Consider correctness, completeness, quality, security, performance.
Respond JSON {"issues":[{"severity":"high|medium|low","description":"...","suggestion":"..."}],"approve":bool}
Result: %+v`, n.task.Instruction, lastResult)
	resp, err := n.agent.Reviewer.Generate(ctx, prompt, &core.LLMOptions{
		Model:       n.agent.Config.Model,
		Temperature: 0.2,
		MaxTokens:   600,
	})
	if err != nil {
		return nil, err
	}
	review, err := parseReview(resp.Text)
	if err != nil {
		return nil, err
	}
	state.Set("reflection.review", review)
	return &core.Result{NodeID: n.id, Success: true, Data: map[string]interface{}{"review": review}}, nil
}

type reflectionDecisionNode struct {
	id    string
	agent *ReflectionAgent
}

// ID returns the decision node identifier.
func (n *reflectionDecisionNode) ID() string { return n.id }

// Type declares the node as a conditional branch in the graph.
func (n *reflectionDecisionNode) Type() graph.NodeType {
	return graph.NodeTypeConditional
}

// Execute inspects review feedback and decides if another delegate iteration
// should run.
func (n *reflectionDecisionNode) Execute(ctx context.Context, state *core.Context) (*core.Result, error) {
	reviewVal, _ := state.Get("reflection.review")
	review, _ := reviewVal.(reviewPayload)
	iterVal, _ := state.Get("reflection.iteration")
	iter, _ := iterVal.(int)
	iter++
	state.Set("reflection.iteration", iter)
	revise := !review.Approve && iter < n.agent.maxIterations
	state.Set("reflection.revise", revise)
	return &core.Result{NodeID: n.id, Success: true, Data: map[string]interface{}{"revise": revise}}, nil
}

type reviewPayload struct {
	Issues []struct {
		Severity    string `json:"severity"`
		Description string `json:"description"`
		Suggestion  string `json:"suggestion"`
	} `json:"issues"`
	Approve bool `json:"approve"`
}

// parseReview decodes the reviewer JSON into a strongly typed payload.
func parseReview(raw string) (reviewPayload, error) {
	var payload reviewPayload
	if err := json.Unmarshal([]byte(ExtractJSON(raw)), &payload); err != nil {
		return payload, err
	}
	return payload, nil
}

func resolveResultHandle(state *core.Context, key string) *core.Result {
	if state == nil {
		return nil
	}
	if value, ok := state.GetHandle(key); ok {
		if res, ok := value.(*core.Result); ok {
			return res
		}
	}
	if value, ok := state.Get(key); ok {
		if res, ok := value.(*core.Result); ok {
			return res
		}
	}
	return nil
}

func taskScope(task *core.Task, state *core.Context) string {
	if task != nil && task.ID != "" {
		return task.ID
	}
	if state != nil {
		return state.GetString("task.id")
	}
	return ""
}
