package registry

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/governance/classification"
)

const maxPreviewBytes = 2048

// editRecord captures the extracted hunk metadata from a write-class
// capability invocation.
type editRecord struct {
	Path           string
	StepID         string
	Origin         string
	LinesAdded     int
	LinesRemoved   int
	UnifiedPreview string
	Truncated      bool
}

// taskIDFromState returns the executing task ID from the state, falling back to
// the supplied agent ID when the state is nil or carries no task ID.
func taskIDFromState(env ports.State, fallback string) string {
	if env != nil {
		if id := strings.TrimSpace(env.TaskID()); id != "" {
			return id
		}
	}
	return fallback
}

// stepIDFromState reads the conventional generic "step_id" working-state value
// when the host has stamped it. It deliberately does not depend on any
// named-agent (euclo) key namespace, keeping the capability layer agnostic.
func stepIDFromState(env ports.State) string {
	if env == nil {
		return ""
	}
	if v, ok := env.GetWorkingValue("step_id"); ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// isWriteClass reports whether the capability is a workspace-mutation class.
// It checks the effect class on the descriptor, falling back to a seed list
// of known write capability IDs.
func isWriteClass(desc descriptor.CapabilityDescriptor) bool {
	for _, ec := range desc.EffectClasses {
		if ec == classification.EffectClassFilesystemMutation {
			return true
		}
	}
	return desc.ID == "file_edit" || desc.ID == "file_write"
}

// editRecordFromInvocation extracts an edit record from a successful write-class
// invocation. Returns ok=false when the invocation isn't a recognised write
// pattern or when required fields are absent.
func editRecordFromInvocation(desc descriptor.CapabilityDescriptor, args map[string]any, result *ports.ToolResult) (editRecord, bool) {
	if result == nil || !result.Success {
		return editRecord{}, false
	}

	switch desc.ID {
	case "file_edit":
		return editRecordFromFileEdit(args)
	case "file_write":
		return editRecordFromFileWrite(args)
	default:
		// For unrecognised write-class capabilities, try a generic path arg.
		return editRecordFromGenericPath(args, desc.ID)
	}
}

func editRecordFromFileEdit(args map[string]any) (editRecord, bool) {
	path, _ := args["path"].(string)
	oldStr, _ := args["old_string"].(string)
	newStr, _ := args["new_string"].(string)
	if path == "" {
		return editRecord{}, false
	}

	added := countLines(newStr)
	removed := countLines(oldStr)
	preview, truncated := capPreview(buildUnifiedHunk(oldStr, newStr))

	return editRecord{
		Path:           path,
		Origin:         "file_edit",
		LinesAdded:     added,
		LinesRemoved:   removed,
		UnifiedPreview: preview,
		Truncated:      truncated,
	}, true
}

func editRecordFromFileWrite(args map[string]any) (editRecord, bool) {
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)
	if path == "" {
		return editRecord{}, false
	}

	added := countLines(content)
	preview, truncated := capPreview(content)

	return editRecord{
		Path:           path,
		Origin:         "file_write",
		LinesAdded:     added,
		UnifiedPreview: preview,
		Truncated:      truncated,
	}, true
}

func editRecordFromGenericPath(args map[string]any, origin string) (editRecord, bool) {
	path, _ := args["path"].(string)
	if path == "" {
		return editRecord{}, false
	}
	return editRecord{
		Path:   path,
		Origin: origin,
	}, true
}

// buildUnifiedHunk creates a minimal unified-diff-style preview from old and
// new strings. For very small changes the output is concise; for large blocks
// it is capped by the caller.
func buildUnifiedHunk(oldStr, newStr string) string {
	if oldStr == newStr {
		return ""
	}
	oldLines := strings.Split(oldStr, "\n")
	newLines := strings.Split(newStr, "\n")

	var b strings.Builder
	b.WriteString("@@ -1,")
	b.WriteString(intStr(len(oldLines)))
	b.WriteString(" +1,")
	b.WriteString(intStr(len(newLines)))
	b.WriteString(" @@\n")
	for _, l := range oldLines {
		b.WriteString("-")
		b.WriteString(l)
		b.WriteString("\n")
	}
	for _, l := range newLines {
		b.WriteString("+")
		b.WriteString(l)
		b.WriteString("\n")
	}
	return b.String()
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

func intStr(n int) string {
	return fmt.Sprintf("%d", n)
}

// capPreview truncates a preview to maxPreviewBytes on a UTF-8 rune boundary
// (slicing mid-rune would emit invalid UTF-8). It returns the capped string and
// whether truncation occurred.
//
// Secret redaction is intentionally NOT done here: the capability wrapper runs
// the canonical runtime.RedactMetadataMap over the whole event metadata (which
// value-scans the preview via looksSensitiveValue), so a second, divergent
// scanner here would be duplicated logic and a second place to keep in sync.
func capPreview(preview string) (string, bool) {
	if len(preview) <= maxPreviewBytes {
		return preview, false
	}
	cut := maxPreviewBytes
	for cut > 0 && !utf8.RuneStart(preview[cut]) {
		cut--
	}
	return preview[:cut], true
}
