package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Messages for populating debug subtabs from runtime / agent events.
// ---------------------------------------------------------------------------

// DebugTestResultMsg delivers test run output to the test subtab.
type DebugTestResultMsg struct {
	Package  string
	Output   []string // raw lines from the test runner
	Passed   int
	Failed   int
	Skipped  int
	Duration string
	Err      error
}

// DebugBenchmarkResultMsg delivers benchmark results to the benchmark subtab.
type DebugBenchmarkResultMsg struct {
	Package string
	Results []BenchmarkEntry
	Err     error
}

// BenchmarkEntry holds one benchmark result row.
type BenchmarkEntry struct {
	Name        string
	Iterations  int
	NsPerOp     float64
	AllocsPerOp int
	BytesPerOp  int64
}

// DebugTraceMsg delivers a parsed trace to the trace subtab.
type DebugTraceMsg struct {
	Trace TraceInfo
}

// DebugPlanDiffMsg delivers plan divergence data to the live-plan-diff subtab.
type DebugPlanDiffMsg struct {
	Diff PlanDiffInfo
}

// ---------------------------------------------------------------------------
// DebugPane
// ---------------------------------------------------------------------------

// DebugPane renders the four debug subtabs: test, benchmark, trace, live-plan-diff.
// It is always held by pointer so mutations survive tea.Model value copies.
type DebugPane struct {
	activeSubTab SubTabID

	// test subtab
	testResults  []DebugTestResultMsg
	testSel      int
	testExpanded map[int]bool // which result rows are expanded

	// benchmark subtab
	benchResults []DebugBenchmarkResultMsg
	benchSel     int

	// trace subtab
	trace    TraceInfo
	traceSel int // selected top-level frame

	// live-plan-diff subtab
	planDiff    PlanDiffInfo
	planDiffSel int

	// per-subtab status line
	statusMsg string

	width, height int
}

// NewDebugPane creates an empty DebugPane with the test subtab active.
func NewDebugPane() *DebugPane {
	return &DebugPane{
		activeSubTab: SubTabDebugTest,
		testExpanded: make(map[int]bool),
	}
}

// SetSubTab switches the visible subtab and clears the status line.
func (p *DebugPane) SetSubTab(id SubTabID) {
	p.activeSubTab = id
	p.statusMsg = ""
}

// SetSize resizes the pane.
func (p *DebugPane) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// HandleInputSubmit accepts a filter or command from the input bar.
// On the test subtab: re-runs the last test package (if any) — stub for now.
// On the trace subtab: filters visible frames by name.
func (p *DebugPane) HandleInputSubmit(value string) tea.Cmd {
	value = strings.TrimSpace(value)
	switch p.activeSubTab {
	case SubTabDebugTest:
		if value != "" {
			p.statusMsg = fmt.Sprintf("running tests: %s", value)
		}
	case SubTabDebugBenchmark:
		if value != "" {
			p.statusMsg = fmt.Sprintf("running benchmark: %s", value)
		}
	case SubTabDebugTrace:
		// Future: filter trace frames
		p.statusMsg = fmt.Sprintf("trace filter: %q (not yet implemented)", value)
	case SubTabDebugPlanDiff:
		p.statusMsg = "refreshing plan diff"
	}
	return nil
}

// Update handles key events and async data messages.
func (p *DebugPane) Update(msg tea.Msg) (DebugPaner, tea.Cmd) {
	switch msg := msg.(type) {
	case DebugTestResultMsg:
		p.testResults = append(p.testResults, msg)
		p.testSel = len(p.testResults) - 1

	case DebugBenchmarkResultMsg:
		p.benchResults = append(p.benchResults, msg)
		p.benchSel = len(p.benchResults) - 1

	case DebugTraceMsg:
		p.trace = msg.Trace
		p.traceSel = 0

	case DebugPlanDiffMsg:
		p.planDiff = msg.Diff
		p.planDiffSel = 0

	case tea.KeyMsg:
		return p.handleKey(msg)
	}
	return p, nil
}

