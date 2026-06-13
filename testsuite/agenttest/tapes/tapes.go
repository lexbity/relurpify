// Package tapes provides shared golden tape promotion, coverage, and rerecord planning.
package tapes

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/platform/fs"
	"codeburg.org/lexbit/relurpify/platform/llm"
	"codeburg.org/lexbit/relurpify/testsuite/agenttest"
)

const staleTapeThreshold = 30 * 24 * time.Hour

const (
	suiteClassificationBenchmark  = "benchmark"
	tapeEntryMissingStatus        = "missing"
	tapeEntryStaleStatus          = "stale"
	tapeEntryLegacyStatus         = "legacy"
	tapeEntryFreshStatus          = "fresh"
	tapeStatusRecordedUnknown     = "recorded unknown"
	baselineFreshStatus           = "baseline fresh"
	baselineStaleStatus           = "baseline stale"
	baselineMissingStatus         = "baseline missing"
	baselineInvalidStatus         = "baseline invalid"
	baselineRecordedUnknownStatus = "baseline recorded unknown"
	benchmarkReportFilename       = "benchmark_report.json"
	benchmarkScoreFilename        = "benchmark_score.json"
	benchmarkComparisonFilename   = "benchmark_comparison.json"
)

// PromotionReport captures the artifacts promoted from a completed run.
type PromotionReport struct {
	Workspace      string
	SuitePath      string
	SuiteName      string
	RunDir         string
	Classification string
	Cases          []PromotionCaseReport
}

// PromotionCaseReport describes one promoted case/model pair.
type PromotionCaseReport struct {
	CaseName                       string
	ModelName                      string
	SourceRunDir                   string
	SourceArtifactsDir             string
	SourceTapePath                 string
	DestinationTapePath            string
	SourceInteractionTapePath      string
	DestinationInteractionTapePath string
	SourceBaselinePath             string
	DestinationBaselinePath        string
	LineagePath                    string
	PromotedArtifacts              []string
	RecordedAt                     time.Time
}

// CoverageReport summarizes golden tape coverage for one or more suites.
type CoverageReport struct {
	Workspace   string
	GeneratedAt time.Time
	Suites      []SuiteCoverage
	Missing     []CoverageEntry
	Stale       []CoverageEntry
	Totals      CoverageTotals
}

// SuiteCoverage groups coverage entries for a suite file.
type SuiteCoverage struct {
	SuiteName    string
	SuitePath    string
	Entries      []CoverageEntry
	PresentTapes int
	MissingTapes int
	StaleTapes   int
}

// CoverageEntry describes one suite/case/model tape fixture.
type CoverageEntry struct {
	SuiteName          string
	SuitePath          string
	CaseName           string
	ModelName          string
	TapePath           string
	BaselinePath       string
	Present            bool
	Legacy             bool
	RecordedAt         time.Time
	Status             string
	BaselineStatus     string
	BaselinePresent    bool
	BaselineRecordedAt time.Time
	Stale              bool
}

// CoverageTotals aggregates the report level counts.
type CoverageTotals struct {
	Suites  int
	Cases   int
	Present int
	Missing int
	Stale   int
}

// RerecordPlan captures the coverage gaps that should be re-recorded.
type RerecordPlan struct {
	Workspace   string
	GeneratedAt time.Time
	Entries     []RerecordEntry
	Totals      RerecordTotals
}

// RerecordEntry is one actionable rerun target.
type RerecordEntry struct {
	Entry            CoverageEntry
	Reason           string
	SuggestedCommand string
}

// RerecordTotals aggregates actionable rerecord targets.
type RerecordTotals struct {
	Entries int
	Missing int
	Stale   int
}

// PromoteAllowed returns whether the suite classification is allowed for tape promotion.
func PromoteAllowed(classification string) bool {
	switch strings.TrimSpace(classification) {
	case "", "capability", "journey", suiteClassificationBenchmark:
		return true
	default:
		return false
	}
}

// PromotionLineageFilename returns the canonical lineage filename for a promoted tape.
func PromotionLineageFilename(caseName, modelName string) string {
	return sanitizeName(caseName) + "__" + sanitizeName(modelName) + ".lineage.json"
}

