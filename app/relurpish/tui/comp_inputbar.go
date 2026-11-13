package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"

	"github.com/charmbracelet/x/ansi"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Messages emitted by InputBar.
type InputSubmittedMsg struct {
	Value    string
	Prefix   string
	Sequence uint64
}

type CommandInvokedMsg struct {
	Draft    string
	Name     string
	Args     []string
	Prefix   string
	Sequence uint64
}

type InputRejectedMsg struct{ Reason string }

type GlobalKeyMsg struct{ Key string }

const commandPaletteRows = 6

// commandItem is a palette entry for an autocomplete dropdown.
type commandItem struct {
	Name        string
	Usage       string
	Description string
	Score       int
}

// InputBar owns multiline request editing and contextual slash completion.
type InputBar struct {
	input     textarea.Model
	history   InputHistory
	palette   []commandItem
	palSel    int
	palOpen   bool
	palPrefix string
	palLabel  string
	width     int
	focused   bool

	// searchMode is entered explicitly with Ctrl+F; typed '?' remains request text.
	searchMode bool

	// File picker state.
	pickerActive bool
	pickerQuery  string
	pickerResult filePickerResultMsg
	pickerSel    int
	workspace    string
	parent       context.Context
	pickerEpoch  uint64
	pickerEdit   uint64
	pickerCaret  int
	runtime      RuntimeAdapter

	// Context-aware command registry.
	cmdReg       *CommandRegistry
	ctxTab       TabID
	ctxSub       SubTabID
	gated        bool
	savedRequest *string
	editSequence uint64
	historyDraft *string
	// Theme is the active semantic style source.
	th *theme.Theme
}

// NewInputBar creates a focused InputBar.
func NewInputBar() *InputBar {
	ti := textarea.New()
	ti.ShowLineNumbers = false
	ti.SetHeight(1)
	ti.CharLimit = 65536
	ti.Prompt = ""
	setInputHint(&ti, "Type here · / commands")
	ti.Focus()
	return &InputBar{input: ti, focused: true, th: theme.Default(), cmdReg: newHostCommandRegistry()}
}

// SetWorkspace sets the workspace path for file picker globbing.
func (b *InputBar) SetWorkspace(path string) {
	b.workspace = path
}

// SetRuntime sets the runtime adapter used to invoke capabilities for the file picker.
func (b *InputBar) SetRuntime(parent context.Context, rt RuntimeAdapter) {
	b.runtime, b.parent = rt, parent
}

// SetCommandRegistry sets the registry used for context-aware palette matching.
func (b *InputBar) SetCommandRegistry(reg *CommandRegistry) {
	b.cmdReg = reg
}

// SetContext updates the active tab/subtab used for palette filtering.
func (b *InputBar) SetContext(tab TabID, sub SubTabID) {
	b.ctxTab = tab
	b.ctxSub = sub
}

// SetGated marks request availability; commands remain usable during setup.
func (b *InputBar) SetGated(v bool) {
	b.gated = v
}

// PaletteState exposes the current command palette state so RootModel can
// render it as a first-class overlay.
func (b *InputBar) PaletteState() (bool, []commandItem, int, string) {
	items := append([]commandItem(nil), b.palette...)
	return b.palOpen && len(items) > 0, items, b.palSel, b.palLabel
}

// IsFilePickerActive reports whether the file picker overlay is active.
func (b *InputBar) IsFilePickerActive() bool {
	return b.pickerActive && len(b.pickerResult.Results) > 0
}

// SetWidth sets the input width.
func (b *InputBar) SetWidth(w int) {
	b.width = w
	b.input.SetWidth(max(1, w-4-17))
}

// SetFocused updates the visible focus state for the input bar.
func (b *InputBar) SetFocused(on bool) {
	b.focused = on
	if on {
		b.input.Focus()
		return
	}
	b.input.Blur()
}

// Focused reports whether the input bar currently owns focus.
func (b *InputBar) Focused() bool {
	return b != nil && b.focused
}

// Value returns the current text value.
func (b *InputBar) Value() string { return b.input.Value() }

// SetValue sets the input text.
func (b *InputBar) SetValue(v string) {
	if b.input.Value() != v {
		b.editSequence++
	}
	b.input.SetValue(v)
}

// SetSearchMode enters or exits explicit search mode.
func (b *InputBar) SetSearchMode(on bool) {
	b.searchMode = on
	if on {
		setInputHint(&b.input, "search...")
		b.input.SetValue("")
		b.SetFocused(true)
		return
	}
	setInputHint(&b.input, "Type here · / commands")
}

