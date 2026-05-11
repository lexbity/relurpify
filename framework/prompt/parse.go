package prompt

import (
	"fmt"
	"os"
	"strings"
)

// ParseResult bundles a parsed config with non-fatal warnings.
type ParseResult struct {
	Config   *PromptConfig
	Warnings []string
}

// ParseFile reads and parses a .prompt file from disk.
func ParseFile(path string) (*ParseResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	result, err := ParseBytes(data, path)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ParseBytes parses a .prompt file from raw bytes. sourcePath is informational.
func ParseBytes(data []byte, sourcePath string) (*ParseResult, error) {
	lines := splitLines(string(data))
	if len(lines) == 0 || lines[0] != "---" {
		return nil, fmt.Errorf("%s: missing front matter opening ---", sourcePath)
	}

	closeIdx := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			closeIdx = i
			break
		}
	}
	if closeIdx < 0 {
		return nil, fmt.Errorf("%s: missing front matter closing ---", sourcePath)
	}

	fmLines := lines[1:closeIdx]
	body := strings.Join(lines[closeIdx+1:], "\n")

	fm, warnings, err := parseFrontMatter(fmLines, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("%s: front matter: %w", sourcePath, err)
	}

	cfg := &PromptConfig{
		Schema:     fm.Schema,
		ID:         fm.ID,
		Tags:       fm.Tags,
		Variables:  fm.Variables,
		Body:       body,
		SourcePath: sourcePath,
	}

	return &ParseResult{Config: cfg, Warnings: warnings}, nil
}

// splitLines splits on \n, strips \r, and drops the trailing empty line added
// by the final newline.
func splitLines(s string) []string {
	raw := strings.Split(s, "\n")
	out := make([]string, 0, len(raw))
	for _, l := range raw {
		out = append(out, strings.TrimRight(l, "\r"))
	}
	if len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}
