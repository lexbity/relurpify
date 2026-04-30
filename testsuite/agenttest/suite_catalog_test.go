package agenttest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepresentativeSuitesLoadCatalog(t *testing.T) {
	for _, root := range findAgentTestsDir(t) {
		files, err := filepath.Glob(filepath.Join(root, "*.yaml"))
		if err != nil {
			t.Fatalf("glob agenttest suites in %s: %v", root, err)
		}
		for _, path := range files {
			if _, err := LoadSuite(path); err != nil {
				t.Fatalf("expected suite %s to load, got %v", path, err)
			}
		}
	}
}

func TestGenericSuiteCatalogMetadata(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "suite.yaml")
	err := os.WriteFile(path, []byte(`
apiVersion: relurpify/v1alpha1
kind: AgentTestSuite
metadata:
  name: generic
  owner: agent-platform
  tier: stable
  classification: capability
spec:
  agent_name: coding
  manifest: relurpify_cfg/agent.manifest.yaml
  execution:
    profile: live
  workspace:
    strategy: derived
  cases:
    - name: smoke
      prompt: summarize
      expect:
        outcome:
          must_succeed: true
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	suite, err := LoadSuite(path)
	if err != nil {
		t.Fatalf("LoadSuite error = %v", err)
	}
	if suite.Metadata.Name != "generic" {
		t.Fatalf("suite.Metadata.Name = %q", suite.Metadata.Name)
	}
	if suite.Metadata.Classification != "capability" {
		t.Fatalf("suite.Metadata.Classification = %q", suite.Metadata.Classification)
	}
}
