package agenttest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
)

// TestAllRegisteredCapabilitiesHaveTestCases verifies that every capability ID
// registered in DefaultRegistry() appears in at least one test case.
// This test fails at build time if a new capability is registered but has no test.
// Phase 5: Coverage gap detector (Spec 5.3)
func TestAllRegisteredCapabilitiesHaveTestCases(t *testing.T) {
	// Collect all capability IDs from the registry
	registry := relurpicabilities.DefaultRegistry()
	allCapabilityIDs := registry.IDs()

	// Load all YAML suite files
	// Try multiple possible paths for test directory
	var testDirs []string
	if _, err := os.Stat(filepath.Join("..", "agenttests")); err == nil {
		testDirs = append(testDirs, filepath.Join("..", "agenttests"))
	}
	if _, err := os.Stat(filepath.Join("..", "..", "testsuite", "agenttests")); err == nil {
		testDirs = append(testDirs, filepath.Join("..", "..", "testsuite", "agenttests"))
	}
	if _, err := os.Stat("agenttests"); err == nil {
		testDirs = append(testDirs, "agenttests")
	}

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
	t.Logf("found %d suite files", len(suiteFiles))

	// Track which capabilities are covered by test cases
	coveredCapabilities := make(map[string]bool)

	for _, suitePath := range suiteFiles {
		suite, err := LoadSuite(suitePath)
		if err != nil {
			t.Logf("warning: failed to load suite %s: %v", suitePath, err)
			continue
		}

		for _, c := range suite.Spec.Cases {
			// Check capability_direct_run
			if c.CapabilityDirectRun != nil {
				coveredCapabilities[c.CapabilityDirectRun.CapabilityID] = true
			}

			// Check euclo expectations
			if c.Expect.Euclo != nil {
				// Primary capability
				if c.Expect.Euclo.PrimaryRelurpicCapability != "" {
					coveredCapabilities[c.Expect.Euclo.PrimaryRelurpicCapability] = true
				}

				// Supporting capabilities
				for _, supp := range c.Expect.Euclo.SupportingRelurpicCapabilities {
					coveredCapabilities[supp] = true
				}

				// Specialized capabilities
				for _, spec := range c.Expect.Euclo.SpecializedCapabilityIDs {
					coveredCapabilities[spec] = true
				}
			}
		}
	}

	// Find uncovered capabilities
	var uncovered []string
	for _, capID := range allCapabilityIDs {
		if !coveredCapabilities[capID] {
			uncovered = append(uncovered, capID)
		}
	}

	if len(uncovered) > 0 {
		t.Errorf("coverage gap detected: %d registered capabilities have no test cases:\n  - %s",
			len(uncovered), strings.Join(uncovered, "\n  - "))
		t.Log("Add test cases to testsuite/agenttests/euclo.*.testsuite.yaml or use capability_direct_run for supporting-only capabilities")
	}
}

