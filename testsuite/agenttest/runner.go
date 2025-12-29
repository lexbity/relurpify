package agenttest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lexcodex/relurpify/agents"
	appruntime "github.com/lexcodex/relurpify/app/relurpish/runtime"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/memory"
	fruntime "github.com/lexcodex/relurpify/framework/runtime"
	"github.com/lexcodex/relurpify/framework/telemetry"
	"github.com/lexcodex/relurpify/framework/toolsys"
	"github.com/lexcodex/relurpify/llm"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type RunOptions struct {
	TargetWorkspace string
	OutputDir       string
	Sandbox         bool
	Timeout         time.Duration

	ModelOverride    string
	EndpointOverride string

	MaxIterations int
	DebugLLM      bool
	DebugAgent    bool

	OllamaReset        string   // none|model|server
	OllamaBinary       string   // default: ollama
	OllamaService      string   // default: ollama
	OllamaResetOn      []string // regexes matched against error to trigger reset+retry
	OllamaResetBetween bool     // reset before each case
}

type SuiteReport struct {
	SuitePath  string
	RunID      string
	StartedAt  time.Time
	FinishedAt time.Time
	Cases      []CaseReport
}

type CaseReport struct {
	Name         string
	Model        string
	Endpoint     string
	Workspace    string
	ArtifactsDir string

	Skipped    bool
	SkipReason string

	Success      bool
	Error        string
	Output       string
	ChangedFiles []string
	ToolCalls    map[string]int
}

type Runner struct {
	Logger *log.Logger
}

func (r *Runner) RunSuite(ctx context.Context, suite *Suite, opts RunOptions) (*SuiteReport, error) {
	if suite == nil {
		return nil, errors.New("suite required")
	}
	if opts.TargetWorkspace == "" {
		return nil, errors.New("target workspace required")
	}
	targetWorkspace, err := filepath.Abs(opts.TargetWorkspace)
	if err != nil {
		return nil, err
	}
	runID := time.Now().UTC().Format("20060102-150405")
	outDir := opts.OutputDir
	if outDir == "" {
		outDir = filepath.Join(targetWorkspace, "relurpify_cfg", "test_runs", suite.Spec.AgentName, runID)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}

	report := &SuiteReport{
		SuitePath:  suite.SourcePath,
		RunID:      runID,
		StartedAt:  time.Now().UTC(),
		FinishedAt: time.Time{},
	}

	models := suite.Spec.Models
	if len(models) == 0 {
		models = []ModelSpec{{Name: "", Endpoint: ""}}
	}

	for _, c := range suite.Spec.Cases {
		caseModels := models
		if c.Overrides.Model != nil {
			caseModels = []ModelSpec{*c.Overrides.Model}
		}
		for _, model := range caseModels {
			cr := r.runCase(ctx, suite, c, model, opts, targetWorkspace, outDir)
			report.Cases = append(report.Cases, cr)
		}
	}

	report.FinishedAt = time.Now().UTC()
	data, _ := json.MarshalIndent(report, "", "  ")
	_ = os.WriteFile(filepath.Join(outDir, "report.json"), data, 0o644)
	return report, nil
}

