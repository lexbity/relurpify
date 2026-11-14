package demo

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	"codeburg.org/lexbit/relurpify/userconfig/securefile"
)

// GuardPolicy supplies a read-only canonical policy fixture.
// DEMO ONLY: these declarations are not a permission grant or live enforcement.
// LIVE REPLACEMENT: NewWorkspaceGuardReader reads the actual confined source.
// SIDE EFFECTS: none; no file, provider, process, policy mutation or activation.
func (s *Session) GuardPolicy(ctx context.Context, section config.GuardSection) (tui.GuardPolicySnapshot, error) {
	if err := ctx.Err(); err != nil {
		return tui.GuardPolicySnapshot{}, err
	}
	var source string
	switch section {
	case config.GuardShell:
		source = "schema: relurpify/policy/shell/v1\nrules:\n  - id: destructive-root\n    pattern: 'rm\\s+-rf\\s+/'\n    action: block\n    reason: 'DEMO pattern fixture; live enforcement is not established'\n"
	case config.GuardIngestion:
		source = "schema: relurpify/policy/ingestion/v1\nrules:\n  - id: example-ingestion\n    name: DEMO declaration\n    enabled: true\n    priority: 100\n    effect:\n      action: deny\n      reason: 'DEMO inspection only; G2 evaluator is unresolved'\n"
	default:
		return tui.GuardPolicySnapshot{}, fmt.Errorf("unknown guard section")
	}
	return tui.GuardPolicySnapshot{Workspace: demoWorkspace, Document: config.GuardPolicyDocument{Section: section, Source: source, Revision: securefile.ComputeDigest([]byte(source))}, Notice: "DEMO: source fixture only; no enforcement is established"}, nil
}
