package agenttest

import (
	"os"
	"path/filepath"
	"testing"
)

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
	if got := suite.Metadata.Tier; got != "stable" {
		t.Fatalf("Tier = %q", got)
	}
	if got := suite.Spec.Execution.Profile; got != "live" {
		t.Fatalf("Execution.Profile = %q", got)
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

func TestSuiteValidateRejectsUnsupportedMemoryBackend(t *testing.T) {
	suite := &Suite{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentTestSuite",
		Spec: SuiteSpec{
			AgentName: "coding",
			Manifest:  "relurpify_cfg/agent.manifest.yaml",
			Memory:    MemorySpec{Backend: "mystery"},
			Cases: []CaseSpec{{
				Name:   "smoke",
				Prompt: "summarize",
			}},
		},
	}

	if err := suite.Validate(); err == nil {
		t.Fatal("expected unsupported memory backend to fail validation")
	}
}

func TestSuiteValidateRejectsIncompleteWorkflowSeed(t *testing.T) {
	suite := &Suite{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentTestSuite",
		Spec: SuiteSpec{
			AgentName: "coding",
			Manifest:  "relurpify_cfg/agent.manifest.yaml",
			Cases: []CaseSpec{{
				Name:   "smoke",
				Prompt: "summarize",
				Setup: SetupSpec{
					Workflows: []WorkflowSeedSpec{{
						Workflow: WorkflowRecordSeedSpec{},
					}},
				},
			}},
		},
	}

	if err := suite.Validate(); err == nil {
		t.Fatal("expected incomplete workflow seed to fail validation")
	}
}

func TestSuiteValidateRejectsIncompleteWorkflowCheckpointSeed(t *testing.T) {
	suite := &Suite{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentTestSuite",
		Spec: SuiteSpec{
			AgentName: "htn",
			Manifest:  "relurpify_cfg/agents/htn.yaml",
			Cases: []CaseSpec{{
				Name:   "resume",
				Prompt: "resume",
				Setup: SetupSpec{
					Workflows: []WorkflowSeedSpec{{
						Workflow: WorkflowRecordSeedSpec{WorkflowID: "wf-1"},
						Checkpoints: []WorkflowCheckpointSeedSpec{{
							TaskID:    "task-1",
							StageName: "explain.explore",
						}},
					}},
				},
			}},
		},
	}

	if err := suite.Validate(); err == nil {
		t.Fatal("expected incomplete workflow checkpoint seed to fail validation")
	}
}

func TestLoadCanonicalHTNAndRewooSuites(t *testing.T) {
	for _, path := range []string{
		"/home/lex/Public/Relurpify/testsuite/agenttests/htn.testsuite.yaml",
		"/home/lex/Public/Relurpify/testsuite/agenttests/rewoo.testsuite.yaml",
	} {
		if _, err := LoadSuite(path); err != nil {
			t.Fatalf("LoadSuite(%q): %v", path, err)
		}
	}
}

func TestLoadCanonicalRexSuite(t *testing.T) {
	path := "/home/lex/Public/Relurpify/testsuite/agenttests/rex.testsuite.yaml"
	if _, err := LoadSuite(path); err != nil {
		t.Fatalf("LoadSuite(%q): %v", path, err)
	}
}

func TestLoadCanonicalEucloCoverageMatrixSuites(t *testing.T) {
	for _, path := range []string{
		"/home/lex/Public/Relurpify/testsuite/agenttests/euclo.code.testsuite.yaml",
		"/home/lex/Public/Relurpify/testsuite/agenttests/euclo.debug.testsuite.yaml",
		"/home/lex/Public/Relurpify/testsuite/agenttests/euclo.tdd.testsuite.yaml",
		"/home/lex/Public/Relurpify/testsuite/agenttests/euclo.review.testsuite.yaml",
		"/home/lex/Public/Relurpify/testsuite/agenttests/euclo.planning.testsuite.yaml",
		"/home/lex/Public/Relurpify/testsuite/agenttests/euclo.transitions.testsuite.yaml",
	} {
		if _, err := LoadSuite(path); err != nil {
			t.Fatalf("LoadSuite(%q): %v", path, err)
		}
	}
}

func TestSuiteValidateRejectsUnsupportedTier(t *testing.T) {
	suite := &Suite{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentTestSuite",
		Metadata:   SuiteMeta{Name: "coding", Tier: "mystery"},
		Spec: SuiteSpec{
			AgentName: "coding",
			Manifest:  "relurpify_cfg/agent.manifest.yaml",
			Cases: []CaseSpec{{
				Name:   "smoke",
				Prompt: "summarize",
			}},
		},
	}

	if err := suite.Validate(); err == nil {
		t.Fatal("expected unsupported tier to fail validation")
	}
}

func TestSuiteValidateRejectsUnsupportedExecutionProfile(t *testing.T) {
	suite := &Suite{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentTestSuite",
		Metadata:   SuiteMeta{Name: "coding"},
		Spec: SuiteSpec{
			AgentName: "coding",
			Manifest:  "relurpify_cfg/agent.manifest.yaml",
			Execution: SuiteExecutionSpec{Profile: "mystery"},
			Cases: []CaseSpec{{
				Name:   "smoke",
				Prompt: "summarize",
			}},
		},
	}

	if err := suite.Validate(); err == nil {
		t.Fatal("expected unsupported execution profile to fail validation")
	}
}

func TestSuiteValidateRejectsInvalidExecutionTimeout(t *testing.T) {
	suite := &Suite{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentTestSuite",
		Metadata:   SuiteMeta{Name: "coding"},
		Spec: SuiteSpec{
			AgentName: "coding",
			Manifest:  "relurpify_cfg/agent.manifest.yaml",
			Execution: SuiteExecutionSpec{Timeout: "nope"},
			Cases: []CaseSpec{{
				Name:   "smoke",
				Prompt: "summarize",
			}},
		},
	}

	if err := suite.Validate(); err == nil {
		t.Fatal("expected invalid execution timeout to fail validation")
	}
}

func TestSuiteValidateRejectsInteractionScriptStepWithoutAction(t *testing.T) {
	suite := &Suite{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentTestSuite",
		Metadata:   SuiteMeta{Name: "euclo-transitions"},
		Spec: SuiteSpec{
			AgentName: "euclo",
			Manifest:  "relurpify_cfg/agent.manifest.yaml",
			Cases: []CaseSpec{{
				Name:   "missing-action",
				Prompt: "plan and implement the change",
				InteractionScript: []InteractionScriptStep{{
					Phase: "understand",
				}},
			}},
		},
	}

	if err := suite.Validate(); err == nil {
		t.Fatal("expected interaction script step without action to fail validation")
	}
}

func TestSuiteIsStrictRunForCIProfiles(t *testing.T) {
	suite := &Suite{
		Spec: SuiteSpec{
			Execution: SuiteExecutionSpec{Profile: "ci-live"},
		},
	}
	if !suite.IsStrictRun("", false) {
		t.Fatal("expected ci-live profile to imply strict mode")
	}
}

func TestLoadSuiteRejectsUnknownFields(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "suite.yaml")
	err := os.WriteFile(path, []byte(`
apiVersion: relurpify/v1alpha1
kind: AgentTestSuite
metadata:
  name: sample
spec:
  agent_name: coding
  manifest: relurpify_cfg/agent.manifest.yaml
  unknown_field: true
  cases:
    - name: smoke
      prompt: summarize
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := LoadSuite(path); err == nil {
		t.Fatal("expected unknown field to fail load")
	}
}

func TestLoadSuiteRejectsUnknownEucloExpectationFields(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "suite.yaml")
	err := os.WriteFile(path, []byte(`
apiVersion: relurpify/v1alpha1
kind: AgentTestSuite
metadata:
  name: sample
spec:
  agent_name: euclo
  manifest: relurpify_cfg/agent.manifest.yaml
  cases:
    - name: smoke
      prompt: summarize
      expect:
        euclo:
          mode: code
          transitions_accepted: 1
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := LoadSuite(path); err == nil {
		t.Fatal("expected unknown expect.euclo field to fail load")
	}
}

func TestSuiteValidateRejectsUnsupportedRecordingMode(t *testing.T) {
	suite := &Suite{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentTestSuite",
		Metadata:   SuiteMeta{Name: "coding"},
		Spec: SuiteSpec{
			AgentName: "coding",
			Manifest:  "relurpify_cfg/agent.manifest.yaml",
			Recording: RecordingSpec{Mode: "mystery"},
			Cases: []CaseSpec{{
				Name:   "smoke",
				Prompt: "summarize",
			}},
		},
	}

	if err := suite.Validate(); err == nil {
		t.Fatal("expected unsupported recording mode to fail validation")
	}
}

func TestSuiteValidateRejectsUnsupportedRecordingStrategy(t *testing.T) {
	suite := &Suite{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentTestSuite",
		Metadata:   SuiteMeta{Name: "coding"},
		Spec: SuiteSpec{
			AgentName: "coding",
			Manifest:  "relurpify_cfg/agent.manifest.yaml",
			Recording: RecordingSpec{Strategy: "mystery"},
			Cases: []CaseSpec{{
				Name:   "smoke",
				Prompt: "summarize",
			}},
		},
	}

	if err := suite.Validate(); err == nil {
		t.Fatal("expected unsupported recording strategy to fail validation")
	}
}

func TestSuiteValidateRejectsUnsupportedControlFlowOverride(t *testing.T) {
	suite := &Suite{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentTestSuite",
		Metadata:   SuiteMeta{Name: "coding"},
		Spec: SuiteSpec{
			AgentName: "coding",
			Manifest:  "relurpify_cfg/agent.manifest.yaml",
			Cases: []CaseSpec{{
				Name:   "smoke",
				Prompt: "summarize",
				Overrides: CaseOverrideSpec{
					ControlFlow: "pipeline",
				},
			}},
		},
	}

	if err := suite.Validate(); err == nil {
		t.Fatal("expected unsupported control_flow override to fail validation")
	}
}

func TestSuiteValidateRejectsInvalidBootstrapTimeoutOverride(t *testing.T) {
	suite := &Suite{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentTestSuite",
		Metadata:   SuiteMeta{Name: "coding"},
		Spec: SuiteSpec{
			AgentName: "coding",
			Manifest:  "relurpify_cfg/agent.manifest.yaml",
			Cases: []CaseSpec{{
				Name:   "smoke",
				Prompt: "summarize",
				Overrides: CaseOverrideSpec{
					BootstrapTimeout: "nope",
				},
			}},
		},
	}

	if err := suite.Validate(); err == nil {
		t.Fatal("expected invalid bootstrap_timeout override to fail validation")
	}
}

func TestSuiteValidateRejectsArtifactChainWithoutPhase(t *testing.T) {
	suite := &Suite{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentTestSuite",
		Metadata:   SuiteMeta{Name: "euclo"},
		Spec: SuiteSpec{
			AgentName: "euclo",
			Manifest:  "relurpify_cfg/agent.manifest.yaml",
			Cases: []CaseSpec{{
				Name:   "artifact_chain",
				Prompt: "plan a change",
				Expect: ExpectSpec{
					Euclo: &EucloExpectSpec{
						ArtifactChain: []ArtifactChainSpec{{
							Kind: "plan",
						}},
					},
				},
			}},
		},
	}

	if err := suite.Validate(); err == nil {
		t.Fatal("expected invalid artifact_chain to fail validation")
	}
}

func TestSuiteValidateRejectsInvalidSetupFileMode(t *testing.T) {
	suite := &Suite{
		APIVersion: "relurpify/v1alpha1",
		Kind:       "AgentTestSuite",
		Metadata:   SuiteMeta{Name: "coding"},
		Spec: SuiteSpec{
			AgentName: "coding",
			Manifest:  "relurpify_cfg/agent.manifest.yaml",
			Cases: []CaseSpec{{
				Name:   "smoke",
				Prompt: "summarize",
				Setup: SetupSpec{
					Files: []SetupFileSpec{{
						Path:    "hello.txt",
						Content: "hello",
						Mode:    "not-octal",
					}},
				},
			}},
		},
	}

	if err := suite.Validate(); err == nil {
		t.Fatal("expected invalid setup file mode to fail validation")
	}
}
