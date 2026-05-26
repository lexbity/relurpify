package euclotui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	"codeburg.org/lexbit/relurpify/framework/cfgload"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type diffNodeKind int

const (
	diffNodeStep diffNodeKind = iota
	diffNodeFile
	diffNodeHunk
)

type diffNode struct {
	Key    string
	Kind   diffNodeKind
	Depth  int
	Label  string
	StepID string
	File   string
	Hunk   *DiffHunkProjection
}

type diffBaseline struct {
	Exists bool
	Data   []byte
}

type DiffPane struct {
	router       *EucloEventRouter
	workspace    string
	width        int
	height       int
	selectedKey  string
	collapsed    map[string]bool
	rejected     map[string]bool
	baselines    map[string]diffBaseline
	lastRendered []diffNode
}

func NewDiffPane(router *EucloEventRouter, workspace string) *DiffPane {
	p := &DiffPane{
		router:    router,
		workspace: strings.TrimSpace(workspace),
		collapsed: make(map[string]bool),
		rejected:  make(map[string]bool),
		baselines: make(map[string]diffBaseline),
	}
	p.primeBaselines()
	return p
}

func (p *DiffPane) SetRouter(router *EucloEventRouter) {
	p.router = router
	p.primeBaselines()
}

func (p *DiffPane) SetWorkspace(workspace string) {
	workspace = strings.TrimSpace(workspace)
	if workspace == p.workspace {
		return
	}
	p.workspace = workspace
	p.baselines = make(map[string]diffBaseline)
	p.primeBaselines()
}

func (p *DiffPane) SetSize(w, h int) {
	p.width = w
	p.height = h
}

func (p *DiffPane) Update(msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	nodes := p.visibleNodes()
	if len(nodes) == 0 {
		switch key.String() {
		case "u", "U":
			return p.systemMsgCmd("No diff changes available")
		}
		return nil
	}
	idx := p.selectedIndex(nodes)
	switch key.String() {
	case "up", "k":
		if idx > 0 {
			p.selectedKey = nodes[idx-1].Key
		}
	case "down", "j":
		if idx < len(nodes)-1 {
			p.selectedKey = nodes[idx+1].Key
		}
	case "home":
		p.selectedKey = nodes[0].Key
	case "end":
		p.selectedKey = nodes[len(nodes)-1].Key
	case "left":
		if idx >= 0 {
			node := nodes[idx]
			if node.Kind == diffNodeStep || node.Kind == diffNodeFile {
				if !p.collapsed[node.Key] {
					p.collapsed[node.Key] = true
					return nil
				}
				if parent := p.parentKey(nodes, idx); parent != "" {
					p.selectedKey = parent
				}
			}
		}
	case "right", "l":
		if idx >= 0 {
			node := nodes[idx]
			if node.Kind == diffNodeStep || node.Kind == diffNodeFile {
				if p.collapsed[node.Key] {
					p.collapsed[node.Key] = false
					return nil
				}
				if child := p.firstChildKey(nodes, idx); child != "" {
					p.selectedKey = child
				}
			}
		}
	case "a":
		return p.applyAllCmd()
	case "s":
		if idx >= 0 {
			return p.applyStepCmd(nodes[idx].StepID)
		}
	case "f":
		if idx >= 0 {
			return p.applyFileCmd(nodes[idx].StepID, nodes[idx].File)
		}
	case "h":
		if idx >= 0 && nodes[idx].Kind == diffNodeHunk && nodes[idx].Hunk != nil {
			return p.applyHunkCmd(nodes[idx].File, *nodes[idx].Hunk)
		}
	case "u":
		if idx >= 0 {
			return p.revertFileCmd(nodes[idx].File)
		}
	case "U":
		return p.revertAllCmd()
	case "x":
		if idx >= 0 {
			p.rejected[nodes[idx].Key] = true
			return p.systemMsgCmd(fmt.Sprintf("Rejected %s", p.nodeLabel(nodes[idx])))
		}
	case "e":
		if idx >= 0 {
			path := p.nodeEditorPath(nodes[idx])
			if path != "" {
				return p.openEditorCmd(path)
			}
		}
	}
	return nil
}

