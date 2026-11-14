package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"codeburg.org/lexbit/relurpify/userconfig/config/security"
	"codeburg.org/lexbit/relurpify/userconfig/securefile"
)

const (
	policySandboxFilename   = "sandbox.policy.yaml"
	policyShellFilename     = "shell.policy.yaml"
	policyLocalToolFilename = "localtool.policy.yaml"
	policyIngestionFilename = "workspaceingestion.policy.yaml"
	// PolicySourceLimit bounds both diagnostic and guard inspection reads.
	PolicySourceLimit = 1 << 20
)

// ReadWorkspacePolicySource reads a canonical policy through a pinned workspace
// root. It never creates files or follows a leaf symlink or a hard-linked file.
// This is the migration target for boot policy reads; diagnostics use it now.
func ReadWorkspacePolicySource(ctx context.Context, workspace, filename string) (data []byte, revision string, err error) {
	switch filename {
	case policySandboxFilename, policyShellFilename, policyLocalToolFilename, policyIngestionFilename:
	default:
		return nil, "", fmt.Errorf("unknown workspace policy %q", filename)
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return nil, "", err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	data, revision, err = securefile.ReadRegularWithin(root, filepath.Join("relurpify_cfg", "security", filename), PolicySourceLimit)
	if err != nil {
		return nil, "", err
	}
	if revision == securefile.AbsentRevision {
		return nil, "", fmt.Errorf("policy %s missing: %w", filename, os.ErrNotExist)
	}
	return data, revision, ctx.Err()
}

// loadConfinedSecurityBundle preserves per-document failures without ever
// retrying an unsafe read through the unconfined boot loader.
func loadConfinedSecurityBundle(workspace string) (*security.Bundle, []ConfigDiagnostic) {
	bundle := &security.Bundle{}
	parsers := []struct {
		filename string
		parse    func(string, []byte) error
	}{
		{policySandboxFilename, func(path string, data []byte) (err error) {
			bundle.Sandbox, err = security.ParseSandboxPolicy(path, workspace, data, StrictDecode)
			return err
		}},
		{policyShellFilename, func(path string, data []byte) (err error) {
			bundle.Shell, err = security.ParseShellPolicy(path, data, StrictDecode)
			return err
		}},
		{policyLocalToolFilename, func(path string, data []byte) (err error) {
			bundle.LocalTool, err = security.ParseLocalToolPolicy(path, data, StrictDecode)
			return err
		}},
		{policyIngestionFilename, func(path string, data []byte) (err error) {
			bundle.Ingestion, err = security.ParseWorkspaceIngestionPolicy(path, data, StrictDecode)
			return err
		}},
	}
	var diags []ConfigDiagnostic
	for _, parser := range parsers {
		path := filepath.Join(workspace, "relurpify_cfg", "security", parser.filename)
		data, _, err := ReadWorkspacePolicySource(context.Background(), workspace, parser.filename)
		if err == nil {
			err = parser.parse(path, data)
		}
		if err != nil {
			diags = append(diags, policyReadDiagnostic(path, err))
		}
	}
	return bundle, diags
}

func policyReadDiagnostic(path string, err error) ConfigDiagnostic {
	severity := configDiagnosticBlocking
	if errors.Is(err, os.ErrNotExist) {
		severity = "warning"
	}
	return ConfigDiagnostic{Path: path, Section: "security", Severity: severity, Message: err.Error()}
}
