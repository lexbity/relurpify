package agenttest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// VerificationContract captures the execution artifacts required to verify a prepared run.
type VerificationContract struct {
	DescriptorPath         string
	ExecutionReportPath    string
	SetupTelemetryPath     string
	ExecutionTelemetryPath string
	SetupLogPath           string
	ExecutionLogPath       string
}

func BuildVerificationContract(desc *PreparedRunDescriptor) (VerificationContract, error) {
	if desc == nil {
		return VerificationContract{}, fmt.Errorf("descriptor required")
	}
	if err := preparedRunEnsure(desc); err != nil {
		return VerificationContract{}, err
	}
	ap := desc.ArtifactPaths()
	return VerificationContract{
		DescriptorPath:         filepath.Join(desc.SetupDir, "prepared_run.json"),
		ExecutionReportPath:    ap.Report,
		SetupTelemetryPath:     ap.SetupTelemetry,
		ExecutionTelemetryPath: ap.ExecutionTelemetry,
		SetupLogPath:           ap.SetupLog,
		ExecutionLogPath:       ap.ExecutionLog,
	}, nil
}

func LoadCaseReport(path string) (*CaseReport, error) {
	if path == "" {
		return nil, fmt.Errorf("report path required")
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	var report CaseReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}
	return &report, nil
}
