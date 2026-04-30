package tui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Messages emitted by InputBar.
type InputSubmittedMsg struct{ Value string }
type CommandInvokedMsg struct {
	Name string
	Args []string
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

// InputBar wraps textinput with prefix-by-tab, ↑↓ history, and inline /command palette.
type InputBar struct {
	input   textinput.Model
	history InputHistory
	palette []commandItem
	palSel  int
	palOpen bool
	width   int
	// searchMode suppresses global keys and shows a search placeholder.
	searchMode bool

	// File picker state
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
}

// NewInputBar creates a focused InputBar.
func NewInputBar() *InputBar {
	ti := textinput.New()
	ti.Placeholder = "Type a message or /help for commands"
	ti.Focus()
	return &InputBar{input: ti}
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

// PaletteState exposes the current command palette state so RootModel can
// render it as a first-class overlay.
func (b *InputBar) PaletteState() (bool, []commandItem, int) {
	items := append([]commandItem(nil), b.palette...)
	return b.palOpen && len(items) > 0, items, b.palSel
}

// IsFilePickerActive reports whether the file picker overlay is active.
func (b *InputBar) IsFilePickerActive() bool {
	return b.pickerActive && len(b.pickerResult.Results) > 0
}

// SetWidth sets the input width.
func (b *InputBar) SetWidth(w int) {
	b.width = w
	b.input.Width = max(10, w-4)
}

// Value returns the current text value.
func (b *InputBar) Value() string { return b.input.Value() }

// SetValue sets the input text.
func (b *InputBar) SetValue(v string) { b.input.SetValue(v) }

// SetSearchMode enters or exits search mode.
func (b *InputBar) SetSearchMode(on bool) {
	b.searchMode = on
	if on {
		b.input.Placeholder = "search messages..."
		b.input.SetValue("")
		b.input.Focus()
	} else {
		b.input.Placeholder = "Type a message or /help for commands"
	}
}

// SetFilePickerMode enters or exits file picker mode.
func (b *InputBar) SetFilePickerMode(on bool) {
	if on {
		b.input.Placeholder = "@ - select files or type path"
		b.input.SetValue("@")
		b.input.Focus()
	} else {
		b.input.Placeholder = "Type a message or /help for commands"
		b.input.SetValue("")
	}
}

// prefix returns the prompt prefix for the current context.
func (b *InputBar) prefix(tab TabID) string {
	if b.searchMode {
		return "/ "
	}
	switch tab {
	case TabSession:
		return "@ "
	case TabConfig:
		return "? "
	default:
		return "> "
	}
}

// Update processes key input and emits typed messages.
func (b *InputBar) Update(msg tea.Msg, activeTab TabID) (*InputBar, tea.Cmd) {
	switch msg := msg.(type) {
	case filePickerResultMsg:
		b.pickerResult = msg
		return b, nil

	case tea.KeyMsg:
		// Intercept global navigation keys only when input is empty.
		if b.input.Value() == "" && !b.palOpen && !b.searchMode {
			switch msg.String() {
			case "1", "2", "3", "4", "5", "6", "?", "ctrl+t", "ctrl+f":
				return b, func() tea.Msg { return GlobalKeyMsg{Key: msg.String()} }
			case "tab", "shift+tab":
				return b, func() tea.Msg { return GlobalKeyMsg{Key: msg.String()} }
			case "[", "]":
				return b, func() tea.Msg { return GlobalKeyMsg{Key: msg.String()} }
			}
		}

		switch msg.String() {
		case "ctrl+c", "ctrl+d":
			return b, func() tea.Msg { return GlobalKeyMsg{Key: msg.String()} }

		case "enter":
			// File picker: select current file
			if b.pickerActive && len(b.pickerResult.Results) > 0 && b.pickerSel < len(b.pickerResult.Results) {
				selectedFile := b.pickerResult.Results[b.pickerSel]
				// Insert selected file as @path token
				currentVal := b.input.Value()
				// Remove the @prefix query
				atIdx := strings.LastIndex(currentVal, "@")
				if atIdx >= 0 {
					beforeAt := currentVal[:atIdx]
					b.input.SetValue(beforeAt + "@" + selectedFile + " ")
					b.pickerActive = false
					b.pickerResult.Results = nil
					b.pickerSel = 0
					b.input.CursorEnd()
					return b, nil
				}
			}

			raw := strings.TrimSpace(b.input.Value())
			b.input.SetValue("")
			b.palOpen = false
			b.palette = nil
			b.palSel = 0
			b.pickerActive = false
			b.pickerResult.Results = nil
			b.pickerSel = 0
			if raw == "" {
				return b, nil
			}
			if strings.HasPrefix(raw, "/") {
				name, args := parseSlashCommand(raw)
				if name != "" {
					b.history.Push(raw)
					return b, func() tea.Msg { return CommandInvokedMsg{Name: name, Args: args} }
				}
			}
			b.history.Push(raw)
			return b, func() tea.Msg { return InputSubmittedMsg{Value: raw} }

		case "esc":
			if b.pickerActive {
				b.pickerActive = false
				b.pickerResult.Results = nil
				b.pickerSel = 0
				return b, nil
			}
			if b.palOpen {
				b.palOpen = false
				b.palette = nil
				b.input.SetValue("")
				return b, nil
			}
			if b.searchMode {
				b.SetSearchMode(false)
				return b, func() tea.Msg { return GlobalKeyMsg{Key: "esc"} }
			}

		case "up":
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

		case "down":
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

		case "tab":
			// File picker: select current file (same as enter)
			if b.pickerActive && len(b.pickerResult.Results) > 0 && b.pickerSel < len(b.pickerResult.Results) {
				selectedFile := b.pickerResult.Results[b.pickerSel]
				currentVal := b.input.Value()
				atIdx := strings.LastIndex(currentVal, "@")
				if atIdx >= 0 {
					beforeAt := currentVal[:atIdx]
					b.input.SetValue(beforeAt + "@" + selectedFile + " ")
					b.pickerActive = false
					b.pickerResult.Results = nil
					b.pickerSel = 0
					b.input.CursorEnd()
					return b, nil
				}
			}
			if b.palOpen && len(b.palette) > 0 {
				b.autocomplete()
				b.updatePalette()
				return b, nil
			}
		}

		// Pass through to textinput.
		var cmd tea.Cmd
		b.input, cmd = b.input.Update(msg)

		// Check if we should open the command palette or file picker.
		val := b.input.Value()
		if strings.HasPrefix(val, "/") {
			b.palOpen = true
			b.updatePalette()
			b.pickerActive = false
			b.pickerResult.Results = nil
		} else if strings.Contains(val, "@") {
			// Find the last @ and get the text after it
			atIdx := strings.LastIndex(val, "@")
			if atIdx >= 0 {
				afterAt := val[atIdx+1:]
				// Only activate picker if @ is not preceded by another word character
				isTokenStart := atIdx == 0 || !isWordChar(rune(val[atIdx-1]))
				if isTokenStart && b.workspace != "" && b.runtime != nil {
					b.pickerActive = true
					b.pickerQuery = "@" + afterAt
					b.pickerSel = 0
					return b, filePickerQueryCmd(b.runtime, b.workspace, "@"+afterAt)
				}
			}
			b.palOpen = false
			b.palette = nil
		} else {
			b.palOpen = false
			b.palette = nil
			b.pickerActive = false
			b.pickerResult.Results = nil
		}
		return b, cmd
	}

	// Pass through blink tick etc.
	var cmd tea.Cmd
	b.input, cmd = b.input.Update(msg)
	return b, cmd
}

func (b *InputBar) autocomplete() {
	if len(b.palette) == 0 {
		return
	}
	if b.palSel < 0 || b.palSel >= len(b.palette) {
		b.palSel = 0
	}
	item := b.palette[b.palSel]
	v := "/" + item.Name
	if strings.Contains(item.Usage, " ") {
		v += " "
	}
	b.input.SetValue(v)
	b.input.CursorEnd()
}

func (b *InputBar) updatePalette() {
	query := strings.TrimPrefix(b.input.Value(), "/")
	if f := strings.Fields(query); len(f) > 0 {
		query = f[0]
	}

	// Get candidate commands: use registry for context-aware filtering if available.
	var candidates []Command
	if b.cmdReg != nil && b.ctxTab != "" {
		candidates = b.cmdReg.Match("", b.ctxTab, b.ctxSub)
	} else {
		candidates = listCommandsSorted()
	}

	var items []commandItem
	for _, cmd := range candidates {
		score := 0
		if query != "" {
			ok, s := fuzzyMatchScore(query, cmd.Name)
			if ok {
				score = s
			} else if ok2, s2 := fuzzyMatchScore(query, cmd.Usage); ok2 {
				score = s2 - 2
			} else {
				continue
			}
		}
		items = append(items, commandItem{Name: cmd.Name, Usage: cmd.Usage, Description: cmd.Description, Score: score})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score == items[j].Score {
			return items[i].Name < items[j].Name
		}
		return items[i].Score > items[j].Score
	})
	if len(items) > commandPaletteRows {
		items = items[:commandPaletteRows]
	}
	b.palette = items
	if b.palSel >= len(items) {
		b.palSel = 0
	}
}

// View renders the input bar (prefix + textinput + hint).
func (b *InputBar) View(activeTab TabID, streaming bool) string {
	prefix := inputPrefixStyle.Render(b.prefix(activeTab))

	var hint string
	if streaming {
		hint = dimStyle.Render(" streaming…  pgup/down scroll | ctrl+c quit")
	} else if b.pickerActive && len(b.pickerResult.Results) > 0 {
		hint = dimStyle.Render(" enter select | esc cancel | ↑↓ navigate")
	} else if b.palOpen && len(b.palette) > 0 {
		hint = dimStyle.Render(" enter run | esc cancel | tab complete | ↑↓ select")
	} else if b.searchMode {
		hint = dimStyle.Render(" esc exit search")
	} else {
		hint = dimStyle.Render(" / commands | @ context | ctrl+f search | ? help")
	}

	content := prefix + b.input.View()
	if hint != "" {
		content += " " + hint
	}
	bar := inputBarNewStyle.Width(b.width).Render(content)

	if b.pickerActive && len(b.pickerResult.Results) > 0 {
		return b.renderPicker() + "\n" + bar
	}
	return bar
}

func (b *InputBar) renderPicker() string {
	lines := []string{panelHeaderStyle.Render("Files")}
	for i, file := range b.pickerResult.Results {
		label := file
		if i == b.pickerSel {
			label = panelItemActiveStyle.Render(label)
		} else {
			label = panelItemStyle.Render(label)
		}
		lines = append(lines, label)
	}
	return panelStyle.Width(b.width).Render(strings.Join(lines, "\n"))
}

func parseSlashCommand(input string) (string, []string) {
	parts := strings.Fields(input)
	if len(parts) == 0 || !strings.HasPrefix(parts[0], "/") {
		return "", nil
	}
	name := strings.TrimPrefix(parts[0], "/")
	return name, parts[1:]
}

// isWordChar checks if a rune is a word character (letter, digit, or underscore).
func isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}
