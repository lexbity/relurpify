package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/userconfig/config/model"
	"codeburg.org/lexbit/relurpify/userconfig/config/security"
)

// ValidateWorkspaceTree runs report-collecting validation of the checked-in
// relurpify_cfg tree: workspace.yaml, security policies, model
// providers/profiles, and tool manifests.
//
// Unlike Load (fail-fast), it accumulates every issue into a ValidationReport.
// It does NOT scan source code — the codebase boundary audit is
// scripts/boundaryaudit.
func ValidateWorkspaceTree(workspace string) *ValidationReport {
	report := &ValidationReport{}
	if strings.TrimSpace(workspace) == "" {
		report.Add("", "workspace", "", "workspace required")
		return report
	}

	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		report.Add("", "workspace", workspace, fmt.Sprintf("resolve workspace: %v", err))
		return report
	}

	cfgRoot := filepath.Join(absWorkspace, "relurpify_cfg")

	// 1. Validate workspace.yaml
	wsPath := filepath.Join(cfgRoot, "workspace.yaml")
	var workspaceCfg *WorkspaceConfig
	if _, err := os.Stat(wsPath); err != nil {
		report.Add("relurpify_cfg/workspace.yaml", "", "", err.Error())
	} else {
		var err error
		workspaceCfg, err = LoadWorkspaceConfig(wsPath, absWorkspace, WorkspaceLoadOptions{})
		if err != nil {
			report.Add("relurpify_cfg/workspace.yaml", "", "", err.Error())
		}
	}

	// 2. Validate security policies
	if _, err := security.LoadSandboxPolicy("", absWorkspace, StrictDecode); err != nil {
		report.Add("relurpify_cfg/security/sandbox.policy.yaml", "", "", err.Error())
	}
	if _, err := security.LoadShellPolicy("", absWorkspace, StrictDecode); err != nil {
		report.Add("relurpify_cfg/security/shell.policy.yaml", "", "", err.Error())
	}
	if _, err := security.LoadLocalToolPolicy("", absWorkspace, StrictDecode); err != nil {
		report.Add("relurpify_cfg/security/localtool.policy.yaml", "", "", err.Error())
	}
	if _, err := security.LoadWorkspaceIngestionPolicy("", absWorkspace, StrictDecode); err != nil {
		report.Add("relurpify_cfg/security/workspaceingestion.policy.yaml", "", "", err.Error())
	}

	// 3. Validate model providers and profiles
	modelDir := filepath.Join(cfgRoot, "model")
	providers, err := model.LoadProviderDir(filepath.Join(modelDir, "provider"), StrictDecode)
	if err != nil {
		report.Add("relurpify_cfg/model/provider", "", "", err.Error())
	}
	if err == nil && workspaceCfg != nil {
		if err := workspaceCfg.ValidateModelRef(providers); err != nil {
			report.Add("relurpify_cfg/workspace.yaml", "model", "", err.Error())
		}
	}
	if _, err := model.LoadProfileDir(filepath.Join(modelDir, "profiles"), StrictDecode); err != nil {
		report.Add("relurpify_cfg/model/profiles", "", "", err.Error())
	}

	// 4. Validate tools
	toolsDir := filepath.Join(cfgRoot, "tools")
	manifests, err := LoadToolManifests(toolsDir)
	if err != nil {
		report.Add("relurpify_cfg/tools", "", "", err.Error())
	} else if policy, err := security.LoadLocalToolPolicy("", absWorkspace, StrictDecode); err != nil {
		report.Add("relurpify_cfg/security/localtool.policy.yaml", "", "", err.Error())
	} else if _, err := BuildRegistry(manifests, policy, nil); err != nil {
		report.Add("relurpify_cfg/tools", "", "", err.Error())
	}

	return report
}
