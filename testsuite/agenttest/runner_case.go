package agenttest

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	contractpkg "github.com/lexcodex/relurpify/framework/contract"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/perfstats"
	fsandbox "github.com/lexcodex/relurpify/framework/sandbox"
	"github.com/lexcodex/relurpify/framework/telemetry"
	"github.com/lexcodex/relurpify/platform/llm"
	ollama "github.com/lexcodex/relurpify/platform/llm/ollama"
)

func (r *Runner) runCase(ctx context.Context, suite *Suite, c CaseSpec, model ModelSpec, opts RunOptions, targetWorkspace, outDir string) CaseReport {
	caseStartedAt := time.Now().UTC()
	layout := newRunCaseLayout(outDir, c.Name, model.Name)

	logger := r.Logger

	templateProfile := resolveTemplateProfile(suite, c)
	exclude := resolveWorkspaceExclude(suite, c)
	ignoreChanges := append([]string{}, suite.Spec.Workspace.IgnoreChanges...)
	if c.Overrides.Workspace != nil {
		if len(c.Overrides.Workspace.IgnoreChanges) > 0 {
			ignoreChanges = append([]string{}, c.Overrides.Workspace.IgnoreChanges...)
		}
	}
	ignoreChanges = uniqueStrings(append(ignoreChanges, defaultIgnoredGeneratedChanges()...))
	workspace := layout.WorkspaceDir

	suiteManifestAbs := suite.ResolvePath(suite.Spec.Manifest)
	suiteManifestAbs = resolveAgainstWorkspace(targetWorkspace, suiteManifestAbs, suite.Spec.Manifest)
	manifestAbs := mapTargetPathToWorkspace(suiteManifestAbs, targetWorkspace, targetWorkspace)
	manifestAbs = fallbackManifestPath(manifestAbs, targetWorkspace)
	loadedManifest, err := manifest.LoadAgentManifest(manifestAbs)
	if err != nil {
		return failedCaseReport(caseStartedAt, c.Name, model.Name, "", "", model.Endpoint, "", "", layout.WorkspaceDir, layout.ArtifactsDir, err.Error(), "infra", 0)
	}

	timeout, err := resolveCaseTimeout(opts, suite, c)
	if err != nil {
		return failedCaseReport(caseStartedAt, c.Name, model.Name, "", manifestModelName(loadedManifest), model.Endpoint, "", "", layout.WorkspaceDir, layout.ArtifactsDir, err.Error(), "infra", 0)
	}

	agentName := suite.Spec.AgentName

	manifestModel := ""
	if loadedManifest.Spec.Agent != nil {
		manifestModel = loadedManifest.Spec.Agent.Model.Name
	}
	execution, err := resolveCaseExecution(suite, c, model, manifestModel, opts, layout, targetWorkspace, workspace)
	if err != nil {
		return failedCaseReport(caseStartedAt, c.Name, model.Name, "", manifestModel, model.Endpoint, "", "", layout.WorkspaceDir, layout.ArtifactsDir, err.Error(), "infra", 0)
	}
	resolvedLayout := newRunCaseLayout(outDir, c.Name, execution.Model)
	initialTapePath := execution.TapePath
	workspace = resolvedLayout.WorkspaceDir
	layout = resolvedLayout
	for _, dir := range []string{layout.ArtifactsDir, layout.TmpDir, filepath.Dir(layout.TelemetryPath), filepath.Dir(layout.LogPath)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return failedCaseReport(caseStartedAt, c.Name, execution.Model, execution.ModelSource, manifestModel, execution.Endpoint, execution.RecordingMode, execution.TapePath, workspace, layout.ArtifactsDir, err.Error(), "infra", 0)
		}
	}
	if err := MaterializeDerivedWorkspace(targetWorkspace, workspace, templateProfile, suite.Spec.Manifest, exclude, resolveWorkspaceFiles(suite, c)); err != nil {
		return failedCaseReport(caseStartedAt, c.Name, execution.Model, execution.ModelSource, manifestModel, execution.Endpoint, execution.RecordingMode, execution.TapePath, workspace, layout.ArtifactsDir, err.Error(), "infra", 0)
	}
	if execution.TapePath == initialTapePath {
		execution.TapePath = resolvedLayout.TapePath
	}
	if logger == nil {
		logFile, err := os.OpenFile(layout.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			defer logFile.Close()
			logger = log.New(logFile, "agenttest ", log.LstdFlags|log.Lmicroseconds)
		} else {
			logger = log.New(os.Stderr, "agenttest ", log.LstdFlags|log.Lmicroseconds)
		}
	}
	telemetrySink, err := telemetry.NewJSONFileTelemetry(layout.TelemetryPath)
	if err != nil {
		return failedCaseReport(caseStartedAt, c.Name, execution.Model, execution.ModelSource, execution.ManifestModel, execution.Endpoint, execution.RecordingMode, execution.TapePath, workspace, layout.ArtifactsDir, err.Error(), "infra", 0)
	}
	defer telemetrySink.Close()
	telemetryMux := telemetry.MultiplexTelemetry{Sinks: []core.Telemetry{telemetrySink}}
	modelProfileProvenance, resolvedProfile, err := resolveCaseModelProfile(targetWorkspace, execution)
	if err != nil {
		return failedCaseReport(caseStartedAt, c.Name, execution.Model, execution.ModelSource, execution.ManifestModel, execution.Endpoint, execution.RecordingMode, execution.TapePath, workspace, layout.ArtifactsDir, err.Error(), "infra", 0)
	}
	if modelProfileProvenance != nil {
		if data, marshalErr := json.MarshalIndent(modelProfileProvenance, "", "  "); marshalErr == nil {
			_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "model_profile.provenance.json"), data, 0o644)
		}
	}
	modelProvenance, err := resolveCaseModelProvenance(execution)
	if err != nil {
		return failedCaseReport(caseStartedAt, c.Name, execution.Model, execution.ModelSource, execution.ManifestModel, execution.Endpoint, execution.RecordingMode, execution.TapePath, workspace, layout.ArtifactsDir, err.Error(), "infra", 0)
	}
	if modelProvenance != nil {
		if data, marshalErr := json.MarshalIndent(modelProvenance, "", "  "); marshalErr == nil {
			_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "model.provenance.json"), data, 0o644)
		}
	}
	providerProvenance := providerProvenanceForExecution(execution)
	if providerProvenance != nil {
		if data, marshalErr := json.MarshalIndent(providerProvenance, "", "  "); marshalErr == nil {
			_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "provider.provenance.json"), data, 0o644)
		}
	}

	executionOpts := opts
	executionOpts.BackendReset = firstNonEmpty(execution.ProviderResetStrategy, executionOpts.BackendReset)
	executionOpts.BackendResetBetween = executionOpts.BackendResetBetween || execution.ProviderResetBetween
	if executionOpts.BackendResetBetween {
		maybeResetBackend(logger, executionOpts, execution.Model)
	}

	var client *ollama.Client
	if resolvedProfile != nil {
		client = ollama.NewClientWithProfile(execution.Endpoint, execution.Model, resolvedProfile.AsOllamaProfile())
	} else {
		client = ollama.NewClient(execution.Endpoint, execution.Model)
	}
	client.SetDebugLogging(executionOpts.DebugLLM)

	lm := core.LanguageModel(client)
	if execution.RecordingMode != "" && execution.RecordingMode != "off" {
		wrapped, err := llm.NewTapeModel(lm, execution.TapePath, execution.RecordingMode)
		if err != nil {
			return failedCaseReport(caseStartedAt, c.Name, execution.Model, execution.ModelSource, execution.ManifestModel, execution.Endpoint, execution.RecordingMode, execution.TapePath, workspace, layout.ArtifactsDir, err.Error(), "infra", 0)
		}
		if err := wrapped.ConfigureHeader(llm.TapeHeader{
			ProviderID:  "ollama",
			ModelName:   execution.Model,
			ModelDigest: modelProvenanceDigest(modelProvenance),
			SuiteName:   suite.Metadata.Name,
			CaseName:    c.Name,
		}); err != nil {
			_ = wrapped.Close()
			return failedCaseReport(caseStartedAt, c.Name, execution.Model, execution.ModelSource, execution.ManifestModel, execution.Endpoint, execution.RecordingMode, execution.TapePath, workspace, layout.ArtifactsDir, err.Error(), "infra", 0)
		}
		defer wrapped.Close()
		lm = wrapped
	}
	instrumented := llm.NewInstrumentedModel(lm, telemetryMux, executionOpts.DebugLLM)

	env := make([]string, 0, len(c.Overrides.ExtraEnv))
	for k, v := range c.Overrides.ExtraEnv {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(env)

	var allowedCapabilities []core.CapabilitySelector
	if c.Overrides.RestrictCapabilities && len(c.Overrides.AllowedCapabilities) > 0 {
		allowedCapabilities = uniqueCapabilitySelectors(c.Overrides.AllowedCapabilities)
	} else if shouldRestrictAllowedCapabilitiesForCase(c) && len(c.Overrides.AllowedCapabilities) > 0 {
		allowedCapabilities = uniqueCapabilitySelectors(c.Overrides.AllowedCapabilities)
	} else {
		allowedCapabilities = mergeCapabilitySelectors(defaultAgenttestAllowedCapabilities(), c.Overrides.AllowedCapabilities)
	}
	caseOpts := opts
	caseOpts.ModelOverride = execution.Model
	caseOpts.EndpointOverride = execution.Endpoint

	var res *core.Result
	var execErr error
	var agent graph.Agent
	var state *core.Context
	var task *core.Task
	var before *WorkspaceSnapshot
	var browserFixtures *browserFixtureServer
	var memStore *preparedMemoryStore
	var memoryBefore MemoryOutcomeReport
	var runner fsandbox.CommandRunner
	var cleanup func()
	skipReason := ""
	retryReasons := make([]string, 0, 1)
	maxRetries := resolveCaseMaxRetries(opts)
	attempts := 0
	for attempt := 1; attempt <= 1+maxRetries; attempt++ {
		if cleanup != nil {
			cleanup()
			cleanup = nil
		}
		if browserFixtures != nil {
			browserFixtures.Close()
			browserFixtures = nil
		}
		if memStore != nil {
			memStore.Close()
			memStore = nil
		}

		attemptEnv, attemptErr := prepareCaseAttempt(ctx, suite, c, caseOpts, workspace, targetWorkspace, manifestAbs, agentName, instrumented, telemetryMux, logger, loadedManifest, env, allowedCapabilities, exclude)
		if attemptErr != nil {
			return failedCaseReport(caseStartedAt, c.Name, execution.Model, execution.ModelSource, execution.ManifestModel, execution.Endpoint, execution.RecordingMode, execution.TapePath, workspace, layout.ArtifactsDir, attemptErr.Error(), "infra", attempt)
		}
		cleanup = attemptEnv.cleanup
		memStore = attemptEnv.memStore
		memoryBefore = attemptEnv.memoryBefore
		before = attemptEnv.before
		browserFixtures = attemptEnv.browserFixtures
		agent = attemptEnv.agent
		state = attemptEnv.state
		task = attemptEnv.task
		runner = attemptEnv.runner
		if reason := attemptEnv.skipReason; reason != "" {
			skipReason = reason
			break
		}

		attempts = attempt
		perfstats.Reset()
		runCtx, cancel := context.WithTimeout(ctx, timeout)
		taskCtx := core.WithTaskContext(runCtx, core.TaskContext{
			ID:          task.ID,
			Type:        task.Type,
			Instruction: task.Instruction,
		})
		// Phase 4: capability_direct_run bypasses full agent loop
		if c.CapabilityDirectRun != nil {
			res, execErr = executeCapabilityDirectRun(taskCtx, c, task, state, agent)
		} else {
			res, execErr = agent.Execute(taskCtx, task, state)
		}
		cancel()
		if !shouldRetryCaseWithBackendReset(execErr, opts.BackendResetOn) {
			break
		}
		if attempt > maxRetries {
			break
		}
		retryReasons = append(retryReasons, execErr.Error())
		maybeResetBackend(logger, opts, execution.Model)
	}
	defer func() {
		if cleanup != nil {
			cleanup()
		}
		if browserFixtures != nil {
			browserFixtures.Close()
		}
		if memStore != nil {
			memStore.Close()
		}
	}()
	if skipReason != "" {
		logger.Printf("case=%s model=%s skipped=true reason=%s", c.Name, execution.Model, skipReason)
		caseFinishedAt := time.Now().UTC()
		return CaseReport{
			Name:          c.Name,
			Model:         execution.Model,
			ModelSource:   execution.ModelSource,
			ManifestModel: execution.ManifestModel,
			Endpoint:      execution.Endpoint,
			RecordingMode: execution.RecordingMode,
			TapePath:      execution.TapePath,
			Workspace:     workspace,
			ArtifactsDir:  layout.ArtifactsDir,
			StartedAt:     caseStartedAt,
			FinishedAt:    caseFinishedAt,
			DurationMS:    caseFinishedAt.Sub(caseStartedAt).Milliseconds(),
			Skipped:       true,
			SkipReason:    skipReason,
			Success:       true,
			Attempts:      attempts,
		}
	}
	output := extractOutput(state, res)
	snapshot := state.Snapshot()
	if data, err := json.MarshalIndent(snapshot, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "context.snapshot.json"), data, 0o644)
	}
	if err := writeInteractionTape(layout.InteractionTapePath, snapshot); err != nil {
		logger.Printf("case=%s model=%s interaction_tape_error=%v", c.Name, execution.Model, err)
	}
	events, _ := ReadTelemetryJSONL(layout.TelemetryPath)
	_, toolCounts := CountToolCalls(events)
	transcript := BuildToolTranscript(events)
	if transcript != nil {
		if data, err := json.MarshalIndent(transcript, "", "  "); err == nil {
			_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "tool_transcript.json"), data, 0o644)
		}
	}
	tokenUsage := CountTokenUsage(events)
	if data, err := json.MarshalIndent(tokenUsage, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "token_usage.json"), data, 0o644)
	}
	frameworkPerf := perfstats.Get()
	if data, err := json.MarshalIndent(frameworkPerf, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "framework_perf.json"), data, 0o644)
	}
	phaseMetrics := BuildPhaseMetrics(snapshot, tokenUsage)
	if data, err := json.MarshalIndent(phaseMetrics, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "phase_metrics.json"), data, 0o644)
	}
	memoryAfter, memoryErr := collectMemoryOutcome(context.Background(), workspace, memStore.Store)
	memoryOutcome := diffMemoryOutcome(memoryBefore, memoryAfter)
	if memoryErr == nil {
		if data, err := json.MarshalIndent(memoryOutcome, "", "  "); err == nil {
			_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "memory_outcome.json"), data, 0o644)
		}
	}

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
	// Build capability coverage for tool validation
	coverage, _ := BuildCoverageFromEvents(agent, events)

	// NEW: Build latency report from transcript (Phase 5)
	toolTranscript := BuildToolTranscript(events)
	latencyReport := BuildLatencyReport(toolTranscript)
	if latencyReport != nil {
		if data, err := json.MarshalIndent(latencyReport, "", "  "); err == nil {
			_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "latency_report.json"), data, 0o644)
		}
	}
	if coverage != nil {
		if data, err := json.MarshalIndent(coverage, "", "  "); err == nil {
			_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "capability_coverage.json"), data, 0o644)
		}
	}

	// OSB Model: Unified assertion evaluation (Phases 2-5)
	var assertionResults []AssertionResult

	// Outcome block evaluation (Phase 2)
	if c.Expect.Outcome != nil {
		results, outcomeErr := evaluateOutcomeExpectations(*c.Expect.Outcome, workspace, output, changed, snapshot, tokenUsage, memoryOutcome, runner)
		assertionResults = append(assertionResults, results...)
		if outcomeErr != nil {
			success = false
			if caseErr == "" {
				caseErr = outcomeErr.Error()
			} else {
				caseErr = caseErr + "; " + outcomeErr.Error()
			}
		}
	}

	// Security block evaluation (Phase 3)
	var securityObservations []SecurityObservation
	if c.Expect.Security != nil {
		secResults, secObs, secErr := evaluateSecurityExpectations(*c.Expect.Security, loadedManifest, workspace, toolTranscript)
		securityObservations = secObs
		assertionResults = append(assertionResults, secResults...)
		if secErr != nil {
			success = false
			secErrMsg := "[security] " + secErr.Error()
			if caseErr == "" {
				caseErr = secErrMsg
			} else {
				caseErr = caseErr + "; " + secErrMsg
			}
		}
		// Write security_observations.json artifact
		if len(securityObservations) > 0 {
			if data, err := json.MarshalIndent(securityObservations, "", "  "); err == nil {
				_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "security_observations.json"), data, 0o644)
			}
		}
	}

	// Benchmark block evaluation (Phase 4) - never fails the test
	var benchmarkObservations []BenchmarkObservation
	if c.Expect.Benchmark != nil {
		benchmarkObservations = evaluateBenchmarkExpectations(*c.Expect.Benchmark, toolTranscript, events, snapshot, tokenUsage)
		// Write benchmark_observations.json artifact
		if len(benchmarkObservations) > 0 {
			if data, err := json.MarshalIndent(benchmarkObservations, "", "  "); err == nil {
				_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "benchmark_observations.json"), data, 0o644)
			}
		}
	}

	// Write assertion_results.json artifact
	if len(assertionResults) > 0 {
		if data, err := json.MarshalIndent(assertionResults, "", "  "); err == nil {
			_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "assertion_results.json"), data, 0o644)
		}
	}

	// Legacy MustSucceed check (backward compat)
	if c.Expect.MustSucceed && !success && caseErr == "" {
		caseErr = "case marked must_succeed but failed"
	}
	if c.Expect.Outcome != nil && c.Expect.Outcome.MustSucceed && !success && caseErr == "" {
		caseErr = "case marked must_succeed but failed"
	}
	failureKind := classifyCaseFailure(execErr, caseErr)
	caseFinishedAt := time.Now().UTC()
	logger.Printf("case=%s model=%s success=%v err=%s", c.Name, execution.Model, success, caseErr)

	baselinePath := BaselineFilePath(targetWorkspace, suite.Metadata.Name, c.Name, execution.Model)
	baselineFound := false
	var performanceWarnings []PerformanceWarning
	if shouldComparePerformanceBaseline(execution.RecordingMode) {
		if baseline, err := LoadPerformanceBaseline(baselinePath); err == nil && baseline != nil {
			baselineFound = true
			performanceWarnings = ComparePerformanceBaseline(CaseReport{
				Name:          c.Name,
				Model:         execution.Model,
				DurationMS:    caseFinishedAt.Sub(caseStartedAt).Milliseconds(),
				TokenUsage:    tokenUsage,
				FrameworkPerf: frameworkPerf,
				PhaseMetrics:  phaseMetrics,
			}, baseline)
			if len(performanceWarnings) > 0 {
				if data, err := json.MarshalIndent(performanceWarnings, "", "  "); err == nil {
					_ = os.WriteFile(filepath.Join(layout.ArtifactsDir, "performance_warnings.json"), data, 0o644)
				}
			}
		}
	}

	return CaseReport{
		Name:                 c.Name,
		Model:                execution.Model,
		ModelDigest:          modelProvenanceDigest(modelProvenance),
		ModelLoadedAs:        modelProvenanceName(modelProvenance),
		ModelSource:          execution.ModelSource,
		Provider:             execution.Provider,
		ManifestModel:        execution.ManifestModel,
		Endpoint:             execution.Endpoint,
		RecordingMode:        execution.RecordingMode,
		TapePath:             execution.TapePath,
		Workspace:            workspace,
		ArtifactsDir:         layout.ArtifactsDir,
		StartedAt:            caseStartedAt,
		FinishedAt:           caseFinishedAt,
		DurationMS:           caseFinishedAt.Sub(caseStartedAt).Milliseconds(),
		Skipped:              false,
		SkipReason:           "",
		Success:              success,
		Error:                caseErr,
		FailureKind:          failureKind,
		Attempts:             attempts,
		RetryCount:           len(retryReasons),
		RetryTriggeredBy:     retryReasons,
		Output:               output,
		ChangedFiles:         changed,
		ToolCalls:            toolCounts,
		TokenUsage:           tokenUsage,
		MemoryOutcome:        memoryOutcome,
		FrameworkPerf:        frameworkPerf,
		PhaseMetrics:         phaseMetrics,
		BaselinePath:         baselinePath,
		BaselineFound:        baselineFound,
		PerformanceWarnings:  performanceWarnings,
		BackendResetStrategy: executionOpts.BackendReset,
		// NEW: Latency tracking (Phase 5)
		ToolLatencies:   getLatencyMapOrEmpty(latencyReport),
		TotalToolTimeMs: getTotalToolTimeOrZero(latencyReport),
	}
}