// PromotedArtifacts returns the canonical artifact names copied for a promoted case.
func PromotedArtifacts(classification string, cr agenttest.CaseReport) []string {
	artifacts := []string{"tape.jsonl"}
	if _, err := os.Stat(filepath.Join(cr.ArtifactsDir, "interaction.tape.jsonl")); err == nil {
		artifacts = append(artifacts, "interaction.tape.jsonl")
	}
	switch strings.ToLower(strings.TrimSpace(classification)) {
	case suiteClassificationBenchmark:
		artifacts = append(artifacts, agenttest.GoldenBaselineFilename(cr.Name, cr.Model))
		if _, err := os.Stat(filepath.Join(cr.ArtifactsDir, benchmarkReportFilename)); err == nil {
			artifacts = append(artifacts, benchmarkReportFilename)
		}
		if _, err := os.Stat(filepath.Join(cr.ArtifactsDir, benchmarkScoreFilename)); err == nil {
			artifacts = append(artifacts, benchmarkScoreFilename)
		}
		if _, err := os.Stat(filepath.Join(cr.ArtifactsDir, benchmarkComparisonFilename)); err == nil {
			artifacts = append(artifacts, benchmarkComparisonFilename)
		}
	default:
		artifacts = append(artifacts, agenttest.GoldenBaselineFilename(cr.Name, cr.Model))
	}
	return uniqueStrings(artifacts)
}

// PromoteRun copies the golden tape and related artifacts for one or more run cases.
func PromoteRun(workspace, suitePath, runDir, caseName string, all bool) (*PromotionReport, error) {
	suite, err := agenttest.LoadSuite(suitePath)
	if err != nil {
		return nil, err
	}
	report, err := loadSuiteReport(filepath.Join(runDir, "report.json"))
	if err != nil {
		return nil, err
	}
	targetCases := selectPromotableCases(report, caseName, all)
	if len(targetCases) == 0 {
		return nil, fmt.Errorf("no promotable cases found in run %s", runDir)
	}
	if !PromoteAllowed(suite.Metadata.Classification) {
		return nil, fmt.Errorf("suite classification %q is not promotable", suite.Metadata.Classification)
	}
	result := &PromotionReport{
		Workspace:      strings.TrimSpace(workspace),
		SuitePath:      suite.SourcePath,
		SuiteName:      suite.Metadata.Name,
		RunDir:         runDir,
		Classification: suite.Metadata.Classification,
	}
	for _, cr := range targetCases {
		promoted, err := promoteCase(suite, runDir, cr)
		if err != nil {
			return nil, err
		}
		result.Cases = append(result.Cases, promoted)
	}
	return result, nil
}

// BuildCoverageReport scans suites for golden tape and baseline coverage.
func BuildCoverageReport(workspace string, suitePaths []string, now time.Time) (*CoverageReport, error) {
	report := &CoverageReport{
		Workspace:   strings.TrimSpace(workspace),
		GeneratedAt: now.UTC(),
	}
	for _, suitePath := range suitePaths {
		suite, err := agenttest.LoadSuite(suitePath)
		if err != nil {
			return nil, err
		}
		suiteCoverage := SuiteCoverage{
			SuiteName: suite.Metadata.Name,
			SuitePath: suite.SourcePath,
		}
		for _, c := range suite.Spec.Cases {
			models := suiteModelsForCase(suite, c)
			if len(models) == 0 {
				entry := CoverageEntry{
					SuiteName: suite.Metadata.Name,
					SuitePath: suite.SourcePath,
					CaseName:  c.Name,
					Status:    "no-models",
				}
				report.Missing = append(report.Missing, entry)
				suiteCoverage.Entries = append(suiteCoverage.Entries, entry)
				suiteCoverage.MissingTapes++
				report.Totals.Cases++
				report.Totals.Missing++
				continue
			}
			for _, model := range models {
				entry, err := buildCoverageEntry(workspace, suite, c, model, now)
				if err != nil {
					return nil, err
				}
				suiteCoverage.Entries = append(suiteCoverage.Entries, entry)
				report.Totals.Cases++
				if entry.Present {
					report.Totals.Present++
					suiteCoverage.PresentTapes++
				} else {
					report.Totals.Missing++
					suiteCoverage.MissingTapes++
					report.Missing = append(report.Missing, entry)
				}
				if entry.Stale {
					report.Totals.Stale++
					suiteCoverage.StaleTapes++
					report.Stale = append(report.Stale, entry)
				}
			}
		}
		report.Totals.Suites++
		report.Suites = append(report.Suites, suiteCoverage)
	}
	return report, nil
}

