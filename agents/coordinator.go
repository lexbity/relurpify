package agents

import (
	"context"
	"fmt"
	"github.com/lexcodex/relurpify/framework/contextmgr"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"strings"
	"sync"
	"time"
)

// AgentCoordinator manages multiple agents with shared context.
type AgentCoordinator struct {
	agents        map[string]graph.Agent
	sharedContext *core.SharedContext
	contextBroker *ContextBroker
	telemetry     core.Telemetry
	Config        CoordinatorConfig
}

// CoordinatorConfig holds tuning parameters for the coordinator.
type CoordinatorConfig struct {
	MaxRecoveryAttempts int
	MaxReviewIterations int
	ReviewSeverity      string // "error", "warning", "info"
}

// ContextBroker manages context sharing between agents.
type ContextBroker struct {
	mu sync.RWMutex

	indexerCache   map[string]interface{}
	plannerPlan    *core.Plan
	executorFocus  *ExecutorContext
	reviewerIssues []ReviewIssue

	contextManager *contextmgr.ContextManager
	budget         *core.ContextBudget
}

// ExecutorContext tracks executor focus.
type ExecutorContext struct {
	CurrentFile   string
	LoadedFiles   map[string]DetailLevel
	ModifiedFiles []string
}

// ReviewIssue records reviewer findings.
type ReviewIssue struct {
	File     string
	Line     int
	Severity string
	Message  string
}

// NewAgentCoordinator builds an agent coordinator with shared context.
func NewAgentCoordinator(telemetry core.Telemetry, budget *core.ContextBudget) *AgentCoordinator {
	if budget == nil {
		budget = core.NewContextBudget(8192)
	}
	shared := core.NewSharedContext(core.NewContext(), budget, &core.SimpleSummarizer{})
	return &AgentCoordinator{
		agents:        make(map[string]graph.Agent),
		sharedContext: shared,
		contextBroker: &ContextBroker{
			indexerCache:   make(map[string]interface{}),
			executorFocus:  &ExecutorContext{LoadedFiles: make(map[string]DetailLevel)},
			contextManager: contextmgr.NewContextManager(budget),
			budget:         budget,
		},
		telemetry: telemetry,
		Config: CoordinatorConfig{
			MaxRecoveryAttempts: 3,
			MaxReviewIterations: 5,
			ReviewSeverity:      "error",
		},
	}
}

// RegisterAgent adds an agent to coordination pool.
func (ac *AgentCoordinator) RegisterAgent(name string, agent graph.Agent) {
	ac.agents[name] = agent
}

// Execute implements the agent execution interface, allowing the coordinator to be used as a sub-agent.
func (ac *AgentCoordinator) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	if task == nil {
		return nil, fmt.Errorf("task is required")
	}

	// If external state is provided, we sync it with our internal shared context
	if state != nil {
		ac.sharedContext.Context.Merge(state)
	}

	strategy := ac.determineStrategy(task)
	var result *core.Result
	var err error

	switch strategy {
	case "plan_execute":
		result, err = ac.executePlanExecuteStrategy(task)
	case "explore_modify":
		result, err = ac.executeExploreModifyStrategy(task)
	case "review_iterate":
		result, err = ac.executeReviewIterateStrategy(task)
	default:
		result, err = ac.executeSingleAgentStrategy(task)
	}

	// Sync back to external state if successful
	if state != nil && err == nil {
		state.Merge(ac.sharedContext.Context)
	}
	return result, err
}

// ExecuteTask coordinates multiple agents to complete a task.
func (ac *AgentCoordinator) ExecuteTask(task *core.Task) (*core.Result, error) {
	return ac.Execute(context.Background(), task, nil)
}