func manifestModelName(m *manifest.AgentManifest) string {
	if m == nil || m.Spec.Agent == nil {
		return ""
	}
	return m.Spec.Agent.Model.Name
}

func failedCaseReport(startedAt time.Time, name, model, modelSource, manifestModel, endpoint, recordingMode, tapePath, workspace, artifactsDir, errMsg, failureKind string, attempts int) CaseReport {
	finishedAt := time.Now().UTC()
	return CaseReport{
		Name:          name,
		Model:         model,
		ModelSource:   modelSource,
		ManifestModel: manifestModel,
		Endpoint:      endpoint,
		RecordingMode: recordingMode,
		TapePath:      tapePath,
		Workspace:     workspace,
		ArtifactsDir:  artifactsDir,
		StartedAt:     startedAt,
		FinishedAt:    finishedAt,
		DurationMS:    finishedAt.Sub(startedAt).Milliseconds(),
		Success:       false,
		Error:         errMsg,
		FailureKind:   failureKind,
		Attempts:      attempts,
	}
}

// getLatencyMapOrEmpty returns the ToolLatencies map or an empty map if nil
func getLatencyMapOrEmpty(report *ToolLatencyReport) map[string]LatencyStats {
	if report == nil {
		return make(map[string]LatencyStats)
	}
	return report.ToolLatencies
}

