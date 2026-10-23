package critic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	reactpkg "codeburg.org/lexbit/relurpify/cognitionzoo/react"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	graph "codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/telemetry"
)

// CriticAgent critiques work from an independent phase — its own model,
// its own criteria — and triggers bounded revisions when authored.
type CriticAgent struct {
	Critic        model.LanguageModel
	Delegate      graph.WorkflowExecutor
	Config        *execution.Config
	maxIterations int

	// permissionChecker is the governance bar (D14): the critic refuses to
	// run without one, exactly as ReWOO does. The critique phase itself is
	// model-only (data, not action), but the delegate it drives is the
	// governed action surface, and a paradigm that cannot answer "who says
	// this agent may act" does not act.
	permissionChecker permissions.CapabilityChecker

	// Authored directive configuration (contract: critique/revise). The zero
	// value preserves the library critique report (FR-9).
	draftCriteria    string
	critiqueCriteria string
	reviseBody       ReviseBodyFunc
	revisePredicate  func(*contextdata.Envelope) bool
}

// MaxRevisionCycles is the authored revise-loop cap (contract constant, not
// authored): at most two revise bodies run before the last verdict stands.
const MaxRevisionCycles = 2

// ReviseBodyFunc executes the authored revise body in-process: the lowered
// body steps run sequentially and return their last result.
type ReviseBodyFunc func(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error)

// ErrNoPermissionChecker is returned when the critic is executed without a
// permission checker. Fail closed: an agent that cannot answer "who says
// this may act" does not act.
var ErrNoPermissionChecker = errors.New("critic agent requires a permission checker (fail closed)")

// ErrInvalidCritique reports critique output that is not the required JSON
// verdict object.
var ErrInvalidCritique = errors.New("critic critique output is not a valid verdict")

type Option func(*CriticAgent)

// WithDraftCriteria installs the authored drafting criterion for the
// directive-mode draft phase. When unset, the delegate's own instruction
// governs.
func WithDraftCriteria(text string) Option {
	return func(a *CriticAgent) {
		if a != nil {
			a.draftCriteria = strings.TrimSpace(text)
		}
	}
}

// WithCritiqueCriteria installs the authored critique criterion that drives
// the directive-mode critique phase (the contract's `critique` directive).
// When unset, the library critique prompt governs.
func WithCritiqueCriteria(text string) Option {
	return func(a *CriticAgent) {
		if a != nil {
			a.critiqueCriteria = strings.TrimSpace(text)
		}
	}
}

// WithReviseCycle installs an authored revise cycle: after each critique the
// predicate is evaluated against the envelope (scratch-aware); when it reads
// true the body runs in-process and the loop re-critiques, bounded by
// MaxRevisionCycles. A nil body disables the cycle.
func WithReviseCycle(body ReviseBodyFunc, predicate func(*contextdata.Envelope) bool) Option {
	return func(a *CriticAgent) {
		if a != nil {
			a.reviseBody = body
			a.revisePredicate = predicate
		}
	}
}

func New(deps *paradigm.Deps, delegate graph.WorkflowExecutor, opts ...Option) *CriticAgent {
	if delegate == nil {
		delegate = reactpkg.New(deps)
	}
	agent := &CriticAgent{Delegate: delegate}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeDeps(deps)
	return agent
}

// InitializeDeps wires the runtime surface: the critique model, the config,
// the permission checker, and the delegate's own dependency initialization.
func (a *CriticAgent) InitializeDeps(deps *paradigm.Deps) error {
	if deps == nil {
		return fmt.Errorf("critic dependencies unavailable")
	}
	a.Critic = deps.Model
	a.Config = deps.Config
	a.permissionChecker = deps.PermissionChecker
	if envAware, ok := a.Delegate.(interface {
		InitializeDeps(*paradigm.Deps) error
	}); ok {
		if err := envAware.InitializeDeps(deps); err != nil {
			return err
		}
	}
	return a.Initialize(deps.Config)
}

// Initialize configures the critic.
func (a *CriticAgent) Initialize(cfg *execution.Config) error {
	a.Config = cfg
	if cfg == nil || cfg.MaxIterations <= 0 {
		a.maxIterations = 3
	} else {
		a.maxIterations = cfg.MaxIterations
	}
	return nil
}