func (ac *AgentCoordinator) executePlanExecuteStrategy(task *core.Task) (*core.Result, error) {
	indexer, ok := ac.agents["indexer"]
	if ok {
		ac.emitEvent("indexer_start")
		indexTask := cloneTask(task)
		indexTask.Instruction = "Update codebase index with latest changes"
		if _, err := indexer.Execute(context.Background(), indexTask, ac.sharedContext.Context); err != nil {
			return nil, fmt.Errorf("indexer failed: %w", err)
		}
		ac.contextBroker.CacheIndexResults(ac.sharedContext.Context)
	}

	planner, ok := ac.agents["planner"]
	if !ok {
		return nil, fmt.Errorf("planner agent not registered")
	}
	ac.emitEvent("planner_start")
	ac.contextBroker.LoadSummariesIntoContext(ac.sharedContext.Context)
	planTask := cloneTask(task)
	planResult, err := planner.Execute(context.Background(), planTask, ac.sharedContext.Context)
	if err != nil {
		return nil, fmt.Errorf("planner failed: %w", err)
	}
	plan := ac.contextBroker.ExtractPlan(planResult)
	if plan == nil {
		return nil, fmt.Errorf("planner returned no plan")
	}

	executor, ok := ac.agents["executor"]
	if !ok {
		return nil, fmt.Errorf("executor agent not registered")
	}
	ac.emitEvent("executor_start")
	ac.contextBroker.LoadFullFilesForPlan(ac.sharedContext.Context, plan)

	planExecutor := &graph.PlanExecutor{
		Options: graph.PlanExecutionOptions{
			MaxRecoveryAttempts: ac.Config.MaxRecoveryAttempts,
			Diagnose: func(ctx context.Context, step core.PlanStep, err error) (string, error) {
				diagAgent, hasDiag := ac.agents["ask"]
				if !hasDiag || err == nil {
					return "", nil
				}
				diagTask := cloneTask(task)
				diagTask.Instruction = fmt.Sprintf("Analyze why this error occurred: %v", err)
				diagRes, dErr := diagAgent.Execute(ctx, diagTask, ac.sharedContext.Context)
				if dErr != nil || diagRes == nil {
					return "", dErr
				}
				if diagnosis, ok := diagRes.Data["text"].(string); ok {
					return diagnosis, nil
				}
				return "", nil
			},
		},
	}
	execResult, err := planExecutor.Execute(context.Background(), executor, task, plan, ac.sharedContext.Context)
	if err != nil {
		return nil, err
	}

	// Aggregate result (for the reviewer)
	reviewer, ok := ac.agents["reviewer"]
	if ok {
		ac.emitEvent("reviewer_start")
		reviewTask := cloneTask(task)
		reviewTask.Instruction = "Review the changes made"
		if reviewTask.Context == nil {
			reviewTask.Context = map[string]any{}
		}
		reviewTask.Context["original_result"] = execResult
		if reviewResult, err := reviewer.Execute(context.Background(), reviewTask, ac.sharedContext.Context); err == nil {
			ac.contextBroker.StoreReviewIssues(reviewResult)
		} else if ac.telemetry != nil {
			ac.telemetry.Emit(core.Event{
				Type:      "reviewer_failed",
				Timestamp: timeNow(),
				Metadata: map[string]interface{}{
					"error": err.Error(),
				},
			})
		}
	}
	return execResult, nil
}

func (ac *AgentCoordinator) executeExploreModifyStrategy(task *core.Task) (*core.Result, error) {
	asker, ok := ac.agents["ask"]
	if ok {
		exploreTask := cloneTask(task)
		exploreTask.Instruction = fmt.Sprintf("Explore codebase to understand: %s", task.Instruction)
		if exploreResult, err := asker.Execute(context.Background(), exploreTask, ac.sharedContext.Context); err == nil {
			ac.contextBroker.CacheExplorationResults(exploreResult)
		}
	}
	executor, ok := ac.agents["executor"]
	if !ok {
		return nil, fmt.Errorf("executor agent not registered")
	}
	return executor.Execute(context.Background(), task, ac.sharedContext.Context)
}

func (ac *AgentCoordinator) executeReviewIterateStrategy(task *core.Task) (*core.Result, error) {
	executor, ok := ac.agents["executor"]
	reviewer, rok := ac.agents["reviewer"]
	if !ok || !rok {
		return nil, fmt.Errorf("executor or reviewer not registered")
	}
	var result *core.Result
	var err error
	var lastIssues []ReviewIssue

	for iteration := 0; iteration < ac.Config.MaxReviewIterations; iteration++ {
		result, err = executor.Execute(context.Background(), task, ac.sharedContext.Context)
		if err != nil {
			return nil, err
		}
		reviewTask := cloneTask(task)
		reviewTask.Instruction = "Review changes and identify issues"
		if reviewTask.Context == nil {
			reviewTask.Context = map[string]any{}
		}
		reviewTask.Context["iteration"] = iteration
		reviewResult, err := reviewer.Execute(context.Background(), reviewTask, ac.sharedContext.Context)
		if err != nil {
			break
		}
		if passed, ok := reviewResult.Data["passed"].(bool); ok && passed {
			break
		}
		ac.contextBroker.StoreReviewIssues(reviewResult)

		issues, hasIssues := reviewResult.Data["issues"].([]ReviewIssue)
		if !hasIssues || len(issues) == 0 {
			break
		}

		// Filter issues by severity
		var criticalIssues []ReviewIssue
		for _, issue := range issues {
			if isSeverityCritical(issue.Severity, ac.Config.ReviewSeverity) {
				criticalIssues = append(criticalIssues, issue)
			}
		}

		if len(criticalIssues) == 0 {
			break
		}

		// Stalemate detection: if issues are identical to last time, stop
		if areIssuesIdentical(lastIssues, criticalIssues) {
			ac.emitEvent("review_stalemate")
			break
		}
		lastIssues = criticalIssues

		if task.Context == nil {
			task.Context = map[string]any{}
		}
		task.Context["review_issues"] = criticalIssues

		// Update instruction to focus on fixing issues
		var issueDesc strings.Builder
		issueDesc.WriteString("Fix the following review issues:\n")
		for _, issue := range criticalIssues {
			issueDesc.WriteString(fmt.Sprintf("- %s:%d: %s\n", issue.File, issue.Line, issue.Message))
		}
		task.Instruction = issueDesc.String()
	}
	return result, nil
}

