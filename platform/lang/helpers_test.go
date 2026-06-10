package lang

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFirstNonEmptyLine_ReturnsFirstNonEmptyLine(t *testing.T) {
	got := FirstNonEmptyLine("  \nhello\nworld")
	assert.Equal(t, "hello", got)
}

func TestFirstNonEmptyLine_EmptyText(t *testing.T) {
	got := FirstNonEmptyLine("")
	assert.Empty(t, got)
}

func TestFirstNonEmptyLine_OnlyBlankLines(t *testing.T) {
	got := FirstNonEmptyLine("  \n\t\r\n  \n")
	assert.Empty(t, got)
}

func TestFirstNonEmptyLine_SingleLine(t *testing.T) {
	got := FirstNonEmptyLine("hello")
	assert.Equal(t, "hello", got)
}

func TestFirstNonEmptyLine_TrimsWhitespace(t *testing.T) {
	got := FirstNonEmptyLine("  hello world  \nnext")
	assert.Equal(t, "hello world", got)
}

func TestFirstNonEmptyLine_TrailingNewline(t *testing.T) {
	got := FirstNonEmptyLine("content\n")
	assert.Equal(t, "content", got)
}