// Execute runs the critic workflow. It refuses to run without a permission
// checker — the ReWOO bar, stated as an error rather than a silent
// downgrade.
func (a *CriticAgent) Execute(ctx context.Context, task *execution.Task, env *contextdata.Envelope) (*execution.Result, error) {
	if a.permissionChecker == nil {
		return nil, ErrNoPermissionChecker
	}
	if env == nil {
		env = contextdata.NewEnvelope("critic", "session")
	}
	if a.directiveMode() {
		return a.executeDirective(ctx, task, env)
	}
	g, err := a.BuildGraph(ctx, task)
	if err != nil {
		return nil, err
	}
	if cfg := a.Config; cfg != nil && cfg.Telemetry != nil {
		if err := g.SetTelemetry(cfg.Telemetry); err != nil {
			return nil, err
		}
	}
	result, err := g.Execute(ctx, env)
	iterations := 0
	if value, ok := contextdata.GetTyped[int](env, "critic.iteration"); ok {
		iterations = value
	}
	a.emitCompleted(ctx, err == nil && result != nil && result.Success, iterations)
	return result, err
}

// Capabilities returns capabilities.
func (a *CriticAgent) Capabilities() []string {
	return []string{"critic"}
}

// directiveMode reports whether authored critique/revise directives drive
// this run. When false, the library critique report is unchanged (FR-9).
func (a *CriticAgent) directiveMode() bool {
	return a != nil && (strings.TrimSpace(a.draftCriteria) != "" ||
		strings.TrimSpace(a.critiqueCriteria) != "" || a.reviseBody != nil || a.revisePredicate != nil)
}

// executeDirective runs the authored loop: draft (delegate) → critique →
// (predicate → body → critique)ⁿ with n ≤ MaxRevisionCycles. A `fail`
// verdict is the quality signal the predicate routes on, not a failure.
// The revise body executes in-process through the delegate execution core,
// and the cap ends the loop normally with the last verdict standing.
func (a *CriticAgent) executeDirective(ctx context.Context, task *execution.Task, env *contextdata.Envelope) (*execution.Result, error) {
	if a.Critic == nil {
		return nil, fmt.Errorf("critic agent missing critique model")
	}
	lastResult, err := a.runDelegateWork(ctx, task, env)
	if err != nil {
		return nil, err
	}
	lastVerdict := ""
	var lastIssues []string
	bodyRuns := 0
	iterations := 0
	for {
		verdict, issues, err := a.runCritique(ctx, task, env, lastResult, a.critiqueCriteria)
		if err != nil {
			return nil, err
		}
		iterations++
		lastVerdict, lastIssues = verdict, issues
		predicateHolds := a.reviseBody != nil && a.revisePredicate != nil && a.revisePredicate(env)
		a.emitIteration(ctx, iterations, verdict == "pass", predicateHolds, bodyRuns)
		if !predicateHolds {
			break
		}
		if bodyRuns >= MaxRevisionCycles {
			a.emitRevisionCapped(ctx, iterations)
			break
		}
		bodyRuns++
		env.SetWorkingValueWithClass("critic.revise_cycle", bodyRuns, contextdata.MemoryClassTask)
		lastResult, err = a.reviseBody(ctx, env)
		if err != nil {
			return nil, err
		}
	}
	env.SetWorkingValueWithClass("critic.iteration", iterations, contextdata.MemoryClassTask)
	a.emitCompleted(ctx, true, iterations)
	return &execution.Result{
		NodeID:  "critic_directive",
		Success: true,
		Data: execution.NewToolResultPayload(map[string]any{
			"critique":        lastVerdict,
			"critique_issues": lastIssues,
			"revisions":       bodyRuns,
		}),
	}, nil
}

// runDelegateWork executes the draft phase through the delegate with state
// isolation (clone → execute → merge), so the critique phase sees the
// draft's results and the delegate never sees the critique.
func (a *CriticAgent) runDelegateWork(ctx context.Context, task *execution.Task, env *contextdata.Envelope) (*execution.Result, error) {
	if a.Delegate == nil {
		return nil, fmt.Errorf("critic agent missing delegate")
	}
	env.SetExecutionPhase("drafting")
	child := env.Clone()
	result, err := a.Delegate.Execute(ctx, task, child)
	if err != nil {
		return nil, err
	}
	env.Merge(child)
	contextdata.SetTyped(env, "critic.last_result", result)
	return result, nil
}

