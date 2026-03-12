package agenttest

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/lexcodex/relurpify/framework/config"
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

type runCaseLayout struct {
	ArtifactsDir  string
	TmpDir        string
	TelemetryPath string
	LogPath       string
	TapePath      string
	WorkspaceDir  string
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
		StartedAt: time.Now().UTC(),
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

func newRunCaseLayout(outDir, caseName, modelName string) runCaseLayout {
	caseKey := sanitizeName(caseName) + "__" + sanitizeName(modelName)
	artifactsDir := filepath.Join(outDir, "artifacts", caseKey)
	tmpDir := filepath.Join(outDir, "tmp", caseKey)
	return runCaseLayout{
		ArtifactsDir:  artifactsDir,
		TmpDir:        tmpDir,
		TelemetryPath: filepath.Join(outDir, "telemetry", caseKey+".jsonl"),
		LogPath:       filepath.Join(outDir, "logs", caseKey+".log"),
		TapePath:      filepath.Join(artifactsDir, "tape.jsonl"),
		WorkspaceDir:  filepath.Join(tmpDir, "workspace"),
	}
}
