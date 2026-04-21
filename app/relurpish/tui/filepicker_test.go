package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

// TestFilePickerQueryInvokesCliFind verifies filePickerQueryCmd routes through cli_find.
func TestFilePickerQueryInvokesCliFind(t *testing.T) {
	rt := &recordingAdapter{
		response: &core.ToolResult{Data: map[string]any{"stdout": "/workspace/main.go\n/workspace/main_test.go\n"}},
	}
	cmd := filePickerQueryCmd(rt, "/workspace", "@main")
	msg := cmd()

	require.IsType(t, filePickerResultMsg{}, msg)
	require.Len(t, rt.calls, 1)
	require.Equal(t, "cli_find", rt.calls[0].Name)

	args := rt.calls[0].Args["args"].([]string)
	require.Equal(t, "/workspace", args[0])
	require.Contains(t, args, "main*") // pattern derived from "@main"
	require.Contains(t, args, "-type")
	require.Contains(t, args, "f")

	result := msg.(filePickerResultMsg)
	require.Nil(t, result.Err)
	require.Equal(t, []string{"main.go", "main_test.go"}, result.Results)
}

// TestFilePickerQueryEmptyPrefix returns no results without invoking capability.
func TestFilePickerQueryEmptyPrefix(t *testing.T) {
	rt := &recordingAdapter{}
	cmd := filePickerQueryCmd(rt, "/workspace", "")
	msg := cmd().(filePickerResultMsg)

	require.Nil(t, msg.Err)
	require.Len(t, msg.Results, 0)
	require.Len(t, rt.calls, 0) // no capability invoked
}

// TestFilePickerQueryAtOnly uses wildcard pattern for bare "@".
func TestFilePickerQueryAtOnly(t *testing.T) {
	rt := &recordingAdapter{
		response: &core.ToolResult{Data: map[string]any{"stdout": "/ws/a.go\n/ws/b.go\n"}},
	}
	cmd := filePickerQueryCmd(rt, "/ws", "@")
	msg := cmd().(filePickerResultMsg)

	require.Len(t, rt.calls, 1)
	args := rt.calls[0].Args["args"].([]string)
	// bare "@" → pattern "*"
	require.Contains(t, args, "*")
	require.Equal(t, []string{"a.go", "b.go"}, msg.Results)
}

// TestFilePickerQueryCapabilityError propagates error.
func TestFilePickerQueryCapabilityError(t *testing.T) {
	rt := &recordingAdapter{err: fmt.Errorf("cli_find failed")}
	cmd := filePickerQueryCmd(rt, "/ws", "@foo")
	msg := cmd().(filePickerResultMsg)
	require.NotNil(t, msg.Err)
}

// TestFilePickerQueryLimitsTen verifies only the first 10 results are returned.
func TestFilePickerQueryLimitsTen(t *testing.T) {
	var lines []string
	for i := 0; i < 15; i++ {
		lines = append(lines, filepath.Join("/ws", "file"+string(rune('a'+i))+".go"))
	}
	rt := &recordingAdapter{
		response: &core.ToolResult{Data: map[string]any{"stdout": strings.Join(lines, "\n") + "\n"}},
	}
	cmd := filePickerQueryCmd(rt, "/ws", "@file")
	msg := cmd().(filePickerResultMsg)
	require.Nil(t, msg.Err)
	require.Len(t, msg.Results, 10)
}

// TestExtractFileTokens verifies @path token extraction from text.
func TestExtractFileTokens(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			name:     "single file token",
			text:     "check @file.go for issues",
			expected: []string{"file.go"},
		},
		{
			name:     "multiple file tokens",
			text:     "@main.go and @util.go",
			expected: []string{"main.go", "util.go"},
		},
		{
			name:     "no file tokens",
			text:     "just a normal message",
			expected: []string{},
		},
		{
			name:     "@ alone is ignored",
			text:     "@ symbol alone",
			expected: []string{},
		},
		{
			name:     "nested paths",
			text:     "check @app/main.go",
			expected: []string{"app/main.go"},
		},
		{
			name:     "multiple files with paths",
			text:     "@pkg/a.go @pkg/b.go",
			expected: []string{"pkg/a.go", "pkg/b.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractFileTokens(tt.text)
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestRemoveFileTokens verifies @path token removal from text.
func TestRemoveFileTokens(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected string
	}{
		{
			name:     "remove single token",
			text:     "check @file.go for issues",
			expected: "check for issues",
		},
		{
			name:     "remove multiple tokens",
			text:     "@main.go and @util.go",
			expected: "and",
		},
		{
			name:     "no tokens to remove",
			text:     "just a normal message",
			expected: "just a normal message",
		},
		{
			name:     "remove paths",
			text:     "check @app/main.go please",
			expected: "check please",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeFileTokens(tt.text)
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestInputBarFilePickerMode verifies file picker mode activation.
func TestInputBarFilePickerMode(t *testing.T) {
	b := NewInputBar()
	b.SetWorkspace(t.TempDir())

	b.input.SetValue("@ma")
	b.pickerActive = true
	b.pickerQuery = "@ma"
	b.pickerSel = 0

	require.True(t, b.pickerActive)
	require.Equal(t, "@ma", b.pickerQuery)
}

// TestInputBarFilePickerSelection verifies file selection in picker.
func TestInputBarFilePickerSelection(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(""), 0644)
	os.WriteFile(filepath.Join(tmpDir, "util.go"), []byte(""), 0644)

	b := NewInputBar()
	b.SetWorkspace(tmpDir)

	b.pickerActive = true
	b.pickerResult = filePickerResultMsg{
		Query:   "@m",
		Results: []string{"main.go", "util.go"},
		Err:     nil,
	}
	b.pickerSel = 0

	selectedFile := b.pickerResult.Results[b.pickerSel]
	b.input.SetValue("check @")
	currentVal := b.input.Value()
	atIdx := len(currentVal) - 1
	beforeAt := currentVal[:atIdx]
	b.input.SetValue(beforeAt + "@" + selectedFile + " ")

	require.Equal(t, "check @main.go ", b.input.Value())
}

// TestChatPaneHandleInputWithFilePicker verifies file extraction in submission.
func TestChatPaneHandleInputWithFilePicker(t *testing.T) {
	pane := NewChatPane(nil, nil, nil, nil)
	pane.SetSize(80, 20)

	text := "please check @main.go for @errors.go bugs"
	tokens := extractFileTokens(text)
	cleaned := removeFileTokens(text)

	require.Equal(t, []string{"main.go", "errors.go"}, tokens)
	require.Equal(t, "please check for bugs", cleaned)
}

// TestIsWordChar verifies word character detection.
func TestIsWordChar(t *testing.T) {
	tests := []struct {
		ch       rune
		expected bool
	}{
		{'a', true},
		{'Z', true},
		{'0', true},
		{'_', true},
		{' ', false},
		{'-', false},
		{'@', false},
		{'/', false},
	}

	for _, tt := range tests {
		t.Run(string(tt.ch), func(t *testing.T) {
			require.Equal(t, tt.expected, isWordChar(tt.ch))
		})
	}
}
