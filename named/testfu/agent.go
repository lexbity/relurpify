package testfu

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	namedfactory "github.com/lexcodex/relurpify/named/factory"
	runnerpkg "github.com/lexcodex/relurpify/named/testfu/runner"
)

type suiteRunner interface {
	RunSuite(ctx context.Context, suite *runnerpkg.Suite, opts runnerpkg.RunOptions) (*runnerpkg.SuiteReport, error)
}

type Agent struct {
	Config    *core.Config
	Tools     *capability.Registry
	Workspace string
	Runner    suiteRunner
}

func init() {
	namedfactory.RegisterNamedAgent("testfu", func(workspace string, env agentenv.AgentEnvironment) graph.WorkflowExecutor {
		return New(env, WithWorkspace(workspace))
	})
}

func New(env agentenv.AgentEnvironment, opts ...Option) *Agent {
	agent := &Agent{}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	_ = agent.InitializeEnvironment(env)
	return agent
}

func (a *Agent) InitializeEnvironment(env agentenv.AgentEnvironment) error {
	a.Config = env.Config
	if a.Tools == nil {
		a.Tools = env.Registry
	}
	if a.Workspace == "" {
		a.Workspace = workspaceFromContext(nil)
	}
	if a.Runner == nil {
		a.Runner = &runnerpkg.Runner{}
	}
	a.registerTools()
	return a.Initialize(env.Config)
}

func (a *Agent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	if a.Runner == nil {
		a.Runner = &runnerpkg.Runner{}
	}
	return nil
}

func (a *Agent) Capabilities() []core.Capability {
	return []core.Capability{
		core.CapabilityExecute,
		core.CapabilityExplain,
	}
}

func (a *Agent) BuildGraph(_ *core.Task) (*graph.Graph, error) {
	g := graph.NewGraph()
	done := graph.NewTerminalNode("testfu_done")
	if err := g.AddNode(done); err != nil {
		return nil, err
	}
	if err := g.SetStart(done.ID()); err != nil {
		return nil, err
	}
	return g, nil
}

func (a *Agent) Execute(ctx context.Context, task *core.Task, state *core.Context) (*core.Result, error) {
	if state == nil {
		state = core.NewContext()
	}
	request := parseRequest(task)
	if request.Action == actionRunAgent {
		return a.executeRunAgent(ctx, request, state)
	}
	report, allPassed, err := a.executeRequest(ctx, request)
	if err != nil {
		return nil, err
	}
	failedCases := failedCaseNames(report)
	state.Set("testfu.report", report)
	state.Set("testfu.passed", allPassed)
	state.Set("testfu.failed_cases", failedCases)
	return &core.Result{
		Success: allPassed,
		Data: map[string]any{
			"report":       report,
			"passed":       allPassed,
			"failed_cases": failedCases,
		},
	}, nil
}

func (a *Agent) executeRunAgent(ctx context.Context, req runRequest, state *core.Context) (*core.Result, error) {
	suiteReports, allPassed, err := a.runAgentSuites(ctx, req)
	if err != nil {
		return nil, err
	}
	var totalPassed, totalFailed, totalSkipped int
	for _, r := range suiteReports {
		totalPassed += r.PassedCases
		totalFailed += r.FailedCases
		totalSkipped += r.SkippedCases
	}
	reportMap := map[string]any{"suites": suiteReports}
	failedCases := failedCaseNames(reportMap)
	state.Set("testfu.agent_suites_report", suiteReports)
	state.Set("testfu.passed", allPassed)
	state.Set("testfu.total_passed", totalPassed)
	state.Set("testfu.total_failed", totalFailed)
	state.Set("testfu.total_skipped", totalSkipped)
	state.Set("testfu.failed_cases", failedCases)
	return &core.Result{
		Success: allPassed,
		Data: map[string]any{
			"report":        suiteReports,
			"passed":        allPassed,
			"total_passed":  totalPassed,
			"total_failed":  totalFailed,
			"total_skipped": totalSkipped,
			"failed_cases":  failedCases,
		},
	}, nil
}