// SetFilePickerMode enters or exits file picker mode.
func (b *InputBar) SetFilePickerMode(on bool) {
	if on {
		setInputHint(&b.input, "@ - select files or type path")
		b.input.SetValue("@")
		b.SetFocused(true)
		return
	}
	setInputHint(&b.input, "Type here · / commands")
	b.input.SetValue("")
}

func setInputHint(model *textarea.Model, value string) {
	if model != nil {
		model.Placeholder = value
	}
}

func (b *InputBar) promptLabel(_ TabID, draft inputDraft) string { return draft.promptLabel }

// Accept clears only the exact draft accepted by the host. A blocked or stale
// submission cannot erase text typed while its message was pending.
func (b *InputBar) Accept(value string, sequence uint64) {
	if b.input.Value() == value && b.editSequence == sequence {
		if b.savedRequest != nil {
			b.SetValue(*b.savedRequest)
			b.savedRequest = nil
		} else {
			b.input.SetValue("")
		}
		b.updatePalette(parseInputDraft(b.input.Value(), b.ctxTab, false))
	}
}

// OpenCommands borrows the same bar for a command and retains the exact Euclo
// draft. Acceptance or Escape restores it; there is no second request composer.
func (b *InputBar) OpenCommands() {
	if b.savedRequest == nil && !strings.HasPrefix(strings.TrimSpace(b.input.Value()), "/") {
		draft := b.input.Value()
		b.savedRequest = &draft
		b.SetValue("/")
	}
	b.updatePalette(parseInputDraft(b.input.Value(), b.ctxTab, false))
}
func (b *InputBar) CancelCommands() bool {
	if b.savedRequest == nil {
		return false
	}
	b.SetValue(*b.savedRequest)
	b.savedRequest = nil
	b.updatePalette(parseInputDraft(b.input.Value(), b.ctxTab, false))
	return true
}

func (b *InputBar) updatePalette(draft inputDraft) {
	if !draft.commandMode {
		b.palOpen = false
		b.palette = nil
		b.palSel = 0
		b.palPrefix = ""
		b.palLabel = ""
		return
	}
	// A multiword path is completed as one catalog entry. Once arguments
	// follow a complete path there are no command-name suggestions.
	shouldOpen := true
	if b.cmdReg != nil {
		for _, command := range b.cmdReg.All() {
			if strings.HasPrefix(draft.paletteQuery, command.Name+" ") {
				shouldOpen = false
			}
		}
	}
	if !shouldOpen {
		b.palOpen = false
		b.palette = nil
		b.palSel = 0
		b.palPrefix = ""
		b.palLabel = ""
		return
	}
	items := commandPaletteItems(b.cmdReg, draft.paletteQuery, b.ctxTab, b.ctxSub)
	b.palette = items
	b.palOpen = len(items) > 0
	b.palSel = 0
	b.palPrefix = draft.prefix
	b.palLabel = draft.promptLabel
}

func (b *InputBar) caretByteOffset() int {
	value := b.input.Value()
	lines := strings.Split(value, "\n")
	row := min(b.input.Line(), len(lines)-1)
	info := b.input.LineInfo()
	column := min(info.StartColumn+info.ColumnOffset, len([]rune(lines[row])))
	offset := len(string([]rune(lines[row])[:column]))
	for _, line := range lines[:row] {
		offset += len(line) + 1
	}
	return offset
}

func (b *InputBar) requestFileCompletion() tea.Cmd {
	if b.runtime == nil || b.parent == nil || b.workspace == "" {
		return func() tea.Msg { return InputRejectedMsg{Reason: "File completion unavailable in this session"} }
	}
	raw := b.input.Value()
	if strings.HasPrefix(strings.TrimSpace(raw), "/") {
		return nil
	}
	caret := b.caretByteOffset()
	query, start, end, ok := filePickerTokenAt(raw, caret)
	if !ok {
		return func() tea.Msg {
			return InputRejectedMsg{Reason: "Place the cursor after an @ reference, then use Alt+F to complete it"}
		}
	}
	b.pickerEpoch++
	b.pickerActive, b.pickerQuery, b.pickerSel = true, query, 0
	b.pickerEdit, b.pickerCaret = b.editSequence, caret
	request := filePickerResultMsg{Query: query, Workspace: b.workspace, Request: b.pickerEpoch, Edit: b.editSequence, Start: start, End: end}
	return filePickerQueryCmd(b.parent, b.runtime, request)
}

