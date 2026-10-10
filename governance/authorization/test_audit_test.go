package authorization

import (
	"testing"

	policy "codeburg.org/lexbit/relurpify/governance/policy"
)

// newTestAuditLogger returns a file-backed audit chain logger over a temp dir.
// The canonical testsuite/testhelper.NewTestAuditLogger is unavailable to this
// package's internal test files: testhelper imports capability/fs, which imports
// governance/authorization, which would close an import cycle in test. The
// behavior is identical (temp dir + cleanup close), so external consumers keep
// using the shared testhelper.
func newTestAuditLogger(t *testing.T) *policy.FileChainAuditLogger {
	t.Helper()
	l, err := policy.NewFileChainAuditLogger(t.TempDir(), policy.FileChainOptions{})
	if err != nil {
		t.Fatalf("NewFileChainAuditLogger: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l
}
