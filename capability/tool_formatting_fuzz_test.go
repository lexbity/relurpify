package capability

import (
	"testing"
)

func FuzzTryParseSingleToolCall(f *testing.F) {
	seeds := []string{
		`{"tool": "file_read", "arguments": {"path": "main.go"}}`,
		`{"name": "test_tool", "args": {"input": "hello"}}`,
		`{"tool_name": "cli_echo", "parameters": {"text": "msg"}}`,
		`{"tool": "file_write", "arguments": {"path": "/tmp/test", "content": "data"}}`,
		`{"tool": "complete", "arguments": {}}`,
		`{}`,
		`invalid json`,
		`{"tool": "", "arguments": null}`,
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, input string) {
		tryParseSingleToolCall(input)
	})
}
