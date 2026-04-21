package fs

import (
	"context"

	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/core"
)

type permissionCacheKey struct {
	action core.FileSystemAction
	path   string
}

type permissionCacheEntry struct {
	checked bool
	err     error
}

type fileAccessChecker func(ctx context.Context, action core.FileSystemAction, path string) error

type traversalPermissionCache struct {
	check  fileAccessChecker
	cached map[permissionCacheKey]permissionCacheEntry
}

func newTraversalPermissionCache(manager *authorization.PermissionManager, agentID string) *traversalPermissionCache {
	if manager == nil {
		return nil
	}
	return newTraversalPermissionCacheWithChecker(func(ctx context.Context, action core.FileSystemAction, path string) error {
		return manager.CheckFileAccess(ctx, agentID, action, path)
	})
}

func newTraversalPermissionCacheWithChecker(check fileAccessChecker) *traversalPermissionCache {
	if check == nil {
		return nil
	}
	return &traversalPermissionCache{
		check:  check,
		cached: make(map[permissionCacheKey]permissionCacheEntry),
	}
}

func (c *traversalPermissionCache) Check(ctx context.Context, action core.FileSystemAction, path string) error {
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
