package reflection

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	reactpkg "codeburg.org/lexbit/relurpify/agents/react"
	graph "codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	frameworkskills "codeburg.org/lexbit/relurpify/framework/skills"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// ReflectionAgent reviews outputs and triggers revisions when needed.
type ReflectionAgent struct {
	Reviewer      contracts.LanguageModel
	Delegate      graph.WorkflowExecutor
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
func (a *ReflectionAgent) Execute(ctx context.Context, task *core.Task, env *contextdata.Envelope) (*core.Result, error) {
	graph, err := a.BuildGraph(task)
	if err != nil {
		return nil, err
	}
	if cfg := a.Config; cfg != nil && cfg.Telemetry != nil {
		graph.SetTelemetry(cfg.Telemetry)
	}
	if env == nil {
		env = contextdata.NewEnvelope("reflection", "session")
	}
	return graph.Execute(ctx, env)
}

// Capabilities returns capabilities.
func (a *ReflectionAgent) Capabilities() []string {
	return []string{"reflection"}
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
	if err := g.AddEdge(decision.ID(), run.ID(), func(res *core.Result, env *contextdata.Envelope) bool {
		revise, _ := env.GetWorkingValue("reflection.revise")
		return revise == true
	}, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(decision.ID(), done.ID(), func(res *core.Result, env *contextdata.Envelope) bool {
		revise, _ := env.GetWorkingValue("reflection.revise")
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
func (n *reflectionDelegateNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	env.SetExecutionPhase("executing")
	child := env.Clone()
	result, err := n.agent.Delegate.Execute(ctx, n.task, child)
	if err != nil {
		return nil, err
	}
	env.Merge(child)
	env.SetHandleScoped("reflection.last_result", result, taskScope(n.task, env))
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
func (n *reflectionReviewNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	lastResult := resolveResultHandle(env, "reflection.last_result")
	prompt := fmt.Sprintf(`Review the following result for task "%s".
%s
Respond JSON {"issues":[{"severity":"high|medium|low","description":"...","suggestion":"..."}],"approve":bool}
Result: %+v`, n.task.Instruction, reflectionReviewGuidance(n.agent, n.task), compactResultForReview(lastResult))
	resp, err := n.agent.Reviewer.Generate(ctx, prompt, &contracts.LLMOptions{
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
	env.SetWorkingValue("reflection.review", review, contextdata.MemoryClassTask)
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
func (n *reflectionDecisionNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	reviewVal, _ := env.GetWorkingValue("reflection.review")
	review, _ := reviewVal.(reviewPayload)
	iterVal, _ := env.GetWorkingValue("reflection.iteration")
	iter, _ := iterVal.(int)
	iter++
	env.SetWorkingValue("reflection.iteration", iter, contextdata.MemoryClassTask)
	assessment := reflectionAssessmentForReview(n.agent, env, review)
	env.SetWorkingValue("reflection.assessment", assessment, contextdata.MemoryClassTask)
	approve := review.Approve && assessment.Allowed
	revise := !approve && iter < n.agent.maxIterations
	env.SetWorkingValue("reflection.revise", revise, contextdata.MemoryClassTask)
	return &core.Result{NodeID: n.id, Success: true, Data: map[string]interface{}{
		"revise":                 revise,
		"issue_score":            assessment.IssueScore,
		"approval_threshold":     assessment.ApprovalThreshold,
		"missing_verification":   assessment.MissingVerification,
		"blocking_reasons":       append([]string{}, assessment.BlockingReasons...),
		"blocking_issue_count":   assessment.BlockingIssueCount,
		"unresolved_issue_count": assessment.UnresolvedIssueCount,
	}}, nil
}

type reviewPayload struct {
	Issues []struct {
		Severity    string `json:"severity"`
		Description string `json:"description"`
		Suggestion  string `json:"suggestion"`
	} `json:"issues"`
	Approve bool `json:"approve"`
}

type reflectionAssessment struct {
	Allowed              bool
	IssueScore           float64
	ApprovalThreshold    float64
	MissingVerification  bool
	BlockingReasons      []string
	BlockingIssueCount   int
	UnresolvedIssueCount int
}

// parseReview decodes the reviewer JSON into a strongly typed payload.
func parseReview(raw string) (reviewPayload, error) {
	var payload reviewPayload
	if err := json.Unmarshal([]byte(reactpkg.ExtractJSON(raw)), &payload); err != nil {
		return payload, err
	}
	return payload, nil
}

func resolveResultHandle(env *contextdata.Envelope, key string) *core.Result {
	if env == nil {
		return nil
	}
	if value, ok := env.GetHandle(key); ok {
		if res, ok := value.(*core.Result); ok {
			return res
		}
	}
	if value, ok := env.GetWorkingValue(key); ok {
		if res, ok := value.(*core.Result); ok {
			return res
		}
	}
	return nil
}

func taskScope(task *core.Task, env *contextdata.Envelope) string {
	if task != nil && task.ID != "" {
		return task.ID
	}
	if env != nil {
		return envGetString(env, "task.id")
	}
	return ""
}

func envGetString(env *contextdata.Envelope, key string) string {
	val, _ := env.GetWorkingValue(key)
	if s, ok := val.(string); ok {
		return s
	}
	return ""
}

func reflectionReviewGuidance(agent *ReflectionAgent, task *core.Task) string {
	var fallback *agentspec.AgentRuntimeSpec
	if agent != nil && agent.Config != nil {
		fallback = agent.Config.AgentSpec
	}
	effective := frameworkskills.ResolveEffectiveSkillPolicy(task, fallback, nil)
	if effective.Spec == nil {
		return "Consider correctness, completeness, quality, security, performance."
	}
	return frameworkskills.RenderReviewPolicy(effective.Policy)
}

func reflectionApprovalPasses(agent *ReflectionAgent, env *contextdata.Envelope, review reviewPayload) bool {
	if agent == nil || agent.Config == nil || agent.Config.AgentSpec == nil {
		return review.Approve
	}
	return reflectionAssessmentForReview(agent, env, review).Allowed && review.Approve
}

func reflectionAssessmentForReview(agent *ReflectionAgent, env *contextdata.Envelope, review reviewPayload) reflectionAssessment {
	var fallback *agentspec.AgentRuntimeSpec
	if agent != nil && agent.Config != nil {
		fallback = agent.Config.AgentSpec
	}
	effective := frameworkskills.ResolveEffectiveSkillPolicy(nil, fallback, nil)
	if effective.Spec == nil {
		return reflectionAssessment{
			Allowed:              review.Approve,
			IssueScore:           0,
			ApprovalThreshold:    0,
			UnresolvedIssueCount: len(review.Issues),
		}
	}
	policy := effective.Policy
	weights := reflectionSeverityWeights(policy.Review.SeverityWeights)
	threshold := reflectionApprovalThreshold(weights)
	assessment := reflectionAssessment{
		Allowed:              true,
		IssueScore:           0,
		ApprovalThreshold:    threshold,
		UnresolvedIssueCount: len(review.Issues),
	}
	for _, issue := range review.Issues {
		severity := strings.ToLower(strings.TrimSpace(issue.Severity))
		assessment.IssueScore += reflectionSeverityWeight(weights, severity)
		if policy.Review.ApprovalRules.RejectOnUnresolvedErrors && severity == "high" {
			assessment.BlockingIssueCount++
			assessment.BlockingReasons = append(assessment.BlockingReasons, "unresolved high-severity review issue")
		}
	}
	if policy.Review.ApprovalRules.RequireVerificationEvidence && !hasVerificationEvidence(env) {
		assessment.MissingVerification = true
		assessment.BlockingReasons = append(assessment.BlockingReasons, "missing verification evidence")
	}
	if assessment.IssueScore > assessment.ApprovalThreshold {
		assessment.BlockingReasons = append(assessment.BlockingReasons,
			fmt.Sprintf("weighted issue score %.2f exceeds threshold %.2f", assessment.IssueScore, assessment.ApprovalThreshold))
	}
	assessment.BlockingReasons = uniqueStrings(assessment.BlockingReasons)
	assessment.Allowed = !assessment.MissingVerification &&
		assessment.BlockingIssueCount == 0 &&
		assessment.IssueScore <= assessment.ApprovalThreshold
	return assessment
}

func compactResultForReview(result *core.Result) map[string]any {
	if result == nil {
		return map[string]any{"present": false}
	}
	data := map[string]any{
		"present": result != nil,
		"node_id": strings.TrimSpace(result.NodeID),
		"success": result.Success,
	}
	if strings.TrimSpace(result.Error) != "" {
		data["error"] = truncateReflectionString(result.Error)
	}
	if len(result.Data) > 0 {
		data["data"] = compactReflectionValue(result.Data, 0)
	}
	return data
}

const (
	reflectionMaxDepth           = 3
	reflectionMaxMapItems        = 10
	reflectionMaxCollectionItems = 6
	reflectionMaxStringLen       = 400
)

func compactReflectionValue(value any, depth int) any {
	if depth >= reflectionMaxDepth {
		return truncateReflectionString(fmt.Sprint(value))
	}
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return truncateReflectionString(typed)
	case []string:
		limit := reflectionMinInt(len(typed), reflectionMaxCollectionItems)
		out := make([]any, 0, limit+1)
		for i := 0; i < limit; i++ {
			out = append(out, truncateReflectionString(typed[i]))
		}
		if len(typed) > limit {
			out = append(out, fmt.Sprintf("... (%d more)", len(typed)-limit))
		}
		return out
	case []any:
		limit := reflectionMinInt(len(typed), reflectionMaxCollectionItems)
		out := make([]any, 0, limit+1)
		for i := 0; i < limit; i++ {
			out = append(out, compactReflectionValue(typed[i], depth+1))
		}
		if len(typed) > limit {
			out = append(out, fmt.Sprintf("... (%d more)", len(typed)-limit))
		}
		return out
	case map[string]any:
		return compactReflectionMap(typed, depth)
	default:
		return truncateReflectionString(fmt.Sprint(value))
	}
}

func compactReflectionMap(values map[string]any, depth int) map[string]any {
	if len(values) == 0 {
		return map[string]any{}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	limit := reflectionMinInt(len(keys), reflectionMaxMapItems)
	out := make(map[string]any, limit+1)
	for _, key := range keys[:limit] {
		out[key] = compactReflectionValue(values[key], depth+1)
	}
	if len(keys) > limit {
		out["_truncated_keys"] = len(keys) - limit
	}
	return out
}

func truncateReflectionString(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= reflectionMaxStringLen {
		return value
	}
	return value[:reflectionMaxStringLen] + "...(truncated)"
}

func reflectionMinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func reflectionSeverityGuidance(weights map[string]float64) string {
	return frameworkskills.RenderSeverityWeights(weights)
}

func reflectionSeverityWeights(input map[string]float64) map[string]float64 {
	return frameworkskills.ResolveSeverityWeights(input)
}

func reflectionSeverityWeight(weights map[string]float64, severity string) float64 {
	if value, ok := weights[severity]; ok {
		return value
	}
	return weights["medium"]
}

func reflectionApprovalThreshold(weights map[string]float64) float64 {
	if low, ok := weights["low"]; ok {
		return low
	}
	return 0.2
}

func uniqueStrings(input []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, item := range input {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func hasVerificationEvidence(env *contextdata.Envelope) bool {
	if env == nil {
		return false
	}
	if raw, ok := env.GetWorkingValue("react.tool_observations"); ok && raw != nil {
		if observations, ok := raw.([]reactpkg.ToolObservation); ok {
			for _, obs := range observations {
				if obs.Success && (strings.Contains(strings.ToLower(obs.Tool), "test") || strings.Contains(strings.ToLower(obs.Tool), "build") || strings.Contains(strings.ToLower(obs.Tool), "check") || strings.Contains(strings.ToLower(obs.Tool), "query")) {
					return true
				}
			}
		}
	}
	if result := resolveResultHandle(env, "reflection.last_result"); result != nil && result.Success {
		if text := fmt.Sprint(result.Data); strings.Contains(strings.ToLower(text), "summary") || strings.Contains(strings.ToLower(text), "passed") {
			return true
		}
	}
	return false
}
