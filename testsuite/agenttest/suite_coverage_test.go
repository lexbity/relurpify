package agenttest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/named/euclo/relurpicabilities"
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

			// Phase 8: Updated to use Benchmark.Euclo
			if c.Expect.Benchmark != nil && c.Expect.Benchmark.Euclo != nil {
				euclo := c.Expect.Benchmark.Euclo
				// Primary capability
				if euclo.PrimaryRelurpicCapability != "" {
					coveredCapabilities[euclo.PrimaryRelurpicCapability] = true
				}

				// Supporting capabilities
				for _, supp := range euclo.SupportingRelurpicCapabilities {
					coveredCapabilities[supp] = true
				}

				// Specialized capabilities
				for _, spec := range euclo.SpecializedCapabilityIDs {
					coveredCapabilities[spec] = true
				}
			}
		}
	}

	// Find uncovered capabilities
	var uncovered []string
	for _, capID := range allCapabilityIDs {
		if coveredCapabilities[capID] {
			continue
		}
		// BKC capabilities (SupportingOnly + ArchaeoAssociated) require the Archaeo
		// service and GraphDB unavailable in CI. They are covered via full-flow
		// archaeology.explore interaction tests rather than direct capability invocation.
		if desc, ok := registry.Lookup(capID); ok && desc.SupportingOnly && desc.ArchaeoAssociated {
			continue
		}
		uncovered = append(uncovered, capID)
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
			// Phase 8: Updated to use Benchmark.Euclo
			if c.Expect.Benchmark != nil && c.Expect.Benchmark.Euclo != nil &&
				c.Expect.Benchmark.Euclo.PrimaryRelurpicCapability != "" {
				primaryCases[c.Expect.Benchmark.Euclo.PrimaryRelurpicCapability] = true
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
//
// Exception: capabilities that are both SupportingOnly and ArchaeoAssociated (BKC
// capabilities) require the Archaeo service and GraphDB which are unavailable in CI.
// These are covered via full archaeology.explore interaction flow tests instead.
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
		// BKC capabilities (SupportingOnly + ArchaeoAssociated) require the Archaeo service
		// and GraphDB unavailable in CI. They are covered via full-flow archaeology tests.
		if desc.ArchaeoAssociated {
			continue
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

// TestNoFilesContainOnVerifiedCases warns if a suite case keeps legacy
// files_contain assertions after verify steps have been added.
func TestNoFilesContainOnVerifiedCases(t *testing.T) {
	testDirs := findAgentTestsDir(t)
	var suiteFiles []string
	for _, testDir := range testDirs {
		files, err := filepath.Glob(filepath.Join(testDir, "euclo.*.testsuite.yaml"))
		if err == nil && len(files) > 0 {
			suiteFiles = append(suiteFiles, files...)
		}
	}

	var warnings []string
	for _, suitePath := range suiteFiles {
		suite, err := LoadSuite(suitePath)
		if err != nil {
			continue
		}
		for _, c := range suite.Spec.Cases {
			hasVerify := c.Expect.Outcome != nil && c.Expect.Outcome.Verify != nil &&
				(len(c.Expect.Outcome.Verify.Steps) > 0 || c.Expect.Outcome.Verify.Script != "")
			if !hasVerify || len(c.Expect.FilesContain) == 0 {
				continue
			}
			warnings = append(warnings, suitePath+": case "+c.Name+" keeps files_contain alongside verify")
		}
	}

	if len(warnings) > 0 {
		t.Logf("files_contain + verify coexistence warnings:\n%s", strings.Join(warnings, "\n"))
	}
}

// TestBKCCasesUseFullFlow verifies that BKC test cases use the full agent interaction
// flow rather than capability_direct_run. BKC capabilities require the Archaeo service
// and GraphDB which are unavailable in CI, so they are covered via full archaeology.explore
// end-to-end tests instead of isolated capability invocation.
func TestBKCCasesUseFullFlow(t *testing.T) {
	suitePath := filepath.Join("..", "agenttests", "euclo.baseline.bkc.testsuite.yaml")

	if _, err := os.Stat(suitePath); os.IsNotExist(err) {
		t.Skip("BKC suite not found")
	}

	suite, err := LoadSuite(suitePath)
	if err != nil {
		t.Fatalf("failed to load BKC suite: %v", err)
	}

	for _, c := range suite.Spec.Cases {
		if c.CapabilityDirectRun != nil {
			t.Errorf("BKC case %q must not use capability_direct_run; use full interaction flow instead", c.Name)
		}
	}

	if len(suite.Spec.Cases) < 2 {
		t.Errorf("expected at least 2 BKC full-flow test cases, got %d", len(suite.Spec.Cases))
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
