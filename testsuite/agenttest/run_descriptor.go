package agenttest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

const (
	PreparedRunSelectionSingle = "single-backend"
	PreparedRunSelectionMatrix = "matrix"
)

// PreparedRunDescriptor is the persisted handoff between agenttest setup and
// the CLI execution surface.
type PreparedRunDescriptor struct {
	RunID                 string                       `json:"run_id"`
	SuitePath             string                       `json:"suite_path"`
	SuiteName             string                       `json:"suite_name"`
	CaseName              string                       `json:"case_name"`
	AgentName             string                       `json:"agent_name"`
	Instruction           string                       `json:"instruction,omitempty"`
	WorkspaceRoot         string                       `json:"workspace_root"`
	RunRoot               string                       `json:"run_root"`
	DerivedWorkspaceRoot  string                       `json:"derived_workspace_root"`
	ConfigPath            string                       `json:"config_path"`
	AgentsDir             string                       `json:"agents_dir"`
	LogsDir               string                       `json:"logs_dir"`
	TelemetryDir          string                       `json:"telemetry_dir"`
	SetupDir              string                       `json:"setup_dir"`
	ExecutionDir          string                       `json:"execution_dir"`
	SetupLogsDir          string                       `json:"setup_logs_dir,omitempty"`
	SetupTelemetryDir     string                       `json:"setup_telemetry_dir,omitempty"`
	SetupArtifactsDir     string                       `json:"setup_artifacts_dir,omitempty"`
	ExecutionLogsDir      string                       `json:"execution_logs_dir,omitempty"`
	ExecutionTelemetryDir string                       `json:"execution_telemetry_dir,omitempty"`
	ExecutionArtifactsDir string                       `json:"execution_artifacts_dir,omitempty"`
	VerificationDir       string                       `json:"verification_dir,omitempty"`
	ServiceResetStrategy  string                       `json:"service_reset_strategy,omitempty"`
	ServiceResetBetween   bool                         `json:"service_reset_between,omitempty"`
	BackendSelection      string                       `json:"backend_selection"`
	BackendProvider       string                       `json:"backend_provider,omitempty"`
	BackendFamily         string                       `json:"backend_family,omitempty"`
	BackendEndpoint       string                       `json:"backend_endpoint,omitempty"`
	BackendBinary         string                       `json:"backend_binary,omitempty"`
	BackendService        string                       `json:"backend_service,omitempty"`
	BackendResetStrategy  string                       `json:"backend_reset_strategy,omitempty"`
	BackendMatrix         []PreparedBackendTarget      `json:"backend_matrix,omitempty"`
	ModelName             string                       `json:"model_name,omitempty"`
	SkipASTIndex          bool                         `json:"skip_ast_index"`
	StrictMode            bool                         `json:"strict_mode"`
	MaxIterations         int                          `json:"max_iterations"`
	MaxRetries            int                          `json:"max_retries"`
	SandboxBackend        string                       `json:"sandbox_backend,omitempty"`
	SetupOverlays         []PreparedRunOverlay         `json:"setup_overlays,omitempty"`
	SeededState           map[string]any               `json:"seeded_state,omitempty"`
	Verification          PreparedVerificationContract `json:"verification"`
	ExpectedArtifacts     []string                     `json:"expected_artifacts,omitempty"`
}

type PreparedBackendTarget struct {
	Provider      string `json:"backend_provider"`
	Family        string `json:"backend_family"`
	Endpoint      string `json:"backend_endpoint"`
	Binary        string `json:"backend_binary,omitempty"`
	Service       string `json:"backend_service,omitempty"`
	ResetStrategy string `json:"backend_reset_strategy,omitempty"`
}

type PreparedRunOverlay struct {
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
	Mode    string `json:"mode,omitempty"`
}

type PreparedVerificationContract struct {
	Steps             []PreparedVerificationStep `json:"verification_steps,omitempty"`
	Script            string                     `json:"verification_script,omitempty"`
	ExpectedArtifacts []string                   `json:"expected_artifacts,omitempty"`
}

type PreparedVerificationStep struct {
	Tool              string         `json:"tool"`
	Args              map[string]any `json:"args,omitempty"`
	ContinueOnFailure bool           `json:"continue_on_failure,omitempty"`
}