// runCritique issues one critique model call and writes the verdict to the
// ephemeral scratch namespace (scratch.critic.verdict / scratch.critic.issues),
// which predicates route on. The critique phase's model never saw the
// drafting phase's reasoning — the separation is the mechanism.
func (a *CriticAgent) runCritique(ctx context.Context, task *execution.Task, env *contextdata.Envelope, lastResult *execution.Result, criteria string) (string, []string, error) {
	// The substrate's compiled slice renders exactly once, into the
	// critique's system slot (D-2); renderer inconsistencies fail the node.
	streamed, err := paradigm.StreamedSection(ctx, env, "critic")
	if err != nil {
		return "", nil, err
	}
	critiqueMessages := []model.Message{
		{Role: "system", Content: critiqueSystemPrompt},
	}
	if streamed != "" {
		critiqueMessages = append(critiqueMessages, model.Message{Role: "system", Content: streamed})
	}
	critiqueMessages = append(critiqueMessages, model.Message{
		Role: "user", Content: critiqueUserPrompt(taskInstruction(task), criteria, compactResultForCritique(lastResult)),
	})
	resp, err := a.Critic.Chat(ctx, critiqueMessages, &model.LLMOptions{
		Model:       a.modelID(),
		Temperature: 0,
		MaxTokens:   512,
	})
	if err != nil {
		return "", nil, err
	}
	verdict, issues, err := decodeCritiqueVerdict(resp.Text)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrInvalidCritique, err)
	}
	env.SetWorkingValueWithClass("scratch.critic.verdict", verdict, contextdata.MemoryClassTask)
	env.SetWorkingValueWithClass("scratch.critic.issues", issues, contextdata.MemoryClassTask)
	a.emitReviewed(ctx, verdict, issues)
	return verdict, issues, nil
}

func (a *CriticAgent) modelID() string {
	if a.Config != nil {
		return a.Config.Model
	}
	return ""
}

func taskInstruction(task *execution.Task) string {
	if task == nil {
		return ""
	}
	return task.Instruction
}

const critiqueSystemPrompt = `You are an independent critique agent. You did not write the work under review. Judge whether it satisfies the criterion. Reply with ONLY a JSON object, no prose.`

func critiqueUserPrompt(instruction, criterion string, result map[string]any) string {
	return fmt.Sprintf(`Task: %s
Criterion: %s
Work result: %v
Return JSON: {"verdict":"pass"|"fail"|"issues","issues":["..."]}`, instruction, criterion, result)
}

// decodeCritiqueVerdict strictly parses a critique verdict. The vocabulary is
// {pass, fail, issues}: "issues" is the revision-needed signal the canonical
// `scratch.critic.verdict contains issues` predicate routes on; any other
// value is invalid model output.
func decodeCritiqueVerdict(raw string) (string, []string, error) {
	var wire struct {
		Verdict string   `json:"verdict"`
		Issues  []string `json:"issues"`
	}
	if err := json.Unmarshal([]byte(reactpkg.ExtractJSON(raw)), &wire); err != nil {
		return "", nil, fmt.Errorf("critique JSON decode: %w", err)
	}
	switch wire.Verdict {
	case "pass", "fail", "issues":
	default:
		return "", nil, fmt.Errorf("invalid critique verdict %q", wire.Verdict)
	}
	return wire.Verdict, wire.Issues, nil
}

func compactResultForCritique(result *execution.Result) map[string]any {
	if result == nil {
		return map[string]any{}
	}
	fields := execution.ResultFields(result.Data)
	compact := make(map[string]any, len(fields))
	for key, value := range fields {
		if text, ok := value.(string); ok && len(text) > 512 {
			compact[key] = text[:512] + "…"
			continue
		}
		compact[key] = value
	}
	return compact
}