func (b *InputBar) completePaletteSelection() {
	if !b.palOpen || len(b.palette) == 0 {
		return
	}
	if b.palSel < 0 || b.palSel >= len(b.palette) {
		b.palSel = 0
	}
	item := b.palette[b.palSel]
	b.SetValue("/" + item.Name + " ")
	b.input.CursorEnd()
	b.palOpen = false
	b.palette = nil
	b.palSel = 0
	b.palPrefix = ""
	b.palLabel = ""
}

func (b *InputBar) completePickerSelection() {
	if !b.pickerActive || len(b.pickerResult.Results) == 0 {
		return
	}
	if b.pickerSel < 0 || b.pickerSel >= len(b.pickerResult.Results) {
		b.pickerSel = 0
	}
	result := b.pickerResult
	current := b.input.Value()
	if result.Edit != b.editSequence || result.Workspace != b.workspace || result.Request != b.pickerEpoch || b.caretByteOffset() != b.pickerCaret || result.End > len(current) {
		return
	}
	selectedFile := result.Results[b.pickerSel]
	b.SetValue(current[:result.Start] + "@" + selectedFile + current[result.End:])
	b.input.CursorEnd()
	b.pickerActive = false
	b.pickerResult.Results = nil
	b.pickerSel = 0
	b.pickerQuery = ""
}

// Update processes key input and emits typed messages.
func (b *InputBar) Update(msg tea.Msg, activeTab TabID) (*InputBar, tea.Cmd) {
	before := b.input.Value()
	defer func() {
		if b.input.Value() != before {
			b.editSequence++
		}
	}()
	switch msg := msg.(type) {
	case filePickerResultMsg:
		if !b.pickerActive || msg.Request != b.pickerEpoch || msg.Edit != b.editSequence || msg.Workspace != b.workspace || b.caretByteOffset() != b.pickerCaret {
			return b, nil
		}
		b.pickerResult = msg
		if msg.Err != nil {
			b.pickerActive = false
			return b, func() tea.Msg { return InputRejectedMsg{Reason: "File completion failed: " + msg.Err.Error()} }
		}
		if len(msg.Results) == 0 {
			b.pickerActive = false
			return b, func() tea.Msg {
				return InputRejectedMsg{Reason: "No matching file references; the request is unchanged"}
			}
		}
		return b, nil

	case tea.KeyMsg:
		if msg.Paste && len(b.input.Value())+len(string(msg.Runes)) > 65536 {
			return b, func() tea.Msg {
				return InputRejectedMsg{Reason: "Paste exceeds the 64 KiB request limit; draft preserved"}
			}
		}

		switch {
		case msg.String() == "alt+f":
			return b, b.requestFileCompletion()
		case msg.String() == "alt+enter":
			b.input.InsertString("\n")
			return b, nil

		case msg.String() == "enter":
			if b.pickerActive && len(b.pickerResult.Results) > 0 {
				b.completePickerSelection()
				return b, nil
			}
			if b.palOpen && len(b.palette) > 0 && strings.TrimSpace(b.input.Value()) != "/"+b.palette[b.palSel].Name {
				b.completePaletteSelection()
				return b, nil
			}
			raw := b.input.Value()
			b.palOpen = false
			b.palette = nil
			b.palSel = 0
			b.palPrefix = ""
			b.palLabel = ""
			b.pickerActive = false
			b.pickerResult.Results = nil
			b.pickerSel = 0
			b.pickerQuery = ""
			if strings.TrimSpace(raw) == "" {
				return b, nil
			}
			prefix, name, args, ok := parseCommandLine(raw)
			if ok {
				b.history.Push(raw)
				sequence := b.editSequence
				return b, func() tea.Msg {
					return CommandInvokedMsg{Draft: raw, Name: name, Args: args, Prefix: prefix, Sequence: sequence}
				}
			}
			if strings.HasPrefix(strings.TrimSpace(raw), "/") {
				return b, func() tea.Msg {
					return InputRejectedMsg{Reason: "Invalid command syntax; use JSON-quoted arguments and /help"}
				}
			}
			draft := parseInputDraft(raw, activeTab, b.searchMode)
			b.history.Push(raw)
			sequence := b.editSequence
			return b, func() tea.Msg {
				return InputSubmittedMsg{
					Value:    raw,
					Prefix:   draft.prefix,
					Sequence: sequence,
				}
			}

		case msg.String() == "esc":
			if b.pickerActive {
				b.pickerActive = false
				b.pickerResult.Results = nil
				b.pickerSel = 0
				b.pickerQuery = ""
				return b, nil
			}
			if b.palOpen {
				b.palOpen = false
				b.palette = nil
				b.palSel = 0
				b.palPrefix = ""
				b.palLabel = ""
				return b, nil
			}
			if b.searchMode {
				b.SetSearchMode(false)
				return b, func() tea.Msg { return GlobalKeyMsg{Key: "esc"} }
			}

		case msg.String() == "up":
			if b.pickerActive && b.pickerSel > 0 {
				b.pickerSel--
				return b, nil
			}
			if b.palOpen {
				b.palSel = max(0, b.palSel-1)
				return b, nil
			}
			if b.input.LineCount() > 1 {
				break
			}
			if b.historyDraft == nil {
				value := b.input.Value()
				b.historyDraft = &value
			}
			prev := b.history.Prev()
			if prev != "" {
				b.input.SetValue(prev)
				b.input.CursorEnd()
			}
			return b, nil

		case msg.String() == "down":
			if b.pickerActive && b.pickerSel < len(b.pickerResult.Results)-1 {
				b.pickerSel++
				return b, nil
			}
			if b.palOpen {
				b.palSel = min(len(b.palette)-1, b.palSel+1)
				return b, nil
			}
			if b.input.LineCount() > 1 {
				break
			}
			if b.historyDraft == nil {
				return b, nil
			}
			next := b.history.Next()
			if b.history.cursor == len(b.history.entries) {
				next = *b.historyDraft
				b.historyDraft = nil
			}
			b.input.SetValue(next)
			b.input.CursorEnd()
			return b, nil

		case msg.String() == "tab":
			if b.pickerActive && len(b.pickerResult.Results) > 0 {
				b.completePickerSelection()
				return b, nil
			}
			if b.palOpen && len(b.palette) > 0 {
				b.completePaletteSelection()
				return b, nil
			}
		}

		var cmd tea.Cmd
		b.input, cmd = b.input.Update(msg)
		raw := b.input.Value()
		draft := parseInputDraft(raw, activeTab, b.searchMode)
		b.updatePalette(draft)
		if b.pickerActive && (b.input.Value() != before || b.caretByteOffset() != b.pickerCaret) {
			b.pickerActive = false
			b.pickerEpoch++
			b.pickerResult.Results = nil
		}
		return b, cmd
	}

	var cmd tea.Cmd
	b.input, cmd = b.input.Update(msg)
	return b, cmd
}