// BuildRerecordPlan derives an actionable rerecord list from the coverage report.
func BuildRerecordPlan(workspace string, suitePaths []string, now time.Time) (*RerecordPlan, error) {
	coverage, err := BuildCoverageReport(workspace, suitePaths, now)
	if err != nil {
		return nil, err
	}
	plan := &RerecordPlan{
		Workspace:   coverage.Workspace,
		GeneratedAt: coverage.GeneratedAt,
	}
	for _, suite := range coverage.Suites {
		for _, entry := range suite.Entries {
			if !needsRerecord(entry) {
				continue
			}
			if containsRerecordEntry(plan.Entries, entry) {
				continue
			}
			rerecord := RerecordEntry{
				Entry:  entry,
				Reason: rerecordReason(entry),
			}
			rerecord.SuggestedCommand = suggestRerecordCommand(entry)
			plan.Entries = append(plan.Entries, rerecord)
			switch {
			case !entry.Present:
				plan.Totals.Missing++
			case entry.Stale || (entry.BaselinePresent && strings.Contains(entry.BaselineStatus, tapeEntryStaleStatus)):
				plan.Totals.Stale++
			}
		}
	}
	plan.Totals.Entries = len(plan.Entries)
	return plan, nil
}

func promoteCase(suite *agenttest.Suite, runDir string, cr agenttest.CaseReport) (PromotionCaseReport, error) {
	if cr.Skipped || !cr.Success {
		return PromotionCaseReport{}, fmt.Errorf("case %q did not pass in run %s", cr.Name, runDir)
	}
	srcTape := filepath.Join(cr.ArtifactsDir, "tape.jsonl")
	inspection, err := llm.InspectTape(srcTape)
	if err != nil {
		return PromotionCaseReport{}, fmt.Errorf("case %q tape invalid: %w", cr.Name, err)
	}
	if inspection.Header == nil {
		return PromotionCaseReport{}, fmt.Errorf("case %q tape has no header", cr.Name)
	}
	if strings.TrimSpace(inspection.Header.SuiteName) != "" && strings.TrimSpace(inspection.Header.SuiteName) != strings.TrimSpace(suite.Metadata.Name) {
		return PromotionCaseReport{}, fmt.Errorf("case %q tape header suite %q does not match suite %q", cr.Name, inspection.Header.SuiteName, suite.Metadata.Name)
	}
	if strings.TrimSpace(inspection.Header.CaseName) != "" && strings.TrimSpace(inspection.Header.CaseName) != strings.TrimSpace(cr.Name) {
		return PromotionCaseReport{}, fmt.Errorf("case %q tape header case %q does not match report case %q", cr.Name, inspection.Header.CaseName, cr.Name)
	}
	if strings.TrimSpace(inspection.Header.ModelName) != "" && strings.TrimSpace(cr.Model) != "" && strings.TrimSpace(inspection.Header.ModelName) != strings.TrimSpace(cr.Model) {
		return PromotionCaseReport{}, fmt.Errorf("case %q tape header model %q does not match report model %q", cr.Name, inspection.Header.ModelName, cr.Model)
	}

	destTape := agenttest.GoldenTapePath(suite.SourcePath, suite.Metadata.Name, cr.Name, cr.Model)
	if destTape == "" {
		return PromotionCaseReport{}, fmt.Errorf("case %q golden tape path unavailable", cr.Name)
	}
	if err := os.MkdirAll(filepath.Dir(destTape), 0o750); err != nil {
		return PromotionCaseReport{}, err
	}
	if err := copyFile(srcTape, destTape); err != nil {
		return PromotionCaseReport{}, err
	}
	report := PromotionCaseReport{
		CaseName:            cr.Name,
		ModelName:           cr.Model,
		SourceRunDir:        filepath.Dir(filepath.Dir(cr.ArtifactsDir)),
		SourceArtifactsDir:  cr.ArtifactsDir,
		SourceTapePath:      srcTape,
		DestinationTapePath: destTape,
		RecordedAt:          inspection.FirstRecordedAt,
	}
	report.PromotedArtifacts = append(report.PromotedArtifacts, "tape.jsonl")

	srcInteractionTape := filepath.Join(cr.ArtifactsDir, "interaction.tape.jsonl")
	if _, err := os.Stat(srcInteractionTape); err == nil {
		destInteractionTape := strings.TrimSuffix(destTape, ".tape.jsonl") + ".interaction.tape.jsonl"
		if err := copyFile(srcInteractionTape, destInteractionTape); err != nil {
			return PromotionCaseReport{}, err
		}
		report.SourceInteractionTapePath = srcInteractionTape
		report.DestinationInteractionTapePath = destInteractionTape
		report.PromotedArtifacts = append(report.PromotedArtifacts, "interaction.tape.jsonl")
	}

	if err := promoteSuiteLayerArtifacts(suite, cr, destTape, &report); err != nil {
		return PromotionCaseReport{}, err
	}

	if err := writePromotionLineage(filepath.Dir(destTape), suite, cr, destTape, report.PromotedArtifacts); err != nil {
		return PromotionCaseReport{}, err
	}
	report.LineagePath = filepath.Join(filepath.Dir(destTape), PromotionLineageFilename(cr.Name, cr.Model))
	report.PromotedArtifacts = uniqueStrings(report.PromotedArtifacts)
	return report, nil
}

