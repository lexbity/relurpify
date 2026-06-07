package main

import (
	"path/filepath"

	"codeburg.org/lexbit/relurpify/execution/prompt"
)

type promptsCheck struct{}

func init() {
	registerCheck(promptsCheck{})
}

func (c promptsCheck) Name() string { return "prompts" }

func (c promptsCheck) Run(workspace string) []Diagnostic {
	reg := prompt.NewRegistry()

	promptDir := filepath.Join(workspace, "templates", "prompts")
	if err := reg.LoadDir(promptDir); err != nil {
		return []Diagnostic{{
			Check:    "prompts",
			Code:     "prompts.load",
			Severity: SeverityError,
			Message:  err.Error(),
		}}
	}

	issues := reg.ValidateAll()
	if len(issues) == 0 {
		return nil
	}

	var diags []Diagnostic
	for id, iss := range issues {
		for _, issue := range iss {
			sev := SeverityWarning
			if issue.Severity == prompt.SeverityError {
				sev = SeverityError
			}
			loc := SourceLoc{File: promptSourcePath(reg, id)}
			if issue.BlockID != "" {
				loc.Line = extractLine(issue.Message)
			}
			code := "prompts.validate"
			if issue.Severity == prompt.SeverityWarning {
				code = "prompts.warning"
			}
			diags = append(diags, Diagnostic{
				Check:    "prompts",
				Code:     code,
				Severity: sev,
				Loc:      loc,
				Message:  issue.Message,
			})
		}
	}
	return diags
}

func promptSourcePath(reg prompt.Registry, id string) string {
	if cfg, ok := reg.Get(id); ok && cfg.SourcePath != "" {
		return cfg.SourcePath
	}
	return id
}
