package agenttest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAgentTestSuitesLoad(t *testing.T) {
	testDirs := findAgentTestsDir(t)

	var suiteFiles []string
	for _, testDir := range testDirs {
		files, err := filepath.Glob(filepath.Join(testDir, "*.yaml"))
		if err == nil && len(files) > 0 {
			suiteFiles = append(suiteFiles, files...)
		}
	}
	if len(suiteFiles) == 0 {
		t.Fatalf("no suite files found in any of the search paths: %v", testDirs)
	}

	for _, suitePath := range suiteFiles {
		if _, err := os.Stat(suitePath); err != nil {
			t.Fatalf("suite file missing: %s: %v", suitePath, err)
		}
		suite, err := LoadSuite(suitePath)
		if err != nil {
			t.Fatalf("failed to load suite %s: %v", suitePath, err)
		}
		if len(suite.Spec.Cases) == 0 {
			t.Fatalf("suite %s has no cases", suitePath)
		}
	}
}

func findAgentTestsDir(t *testing.T) []string {
	t.Helper()
	var testDirs []string
	possiblePaths := []string{
		filepath.Join("..", "agenttests"),
		filepath.Join("..", "..", "testsuite", "agenttests"),
		"agenttests",
	}
	for _, path := range possiblePaths {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			testDirs = append(testDirs, path)
		}
	}
	if len(testDirs) == 0 {
		t.Fatalf("no agenttests directory found in any of: %v", possiblePaths)
	}
	return testDirs
}