// getTotalToolTimeOrZero returns the TotalToolTimeMs or 0 if nil
func getTotalToolTimeOrZero(report *ToolLatencyReport) int64 {
	if report == nil {
		return 0
	}
	return report.TotalToolTimeMs
}

func resolveCaseMaxRetries(opts RunOptions) int {
	switch {
	case opts.MaxRetries == 0:
		return 3
	case opts.MaxRetries < 0:
		return 0
	default:
		return opts.MaxRetries
	}
}

func writeInteractionTape(path string, snapshot *core.ContextSnapshot) error {
	if snapshot == nil {
		return nil
	}
	raw, ok := snapshot.State["euclo.interaction_records"]
	if !ok || raw == nil {
		return nil
	}
	lines, err := marshalInteractionRecords(raw)
	if err != nil {
		return err
	}
	if len(lines) == 0 {
		return nil
	}
	return os.WriteFile(path, lines, 0o644)
}

func marshalInteractionRecords(raw any) ([]byte, error) {
	records, ok := raw.([]any)
	if !ok {
		if typed, ok := raw.([]map[string]any); ok {
			records = make([]any, 0, len(typed))
			for _, item := range typed {
				records = append(records, item)
			}
		} else {
			return nil, nil
		}
	}
	var out []byte
	for _, record := range records {
		line, err := json.Marshal(record)
		if err != nil {
			return nil, err
		}
		out = append(out, line...)
		out = append(out, '\n')
	}
	return out, nil
}

