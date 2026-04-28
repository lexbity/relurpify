package fs

import (
	"context"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type permissionCacheKey struct {
	action contracts.FileSystemAction
	path   string
}

type permissionCacheEntry struct {
	checked bool
	err     error
}

type fileAccessChecker func(ctx context.Context, action contracts.FileSystemAction, path string) error

type traversalPermissionCache struct {
	check  fileAccessChecker
	cached map[permissionCacheKey]permissionCacheEntry
}

// Note: newTraversalPermissionCache temporarily disabled during interface migration.
// The permission caching functionality should be reimplemented using FilePermissionChecker
// interface in a future update.
/*
func newTraversalPermissionCache(checker FilePermissionChecker, agentID string) *traversalPermissionCache {
	if checker == nil {
		return nil
	}
	return newTraversalPermissionCacheWithChecker(func(ctx context.Context, action contracts.FileSystemAction, path string) error {
		// Cache implementation would go here
		return nil
	})
}
*/

func newTraversalPermissionCacheWithChecker(check fileAccessChecker) *traversalPermissionCache {
	if check == nil {
		return nil
	}
	return &traversalPermissionCache{
		check:  check,
		cached: make(map[permissionCacheKey]permissionCacheEntry),
	}
}

func (c *traversalPermissionCache) Check(ctx context.Context, action contracts.FileSystemAction, path string) error {
	if c == nil {
		return nil
	}
	key := permissionCacheKey{action: action, path: path}
	if entry, ok := c.cached[key]; ok && entry.checked {
		return entry.err
	}
	err := c.check(ctx, action, path)
	c.cached[key] = permissionCacheEntry{checked: true, err: err}
	return err
}
