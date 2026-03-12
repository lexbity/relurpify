package agenttest

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/agents"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/telemetry"
	"github.com/lexcodex/relurpify/platform/llm"
)

func (r *Runner) runCase(ctx context.Context, suite *Suite, c CaseSpec, model ModelSpec, opts RunOptions, targetWorkspace, outDir string) CaseReport {
	layout := newRunCaseLayout(outDir, c.Name, model.Name)
	for _, dir := range []string{layout.ArtifactsDir, layout.TmpDir, filepath.Dir(layout.TelemetryPath), filepath.Dir(layout.LogPath)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return CaseReport{Name: c.Name, Model: model.Name, Endpoint: model.Endpoint, ArtifactsDir: layout.ArtifactsDir, Success: false, Error: err.Error()}
		}
	}

	logger := r.Logger
	if logger == nil {
		logFile, err := os.OpenFile(layout.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			defer logFile.Close()
			logger = log.New(logFile, "agenttest ", log.LstdFlags|log.Lmicroseconds)
		} else {
			logger = log.New(os.Stderr, "agenttest ", log.LstdFlags|log.Lmicroseconds)
		}
	}

	templateProfile := suite.Spec.Workspace.TemplateProfile
	exclude := append([]string{}, suite.Spec.Workspace.Exclude...)
	ignoreChanges := append([]string{}, suite.Spec.Workspace.IgnoreChanges...)
	templateFiles := append([]SetupFileSpec{}, suite.Spec.Workspace.Files...)
	if c.Overrides.Workspace != nil {
		if c.Overrides.Workspace.TemplateProfile != "" {
			templateProfile = c.Overrides.Workspace.TemplateProfile
		}
		if len(c.Overrides.Workspace.Exclude) > 0 {
			exclude = append([]string{}, c.Overrides.Workspace.Exclude...)
		}
		if len(c.Overrides.Workspace.IgnoreChanges) > 0 {
			ignoreChanges = append([]string{}, c.Overrides.Workspace.IgnoreChanges...)
		}
		if len(c.Overrides.Workspace.Files) > 0 {
			templateFiles = append(templateFiles, c.Overrides.Workspace.Files...)
		}
	}
	if templateProfile == "" {
		templateProfile = "default"
	}
	if len(exclude) == 0 {
		exclude = []string{
			".git/**",
			".gocache/**",
			".gomodcache/**",
			"relurpify_cfg/test_runs/**",
		}
	}
	ignoreChanges = uniqueStrings(append(ignoreChanges, defaultIgnoredGeneratedChanges()...))

	workspace := layout.WorkspaceDir
	if err := MaterializeDerivedWorkspace(targetWorkspace, workspace, templateProfile, suite.Spec.Manifest, exclude, templateFiles); err != nil {
		return CaseReport{Name: c.Name, Model: model.Name, Endpoint: model.Endpoint, Workspace: workspace, ArtifactsDir: layout.ArtifactsDir, Success: false, Error: err.Error()}
	}

	suiteManifestAbs := suite.ResolvePath(suite.Spec.Manifest)
	suiteManifestAbs = resolveAgainstWorkspace(targetWorkspace, suiteManifestAbs, suite.Spec.Manifest)
	manifestAbs := mapTargetPathToWorkspace(suiteManifestAbs, targetWorkspace, workspace)
	manifestAbs = fallbackManifestPath(manifestAbs, workspace)

	// Apply fixtures before taking the baseline snapshot so setup changes don't
	// count as agent-driven modifications.
	cleanup, err := applySetup(workspace, c.Setup, opts.Sandbox, logger)
	if err != nil {
		return CaseReport{Name: c.Name, Model: model.Name, Endpoint: model.Endpoint, Workspace: workspace, ArtifactsDir: layout.ArtifactsDir, Success: false, Error: err.Error()}
	}
	if cleanup != nil {
		defer cleanup()
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}

	agentName := suite.Spec.AgentName
	telemetrySink, err := telemetry.NewJSONFileTelemetry(layout.TelemetryPath)
	if err != nil {
		return CaseReport{Name: c.Name, Model: model.Name, Endpoint: model.Endpoint, Workspace: workspace, ArtifactsDir: layout.ArtifactsDir, Success: false, Error: err.Error()}
	}
	defer telemetrySink.Close()
	telemetryMux := telemetry.MultiplexTelemetry{Sinks: []core.Telemetry{telemetrySink}}

	memStore, err := prepareCaseMemory(workspace, suite, c, telemetryMux)
	if err != nil {
		return CaseReport{Name: c.Name, Model: model.Name, Endpoint: model.Endpoint, Workspace: workspace, ArtifactsDir: layout.ArtifactsDir, Success: false, Error: err.Error()}
	}
	if memStore != nil {
		defer memStore.Close()
	}
	if err := seedCaseState(ctx, workspace, memStore.Store, c.Setup); err != nil {
		return CaseReport{Name: c.Name, Model: model.Name, Endpoint: model.Endpoint, Workspace: workspace, ArtifactsDir: layout.ArtifactsDir, Success: false, Error: err.Error()}
	}
	before, err := SnapshotWorkspace(workspace, exclude)
	if err != nil {
		return CaseReport{Name: c.Name, Model: model.Name, Endpoint: model.Endpoint, Workspace: workspace, ArtifactsDir: layout.ArtifactsDir, Success: false, Error: err.Error()}
	}

	browserFixtures, err := startBrowserFixtureServer(suite, targetWorkspace, workspace, c)
	if err != nil {
		return CaseReport{Name: c.Name, Model: model.Name, Endpoint: model.Endpoint, Workspace: workspace, ArtifactsDir: layout.ArtifactsDir, Success: false, Error: err.Error()}
	}
	if browserFixtures != nil {
		defer browserFixtures.Close()
	}

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
			tapePath = layout.TapePath
		} else {
			resolved := suite.ResolvePath(tapePath)
			tapePath = resolveAgainstWorkspace(targetWorkspace, resolved, tapePath)
			tapePath = mapTargetPathToWorkspace(tapePath, targetWorkspace, workspace)
		}
		wrapped, err := llm.NewTapeModel(lm, tapePath, recording.Mode)
		if err == nil {
			lm = wrapped
		}
	}
	instrumented := llm.NewInstrumentedModel(lm, telemetryMux, opts.DebugLLM)

	spec := &core.AgentRuntimeSpec{}
	manifest, err := manifest.LoadAgentManifest(manifestAbs)
	if err == nil && manifest.Spec.Agent != nil {
		spec = agents.ApplyManifestDefaults(manifest.Spec.Agent, manifest.Spec.Defaults)
	}
	spec = effectiveAgentSpecForCase(spec, c)

	env := make([]string, 0, len(c.Overrides.ExtraEnv))
	for k, v := range c.Overrides.ExtraEnv {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(env)

	var allowedCapabilities []core.CapabilitySelector
	if c.Overrides.RestrictCapabilities && len(c.Overrides.AllowedCapabilities) > 0 {
		allowedCapabilities = uniqueCapabilitySelectors(c.Overrides.AllowedCapabilities)
	} else {
		allowedCapabilities = mergeCapabilitySelectors(defaultAgenttestAllowedCapabilities(), c.Overrides.AllowedCapabilities)
	}
	agent, state, err := buildAgent(workspace, manifestAbs, agentName, spec, instrumented, telemetryMux, opts, env, allowedCapabilities, c, memStore.Store)
	if err != nil {
		return CaseReport{Name: c.Name, Model: modelName, Endpoint: endpoint, Workspace: workspace, ArtifactsDir: layout.ArtifactsDir, Success: false, Error: err.Error()}
	}
	if reason, ok := shouldSkipCase(c.Requires, agent); ok {
		logger.Printf("case=%s model=%s skipped=true reason=%s", c.Name, modelName, reason)
		return CaseReport{
			Name:         c.Name,
			Model:        modelName,
			Endpoint:     endpoint,
			Workspace:    workspace,
			ArtifactsDir: layout.ArtifactsDir,
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
		Context:     cloneContextMap(c.Context),
		Metadata:    cloneStringMap(c.Metadata),
	}
	if task.Context == nil {
		task.Context = make(map[string]any)
	}
	task.Context["workspace"] = workspace
	if browserFixtures != nil {
		browserFixtures.InjectTask(task)
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
	snapshot := state.Snapshot()
	if data, err := json.MarshalIndent(snapshot, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "context.snapshot.json"), data, 0o644)
	}
	events, _ := ReadTelemetryJSONL(layout.TelemetryPath)
	_, toolCounts := CountToolCalls(events)

	after, snapErr := SnapshotWorkspace(workspace, exclude)
	var changed []string
	if snapErr == nil {
		changed = DiffSnapshots(before, after)
		changed = FilterChangedFiles(changed, ignoreChanges)
		changed = includeExpectedChangedFiles(changed, before, after, c.Expect.FilesChanged)
	}
	if data, err := json.MarshalIndent(changed, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "changed_files.json"), data, 0o644)
	}

	success := execErr == nil && (res == nil || res.Success)
	caseErr := ""
	if execErr != nil {
		caseErr = execErr.Error()
	}
	if res != nil && !res.Success && caseErr == "" {
		caseErr = "agent returned unsuccessful result"
	}
	if assertErr := evaluateExpectations(c.Expect, output, changed, toolCounts, snapshot); assertErr != nil {
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
		ArtifactsDir: layout.ArtifactsDir,
		Skipped:      false,
		SkipReason:   "",
		Success:      success,
		Error:        caseErr,
		Output:       output,
		ChangedFiles: changed,
		ToolCalls:    toolCounts,
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
