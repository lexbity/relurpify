package runtime

import (
	"testing"
)

// TestDoctorReportRunscDockerNotBlocking verifies that runsc and docker
// are marked as Required: false and Blocking: false.
func TestDoctorReportRunscDockerNotBlocking(t *testing.T) {
	// Create a minimal report with mock dependencies
	report := DoctorReport{
		Workspace:        "/tmp/test",
		ConfigExists:     true,
		ManifestExists:   true,
		WorkspacePresent: true,
		Dependencies: []DependencyStatus{
			{
				Name:      "runsc",
				Required:  false,
				Available: false,
				Blocking:  false,
				Details:   "not found — sandbox unavailable — tool sandboxing disabled",
			},
			{
				Name:      "docker",
				Required:  false,
				Available: false,
				Blocking:  false,
				Details:   "not found — sandbox unavailable — tool sandboxing disabled",
			},
			{
				Name:      "ollama",
				Required:  true,
				Available: true,
				Blocking:  false,
				Details:   "qwen2.5-coder:14b",
			},
		},
	}

	// Verify HasBlockingIssues returns false when runsc/docker are unavailable
	if report.HasBlockingIssues() {
		t.Error("expected HasBlockingIssues() to be false when only runsc/docker are unavailable")
	}
}

// TestDoctorReportOllamaBlocking verifies that ollama is Blocking when unavailable.
func TestDoctorReportOllamaBlocking(t *testing.T) {
	report := DoctorReport{
		Workspace:        "/tmp/test",
		ConfigExists:     true,
		ManifestExists:   true,
		WorkspacePresent: true,
		Dependencies: []DependencyStatus{
			{
				Name:      "runsc",
				Required:  false,
				Available: true,
				Blocking:  false,
				Details:   "v1.0.0",
			},
			{
				Name:      "docker",
				Required:  false,
				Available: true,
				Blocking:  false,
				Details:   "v20.10.0",
			},
			{
				Name:      "ollama",
				Required:  true,
				Available: false,
				Blocking:  true,
				Details:   "connection refused",
			},
		},
	}

	// Verify HasBlockingIssues returns true when ollama is unavailable
	if !report.HasBlockingIssues() {
		t.Error("expected HasBlockingIssues() to be true when ollama is unavailable")
	}
}

// TestFormatSandboxDetail verifies the sandbox detail formatter.
func TestFormatSandboxDetail(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty detail",
			input:    "",
			expected: "sandbox unavailable — tool sandboxing disabled",
		},
		{
			name:     "not found error",
			input:    "not found",
			expected: "not found — sandbox unavailable — tool sandboxing disabled",
		},
		{
			name:     "generic error",
			input:    "error: connection refused",
			expected: "error: connection refused — sandbox unavailable — tool sandboxing disabled",
		},
		{
			name:     "version string",
			input:    "v1.0.0",
			expected: "v1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatSandboxDetail(tt.input)
			if result != tt.expected {
				t.Errorf("formatSandboxDetail(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestDoctorReportWithMissingConfig verifies blocking behavior when config is missing.
func TestDoctorReportWithMissingConfig(t *testing.T) {
	report := DoctorReport{
		Workspace:        "/tmp/test",
		ConfigExists:     false,
		ManifestExists:   true,
		WorkspacePresent: true,
	}

	if !report.HasBlockingIssues() {
		t.Error("expected HasBlockingIssues() to be true when config is missing")
	}
}

// TestDoctorReportWithMissingManifest verifies blocking behavior when manifest is missing.
func TestDoctorReportWithMissingManifest(t *testing.T) {
	report := DoctorReport{
		Workspace:        "/tmp/test",
		ConfigExists:     true,
		ManifestExists:   false,
		WorkspacePresent: true,
	}

	if !report.HasBlockingIssues() {
		t.Error("expected HasBlockingIssues() to be true when manifest is missing")
	}
}
