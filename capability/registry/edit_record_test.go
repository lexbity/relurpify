package registry

import (
	"strings"
	"testing"
	"unicode/utf8"

	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/governance/classification"
)

func TestIsWriteClass_EffectClassMutation(t *testing.T) {
	desc := descriptor.CapabilityDescriptor{
		ID:            "custom_writer",
		EffectClasses: []classification.EffectClass{classification.EffectClassFilesystemMutation},
	}
	if !isWriteClass(desc) {
		t.Fatal("expected isWriteClass=true for custom_writer with filesystem-mutation effect class")
	}
}

func TestIsWriteClass_SeedIDs(t *testing.T) {
	for _, id := range []string{"file_edit", "file_write"} {
		desc := descriptor.CapabilityDescriptor{ID: id}
		if !isWriteClass(desc) {
			t.Fatalf("expected isWriteClass=true for %s (seed ID)", id)
		}
	}
}

func TestIsWriteClass_NonWriteRejected(t *testing.T) {
	desc := descriptor.CapabilityDescriptor{
		ID:            "file_read",
		EffectClasses: []classification.EffectClass{classification.EffectClassProcessSpawn},
	}
	if isWriteClass(desc) {
		t.Fatal("expected isWriteClass=false for file_read")
	}
}

func TestEditRecord_FileEditBasic(t *testing.T) {
	desc := descriptor.CapabilityDescriptor{ID: "file_edit"}
	args := map[string]any{
		"path":       "demo.txt",
		"old_string": "hello\nworld\n",
		"new_string": "hello\nuniverse\n",
	}
	result := &ports.ToolResult{Success: true}

	rec, ok := editRecordFromInvocation(desc, args, result)
	if !ok {
		t.Fatal("expected ok=true for file_edit")
	}
	if rec.Path != "demo.txt" {
		t.Fatalf("Path = %q, want demo.txt", rec.Path)
	}
	if rec.LinesRemoved != 2 {
		t.Fatalf("LinesRemoved = %d, want 2", rec.LinesRemoved)
	}
	if rec.LinesAdded != 2 {
		t.Fatalf("LinesAdded = %d, want 2", rec.LinesAdded)
	}
	if !strings.Contains(rec.UnifiedPreview, "hello") {
		t.Fatalf("preview should contain old/new content")
	}
	if rec.Truncated {
		t.Fatal("expected no truncation")
	}
}

func TestEditRecord_FileEditAllAdded(t *testing.T) {
	desc := descriptor.CapabilityDescriptor{ID: "file_edit"}
	args := map[string]any{
		"path":       "new.txt",
		"old_string": "",
		"new_string": "line1\nline2\nline3\n",
	}
	result := &ports.ToolResult{Success: true}

	rec, ok := editRecordFromInvocation(desc, args, result)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if rec.LinesRemoved != 0 {
		t.Fatalf("LinesRemoved = %d, want 0", rec.LinesRemoved)
	}
	if rec.LinesAdded != 3 {
		t.Fatalf("LinesAdded = %d, want 3", rec.LinesAdded)
	}
}

func TestEditRecord_FileEditMissingPath(t *testing.T) {
	desc := descriptor.CapabilityDescriptor{ID: "file_edit"}
	args := map[string]any{
		"old_string": "old",
		"new_string": "new",
	}
	_, ok := editRecordFromInvocation(desc, args, &ports.ToolResult{Success: true})
	if ok {
		t.Fatal("expected ok=false when path is missing")
	}
}

func TestEditRecord_FileEditFailedResult(t *testing.T) {
	desc := descriptor.CapabilityDescriptor{ID: "file_edit"}
	_, ok := editRecordFromInvocation(desc, nil, &ports.ToolResult{Success: false})
	if ok {
		t.Fatal("expected ok=false for failed result")
	}
}

func TestEditRecord_FileWriteBasic(t *testing.T) {
	desc := descriptor.CapabilityDescriptor{ID: "file_write"}
	args := map[string]any{
		"path":    "output.txt",
		"content": "first\nsecond\nthird\n",
	}
	result := &ports.ToolResult{Success: true}

	rec, ok := editRecordFromInvocation(desc, args, result)
	if !ok {
		t.Fatal("expected ok=true for file_write")
	}
	if rec.Path != "output.txt" {
		t.Fatalf("Path = %q, want output.txt", rec.Path)
	}
	if rec.LinesAdded != 3 {
		t.Fatalf("LinesAdded = %d, want 3", rec.LinesAdded)
	}
	if rec.LinesRemoved != 0 {
		t.Fatalf("LinesRemoved = %d, want 0", rec.LinesRemoved)
	}
	if rec.Origin != "file_write" {
		t.Fatalf("Origin = %q, want file_write", rec.Origin)
	}
}