type preparedCaseAttempt struct {
	cleanup         func()
	memStore        *preparedMemoryStore
	memoryBefore    MemoryOutcomeReport
	before          *WorkspaceSnapshot
	browserFixtures *browserFixtureServer
	agent           graph.Agent
	state           *core.Context
	task            *core.Task
	skipReason      string
	runner          fsandbox.CommandRunner
}

func prepareCaseAttempt(ctx context.Context, suite *Suite, c CaseSpec, opts RunOptions, workspace, targetWorkspace, manifestAbs, agentName string, model core.LanguageModel, telemetry core.Telemetry, logger *log.Logger, loadedManifest *manifest.AgentManifest, extraEnv []string, allowedCapabilities []core.CapabilitySelector, exclude []string) (*preparedCaseAttempt, error) {
	if err := MaterializeDerivedWorkspace(targetWorkspace, workspace, resolveTemplateProfile(suite, c), suite.Spec.Manifest, resolveWorkspaceExclude(suite, c), resolveWorkspaceFiles(suite, c)); err != nil {
		return nil, err
	}

	cleanup, err := applySetup(workspace, c.Setup, opts.Sandbox, logger)
	if err != nil {
		return nil, err
	}
	attempt := &preparedCaseAttempt{cleanup: cleanup}
	defer func() {
		if err != nil && attempt.cleanup != nil {
			attempt.cleanup()
		}
		if err != nil && attempt.browserFixtures != nil {
			attempt.browserFixtures.Close()
		}
		if err != nil && attempt.memStore != nil {
			attempt.memStore.Close()
		}
	}()

	memStore, err := prepareCaseMemory(workspace, suite, c, telemetry)
	if err != nil {
		return nil, err
	}
	attempt.memStore = memStore
	if err := seedCaseState(ctx, workspace, memStore.Store, c.Setup); err != nil {
		return nil, err
	}
	if attempt.memoryBefore, err = collectMemoryOutcome(ctx, workspace, memStore.Store); err != nil {
		return nil, err
	}

	before, err := SnapshotWorkspace(workspace, exclude)
	if err != nil {
		return nil, err
	}
	attempt.before = before

	browserFixtures, err := startBrowserFixtureServer(suite, targetWorkspace, workspace, c)
	if err != nil {
		return nil, err
	}
	attempt.browserFixtures = browserFixtures

	baseSpec := contractpkg.ApplyManifestDefaults(loadedManifest.Spec.Agent, loadedManifest.Spec.Defaults)
	if baseSpec == nil {
		baseSpec = &core.AgentRuntimeSpec{}
	}
	agentSpec := effectiveAgentSpecForCase(baseSpec, c)

	bootstrapCtx, cancelBootstrap := context.WithTimeout(ctx, resolveBootstrapTimeout(opts, c))
	agent, state, runner, err := buildAgent(bootstrapCtx, workspace, manifestAbs, agentName, agentSpec, model, telemetry, opts, extraEnv, allowedCapabilities, c, memStore.Store)
	cancelBootstrap()
	if err != nil {
		return nil, err
	}
	// Phase 4: Inject setup.state_keys into context before agent execution
	for key, value := range c.Setup.StateKeys {
		state.Set(key, value)
	}
	attempt.agent = agent
	attempt.state = state
	attempt.runner = runner
	if reason, ok := shouldSkipCase(c.Requires, agent); ok {
		attempt.skipReason = reason
		return attempt, nil
	}

	taskType := core.TaskType(c.TaskType)
	if taskType == "" {
		taskType = core.TaskTypeCodeGeneration
	}
	taskID := fmt.Sprintf("agenttest-%d", time.Now().UnixNano())
	if override := strings.TrimSpace(c.Metadata["task_id"]); override != "" {
		taskID = override
	}
	task := &core.Task{
		ID:          taskID,
		Instruction: c.Prompt,
		Type:        taskType,
		Context:     cloneContextMap(c.Context),
		Metadata:    cloneStringMap(c.Metadata),
	}
	if task.Context == nil {
		task.Context = make(map[string]any)
	}
	if len(c.InteractionScript) > 0 {
		task.Context["euclo.interaction_script"] = interactionScriptContext(c.InteractionScript)
	}
	task.Context["workspace"] = workspace
	seedWorkflowRetrievalStateForCase(state, task, c)
	if browserFixtures != nil {
		browserFixtures.InjectTask(task)
	}
	state.Set("task.id", task.ID)
	state.Set("task.type", string(task.Type))
	state.Set("task.instruction", task.Instruction)
	attempt.task = task

	return attempt, nil
}

