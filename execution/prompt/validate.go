package prompt

import (
	"strings"
)

// IssueSeverity classifies a validation finding.
type IssueSeverity int

const (
	SeverityWarning IssueSeverity = iota
	SeverityError
)

func (s IssueSeverity) String() string {
	switch s {
	case SeverityWarning:
		return "warning"
	case SeverityError:
		return "error"
	default:
		return "unknown"
	}
}

// ValidationIssue records a single validation finding for a prompt or provider.
type ValidationIssue struct {
	PromptID string
	BlockID  string // empty when issue is file-level
	Severity IssueSeverity
	Message  string
}

func (v ValidationIssue) Error() string {
	if v.BlockID != "" {
		return v.Severity.String() + ": [" + v.PromptID + "/" + v.BlockID + "] " + v.Message
	}
	return v.Severity.String() + ": [" + v.PromptID + "] " + v.Message
}

// validateConfig performs structural validation on a parsed PromptConfig.
func validateConfig(cfg *PromptConfig) []ValidationIssue {
	var issues []ValidationIssue

	if cfg == nil {
		return []ValidationIssue{{
			Severity: SeverityError,
			Message:  "prompt config is nil",
		}}
	}

	fail := func(msg string) {
		issues = append(issues, ValidationIssue{PromptID: cfg.ID, Severity: SeverityError, Message: msg})
	}
	warn := func(msg string) {
		issues = append(issues, ValidationIssue{PromptID: cfg.ID, Severity: SeverityWarning, Message: msg})
	}

	if cfg.Schema != "framework.prompt/v2" {
		fail("unknown schema: " + cfg.Schema)
	}
	if cfg.ID == "" {
		fail("missing required field: id")
	}
	if !validPromptID(cfg.ID) {
		fail("invalid id: " + cfg.ID)
	}

	seenTags := make(map[string]struct{}, len(cfg.Tags))
	for _, tag := range cfg.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			fail("tag values must not be empty")
			continue
		}
		if _, ok := seenTags[tag]; ok {
			continue
		}
		seenTags[tag] = struct{}{}
	}

	if strings.TrimSpace(cfg.Body) == "" {
		fail("prompt body is required")
	}

	resolved := make(map[string]string, len(cfg.Variables))
	for name, decl := range cfg.Variables {
		if !validIdentifier(name) {
			fail("invalid variable name: " + name)
		}
		if strings.TrimSpace(decl.Default) == "" {
			warn("empty default for variable: " + name)
		}
		resolved[name] = decl.Default
	}

	if strings.TrimSpace(cfg.Body) != "" {
		if _, err := renderMarkdownBody(cfg.Body, resolved); err != nil {
			fail(err.Error())
		}
		used := collectUsedVars(cfg.Body)
		for name := range resolved {
			if !used[name] {
				warn("variable declared but not used: " + name)
			}
		}
	}

	return issues
}

// collectUsedVars returns the set of variable names referenced in the body.
func collectUsedVars(body string) map[string]bool {
	return markdownReferencedVariables(body)
}

// ValidateStructuredMap checks that a structured prompt output contains the
// required keys before state mutation proceeds.
func ValidateStructuredMap(promptID, blockID string, value map[string]any, requiredKeys []string) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	add := func(message string) {
		issues = append(issues, ValidationIssue{
			PromptID: promptID,
			BlockID:  blockID,
			Severity: SeverityError,
			Message:  message,
		})
	}
	if value == nil {
		add("structured output is missing")
		return issues
	}
	for _, key := range requiredKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := value[key]; !ok {
			add("missing required field: " + key)
		}
	}
	return issues
}