func BuildPreparedRunDescriptor(suite *Suite, c CaseSpec, model ModelSpec, opts RunOptions, targetWorkspace, runRoot, runID string) (*PreparedRunDescriptor, error) {
	if suite == nil {
		return nil, fmt.Errorf("suite required")
	}
	if strings.TrimSpace(c.Name) == "" {
		return nil, fmt.Errorf("case name required")
	}
	if strings.TrimSpace(runID) == "" {
		return nil, fmt.Errorf("run id required")
	}
	targetWorkspace = cleanAbsolutePath(targetWorkspace)
	if targetWorkspace == "" {
		return nil, fmt.Errorf("workspace root required")
	}
	runRoot = cleanAbsolutePath(runRoot)
	if runRoot == "" {
		return nil, fmt.Errorf("run root required")
	}
	artifacts := NewPreparedRunArtifacts(targetWorkspace, runRoot, suite.Spec.AgentName, runID)
	if err := artifacts.Ensure(); err != nil {
		return nil, err
	}

	suitePath := cleanAbsolutePath(suite.SourcePath)
	if suitePath == "" {
		suitePath = suite.SourcePath
	}
	backendTargets := buildPreparedBackendTargets(suite, c, model, opts)
	backendSelection := PreparedRunSelectionSingle
	if len(backendTargets) > 1 {
		backendSelection = PreparedRunSelectionMatrix
	}
	selected := backendTargets[0]

	verification := buildPreparedVerificationContract(c)
	setupOverlays := make([]PreparedRunOverlay, 0, len(c.Setup.Files))
	for _, overlay := range c.Setup.Files {
		if strings.TrimSpace(overlay.Path) == "" {
			continue
		}
		setupOverlays = append(setupOverlays, PreparedRunOverlay{
			Path:    filepath.Clean(overlay.Path),
			Content: overlay.Content,
			Mode:    overlay.Mode,
		})
	}

	seededState := cloneContextMap(c.Setup.StateKeys)
	expectedArtifacts := UniqueStrings(append(append([]string{}, verification.ExpectedArtifacts...), expectedArtifactsForCase(c)...))

	desc := &PreparedRunDescriptor{
		RunID:                 strings.TrimSpace(runID),
		SuitePath:             suitePath,
		SuiteName:             strings.TrimSpace(suite.Metadata.Name),
		CaseName:              strings.TrimSpace(c.Name),
		AgentName:             strings.TrimSpace(suite.Spec.AgentName),
		Instruction:           strings.TrimSpace(c.Prompt),
		WorkspaceRoot:         targetWorkspace,
		RunRoot:               artifacts.RunRoot,
		DerivedWorkspaceRoot:  artifacts.SetupWorkspaceDir,
		ConfigPath:            filepath.Join(artifacts.SetupWorkspaceDir, config.DirName, "config.yaml"),
		AgentsDir:             filepath.Join(artifacts.SetupWorkspaceDir, config.DirName, "agents"),
		LogsDir:               artifacts.ExecutionLogsDir,
		TelemetryDir:          artifacts.ExecutionTelemetryDir,
		SetupDir:              artifacts.SetupDir,
		ExecutionDir:          artifacts.ExecutionDir,
		SetupLogsDir:          artifacts.SetupLogsDir,
		SetupTelemetryDir:     artifacts.SetupTelemetryDir,
		SetupArtifactsDir:     artifacts.SetupArtifactsDir,
		ExecutionLogsDir:      artifacts.ExecutionLogsDir,
		ExecutionTelemetryDir: artifacts.ExecutionTelemetryDir,
		ExecutionArtifactsDir: artifacts.ExecutionArtifactsDir,
		VerificationDir:       artifacts.VerificationDir,
		ServiceResetStrategy:  firstNonEmpty(opts.BackendReset, selected.ResetStrategy),
		ServiceResetBetween:   opts.BackendResetBetween,
		BackendSelection:      backendSelection,
		BackendProvider:       selected.Provider,
		BackendFamily:         selected.Family,
		BackendEndpoint:       selected.Endpoint,
		BackendBinary:         selected.Binary,
		BackendService:        selected.Service,
		BackendResetStrategy:  selected.ResetStrategy,
		BackendMatrix:         backendTargets,
		ModelName:             firstNonEmpty(model.Name, selected.Provider),
		SkipASTIndex:          opts.SkipASTIndex,
		StrictMode:            suite.IsStrictRun(opts.Profile, opts.Strict),
		MaxIterations:         resolveCaseMaxIterations(opts, c),
		MaxRetries:            resolveCaseMaxRetries(opts),
		SetupOverlays:         setupOverlays,
		SeededState:           seededState,
		Verification:          verification,
		ExpectedArtifacts:     expectedArtifacts,
	}

	if err := desc.Normalize(); err != nil {
		return nil, err
	}
	return desc, nil
}

