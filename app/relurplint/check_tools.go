package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/toolcapabilities"
	"codeburg.org/lexbit/relurpify/testsuite/configcheck"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	"codeburg.org/lexbit/relurpify/userconfig/templates"
)

type toolsCheck struct{}

// codeToolUnderdeclared is the diagnostic code for a tool manifest whose
// declared risk/effect classes are missing entries derived from its own
// command/sandbox config (configcheck.DeriveExpectedCapability).
const codeToolUnderdeclared = "tool.underdeclared"

func init() {
	registerCheck(toolsCheck{})
}

func (c toolsCheck) Name() string { return "tools" }

func (c toolsCheck) Run(workspace string) []Diagnostic {
	report := config.ValidateWorkspaceTree(workspace)

	var diags []Diagnostic
	for _, issue := range report.Issues {
		if !isToolIssue(issue) {
			continue
		}
		code := "tool.schema"
		diags = append(diags, Diagnostic{
			Check:    "tools",
			Code:     code,
			Severity: SeverityError,
			Loc:      SourceLoc{File: issue.File, Line: extractLine(issue.Reason)},
			Message:  issue.Reason,
		})
	}

	sec2Diags := runSEC2Check(workspace)
	diags = append(diags, sec2Diags...)

	embedDiags := runEmbeddedSEC2Check()
	diags = append(diags, embedDiags...)

	return diags
}

func runEmbeddedSEC2Check() []Diagnostic {
	tmpDir, err := os.MkdirTemp("", "relurpify-embed-check")
	if err != nil {
		return []Diagnostic{{
			Check:    "tools",
			Code:     "embed.tempdir",
			Severity: SeverityError,
			Message:  fmt.Sprintf("create temp dir for embedded check: %v", err),
		}}
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	outDir := filepath.Join(tmpDir, "relurpify_cfg")
	if err := templates.GenerateConfig(outDir); err != nil {
		return nil
	}

	toolsDir := filepath.Join(outDir, "tools")
	manifests, err := config.LoadToolManifests(toolsDir)
	if err != nil {
		return nil
	}

	results := configcheck.CheckAllManifests(manifests)
	if len(results) == 0 {
		return nil
	}

	var diags []Diagnostic
	for name, issues := range results {
		for _, issue := range issues {
			sourcePath := "embedded:" + manifestRelPath(manifests, name)
			diags = append(diags, Diagnostic{
				Check:    "tools",
				Code:     codeToolUnderdeclared,
				Severity: SeverityError,
				Loc:      SourceLoc{File: sourcePath},
				Message:  name + ": " + issue,
			})
		}
	}
	return diags
}

func runSEC2Check(workspace string) []Diagnostic {
	toolsDir := filepath.Join(workspace, "relurpify_cfg", "tools")
	manifests, err := config.LoadToolManifests(toolsDir)
	if err != nil {
		return nil
	}

	results := configcheck.CheckAllManifests(manifests)
	if len(results) == 0 {
		return nil
	}

	var diags []Diagnostic
	for name, issues := range results {
		for _, issue := range issues {
			diags = append(diags, Diagnostic{
				Check:    "tools",
				Code:     codeToolUnderdeclared,
				Severity: SeverityError,
				Loc:      SourceLoc{File: manifestRelPath(manifests, name)},
				Message:  name + ": " + issue,
			})
		}
	}
	return diags
}

func manifestRelPath(manifests []*toolcapabilities.ToolManifest, name string) string {
	for _, m := range manifests {
		if m != nil && m.Name == name && m.SourcePath != "" {
			// SourcePath is like /abs/path/relurpify_cfg/tools/shell/bash.tool.yaml.
			// Extract the part starting at "relurpify_cfg/".
			if idx := strings.Index(m.SourcePath, "relurpify_cfg"); idx >= 0 {
				return m.SourcePath[idx:]
			}
			return m.SourcePath
		}
	}
	return "relurpify_cfg/tools/" + name + ".tool.yaml"
}
