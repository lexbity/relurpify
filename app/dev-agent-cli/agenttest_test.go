package main

import (
	"testing"

	"github.com/lexcodex/relurpify/testsuite/agenttest"
)

func TestShouldRunAgentTestSuiteExcludesQuarantinedByDefault(t *testing.T) {
	suite := &agenttest.Suite{
		Metadata: agenttest.SuiteMeta{
			Name:        "coding",
			Tier:        "stable",
			Quarantined: true,
		},
		Spec: agenttest.SuiteSpec{
			Execution: agenttest.SuiteExecutionSpec{Profile: "live"},
		},
	}

	if shouldRunAgentTestSuite(suite, "", "", false) {
		t.Fatal("expected quarantined suite to be filtered out")
	}
}

func TestShouldRunAgentTestSuiteMatchesTierAndProfile(t *testing.T) {
	suite := &agenttest.Suite{
		Metadata: agenttest.SuiteMeta{
			Name: "coding",
			Tier: "smoke",
		},
		Spec: agenttest.SuiteSpec{
			Execution: agenttest.SuiteExecutionSpec{Profile: "ci-live"},
		},
	}

	if !shouldRunAgentTestSuite(suite, "smoke", "ci-live", false) {
		t.Fatal("expected suite to match tier/profile filters")
	}
	if shouldRunAgentTestSuite(suite, "stable", "ci-live", false) {
		t.Fatal("expected tier mismatch to filter suite out")
	}
	if shouldRunAgentTestSuite(suite, "smoke", "replay", false) {
		t.Fatal("expected profile mismatch to filter suite out")
	}
}

func TestResolveAgentTestLane(t *testing.T) {
	preset, err := resolveAgentTestLane("pr-smoke")
	if err != nil {
		t.Fatalf("resolveAgentTestLane: %v", err)
	}
	if preset.Tier != "smoke" || preset.Profile != "ci-live" || !preset.Strict {
		t.Fatalf("unexpected preset: %+v", preset)
	}

	quarantinedPreset, err := resolveAgentTestLane("quarantined-live")
	if err != nil {
		t.Fatalf("resolveAgentTestLane(quarantined-live): %v", err)
	}
	if quarantinedPreset.Tier != "" || quarantinedPreset.Profile != "ci-live" || !quarantinedPreset.Strict || !quarantinedPreset.IncludeQuarantined {
		t.Fatalf("unexpected quarantined preset: %+v", quarantinedPreset)
	}

	if _, err := resolveAgentTestLane("unknown"); err == nil {
		t.Fatal("expected unknown lane to fail")
	}
}

func TestFilterSuiteCasesByTags(t *testing.T) {
	suite := &agenttest.Suite{
		Spec: agenttest.SuiteSpec{
			Cases: []agenttest.CaseSpec{
				{Name: "one", Prompt: "one", Tags: []string{"smoke", "browser"}},
				{Name: "two", Prompt: "two", Tags: []string{"slow"}},
				{Name: "three", Prompt: "three"},
			},
		},
	}

	filtered := agenttest.FilterSuiteCasesByTags(suite, []string{"browser"})
	if len(filtered.Spec.Cases) != 1 || filtered.Spec.Cases[0].Name != "one" {
		t.Fatalf("unexpected filtered cases: %+v", filtered.Spec.Cases)
	}

	filtered = agenttest.FilterSuiteCasesByTags(suite, []string{"slow", "browser"})
	if len(filtered.Spec.Cases) != 2 {
		t.Fatalf("expected two matching cases, got %d", len(filtered.Spec.Cases))
	}

	filtered = agenttest.FilterSuiteCasesByTags(suite, []string{"missing"})
	if len(filtered.Spec.Cases) != 0 {
		t.Fatalf("expected no matching cases, got %d", len(filtered.Spec.Cases))
	}
}
