package fs

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/governance/permissions"
)

func TestTraversalPermissionCacheMemoizesByActionAndPath(t *testing.T) {
	ctx := context.Background()
	counts := make(map[permissionCacheKey]int)
	cache := newTraversalPermissionCacheWithChecker(func(ctx context.Context, action permissions.FileSystemAction, path string) error {
		counts[permissionCacheKey{action: action, path: path}]++
		return nil
	})

	require.NotNil(t, cache)
	assert.NoError(t, cache.Check(ctx, permissions.FileSystemList, "/tmp/project"))
	assert.NoError(t, cache.Check(ctx, permissions.FileSystemList, "/tmp/project"))
	assert.NoError(t, cache.Check(ctx, permissions.FileSystemRead, "/tmp/project"))
	assert.NoError(t, cache.Check(ctx, permissions.FileSystemRead, "/tmp/project/main.go"))
	assert.NoError(t, cache.Check(ctx, permissions.FileSystemRead, "/tmp/project/main.go"))

	assert.Equal(t, 1, counts[permissionCacheKey{action: permissions.FileSystemList, path: "/tmp/project"}])
	assert.Equal(t, 1, counts[permissionCacheKey{action: permissions.FileSystemRead, path: "/tmp/project"}])
	assert.Equal(t, 1, counts[permissionCacheKey{action: permissions.FileSystemRead, path: "/tmp/project/main.go"}])
}

func TestTraversalPermissionCacheMemoizesErrors(t *testing.T) {
	ctx := context.Background()
	expected := errors.New("denied")
	calls := 0
	cache := newTraversalPermissionCacheWithChecker(func(ctx context.Context, action permissions.FileSystemAction, path string) error {
		calls++
		return expected
	})

	require.NotNil(t, cache)
	assert.ErrorIs(t, cache.Check(ctx, permissions.FileSystemRead, "/tmp/project/secret.txt"), expected)
	assert.ErrorIs(t, cache.Check(ctx, permissions.FileSystemRead, "/tmp/project/secret.txt"), expected)
	assert.Equal(t, 1, calls)
}