func interactionScriptContext(script []InteractionScriptStep) []map[string]any {
	steps := make([]map[string]any, 0, len(script))
	for _, step := range script {
		entry := map[string]any{
			"action": step.Action,
		}
		if step.Phase != "" {
			entry["phase"] = step.Phase
		}
		if step.Kind != "" {
			entry["kind"] = step.Kind
		}
		if step.Text != "" {
			entry["text"] = step.Text
		}
		steps = append(steps, entry)
	}
	return steps
}

func seedWorkflowRetrievalStateForCase(state *core.Context, task *core.Task, c CaseSpec) {
	if state == nil || task == nil || task.Context == nil {
		return
	}
	workflowID, ok := task.Context["workflow_id"]
	if !ok || strings.TrimSpace(fmt.Sprint(workflowID)) == "" {
		return
	}
	var summary string
	var seededPlan map[string]any
	for _, workflow := range c.Setup.Workflows {
		if workflow.Workflow.WorkflowID != fmt.Sprint(workflowID) {
			continue
		}
		for _, record := range workflow.Knowledge {
			if text := strings.TrimSpace(record.Content); text != "" {
				if summary != "" {
					summary += "\n"
				}
				summary += text
			}
			if seededPlan == nil {
				seededPlan = seededWorkflowPlan(record)
			}
		}
		break
	}
	if summary == "" {
		return
	}
	payload := map[string]any{
		"query":   task.Instruction,
		"summary": summary,
		"scope":   fmt.Sprintf("workflow:%s", fmt.Sprint(workflowID)),
	}
	task.Context["workflow_retrieval"] = payload
	mode := strings.ToLower(strings.TrimSpace(fmt.Sprint(task.Context["mode"])))
	switch mode {
	case "architect":
		state.Set("planner.workflow_retrieval", payload)
	default:
		state.Set("pipeline.workflow_retrieval", payload)
	}
	if seededPlan != nil {
		state.Set("pipeline.plan", seededPlan)
		state.Set("euclo.seeded_pipeline_plan", seededPlan)
		explorationID := fmt.Sprintf("%s:seeded-exploration", strings.TrimSpace(fmt.Sprint(workflowID)))
		state.Set("euclo.active_exploration_id", explorationID)
		state.Set("euclo.active_exploration_snapshot_id", explorationID+":snapshot")
	}
}