func (r *Runner) runCase(ctx context.Context, suite *Suite, c CaseSpec, model ModelSpec, opts RunOptions, targetWorkspace, outDir string) CaseReport {
	caseDir := filepath.Join(outDir, fmt.Sprintf("%s__%s", sanitizeName(c.Name), sanitizeName(model.Name)))
	_ = os.MkdirAll(caseDir, 0o755)
	telemetryPath := filepath.Join(caseDir, "telemetry.jsonl")
	logPath := filepath.Join(caseDir, "agenttest.log")

	logger := r.Logger
	if logger == nil {
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			defer logFile.Close()
			logger = log.New(logFile, "agenttest ", log.LstdFlags|log.Lmicroseconds)
		} else {
			logger = log.New(os.Stderr, "agenttest ", log.LstdFlags|log.Lmicroseconds)
		}
	}

	workspaceStrategy := suite.Spec.Workspace.Strategy
	exclude := append([]string{}, suite.Spec.Workspace.Exclude...)
	if c.Overrides.Workspace != nil {
		if c.Overrides.Workspace.Strategy != "" {
			workspaceStrategy = c.Overrides.Workspace.Strategy
		}
		if len(c.Overrides.Workspace.Exclude) > 0 {
			exclude = append([]string{}, c.Overrides.Workspace.Exclude...)
		}
	}
	if workspaceStrategy == "" {
		workspaceStrategy = "copy"
	}
	if len(exclude) == 0 {
		exclude = []string{
			".git/**",
			".gocache/**",
			".gomodcache/**",
			"relurpify_cfg/test_runs/**",
		}
	}

	workspace := targetWorkspace
	if strings.EqualFold(workspaceStrategy, "copy") {
		workspace = filepath.Join(caseDir, "workspace")
		_ = os.RemoveAll(workspace)
		if err := os.MkdirAll(workspace, 0o755); err == nil {
			if err := CopyWorkspace(targetWorkspace, workspace, exclude); err != nil {
				return CaseReport{Name: c.Name, Model: model.Name, Endpoint: model.Endpoint, Workspace: workspace, ArtifactsDir: caseDir, Success: false, Error: err.Error()}
			}
		}
	}

	suiteManifestAbs := suite.ResolvePath(suite.Spec.Manifest)
	suiteManifestAbs = resolveAgainstWorkspace(targetWorkspace, suiteManifestAbs, suite.Spec.Manifest)
	manifestAbs := suiteManifestAbs
	if strings.EqualFold(workspaceStrategy, "copy") {
		manifestAbs = mapTargetPathToWorkspace(suiteManifestAbs, targetWorkspace, workspace)
	}
	manifestAbs = fallbackManifestPath(manifestAbs, workspace)
	// In in-place mode we keep the manifest as-is (tool authorization depends on
	// broad tool-level permissions), and enforce safety via the agent file matrix
	// instead (default-deny with allowlisted paths).

	// Apply fixtures before taking the baseline snapshot so setup changes don't
	// count as agent-driven modifications.
	cleanup, err := applySetup(workspace, c.Setup, opts.Sandbox, logger)
	if err != nil {
		return CaseReport{Name: c.Name, Model: model.Name, Endpoint: model.Endpoint, Workspace: workspace, ArtifactsDir: caseDir, Success: false, Error: err.Error()}
	}
	if cleanup != nil {
		defer cleanup()
	}
	before, err := SnapshotWorkspace(workspace, exclude)
	if err != nil {
		return CaseReport{Name: c.Name, Model: model.Name, Endpoint: model.Endpoint, Workspace: workspace, ArtifactsDir: caseDir, Success: false, Error: err.Error()}
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}

	agentName := suite.Spec.AgentName
	telemetrySink, err := telemetry.NewJSONFileTelemetry(telemetryPath)
	if err != nil {
		return CaseReport{Name: c.Name, Model: model.Name, Endpoint: model.Endpoint, Workspace: workspace, ArtifactsDir: caseDir, Success: false, Error: err.Error()}
	}
	defer telemetrySink.Close()
	telemetry := telemetry.MultiplexTelemetry{Sinks: []core.Telemetry{telemetrySink}}

	modelName := firstNonEmpty(opts.ModelOverride, model.Name)
	endpoint := firstNonEmpty(opts.EndpointOverride, model.Endpoint)
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}

	if opts.OllamaResetBetween {
		maybeResetOllama(logger, opts, modelName)
	}

	client := llm.NewClient(endpoint, modelName)
	client.SetDebugLogging(opts.DebugLLM)

	lm := core.LanguageModel(client)
	recording := suite.Spec.Recording
	if c.Overrides.Recording != nil {
		recording = *c.Overrides.Recording
	}
	if recording.Mode != "" && recording.Mode != "off" {
		tapePath := recording.Tape
		if tapePath == "" {
			tapePath = filepath.Join(caseDir, "tape.jsonl")
		} else {
			resolved := suite.ResolvePath(tapePath)
			tapePath = resolveAgainstWorkspace(targetWorkspace, resolved, tapePath)
			if strings.EqualFold(workspaceStrategy, "copy") {
				tapePath = mapTargetPathToWorkspace(tapePath, targetWorkspace, workspace)
			}
		}
		wrapped, err := llm.NewTapeModel(lm, tapePath, recording.Mode)
		if err == nil {
			lm = wrapped
		}
	}
	instrumented := llm.NewInstrumentedModel(lm, telemetry, opts.DebugLLM)

	spec := &core.AgentRuntimeSpec{}
	manifest, err := manifest.LoadAgentManifest(manifestAbs)
	if err == nil && manifest.Spec.Agent != nil {
		spec = manifest.Spec.Agent
	}
	spec = effectiveAgentSpecForCase(spec, c)
	// Apply any case tool matrix overrides (useful for prompt-only tests).
	if len(c.Overrides.ToolMatrix) > 0 {
		if spec.Tools == (core.AgentToolMatrix{}) {
			spec.Tools = core.AgentToolMatrix{}
		}
		for k, v := range c.Overrides.ToolMatrix {
			switch strings.ToLower(k) {
			case "file_read":
				spec.Tools.FileRead = v
			case "file_write":
				spec.Tools.FileWrite = v
			case "file_edit":
				spec.Tools.FileEdit = v
			case "bash_execute":
				spec.Tools.BashExecute = v
			case "lsp_query":
				spec.Tools.LSPQuery = v
			case "search_codebase":
				spec.Tools.SearchCodebase = v
			case "web_search":
				spec.Tools.WebSearch = v
			}
		}
	}

	env := make([]string, 0, len(c.Overrides.ExtraEnv))
	for k, v := range c.Overrides.ExtraEnv {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(env)

	allowedTools := mergeStrings(defaultAgenttestAllowedTools(), c.Overrides.AllowedTools)
	agent, state, err := buildAgent(workspace, manifestAbs, agentName, spec, instrumented, telemetry, opts, env, allowedTools)
	if err != nil {
		return CaseReport{Name: c.Name, Model: modelName, Endpoint: endpoint, Workspace: workspace, ArtifactsDir: caseDir, Success: false, Error: err.Error()}
	}
	if reason, ok := shouldSkipCase(c.Requires, agent); ok {
		logger.Printf("case=%s model=%s skipped=true reason=%s", c.Name, modelName, reason)
		return CaseReport{
			Name:         c.Name,
			Model:        modelName,
			Endpoint:     endpoint,
			Workspace:    workspace,
			ArtifactsDir: caseDir,
			Skipped:      true,
			SkipReason:   reason,
			Success:      true,
		}
	}

	taskType := core.TaskType(c.TaskType)
	if taskType == "" {
		taskType = core.TaskTypeCodeGeneration
	}
	task := &core.Task{
		ID:          fmt.Sprintf("agenttest-%d", time.Now().UnixNano()),
		Instruction: c.Prompt,
		Type:        taskType,
		Context:     c.Context,
		Metadata:    c.Metadata,
	}
	state.Set("task.id", task.ID)
	state.Set("task.type", string(task.Type))
	state.Set("task.instruction", task.Instruction)

	var res *core.Result
	var execErr error
	attempts := 1
	if len(opts.OllamaResetOn) > 0 {
		attempts = 2
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		runCtx, cancel := context.WithTimeout(ctx, timeout)
		taskCtx := core.WithTaskContext(runCtx, core.TaskContext{
			ID:          task.ID,
			Type:        task.Type,
			Instruction: task.Instruction,
		})
		res, execErr = agent.Execute(taskCtx, task, state)
		cancel()
		if execErr == nil || attempt == attempts || !shouldResetOllama(execErr, opts.OllamaResetOn) {
			break
		}
		maybeResetOllama(logger, opts, modelName)
	}
	output := extractOutput(state, res)
	if data, err := json.MarshalIndent(state.Snapshot(), "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(caseDir, "context.snapshot.json"), data, 0o644)
	}
	events, _ := ReadTelemetryJSONL(telemetryPath)
	_, toolCounts := CountToolCalls(events)

	after, snapErr := SnapshotWorkspace(workspace, exclude)
	var changed []string
	if snapErr == nil {
		changed = DiffSnapshots(before, after)
	}
	if data, err := json.MarshalIndent(changed, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(caseDir, "changed_files.json"), data, 0o644)
	}

	success := execErr == nil && (res == nil || res.Success)
	caseErr := ""
	if execErr != nil {
		caseErr = execErr.Error()
	}
	if res != nil && !res.Success && caseErr == "" {
		caseErr = "agent returned unsuccessful result"
	}
	if assertErr := evaluateExpectations(c.Expect, output, changed, toolCounts); assertErr != nil {
		success = false
		if caseErr == "" {
			caseErr = assertErr.Error()
		} else {
			caseErr = caseErr + "; " + assertErr.Error()
		}
	}
	if c.Expect.MustSucceed && !success && caseErr == "" {
		caseErr = "case marked must_succeed but failed"
	}
	logger.Printf("case=%s model=%s success=%v err=%s", c.Name, modelName, success, caseErr)

	return CaseReport{
		Name:         c.Name,
		Model:        modelName,
		Endpoint:     endpoint,
		Workspace:    workspace,
		ArtifactsDir: caseDir,
		Skipped:      false,
		SkipReason:   "",
		Success:      success,
		Error:        caseErr,
		Output:       output,
		ChangedFiles: changed,
		ToolCalls:    toolCounts,
	}
}

