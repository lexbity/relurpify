package thoughtrecipe

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"

	blackboardagent "codeburg.org/lexbit/relurpify/cognitionzoo/blackboard"
	chaineragent "codeburg.org/lexbit/relurpify/cognitionzoo/chainer"
	htnagent "codeburg.org/lexbit/relurpify/cognitionzoo/htn"
	htnruntime "codeburg.org/lexbit/relurpify/cognitionzoo/htn/runtime"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	pipelineagent "codeburg.org/lexbit/relurpify/cognitionzoo/pipeline"
	planneragent "codeburg.org/lexbit/relurpify/cognitionzoo/planner"
	reactagent "codeburg.org/lexbit/relurpify/cognitionzoo/react"
	reflectionagent "codeburg.org/lexbit/relurpify/cognitionzoo/reflection"
	rewooagent "codeburg.org/lexbit/relurpify/cognitionzoo/rewoo"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

const (
	executionCapabilityIDKey = "execution_capability_id"
)

// stepCore is the shared plumbing embedded by all per-kind node types.
type stepCore struct {
	id       string
	deps     *paradigm.Deps
	step     ExecutionStep
	resolver interaction.Resolver
}

func (c *stepCore) ID() string                    { return c.id }
func (c *stepCore) NodeType() agentgraph.NodeType { return agentgraph.NodeTypeTool }

// setResolver wires the InteractionResolver consumed by the on_error: ask
// policy (D12). It is installed by the graph builder from the Euclo composition
// root; the resolver is required at Euclo construction, so a step carrying an
// ask policy always has one.
func (c *stepCore) setResolver(resolver interaction.Resolver) {
	if c != nil {
		c.resolver = resolver
	}
}

func (c *stepCore) buildTask(ctx context.Context, env *contextdata.Envelope) (*execution.Task, error) {
	data := thoughtrecipeTemplateData(env, c.step)
	c.writeStepMetadata(env)

	var instruction string
	if c.step.PromptID != "" {
		if c.deps.PromptRegistry == nil {
			return nil, fmt.Errorf("thoughtrecipe step %q: prompt_id requires a registry", c.step.ID)
		}
		var err error
		instruction, err = c.resolveFromRegistry(ctx, env)
		if err != nil {
			return nil, err
		}
	}

	if instruction == "" {
		if strings.TrimSpace(c.step.Question) != "" {
			instruction = c.renderTemplate(c.step.Question, data)
		}
	}
	if instruction == "" {
		if strings.TrimSpace(c.step.Goal) != "" {
			instruction = c.renderTemplate(c.step.Goal, data)
		}
	}
	if instruction == "" {
		instruction = c.renderTemplate(c.step.Prompt, data)
		if instruction == "" {
			instruction = c.step.Prompt
		}
	}

	task := &execution.Task{
		ID:          c.id,
		Type:        c.step.Paradigm,
		Instruction: instruction,
		Data:        make(map[string]any),
		Context:     data,
		Metadata:    c.stepMetadata(),
	}

	if c.step.PromptID != "" {
		task.Context["prompt_id"] = c.step.PromptID
	}

	return task, nil
}

