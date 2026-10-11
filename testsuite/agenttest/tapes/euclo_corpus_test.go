package tapes

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/testsuite/agenttest"
)

// The corpus gate is general, not per-tape: every committed lineage under
// testsuite/agenttests/tapes/<suite>/ is cross-checked against the suite YAML
// it names. A lineage must name a case that exists in its suite (the hardcoded
// per-tape assertion this test replaces could not notice a rename), its status
// must be a known value, and the status must agree with reality on disk: a
// healthy lineage owns a replayable tape, a stale lineage owns nothing.
//
// Committed statuses (wave 4 D12): "" (healthy) and
// "stale-awaiting-rerecord" (tape deleted on fingerprint mismatch; replay
// coverage explicitly zero until re-recorded). Anything else fails here.
const (
	lineageStatusHealthy = ""
	lineageStatusStale   = "stale-awaiting-rerecord"
)

func TestCommittedLineageCorpus(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")
	tapesDir := filepath.Join(repoRoot, "testsuite", "agenttests", "tapes")
	lineages, err := filepath.Glob(filepath.Join(tapesDir, "*", "*.lineage.json"))
	if err != nil {
		t.Fatalf("glob committed lineages: %v", err)
	}
	if len(lineages) == 0 {
		t.Fatal("no committed lineages found under testsuite/agenttests/tapes; the corpus gate has nothing to enforce")
	}
	for _, lineagePath := range lineages {
		t.Run(filepath.Base(lineagePath), func(t *testing.T) {
			checkCommittedLineage(t, repoRoot, lineagePath)
		})
	}
}

func checkCommittedLineage(t *testing.T, repoRoot, lineagePath string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(lineagePath))
	if err != nil {
		t.Fatalf("read lineage: %v", err)
	}
	var record PromotionLineageRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("decode lineage: %v", err)
	}
	if strings.TrimSpace(record.SuitePath) == "" {
		t.Fatal("lineage suite_path is empty")
	}
	suiteAbs := filepath.Join(repoRoot, filepath.FromSlash(record.SuitePath))
	suite, err := agenttest.LoadSuite(filepath.Clean(suiteAbs))
	if err != nil {
		t.Fatalf("load lineage suite %q: %v", record.SuitePath, err)
	}
	// Membership: the lineage's case must exist in the suite the lineage
	// names. A lineage whose case was renamed or deleted is a lie about
	// coverage regardless of what sits on disk.
	casePresent := false
	for _, c := range suite.Spec.Cases {
		if c.Name == record.CaseName {
			casePresent = true
			break
		}
	}
	if !casePresent {
		t.Fatalf("lineage case_name %q is not a case in suite %q", record.CaseName, record.SuitePath)
	}

	tapeRel := filepath.FromSlash(record.DestinationTape)
	tapeAbs := filepath.Join(repoRoot, tapeRel)
	switch record.Status {
	case lineageStatusHealthy:
		inspection, err := llm.InspectTape(tapeAbs)
		if err != nil {
			t.Fatalf("healthy lineage tape %q does not inspect: %v", tapeRel, err)
		}
		if inspection.Header != nil &&
			strings.TrimSpace(inspection.Header.CaseName) != "" &&
			strings.TrimSpace(inspection.Header.CaseName) != strings.TrimSpace(record.CaseName) {
			t.Fatalf("tape header case %q does not match lineage case_name %q",
				inspection.Header.CaseName, record.CaseName)
		}
	case lineageStatusStale:
		if _, err := os.Stat(tapeAbs); err == nil {
			t.Fatalf("lineage is marked %q but its tape %q exists; re-record or delete the tape", lineageStatusStale, tapeRel)
		}
	default:
		t.Fatalf("lineage status %q is not a known committed status (%q or %q)",
			record.Status, lineageStatusHealthy, lineageStatusStale)
	}
}