// runAgentSuites globs all suite files matching the agent name, applies tag
// filtering, distributes the context deadline budget across runs, and marks
// remaining suites as skipped when the budget is exhausted.
func (a *Agent) runAgentSuites(ctx context.Context, req runRequest) (map[string]*runnerpkg.SuiteReport, bool, error) {
	seen := map[string]struct{}{}
	var paths []string
	for _, pattern := range []string{
		filepath.Join(req.Workspace, "testsuite", "agenttests", req.AgentName+".testsuite.yaml"),
		filepath.Join(req.Workspace, "testsuite", "agenttests", req.AgentName+".*.testsuite.yaml"),
	} {
		matches, _ := filepath.Glob(pattern)
		sort.Strings(matches)
		for _, m := range matches {
			if _, ok := seen[m]; !ok {
				seen[m] = struct{}{}
				paths = append(paths, m)
			}
		}
	}
	results := make(map[string]*runnerpkg.SuiteReport, len(paths))
	deadline, hasDeadline := ctx.Deadline()
	for i, suitePath := range paths {
		if hasDeadline && time.Until(deadline) <= 0 {
			for _, remaining := range paths[i:] {
				results[filepath.Base(remaining)] = &runnerpkg.SuiteReport{SkippedCases: 1}
			}
			break
		}
		suite, err := runnerpkg.LoadSuite(suitePath)
		if err != nil {
			return results, false, err
		}
		if len(req.Tags) > 0 {
			suite = runnerpkg.FilterSuiteCasesByTags(suite, req.Tags)
		}
		if len(suite.Spec.Cases) == 0 {
			continue
		}
		opts := req.RunOptions()
		if hasDeadline {
			opts.Timeout = runnerpkg.BudgetedTimeout(time.Until(deadline), len(paths)-i, 30*time.Second)
		}
		report, err := a.Runner.RunSuite(ctx, suite, opts)
		if err != nil {
			return results, false, err
		}
		results[filepath.Base(suitePath)] = report
	}
	allPassed := true
	for _, r := range results {
		if !suitePassed(r) {
			allPassed = false
			break
		}
	}
	return results, allPassed, nil
}

func (a *Agent) CapabilityRegistry() *capability.Registry {
	if a == nil {
		return nil
	}
	return a.Tools
}

func (a *Agent) executeRequest(ctx context.Context, req runRequest) (map[string]any, bool, error) {
	switch req.Action {
	case actionListSuites:
		suites, err := listSuites(req.Workspace)
		if err != nil {
			return nil, false, err
		}
		return map[string]any{
			"action": "list_suites",
			"suites": suites,
		}, true, nil
	case actionRunCase:
		report, err := a.runCase(ctx, req)
		if err != nil {
			return nil, false, err
		}
		return map[string]any{
			"action": "run_case",
			"case":   report,
		}, report.Success, nil
	default:
		report, err := a.runSuite(ctx, req)
		if err != nil {
			return nil, false, err
		}
		passed := suitePassed(report)
		return map[string]any{
			"action": "run_suite",
			"suite":  report,
		}, passed, nil
	}
}

func (a *Agent) runSuite(ctx context.Context, req runRequest) (*runnerpkg.SuiteReport, error) {
	suitePath, err := resolveSuitePath(req.Workspace, req.SuitePath)
	if err != nil {
		return nil, err
	}
	suite, err := runnerpkg.LoadSuite(suitePath)
	if err != nil {
		return nil, err
	}
	return a.Runner.RunSuite(ctx, suite, req.RunOptions())
}

func (a *Agent) runCase(ctx context.Context, req runRequest) (*runnerpkg.CaseReport, error) {
	suitePath, err := resolveSuitePath(req.Workspace, req.SuitePath)
	if err != nil {
		return nil, err
	}
	suite, err := runnerpkg.LoadSuite(suitePath)
	if err != nil {
		return nil, err
	}
	filtered := *suite
	filtered.Spec.Cases = nil
	for _, c := range suite.Spec.Cases {
		if strings.EqualFold(strings.TrimSpace(c.Name), strings.TrimSpace(req.CaseName)) {
			filtered.Spec.Cases = append(filtered.Spec.Cases, c)
			break
		}
	}
	if len(filtered.Spec.Cases) == 0 {
		return nil, fmt.Errorf("testfu: case %q not found in %s", req.CaseName, filepath.Base(suitePath))
	}
	report, err := a.Runner.RunSuite(ctx, &filtered, req.RunOptions())
	if err != nil {
		return nil, err
	}
	if len(report.Cases) == 0 {
		return nil, fmt.Errorf("testfu: suite run returned no case reports")
	}
	return &report.Cases[0], nil
}

func workspaceFromContext(task *core.Task) string {
	if task != nil && task.Context != nil {
		if value := strings.TrimSpace(fmt.Sprint(task.Context["workspace"])); value != "" && value != "<nil>" {
			return value
		}
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

type action string

const (
	actionRunSuite   action = "run_suite"
	actionRunCase    action = "run_case"
	actionListSuites action = "list_suites"
	actionRunAgent   action = "run_agent"
)

type runRequest struct {
	Action    action
	Workspace string
	SuitePath string
	CaseName  string
	AgentName string // for actionRunAgent
	Tags      []string
	Lane      string
	Model     string
	Endpoint  string
	Timeout   time.Duration
}

func (r runRequest) RunOptions() runnerpkg.RunOptions {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	return runnerpkg.RunOptions{
		TargetWorkspace:  r.Workspace,
		Timeout:          timeout,
		ModelOverride:    strings.TrimSpace(r.Model),
		EndpointOverride: strings.TrimSpace(r.Endpoint),
		Lane:             strings.TrimSpace(r.Lane),
	}
}
