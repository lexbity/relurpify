package intake

import (
	"strings"
	"text/template"

	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	"codeburg.org/lexbit/relurpify/named/euclo/families"
)

// BuildStreamRequest creates a contextstream.Request from family selection.
// It accepts only family selection, instruction, max tokens, and mode — no file parameters.
// File context arrives via prior ingestion only.
func BuildStreamRequest(sel families.FamilySelection, instruction string, maxTokens int, mode contextstream.Mode) *contextstream.Request {
	// Get the family to retrieve its retrieval template
	// For now, we'll need the family registry to look up the template
	// This is a limitation - we'll need to pass the family or template directly
	// For Phase 5, we'll accept the template as a parameter instead
	return nil
}

// BuildStreamRequestWithTemplate creates a contextstream.Request with an explicit template.
// If an envelope is provided, its user files and session pins are encoded as anchors
// in the retrieval query so the compiler can preserve that context.
func BuildStreamRequestWithTemplate(templateStr, instruction string, envelope *TaskEnvelope, maxTokens int, mode contextstream.Mode) *contextstream.Request {
	if templateStr == "" {
		return nil
	}

	// Render the template with instruction
	queryText, err := renderTemplate(templateStr, instruction)
	if err != nil {
		// If template rendering fails, use instruction as-is
		queryText = instruction
	}

	return &contextstream.Request{
		Query: retrieval.RetrievalQuery{
			Text:    queryText,
			Anchors: buildQueryAnchors(envelope),
		},
		MaxTokens: maxTokens,
		Mode:      mode,
	}
}

func buildQueryAnchors(envelope *TaskEnvelope) []retrieval.AnchorRef {
	if envelope == nil {
		return nil
	}
	anchors := make([]retrieval.AnchorRef, 0, len(envelope.UserFiles)+len(envelope.SessionPins))
	for _, filePath := range envelope.UserFiles {
		filePath = strings.TrimSpace(filePath)
		if filePath == "" {
			continue
		}
		anchors = append(anchors, retrieval.AnchorRef{
			AnchorID:   "file:" + filePath,
			Term:       filePath,
			Definition: "user-selected file",
			Class:      "user_file",
			Active:     true,
		})
	}
	for _, filePath := range envelope.SessionPins {
		filePath = strings.TrimSpace(filePath)
		if filePath == "" {
			continue
		}
		anchors = append(anchors, retrieval.AnchorRef{
			AnchorID:   "pin:" + filePath,
			Term:       filePath,
			Definition: "session-pinned file",
			Class:      "session_pin",
			Active:     true,
		})
	}
	if len(anchors) == 0 {
		return nil
	}
	return anchors
}

// renderTemplate renders a template string with the instruction.
func renderTemplate(templateStr, instruction string) (string, error) {
	tmpl, err := template.New("query").Parse(templateStr)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	data := struct {
		Instruction string
	}{
		Instruction: instruction,
	}

	err = tmpl.Execute(&buf, data)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}