func (d *PreparedRunDescriptor) Normalize() error {
	if d == nil {
		return fmt.Errorf("descriptor required")
	}
	d.RunID = strings.TrimSpace(d.RunID)
	d.SuitePath = cleanAbsolutePath(d.SuitePath)
	d.WorkspaceRoot = cleanAbsolutePath(d.WorkspaceRoot)
	d.RunRoot = cleanAbsolutePath(d.RunRoot)
	d.DerivedWorkspaceRoot = cleanAbsolutePath(d.DerivedWorkspaceRoot)
	d.ConfigPath = cleanAbsolutePath(d.ConfigPath)
	d.AgentsDir = cleanAbsolutePath(d.AgentsDir)
	d.LogsDir = cleanAbsolutePath(d.LogsDir)
	d.TelemetryDir = cleanAbsolutePath(d.TelemetryDir)
	d.SetupDir = cleanAbsolutePath(d.SetupDir)
	d.ExecutionDir = cleanAbsolutePath(d.ExecutionDir)
	d.SetupLogsDir = cleanAbsolutePath(d.SetupLogsDir)
	d.SetupTelemetryDir = cleanAbsolutePath(d.SetupTelemetryDir)
	d.SetupArtifactsDir = cleanAbsolutePath(d.SetupArtifactsDir)
	d.ExecutionLogsDir = cleanAbsolutePath(d.ExecutionLogsDir)
	d.ExecutionTelemetryDir = cleanAbsolutePath(d.ExecutionTelemetryDir)
	d.ExecutionArtifactsDir = cleanAbsolutePath(d.ExecutionArtifactsDir)
	d.VerificationDir = cleanAbsolutePath(d.VerificationDir)
	d.BackendSelection = strings.TrimSpace(d.BackendSelection)
	d.BackendProvider = strings.TrimSpace(d.BackendProvider)
	d.BackendFamily = strings.TrimSpace(d.BackendFamily)
	d.BackendEndpoint = strings.TrimSpace(d.BackendEndpoint)
	d.BackendBinary = strings.TrimSpace(d.BackendBinary)
	d.BackendService = strings.TrimSpace(d.BackendService)
	d.BackendResetStrategy = strings.TrimSpace(d.BackendResetStrategy)
	d.ServiceResetStrategy = strings.TrimSpace(d.ServiceResetStrategy)
	d.ModelName = strings.TrimSpace(d.ModelName)
	d.SandboxBackend = strings.TrimSpace(d.SandboxBackend)
	d.SetupOverlays = normalizePreparedRunOverlays(d.SetupOverlays)
	d.ExpectedArtifacts = UniqueStrings(d.ExpectedArtifacts)
	if err := d.Verification.Normalize(); err != nil {
		return err
	}
	if err := d.Validate(); err != nil {
		return err
	}
	return nil
}

func (d *PreparedRunDescriptor) Validate() error {
	if d == nil {
		return fmt.Errorf("descriptor required")
	}
	if d.RunID == "" {
		return fmt.Errorf("run_id required")
	}
	if d.SuitePath == "" {
		return fmt.Errorf("suite_path required")
	}
	if d.SuiteName == "" {
		return fmt.Errorf("suite_name required")
	}
	if d.CaseName == "" {
		return fmt.Errorf("case_name required")
	}
	if d.AgentName == "" {
		return fmt.Errorf("agent_name required")
	}
	if d.Instruction == "" {
		return fmt.Errorf("instruction required")
	}
	if d.WorkspaceRoot == "" {
		return fmt.Errorf("workspace_root required")
	}
	if d.RunRoot == "" {
		return fmt.Errorf("run_root required")
	}
	if d.DerivedWorkspaceRoot == "" {
		return fmt.Errorf("derived_workspace_root required")
	}
	if d.ConfigPath == "" {
		return fmt.Errorf("config_path required")
	}
	if d.SetupDir == "" {
		return fmt.Errorf("setup_dir required")
	}
	if d.ExecutionDir == "" {
		return fmt.Errorf("execution_dir required")
	}
	if d.BackendSelection == "" {
		return fmt.Errorf("backend_selection required")
	}
	if d.BackendSelection != PreparedRunSelectionSingle && d.BackendSelection != PreparedRunSelectionMatrix {
		return fmt.Errorf("backend_selection %q unsupported", d.BackendSelection)
	}
	if d.BackendSelection == PreparedRunSelectionSingle {
		if d.BackendProvider == "" {
			return fmt.Errorf("backend_provider required")
		}
		if d.BackendFamily == "" {
			return fmt.Errorf("backend_family required")
		}
		if d.BackendEndpoint == "" {
			return fmt.Errorf("backend_endpoint required")
		}
	}
	if d.MaxIterations < 0 {
		return fmt.Errorf("max_iterations must be >= 0")
	}
	if d.MaxRetries < 0 {
		return fmt.Errorf("max_retries must be >= 0")
	}
	if err := d.Verification.Validate(); err != nil {
		return err
	}
	for i, backend := range d.BackendMatrix {
		if err := backend.Validate(); err != nil {
			return fmt.Errorf("backend_matrix[%d]: %w", i, err)
		}
	}
	return nil
}

