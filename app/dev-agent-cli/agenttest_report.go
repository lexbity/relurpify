package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/testsuite/agenttest"
)

type preparedRunReport struct {
	DescriptorPath         string   `json:"descriptor_path"`
	Workspace              string   `json:"workspace"`
	ConfigPath             string   `json:"config_path"`
	ManifestPath           string   `json:"manifest_path"`
	SetupLogPath           string   `json:"setup_log_path,omitempty"`
	SetupTelemetryPath     string   `json:"setup_telemetry_path,omitempty"`
	ExecutionLogPath       string   `json:"execution_log_path,omitempty"`
	ExecutionTelemetryPath string   `json:"execution_telemetry_path,omitempty"`
	BackendProvider        string   `json:"backend_provider,omitempty"`
	BackendFamily          string   `json:"backend_family,omitempty"`
	BackendEndpoint        string   `json:"backend_endpoint,omitempty"`
	Services               []string `json:"services,omitempty"`
	Projection             any      `json:"projection,omitempty"`
}

func writePreparedRunReport(path string, report preparedRunReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func reportFromPreparedRun(desc *agenttest.PreparedRunDescriptor, workspace string, serviceIDs []string, result *core.Result) preparedRunReport {
	if desc == nil {
		return preparedRunReport{}
	}
	if strings.TrimSpace(workspace) == "" {
		workspace = desc.DerivedWorkspaceRoot
	}
	configPath := desc.ConfigPath
	manifestPath := desc.ManifestPath
	if strings.TrimSpace(workspace) != "" {
		configPath = filepath.Join(workspace, ".relurpify_state", "workspace.yaml")
		manifestPath = filepath.Join(workspace, "relurpify_cfg", "agents", "coding.yaml")
	}
	report := preparedRunReport{
		DescriptorPath:         filepath.Join(desc.SetupDir, "prepared_run.json"),
		Workspace:              workspace,
		ConfigPath:             configPath,
		ManifestPath:           manifestPath,
		SetupLogPath:           filepath.Join(desc.SetupLogsDir, "agenttest.log"),
		SetupTelemetryPath:     filepath.Join(desc.SetupTelemetryDir, "agenttest.jsonl"),
		ExecutionLogPath:       filepath.Join(desc.ExecutionLogsDir, "agenttest.log"),
		ExecutionTelemetryPath: filepath.Join(desc.ExecutionTelemetryDir, "agenttest.jsonl"),
		BackendProvider:        desc.BackendProvider,
		BackendFamily:          desc.BackendFamily,
		BackendEndpoint:        desc.BackendEndpoint,
		Services:               serviceIDs,
	}
	if result != nil && len(result.Data) > 0 {
		if projection, ok := result.Data["projection"]; ok {
			report.Projection = projection
		}
	}
	return report
}
