// Package testhelper provides fixture builders and helpers shared across the
// repository's test suites.
package testhelper

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// TestMustWriteAndMkdirAllCreateTree covers the two path helpers directly so a
// regression in their secure-mode wiring surfaces here rather than in every
// consumer test.
func TestMustWriteAndMkdirAllCreateTree(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "nested", "deep")
	MustMkdirAll(t, dir)
	MustWrite(t, filepath.Join(dir, "file.txt"), "content")

	data, err := os.ReadFile(filepath.Clean(filepath.Join(dir, "file.txt")))
	require.NoError(t, err)
	require.Equal(t, "content", string(data))
}

// TestWriteValidWorkspaceLoads proves the fixture produces a workspace tree the
// configuration validator accepts (no network, no model). config.Load runs in
// strict mode and rejects the deliberately minimal fixture's defaulted
// sections, so the cheapest faithful loader assertion is the tree validator.
func TestWriteValidWorkspaceLoads(t *testing.T) {
	ws := t.TempDir()
	WriteValidWorkspace(t, ws)

	report := config.ValidateWorkspaceTree(ws)
	require.NoError(t, report.Err(), "WriteValidWorkspace must produce a valid workspace tree")
}

// TestInitGitRepoHasOneCommit proves the repository fixture commits its content
// exactly once. Skipped (not failed) when git is unavailable; the statements are
// exercised wherever CI has git.
func TestInitGitRepoHasOneCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	ws := t.TempDir()
	MustWrite(t, filepath.Join(ws, "README.md"), "hello world")
	MustMkdirAll(t, filepath.Join(ws, "src"))
	MustWrite(t, filepath.Join(ws, "src", "main.go"), "package main\n")

	InitGitRepo(t, ws)

	out, err := exec.Command("git", "-C", ws, "log", "--oneline").CombinedOutput() //nolint:gosec // test-only git invocation against a controlled temp dir
	require.NoError(t, err, string(out))
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	require.Len(t, lines, 1, "expected exactly one commit, got: %s", out)
}

// TestNewTestAuditLoggerRoundTrip proves the file-backed audit logger helper
// writes a record and the chain verifies, exercising the real chain rather than
// a ring-buffer stand-in (SBH-1 D-10).
func TestNewTestAuditLoggerRoundTrip(t *testing.T) {
	logger := NewTestAuditLogger(t)
	ctx := context.Background()

	require.NoError(t, logger.Log(ctx, policy.AuditRecord{
		AgentID: "test-agent",
		Action:  "test:action",
		Type:    "permission_request",
		Result:  "granted",
	}))

	verification, err := logger.VerifyChain(ctx, policy.AuditChainFilter{})
	require.NoError(t, err)
	require.True(t, verification.Verified, "chain verification failed: %s", verification.Failure)
	require.Equal(t, 1, verification.EntryCount, "one logged record must be committed to the chain")
}