func (p *DebugPane) handleKey(msg tea.KeyMsg) (DebugPaner, tea.Cmd) {
	p.statusMsg = ""
	switch p.activeSubTab {
	case SubTabDebugTest:
		switch msg.String() {
		case "up":
			if p.testSel > 0 {
				p.testSel--
			}
		case "down":
			if p.testSel < len(p.testResults)-1 {
				p.testSel++
			}
		case "enter", "x":
			// Toggle expansion of the selected result.
			if _, open := p.testExpanded[p.testSel]; open {
				delete(p.testExpanded, p.testSel)
			} else {
				p.testExpanded[p.testSel] = true
			}
		case "X":
			// Collapse all.
			p.testExpanded = make(map[int]bool)
		}

	case SubTabDebugBenchmark:
		switch msg.String() {
		case "up":
			if p.benchSel > 0 {
				p.benchSel--
			}
		case "down":
			if p.benchSel < len(p.benchResults)-1 {
				p.benchSel++
			}
		}

	case SubTabDebugTrace:
		switch msg.String() {
		case "up":
			if p.traceSel > 0 {
				p.traceSel--
			}
		case "down":
			if p.traceSel < len(p.trace.Frames)-1 {
				p.traceSel++
			}
		}

	case SubTabDebugPlanDiff:
		rows := p.planDiffRows()
		switch msg.String() {
		case "up":
			if p.planDiffSel > 0 {
				p.planDiffSel--
			}
		case "down":
			if p.planDiffSel < len(rows)-1 {
				p.planDiffSel++
			}
		}
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Row helpers
// ---------------------------------------------------------------------------

type debugPlanDiffRow struct {
	label string
	kind  string // "step" | "drift"
	id    string
}

func (p *DebugPane) planDiffRows() []debugPlanDiffRow {
	var rows []debugPlanDiffRow
	for _, s := range p.planDiff.Steps {
		glyph := stepStatusGlyph(s.Status)
		style := dimStyle
		switch s.Status {
		case "done":
			style = completedStyle
		case "running":
			style = inProgressStyle
		case "failed":
			style = diffRemoveStyle
		}
		label := style.Render(glyph) + "  " + s.Title
		rows = append(rows, debugPlanDiffRow{label: label, kind: "step", id: s.ID})
	}
	for _, d := range p.planDiff.AnchorDrifts {
		label := diffRemoveStyle.Render("⚓ drift") + "  " +
			filePathStyle.Render(d.AnchorName) +
			dimStyle.Render(fmt.Sprintf("  %s:%d  %s", d.FilePath, d.Line, d.Reason))
		rows = append(rows, debugPlanDiffRow{label: label, kind: "drift", id: d.AnchorName})
	}
	return rows
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (p *DebugPane) View() string {
	switch p.activeSubTab {
	case SubTabDebugTest:
		return p.viewTest()
	case SubTabDebugBenchmark:
		return p.viewBenchmark()
	case SubTabDebugTrace:
		return p.viewTrace()
	case SubTabDebugPlanDiff:
		return p.viewPlanDiff()
	default:
		return p.viewTest()
	}
}

func (p *DebugPane) viewTest() string {
	widths := (&PlannerPane{width: p.width}).splitWidths(5, 7)
	summary := []string{
		fmt.Sprintf("runs:    %d", len(p.testResults)),
	}
	if len(p.testResults) > 0 {
		latest := p.testResults[p.testSel]
		summary = append(summary,
			fmt.Sprintf("passed:  %d", latest.Passed),
			fmt.Sprintf("failed:  %d", latest.Failed),
			fmt.Sprintf("skipped: %d", latest.Skipped),
		)
		if latest.Duration != "" {
			summary = append(summary, dimStyle.Render("duration")+"  "+latest.Duration)
		}
	}
	if p.statusMsg != "" {
		summary = append(summary, "", inProgressStyle.Render(p.statusMsg))
	}
	listLines := make([]string, 0, len(p.testResults))
	for _, r := range p.testResults {
		line := fmt.Sprintf("%s  PASS %d  FAIL %d  SKIP %d", r.Package, r.Passed, r.Failed, r.Skipped)
		if r.Err != nil {
			line = fmt.Sprintf("%s  ERROR  %s", r.Package, r.Err.Error())
		}
		listLines = append(listLines, line)
	}
	detail := []string{}
	if len(p.testResults) == 0 {
		detail = append(detail,
			dimStyle.Render("No test results yet."),
			dimStyle.Render("Type a package path in the input bar to run tests."),
		)
	} else if p.testSel < len(p.testResults) {
		r := p.testResults[p.testSel]
		detail = append(detail, r.Package, "")
		if r.Err != nil {
			detail = append(detail, diffRemoveStyle.Render(r.Err.Error()))
		}
		shown := r.Output
		maxLines := 20
		if len(shown) > maxLines {
			shown = shown[len(shown)-maxLines:]
		}
		for _, ol := range shown {
			style := dimStyle
			if strings.Contains(ol, "FAIL") || strings.Contains(ol, "Error") {
				style = diffRemoveStyle
			} else if strings.Contains(ol, "PASS") || strings.Contains(ol, "ok") {
				style = completedStyle
			}
			detail = append(detail, style.Render(ol))
		}
	}
	return strings.Join([]string{
		sectionHeaderStyle.Render("Debug: Tests"),
		lipgloss.JoinHorizontal(lipgloss.Top,
			plannerPanel("Summary", widths[0], summary...),
			plannerPanel("Runs", widths[1], plannerList(listLines, p.testSel, p.height-10)),
		),
		plannerPanel("Output", p.width, detail...),
		dimStyle.Render("↑↓ navigate  enter/x expand  X collapse all  type pkg + enter to run"),
	}, "\n")
}

func (p *DebugPane) viewBenchmark() string {
	widths := (&PlannerPane{width: p.width}).splitWidths(4, 8)
	summary := []string{
		fmt.Sprintf("runs: %d", len(p.benchResults)),
	}
	if p.statusMsg != "" {
		summary = append(summary, "", inProgressStyle.Render(p.statusMsg))
	}
	listLines := make([]string, 0, len(p.benchResults))
	for _, r := range p.benchResults {
		line := fmt.Sprintf("%s  %d benchmark(s)", r.Package, len(r.Results))
		if r.Err != nil {
			line = fmt.Sprintf("%s  ERROR  %s", r.Package, r.Err.Error())
		}
		listLines = append(listLines, line)
	}
	detail := []string{}
	if len(p.benchResults) == 0 {
		detail = append(detail,
			dimStyle.Render("No benchmark results yet."),
			dimStyle.Render("Type a package path in the input bar to run benchmarks."),
		)
	} else {
		r := p.benchResults[p.benchSel]
		detail = append(detail, r.Package, "")
		if r.Err != nil {
			detail = append(detail, diffRemoveStyle.Render(r.Err.Error()))
		} else {
			for _, entry := range r.Results {
				detail = append(detail,
					fmt.Sprintf("%s  %.1f ns/op  %d allocs/op  %d B/op", entry.Name, entry.NsPerOp, entry.AllocsPerOp, entry.BytesPerOp),
				)
			}
		}
	}
	return strings.Join([]string{
		sectionHeaderStyle.Render("Debug: Benchmarks"),
		lipgloss.JoinHorizontal(lipgloss.Top,
			plannerPanel("Summary", widths[0], summary...),
			plannerPanel("Runs", widths[1], plannerList(listLines, p.benchSel, p.height-10)),
		),
		plannerPanel("Results", p.width, detail...),
		dimStyle.Render("↑↓ navigate  type pkg + enter to run"),
	}, "\n")
}

func (p *DebugPane) viewTrace() string {
	widths := (&PlannerPane{width: p.width}).splitWidths(5, 7)
	title := "Execution Trace"
	if p.trace.Description != "" {
		title = "Trace: " + p.trace.Description
	}
	summary := []string{
		fmt.Sprintf("frames: %d", len(p.trace.Frames)),
	}
	if p.statusMsg != "" {
		summary = append(summary, "", inProgressStyle.Render(p.statusMsg))
	}
	listLines := make([]string, 0, len(p.trace.Frames))
	for _, frame := range p.trace.Frames {
		listLines = append(listLines, renderTraceFrame(frame, 0))
	}
	detail := []string{}
	if len(p.trace.Frames) == 0 {
		detail = append(detail,
			dimStyle.Render("No trace data yet."),
			dimStyle.Render("The agent will emit trace frames during execution."),
		)
	} else if p.traceSel < len(p.trace.Frames) {
		frame := p.trace.Frames[p.traceSel]
		detail = append(detail,
			frame.FuncName,
			dimStyle.Render(fmt.Sprintf("%s:%d", frame.FilePath, frame.Line)),
		)
		if frame.Duration != "" {
			detail = append(detail, dimStyle.Render("Duration")+"  "+frame.Duration)
		}
		if frame.IsError && frame.ErrorMsg != "" {
			detail = append(detail, "", diffRemoveStyle.Render(frame.ErrorMsg))
		}
		if len(frame.Children) > 0 {
			detail = append(detail, "", dimStyle.Render("Children"))
			for _, child := range frame.Children {
				detail = append(detail, renderTraceFrame(child, 1))
			}
		}
	}
	return strings.Join([]string{
		sectionHeaderStyle.Render(title),
		lipgloss.JoinHorizontal(lipgloss.Top,
			plannerPanel("Summary", widths[0], summary...),
			plannerPanel("Frames", widths[1], plannerList(listLines, p.traceSel, p.height-10)),
		),
		plannerPanel("Selected Frame", p.width, detail...),
		dimStyle.Render("↑↓ navigate frames"),
	}, "\n")
}

func renderTraceFrame(f TraceFrame, indent int) string {
	prefix := strings.Repeat("  ", indent)
	style := dimStyle
	durationLabel := ""
	if f.Duration != "" {
		durationLabel = "  " + dimStyle.Render(f.Duration)
	}
	funcLabel := filePathStyle.Render(f.FuncName)
	locLabel := dimStyle.Render(fmt.Sprintf("%s:%d", f.FilePath, f.Line))
	if f.IsError {
		style = diffRemoveStyle
		funcLabel = style.Render(f.FuncName)
		if f.ErrorMsg != "" {
			locLabel = style.Render(f.ErrorMsg)
		}
	}
	childHint := ""
	if len(f.Children) > 0 {
		childHint = dimStyle.Render(fmt.Sprintf("  [%d child(ren)]", len(f.Children)))
	}
	_ = style
	return prefix + funcLabel + "  " + locLabel + durationLabel + childHint
}

func (p *DebugPane) viewPlanDiff() string {
	rows := p.planDiffRows()
	widths := (&PlannerPane{width: p.width}).splitWidths(4, 5, 5)
	title := "Live Plan Diff"
	if p.planDiff.WorkflowID != "" {
		title = "Plan Diff: " + p.planDiff.WorkflowID
	}
	summary := []string{
		fmt.Sprintf("steps:  %d", len(p.planDiff.Steps)),
		fmt.Sprintf("drifts: %d", len(p.planDiff.AnchorDrifts)),
	}
	if p.statusMsg != "" {
		summary = append(summary, "", inProgressStyle.Render(p.statusMsg))
	}
	listLines := make([]string, 0, len(rows))
	for _, row := range rows {
		listLines = append(listLines, row.label)
	}
	detail := []string{}
	if len(rows) == 0 {
		detail = append(detail,
			dimStyle.Render("No plan divergence detected."),
			dimStyle.Render("This view updates as the agent executes plan steps."),
		)
	} else if p.planDiffSel < len(rows) {
		row := rows[p.planDiffSel]
		switch row.kind {
		case "step":
			for _, step := range p.planDiff.Steps {
				if step.ID != row.id {
					continue
				}
				detail = append(detail,
					step.Title,
					"",
					dimStyle.Render("Status")+"  "+step.Status,
					dimStyle.Render("Attempts")+fmt.Sprintf("  %d", step.Attempts),
				)
				if len(step.DependsOn) > 0 {
					detail = append(detail, "", dimStyle.Render("Depends on"))
					detail = append(detail, step.DependsOn...)
				}
				break
			}
		case "drift":
			for _, drift := range p.planDiff.AnchorDrifts {
				if drift.AnchorName != row.id {
					continue
				}
				detail = append(detail,
					drift.AnchorName,
					"",
					dimStyle.Render("Location")+fmt.Sprintf("  %s:%d", drift.FilePath, drift.Line),
					dimStyle.Render("Reason")+"  "+drift.Reason,
				)
				break
			}
		}
	}
	var driftLines []string
	if len(p.planDiff.AnchorDrifts) == 0 {
		driftLines = append(driftLines, dimStyle.Render("No anchor drift detected."))
	} else {
		for _, drift := range p.planDiff.AnchorDrifts {
			driftLines = append(driftLines, fmt.Sprintf("%s  %s:%d", drift.AnchorName, drift.FilePath, drift.Line))
		}
	}
	return strings.Join([]string{
		sectionHeaderStyle.Render(title),
		lipgloss.JoinHorizontal(lipgloss.Top,
			plannerPanel("Summary", widths[0], summary...),
			plannerPanel("Divergence", widths[1], plannerList(listLines, p.planDiffSel, p.height-12)),
			plannerPanel("Detail", widths[2], detail...),
		),
		plannerPanel("Anchor Drift", p.width, driftLines...),
		dimStyle.Render("↑↓ navigate  enter refresh"),
	}, "\n")
}

// Verify that *DebugPane satisfies DebugPaner at compile time.
var _ DebugPaner = (*DebugPane)(nil)
