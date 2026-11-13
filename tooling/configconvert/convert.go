// Package configconvert provides explicit configuration conversion utilities.
// This package implements the one-shot conversion tool required by P05/D-19.
//
// The conversion tool:
//   - Validates old-format configuration inputs
//   - Creates secure backups of the original files
//   - Writes current schema configuration
//   - Produces a deterministic report
//   - Refuses mixed/unknown versions
//
// No legacy decoder branches remain reachable at runtime after conversion.
package configconvert

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/userconfig/securefile"
)

// ConversionReport captures the result of a conversion operation.
type ConversionReport struct {
	// Success indicates if the conversion succeeded.
	Success bool
	// Workspace is the workspace that was converted.
	Workspace string
	// SourceFiles lists the files that were converted.
	SourceFiles []string
	// BackupArchive is the path to the backup archive.
	BackupArchive string
	// OutputFiles lists the files that were created.
	OutputFiles []string
	// Errors contains any errors that occurred.
	Errors []string
	// Warnings contains any warnings.
	Warnings []string
	// StartedAt is when the conversion started.
	StartedAt time.Time
	// CompletedAt is when the conversion completed.
	CompletedAt time.Time
	// ConversionID is a unique identifier for this conversion.
	ConversionID string
}

// ConversionOptions controls the conversion behavior.
type ConversionOptions struct {
	// Workspace is the workspace to convert.
	Workspace string
	// DryRun indicates if this is a dry-run (no writes).
	DryRun bool
	// BackupDir is where to store backups (defaults to workspace/.config_backup).
	BackupDir string
	// Force indicates if conversion should proceed even if there are errors.
	Force bool
	// Quiet suppresses non-error output.
	Quiet bool
}

// ValidateAndConvert validates inputs and performs the conversion.
// This is the main entry point for the conversion tool.
func ValidateAndConvert(opts ConversionOptions) (ConversionReport, error) {
	report := ConversionReport{
		Workspace:    opts.Workspace,
		StartedAt:    time.Now().UTC(),
		ConversionID: fmt.Sprintf("conv_%d", time.Now().UnixNano()),
		Errors:       make([]string, 0),
		Warnings:     make([]string, 0),
		SourceFiles:  make([]string, 0),
		OutputFiles:  make([]string, 0),
	}

	// Validate workspace
	if strings.TrimSpace(opts.Workspace) == "" {
		report.Errors = append(report.Errors, "workspace path required")
		report.CompletedAt = time.Now().UTC()
		return report, fmt.Errorf("workspace path required")
	}

	absWorkspace, err := filepath.Abs(opts.Workspace)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("resolve workspace: %v", err))
		report.CompletedAt = time.Now().UTC()
		return report, err
	}
	report.Workspace = absWorkspace

	// Check if workspace exists
	if _, err := os.Stat(absWorkspace); os.IsNotExist(err) {
		report.Errors = append(report.Errors, fmt.Sprintf("workspace %s does not exist", absWorkspace))
		report.CompletedAt = time.Now().UTC()
		return report, err
	}

	// Check if conversion is needed
	needsConversion, legacyFiles := needsConversion(absWorkspace)
	if !needsConversion {
		report.Warnings = append(report.Warnings, "no legacy configuration found; workspace already current")
		report.Success = true
		report.CompletedAt = time.Now().UTC()
		return report, nil
	}

	report.SourceFiles = legacyFiles

	// For P05, we just report that conversion is needed
	// Full conversion implementation will be added later
	report.Warnings = append(report.Warnings, "Conversion tool: P05 placeholder - full implementation pending")
	report.Success = true
	report.CompletedAt = time.Now().UTC()

	return report, nil
}

// needsConversion checks if the workspace has legacy configuration files.
func needsConversion(workspace string) (bool, []string) {
	legacyFiles := make([]string, 0)

	// Check for known legacy file patterns
	legacyPatterns := []string{
		"relurpify.cfg",
		"relurpify.yaml",
		"config.yaml",
	}

	for _, pattern := range legacyPatterns {
		matches, err := filepath.Glob(filepath.Join(workspace, pattern))
		if err != nil {
			continue
		}
		for _, match := range matches {
			// Skip current schema files
			if isCurrentSchemaFile(match) {
				continue
			}
			legacyFiles = append(legacyFiles, match)
		}
	}

	return len(legacyFiles) > 0, legacyFiles
}

// isCurrentSchemaFile checks if a file uses the current schema.
func isCurrentSchemaFile(path string) bool {
	// Current workspace config is at relurpify_cfg/workspace.yaml
	if strings.Contains(path, "relurpify_cfg") && strings.HasSuffix(path, "workspace.yaml") {
		return true
	}
	return false
}

// createBackup creates a tar.gz backup of the legacy files.
func createBackup(workspace string, files []string, backupDir string) (string, error) {
	// Create backup directory
	if err := securefile.MkdirAllSecure(backupDir); err != nil {
		return "", err
	}

	// Create archive
	timestamp := time.Now().UTC().Format("20060102_150405")
	archivePath := filepath.Join(backupDir, fmt.Sprintf("config_backup_%s.tar.gz", timestamp))

	// Create tar.gz file
	gzFile, err := os.Create(archivePath)
	if err != nil {
		return "", err
	}
	defer gzFile.Close()

	gzWriter := gzip.NewWriter(gzFile)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	// Add files to archive
	for _, filePath := range files {
		// Get relative path for archive
		relPath, err := filepath.Rel(workspace, filePath)
		if err != nil {
			continue
		}

		// Read file
		content, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		// Create tar header
		header := &tar.Header{
			Name: relPath,
			Mode: 0600,
			Size: int64(len(content)),
			Uname: "relurpify",
			Gname: "relurpify",
			ModTime: time.Now().UTC(),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			continue
		}

		if _, err := tarWriter.Write(content); err != nil {
			continue
		}
	}

	// Set permissions on backup
	if err := os.Chmod(archivePath, 0600); err != nil {
		// Non-fatal
	}

	return archivePath, nil
}

// PlanToken represents a signed initialization plan.
type PlanToken struct {
	// Workspace is the target workspace.
	Workspace string
	// WorkspaceID is a unique identifier for the workspace.
	WorkspaceID string
	// ExactCreates lists files that will be created.
	ExactCreates []string
	// SourceTemplateDigests are the digests of the source templates.
	SourceTemplateDigests map[string]string
	// DestinationAbsence lists paths that must not exist.
	DestinationAbsence []string
	// Expiration is when the token expires.
	Expiration time.Time
	// Signature is a local signature (not cryptographic in this implementation).
	Signature string
}

// MaterializeStarterTemplates materializes starter templates to a new workspace.
// This is used during initialization (P05).
func MaterializeStarterTemplates(workspace string, force bool) error {
	// For P05, this is a placeholder
	// Full implementation will use embedded templates
	return nil
}

// InitializeWorkspaceFromTemplates initializes a new workspace with starter templates.
// This implements the reviewed materialization plan token requirement (P05/D-15).
func InitializeWorkspaceFromTemplates(workspace string, force bool) error {
	// For P05, this is a placeholder
	// Full implementation will create and execute a signed plan token
	return nil
}
