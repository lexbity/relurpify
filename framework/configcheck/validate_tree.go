package configcheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/cfgload"
	"codeburg.org/lexbit/relurpify/framework/cfgload/model"
	"codeburg.org/lexbit/relurpify/framework/cfgload/security"
)

// ValidateWorkspaceTree validates the checked-in relurpify_cfg tree and codebase boundary policies.
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
	var workspaceCfg *cfgload.WorkspaceConfig
	if _, err := os.Stat(wsPath); err != nil {
		report.Add("relurpify_cfg/workspace.yaml", "", "", err.Error())
	} else {
		var err error
		workspaceCfg, err = cfgload.LoadWorkspaceConfig(wsPath, absWorkspace, cfgload.WorkspaceLoadOptions{})
		if err != nil {
			report.Add("relurpify_cfg/workspace.yaml", "", "", err.Error())
		}
	}

	// 2. Validate security policies
	if _, err := security.LoadSandboxPolicy("", absWorkspace, cfgload.StrictDecode); err != nil {
		report.Add("relurpify_cfg/security/sandbox.policy.yaml", "", "", err.Error())
	}
	if _, err := security.LoadShellPolicy("", absWorkspace, cfgload.StrictDecode); err != nil {
		report.Add("relurpify_cfg/security/shell.policy.yaml", "", "", err.Error())
	}
	if _, err := security.LoadLocalToolPolicy("", absWorkspace, cfgload.StrictDecode); err != nil {
		report.Add("relurpify_cfg/security/localtool.policy.yaml", "", "", err.Error())
	}
	if _, err := security.LoadWorkspaceIngestionPolicy("", absWorkspace, cfgload.StrictDecode); err != nil {
		report.Add("relurpify_cfg/security/workspaceingestion.policy.yaml", "", "", err.Error())
	}

	// 3. Validate model providers and profiles
	modelDir := filepath.Join(cfgRoot, "model")
	providers, err := model.LoadProviderDir(filepath.Join(modelDir, "provider"), cfgload.StrictDecode)
	if err != nil {
		report.Add("relurpify_cfg/model/provider", "", "", err.Error())
	}
	if err == nil && workspaceCfg != nil {
		if err := workspaceCfg.ValidateModelRef(providers); err != nil {
			report.Add("relurpify_cfg/workspace.yaml", "model", "", err.Error())
		}
	}
	if _, err := model.LoadProfileDir(filepath.Join(modelDir, "profiles"), cfgload.StrictDecode); err != nil {
		report.Add("relurpify_cfg/model/profiles", "", "", err.Error())
	}

	// 4. Validate tools
	toolsDir := filepath.Join(cfgRoot, "tools")
	manifests, err := cfgload.LoadToolManifests(toolsDir)
	if err != nil {
		report.Add("relurpify_cfg/tools", "", "", err.Error())
	} else if policy, err := security.LoadLocalToolPolicy("", absWorkspace, cfgload.StrictDecode); err != nil {
		report.Add("relurpify_cfg/security/localtool.policy.yaml", "", "", err.Error())
	} else if _, err := cfgload.BuildRegistry(manifests, policy, nil); err != nil {
		report.Add("relurpify_cfg/tools", "", "", err.Error())
	}

	// 5. Validate agents (declared in workspace.yaml agents: section; validated by LoadWorkspaceConfig)

	// 6. Audit codebase for config/environment boundary violations
	auditCodebaseBoundary(absWorkspace, report)

	return report
}

func auditCodebaseBoundary(workspaceAbs string, report *cfgload.ValidationReport) {
	// Only scan if it looks like the actual repository workspace
	if _, err := os.Stat(filepath.Join(workspaceAbs, "framework", "cfgload")); err != nil {
		return
	}

	fset := token.NewFileSet()
	err := filepath.WalkDir(workspaceAbs, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		rel, err := filepath.Rel(workspaceAbs, path)
		if err != nil {
			return nil
		}

		// Exclude allowed packages/directories for env lookup
		isEnvExempt := strings.HasPrefix(rel, "framework/cfgload/") || strings.HasPrefix(rel, "framework/runtimeenv/") || strings.Contains(rel, "/testsuite/")

		// Exclude allowed packages/directories for config file read/write
		isConfigExempt := strings.HasPrefix(rel, "framework/cfgload/") || strings.Contains(rel, "/testsuite/")

		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		file, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			return nil
		}

		lines := strings.Split(string(src), "\n")

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			x, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if x.Name != "os" {
				return true
			}

			pos := fset.Position(sel.Pos())
			snippet := ""
			if pos.Line > 0 && pos.Line <= len(lines) {
				snippet = strings.TrimSpace(lines[pos.Line-1])
			}

			// Rule 1: os.Getenv or os.Environ is forbidden outside allowed env packages
			if sel.Sel.Name == "Getenv" || sel.Sel.Name == "Environ" {
				if !isEnvExempt {
					report.Add(rel, fmt.Sprintf("line:%d", pos.Line), "os."+sel.Sel.Name, fmt.Sprintf("direct environment boundary violation: %s", snippet))
				}
				return true
			}

			// Rule 2: os.ReadFile, os.ReadDir, os.WriteFile, os.OpenFile, os.Create with "relurpify_cfg" is forbidden outside allowed config packages
			if sel.Sel.Name == "ReadFile" || sel.Sel.Name == "ReadDir" || sel.Sel.Name == "WriteFile" || sel.Sel.Name == "OpenFile" || sel.Sel.Name == "Create" {
				if !isConfigExempt {
					hasRelurpifyCfg := false
					for _, arg := range call.Args {
						if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING {
							if strings.Contains(lit.Value, "relurpify_cfg") {
								hasRelurpifyCfg = true
								break
							}
						}
					}
					if hasRelurpifyCfg || strings.Contains(snippet, "relurpify_cfg") {
						report.Add(rel, fmt.Sprintf("line:%d", pos.Line), "os."+sel.Sel.Name, fmt.Sprintf("direct config path boundary violation: %s", snippet))
					}
				}
			}

			return true
		})

		return nil
	})
	if err != nil {
		report.Add("", "codebase_audit", "", fmt.Sprintf("failed to walk workspace directory: %v", err))
	}
}
