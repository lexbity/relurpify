package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveAuthorName(t *testing.T) {
	name := ResolveAuthorName()
	require.NotEmpty(t, name)
	require.Equal(t, name, strings.TrimSpace(name))
}