func (p *DiffPane) View() string {
	if p.width < 72 {
		return dimStyle.Render("Terminal too narrow. Minimum 72 columns required.")
	}
	snap := p.snapshot()
	nodes := p.visibleNodesFrom(snap)
	p.lastRendered = nodes
	if len(nodes) == 0 {
		return eucloFrameStyle.Render(strings.Join([]string{
			sectionHeaderStyle.Render("Diff Surface"),
			dimStyle.Render("No diff hunks available."),
		}, "\n"))
	}
	leftW, rightW := diffSplitWidths(p.width)
	selected := p.selectedIndex(nodes)
	tree := p.renderTree(nodes, selected, leftW)
	detail := p.renderDetail(nodes[selected], rightW)
	return lipgloss.JoinHorizontal(lipgloss.Top, tree, detail)
}

func (p *DiffPane) snapshot() EucloProjectionSnapshot {
	if p == nil || p.router == nil {
		return EucloProjectionSnapshot{}
	}
	return p.router.Snapshot()
}

func (p *DiffPane) visibleNodes() []diffNode {
	return p.visibleNodesFrom(p.snapshot())
}

func (p *DiffPane) visibleNodesFrom(snap EucloProjectionSnapshot) []diffNode {
	if len(snap.Diff.Order) == 0 {
		return nil
	}
	var out []diffNode
	for _, stepID := range snap.Diff.Order {
		step := snap.Diff.Steps[stepID]
		if step == nil {
			continue
		}
		stepNode := diffNode{
			Key:    "step:" + stepID,
			Kind:   diffNodeStep,
			Depth:  0,
			Label:  p.stepLabel(step),
			StepID: stepID,
		}
		out = append(out, stepNode)
		if p.collapsed[stepNode.Key] {
			continue
		}
		for _, filePath := range step.Order {
			file := step.Files[filePath]
			if file == nil {
				continue
			}
			fileNode := diffNode{
				Key:    "file:" + stepID + ":" + filePath,
				Kind:   diffNodeFile,
				Depth:  1,
				Label:  p.fileLabel(*file),
				StepID: stepID,
				File:   filePath,
			}
			out = append(out, fileNode)
			if p.collapsed[fileNode.Key] {
				continue
			}
			for i := range file.Hunks {
				hunk := file.Hunks[i]
				hunkNode := diffNode{
					Key:    "hunk:" + stepID + ":" + filePath + ":" + strconv.Itoa(i),
					Kind:   diffNodeHunk,
					Depth:  2,
					Label:  p.hunkLabel(hunk, i),
					StepID: stepID,
					File:   filePath,
					Hunk:   &hunk,
				}
				out = append(out, hunkNode)
			}
		}
	}
	return out
}

func (p *DiffPane) selectedIndex(nodes []diffNode) int {
	if len(nodes) == 0 {
		return 0
	}
	if p.selectedKey == "" {
		p.selectedKey = nodes[0].Key
		return 0
	}
	for i, node := range nodes {
		if node.Key == p.selectedKey {
			return i
		}
	}
	p.selectedKey = nodes[0].Key
	return 0
}

func (p *DiffPane) parentKey(nodes []diffNode, idx int) string {
	if idx <= 0 || idx >= len(nodes) {
		return ""
	}
	node := nodes[idx]
	for i := idx - 1; i >= 0; i-- {
		if nodes[i].Depth < node.Depth {
			return nodes[i].Key
		}
	}
	return ""
}

func (p *DiffPane) firstChildKey(nodes []diffNode, idx int) string {
	if idx < 0 || idx >= len(nodes)-1 {
		return ""
	}
	if nodes[idx+1].Depth > nodes[idx].Depth {
		return nodes[idx+1].Key
	}
	return ""
}

