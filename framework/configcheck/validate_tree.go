package configcheck

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/cfgload"
	"codeburg.org/lexbit/relurpify/framework/cfgload/model"
	"codeburg.org/lexbit/relurpify/framework/cfgload/security"
)

// ValidateWorkspaceTree validates the checked-in relurpify_cfg tree.
func ValidateWorkspaceTree(workspace string) *cfgload.ValidationReport {
	report := &cfgload.ValidationReport{}
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
	if _, err := os.Stat(wsPath); err != nil {
		report.Add("relurpify_cfg/workspace.yaml", "", "", err.Error())
	} else {
		if _, err := cfgload.LoadWorkspaceConfig(wsPath, absWorkspace, cfgload.WorkspaceLoadOptions{}); err != nil {
			report.Add("relurpify_cfg/workspace.yaml", "", "", err.Error())
		}
	}

	// 2. Validate security policies
	if _, err := security.LoadSandboxPolicy("", absWorkspace); err != nil {
		report.Add("relurpify_cfg/security/sandbox.policy.yaml", "", "", err.Error())
	}
	if _, err := security.LoadShellPolicy("", absWorkspace); err != nil {
		report.Add("relurpify_cfg/security/shell.policy.yaml", "", "", err.Error())
	}
	if _, err := security.LoadLocalToolPolicy("", absWorkspace); err != nil {
		report.Add("relurpify_cfg/security/localtool.policy.yaml", "", "", err.Error())
	}
	if _, err := security.LoadWorkspaceIngestionPolicy("", absWorkspace); err != nil {
		report.Add("relurpify_cfg/security/workspaceingestion.policy.yaml", "", "", err.Error())
	}

	// 3. Validate model providers and profiles
	modelDir := filepath.Join(cfgRoot, "model")
	if _, err := model.LoadProviderDir(filepath.Join(modelDir, "provider")); err != nil {
		report.Add("relurpify_cfg/model/provider", "", "", err.Error())
	}
	if _, err := model.LoadProfileDir(filepath.Join(modelDir, "profiles")); err != nil {
		report.Add("relurpify_cfg/model/profiles", "", "", err.Error())
	}

	// 4. Validate tools
	toolsDir := filepath.Join(cfgRoot, "tools")
	if _, err := cfgload.LoadToolManifests(toolsDir); err != nil {
		report.Add("relurpify_cfg/tools", "", "", err.Error())
	}

	// 5. Validate agents
	agentsDir := filepath.Join(cfgRoot, "agents")
	basePath := filepath.Join(agentsDir, "_base.agent.yaml")
	var baseAgent *cfgload.AgentConfig
	if _, err := os.Stat(basePath); err == nil {
		baseAgent, err = cfgload.LoadBaseAgentConfig(basePath, absWorkspace, nil, "")
		if err != nil {
			report.Add("relurpify_cfg/agents/_base.agent.yaml", "", "", err.Error())
		}
	}
	if _, err := cfgload.LoadAgentRegistry(agentsDir, baseAgent, absWorkspace, nil, ""); err != nil {
		report.Add("relurpify_cfg/agents", "", "", err.Error())
	}

	return report
}