func (c *stepCore) buildAgent(task *execution.Task) (agentgraph.WorkflowExecutor, error) {
	deps := c.deps
	if scopedRegistry := c.scopedRegistry(); scopedRegistry != nil {
		deps = depsWithRegistry(deps, scopedRegistry)
	}

	switch strings.ToLower(strings.TrimSpace(c.step.Paradigm)) {
	case "react":
		opts := c.streamOptions()
		if cap, err := untilIterationCap(c.step.Directives); err != nil {
			return nil, err
		} else if cap > 0 {
			opts = append(opts, reactagent.WithMaxIterations(cap))
		}
		return reactagent.New(deps, opts...), nil
	case "planner":
		plannerOpts, err := plannerOptions(c.step)
		if err != nil {
			return nil, &paradigm.ErrContractViolation{Step: c.step.ID, Paradigm: c.step.Paradigm, Cause: err}
		}
		return planneragent.New(deps, append(c.streamOptionsPlanner(), plannerOpts...)...), nil
	case "htn":
		htnOpts, err := htnOptions(c.step)
		if err != nil {
			return nil, &paradigm.ErrContractViolation{Step: c.step.ID, Paradigm: c.step.Paradigm, Cause: err}
		}
		primitive := reactagent.New(deps, c.streamOptions()...)
		opts := []htnagent.Option{htnagent.WithPrimitiveExec(primitive)}
		opts = append(opts, htnOpts...)
		opts = append(opts, c.streamOptionsHTN()...)
		return htnagent.New(deps, htnruntime.NewMethodLibrary(), opts...), nil
	case "reflection":
		delegate := reactagent.New(deps, c.streamOptions()...)
		reflectionOpts, err := reflectionOptions(c.step, deps)
		if err != nil {
			return nil, &paradigm.ErrContractViolation{Step: c.step.ID, Paradigm: c.step.Paradigm, Cause: err}
		}
		return reflectionagent.New(deps, delegate, reflectionOpts...), nil
	case "blackboard":
		return blackboardagent.New(deps, c.streamOptionsBlackboard()...), nil
	case "chainer":
		chainBuilder := func(*execution.Task) (*chaineragent.Chain, error) {
			return buildChainerChain(c.step.Directives)
		}
		opts := append([]chaineragent.Option{chaineragent.WithChainBuilder(chainBuilder)}, c.streamOptionsChainer()...)
		return chaineragent.New(deps, opts...), nil
	case "pipeline":
		return pipelineagent.New(deps, c.streamOptionsPipeline()...), nil
	case "rewoo":
		rewooOpts, err := rewooOptions(c.step)
		if err != nil {
			return nil, &paradigm.ErrContractViolation{Step: c.step.ID, Paradigm: c.step.Paradigm, Cause: err}
		}
		return rewooagent.New(deps, append(c.streamOptionsRewoo(), rewooOpts...)...), nil
	default:
		// Defense-in-depth: the loader and RegisterCompiled validate every
		// paradigm against the contract registry, so this branch is reachable
		// only when a caller constructs an ExecutionStep bypassing both. Return
		// the typed contract-violation guard instead of a plain-text error.
		return nil, &paradigm.ErrContractViolation{Step: c.step.ID, Paradigm: c.step.Paradigm}
	}
}

func (c *stepCore) writeCaptures(ctx context.Context, env *contextdata.Envelope, result *execution.Result) error {
	if len(c.step.CaptureBindings) == 0 {
		return nil
	}
	_, err := ApplyCaptureBindings(env, c.step.CaptureBindings, execution.ResultFields(result.Data))
	if err != nil {
		return err
	}
	c.enqueueCaptureItems(ctx, env, c.step.CaptureBindings, execution.ResultFields(result.Data))
	return nil
}

func (c *stepCore) renderTemplate(src string, data map[string]any) string {
	src = strings.TrimSpace(src)
	if src == "" {
		return ""
	}
	tpl, err := template.New("thoughtrecipe-step").Option("missingkey=zero").Parse(src)
	if err != nil {
		return src
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return src
	}
	return buf.String()
}

func mustRouteKind(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, "euclo.dispatch.route_kind")
	return strings.TrimSpace(v)
}

func thoughtrecipeTemplateData(env *contextdata.Envelope, step ExecutionStep) map[string]any {
	data := map[string]any{
		"TaskID":    "",
		"SessionID": "",
		"StepID":    step.ID,
		"Paradigm":  step.Paradigm,
		"Prompt":    step.Prompt,
		"Goal":      step.Goal,
	}
	if len(step.Sources) > 0 {
		data["RunSources"] = append([]string(nil), step.Sources...)
	}
	if len(step.Directives) > 0 {
		data["RunDirectives"] = DirectiveNames(step.Directives)
	}
	if env != nil {
		data["TaskID"] = env.TaskIDSnapshot()
		data["SessionID"] = env.SessionIDSnapshot()
		for key, value := range env.Snapshot() {
			data[key] = value
		}
	}
	return data
}

func stringsFromAny(v any) []string {
	switch typed := v.(type) {
	case nil:
		return nil
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{strings.TrimSpace(typed)}
	default:
		return nil
	}
}

func lookupTemplateValue(data map[string]any, ref string) (any, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" || data == nil {
		return nil, false
	}
	value, ok := data[ref]
	return value, ok
}

// newNodeForStep creates the appropriate node type for the step's Kind and
// installs the InteractionResolver on its stepCore (D12) when one is supplied.
func newNodeForStep(id string, deps *paradigm.Deps, resolver interaction.Resolver, step ExecutionStep) agentgraph.Node {
	var node agentgraph.Node
	switch step.Kind {
	case StepKindDelegate:
		node = NewDelegateNode(id, deps, step)
	case StepKindAsk:
		node = NewAskNode(id, deps, step)
	case StepKindCapability:
		node = NewCapabilityNode(id, deps, step)
	default:
		node = NewRunNode(id, deps, step)
	}
	if resolver != nil {
		if settable, ok := node.(interface{ setResolver(interaction.Resolver) }); ok {
			settable.setResolver(resolver)
		}
	}
	return node
}

func askFrameKey(stepID string) string {
	return "euclo.execution.ask." + sanitizeComponent(stepID) + ".frame"
}
