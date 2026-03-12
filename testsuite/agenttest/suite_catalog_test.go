package agenttest

import (
	"path/filepath"
	"testing"
)

func TestLoadAllCommittedSuites(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "agenttests", "*.testsuite.yaml"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("expected committed agent suites")
	}

	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			suite, err := LoadSuite(path)
			if err != nil {
				t.Fatalf("LoadSuite(%q) error = %v", path, err)
			}
			if suite.Metadata.Name == "" {
				t.Fatalf("suite %q missing metadata.name after load", path)
			}
			if len(suite.Spec.Cases) == 0 {
				t.Fatalf("suite %q missing cases after load", path)
			}
		})
	}
}