func shouldSkipCase(req RequiresSpec, agent graph.Agent) (reason string, ok bool) {
	for _, bin := range req.Executables {
		bin = strings.TrimSpace(bin)
		if bin == "" {
			continue
		}
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Sprintf("missing executable %s", bin), true
		}
	}
	if len(req.Tools) > 0 {
		reg := extractToolRegistry(agent)
		if reg == nil {
			return "agent has no tool registry", true
		}
		for _, name := range req.Tools {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if _, ok := reg.Get(name); !ok {
				return fmt.Sprintf("missing tool %s", name), true
			}
		}
	}
	return "", false
}

func extractToolRegistry(agent graph.Agent) *toolsys.ToolRegistry {
	switch a := agent.(type) {
	case *agents.CodingAgent:
		return a.Tools
	case *agents.ExpertCoderAgent:
		return a.Tools
	case *agents.PlannerAgent:
		return a.Tools
	case *agents.ReActAgent:
		return a.Tools
	default:
		return nil
	}
}

func resolveAgainstWorkspace(workspace, resolvedBySuite, original string) string {
	// Many suites will use workspace-relative paths (e.g. relurpify_cfg/agent.manifest.yaml
	// or relurpify_cfg/agents/x.yaml) even though the suite file lives under
	// relurpify_cfg/testsuites/. If the
	// suite-relative resolution doesn't exist, fall back to workspace-relative.
	if resolvedBySuite != "" {
		if _, err := os.Stat(resolvedBySuite); err == nil {
			return resolvedBySuite
		}
	}
	if original == "" || filepath.IsAbs(original) {
		return resolvedBySuite
	}
	candidate := filepath.Clean(filepath.Join(workspace, original))
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return resolvedBySuite
}

