package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// chatCanvasLines keeps reading coordinates independent of asynchronous token
// arrival. A scrolled canvas uses an absolute line anchor, not a moving tail.
func (m RootModel) chatCanvasLines() []string {
	var lines []string
	var conversation []ConversationLine
	if m.conversation != nil {
		conversation = m.frontendState.Messages
	}
	if m.chat != nil {
		for _, message := range m.chat.Messages() {
			conversation = append(conversation, ConversationLine{Role: string(message.Role), Text: message.Content.Text})
		}
	}
	for _, message := range conversation {
		if message.Text == "" {
			continue
		}
		lines = append(lines, message.Role)
		text := ansi.Hardwrap(terminalText(message.Text), max(1, m.width), true)
		lines = append(lines, strings.Split(text, "\n")...)
		lines = append(lines, "")
	}
	if len(lines) == 0 {
		return []string{"", "  What would you like to work on?", "", "  Plain text requests work from Euclo. /help opens commands."}
	}
	return lines
}
