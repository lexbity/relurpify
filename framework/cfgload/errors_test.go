package cfgload

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScanErrorFormatting(t *testing.T) {
	err := &ScanError{Path: "framework/agentenv/workspace.go", Err: errors.New("boom")}
	require.Equal(t, "scan framework/agentenv/workspace.go: boom", err.Error())
	require.EqualError(t, err, "scan framework/agentenv/workspace.go: boom")
}

func TestAuditErrorFormatting(t *testing.T) {
	err := &AuditError{Findings: []Finding{{File: "app/relurpish/runtime/config.go"}}}
	require.Equal(t, "audit failed: 1 ambient config findings", err.Error())
	var target *AuditError
	require.ErrorAs(t, err, &target)
}
