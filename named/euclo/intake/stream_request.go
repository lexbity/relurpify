package intake

import (
	"fmt"
	"strings"
	"text/template"

	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	"codeburg.org/lexbit/relurpify/named/euclo/families"
)

// BuildStreamRequest creates a contextstream.Request from family selection.
// It preserves a structured request and uses the family as a retrieval anchor.
func BuildStreamRequest(sel families.FamilySelection, instruction string, maxTokens int, mode contextstream.Mode) *contextstream.Request {
	instruction = strings.TrimSpace(instruction)
	familyID := strings.TrimSpace(sel.WinningFamily)
	if instruction == "" && familyID == "" {
		return nil
	}

	queryText := instruction
	if queryText == "" {
		queryText = familyID
	}
	if queryText == "" {
		return nil
	}

	anchors := make([]retrieval.AnchorRef, 0, 1)
	if familyID != "" {
		anchors = append(anchors, retrieval.AnchorRef{
			AnchorID:   "family:" + familyID,
			Term:       familyID,
			Definition: "winning family selection",
			Class:      "family",
			Active:     true,
		})
	}

	req := &contextstream.Request{
		Query: retrieval.RetrievalQuery{
			Text:    queryText,
			Anchors: anchors,
		},
		MaxTokens: maxTokens,
		Mode:      mode,
	}
	if familyID != "" {
		req.Metadata = map[string]any{
			"winning_family": familyID,
		}
	}
	if req.MaxTokens < 0 {
		req.MaxTokens = 0
	}
	if req.Metadata != nil && familyID != "" && req.Metadata["winning_family"] == "" {
		req.Metadata["winning_family"] = fmt.Sprint(familyID)
	}
	return req
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
