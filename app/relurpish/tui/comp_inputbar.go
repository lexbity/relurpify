package tui

import (
	"strings"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
	"github.com/charmbracelet/bubbles/textinput"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Messages emitted by InputBar.
type InputSubmittedMsg struct {
	Value  string
	Prefix string
}

type CommandInvokedMsg struct {
	Name   string
	Args   []string
	Prefix string
}

type GlobalKeyMsg struct{ Key string }

const commandPaletteRows = 6

// commandItem is a palette entry for an autocomplete dropdown.
type commandItem struct {
	Name        string
	Usage       string
	Description string
	Score       int
}

// InputBar wraps textinput with prefix-sensitive completion and file picker support.
type InputBar struct {
	input     textinput.Model
	history   InputHistory
	palette   []commandItem
	palSel    int
	palOpen   bool
	palPrefix string
	palLabel  string
	width     int
	focused   bool

	// searchMode is the explicit Ctrl+F mode. Typed `?` is still parsed from the buffer.
	searchMode bool

	// File picker state.
	pickerActive bool
	pickerQuery  string
	pickerResult filePickerResultMsg
	pickerSel    int
	workspace    string
	runtime      RuntimeAdapter

	// Context-aware command registry.
	cmdReg *CommandRegistry
	ctxTab TabID
	ctxSub SubTabID
	gated  bool
	// Theme is the active semantic style source.
	th *theme.Theme
}

// NewInputBar creates a focused InputBar.
func NewInputBar() *InputBar {
	ti := textinput.New()
	ti.Placeholder = "Type a message, /help, :git status, or ?search"
	ti.Focus()
	return &InputBar{input: ti, focused: true, th: theme.Default()}
}

// SetWorkspace sets the workspace path for file picker globbing.
func (b *InputBar) SetWorkspace(path string) {
	b.workspace = path
}

// SetRuntime sets the runtime adapter used to invoke capabilities for the file picker.
func (b *InputBar) SetRuntime(rt RuntimeAdapter) {
	b.runtime = rt
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

// SetGated marks whether the input bar should block prompt submissions (>).
// Slash commands, shell commands, and search remain active.
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
	b.input.Width = max(1, w-4)
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
func (b *InputBar) SetValue(v string) { b.input.SetValue(v) }

// SetSearchMode enters or exits explicit search mode.
func (b *InputBar) SetSearchMode(on bool) {
	b.searchMode = on
	if on {
		b.input.Placeholder = "search..."
		b.input.SetValue("")
		b.SetFocused(true)
		return
	}
	b.input.Placeholder = "Type a message, /help, :git status, or ?search"
}

// SetFilePickerMode enters or exits file picker mode.
func (b *InputBar) SetFilePickerMode(on bool) {
	if on {
		b.input.Placeholder = "@ - select files or type path"
		b.input.SetValue("@")
		b.SetFocused(true)
		return
	}
	b.input.Placeholder = "Type a message, /help, :git status, or ?search"
	b.input.SetValue("")
}

func (b *InputBar) promptLabel(activeTab TabID, draft inputDraft) string {
	if draft.promptLabel != "" {
		return draft.promptLabel
	}
	if b.searchMode {
		return "search"
	}
	if draft.prefix != "" {
		switch draft.prefix {
		case "/":
			return "slash"
		case ":":
			return "shell"
		case "?":
			return "search"
		case ">":
			if b.gated {
				return "running"
			}
			return "prompt"
		}
	}
	if b.gated {
		return "running"
	}
	if activeTab == TabChat {
		return "prompt"
	}
	return "filter"
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
	shouldOpen := draft.command == "" || (len(draft.args) == 0 && !draft.trailingSpace)
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

func (b *InputBar) updateFilePicker(raw string, draft inputDraft, activeTab TabID) tea.Cmd {
	if draft.commandMode || draft.prefix == "?" {
		b.pickerActive = false
		b.pickerResult.Results = nil
		b.pickerSel = 0
		b.pickerQuery = ""
		return nil
	}
	if draft.prefix == "" && activeTab != TabChat {
		b.pickerActive = false
		b.pickerResult.Results = nil
		b.pickerSel = 0
		b.pickerQuery = ""
		return nil
	}
	token, ok := filePickerTokenPrefix(raw)
	if !ok || b.workspace == "" || b.runtime == nil {
		b.pickerActive = false
		b.pickerResult.Results = nil
		b.pickerSel = 0
		b.pickerQuery = ""
		return nil
	}
	if token == b.pickerQuery && b.pickerActive {
		return nil
	}
	b.pickerActive = true
	b.pickerQuery = token
	b.pickerSel = 0
	return filePickerQueryCmd(b.runtime, b.workspace, token)
}

func (b *InputBar) completePaletteSelection() {
	if !b.palOpen || len(b.palette) == 0 {
		return
	}
	if b.palSel < 0 || b.palSel >= len(b.palette) {
		b.palSel = 0
	}
	item := b.palette[b.palSel]
	raw := b.input.Value()
	b.input.SetValue(replaceTokenAtPrefix(raw, b.palPrefix, item.Name))
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
	selectedFile := b.pickerResult.Results[b.pickerSel]
	currentVal := b.input.Value()
	if updated := filePickerReplaceToken(currentVal, selectedFile); updated != currentVal {
		b.input.SetValue(updated)
		b.input.CursorEnd()
	}
	b.pickerActive = false
	b.pickerResult.Results = nil
	b.pickerSel = 0
	b.pickerQuery = ""
}

// Update processes key input and emits typed messages.
func (b *InputBar) Update(msg tea.Msg, activeTab TabID) (*InputBar, tea.Cmd) {
	switch msg := msg.(type) {
	case filePickerResultMsg:
		b.pickerResult = msg
		return b, nil

	case tea.KeyMsg:
		if b.input.Value() == "" && !b.palOpen && !b.searchMode {
			if keyMatchesBinding(GlobalKeys.Tab1, msg.String()) ||
				keyMatchesBinding(GlobalKeys.Tab2, msg.String()) ||
				keyMatchesBinding(GlobalKeys.Tab3, msg.String()) ||
				keyMatchesBinding(GlobalKeys.Tab4, msg.String()) ||
				keyMatchesBinding(GlobalKeys.Tab5, msg.String()) ||
				keyMatchesBinding(GlobalKeys.Tab6, msg.String()) ||
				keyMatchesBinding(GlobalKeys.SearchMode, msg.String()) ||
				keyMatchesBinding(GlobalKeys.Help, msg.String()) ||
				keyMatchesBinding(GlobalKeys.FocusRegion1, msg.String()) ||
				keyMatchesBinding(GlobalKeys.AgentPicker, msg.String()) {
				return b, func() tea.Msg { return GlobalKeyMsg{Key: msg.String()} }
			}
			if msg.String() == "[" || msg.String() == "]" {
				return b, func() tea.Msg { return GlobalKeyMsg{Key: msg.String()} }
			}
		}

		switch {
		case keyMatchesBinding(GlobalKeys.Quit, msg.String()), keyMatchesBinding(GlobalKeys.Help, msg.String()):
			return b, func() tea.Msg { return GlobalKeyMsg{Key: msg.String()} }

		case msg.String() == "enter":
			if b.pickerActive && len(b.pickerResult.Results) > 0 {
				b.completePickerSelection()
				return b, nil
			}
			if b.palOpen && len(b.palette) > 0 {
				b.completePaletteSelection()
				return b, nil
			}
			raw := strings.TrimSpace(b.input.Value())
			b.input.SetValue("")
			b.palOpen = false
			b.palette = nil
			b.palSel = 0
			b.palPrefix = ""
			b.palLabel = ""
			b.pickerActive = false
			b.pickerResult.Results = nil
			b.pickerSel = 0
			b.pickerQuery = ""
			if raw == "" {
				return b, nil
			}
			prefix, name, args, ok := parseCommandLine(raw)
			if ok {
				b.history.Push(raw)
				return b, func() tea.Msg { return CommandInvokedMsg{Name: name, Args: args, Prefix: prefix} }
			}
			draft := parseInputDraft(raw, activeTab, b.searchMode)
			b.history.Push(raw)
			return b, func() tea.Msg {
				return InputSubmittedMsg{
					Value:  sanitizeSubmittedValue(raw, draft.prefix),
					Prefix: draft.prefix,
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
			if b.palOpen && b.palSel > 0 {
				b.palSel--
				return b, nil
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
			if b.palOpen && b.palSel < len(b.palette)-1 {
				b.palSel++
				return b, nil
			}
			next := b.history.Next()
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
		if pickerCmd := b.updateFilePicker(raw, draft, activeTab); pickerCmd != nil {
			return b, pickerCmd
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
	if b.gated {
		hint = b.th.Dim().Render(" running — > blocked | : and / active | ctrl+c quit")
	} else if streaming {
		hint = b.th.Dim().Render(" streaming…  pgup/down scroll | ctrl+c quit")
	} else if b.pickerActive && len(b.pickerResult.Results) > 0 {
		hint = b.th.Dim().Render(" enter/tab select | esc cancel | ↑↓ navigate")
	} else if b.palOpen && len(b.palette) > 0 {
		hint = b.th.Dim().Render(" enter/tab complete | esc cancel | ↑↓ select")
	} else if b.searchMode || draft.prefix == "?" {
		hint = b.th.Dim().Render(" esc exit search | enter apply")
	} else {
		hint = ""
	}

	content := prefix + b.input.View()
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
	return barStyle.Width(b.width).Render(content)
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
		label := file
		if i == b.pickerSel {
			label = b.th.Active().Render(label)
		} else {
			label = b.th.Body().Render(label)
		}
		lines = append(lines, label)
	}
	return b.th.Panel().Width(b.width).Render(strings.Join(lines, "\n"))
}

func sanitizeSubmittedValue(raw, prefix string) string {
	if prefix == "" {
		return raw
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if raw == prefix {
		return ""
	}
	if raw[0] == prefix[0] && len(raw) > 1 {
		return strings.TrimSpace(raw[1:])
	}
	return raw
}

// SetTheme sets the active semantic style source.
func (b *InputBar) SetTheme(th *theme.Theme) {
	if th != nil {
		b.th = th
	}
}
