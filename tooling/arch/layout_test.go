package arch

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDomainLayout(t *testing.T) {
	// Verify that every expected top-level domain directory exists and has a doc.go
	domains := map[string]string{
		"capability": "what can an agent do?",
		"context":    "what does the agent know, and what flows between steps?",
		"execution":  "how does one agent run?",
		"governance": "what rules bound the run, and who is acting?",
		"jobs":       "how is deferred or long-running work durably scheduled",
		"model":      "the model abstraction",
		"telemetry":  "observation of a run",
		"userconfig": "the curated control surface",
	}

	for domain := range domains {
		dir := filepath.Join("..", "..", domain)
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("domain directory %s: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("domain path %s is not a directory", dir)
			continue
		}

		doc := filepath.Join(dir, "doc.go")
		if _, err := os.Stat(doc); err != nil {
			t.Errorf("domain %s missing doc.go: %v", domain, err)
		}

		// Verify no framework/ directory remains
		if _, err := os.Stat(filepath.Join("..", "..", "framework")); err == nil {
			t.Error("framework/ directory still exists; tree relocation incomplete")
		}
	}
}

func TestNoFrameworkPackage(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("..", "..", "framework", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) > 0 {
		t.Errorf("framework/ still contains %d entries; tree relocation incomplete", len(matches))
	}
}
