package fs

import (
	"context"
	"errors"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTraversalPermissionCacheMemoizesByActionAndPath(t *testing.T) {
	ctx := context.Background()
	counts := make(map[permissionCacheKey]int)
	cache := newTraversalPermissionCacheWithChecker(func(ctx context.Context, action core.FileSystemAction, path string) error {
		counts[permissionCacheKey{action: action, path: path}]++
		return nil
	})

	require.NotNil(t, cache)
	assert.NoError(t, cache.Check(ctx, core.FileSystemList, "/tmp/project"))
	assert.NoError(t, cache.Check(ctx, core.FileSystemList, "/tmp/project"))
	assert.NoError(t, cache.Check(ctx, core.FileSystemRead, "/tmp/project"))
	assert.NoError(t, cache.Check(ctx, core.FileSystemRead, "/tmp/project/main.go"))
	assert.NoError(t, cache.Check(ctx, core.FileSystemRead, "/tmp/project/main.go"))

	assert.Equal(t, 1, counts[permissionCacheKey{action: core.FileSystemList, path: "/tmp/project"}])
	assert.Equal(t, 1, counts[permissionCacheKey{action: core.FileSystemRead, path: "/tmp/project"}])
	assert.Equal(t, 1, counts[permissionCacheKey{action: core.FileSystemRead, path: "/tmp/project/main.go"}])
}

func TestTraversalPermissionCacheMemoizesErrors(t *testing.T) {
	ctx := context.Background()
	expected := errors.New("denied")
	calls := 0
	cache := newTraversalPermissionCacheWithChecker(func(ctx context.Context, action core.FileSystemAction, path string) error {
		calls++
		return expected
	})

	require.NotNil(t, cache)
	assert.ErrorIs(t, cache.Check(ctx, core.FileSystemRead, "/tmp/project/secret.txt"), expected)
	assert.ErrorIs(t, cache.Check(ctx, core.FileSystemRead, "/tmp/project/secret.txt"), expected)
	assert.Equal(t, 1, calls)
}