func promoteSuiteLayerArtifacts(suite *agenttest.Suite, cr agenttest.CaseReport, destTape string, report *PromotionCaseReport) error {
	switch strings.ToLower(strings.TrimSpace(suite.Metadata.Classification)) {
	case suiteClassificationBenchmark:
		destBaseline := filepath.Join(filepath.Dir(destTape), agenttest.GoldenBaselineFilename(cr.Name, cr.Model))
		if baseline := agenttest.BuildPerformanceBaseline(cr, cr.FinishedAt); baseline != nil {
			if err := agenttest.WritePerformanceBaseline(destBaseline, baseline); err != nil {
				return err
			}
			report.DestinationBaselinePath = destBaseline
			report.PromotedArtifacts = append(report.PromotedArtifacts, filepath.Base(destBaseline))
		}
		for _, artifact := range []string{benchmarkReportFilename, benchmarkScoreFilename, benchmarkComparisonFilename} {
			src := filepath.Join(cr.ArtifactsDir, artifact)
			if _, err := os.Stat(src); err == nil {
				dst := filepath.Join(filepath.Dir(destTape), artifact)
				if err := copyFile(src, dst); err != nil {
					return err
				}
				report.PromotedArtifacts = append(report.PromotedArtifacts, artifact)
			}
		}
	default:
		destBaseline := filepath.Join(filepath.Dir(destTape), agenttest.GoldenBaselineFilename(cr.Name, cr.Model))
		if baseline := agenttest.BuildPerformanceBaseline(cr, cr.FinishedAt); baseline != nil {
			if err := agenttest.WritePerformanceBaseline(destBaseline, baseline); err != nil {
				return err
			}
			report.DestinationBaselinePath = destBaseline
			report.PromotedArtifacts = append(report.PromotedArtifacts, filepath.Base(destBaseline))
		}
	}
	return nil
}

