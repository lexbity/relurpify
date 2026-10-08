package authorization

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/governance/permissions"
)

// TestCommandCorpusAskRate measures the escalation rate over a corpus of
// real-world commands (R-5 detection) and proves dynamic/opaque commands never
// silently run.
func TestCommandCorpusAskRate(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("command_semantics_testdata", "commands.txt"))
	require.NoError(t, err)

	var commands [][]string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		commands = append(commands, strings.Fields(line))
	}

	total, ask := 0, 0
	for _, argv := range commands {
		total++
		res := &LiftedPermissions{}
		if err := liftCommand(argv, res, 0); err != nil {
			ask++
			continue
		}
		if res.HasDynamic {
			ask++
			continue
		}
		if _, _, dynamic := UnwrapCommand(argv); dynamic {
			ask++
		}
	}
	require.GreaterOrEqual(t, total, 90, "corpus must be substantive")
	rate := float64(ask) / float64(total)
	t.Logf("command corpus: %d commands, %d ask (%.1f%%)", total, ask, rate*100)
	require.LessOrEqual(t, rate, 0.10, "ask rate exceeded the 10%% budget")

	// Zero allow-of-dynamic: every dynamic/opaque command escalates to HITL.
	for _, argv := range commands {
		res := &LiftedPermissions{}
		_ = liftCommand(argv, res, 0)
		_, _, unwrapDynamic := UnwrapCommand(argv)
		if !res.HasDynamic && !unwrapDynamic {
			continue
		}
		pm, hitl := newCommandApprovalManager(t)
		err := AuthorizeCommand(context.Background(), pm, "agent-1", &BashConfig{Default: permissions.DecisionAllow}, CommandAuthorizationRequest{Command: argv})
		require.NoError(t, err, "approver approves the ask for %v", argv)
		require.NotEmpty(t, hitl.requests, "dynamic command %v must escalate to ask", argv)
	}
}
