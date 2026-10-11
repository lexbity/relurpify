package main

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAgentTestHelpMatchesREADME proves the CLI honesty gate (D9): every
// flag documented in the agenttests README exists on the run command, and
// the deleted dead flags are documented nowhere.
func TestAgentTestHelpMatchesREADME(t *testing.T) {
	cmd := newAgentTestRunCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	require.NoError(t, cmd.Help())
	help := buf.String()

	// Extract --flag names from the help output.
	helpFlags := map[string]bool{}
	for _, m := range regexp.MustCompile(`--([a-z][a-z0-9-]*)`).FindAllStringSubmatch(help, -1) {
		helpFlags[m[1]] = true
	}

	raw, err := os.ReadFile("../../testsuite/agenttests/README.md")
	require.NoError(t, err)
	readme := string(raw)
	// Scope to the "Common flags" block: other sections document OTHER
	// subcommands' flags (e.g. promote's --run).
	common := readme
	if start := strings.Index(readme, "Common flags:"); start >= 0 {
		rest := readme[start:]
		if end := strings.Index(rest, "\n```"); end >= 0 {
			common = rest[:end]
		}
	}
	readmeFlags := map[string]bool{}
	for _, m := range regexp.MustCompile(`--([a-z][a-z0-9-]*)`).FindAllStringSubmatch(common, -1) {
		readmeFlags[m[1]] = true
	}

	for flag := range readmeFlags {
		require.True(t, helpFlags[flag], "README documents --%s but the run command does not have it", flag)
	}
	for _, dead := range []string{"sandbox", "lane", "bootstrap-timeout", "debug-agent", "backend-reset-between"} {
		require.False(t, readmeFlags[dead], "the deleted dead flag --%s must not reappear in the README", dead)
		require.False(t, helpFlags[dead], "the deleted dead flag --%s must not reappear on the command", dead)
	}

	// The timeout contract is visible: per-case execution timeout, 0 disables.
	require.True(t, strings.Contains(help, "timeout"), "the run command documents --timeout")
	require.Contains(t, readme, "0 disables", "the README states the disable opt-out")
}
