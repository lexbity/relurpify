package skills

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestRenderPlanningPolicyIncludesSharedFields(t *testing.T) {
	policy := ResolvedSkillPolicy{
		PhaseCapabilities: map[string][]string{
			"explore": {"file_read"},
			"verify":  {"go_test"},
		},
		VerificationSuccessCapabilities: []string{"go_test"},
		Planning: ResolvedPlanningPolicy{
			RequiredBeforeEdit:          []string{"go_workspace_detect"},
			PreferredEditCapabilities:   []string{"file_write"},
			PreferredVerifyCapabilities: []string{"go_test"},
			StepTemplates:               []core.SkillStepTemplate{{Kind: "verify", Description: "Run tests"}},
			RequireVerificationStep:     true,
		},
	}

	rendered := RenderPlanningPolicy(policy, PlanningRenderOptions{
		IncludePhaseCapabilities:   true,
		IncludeVerificationSuccess: true,
	})

	require.Contains(t, rendered, "Explore capabilities: file_read")
	require.Contains(t, rendered, "Verification success capabilities: go_test")
	require.Contains(t, rendered, "Required before edit: go_workspace_detect")
	require.Contains(t, rendered, "Preferred verify capabilities: go_test")
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
		VerificationSuccessCapabilities: []string{"rust_cargo_test"},
		RecoveryProbeCapabilities:       []string{"rust_workspace_detect", "rust_cargo_metadata"},
	}

	rendered := RenderExecutionPolicy(&policy, true)

	require.Contains(t, rendered, "Verification success capabilities: rust_cargo_test")
	require.Contains(t, rendered, "Stop immediately after a successful verification capability runs after the latest edit.")
	require.Contains(t, rendered, "Preferred recovery probes on failures: rust_workspace_detect, rust_cargo_metadata")
}
