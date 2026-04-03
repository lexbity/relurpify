package plans

import (
	"testing"
	"time"

	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/stretchr/testify/require"
)

func TestDiffStrings(t *testing.T) {
	require.Nil(t, diffStrings(nil, nil))
	require.Nil(t, diffStrings([]string{}, []string{}))
	require.Equal(t, []string{"c"}, diffStrings([]string{"a", "b", "c"}, []string{"a", "b"}))
	require.Empty(t, diffStrings([]string{"a", "b"}, []string{"a", "b", "c"}))
	require.Equal(t, []string{"a", "c"}, diffStrings([]string{"a", "b", "c"}, []string{"b"}))
}

func TestStringSlicesEqual(t *testing.T) {
	require.True(t, stringSlicesEqual(nil, nil))
	require.True(t, stringSlicesEqual([]string{}, []string{}))
	require.True(t, stringSlicesEqual([]string{"a", "b"}, []string{"a", "b"}))
	require.False(t, stringSlicesEqual([]string{"a"}, []string{"a", "b"}))
	require.False(t, stringSlicesEqual([]string{"a", "b"}, []string{"a", "c"}))
}

func TestStepsEqual(t *testing.T) {
	now := time.Now().UTC()
	step1 := &frameworkplan.PlanStep{
		ID:              "s1",
		Description:     "desc",
		Status:          frameworkplan.PlanStepPending,
		ConfidenceScore: 0.5,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	step2 := &frameworkplan.PlanStep{
		ID:              "s1",
		Description:     "desc",
		Status:          frameworkplan.PlanStepPending,
		ConfidenceScore: 0.5,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	require.True(t, stepsEqual(step1, step2))

	step3 := &frameworkplan.PlanStep{
		ID:              "s1",
		Description:     "desc",
		Status:          frameworkplan.PlanStepCompleted,
		ConfidenceScore: 0.5,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	require.False(t, stepsEqual(step1, step3))

	gate := &frameworkplan.EvidenceGate{RequiredAnchors: []string{"a"}}
	step1.EvidenceGate = gate
	require.False(t, stepsEqual(step1, step2))
	step2.EvidenceGate = gate
	require.True(t, stepsEqual(step1, step2))

	// test nil
	require.True(t, stepsEqual(nil, nil))
	require.False(t, stepsEqual(step1, nil))
	require.False(t, stepsEqual(nil, step2))
}