func (p *DiffPane) renderTree(nodes []diffNode, selected int, width int) string {
	var lines []string
	lines = append(lines, sectionHeaderStyle.Render("Change Tree"))
	for i, node := range nodes {
		if node.Depth > 2 {
			continue
		}
		indent := strings.Repeat("  ", node.Depth)
		line := indent + p.treePrefix(node) + " " + node.Label
		if p.rejected[node.Key] {
			line = diffRemoveStyle.Render(line + " [rejected]")
		} else if i == selected {
			line = panelItemActiveStyle.Render(line)
		} else {
			line = panelItemStyle.Render(line)
		}
		lines = append(lines, line)
		if node.Kind == diffNodeHunk && node.Hunk != nil {
			if node.Hunk.VerificationFailed {
				lines = append(lines, indent+"    "+diffRemoveStyle.Render("✗ Verification failed"))
				if node.Hunk.VerificationLog != "" {
					for _, l := range strings.Split(strings.TrimSuffix(node.Hunk.VerificationLog, "\n"), "\n") {
						lines = append(lines, indent+"    "+dimStyle.Render(l))
					}
				}
			}
			if node.Hunk.DeferredMarkdownPath != "" {
				lines = append(lines, indent+"    "+dimStyle.Render("⚠ Deferred: ")+filePathStyle.Render(node.Hunk.DeferredMarkdownPath))
			}
		}
	}
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("a=apply all  s=apply step  f=apply file  h=apply hunk  u=revert file  U=revert all"))
	lines = append(lines, dimStyle.Render("x=reject node  e=open editor  arrows=navigate"))
	return eucloFrameStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (p *DiffPane) renderDetail(node diffNode, width int) string {
	lines := []string{sectionHeaderStyle.Render("Diff Viewer")}
	switch node.Kind {
	case diffNodeStep:
		lines = append(lines, headerStyle.Render(node.Label))
		lines = append(lines, p.stepDetailLines(node.StepID)...)
		lines = append(lines, "")
		lines = append(lines, p.renderStepDiff(node.StepID)...)
	case diffNodeFile:
		lines = append(lines, filePathStyle.Render(node.File))
		lines = append(lines, p.fileDetailLines(node.StepID, node.File)...)
		lines = append(lines, "")
		lines = append(lines, p.renderFileDiff(node.StepID, node.File)...)
	case diffNodeHunk:
		if node.Hunk != nil {
			lines = append(lines, filePathStyle.Render(node.File))
			if node.Hunk.Summary != "" {
				lines = append(lines, node.Hunk.Summary)
			}
			if node.Hunk.VerificationFailed {
				lines = append(lines, diffRemoveStyle.Render("Verification failed"))
			}
			lines = append(lines, "")
			lines = append(lines, renderUnifiedDiffText(node.Hunk.Body)...)
		}
	}
	if len(lines) == 1 {
		lines = append(lines, dimStyle.Render("No diff selection."))
	}
	return eucloFrameStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (p *DiffPane) stepLabel(step *DiffStepProjection) string {
	if step == nil {
		return "Step:"
	}
	label := fmt.Sprintf("Step: %s", step.StepID)
	if step.Origin != "" {
		label += " " + dimStyle.Render("("+step.Origin+")")
	}
	if step.VerificationFailed {
		label += " " + diffRemoveStyle.Render("✗")
	}
	return label
}

func (p *DiffPane) fileLabel(file DiffFileProjection) string {
	label := fmt.Sprintf("%s [%d]", file.File, len(file.Hunks))
	if file.VerificationFailed {
		label += " " + diffRemoveStyle.Render("✗")
	}
	return label
}

func (p *DiffPane) hunkLabel(hunk DiffHunkProjection, idx int) string {
	label := hunk.Summary
	if label == "" {
		label = fmt.Sprintf("Hunk %d", idx+1)
	}
	if hunk.LinesAdded != 0 || hunk.LinesRemoved != 0 {
		label += fmt.Sprintf(" [+%d/-%d]", hunk.LinesAdded, hunk.LinesRemoved)
	}
	return label
}

func (p *DiffPane) treePrefix(node diffNode) string {
	switch node.Kind {
	case diffNodeStep:
		if p.collapsed[node.Key] {
			return "▶"
		}
		return "▼"
	case diffNodeFile:
		if p.collapsed[node.Key] {
			return "▸"
		}
		return "▾"
	case diffNodeHunk:
		return "•"
	default:
		return " "
	}
}

func (p *DiffPane) stepDetailLines(stepID string) []string {
	snap := p.snapshot()
	step := snap.Diff.Steps[stepID]
	if step == nil {
		return []string{dimStyle.Render("No step details available.")}
	}
	lines := []string{
		dimStyle.Render(fmt.Sprintf("files: %d", len(step.Files))),
	}
	if step.VerificationFailed {
		lines = append(lines, diffRemoveStyle.Render("verification failed"))
	}
	if step.VerificationLog != "" {
		lines = append(lines, step.VerificationLog)
	}
	if step.DeferredMarkdownPath != "" {
		lines = append(lines, dimStyle.Render("deferred: ")+filePathStyle.Render(step.DeferredMarkdownPath))
	}
	return lines
}

func (p *DiffPane) fileDetailLines(stepID, filePath string) []string {
	snap := p.snapshot()
	step := snap.Diff.Steps[stepID]
	if step == nil {
		return []string{dimStyle.Render("No file details available.")}
	}
	file := step.Files[filePath]
	if file == nil {
		return []string{dimStyle.Render("No file details available.")}
	}
	lines := []string{
		dimStyle.Render(fmt.Sprintf("hunks: %d", len(file.Hunks))),
	}
	if file.VerificationFailed {
		lines = append(lines, diffRemoveStyle.Render("verification failed"))
	}
	if file.VerificationLog != "" {
		lines = append(lines, file.VerificationLog)
	}
	if file.DeferredMarkdownPath != "" {
		lines = append(lines, dimStyle.Render("deferred: ")+filePathStyle.Render(file.DeferredMarkdownPath))
	}
	return lines
}

func (p *DiffPane) renderStepDiff(stepID string) []string {
	snap := p.snapshot()
	step := snap.Diff.Steps[stepID]
	if step == nil {
		return []string{dimStyle.Render("No diff body available.")}
	}
	var bodies []string
	for _, filePath := range step.Order {
		file := step.Files[filePath]
		if file == nil {
			continue
		}
		for _, hunk := range file.Hunks {
			bodies = append(bodies, renderUnifiedDiffTextLines(hunk.Body)...)
		}
	}
	if len(bodies) == 0 {
		return []string{dimStyle.Render("No diff body available.")}
	}
	return bodies
}

func (p *DiffPane) renderFileDiff(stepID, filePath string) []string {
	snap := p.snapshot()
	step := snap.Diff.Steps[stepID]
	if step == nil {
		return []string{dimStyle.Render("No diff body available.")}
	}
	file := step.Files[filePath]
	if file == nil {
		return []string{dimStyle.Render("No diff body available.")}
	}
	var bodies []string
	for _, hunk := range file.Hunks {
		bodies = append(bodies, renderUnifiedDiffTextLines(hunk.Body)...)
	}
	if len(bodies) == 0 {
		return []string{dimStyle.Render("No diff body available.")}
	}
	return bodies
}

func (p *DiffPane) nodeEditorPath(node diffNode) string {
	switch node.Kind {
	case diffNodeHunk:
		if node.Hunk != nil && node.Hunk.DeferredMarkdownPath != "" {
			return p.resolvePath(node.Hunk.DeferredMarkdownPath)
		}
		return p.resolvePath(node.File)
	case diffNodeFile:
		return p.resolvePath(node.File)
	case diffNodeStep:
		snap := p.snapshot()
		step := snap.Diff.Steps[node.StepID]
		if step == nil {
			return ""
		}
		for _, filePath := range step.Order {
			if filePath != "" {
				return p.resolvePath(filePath)
			}
		}
	}
	return ""
}

func (p *DiffPane) openEditorCmd(path string) tea.Cmd {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	return tea.ExecProcess(editorProcess(path), func(err error) tea.Msg {
		if err != nil {
			return tui.ChatSystemMsg{Text: fmt.Sprintf("Editor error: %v", err)}
		}
		return nil
	})
}

func editorProcess(path string) *exec.Cmd {
	return exec.Command(tui.EditorPath(), path)
}

func (p *DiffPane) applyAllCmd() tea.Cmd {
	nodes := p.visibleNodes()
	if len(nodes) == 0 {
		return p.systemMsgCmd("No diff changes available")
	}
	snap := p.snapshot()
	var applied int
	for _, stepID := range snap.Diff.Order {
		if err := p.applyStep(stepID); err != nil {
			return p.systemMsgCmd(fmt.Sprintf("Apply all failed: %v", err))
		}
		applied++
	}
	return p.systemMsgCmd(fmt.Sprintf("Applied %d step(s)", applied))
}

func (p *DiffPane) applyStepCmd(stepID string) tea.Cmd {
	if err := p.applyStep(stepID); err != nil {
		return p.systemMsgCmd(fmt.Sprintf("Apply step failed: %v", err))
	}
	return p.systemMsgCmd(fmt.Sprintf("Applied step %s", stepID))
}

func (p *DiffPane) applyFileCmd(stepID, filePath string) tea.Cmd {
	if err := p.applyFile(stepID, filePath); err != nil {
		return p.systemMsgCmd(fmt.Sprintf("Apply file failed: %v", err))
	}
	return p.systemMsgCmd(fmt.Sprintf("Applied file %s", filePath))
}

func (p *DiffPane) applyHunkCmd(filePath string, hunk DiffHunkProjection) tea.Cmd {
	if err := p.applyHunk(filePath, hunk); err != nil {
		return p.systemMsgCmd(fmt.Sprintf("Apply hunk failed: %v", err))
	}
	return p.systemMsgCmd(fmt.Sprintf("Applied hunk %s", filePath))
}

func (p *DiffPane) revertFileCmd(filePath string) tea.Cmd {
	if err := p.revertFile(filePath); err != nil {
		return p.systemMsgCmd(fmt.Sprintf("Revert file failed: %v", err))
	}
	return p.systemMsgCmd(fmt.Sprintf("Reverted file %s", filePath))
}

func (p *DiffPane) revertAllCmd() tea.Cmd {
	if err := p.revertAll(); err != nil {
		return p.systemMsgCmd(fmt.Sprintf("Revert all failed: %v", err))
	}
	return p.systemMsgCmd("Reverted diff checkpoint")
}

func (p *DiffPane) applyStep(stepID string) error {
	snap := p.snapshot()
	step := snap.Diff.Steps[stepID]
	if step == nil {
		return fmt.Errorf("step %q not found", stepID)
	}
	for _, filePath := range step.Order {
		if err := p.applyFile(stepID, filePath); err != nil {
			return err
		}
	}
	return nil
}

func (p *DiffPane) applyFile(stepID, filePath string) error {
	snap := p.snapshot()
	step := snap.Diff.Steps[stepID]
	if step == nil {
		return fmt.Errorf("step %q not found", stepID)
	}
	file := step.Files[filePath]
	if file == nil {
		return fmt.Errorf("file %q not found", filePath)
	}
	return p.applyHunks(filePath, file.Hunks)
}

func (p *DiffPane) applyHunk(filePath string, hunk DiffHunkProjection) error {
	return p.applyHunks(filePath, []DiffHunkProjection{hunk})
}

func (p *DiffPane) applyHunks(filePath string, hunks []DiffHunkProjection) error {
	if len(hunks) == 0 {
		return nil
	}
	abs := p.resolvePath(filePath)
	if abs == "" {
		return fmt.Errorf("workspace path unavailable")
	}
	if err := p.captureBaseline(filePath); err != nil {
		return err
	}
	current, err := os.ReadFile(abs)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	updated := append([]byte(nil), current...)
	for _, hunk := range hunks {
		updated, err = applyHunkBody(updated, hunk.Body)
		if err != nil {
			return fmt.Errorf("%s: %w", filePath, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	if _, err := cfgload.CreateTimestampedBackup(abs); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(abs, updated, 0o644)
}

func (p *DiffPane) revertFile(filePath string) error {
	abs := p.resolvePath(filePath)
	if abs == "" {
		return fmt.Errorf("workspace path unavailable")
	}
	baseline, ok := p.baselines[filePath]
	if !ok {
		if err := p.captureBaseline(filePath); err != nil {
			return err
		}
		baseline = p.baselines[filePath]
	}
	if !baseline.Exists {
		if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	return os.WriteFile(abs, baseline.Data, 0o644)
}

func (p *DiffPane) revertAll() error {
	for filePath := range p.baselines {
		if err := p.revertFile(filePath); err != nil {
			return err
		}
	}
	return nil
}

func (p *DiffPane) captureBaseline(filePath string) error {
	if _, ok := p.baselines[filePath]; ok {
		return nil
	}
	abs := p.resolvePath(filePath)
	if abs == "" {
		return fmt.Errorf("workspace path unavailable")
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			p.baselines[filePath] = diffBaseline{Exists: false}
			return nil
		}
		return err
	}
	p.baselines[filePath] = diffBaseline{Exists: true, Data: append([]byte(nil), data...)}
	return nil
}

func (p *DiffPane) primeBaselines() {
	snap := p.snapshot()
	for _, stepID := range snap.Diff.Order {
		step := snap.Diff.Steps[stepID]
		if step == nil {
			continue
		}
		for _, filePath := range step.Order {
			_ = p.captureBaseline(filePath)
		}
	}
}

func (p *DiffPane) resolvePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	root := p.workspace
	if root == "" {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(root, path))
}

func (p *DiffPane) systemMsgCmd(text string) tea.Cmd {
	return func() tea.Msg {
		return tui.ChatSystemMsg{Text: text}
	}
}

func (p *DiffPane) nodeLabel(node diffNode) string {
	switch node.Kind {
	case diffNodeStep:
		return node.Label
	case diffNodeFile:
		return node.File
	case diffNodeHunk:
		if node.Hunk != nil && node.Hunk.Summary != "" {
			return node.Hunk.Summary
		}
		return node.Label
	default:
		return node.Label
	}
}

func renderUnifiedDiffText(text string) []string {
	if strings.TrimSpace(text) == "" {
		return []string{dimStyle.Render("(empty diff)")}
	}
	return renderUnifiedDiffTextLines(text)
}

func renderUnifiedDiffTextLines(text string) []string {
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "@@"):
			out = append(out, headerStyle.Render(line))
		case strings.HasPrefix(line, "+"):
			out = append(out, diffAddStyle.Render(line))
		case strings.HasPrefix(line, "-"):
			out = append(out, diffRemoveStyle.Render(line))
		default:
			out = append(out, dimStyle.Render(line))
		}
	}
	if len(out) == 0 {
		return []string{dimStyle.Render("(empty diff)")}
	}
	return out
}

