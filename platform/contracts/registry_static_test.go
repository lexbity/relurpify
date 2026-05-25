package contracts

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStaticToolRegistry(t *testing.T) {
	manifests := []*ToolManifest{
		{Name: "beta"},
		{Name: "alpha"},
	}

	reg := NewStaticToolRegistry(manifests)
	require.NotNil(t, reg)

	manifest, ok := reg.LookupTool("alpha")
	require.True(t, ok)
	require.Equal(t, "alpha", manifest.Name)

	ordered := reg.ListTools()
	require.Len(t, ordered, 2)
	require.Equal(t, "alpha", ordered[0].Name)
	require.Equal(t, "beta", ordered[1].Name)
}
