package prep

import (
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/model"
)

const streamedOpenTag = "<streamed-context"
const streamedCloseTag = "</streamed-context>"

// extractStreamedSections pulls the rendered streamed-context sections out of
// the recorded model messages, in call order, by the renderer's open/close
// markers. Tests may also assert on the raw messages; this extraction exists
// so cases can pin the section's budget header and rank order without
// re-parsing message text.
func extractStreamedSections(calls [][]model.Message) []string {
	var sections []string
	for _, messages := range calls {
		rest := joinMessages(messages)
		for {
			start := strings.Index(rest, streamedOpenTag)
			if start < 0 {
				break
			}
			end := strings.Index(rest[start:], streamedCloseTag)
			if end < 0 {
				sections = append(sections, rest[start:])
				break
			}
			sections = append(sections, rest[start:start+end+len(streamedCloseTag)])
			rest = rest[start+end+len(streamedCloseTag):]
		}
	}
	return sections
}

func joinMessages(messages []model.Message) string {
	var b strings.Builder
	for _, msg := range messages {
		b.WriteString(msg.Content)
		b.WriteString("\n")
		for _, call := range msg.ToolCalls {
			b.WriteString(call.Name)
			b.WriteString(fmt.Sprintf(" %v", call.Args))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}