func fallbackManifestPath(manifestPath, workspace string) string {
	if manifestPath != "" {
		if _, err := os.Stat(manifestPath); err == nil {
			return manifestPath
		}
	}
	if workspace == "" {
		return manifestPath
	}
	candidates := []string{
		filepath.Join(workspace, "relurpify_cfg", "agent.manifest.yaml"),
		filepath.Join(workspace, "relurpify_cfg", "testsuites", "agent.manifest.yaml"),
		filepath.Join(workspace, "relurpify_cfg", "testsuite", "agent.manifest.yaml"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return manifestPath
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func mergeStrings(a, b []string) []string {
	out := make([]string, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	return out
}

func effectiveAgentSpecForCase(base *core.AgentRuntimeSpec, c CaseSpec) *core.AgentRuntimeSpec {
	if base == nil {
		base = &core.AgentRuntimeSpec{}
	}
	clone := *base

	// Agenttest defaults: keep writes safe without relying on filesystem-permission
	// rewrites (tools declare broad perms for authorization).
	clone.Files.Write.Default = core.AgentPermissionDeny
	clone.Files.Edit.Default = core.AgentPermissionDeny
	clone.Files.Write.RequireApproval = false
	clone.Files.Edit.RequireApproval = false

	allow := []string{
		"relurpify_cfg/test_runs/**",
		"relurpify_cfg/memory/**",
		"testsuite/agenttest_fixtures/**",
	}
	for _, f := range c.Setup.Files {
		if strings.TrimSpace(f.Path) == "" {
			continue
		}
		allow = append(allow, filepath.ToSlash(filepath.Clean(f.Path)))
		allow = append(allow, filepath.ToSlash(filepath.Clean(f.Path))+".bak")
	}
	for _, pat := range c.Expect.FilesChanged {
		if strings.TrimSpace(pat) == "" {
			continue
		}
		allow = append(allow, filepath.ToSlash(filepath.Clean(pat)))
		allow = append(allow, filepath.ToSlash(filepath.Clean(pat))+".bak")
	}
	clone.Files.Write.AllowPatterns = uniqueStrings(append(clone.Files.Write.AllowPatterns, allow...))
	clone.Files.Edit.AllowPatterns = uniqueStrings(append(clone.Files.Edit.AllowPatterns, allow...))
	return &clone
}

func buildAgent(workspace, manifestPath, agentName string, agentSpec *core.AgentRuntimeSpec, model core.LanguageModel, telemetry core.Telemetry, opts RunOptions, extraEnv []string, allowedTools []string) (graph.Agent, *core.Context, error) {
	manifest, err := manifest.LoadAgentManifest(manifestPath)
	if err != nil {
		return nil, nil, err
	}

	audit := core.NewInMemoryAuditLogger(512)
	hitl := fruntime.NewHITLBroker(30 * time.Second)
	permMgr, err := fruntime.NewPermissionManager(workspace, &manifest.Spec.Permissions, audit, hitl)
	if err != nil {
		return nil, nil, err
	}

	var runner fruntime.CommandRunner
	if opts.Sandbox {
		reg, err := fruntime.RegisterAgent(context.Background(), fruntime.RuntimeConfig{
			ManifestPath: manifestPath,
			Sandbox:      appruntime.DefaultConfig().Sandbox,
			AuditLimit:   512,
			BaseFS:       workspace,
			HITLTimeout:  30 * time.Second,
		})
		if err != nil {
			return nil, nil, err
		}
		runner, err = fruntime.NewSandboxCommandRunner(reg.Manifest, reg.Runtime, workspace)
		if err != nil {
			return nil, nil, err
		}
		permMgr = reg.Permissions
	} else {
		runner = fruntime.NewLocalCommandRunner(workspace, extraEnv)
	}

	registry, err := appruntime.BuildToolRegistry(workspace, runner, appruntime.ToolRegistryOptions{
		AgentID:           manifest.Metadata.Name,
		PermissionManager: permMgr,
		AgentSpec:         agentSpec,
	})
	if err != nil {
		return nil, nil, err
	}
	applyAgentTestToolDefaults(registry, allowedTools)
	registry.UseTelemetry(telemetry)
	registry.UsePermissionManager(manifest.Metadata.Name, permMgr)
	registry.UseAgentSpec(manifest.Metadata.Name, agentSpec)

	memory, err := memory.NewHybridMemory(filepath.Join(workspace, "relurpify_cfg", "memory"))
	if err != nil {
		return nil, nil, err
	}

	agent := instantiateAgentByName(agentName, model, registry, memory)

	maxIterations := opts.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 8
	}
	cfg := &core.Config{
		Name:              manifest.Metadata.Name,
		Model:             firstNonEmpty(opts.ModelOverride, agentSpec.Model.Name),
		OllamaEndpoint:    firstNonEmpty(opts.EndpointOverride, "http://localhost:11434"),
		MaxIterations:     maxIterations,
		OllamaToolCalling: agentSpec.ToolCallingEnabled(),
		AgentSpec:         agentSpec,
		DebugLLM:          opts.DebugLLM,
		DebugAgent:        opts.DebugAgent,
		Telemetry:         telemetry,
	}
	if err := agent.Initialize(cfg); err != nil {
		return nil, nil, err
	}
	if reflection, ok := agent.(*agents.ReflectionAgent); ok && reflection.Delegate != nil {
		_ = reflection.Delegate.Initialize(cfg)
	}
	return agent, core.NewContext(), nil
}

func defaultAgenttestAllowedTools() []string {
	return []string{
		"file_read",
		"file_list",
		"file_search",
		"file_create",
		"file_delete",
		"file_write",
		"search_grep",
		"search_find_similar",
		"search_semantic",
		"git_diff",
		"git_history",
		"git_blame",
		"exec_run_tests",
		"exec_run_build",
		"exec_run_linter",
		"query_ast",
	}
}

func applyAgentTestToolDefaults(registry *toolsys.ToolRegistry, allowedTools []string) {
	if registry == nil {
		return
	}
	_ = registerToolAlias(registry, "read_file", "file_read")
	_ = registerToolAlias(registry, "write_file", "file_write")
	registry.RestrictTo(uniqueStrings(allowedTools))
}

func registerToolAlias(registry *toolsys.ToolRegistry, alias, target string) error {
	if registry == nil || alias == "" || target == "" {
		return nil
	}
	if _, ok := registry.Get(alias); ok {
		return nil
	}
	targetTool, ok := registry.Get(target)
	if !ok {
		return nil
	}
	return registry.Register(&aliasTool{
		alias:  alias,
		target: targetTool,
	})
}

type aliasTool struct {
	alias  string
	target core.Tool
}

func (t *aliasTool) Name() string        { return t.alias }
func (t *aliasTool) Description() string { return "Alias for " + t.target.Name() }
func (t *aliasTool) Category() string    { return t.target.Category() }
func (t *aliasTool) Parameters() []core.ToolParameter {
	return t.target.Parameters()
}
func (t *aliasTool) Execute(ctx context.Context, state *core.Context, args map[string]interface{}) (*core.ToolResult, error) {
	return t.target.Execute(ctx, state, args)
}
func (t *aliasTool) IsAvailable(ctx context.Context, state *core.Context) bool {
	return t.target.IsAvailable(ctx, state)
}
func (t *aliasTool) Permissions() core.ToolPermissions {
	return t.target.Permissions()
}

func instantiateAgentByName(name string, model core.LanguageModel, tools *toolsys.ToolRegistry, memory memory.MemoryStore) graph.Agent {
	switch strings.ToLower(name) {
	case "planner":
		return &agents.PlannerAgent{Model: model, Tools: tools, Memory: memory}
	case "react":
		return &agents.ReActAgent{Model: model, Tools: tools, Memory: memory}
	case "reflection":
		return &agents.ReflectionAgent{
			Reviewer: model,
			Delegate: &agents.CodingAgent{Model: model, Tools: tools, Memory: memory},
		}
	case "expert":
		return &agents.ExpertCoderAgent{Model: model, Tools: tools, Memory: memory}
	case "eternal":
		return &agents.EternalAgent{Model: model}
	default:
		return &agents.CodingAgent{Model: model, Tools: tools, Memory: memory}
	}
}

func applySetup(workspace string, setup SetupSpec, sandbox bool, logger *log.Logger) (cleanup func(), err error) {
	type original struct {
		path    string
		existed bool
		data    []byte
	}
	var originals []original
	for _, f := range setup.Files {
		if f.Path == "" {
			continue
		}
		target := filepath.Join(workspace, filepath.FromSlash(f.Path))
		if data, readErr := os.ReadFile(target); readErr == nil {
			originals = append(originals, original{path: target, existed: true, data: data})
		} else {
			originals = append(originals, original{path: target, existed: false})
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(target, []byte(f.Content), 0o644); err != nil {
			return nil, err
		}
	}
	cleanup = func() {
		for _, orig := range originals {
			if orig.existed {
				_ = os.WriteFile(orig.path, orig.data, 0o644)
			} else {
				_ = os.Remove(orig.path)
			}
		}
	}
	if setup.GitInit {
		_ = os.RemoveAll(filepath.Join(workspace, ".git"))
		if !sandbox {
			cmd := exec.Command("git", "init")
			cmd.Dir = workspace
			_ = cmd.Run()
		}
	}
	if logger != nil {
		logger.Printf("setup complete for %s", workspace)
	}
	return cleanup, nil
}

func extractOutput(state *core.Context, res *core.Result) string {
	if res != nil && res.Data != nil {
		if val, ok := res.Data["final_output"]; ok {
			if s, ok := val.(string); ok && s != "" {
				return s
			}
		}
		if text, ok := res.Data["text"].(string); ok && text != "" {
			return text
		}
	}
	history := state.History()
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "assistant" && strings.TrimSpace(history[i].Content) != "" {
			return history[i].Content
		}
	}
	if res != nil && res.Data != nil {
		if data, err := json.MarshalIndent(res.Data, "", "  "); err == nil {
			return string(data)
		}
	}
	return ""
}

func evaluateExpectations(expect ExpectSpec, output string, changed []string, toolCalls map[string]int) error {
	var failures []string

	if expect.NoFileChanges && len(changed) > 0 {
		failures = append(failures, fmt.Sprintf("expected no file changes, got %d", len(changed)))
	}
	if len(expect.FilesChanged) > 0 {
		for _, pat := range expect.FilesChanged {
			found := false
			for _, f := range changed {
				if matchGlob(pat, f) {
					found = true
					break
				}
			}
			if !found {
				failures = append(failures, fmt.Sprintf("expected file change matching %q", pat))
			}
		}
	}

	for _, s := range expect.OutputContains {
		if s == "" {
			continue
		}
		if !strings.Contains(output, s) {
			failures = append(failures, fmt.Sprintf("output missing substring %q", s))
		}
	}
	for _, rx := range expect.OutputRegex {
		if rx == "" {
			continue
		}
		re, err := regexp.Compile(rx)
		if err != nil {
			failures = append(failures, fmt.Sprintf("invalid output_regex %q", rx))
			continue
		}
		if !re.MatchString(output) {
			failures = append(failures, fmt.Sprintf("output does not match regex %q", rx))
		}
	}

	toolTotal := 0
	for _, n := range toolCalls {
		toolTotal += n
	}
	if expect.MaxToolCalls > 0 && toolTotal > expect.MaxToolCalls {
		failures = append(failures, fmt.Sprintf("expected <=%d tool calls, got %d", expect.MaxToolCalls, toolTotal))
	}
	for _, name := range expect.ToolCallsMustInclude {
		if toolCalls[name] == 0 {
			failures = append(failures, fmt.Sprintf("expected tool call %q", name))
		}
	}
	for _, name := range expect.ToolCallsMustExclude {
		if toolCalls[name] > 0 {
			failures = append(failures, fmt.Sprintf("unexpected tool call %q", name))
		}
	}

	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

func mapTargetPathToWorkspace(absPath, targetWorkspace, workspace string) string {
	rel, err := filepath.Rel(targetWorkspace, absPath)
	if err != nil {
		return absPath
	}
	if strings.HasPrefix(rel, "..") {
		return absPath
	}
	return filepath.Join(workspace, rel)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func sanitizeName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unnamed"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_")
}
