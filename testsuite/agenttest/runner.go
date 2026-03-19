package agenttest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/manifest"
)

type RunOptions struct {
	TargetWorkspace  string
	OutputDir        string
	Sandbox          bool
	Timeout          time.Duration
	BootstrapTimeout time.Duration
	SkipASTIndex     bool
	Profile          string
	Strict           bool
	MaxRetries       int

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
	SuitePath      string
	RunID          string
	Profile        string
	Strict         bool
	StartedAt      time.Time
	FinishedAt     time.Time
	DurationMS     int64
	PassedCases    int
	FailedCases    int
	SkippedCases   int
	InfraFailures  int
	AssertFailures int
	Cases          []CaseReport
	Performance    PerformanceSummary `json:"performance,omitempty"`
}

type CaseReport struct {
	Name          string
	Model         string
	ModelDigest   string
	ModelLoadedAs string
	ModelSource   string
	ManifestModel string
	Endpoint      string
	RecordingMode string
	TapePath      string
	Workspace     string
	ArtifactsDir  string
	StartedAt     time.Time
	FinishedAt    time.Time
	DurationMS    int64

	Skipped    bool
	SkipReason string

	Success             bool
	Error               string
	FailureKind         string
	Attempts            int
	RetryCount          int
	RetryTriggeredBy    []string
	Output              string
	ChangedFiles        []string
	ToolCalls           map[string]int
	TokenUsage          TokenUsageReport
	MemoryOutcome       MemoryOutcomeReport
	PhaseMetrics        []PhaseMetric        `json:"phase_metrics,omitempty"`
	BaselinePath        string               `json:"baseline_path,omitempty"`
	BaselineFound       bool                 `json:"baseline_found,omitempty"`
	PerformanceWarnings []PerformanceWarning `json:"performance_warnings,omitempty"`
}

type TokenUsageReport struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	LLMCalls         int
}

type MemoryOutcomeReport struct {
	DeclarativeRecordsCreated int
	ProceduralRecordsCreated  int
	MemoryRecordsCreated      int
	WorkflowRowsCreated       int
	WorkflowStateUpdated      bool
}

type Runner struct {
	Logger *log.Logger
}

type runCaseLayout struct {
	ArtifactsDir        string
	TmpDir              string
	TelemetryPath       string
	LogPath             string
	TapePath            string
	InteractionTapePath string
	WorkspaceDir        string
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
	workspacePaths := config.New(targetWorkspace)
	runID := time.Now().UTC().Format("20060102-150405.000")
	outDir := opts.OutputDir
	if outDir == "" {
		outDir = workspacePaths.TestRunDir(suite.Spec.AgentName, runID)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}

	report := &SuiteReport{
		SuitePath: suite.SourcePath,
		RunID:     runID,
		Profile:   suite.EffectiveProfile(opts.Profile),
		Strict:    suite.IsStrictRun(opts.Profile, opts.Strict),
		StartedAt: time.Now().UTC(),
	}

	models := suite.Spec.Models
	if len(models) == 0 {
		models = []ModelSpec{{Name: "", Endpoint: ""}}
	}
	if err := r.preflightSuite(ctx, suite, opts, targetWorkspace, models); err != nil {
		return nil, err
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
	report.DurationMS = report.FinishedAt.Sub(report.StartedAt).Milliseconds()
	for _, c := range report.Cases {
		switch {
		case c.Skipped:
			report.SkippedCases++
		case c.Success:
			report.PassedCases++
		default:
			report.FailedCases++
			if c.FailureKind == "infra" {
				report.InfraFailures++
			} else {
				report.AssertFailures++
			}
		}
	}
	report.Performance = SummarizePerformance(report.Cases)
	data, _ := json.MarshalIndent(report, "", "  ")
	_ = os.WriteFile(filepath.Join(outDir, "report.json"), data, 0o644)
	return report, nil
}

func (r *Runner) preflightSuite(ctx context.Context, suite *Suite, opts RunOptions, targetWorkspace string, models []ModelSpec) error {
	manifestModel := ""
	if suite != nil {
		suiteManifestAbs := suite.ResolvePath(suite.Spec.Manifest)
		suiteManifestAbs = resolveAgainstWorkspace(targetWorkspace, suiteManifestAbs, suite.Spec.Manifest)
		suiteManifestAbs = fallbackManifestPath(suiteManifestAbs, targetWorkspace)
		if loadedManifest, err := manifest.LoadAgentManifest(suiteManifestAbs); err == nil && loadedManifest.Spec.Agent != nil {
			manifestModel = loadedManifest.Spec.Agent.Model.Name
		}
	}
	checked := map[string]struct{}{}
	layout := newRunCaseLayout(filepath.Join(targetWorkspace, "relurpify_cfg", "test_runs_preflight"), "preflight", "preflight")
	for _, c := range suite.Spec.Cases {
		caseModels := models
		if c.Overrides.Model != nil {
			caseModels = []ModelSpec{*c.Overrides.Model}
		}
		for _, model := range caseModels {
			exec, err := resolveCaseExecution(suite, c, model, manifestModel, opts, layout, targetWorkspace, targetWorkspace)
			if err != nil {
				return err
			}
			if !shouldPreflightOllama(exec.RecordingMode) {
				continue
			}
			key := strings.TrimSpace(exec.Endpoint) + "|" + strings.TrimSpace(exec.Model)
			if _, ok := checked[key]; ok {
				continue
			}
			checked[key] = struct{}{}
			if err := preflightOllama(exec.Endpoint, exec.Model); err != nil {
				return fmt.Errorf("ollama preflight failed for suite %s case %s: %w", filepath.Base(suite.SourcePath), c.Name, err)
			}
		}
	}
	return nil
}

func newRunCaseLayout(outDir, caseName, modelName string) runCaseLayout {
	caseKey := sanitizeName(caseName) + "__" + sanitizeName(modelName)
	artifactsDir := filepath.Join(outDir, "artifacts", caseKey)
	tmpDir := filepath.Join(outDir, "tmp", caseKey)
	return runCaseLayout{
		ArtifactsDir:        artifactsDir,
		TmpDir:              tmpDir,
		TelemetryPath:       filepath.Join(outDir, "telemetry", caseKey+".jsonl"),
		LogPath:             filepath.Join(outDir, "logs", caseKey+".log"),
		TapePath:            filepath.Join(artifactsDir, "tape.jsonl"),
		InteractionTapePath: filepath.Join(artifactsDir, "interaction.tape.jsonl"),
		WorkspaceDir:        filepath.Join(tmpDir, "workspace"),
	}
}