func diffSplitWidths(total int) (int, int) {
	if total < 0 {
		total = 0
	}
	left := (total * 40) / 100
	if left < 28 {
		left = 28
	}
	right := total - left
	if right < 28 {
		right = 28
	}
	if left+right > total && total > 0 {
		right = total - left
	}
	return left, right
}

func applyHunkBody(current []byte, body string) ([]byte, error) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return current, nil
	}
	if !looksLikeUnifiedDiff(trimmed) {
		return []byte(body), nil
	}
	baseLines := splitPatchLines(string(current))
	patched, err := applyUnifiedPatch(baseLines, body)
	if err != nil {
		return nil, err
	}
	return []byte(strings.Join(patched, "\n")), nil
}

func looksLikeUnifiedDiff(body string) bool {
	return strings.Contains(body, "\n@@") || strings.HasPrefix(body, "@@") || strings.Contains(body, "\n+++") || strings.Contains(body, "\n---")
}

func splitPatchLines(text string) []string {
	if text == "" {
		return nil
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return []string{""}
	}
	return strings.Split(text, "\n")
}

func applyUnifiedPatch(base []string, patch string) ([]string, error) {
	lines := splitPatchLines(patch)
	var out []string
	idx := 0
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
			continue
		}
		if !strings.HasPrefix(line, "@@") {
			continue
		}
		for i+1 < len(lines) {
			next := lines[i+1]
			if strings.HasPrefix(next, "@@") {
				break
			}
			i++
			switch {
			case strings.HasPrefix(next, " "):
				if idx < len(base) {
					out = append(out, base[idx])
					idx++
				} else {
					out = append(out, strings.TrimPrefix(next, " "))
				}
			case strings.HasPrefix(next, "-"):
				if idx < len(base) {
					idx++
				}
			case strings.HasPrefix(next, "+"):
				out = append(out, strings.TrimPrefix(next, "+"))
			case strings.HasPrefix(next, "\\"):
				// "\ No newline at end of file" marker.
			default:
				out = append(out, next)
			}
		}
	}
	for ; idx < len(base); idx++ {
		out = append(out, base[idx])
	}
	if len(out) == 0 && len(base) == 0 {
		return []string{}, nil
	}
	return out, nil
}