func resolveCaseMaxIterations(opts RunOptions, c CaseSpec) int {
	if c.Overrides.MaxIterations > 0 {
		return c.Overrides.MaxIterations
	}
	if opts.MaxIterations > 0 {
		return opts.MaxIterations
	}
	return 8
}

func (d *PreparedRunDescriptor) Write(path string) error {
	if d == nil {
		return fmt.Errorf("descriptor required")
	}
	if err := d.Normalize(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	if err := fs.MkdirAllSecure(filepath.Dir(path)); err != nil {
		return err
	}
	return fs.WriteFileSecure(path, data)
}

func LoadPreparedRunDescriptor(path string) (*PreparedRunDescriptor, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	var desc PreparedRunDescriptor
	if err := json.Unmarshal(data, &desc); err != nil {
		return nil, err
	}
	if err := desc.Normalize(); err != nil {
		return nil, err
	}
	return &desc, nil
}

func (c *PreparedVerificationContract) Normalize() error {
	if c == nil {
		return nil
	}
	c.Steps = normalizePreparedVerificationSteps(c.Steps)
	c.ExpectedArtifacts = UniqueStrings(c.ExpectedArtifacts)
	return nil
}

func (c PreparedVerificationContract) Validate() error {
	for i, step := range c.Steps {
		if err := step.Validate(); err != nil {
			return fmt.Errorf("verification_steps[%d]: %w", i, err)
		}
	}
	return nil
}

func (s PreparedVerificationStep) Validate() error {
	if strings.TrimSpace(s.Tool) == "" {
		return fmt.Errorf("verification step tool required")
	}
	return nil
}

func (b PreparedBackendTarget) Validate() error {
	if strings.TrimSpace(b.Provider) == "" {
		return fmt.Errorf("backend_provider required")
	}
	if strings.TrimSpace(b.Family) == "" {
		return fmt.Errorf("backend_family required")
	}
	if strings.TrimSpace(b.Endpoint) == "" {
		return fmt.Errorf("backend_endpoint required")
	}
	return nil
}

func buildPreparedBackendTargets(suite *Suite, c CaseSpec, model ModelSpec, opts RunOptions) []PreparedBackendTarget {
	selected := PreparedBackendTarget{
		Provider:      firstNonEmpty(model.Provider, "ollama"),
		Family:        firstNonEmpty(model.Provider, "ollama"),
		Endpoint:      firstNonEmpty(opts.EndpointOverride, model.Endpoint, "http://localhost:11434"),
		Binary:        firstNonEmpty(opts.BackendBinary, model.Provider, "ollama"),
		Service:       firstNonEmpty(opts.BackendService, model.Provider, "ollama"),
		ResetStrategy: firstNonEmpty(model.ResetStrategy, opts.BackendReset),
	}

	matrix := []PreparedBackendTarget{selected}
	rows := ExpandSuiteModelMatrix(suite.Spec.Models, suite.Spec.Providers, suite.Spec.Execution.MatrixOrder)
	if len(rows) == 0 {
		rows = []ModelSpec{{Name: model.Name, Provider: selected.Provider, Endpoint: selected.Endpoint, ResetStrategy: selected.ResetStrategy, ResetBetween: false}}
	}
	for _, row := range rows {
		target := PreparedBackendTarget{
			Provider:      firstNonEmpty(row.Provider, selected.Provider),
			Family:        firstNonEmpty(row.Provider, selected.Family),
			Endpoint:      firstNonEmpty(opts.EndpointOverride, row.Endpoint, selected.Endpoint),
			Binary:        firstNonEmpty(opts.BackendBinary, row.Provider, selected.Binary),
			Service:       firstNonEmpty(opts.BackendService, row.Provider, selected.Service),
			ResetStrategy: firstNonEmpty(row.ResetStrategy, opts.BackendReset),
		}
		matrix = append(matrix, target)
	}

	matrix = uniquePreparedBackendTargets(matrix)
	if len(matrix) == 0 {
		matrix = []PreparedBackendTarget{selected}
	}
	if len(matrix) == 1 {
		return matrix
	}
	return matrix
}

func buildPreparedVerificationContract(c CaseSpec) PreparedVerificationContract {
	contract := PreparedVerificationContract{}
	if c.Expect.Outcome != nil && c.Expect.Outcome.Verify != nil {
		for _, step := range c.Expect.Outcome.Verify.Steps {
			contract.Steps = append(contract.Steps, PreparedVerificationStep{
				Tool:              strings.TrimSpace(step.Tool),
				Args:              cloneContextMap(step.Args),
				ContinueOnFailure: step.ContinueOnFailure,
			})
		}
		contract.Script = strings.TrimSpace(c.Expect.Outcome.Verify.Script)
	}
	if c.Expect.Outcome != nil {
		contract.ExpectedArtifacts = append(contract.ExpectedArtifacts, expectedArtifactsForOutcome(*c.Expect.Outcome)...)
	}
	_ = contract.Normalize()
	return contract
}

func expectedArtifactsForCase(c CaseSpec) []string {
	if c.Expect.Outcome == nil {
		return nil
	}
	return expectedArtifactsForOutcome(*c.Expect.Outcome)
}

func expectedArtifactsForOutcome(outcome OutcomeSpec) []string {
	expected := make([]string, 0, len(outcome.FilesChanged)+len(outcome.FilesContain))
	expected = append(expected, outcome.FilesChanged...)
	for _, file := range outcome.FilesContain {
		if strings.TrimSpace(file.Path) == "" {
			continue
		}
		expected = append(expected, file.Path)
	}
	if outcome.Verify != nil && strings.TrimSpace(outcome.Verify.Script) != "" {
		expected = append(expected, outcome.Verify.Script)
	}
	return expected
}

func normalizePreparedRunOverlays(overlays []PreparedRunOverlay) []PreparedRunOverlay {
	if len(overlays) == 0 {
		return nil
	}
	out := make([]PreparedRunOverlay, 0, len(overlays))
	for _, overlay := range overlays {
		overlay.Path = filepath.Clean(strings.TrimSpace(overlay.Path))
		overlay.Mode = strings.TrimSpace(overlay.Mode)
		if overlay.Path == "" {
			continue
		}
		out = append(out, overlay)
	}
	return out
}

func normalizePreparedVerificationSteps(steps []PreparedVerificationStep) []PreparedVerificationStep {
	if len(steps) == 0 {
		return nil
	}
	out := make([]PreparedVerificationStep, 0, len(steps))
	for _, step := range steps {
		step.Tool = strings.TrimSpace(step.Tool)
		if step.Args == nil {
			step.Args = nil
		}
		if step.Tool == "" {
			continue
		}
		out = append(out, step)
	}
	return out
}

func uniquePreparedBackendTargets(in []PreparedBackendTarget) []PreparedBackendTarget {
	if len(in) == 0 {
		return nil
	}
	type key struct {
		provider string
		family   string
		endpoint string
		binary   string
		service  string
		reset    string
	}
	seen := map[key]struct{}{}
	out := make([]PreparedBackendTarget, 0, len(in))
	for _, target := range in {
		target.Provider = strings.TrimSpace(target.Provider)
		target.Family = strings.TrimSpace(target.Family)
		target.Endpoint = strings.TrimSpace(target.Endpoint)
		target.Binary = strings.TrimSpace(target.Binary)
		target.Service = strings.TrimSpace(target.Service)
		target.ResetStrategy = strings.TrimSpace(target.ResetStrategy)
		k := key{
			provider: target.Provider,
			family:   target.Family,
			endpoint: target.Endpoint,
			binary:   target.Binary,
			service:  target.Service,
			reset:    target.ResetStrategy,
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, target)
	}
	return out
}

// ArtifactPaths captures the canonical artifact file paths for a prepared run.
type ArtifactPaths struct {
	SetupLog          string
	SetupTelemetry    string
	ExecutionLog      string
	ExecutionTelemetry string
	Report            string
	Verification      string
}

// ArtifactPaths returns the canonical artifact file paths for this run.
func (d *PreparedRunDescriptor) ArtifactPaths() ArtifactPaths {
	return ArtifactPaths{
		SetupLog:          filepath.Join(d.SetupLogsDir, "agenttest.log"),
		SetupTelemetry:    filepath.Join(d.SetupTelemetryDir, "agenttest.jsonl"),
		ExecutionLog:      filepath.Join(d.ExecutionLogsDir, "agenttest.log"),
		ExecutionTelemetry: filepath.Join(d.ExecutionTelemetryDir, "agenttest.jsonl"),
		Report:            filepath.Join(d.ExecutionDir, "report.json"),
		Verification:      filepath.Join(d.VerificationDir, "verification.json"),
	}
}
