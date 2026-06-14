package main

import (
	"encoding/json"
	"path/filepath"

	"codeburg.org/lexbit/relurpify/capability/toolcapabilities"
	"codeburg.org/lexbit/relurpify/testsuite/configcheck"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

type toolsCheck struct{}

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

	return diags
}

func runSEC2Check(workspace string) []Diagnostic {
	toolsDir := filepath.Join(workspace, "relurpify_cfg", "tools")
	manifests, err := config.LoadToolManifests(toolsDir)
	if err != nil {
		return nil
	}

	convertedManifests := make([]*toolcapabilities.ToolManifest, 0, len(manifests))
	for _, m := range manifests {
		if m == nil {
			continue
		}
		var converted toolcapabilities.ToolManifest
		data, err := json.Marshal(m)
		if err != nil {
			return nil
		}
		if err := json.Unmarshal(data, &converted); err != nil {
			return nil
		}
		converted.SourcePath = m.SourcePath
		converted.CanonicalName = m.CanonicalName
		convertedManifests = append(convertedManifests, &converted)
	}

	results := configcheck.CheckAllManifests(convertedManifests)
	if len(results) == 0 {
		return nil
	}

	var diags []Diagnostic
	for name, issues := range results {
		for _, issue := range issues {
			diags = append(diags, Diagnostic{
				Check:    "tools",
				Code:     "tool.underdeclared",
				Severity: SeverityError,
				Loc:      SourceLoc{File: manifestSourcePath(convertedManifests, name)},
				Message:  name + ": " + issue,
			})
		}
	}
	return diags
}

func manifestSourcePath(manifests []*toolcapabilities.ToolManifest, name string) string {
	for _, m := range manifests {
		if m != nil && m.Name == name && m.SourcePath != "" {
			rel, err := filepath.Rel(filepath.Dir(m.SourcePath)+"/..", m.SourcePath)
			if err == nil {
				return rel
			}
			return m.SourcePath
		}
	}
	return "relurpify_cfg/tools/" + name + ".tool.yaml"
}