func TestEditRecord_FileWriteEmptyContent(t *testing.T) {
	desc := descriptor.CapabilityDescriptor{ID: "file_write"}
	args := map[string]any{
		"path":    "empty.txt",
		"content": "",
	}
	rec, ok := editRecordFromInvocation(desc, args, &ports.ToolResult{Success: true})
	if !ok {
		t.Fatal("expected ok=true even with empty content")
	}
	if rec.LinesAdded != 0 {
		t.Fatalf("LinesAdded = %d, want 0", rec.LinesAdded)
	}
}

func TestEditRecord_FileWriteMissingPath(t *testing.T) {
	desc := descriptor.CapabilityDescriptor{ID: "file_write"}
	_, ok := editRecordFromInvocation(desc, map[string]any{}, &ports.ToolResult{Success: true})
	if ok {
		t.Fatal("expected ok=false when path is missing")
	}
}

func TestEditRecord_UnknownCapabilityGenericPath(t *testing.T) {
	desc := descriptor.CapabilityDescriptor{ID: "custom_mutator"}
	args := map[string]any{
		"path": "some_file.go",
	}
	rec, ok := editRecordFromInvocation(desc, args, &ports.ToolResult{Success: true})
	if !ok {
		t.Fatal("expected ok=true for generic path-based capability")
	}
	if rec.Path != "some_file.go" {
		t.Fatalf("Path = %q", rec.Path)
	}
}

func TestEditRecord_PreviewCapped(t *testing.T) {
	desc := descriptor.CapabilityDescriptor{ID: "file_write"}
	big := strings.Repeat("line\n", 5000)
	args := map[string]any{
		"path":    "big.txt",
		"content": big,
	}
	rec, ok := editRecordFromInvocation(desc, args, &ports.ToolResult{Success: true})
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !rec.Truncated {
		t.Fatal("expected truncation for large content")
	}
	if len(rec.UnifiedPreview) > maxPreviewBytes {
		t.Fatalf("preview length = %d, want ≤ %d", len(rec.UnifiedPreview), maxPreviewBytes)
	}
}

func TestBuildUnifiedHunk(t *testing.T) {
	hunk := buildUnifiedHunk("old\n", "new\n")
	if !strings.Contains(hunk, "-old") {
		t.Fatalf("hunk missing -old: %q", hunk)
	}
	if !strings.Contains(hunk, "+new") {
		t.Fatalf("hunk missing +new: %q", hunk)
	}
}

func TestBuildUnifiedHunk_Identical(t *testing.T) {
	if hunk := buildUnifiedHunk("same\n", "same\n"); hunk != "" {
		t.Fatalf("expected empty hunk for identical, got %q", hunk)
	}
}

func TestCapPreview_ShortUnchanged(t *testing.T) {
	in := "func main() {\n\treturn \"hello\"\n}\n"
	got, truncated := capPreview(in)
	if truncated {
		t.Fatal("short preview should not be truncated")
	}
	if got != in {
		t.Fatalf("capPreview mutated a short string: %q", got)
	}
}

func TestCapPreview_TruncatesOnRuneBoundary(t *testing.T) {
	// A run of multibyte runes (3 bytes each) longer than the cap. A naive
	// byte slice would split a rune and emit invalid UTF-8.
	in := strings.Repeat("世", maxPreviewBytes) // 3 bytes/rune → 3×cap bytes
	got, truncated := capPreview(in)
	if !truncated {
		t.Fatal("expected truncation for oversized preview")
	}
	if len(got) > maxPreviewBytes {
		t.Fatalf("capPreview exceeded cap: %d > %d", len(got), maxPreviewBytes)
	}
	if !utf8.ValidString(got) {
		t.Fatal("capPreview produced invalid UTF-8 (split a rune)")
	}
}

func TestCapPreview_Empty(t *testing.T) {
	if got, truncated := capPreview(""); got != "" || truncated {
		t.Fatalf("capPreview('') = %q, %v", got, truncated)
	}
}

func TestCountLines(t *testing.T) {
	if got := countLines(""); got != 0 {
		t.Fatalf("countLines('') = %d", got)
	}
	if got := countLines("single"); got != 1 {
		t.Fatalf("countLines('single') = %d", got)
	}
	if got := countLines("a\nb\nc\n"); got != 3 {
		t.Fatalf("countLines('a\\nb\\nc\\n') = %d", got)
	}
	if got := countLines("a\nb\nc"); got != 3 {
		t.Fatalf("countLines('a\\nb\\nc') = %d", got)
	}
}
