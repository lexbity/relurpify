package cfgload

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// Render returns a stable textual report for the inventory.
func (i Inventory) Render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "phase 1 ambient config inventory\n")
	fmt.Fprintf(&b, "root: .\n")
	fmt.Fprintf(&b, "files: %d\n", len(i.Files))
	fmt.Fprintf(&b, "findings: %d\n\n", len(i.Findings))

	lastFile := ""
	for _, finding := range i.Findings {
		if finding.File != lastFile {
			if lastFile != "" {
				b.WriteString("\n")
			}
			fmt.Fprintf(&b, "%s\n", finding.File)
			lastFile = finding.File
		}
		fmt.Fprintf(&b, "  %s:%d [%s] %s\n", finding.File, finding.Line, finding.Kind, finding.Snippet)
	}
	return strings.TrimSpace(b.String()) + "\n"
}

// WriteReport writes the rendered report to w.
func (i Inventory) WriteReport(w io.Writer) error {
	_, err := io.WriteString(w, i.Render())
	return err
}

// SortFindings ensures a deterministic order for ad hoc callers.
func (i *Inventory) SortFindings() {
	sort.Slice(i.Findings, func(a, b int) bool {
		x := i.Findings[a]
		y := i.Findings[b]
		if x.File != y.File {
			return x.File < y.File
		}
		if x.Line != y.Line {
			return x.Line < y.Line
		}
		if x.Symbol != y.Symbol {
			return x.Symbol < y.Symbol
		}
		return x.Kind < y.Kind
	})
}
