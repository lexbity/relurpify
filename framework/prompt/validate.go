package prompt

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

// validParadigms is the set of allowed paradigm tag values.
var validParadigms = map[string]bool{
	"react":      true,
	"pipeline":   true,
	"htn":        true,
	"goalcon":    true,
	"blackboard": true,
	"rewoo":      true,
}

// validKinds is the set of allowed kind values for tags and block metadata.
var validKinds = map[string]bool{
	"persona":    true,
	"task":       true,
	"capability": true,
	"format":     true,
	"constraint": true,
	"fragment":   true,
}

// validStabilities is the set of allowed stability values.
var validStabilities = map[string]bool{
	"experimental": true,
	"beta":         true,
	"stable":       true,
	"deprecated":   true,
}

// validateConfig performs structural validation on a parsed PromptConfig.
// It does not validate inheritance references (done after all files are loaded).
func validateConfig(cfg *PromptConfig) []ValidationIssue {
	var issues []ValidationIssue

	warn := func(blockID, msg string) {
		issues = append(issues, ValidationIssue{PromptID: cfg.ID, BlockID: blockID, Severity: SeverityWarning, Message: msg})
	}
	fail := func(blockID, msg string) {
		issues = append(issues, ValidationIssue{PromptID: cfg.ID, BlockID: blockID, Severity: SeverityError, Message: msg})
	}

	if cfg.APIVersion != "framework.prompt/v1" {
		fail("", "unknown apiVersion: "+cfg.APIVersion)
	}
	if cfg.ID == "" {
		fail("", "missing required field: id")
	}
	if cfg.Name == "" {
		fail("", "missing required field: name")
	}

	// Paradigm tag validation.
	for _, p := range cfg.Tags.Paradigm {
		if !validParadigms[p] {
			fail("", "unknown paradigm tag: "+p)
		}
	}

	// Kind validation.
	if cfg.Tags.Kind != "" && !validKinds[cfg.Tags.Kind] {
		warn("", "unknown kind tag: "+cfg.Tags.Kind)
	}

	// Stability validation.
	if cfg.Tags.Stability != "" && !validStabilities[cfg.Tags.Stability] {
		warn("", "unknown stability: "+cfg.Tags.Stability)
	}

	// Block-level checks.
	seenBlockIDs := make(map[string]bool)
	for i := range cfg.Blocks {
		b := &cfg.Blocks[i]
		if seenBlockIDs[b.ID] {
			fail(b.ID, "duplicate block id")
		}
		seenBlockIDs[b.ID] = true

		if b.From == SourceProvider && b.Provider == "" {
			fail(b.ID, "block has from: provider but no provider field")
		}
		if b.From == SourceStatic && b.Content == "" {
			warn(b.ID, "static block has empty content")
		}
		if b.From == SourceProvider && b.Content != "" {
			warn(b.ID, "provider block has content body (ignored at resolve time)")
		}
		if b.Kind != "" && !validKinds[b.Kind] {
			warn(b.ID, "unknown block kind: "+b.Kind)
		}
	}

	// Unused variable warnings: collect all variable references in blocks.
	if len(cfg.Variables) > 0 {
		used := collectUsedVars(cfg.Blocks)
		for name := range cfg.Variables {
			if !used[name] {
				warn("", "variable declared but not used: "+name)
			}
		}
	}

	return issues
}

// collectUsedVars returns the set of variable names referenced in block content.
func collectUsedVars(blocks []PromptBlock) map[string]bool {
	used := make(map[string]bool)
	for _, b := range blocks {
		scanVarRefs(b.Content, used)
	}
	return used
}

// scanVarRefs populates used with variable names found in {name} patterns.
func scanVarRefs(s string, used map[string]bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			i++ // escaped brace
			continue
		}
		if s[i] == '{' {
			j := i + 1
			for j < len(s) && s[j] != '}' && s[j] != '{' && s[j] != '\n' {
				j++
			}
			if j < len(s) && s[j] == '}' {
				name := s[i+1 : j]
				if name != "" {
					used[name] = true
				}
			}
			i = j
		}
	}
}
