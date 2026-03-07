package agenttest

import "testing"

func TestSuiteValidateDefaultsDerivedWorkspaceSettings(t *testing.T) {
	suite := &Suite{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentTestSuite",
		Spec: SuiteSpec{
			AgentName: "coding",
			Manifest:  "relurpify_cfg/agent.manifest.yaml",
			Cases: []CaseSpec{{
				Name:   "smoke",
				Prompt: "summarize",
			}},
		},
	}

	if err := suite.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if got := suite.Spec.Workspace.Strategy; got != "derived" {
		t.Fatalf("Strategy = %q", got)
	}
	if got := suite.Spec.Workspace.TemplateProfile; got != "default" {
		t.Fatalf("TemplateProfile = %q", got)
	}
}

func TestSuiteValidateRejectsLegacyWorkspaceStrategies(t *testing.T) {
	suite := &Suite{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentTestSuite",
		Spec: SuiteSpec{
			AgentName: "coding",
			Manifest:  "relurpify_cfg/agent.manifest.yaml",
			Workspace: WorkspaceSpec{Strategy: "copy"},
			Cases: []CaseSpec{{
				Name:   "smoke",
				Prompt: "summarize",
			}},
		},
	}

	if err := suite.Validate(); err == nil {
		t.Fatal("expected Validate() to reject legacy workspace strategy")
	}
}