func isSeverityCritical(issueSeverity, configSeverity string) bool {
	levels := map[string]int{"info": 0, "warning": 1, "error": 2, "critical": 3}
	return levels[strings.ToLower(issueSeverity)] >= levels[strings.ToLower(configSeverity)]
}

func areIssuesIdentical(a, b []ReviewIssue) bool {
	if len(a) != len(b) {
		return false
	}
	// Simple O(N^2) check is fine for small issue counts
	for i := range a {
		found := false
		for j := range b {
			if a[i] == b[j] {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (ac *AgentCoordinator) executeSingleAgentStrategy(task *core.Task) (*core.Result, error) {
	executor, ok := ac.agents["executor"]
	if ok {
		return executor.Execute(context.Background(), task, ac.sharedContext.Context)
	}
	for _, agent := range ac.agents {
		return agent.Execute(context.Background(), task, ac.sharedContext.Context)
	}
	return nil, fmt.Errorf("no agents registered")
}

func (ac *AgentCoordinator) determineStrategy(task *core.Task) string {
	if task.Metadata != nil {
		if strategy, ok := task.Metadata["strategy"]; ok && strategy != "" {
			return strategy
		}
	}

	instruction := strings.ToLower(task.Instruction)
	if strings.Contains(instruction, "refactor") ||
		strings.Contains(instruction, "redesign") ||
		strings.Contains(instruction, "architecture") {
		return "plan_execute"
	}
	if strings.Contains(instruction, "explore") ||
		strings.Contains(instruction, "understand") ||
		strings.Contains(instruction, "explain") {
		return "explore_modify"
	}
	reqReview := false
	if task.Metadata != nil {
		reqReview = strings.ToLower(task.Metadata["require_review"]) == "true"
	}
	if strings.Contains(instruction, "review") ||
		strings.Contains(instruction, "improve") ||
		reqReview {
		return "review_iterate"
	}
	return "single_agent"
}

func (ac *AgentCoordinator) emitEvent(name string) {
	if ac.telemetry == nil {
		return
	}
	ac.telemetry.Emit(core.Event{
		Type:      core.EventType(name),
		Timestamp: timeNow(),
	})
}

// ContextBroker helpers.
func (cb *ContextBroker) CacheIndexResults(ctx *core.Context) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if summaries, ok := ctx.Get("ast_summaries"); ok {
		cb.indexerCache["ast_summaries"] = summaries
	}
}

func (cb *ContextBroker) LoadSummariesIntoContext(ctx *core.Context) {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	if summaries, ok := cb.indexerCache["ast_summaries"]; ok {
		ctx.Set("loaded_summaries", summaries)
	}
}

func (cb *ContextBroker) ExtractPlan(result *core.Result) *core.Plan {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if result == nil {
		return nil
	}
	var plan *core.Plan
	if value, ok := result.Data["plan"]; ok {
		switch typed := value.(type) {
		case core.Plan:
			copy := typed
			plan = &copy
		case *core.Plan:
			plan = typed
		}
	}
	if plan == nil {
		plan = &core.Plan{
			Steps:        make([]core.PlanStep, 0),
			Files:        make([]string, 0),
			Dependencies: make(map[string][]string),
		}
	}
	if steps, ok := result.Data["plan_steps"].([]core.PlanStep); ok {
		plan.Steps = steps
	}
	if files, ok := result.Data["files"].([]string); ok && len(plan.Files) == 0 {
		plan.Files = files
	}
	if plan.Dependencies == nil {
		plan.Dependencies = make(map[string][]string)
	}
	cb.plannerPlan = plan
	return plan
}

func (cb *ContextBroker) LoadFullFilesForPlan(ctx *core.Context, plan *core.Plan) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if plan == nil {
		return
	}
	for _, file := range plan.Files {
		cb.executorFocus.LoadedFiles[file] = DetailFull
	}
	ctx.Set("executor_files", plan.Files)
}

func (cb *ContextBroker) StoreReviewIssues(result *core.Result) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if result == nil {
		return
	}
	if issues, ok := result.Data["issues"].([]ReviewIssue); ok {
		cb.reviewerIssues = issues
	}
}

func (cb *ContextBroker) CacheExplorationResults(result *core.Result) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if result != nil {
		cb.indexerCache["exploration"] = result.Data
	}
}

func cloneTask(task *core.Task) *core.Task {
	if task == nil {
		return nil
	}
	clone := *task
	if task.Context != nil {
		clone.Context = make(map[string]any, len(task.Context))
		for k, v := range task.Context {
			clone.Context[k] = v
		}
	}
	if task.Metadata != nil {
		clone.Metadata = make(map[string]string, len(task.Metadata))
		for k, v := range task.Metadata {
			clone.Metadata[k] = v
		}
	}
	return &clone
}

func timeNow() time.Time {
	return time.Now().UTC()
}
