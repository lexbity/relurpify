package fs

import (
	"context"
	"errors"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/contracts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTraversalPermissionCacheMemoizesByActionAndPath(t *testing.T) {
	ctx := context.Background()
	counts := make(map[permissionCacheKey]int)
	cache := newTraversalPermissionCacheWithChecker(func(ctx context.Context, action contracts.FileSystemAction, path string) error {
		counts[permissionCacheKey{action: action, path: path}]++
		return nil
	})

	require.NotNil(t, cache)
	assert.NoError(t, cache.Check(ctx, contracts.FileSystemList, "/tmp/project"))
	assert.NoError(t, cache.Check(ctx, contracts.FileSystemList, "/tmp/project"))
	assert.NoError(t, cache.Check(ctx, contracts.FileSystemRead, "/tmp/project"))
	assert.NoError(t, cache.Check(ctx, contracts.FileSystemRead, "/tmp/project/main.go"))
	assert.NoError(t, cache.Check(ctx, contracts.FileSystemRead, "/tmp/project/main.go"))

	assert.Equal(t, 1, counts[permissionCacheKey{action: contracts.FileSystemList, path: "/tmp/project"}])
	assert.Equal(t, 1, counts[permissionCacheKey{action: contracts.FileSystemRead, path: "/tmp/project"}])
	assert.Equal(t, 1, counts[permissionCacheKey{action: contracts.FileSystemRead, path: "/tmp/project/main.go"}])
}

func TestTraversalPermissionCacheMemoizesErrors(t *testing.T) {
	ctx := context.Background()
	expected := errors.New("denied")
	calls := 0
	cache := newTraversalPermissionCacheWithChecker(func(ctx context.Context, action contracts.FileSystemAction, path string) error {
		calls++
		return expected
	})

	require.NotNil(t, cache)
	assert.ErrorIs(t, cache.Check(ctx, contracts.FileSystemRead, "/tmp/project/secret.txt"), expected)
	assert.ErrorIs(t, cache.Check(ctx, contracts.FileSystemRead, "/tmp/project/secret.txt"), expected)
	assert.Equal(t, 1, calls)
}