func seededWorkflowPlan(record WorkflowKnowledgeSeedSpec) map[string]any {
	title := strings.TrimSpace(record.Title)
	content := strings.TrimSpace(record.Content)
	lowerTitle := strings.ToLower(title)
	lowerContent := strings.ToLower(content)
	if !strings.Contains(lowerTitle, "compiled plan") && !strings.HasPrefix(lowerContent, "plan:") {
		return nil
	}
	step := map[string]any{
		"id":          "seeded-plan-step-1",
		"title":       firstNonEmpty(title, "Compiled plan"),
		"description": content,
	}
	if scope := seededWorkflowPlanScope(content); len(scope) > 0 {
		step["scope"] = scope
	}
	return map[string]any{
		"source":  "agenttest.workflow_knowledge",
		"summary": content,
		"steps":   []map[string]any{step},
	}
}

func seededWorkflowPlanScope(content string) []string {
	re := regexp.MustCompile(`[\w./-]+\.(?:go|md|yaml|yml|json|toml|txt)`)
	matches := re.FindAllString(content, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if _, ok := seen[match]; ok {
			continue
		}
		seen[match] = struct{}{}
		out = append(out, match)
	}
	return out
}

func shouldRestrictAllowedCapabilitiesForCase(c CaseSpec) bool {
	mode := ""
	if c.Context != nil {
		if raw, ok := c.Context["mode"]; ok {
			mode = strings.ToLower(strings.TrimSpace(fmt.Sprint(raw)))
		}
	}
	switch mode {
	case "ask", "debug", "architect":
		return true
	}
	return core.TaskType(c.TaskType) == core.TaskTypeAnalysis
}

