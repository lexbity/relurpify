package agenttest

import (
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/platform/fs"
)

// PreparedRunArtifacts captures the canonical on-disk layout for a prepared
// live run. The layout is intentionally split so setup and execution artifacts
// stay independently inspectable.
type PreparedRunArtifacts struct {
	WorkspaceRoot         string
	RunRoot               string
	RunID                 string
	AgentName             string
	SetupDir              string
	SetupWorkspaceDir     string
	SetupLogsDir          string
	SetupTelemetryDir     string
	SetupArtifactsDir     string
	SetupDescriptorPath   string
	ExecutionDir          string
	ExecutionLogsDir      string
	ExecutionTelemetryDir string
	ExecutionArtifactsDir string
	VerificationDir       string
	RunReportPath         string
}

func NewPreparedRunArtifacts(workspaceRoot, runRoot, agentName, runID string) PreparedRunArtifacts {
	workspaceRoot = cleanAbsolutePath(workspaceRoot)
	runRoot = cleanAbsolutePath(runRoot)
	agentName = strings.TrimSpace(agentName)
	runID = strings.TrimSpace(runID)

	return PreparedRunArtifacts{
		WorkspaceRoot:         workspaceRoot,
		RunRoot:               runRoot,
		RunID:                 runID,
		AgentName:             agentName,
		SetupDir:              filepath.Join(runRoot, "setup"),
		SetupWorkspaceDir:     filepath.Join(runRoot, "setup", "workspace"),
		SetupLogsDir:          filepath.Join(runRoot, "setup", "logs"),
		SetupTelemetryDir:     filepath.Join(runRoot, "setup", "telemetry"),
		SetupArtifactsDir:     filepath.Join(runRoot, "setup", "artifacts"),
		SetupDescriptorPath:   filepath.Join(runRoot, "setup", "prepared_run.json"),
		ExecutionDir:          filepath.Join(runRoot, "execution"),
		ExecutionLogsDir:      filepath.Join(runRoot, "execution", "logs"),
		ExecutionTelemetryDir: filepath.Join(runRoot, "execution", "telemetry"),
		ExecutionArtifactsDir: filepath.Join(runRoot, "execution", "artifacts"),
		VerificationDir:       filepath.Join(runRoot, "verification"),
		RunReportPath:         filepath.Join(runRoot, "execution", "report.json"),
	}
}

func (a PreparedRunArtifacts) AllDirs() []string {
	return []string{
		a.RunRoot,
		a.SetupDir,
		a.SetupWorkspaceDir,
		a.SetupLogsDir,
		a.SetupTelemetryDir,
		a.SetupArtifactsDir,
		a.ExecutionDir,
		a.ExecutionLogsDir,
		a.ExecutionTelemetryDir,
		a.ExecutionArtifactsDir,
		a.VerificationDir,
	}
}

func (a PreparedRunArtifacts) Ensure() error {
	for _, dir := range a.AllDirs() {
		if dir == "" {
			continue
		}
		if err := fs.MkdirAllSecure(dir); err != nil {
			return err
		}
	}
	return nil
}

func (a PreparedRunArtifacts) DescriptorPath() string {
	return a.SetupDescriptorPath
}

func (a PreparedRunArtifacts) SetupRoot() string {
	return a.SetupDir
}

func (a PreparedRunArtifacts) ExecutionRoot() string {
	return a.ExecutionDir
}

func (a PreparedRunArtifacts) Normalized() PreparedRunArtifacts {
	a.WorkspaceRoot = cleanAbsolutePath(a.WorkspaceRoot)
	a.RunRoot = cleanAbsolutePath(a.RunRoot)
	a.RunID = strings.TrimSpace(a.RunID)
	a.AgentName = strings.TrimSpace(a.AgentName)
	a.SetupDir = cleanAbsolutePath(a.SetupDir)
	a.SetupWorkspaceDir = cleanAbsolutePath(a.SetupWorkspaceDir)
	a.SetupLogsDir = cleanAbsolutePath(a.SetupLogsDir)
	a.SetupTelemetryDir = cleanAbsolutePath(a.SetupTelemetryDir)
	a.SetupArtifactsDir = cleanAbsolutePath(a.SetupArtifactsDir)
	a.SetupDescriptorPath = cleanAbsolutePath(a.SetupDescriptorPath)
	a.ExecutionDir = cleanAbsolutePath(a.ExecutionDir)
	a.ExecutionLogsDir = cleanAbsolutePath(a.ExecutionLogsDir)
	a.ExecutionTelemetryDir = cleanAbsolutePath(a.ExecutionTelemetryDir)
	a.ExecutionArtifactsDir = cleanAbsolutePath(a.ExecutionArtifactsDir)
	a.VerificationDir = cleanAbsolutePath(a.VerificationDir)
	a.RunReportPath = cleanAbsolutePath(a.RunReportPath)
	return a
}

func cleanAbsolutePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(path)
}
