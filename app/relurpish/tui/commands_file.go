package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

// filePickerResultMsg carries the exact request identity and replacement span.
// Editing, moving the caret, canceling, or switching workspace invalidates it.
type filePickerResultMsg struct {
	Query, Workspace string
	Request, Edit    uint64
	Start, End       int
	Results          []string
	Err              error
}

func filePickerQueryCmd(parent context.Context, rt RuntimeAdapter, request filePickerResultMsg) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 2*time.Second)
		defer cancel()
		pattern := strings.TrimPrefix(request.Query, "@") + "*"
		result, err := rt.InvokeCapability(ctx, "cli_find", map[string]any{CapArgs: []string{request.Workspace, "-maxdepth", "4", "-name", pattern, "-type", "f", "-not", "-path", "*/.*", "-print0"}})
		if err != nil {
			request.Err = err
			return request
		}
		output := ""
		if result != nil {
			output, _ = result.Data["stdout"].(string)
		}
		if len(output) > 1<<20 || !utf8.ValidString(output) {
			request.Err = fmt.Errorf("file completion exceeds the 1 MiB UTF-8 result limit")
			return request
		}
		for _, filename := range strings.Split(output, "\x00") {
			if filename == "" {
				continue
			}
			relative, err := filepath.Rel(request.Workspace, filename)
			if err != nil || !filepath.IsLocal(relative) {
				continue
			}
			request.Results = append(request.Results, relative)
		}
		sort.Strings(request.Results)
		if len(request.Results) > 10 {
			request.Results = request.Results[:10]
		}
		return request
	}
}

// filePickerTokenAt uses UTF-8 byte spans, independent of terminal cell width.
// @ text is ordinary request text until completion is explicitly requested.
func filePickerTokenAt(input string, caret int) (string, int, int, bool) {
	if caret < 0 || caret > len(input) || !utf8.ValidString(input[:caret]) {
		return "", 0, 0, false
	}
	start := caret
	for start > 0 {
		r, size := utf8.DecodeLastRuneInString(input[:start])
		if unicode.IsSpace(r) {
			break
		}
		start -= size
	}
	end := caret
	for end < len(input) {
		r, size := utf8.DecodeRuneInString(input[end:])
		if unicode.IsSpace(r) {
			break
		}
		end += size
	}
	if start >= caret || input[start] != '@' {
		return "", 0, 0, false
	}
	return input[start:caret], start, end, true
}
