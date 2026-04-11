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

func TestLoadEucloSuitesIncludeClassification(t *testing.T) {
	cases := map[string]string{
		filepath.Join("..", "agenttests", "euclo.code.testsuite.yaml"):                "capability",
		filepath.Join("..", "agenttests", "euclo.archaeology.testsuite.yaml"):         "journey",
		filepath.Join("..", "agenttests", "euclo.intent.journey.testsuite.yaml"):      "journey",
		filepath.Join("..", "agenttests", "euclo.performance_context.testsuite.yaml"): "benchmark",
	}
	for path, want := range cases {
		suite, err := LoadSuite(path)
		if err != nil {
			t.Fatalf("LoadSuite(%q) error = %v", path, err)
		}
		if got := suite.Metadata.Classification; got != want {
			t.Fatalf("suite %q classification = %q, want %q", path, got, want)
		}
	}
}

func TestLoadBenchmarkSuitesIncludeScoringMetadata(t *testing.T) {
	suite, err := LoadSuite(filepath.Join("..", "agenttests", "euclo.performance_context.testsuite.yaml"))
	if err != nil {
		t.Fatalf("LoadSuite error = %v", err)
	}
	if got := suite.Metadata.Benchmark.ScoreFamily; got != "context-pressure" {
		t.Fatalf("unexpected benchmark score family %q", got)
	}
	if got := suite.Metadata.Benchmark.ComparisonWindow; got != "suite" {
		t.Fatalf("unexpected benchmark comparison window %q", got)
	}
}
