package core

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type testRegistryCloser struct {
	closed int
	err    error
}

func (c *testRegistryCloser) Close() error {
	c.closed++
	return c.err
}

func TestObjectRegistryRemoveClosesRegisteredCloser(t *testing.T) {
	reg := NewObjectRegistry()
	closer := &testRegistryCloser{}
	handle := reg.Register(closer)

	reg.Remove(handle)

	require.Equal(t, 1, closer.closed)
	_, ok := reg.Lookup(handle)
	require.False(t, ok)
}

func TestObjectRegistryClearScopeClosesScopedValuesOnly(t *testing.T) {
	reg := NewObjectRegistry()
	scoped := &testRegistryCloser{}
	unscoped := &testRegistryCloser{}

	scopedHandle := reg.RegisterScoped("task-1", scoped)
	unscopedHandle := reg.Register(unscoped)

	reg.ClearScope("task-1")

	require.Equal(t, 1, scoped.closed)
	require.Equal(t, 0, unscoped.closed)
	_, ok := reg.Lookup(scopedHandle)
	require.False(t, ok)
	_, ok = reg.Lookup(unscopedHandle)
	require.True(t, ok)
}

func TestObjectRegistryCloseAllClosesEverythingAndJoinsErrors(t *testing.T) {
	reg := NewObjectRegistry()
	first := &testRegistryCloser{err: errors.New("first close failed")}
	second := &testRegistryCloser{}
	reg.Register(first)
	reg.RegisterScoped("task-2", second)

	err := reg.CloseAll()

	require.Error(t, err)
	require.ErrorContains(t, err, "first close failed")
	require.Equal(t, 1, first.closed)
	require.Equal(t, 1, second.closed)
}