func buildCoverageEntry(workspace string, suite *agenttest.Suite, c agenttest.CaseSpec, model agenttest.ModelSpec, now time.Time) (CoverageEntry, error) {
	tapePath := agenttest.GoldenTapePath(suite.SourcePath, suite.Metadata.Name, c.Name, model.Name)
	entry := CoverageEntry{
		SuiteName: suite.Metadata.Name,
		SuitePath: suite.SourcePath,
		CaseName:  c.Name,
		ModelName: model.Name,
		TapePath:  tapePath,
	}
	entry.BaselinePath = agenttest.BaselineFilePath(workspace, suite.Metadata.Name, c.Name, model.Name)
	if _, err := os.Stat(entry.BaselinePath); err == nil {
		entry.BaselinePresent = true
		if baseline, err := agenttest.LoadPerformanceBaseline(entry.BaselinePath); err == nil && baseline != nil {
			if recordedAt, err := time.Parse("2006-01-02", baseline.RecordedAt); err == nil {
				entry.BaselineRecordedAt = recordedAt
				entry.BaselineStatus = baselineStatus(recordedAt, now)
			} else {
				entry.BaselineStatus = baselineInvalidStatus
			}
		} else {
			entry.BaselineStatus = baselineInvalidStatus
		}
	} else {
		entry.BaselineStatus = baselineMissingStatus
	}
	inspection, err := llm.InspectTape(tapePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			entry.Status = tapeEntryMissingStatus
			return entry, nil
		}
		return CoverageEntry{}, err
	}
	entry.Present = true
	entry.Legacy = inspection.Legacy
	entry.RecordedAt = inspection.FirstRecordedAt
	entry.Status = tapeStatus(inspection, now)
	entry.Stale = entry.Status == tapeEntryStaleStatus
	return entry, nil
}

func tapeStatus(inspection *llm.TapeInspection, now time.Time) string {
	if inspection == nil {
		return tapeEntryMissingStatus
	}
	if inspection.Legacy {
		return tapeEntryLegacyStatus
	}
	if inspection.FirstRecordedAt.IsZero() {
		return tapeStatusRecordedUnknown
	}
	if now.Sub(inspection.FirstRecordedAt) > staleTapeThreshold {
		return tapeEntryStaleStatus
	}
	return tapeEntryFreshStatus
}

func baselineStatus(recordedAt time.Time, now time.Time) string {
	if recordedAt.IsZero() {
		return baselineRecordedUnknownStatus
	}
	if now.Sub(recordedAt) > staleTapeThreshold {
		return baselineStaleStatus
	}
	return baselineFreshStatus
}

func rerecordReason(entry CoverageEntry) string {
	switch {
	case !entry.Present:
		return "missing golden tape"
	case entry.Stale:
		return "stale golden tape"
	case entry.BaselinePresent && strings.Contains(entry.BaselineStatus, tapeEntryStaleStatus):
		return "stale performance baseline"
	default:
		return "refresh requested"
	}
}

func needsRerecord(entry CoverageEntry) bool {
	return !entry.Present || entry.Stale || (entry.BaselinePresent && strings.Contains(entry.BaselineStatus, tapeEntryStaleStatus))
}

func containsRerecordEntry(entries []RerecordEntry, entry CoverageEntry) bool {
	for _, existing := range entries {
		if existing.Entry.SuiteName == entry.SuiteName &&
			existing.Entry.SuitePath == entry.SuitePath &&
			existing.Entry.CaseName == entry.CaseName &&
			existing.Entry.ModelName == entry.ModelName {
			return true
		}
	}
	return false
}

func suggestRerecordCommand(entry CoverageEntry) string {
	if entry.SuiteName == "" || entry.CaseName == "" {
		return ""
	}
	if entry.ModelName != "" {
		return fmt.Sprintf("agenttest rerecord --suite %s --case %s --model %s", entry.SuiteName, entry.CaseName, entry.ModelName)
	}
	return fmt.Sprintf("agenttest rerecord --suite %s --case %s", entry.SuiteName, entry.CaseName)
}

