package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/capability/configcheck"
	"codeburg.org/lexbit/relurpify/capability/toolcapabilities"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	"codeburg.org/lexbit/relurpify/userconfig/templates"
)

type toolsCheck struct{}

// codeToolUnderdeclared is the diagnostic code for a tool manifest whose
// declared risk/effect classes are missing entries derived from its own
// command/sandbox config (configcheck.DeriveExpectedCapability).
const codeToolUnderdeclared = "tool.underdeclared"

// codeToolEgressAllowlist is the diagnostic code for a manifest whose egress
// allowlists violate the mandatory-denylist policy (a non-public literal in
// allow_hosts, or a host present in both allow_hosts and allow_private_hosts).
const codeToolEgressAllowlist = "tool.egress_allowlist"

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

	var diags []Diagnostic
	for name, issues := range configcheck.CheckAllManifests(manifests) {
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
	diags = append(diags, egressDiagnostics(manifests, func(name string) string {
		return "embedded:" + manifestRelPath(manifests, name)
	})...)
	return diags
}

func runSEC2Check(workspace string) []Diagnostic {
	toolsDir := filepath.Join(workspace, "relurpify_cfg", "tools")
	manifests, err := config.LoadToolManifests(toolsDir)
	if err != nil {
		return nil
	}

	var diags []Diagnostic
	for name, issues := range configcheck.CheckAllManifests(manifests) {
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
	diags = append(diags, egressDiagnostics(manifests, func(name string) string {
		return manifestRelPath(manifests, name)
	})...)
	return diags
}

// egressDiagnostics reports blocking egress-allowlist violations for a
// manifest set. manifestLoc resolves a tool name to a display location.
func egressDiagnostics(manifests []*toolcapabilities.ToolManifest, manifestLoc func(string) string) []Diagnostic {
	var diags []Diagnostic
	for name, issues := range configcheck.CheckNetworkAllowlists(manifests) {
		for _, issue := range issues {
			diags = append(diags, Diagnostic{
				Check:    "tools",
				Code:     codeToolEgressAllowlist,
				Severity: SeverityError,
				Loc:      SourceLoc{File: manifestLoc(name)},
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