func resolveTemplateProfile(suite *Suite, c CaseSpec) string {
	templateProfile := suite.Spec.Workspace.TemplateProfile
	if c.Overrides.Workspace != nil && c.Overrides.Workspace.TemplateProfile != "" {
		templateProfile = c.Overrides.Workspace.TemplateProfile
	}
	if templateProfile == "" {
		return "default"
	}
	return templateProfile
}

func resolveWorkspaceExclude(suite *Suite, c CaseSpec) []string {
	exclude := append([]string{}, suite.Spec.Workspace.Exclude...)
	if c.Overrides.Workspace != nil && len(c.Overrides.Workspace.Exclude) > 0 {
		exclude = append([]string{}, c.Overrides.Workspace.Exclude...)
	}
	if len(exclude) == 0 {
		return []string{
			".git/**",
			".gocache/**",
			".gomodcache/**",
			"relurpify_cfg/test_runs/**",
		}
	}
	return exclude
}

func resolveWorkspaceFiles(suite *Suite, c CaseSpec) []SetupFileSpec {
	files := append([]SetupFileSpec{}, suite.Spec.Workspace.Files...)
	if c.Overrides.Workspace != nil && len(c.Overrides.Workspace.Files) > 0 {
		files = append(files, c.Overrides.Workspace.Files...)
	}
	return files
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
		target, err := resolvePathWithin(workspace, f.Path)
		if err != nil {
			return nil, err
		}
		mode, err := parseSetupFileMode(f.Mode)
		if err != nil {
			return nil, err
		}
		if data, readErr := os.ReadFile(target); readErr == nil {
			originals = append(originals, original{path: target, existed: true, data: data})
		} else {
			originals = append(originals, original{path: target, existed: false})
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(target, []byte(f.Content), mode); err != nil {
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
		gitDir, err := resolvePathWithin(workspace, ".git")
		if err != nil {
			return nil, err
		}
		_ = os.RemoveAll(gitDir)
		if !sandbox {
			for _, args := range [][]string{
				{"init"},
				{"config", "user.name", "agenttest"},
				{"config", "user.email", "agenttest@example.invalid"},
				{"add", "."},
				{"commit", "-m", "agenttest baseline"},
			} {
				cmd := exec.Command("git", args...)
				cmd.Dir = workspace
				_ = cmd.Run()
			}
		}
	}
	if logger != nil {
		logger.Printf("setup complete for %s", workspace)
	}
	return cleanup, nil
}

func extractOutput(state *core.Context, res *core.Result) string {
	if res != nil && res.Data != nil {
		if text := finalOutputText(res.Data["final_output"]); text != "" {
			return text
		}
		if text, ok := res.Data["text"].(string); ok && text != "" {
			return text
		}
	}
	if state != nil {
		for _, key := range []string{
			"react.final_output",
			"pipeline.final_output",
			"rewoo.synthesis",
			"architect.summary",
			"planner.summary",
		} {
			if val, ok := state.Get(key); ok {
				if text := finalOutputText(val); text != "" {
					return text
				}
			}
		}
	}
	if res != nil && res.Data != nil {
		for _, key := range []string{"summary", "output"} {
			if text, ok := res.Data[key].(string); ok && strings.TrimSpace(text) != "" {
				return text
			}
		}
	}
	if state != nil {
		if text := singleAssistantMessage(state.History()); text != "" {
			return text
		}
	}
	if res != nil && res.Data != nil {
		if data, err := json.MarshalIndent(res.Data, "", "  "); err == nil {
			return string(data)
		}
	}
	return ""
}

func resolveCaseModelProvenance(execution resolvedCaseExecution) (*BackendModelProvenance, error) {
	if !shouldPreflightBackend(execution.RecordingMode) {
		return nil, nil
	}
	return lookupBackendModelProvenance(execution.Endpoint, execution.Model)
}

func modelProvenanceDigest(provenance *BackendModelProvenance) string {
	if provenance == nil {
		return ""
	}
	return provenance.Digest
}

func modelProvenanceName(provenance *BackendModelProvenance) string {
	if provenance == nil {
		return ""
	}
	return firstNonEmpty(provenance.LoadedName, provenance.LoadedModel)
}

type BackendProviderProvenance struct {
	Provider      string `json:"provider,omitempty"`
	Endpoint      string `json:"endpoint,omitempty"`
	ResetStrategy string `json:"reset_strategy,omitempty"`
	ResetBetween  bool   `json:"reset_between,omitempty"`
}

func providerProvenanceForExecution(execution resolvedCaseExecution) *BackendProviderProvenance {
	if strings.TrimSpace(execution.Provider) == "" && strings.TrimSpace(execution.Endpoint) == "" {
		return nil
	}
	return &BackendProviderProvenance{
		Provider:      execution.Provider,
		Endpoint:      execution.Endpoint,
		ResetStrategy: execution.ProviderResetStrategy,
		ResetBetween:  execution.ProviderResetBetween,
	}
}

func finalOutputText(val any) string {
	switch typed := val.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		for _, key := range []string{"summary", "text", "output"} {
			if text, ok := typed[key].(string); ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}

func singleAssistantMessage(history []core.Interaction) string {
	if len(history) == 0 {
		return ""
	}
	found := ""
	for _, item := range history {
		if item.Role != "assistant" {
			continue
		}
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		if found != "" {
			return ""
		}
		found = content
	}
	return found
}

func shouldRetryCaseWithBackendReset(err error, patterns []string) bool {
	if err == nil {
		return false
	}
	if !isInfrastructureError(err.Error()) {
		return false
	}
	return shouldResetBackend(err, patterns)
}

// capabilityDirectRunner is the narrow interface the test harness requires to
// invoke a capability directly, bypassing the full agent execution loop.
type capabilityDirectRunner interface {
	DirectCapabilityRun(ctx context.Context, capabilityID, invokingPrimary string, task *core.Task, state *core.Context) (*core.Result, error)
}

// executeCapabilityDirectRun runs a capability directly through the agent's
// dispatcher, bypassing the full agent loop. Used for testing supporting-only
// capabilities in isolation.
func executeCapabilityDirectRun(ctx context.Context, c CaseSpec, task *core.Task, state *core.Context, agent graph.Agent) (*core.Result, error) {
	runner, ok := agent.(capabilityDirectRunner)
	if !ok {
		return nil, fmt.Errorf("agent does not support capability_direct_run (missing DirectCapabilityRun method)")
	}
	spec := c.CapabilityDirectRun
	return runner.DirectCapabilityRun(ctx, spec.CapabilityID, spec.InvokingPrimary, task, state)
}
