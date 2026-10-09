package contextdata

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOriginClassValid(t *testing.T) {
	for _, class := range []OriginClass{OriginUser, OriginTool, OriginLLM} {
		require.True(t, class.Valid(), "%q must be valid", class)
	}
	require.False(t, OriginClass("derivation").Valid())
	require.False(t, OriginClass("").Valid())
}

func TestMostRestrictive(t *testing.T) {
	require.Equal(t, OriginLLM, MostRestrictive(OriginLLM, OriginUser))
	require.Equal(t, OriginTool, MostRestrictive(OriginUser, OriginTool))
	require.Equal(t, OriginTool, MostRestrictive(OriginTool, OriginTool))
	require.Equal(t, OriginUser, MostRestrictive(OriginUser, OriginUser))
	// Unknown classes are maximally restrictive so trust can never be elevated.
	require.Equal(t, OriginClass("mystery"), MostRestrictive(OriginClass("mystery"), OriginLLM))
}
