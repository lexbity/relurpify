package runtime

import (
	"fmt"
	"time"
)

// Readiness carries the workspace's degradation state. Canonical readiness is
// doctor's (BuildDoctorReport -> DoctorReport.Ready); status and the TUI gate
// consume that verdict, so this struct only records when a composed runtime
// fell back to the degraded deny-all path and why.
type Readiness struct {
	Degraded bool
	Reason   string
}

// WorkspaceReadiness describes the overall readiness state of a workspace (P05).
// This extends the basic Readiness struct with detailed state information.
type WorkspaceReadiness struct {
	// Ready indicates if the workspace is fully ready.
	Ready bool
	// Degraded indicates if there are degraded but non-blocking issues.
	Degraded bool
	// BlockingIssues lists issues that block the workspace from being ready.
	BlockingIssues []BlockingIssue
	// Warnings lists non-blocking warnings.
	Warnings []string
	// CheckedAt is when the readiness was last checked.
	CheckedAt time.Time
	// Generation is the current active generation ID.
	Generation string
	// StagedGeneration is the staged generation ID (if any).
	StagedGeneration string
	// SandboxReady indicates if the sandbox is ready.
	SandboxReady bool
	// ModelReady indicates if the model is ready.
	ModelReady bool
	// ModelDegraded indicates if the model is degraded.
	ModelDegraded bool
	// ConfigValid indicates if the configuration is valid.
	ConfigValid bool
	// AuditReady indicates if audit/storage is ready.
	AuditReady bool
}

// BlockingIssue represents a single blocking issue.
type BlockingIssue struct {
	// ID is a unique identifier for this issue.
	ID string
	// Category is the category of the issue (config, model, sandbox, etc.).
	Category string
	// Message is a human-readable description of the issue.
	Message string
	// CheckID is the ID of the doctor check that found this issue.
	CheckID string
	// Severity indicates how severe the issue is (error, warning, info).
	Severity IssueSeverity
	// Action describes what action is needed to resolve the issue.
	Action string
}

// IssueSeverity represents the severity of an issue.
type IssueSeverity string

const (
	SeverityError   IssueSeverity = "error"
	SeverityWarning IssueSeverity = "warning"
	SeverityInfo    IssueSeverity = "info"
)

// IsReady returns true if the workspace is ready (no blocking issues).
func (r *WorkspaceReadiness) IsReady() bool {
	return r.Ready && len(r.BlockingIssues) == 0
}

// HasBlockingIssues returns true if there are any blocking issues.
func (r *WorkspaceReadiness) HasBlockingIssues() bool {
	for _, issue := range r.BlockingIssues {
		if issue.Severity == SeverityError {
			return true
		}
	}
	return false
}

// NeedsInitialization returns true if the workspace needs initialization.
func (r *WorkspaceReadiness) NeedsInitialization() bool {
	// Check if config exists and is valid
	if !r.ConfigValid {
		// Check if this is because config doesn't exist
		for _, issue := range r.BlockingIssues {
			if issue.Category == "config" && issue.ID == "config_missing" {
				return true
			}
		}
	}
	return false
}

// CanActivateGeneration returns true if a new generation can be activated.
func (r *WorkspaceReadiness) CanActivateGeneration() bool {
	// Can activate if ready or only has degraded issues
	return r.Ready || (r.Degraded && !r.HasBlockingIssues())
}

// ReadinessSnapshot is a point-in-time snapshot of workspace readiness.
type ReadinessSnapshot struct {
	// Readiness is the current readiness state.
	Readiness WorkspaceReadiness
	// Checks is the list of check results that contributed to this snapshot.
	Checks []DoctorCheck
	// LastUpdated is when this snapshot was created.
	LastUpdated time.Time
}

// CheckWorkspaceReadiness checks the current workspace readiness.
// This is called by Doctor to build the readiness section of the report.
func CheckWorkspaceReadiness(report *DoctorReport) WorkspaceReadiness {
	readiness := WorkspaceReadiness{
		Ready:           report.Ready(),
		Degraded:        report.ModelDegraded,
		BlockingIssues:  make([]BlockingIssue, 0),
		Warnings:        make([]string, 0),
		CheckedAt:       time.Now().UTC(),
		SandboxReady:    report.SandboxReady,
		ModelReady:      report.ModelReady,
		ModelDegraded:   report.ModelDegraded,
		ConfigValid:     report.ConfigExists && report.ConfigError == "",
		AuditReady:      report.AuditStoreHealthy,
	}

	// Extract blocking issues from dependencies
	for _, dep := range report.Dependencies {
		if dep.Blocking {
			readiness.BlockingIssues = append(readiness.BlockingIssues, BlockingIssue{
				ID:       dep.Name + "_missing",
				Category: "dependency",
				Message:  fmt.Sprintf("%s is not available", dep.Name),
				CheckID:  "dep_" + dep.Name,
				Severity: SeverityError,
				Action:   fmt.Sprintf("Install or configure %s", dep.Name),
			})
		} else if dep.Degraded {
			readiness.Warnings = append(readiness.Warnings,
				fmt.Sprintf("%s is degraded: %s", dep.Name, dep.Details))
		}
	}

	// Check config
	if !report.ConfigExists {
		readiness.BlockingIssues = append(readiness.BlockingIssues, BlockingIssue{
			ID:       "config_missing",
			Category: "config",
			Message:  "Workspace configuration is missing",
			CheckID:  "config_exist",
			Severity: SeverityError,
			Action:   "Initialize workspace with starter templates",
		})
	} else if report.ConfigError != "" {
		readiness.BlockingIssues = append(readiness.BlockingIssues, BlockingIssue{
			ID:       "config_invalid",
			Category: "config",
			Message:  fmt.Sprintf("Workspace configuration is invalid: %s", report.ConfigError),
			CheckID:  "config_validate",
			Severity: SeverityError,
			Action:   "Fix configuration errors",
		})
	}

	// Check sandbox
	if !report.SandboxReady {
		readiness.BlockingIssues = append(readiness.BlockingIssues, BlockingIssue{
			ID:       "sandbox_not_ready",
			Category: "sandbox",
			Message:  "Sandbox is not ready",
			CheckID:  "sandbox_check",
			Severity: SeverityError,
			Action:   "Verify sandbox backend and configuration",
		})
	}

	// Check model
	if !report.ModelReady && !report.ModelDegraded {
		readiness.BlockingIssues = append(readiness.BlockingIssues, BlockingIssue{
			ID:       "model_unavailable",
			Category: "model",
			Message:  "Model provider is unavailable",
			CheckID:  "model_check",
			Severity: SeverityError,
			Action:   "Configure a working model provider",
		})
	} else if report.ModelDegraded {
		readiness.Warnings = append(readiness.Warnings,
			"Model provider is degraded")
	}

	// Check audit/store health from the Doctor report.
	readiness.AuditReady = report.AuditStoreHealthy
	if !report.AuditStoreHealthy {
		readiness.BlockingIssues = append(readiness.BlockingIssues, BlockingIssue{
			ID:       "audit_unavailable",
			Category: "audit",
			Message:  "Audit or store dependency is not healthy",
			CheckID:  "audit_chain",
			Severity: SeverityError,
			Action:   "Repair the audit/store dependency and recheck Doctor",
		})
	}

	// Update ready state
	readiness.Ready = len(readiness.BlockingIssues) == 0 &&
		readiness.SandboxReady &&
		(report.ModelReady || report.ModelDegraded) &&
		readiness.ConfigValid &&
		readiness.AuditReady

	return readiness
}
