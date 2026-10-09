package knowledge

import "fmt"

// ContentAdmissionRequest carries the fields every admission path validates.
type ContentAdmissionRequest struct {
	Content     []byte
	ContentType string
	MaxTokens   int // 0 means unbounded
}

// ValidateContentAdmission applies the structural checks shared by every
// admission path: content and content type must be present, and the content
// must fit the configured token ceiling when bounded.
func ValidateContentAdmission(req ContentAdmissionRequest) error {
	if len(req.Content) == 0 {
		return fmt.Errorf("content is required")
	}
	if req.ContentType == "" {
		return fmt.Errorf("content_type is required")
	}
	if req.MaxTokens > 0 {
		estimated := len(req.Content) / 4 // 1 token ≈ 4 bytes for text
		if estimated > req.MaxTokens {
			return fmt.Errorf("content exceeds max size: %d tokens estimated", estimated)
		}
	}
	return nil
}

// SuspicionReason reports whether content looks suspicious and why. The
// persistence writer and the grounding service share this predicate so both
// admission gates behave identically.
func SuspicionReason(content []byte) (string, bool) {
	for _, b := range content {
		if b == 0 {
			return "binary content detected", true
		}
	}
	nonPrintable := 0
	for _, r := range string(content) {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			nonPrintable++
		}
	}
	if len(content) > 0 && float64(nonPrintable)/float64(len(content)) > 0.1 {
		return "high non-printable character ratio", true
	}
	return "", false
}
