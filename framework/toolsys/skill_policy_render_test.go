package toolsys

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestRenderPlanningPolicyIncludesSharedFields(t *testing.T) {
	policy := ResolvedSkillPolicy{
		PhaseTools: map[string][]string{
			"explore": {"file_read"},
			"verify":  {"go_test"},
		},
		VerificationSuccessTools: []string{"go_test"},
		Planning: ResolvedPlanningPolicy{
			RequiredBeforeEdit:      []string{"go_workspace_detect"},
			PreferredEditTools:      []string{"file_write"},
			PreferredVerifyTools:    []string{"go_test"},
			StepTemplates:           []core.SkillStepTemplate{{Kind: "verify", Description: "Run tests"}},
			RequireVerificationStep: true,
		},
	}

	rendered := RenderPlanningPolicy(policy, PlanningRenderOptions{
		IncludePhaseTools:          true,
		IncludeVerificationSuccess: true,
	})

	require.Contains(t, rendered, "Explore tools: file_read")
	require.Contains(t, rendered, "Verification success tools: go_test")
	require.Contains(t, rendered, "Required before edit: go_workspace_detect")
	require.Contains(t, rendered, "Preferred verify tools: go_test")
	require.Contains(t, rendered, "Preferred step templates: verify: Run tests")
	require.Contains(t, rendered, "Plans must include an explicit verification step.")
}

func TestRenderReviewPolicyIncludesSeveritySummary(t *testing.T) {
	policy := ResolvedSkillPolicy{
		Review: ResolvedReviewPolicy{
			Criteria:  []string{"correctness", "completeness"},
			FocusTags: []string{"verification"},
			ApprovalRules: core.AgentReviewApprovalRules{
				RequireVerificationEvidence: true,
				RejectOnUnresolvedErrors:    true,
			},
			SeverityWeights: map[string]float64{"high": 1.0, "medium": 0.4, "low": 0.1},
		},
	}

	rendered := RenderReviewPolicy(policy)

	require.Contains(t, rendered, "Review criteria: correctness, completeness")
	require.Contains(t, rendered, "Focus tags: verification")
	require.Contains(t, rendered, "Require verification evidence before approval.")
	require.Contains(t, rendered, "Reject outputs with unresolved errors.")
	require.Contains(t, rendered, "Severity weights: high=1.00, medium=0.40, low=0.10")
}

func TestRenderExecutionPolicyIncludesRecoveryAndStopOnSuccess(t *testing.T) {
	policy := ResolvedSkillPolicy{
		VerificationSuccessTools: []string{"rust_cargo_test"},
		RecoveryProbeTools:       []string{"rust_workspace_detect", "rust_cargo_metadata"},
	}

	rendered := RenderExecutionPolicy(&policy, true)

	require.Contains(t, rendered, "Verification success tools: rust_cargo_test")
	require.Contains(t, rendered, "Stop immediately after a successful verification tool runs after the latest edit.")
	require.Contains(t, rendered, "Preferred recovery probes on failures: rust_workspace_detect, rust_cargo_metadata")
}