// findAgentTestsDir discovers the agenttests directory from various possible locations.
func findAgentTestsDir(t *testing.T) []string {
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

// TestPrimaryCapabilitiesHaveDedicatedCases verifies that primary capabilities
// (non-supporting-only) have at least one dedicated test case as the primary.
func TestPrimaryCapabilitiesHaveDedicatedCases(t *testing.T) {
	registry := relurpicabilities.DefaultRegistry()

	testDirs := findAgentTestsDir(t)
	var suiteFiles []string
	for _, testDir := range testDirs {
		files, err := filepath.Glob(filepath.Join(testDir, "*.yaml"))
		if err == nil && len(files) > 0 {
			suiteFiles = append(suiteFiles, files...)
		}
	}

	// Track which primary capabilities have dedicated test cases
	primaryCases := make(map[string]bool)

	for _, suitePath := range suiteFiles {
		suite, err := LoadSuite(suitePath)
		if err != nil {
			continue
		}

		for _, c := range suite.Spec.Cases {
			if c.Expect.Euclo != nil && c.Expect.Euclo.PrimaryRelurpicCapability != "" {
				primaryCases[c.Expect.Euclo.PrimaryRelurpicCapability] = true
			}

			// capability_direct_run without invoking_primary counts as primary
			if c.CapabilityDirectRun != nil && c.CapabilityDirectRun.InvokingPrimary == "" {
				desc, ok := registry.Lookup(c.CapabilityDirectRun.CapabilityID)
				if ok && !desc.SupportingOnly {
					primaryCases[c.CapabilityDirectRun.CapabilityID] = true
				}
			}
		}
	}

	// Find primary capabilities without dedicated test cases
	var missing []string
	for _, capID := range registry.IDs() {
		desc, ok := registry.Lookup(capID)
		if !ok {
			continue
		}
		if desc.SupportingOnly {
			continue // Skip supporting-only capabilities
		}
		if !primaryCases[capID] {
			missing = append(missing, capID)
		}
	}

	if len(missing) > 0 {
		t.Errorf("primary capabilities without dedicated test cases:\n  - %s",
			strings.Join(missing, "\n  - "))
	}
}

// TestSupportingCapabilitiesHaveIsolationCases verifies that supporting-only
// capabilities have isolation test cases using capability_direct_run.
func TestSupportingCapabilitiesHaveIsolationCases(t *testing.T) {
	registry := relurpicabilities.DefaultRegistry()

	testDirs := findAgentTestsDir(t)
	var suiteFiles []string
	for _, testDir := range testDirs {
		files, err := filepath.Glob(filepath.Join(testDir, "*.yaml"))
		if err == nil && len(files) > 0 {
			suiteFiles = append(suiteFiles, files...)
		}
	}

	// Track which supporting capabilities have isolation tests
	supportingCases := make(map[string]bool)

	for _, suitePath := range suiteFiles {
		suite, err := LoadSuite(suitePath)
		if err != nil {
			continue
		}

		for _, c := range suite.Spec.Cases {
			// Direct run of a supporting capability counts as isolation test
			if c.CapabilityDirectRun != nil {
				desc, ok := registry.Lookup(c.CapabilityDirectRun.CapabilityID)
				if ok && desc.SupportingOnly {
					supportingCases[c.CapabilityDirectRun.CapabilityID] = true
				}
			}

			// Direct run with invoking_primary for a supporting capability
			if c.CapabilityDirectRun != nil && c.CapabilityDirectRun.InvokingPrimary != "" {
				desc, ok := registry.Lookup(c.CapabilityDirectRun.CapabilityID)
				if ok && desc.SupportingOnly {
					supportingCases[c.CapabilityDirectRun.CapabilityID] = true
				}
			}
		}
	}

	// Find supporting-only capabilities without isolation tests
	var missing []string
	for _, capID := range registry.IDs() {
		desc, ok := registry.Lookup(capID)
		if !ok {
			continue
		}
		if !desc.SupportingOnly {
			continue // Only check supporting-only
		}
		if !supportingCases[capID] {
			missing = append(missing, capID)
		}
	}

	if len(missing) > 0 {
		t.Errorf("supporting-only capabilities without isolation test cases:\n  - %s\n\nAdd cases using capability_direct_run to euclo.baseline.supporting.testsuite.yaml",
			strings.Join(missing, "\n  - "))
	}
}

// TestSuiteFilesAreLoadable verifies that all YAML suite files can be loaded and validated.
func TestSuiteFilesAreLoadable(t *testing.T) {
	testDirs := findAgentTestsDir(t)
	var suiteFiles []string
	for _, testDir := range testDirs {
		files, err := filepath.Glob(filepath.Join(testDir, "*.yaml"))
		if err == nil && len(files) > 0 {
			suiteFiles = append(suiteFiles, files...)
		}
	}

	if len(suiteFiles) == 0 {
		t.Log("no suite files found, skipping")
		return
	}

	for _, suitePath := range suiteFiles {
		_, err := LoadSuite(suitePath)
		if err != nil {
			t.Errorf("failed to load suite %s: %v", suitePath, err)
		}
	}
}

// TestBKCCasesUseDirectRun verifies that BKC test cases use capability_direct_run.
func TestBKCCasesUseDirectRun(t *testing.T) {
	suitePath := filepath.Join("..", "agenttests", "euclo.baseline.bkc.testsuite.yaml")

	if _, err := os.Stat(suitePath); os.IsNotExist(err) {
		t.Skip("BKC suite not found")
	}

	suite, err := LoadSuite(suitePath)
	if err != nil {
		t.Fatalf("failed to load BKC suite: %v", err)
	}

	for _, c := range suite.Spec.Cases {
		if c.CapabilityDirectRun == nil {
			t.Errorf("BKC case %q should use capability_direct_run", c.Name)
		}
	}

	if len(suite.Spec.Cases) != 4 {
		t.Errorf("expected 4 BKC test cases, got %d", len(suite.Spec.Cases))
	}
}

// TestSupportingCasesUseDirectRun verifies that supporting capability test cases
// use capability_direct_run.
func TestSupportingCasesUseDirectRun(t *testing.T) {
	suitePath := filepath.Join("..", "agenttests", "euclo.baseline.supporting.testsuite.yaml")

	if _, err := os.Stat(suitePath); os.IsNotExist(err) {
		t.Skip("supporting suite not found")
	}

	suite, err := LoadSuite(suitePath)
	if err != nil {
		t.Fatalf("failed to load supporting suite: %v", err)
	}

	directRunCount := 0
	for _, c := range suite.Spec.Cases {
		if c.CapabilityDirectRun != nil {
			directRunCount++
		}
	}

	if directRunCount != len(suite.Spec.Cases) {
		t.Errorf("expected all %d supporting cases to use capability_direct_run, got %d",
			len(suite.Spec.Cases), directRunCount)
	}
}
