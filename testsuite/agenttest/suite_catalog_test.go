package agenttest

import (
	"os"
	"path/filepath"
	"strings"
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
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile(%q) error = %v", path, err)
			}
			raw := string(data)
			for _, needle := range []string{
				"owner:",
				"tier:",
				"quarantined:",
				"execution:",
				"profile:",
				"strict:",
			} {
				if !strings.Contains(raw, needle) {
					t.Fatalf("suite %q must declare %q explicitly in YAML", path, needle)
				}
			}
			suite, err := LoadSuite(path)
			if err != nil {
				t.Fatalf("LoadSuite(%q) error = %v", path, err)
			}
			if suite.Metadata.Name == "" {
				t.Fatalf("suite %q missing metadata.name after load", path)
			}
			if strings.TrimSpace(suite.Metadata.Owner) == "" {
				t.Fatalf("suite %q missing metadata.owner after load", path)
			}
			if len(suite.Spec.Cases) == 0 {
				t.Fatalf("suite %q missing cases after load", path)
			}
		})
	}
}