// View renders the input bar and any active inline hint.
func (b *InputBar) View(activeTab TabID, streaming bool) string {
	draft := parseInputDraft(b.input.Value(), activeTab, b.searchMode)
	prompt := b.promptLabel(activeTab, draft)
	prefix := ""
	if prompt != "" {
		prefix = b.th.Active().Render(prompt + " ")
	}

	var hint string
	switch {
	case b.gated:
		hint = b.th.Dim().Render(" running · / commands available")
	case streaming:
		hint = b.th.Dim().Render(" streaming · /cancel · Ctrl+Q quit")
	case b.pickerActive && len(b.pickerResult.Results) > 0:
		hint = b.th.Dim().Render(" enter/tab select | esc cancel | ↑↓ navigate")
	case b.palOpen && len(b.palette) > 0:
		hint = b.th.Dim().Render(" enter/tab complete | esc cancel | ↑↓ select")
	case b.searchMode || draft.prefix == "?":
		hint = b.th.Dim().Render(" esc exit search | enter apply")
	default:
		hint = ""
	}

	content := prefix + terminalText(b.input.View())
	if hint != "" {
		content += " " + hint
	}
	barStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(b.th.Palette().Dim).
		Background(b.th.Palette().Surface).
		Padding(0, 1)
	if b.Focused() {
		barStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(b.th.Palette().Primary).
			Background(b.th.Palette().Background).
			Padding(0, 1)
	}
	return barStyle.Width(max(1, b.width-2)).Render(ansi.Truncate(content, max(1, b.width-4), ""))
}

// PickerView renders the active file picker overlay.
func (b *InputBar) PickerView() string {
	if !b.pickerActive || len(b.pickerResult.Results) == 0 {
		return ""
	}
	return b.renderPicker()
}

func (b *InputBar) renderPicker() string {
	lines := []string{b.th.Subhead().Render("Files")}
	for i, file := range b.pickerResult.Results {
		label := terminalText(file)
		if i == b.pickerSel {
			label = b.th.Active().Render(label)
		} else {
			label = b.th.Body().Render(label)
		}
		lines = append(lines, label)
	}
	return b.th.Panel().Width(b.width).Render(strings.Join(lines, "\n"))
}

// SetTheme sets the active semantic style source.
func (b *InputBar) SetTheme(th *theme.Theme) {
	if th != nil {
		b.th = th
	}
}
