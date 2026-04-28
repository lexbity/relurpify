package fileops

import (
	"codeburg.org/lexbit/relurpify/platform/contracts"
	clinix "codeburg.org/lexbit/relurpify/platform/shell/command"
)

// NewGitTool exposes a generic git CLI wrapper for compatibility with older
// coding-agent workflows and testsuites.
func NewGitTool(basePath string) contracts.Tool {
	return clinix.NewCommandTool(basePath, clinix.CommandToolConfig{
		Name:        "cli_git",
		Description: "Runs git with the provided arguments.",
		Command:     "git",
		Category:    "git",
		Tags:        []string{contracts.TagExecute},
	})
}
