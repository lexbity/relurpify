package plan

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestValidatePlanRejectsMalformedPlans covers the plan-dependency validator's
// rejection branches and the cycle formatter, which were previously untouched.
func TestValidatePlanRejectsMalformedPlans(t *testing.T) {
	require.NoError(t, ValidatePlan(nil))
	require.NoError(t, ValidatePlan(&Plan{
		Steps:        []PlanStep{{ID: "a"}, {ID: "b", DependsOn: []string{"a"}}},
		Dependencies: map[string][]string{"b": {"a"}},
	}))

	cases := []struct {
		name    string
		plan    *Plan
		wantSub string
	}{
		{
			name:    "missing step id",
			plan:    &Plan{Steps: []PlanStep{{ID: ""}}, Dependencies: map[string][]string{}},
			wantSub: "missing id",
		},
		{
			name:    "duplicate step id",
			plan:    &Plan{Steps: []PlanStep{{ID: "a"}, {ID: "a"}}, Dependencies: map[string][]string{}},
			wantSub: "duplicate step id",
		},
		{
			name:    "unknown dependency step",
			plan:    &Plan{Steps: []PlanStep{{ID: "a"}}, Dependencies: map[string][]string{"ghost": {"a"}}},
			wantSub: "unknown step",
		},
		{
			name:    "missing dependency target",
			plan:    &Plan{Steps: []PlanStep{{ID: "a"}}, Dependencies: map[string][]string{"a": {"ghost"}}},
			wantSub: "references missing step",
		},
		{
			name:    "self dependency",
			plan:    &Plan{Steps: []PlanStep{{ID: "a"}}, Dependencies: map[string][]string{"a": {"a"}}},
			wantSub: "depends on itself",
		},
		{
			name:    "empty dependency id",
			plan:    &Plan{Steps: []PlanStep{{ID: "a"}}, Dependencies: map[string][]string{"a": {""}}},
			wantSub: "empty dependency id",
		},
		{
			name:    "dependency cycle",
			plan:    &Plan{Steps: []PlanStep{{ID: "a"}, {ID: "b"}}, Dependencies: map[string][]string{"a": {"b"}, "b": {"a"}}},
			wantSub: "cycle detected",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePlan(tc.plan)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantSub)
		})
	}
}

// TestFormatCycle covers both the in-stack and not-in-stack paths.
func TestFormatCycle(t *testing.T) {
	require.Equal(t, "a -> b -> a", formatCycle([]string{"a", "b"}, "a"))
	require.Equal(t, "missing", formatCycle([]string{"a"}, "missing"))
}