func writePromotionLineage(destDir string, suite *agenttest.Suite, cr agenttest.CaseReport, destTape string, promotedArtifacts []string) error {
	if suite == nil {
		return nil
	}
	record := PromotionLineageRecord{
		SuiteName:         suite.Metadata.Name,
		SuitePath:         suite.SourcePath,
		Classification:    suite.Metadata.Classification,
		CaseName:          cr.Name,
		Model:             cr.Model,
		Provider:          cr.Provider,
		Layer:             strings.TrimSpace(suite.Metadata.Classification),
		PromotedArtifacts: uniqueStrings(promotedArtifacts),
		SourceRunDir:      filepath.Dir(filepath.Dir(cr.ArtifactsDir)),
		SourceArtifacts:   cr.ArtifactsDir,
		DestinationTape:   destTape,
		CreatedAt:         time.Now().UTC(),
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return fs.WriteFileSecure(filepath.Join(destDir, PromotionLineageFilename(cr.Name, cr.Model)), data)
}

// PromotionLineageRecord records what was promoted and from where.
type PromotionLineageRecord struct {
	SuiteName         string    `json:"suite_name"`
	SuitePath         string    `json:"suite_path"`
	Classification    string    `json:"classification"`
	CaseName          string    `json:"case_name"`
	Model             string    `json:"model"`
	Provider          string    `json:"provider,omitempty"`
	Layer             string    `json:"layer"`
	PromotedArtifacts []string  `json:"promoted_artifacts"`
	SourceRunDir      string    `json:"source_run_dir"`
	SourceArtifacts   string    `json:"source_artifacts"`
	DestinationTape   string    `json:"destination_tape,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

func loadSuiteReport(path string) (*agenttest.SuiteReport, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	var report agenttest.SuiteReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}
	return &report, nil
}

func selectPromotableCases(report *agenttest.SuiteReport, caseName string, all bool) []agenttest.CaseReport {
	if report == nil {
		return nil
	}
	if all {
		out := append([]agenttest.CaseReport(nil), report.Cases...)
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return out
	}
	for _, c := range report.Cases {
		if c.Name == caseName {
			return []agenttest.CaseReport{c}
		}
	}
	return nil
}

func suiteModelsForCase(suite *agenttest.Suite, c agenttest.CaseSpec) []agenttest.ModelSpec {
	if c.Overrides.Model != nil {
		return expandSuiteModelMatrix([]agenttest.ModelSpec{*c.Overrides.Model}, suite.Spec.Providers, suite.Spec.Execution.MatrixOrder)
	}
	return expandSuiteModelMatrix(suite.Spec.Models, suite.Spec.Providers, suite.Spec.Execution.MatrixOrder)
}

func expandSuiteModelMatrix(models []agenttest.ModelSpec, providers []agenttest.ProviderSpec, order string) []agenttest.ModelSpec {
	if len(models) == 0 {
		models = []agenttest.ModelSpec{{Name: "", Endpoint: ""}}
	}
	if len(providers) == 0 {
		return append([]agenttest.ModelSpec(nil), models...)
	}
	if strings.TrimSpace(order) == "model-first" {
		return expandSuiteModelMatrixModelFirst(models, providers)
	}
	return expandSuiteModelMatrixProviderFirst(models, providers)
}

func expandSuiteModelMatrixProviderFirst(models []agenttest.ModelSpec, providers []agenttest.ProviderSpec) []agenttest.ModelSpec {
	rows := make([]agenttest.ModelSpec, 0, len(models)*len(providers))
	for _, provider := range providers {
		for _, model := range models {
			rows = append(rows, modelForProvider(model, provider))
		}
	}
	return rows
}

func expandSuiteModelMatrixModelFirst(models []agenttest.ModelSpec, providers []agenttest.ProviderSpec) []agenttest.ModelSpec {
	rows := make([]agenttest.ModelSpec, 0, len(models)*len(providers))
	for _, model := range models {
		for _, provider := range providers {
			rows = append(rows, modelForProvider(model, provider))
		}
	}
	return rows
}

func modelForProvider(model agenttest.ModelSpec, provider agenttest.ProviderSpec) agenttest.ModelSpec {
	out := model
	if strings.TrimSpace(provider.Name) != "" {
		out.Provider = provider.Name
	}
	if strings.TrimSpace(provider.Endpoint) != "" {
		out.Endpoint = provider.Endpoint
	}
	if strings.TrimSpace(provider.ResetStrategy) != "" {
		out.ResetStrategy = provider.ResetStrategy
	}
	if provider.ResetBetween {
		out.ResetBetween = true
	}
	return out
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func sanitizeName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unnamed"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "unnamed"
	}
	return out
}

func copyFile(src, dst string) error {
	// #nosec G304 -- src is derived from validated run artifacts or golden tape paths.
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	return fs.WriteFileSecure(dst, data)
}
