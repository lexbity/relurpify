package identity

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolutionErrorFormatting(t *testing.T) {
	inner := errors.New("lookup failed")
	err := WrapResolutionError(ResolutionErrorUnknownToken, "unknown bearer token", inner)
	require.Equal(t, "unknown bearer token", err.Error())
	require.ErrorIs(t, err, inner)
	require.Contains(t, fmt.Sprintf("%+v", err), "lookup failed")
}

func TestResolutionErrorWithDetail(t *testing.T) {
	err := NewResolutionError(ResolutionErrorAmbiguousSubject, "ambiguous subject").WithDetail("tenant_id", "tenant-1")
	require.Equal(t, ResolutionErrorAmbiguousSubject, err.Code)
	require.Equal(t, "tenant-1", err.Detail["tenant_id"])
}