// BuildGraph wires the library-mode critique report: delegate run →
// critique → done. Authored directives drive the revision loop instead
// (executeDirective); the library critic reports and stops.
func (a *CriticAgent) BuildGraph(ctx context.Context, task *execution.Task) (*graph.Graph, error) {
	if a.Delegate == nil {
		return nil, fmt.Errorf("critic agent missing delegate")
	}
	if a.Critic == nil {
		return nil, fmt.Errorf("critic agent missing critique model")
	}
	g := graph.NewGraph()
	run := &criticDelegateNode{id: "critic_execute", agent: a, task: task}
	critique := &criticReportNode{id: "critic_critique", agent: a, task: task}
	done := graph.NewTerminalNode("critic_done")
	for _, node := range []graph.Node{run, critique, done} {
		if err := g.AddNode(node); err != nil {
			return nil, err
		}
	}
	if err := g.AddEdge(run.ID(), critique.ID(), nil, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(critique.ID(), done.ID(), nil, false); err != nil {
		return nil, err
	}
	if err := g.SetStart(run.ID()); err != nil {
		return nil, err
	}
	return g, nil
}

type criticDelegateNode struct {
	id    string
	agent *CriticAgent
	task  *execution.Task
}

func (n *criticDelegateNode) ID() string { return n.id }

func (n *criticDelegateNode) Type() graph.NodeType { return graph.NodeTypeSystem }

// Execute runs the delegate agent while isolating state mutations until the
// child run succeeds.
func (n *criticDelegateNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	return n.agent.runDelegateWork(ctx, n.task, env)
}

type criticReportNode struct {
	id    string
	agent *CriticAgent
	task  *execution.Task
}

func (n *criticReportNode) ID() string { return n.id }

func (n *criticReportNode) Type() graph.NodeType { return graph.NodeTypeObservation }

// Contract declares the report node: model-only, replay-safe.
func (n *criticReportNode) Contract() graph.NodeContract {
	return graph.NodeContract{
		SideEffectClass: graph.SideEffectNone,
		Idempotency:     graph.IdempotencyReplaySafe,
	}
}

func (n *criticReportNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	lastResult, _ := contextdata.GetTyped[*execution.Result](env, "critic.last_result")
	verdict, issues, err := n.agent.runCritique(ctx, n.task, env, lastResult, n.agent.critiqueCriteria)
	if err != nil {
		return nil, err
	}
	env.SetWorkingValueWithClass("critic.iteration", 1, contextdata.MemoryClassTask)
	return &execution.Result{
		Success: true,
		Data: execution.NewToolResultPayload(map[string]any{
			"critique":        verdict,
			"critique_issues": issues,
			"revisions":       0,
		}),
	}, nil
}

// Telemetry: paradigm-level events carrying the critique-iteration
// decisions the graph model does not. The event-type strings follow the
// shipped `<paradigm>.<event>` vocabulary; upstreaming named constants
// into the telemetry package is part of the contribution checklist.
const (
	eventCriticIteration      telemetry.EventType = "critic.iteration"
	eventCriticCompleted      telemetry.EventType = "critic.completed"
	eventCriticReviewed       telemetry.EventType = "critic.reviewed"
	eventCriticRevisionCapped telemetry.EventType = "critic.revision_capped"
)

func (a *CriticAgent) emit(ctx context.Context, eventType telemetry.EventType, message string, metadata map[string]any) {
	if a == nil || a.Config == nil || a.Config.Telemetry == nil {
		return
	}
	ev := telemetry.Event{
		Type:      eventType,
		Message:   message,
		Timestamp: time.Now().UTC(),
		Metadata:  metadata,
	}
	telemetry.StampCorrelation(ctx, &ev)
	a.Config.Telemetry.Emit(ev)
}

func (a *CriticAgent) emitIteration(ctx context.Context, iteration int, approve, revise bool, reviseCycle int) {
	a.emit(ctx, eventCriticIteration, "critic iteration", map[string]any{
		"iteration":    iteration,
		"approve":      approve,
		"revise":       revise,
		"revise_cycle": reviseCycle,
		"paradigm":     "critic",
	})
}

func (a *CriticAgent) emitReviewed(ctx context.Context, verdict string, issues []string) {
	a.emit(ctx, eventCriticReviewed, "critic reviewed", map[string]any{
		"verdict":  verdict,
		"issues":   issues,
		"paradigm": "critic",
	})
}

func (a *CriticAgent) emitRevisionCapped(ctx context.Context, iteration int) {
	a.emit(ctx, eventCriticRevisionCapped, "critic revision capped", map[string]any{
		"iteration": iteration,
		"paradigm":  "critic",
	})
}

func (a *CriticAgent) emitCompleted(ctx context.Context, success bool, iterations int) {
	a.emit(ctx, eventCriticCompleted, "critic completed", map[string]any{
		"success":    success,
		"iterations": iterations,
		"paradigm":   "critic",
	})
}
