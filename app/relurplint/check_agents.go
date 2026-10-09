package main

import (
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// codeAgentBashDefault warns that a workspace agent declares bash rules but no
// explicit bash default. The terminal default is now ask (P-4), so commands
// that match no rule require approval; an explicit default is the migration.
const codeAgentBashDefault = "agent.bash_default"

type agentsCheck struct{}

func init() {
	registerCheck(agentsCheck{})
}

func (c agentsCheck) Name() string { return "agents" }

func (c agentsCheck) Run(workspace string) []Diagnostic {
	agentsDir := config.New(workspace).AgentsDir()
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return nil
	}

	var diags []Diagnostic
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(agentsDir, entry.Name())
		snapshot, err := config.LoadDocument(path)
		if err != nil || snapshot.Document == nil {
			continue
		}
		node, ok := snapshot.Document.Section("agent")
		if !ok {
			continue
		}
		spec, err := config.DecodeAgentSection(node)
		if err != nil || spec == nil {
			continue
		}
		if len(spec.Bash.AllowPatterns) == 0 && len(spec.Bash.DenyPatterns) == 0 {
			continue
		}
		if strings.TrimSpace(string(spec.Bash.Default)) != "" {
			continue
		}
		diags = append(diags, Diagnostic{
			Check:    "agents",
			Code:     codeAgentBashDefault,
			Severity: SeverityWarning,
			Loc:      SourceLoc{File: path},
			Message: "bash_permissions declares rules but no explicit default; " +
				"commands that match no rule now require approval (ask). " +
				"Set bash_permissions.default: allow to preserve prior behavior, or ask/deny to stay strict.",
		})
	}
	return diags
}
