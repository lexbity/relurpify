package promptprovider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncate_MultibyteNotSplit(t *testing.T) {
	s := "hello" + "世" + "world"
	got := truncate(s, 7)
	assert.Equal(t, "hello"+"世"+"w"+"…", got)
}

func TestTruncate_Ellipsis(t *testing.T) {
	got := truncate("hello world", 5)
	assert.Equal(t, "hello"+"…", got)
}

func TestTruncate_ZeroMax(t *testing.T) {
	got := truncate("hello", 0)
	assert.Empty(t, got)
}

func TestTruncate_NegativeMax(t *testing.T) {
	got := truncate("hello", -1)
	assert.Empty(t, got)
}

func TestTruncate_ShorterThanMax(t *testing.T) {
	got := truncate("hi", 10)
	assert.Equal(t, "hi", got)
}

func TestTruncate_EmptyString(t *testing.T) {
	got := truncate("", 5)
	assert.Empty(t, got)
}

func TestTruncate_Trimmed(t *testing.T) {
	got := truncate("  hello  ", 10)
	assert.Equal(t, "hello", got)
}
