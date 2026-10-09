package planner

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTruncatePlannerValue covers the planner's string-truncation helper,
// including the empty-limit, short, and truncated paths.
func TestTruncatePlannerValue(t *testing.T) {
	require.Empty(t, truncate("anything", 0))
	require.Empty(t, truncate("anything", -1))
	require.Equal(t, "short", truncate("short", 10))
	require.Equal(t, "abc…", truncate("abcdef", 3))
	require.Equal(t, "trimmed", truncate("  trimmed  ", 10))
}
