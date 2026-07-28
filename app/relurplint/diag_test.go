package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestExitCodeEmpty(t *testing.T) {
	if code := ExitCode(nil); code != 0 {
		t.Fatalf("expected 0 for nil, got %d", code)
	}
	if code := ExitCode([]Diagnostic{}); code != 0 {
		t.Fatalf("expected 0 for empty, got %d", code)
	}
}

func TestExitCodeWarningOnly(t *testing.T) {
	diags := []Diagnostic{
		{Severity: SeverityWarning, Message: "a warning"},
	}
	if code := ExitCode(diags); code != 0 {
		t.Fatalf("expected 0 for warnings only, got %d", code)
	}
}

func TestExitCodeError(t *testing.T) {
	diags := []Diagnostic{
		{Severity: SeverityError, Message: "an error"},
	}
	if code := ExitCode(diags); code != 1 {
		t.Fatalf("expected 1 for error, got %d", code)
	}
}

func TestExitCodeMixed(t *testing.T) {
	diags := []Diagnostic{
		{Severity: SeverityWarning, Message: "warn"},
		{Severity: SeverityError, Message: "err"},
	}
	if code := ExitCode(diags); code != 1 {
		t.Fatalf("expected 1 for mixed, got %d", code)
	}
}

func TestRenderTextEmpty(t *testing.T) {
	var buf bytes.Buffer
	Render(nil, "text", &buf)
	if buf.Len() != 0 {
		t.Fatalf("expected empty output for nil, got %q", buf.String())
	}
}

func TestRenderTextSingle(t *testing.T) {
	diags := []Diagnostic{
		{
			Check:    "config",
			Code:     "test.code",
			Severity: SeverityError,
			Loc:      SourceLoc{File: "test.yaml", Line: 5},
			Message:  "something went wrong",
		},
	}
	var buf bytes.Buffer
	Render(diags, "text", &buf)
	out := buf.String()
	if !strings.Contains(out, "test.yaml:5") {
		t.Fatalf("expected file:line in output, got %q", out)
	}
	if !strings.Contains(out, "error") {
		t.Fatalf("expected severity in output, got %q", out)
	}
	if !strings.Contains(out, "something went wrong") {
		t.Fatalf("expected message in output, got %q", out)
	}
}

func TestRenderTextFileLevel(t *testing.T) {
	diags := []Diagnostic{
		{
			Severity: SeverityWarning,
			Loc:      SourceLoc{File: "test.yaml"},
			Message:  "file-level issue",
		},
	}
	var buf bytes.Buffer
	Render(diags, "text", &buf)
	out := buf.String()
	if !strings.Contains(out, "test.yaml") {
		t.Fatalf("expected filename, got %q", out)
	}
	if strings.Contains(out, ":0") {
		t.Fatalf("expected no line number for file-level, got %q", out)
	}
}

func TestRenderJSONShape(t *testing.T) {
	diags := []Diagnostic{
		{
			Check:    "tools",
			Code:     codeToolUnderdeclared,
			Severity: SeverityError,
			Loc:      SourceLoc{File: "tool.yaml", Line: 10},
			Message:  "missing risk_class",
		},
		{
			Check:    "config",
			Severity: SeverityWarning,
			Loc:      SourceLoc{File: "cfg.yaml"},
			Message:  "deprecated field",
		},
	}
	var buf bytes.Buffer
	Render(diags, "json", &buf)

	var parsed jsonOutput
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	if parsed.Summary.Total != 2 {
		t.Fatalf("expected 2 total, got %d", parsed.Summary.Total)
	}
	if parsed.Summary.Errors != 1 {
		t.Fatalf("expected 1 error, got %d", parsed.Summary.Errors)
	}
	if parsed.Summary.Warnings != 1 {
		t.Fatalf("expected 1 warning, got %d", parsed.Summary.Warnings)
	}
	if len(parsed.Diagnostics) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(parsed.Diagnostics))
	}
}

func TestRenderJSONEmpty(t *testing.T) {
	var buf bytes.Buffer
	Render([]Diagnostic{}, "json", &buf)

	var parsed jsonOutput
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed.Summary.Total != 0 {
		t.Fatalf("expected 0 total, got %d", parsed.Summary.Total)
	}
}

func TestSeverityFromString(t *testing.T) {
	if s := SeverityFromString("error"); s != SeverityError {
		t.Fatalf("expected error, got %v", s)
	}
	if s := SeverityFromString("warning"); s != SeverityWarning {
		t.Fatalf("expected warning, got %v", s)
	}
	if s := SeverityFromString("unknown"); s != SeverityWarning {
		t.Fatalf("expected warning for unknown, got %v", s)
	}
}

func TestSeverityString(t *testing.T) {
	if SeverityError.String() != "error" {
		t.Fatalf("expected 'error', got %q", SeverityError.String())
	}
	if SeverityWarning.String() != "warning" {
		t.Fatalf("expected 'warning', got %q", SeverityWarning.String())
	}
}
