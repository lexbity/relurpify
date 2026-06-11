package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type Severity int

const (
	SeverityWarning Severity = iota
	SeverityError
)

func (s Severity) String() string {
	switch s {
	case SeverityWarning:
		return "warning"
	case SeverityError:
		return "error"
	default:
		return fmt.Sprintf("severity(%d)", s)
	}
}

type SourceLoc struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

type Diagnostic struct {
	Check    string    `json:"check"`
	Code     string    `json:"code,omitempty"`
	Severity Severity  `json:"severity"`
	Loc      SourceLoc `json:"loc"`
	Message  string    `json:"message"`
}

type diagSummary struct {
	Total    int `json:"total"`
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
}

type jsonOutput struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
	Summary     diagSummary  `json:"summary"`
}

func Render(diags []Diagnostic, format string, w io.Writer) {
	switch format {
	case "json":
		renderJSON(diags, w)
	default:
		renderText(diags, w)
	}
}

func renderText(diags []Diagnostic, w io.Writer) {
	if len(diags) == 0 {
		return
	}
	for _, d := range diags {
		loc := d.Loc.File
		if d.Loc.Line > 0 {
			loc = fmt.Sprintf("%s:%d", loc, d.Loc.Line)
		}
		code := d.Code
		if code != "" {
			code = " " + code
		}
		_, _ = fmt.Fprintf(w, "%s: [%s%s] %s\n", loc, d.Severity, code, d.Message)
	}
}

func renderJSON(diags []Diagnostic, w io.Writer) {
	out := jsonOutput{
		Diagnostics: diags,
	}
	for _, d := range diags {
		out.Summary.Total++
		switch d.Severity {
		case SeverityError:
			out.Summary.Errors++
		default:
			out.Summary.Warnings++
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		panic(err)
	}
}

func ExitCode(diags []Diagnostic) int {
	for _, d := range diags {
		if d.Severity == SeverityError {
			return 1
		}
	}
	return 0
}

// SeverityFromString converts a severity name to its Severity value.
func SeverityFromString(s string) Severity {
	switch strings.ToLower(s) {
	case "error":
		return SeverityError
	default:
		return SeverityWarning
	}
}
