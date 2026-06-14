package authorization

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/governance/permissions"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
)

// SetFilesystemGuardRoots configures filesystem roots that must never be
// matched by manifest permissions, plus roots that are excluded from the
// default workspace glob unless explicitly declared.
func (m *PermissionManager) SetFilesystemGuardRoots(protectedRoots, excludedRoots []string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fsProtectedRoots = normalizeFilesystemRoots(protectedRoots)
	m.fsExcludedRoots = normalizeFilesystemRoots(excludedRoots)
	m.fsPermCache = make(map[string]*permissions.FileSystemPermission)
}

// inflateScopes rewrites any workspace placeholders inside the declared
// filesystem permissions so later matching can operate on concrete paths.
func (m *PermissionManager) inflateScopes() {
	if m == nil || m.declared == nil {
		return
	}
	ws := filepath.ToSlash(filepath.Clean(m.basePath))
	for i := range m.declared.FileSystem {
		m.declared.FileSystem[i].Path = expandWorkspacePlaceholder(ws, m.declared.FileSystem[i].Path)
	}
}

// expandWorkspacePlaceholder replaces instances of ${workspace} markers with
// the actual base path, keeping relative globs compatible with matchers.
func expandWorkspacePlaceholder(workspace, pattern string) string {
	if pattern == "" {
		return pattern
	}
	replacer := strings.NewReplacer(
		"${workspace}", workspace,
		"${WORKSPACE}", workspace,
		"{{workspace}}", workspace,
		"{{WORKSPACE}}", workspace,
	)
	resolved := filepath.ToSlash(replacer.Replace(pattern))
	if filepath.IsAbs(resolved) {
		return resolved
	}
	if workspace == "" {
		return filepath.ToSlash(resolved)
	}
	resolved = strings.TrimPrefix(resolved, "./")
	return filepath.ToSlash(filepath.Join(workspace, resolved))
}

// normalizePath sanitizes user input by resolving relative segments and
// symlinks, and preventing traversal outside the workspace base.
//
// The resolved path is guaranteed to be within m.basePath after all
// symlink resolution. Paths that escape the workspace (including via
// symlinks pointing outside) return an error.
func (m *PermissionManager) normalizePath(path string) (string, error) {
	if path == "" {
		return "", errors.New("path required")
	}
	// Join with basePath for relative paths so we can always check the
	// result against the workspace boundary.
	resolved := path
	if !filepath.IsAbs(resolved) {
		if m.basePath == "" {
			return filepath.ToSlash(filepath.Clean(resolved)), nil
		}
		resolved = filepath.Join(m.basePath, resolved)
	}
	resolved = filepath.Clean(resolved)
	// Resolve symlinks for the longest existing prefix, then append any
	// non-existent suffix. This handles symlink-to-directory where the
	// final file component does not yet exist (e.g. new file creation
	// through a symlinked directory).
	if canonical, err := resolveCanonicalPath(resolved); err == nil {
		resolved = canonical
	}
	// Verify the resolved path is within the workspace.
	base := filepath.Clean(m.basePath)
	if resolved != base && !strings.HasPrefix(resolved, base+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes workspace %q", resolved, base)
	}
	return filepath.ToSlash(resolved), nil
}

func resolveCanonicalPath(path string) (string, error) {
	if path == "" {
		return "", errors.New("path required")
	}
	clean := filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		return filepath.Clean(resolved), nil
	}
	// Walk up from the path until we find an existing ancestor whose
	// symlinks can be resolved, then re-append the non-existent suffix.
	// This supports paths like /workspace/symlink_to_dir/newfile.go
	// where the symlink exists but the leaf file does not.
	suffix := []string{filepath.Base(clean)}
	parent := filepath.Dir(clean)
	for parent != "." && parent != "/" && parent != clean {
		if resolved, err := filepath.EvalSymlinks(parent); err == nil {
			// Re-append suffix from leaf to resolved ancestor
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		suffix = append(suffix, filepath.Base(parent))
		parent = filepath.Dir(parent)
	}
	// Nothing exists on this path — return the cleaned original
	return clean, nil
}

// findFilesystemPermission returns the first filesystem permission matching the
// requested action/path pair.
func (m *PermissionManager) findFilesystemPermission(action permissions.FileSystemAction, path string) *permissions.FileSystemPermission {
	if m == nil || m.declared == nil {
		return nil
	}
	normalized := filepath.ToSlash(filepath.Clean(path))
	if m.isFilesystemProtectedPath(normalized) {
		return nil
	}
	cacheKey := string(action) + ":" + normalized
	m.mu.RLock()
	if perm, ok := m.fsPermCache[cacheKey]; ok {
		m.mu.RUnlock()
		return perm
	}
	m.mu.RUnlock()
	var matched *permissions.FileSystemPermission
	protectedRoots := m.filesystemGuardRootsSnapshot()
	for _, perm := range m.declared.FileSystem {
		if perm.Action != action {
			continue
		}
		if matchGlob(perm.Path, normalized) {
			if len(protectedRoots) > 0 && m.isFilesystemExcludedPath(normalized, protectedRoots) && !filesystemPatternTargetsExcludedRoot(perm.Path, protectedRoots) {
				continue
			}
			permCopy := perm
			matched = &permCopy
			break
		}
	}
	m.mu.Lock()
	m.fsPermCache[cacheKey] = matched
	m.mu.Unlock()
	return matched
}

func normalizeFilesystemRoots(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	out := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		path = filepath.ToSlash(filepath.Clean(path))
		if path == "" || path == "." {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}

func (m *PermissionManager) filesystemGuardRootsSnapshot() []string {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.fsProtectedRoots) == 0 && len(m.fsExcludedRoots) == 0 {
		return nil
	}
	roots := make([]string, 0, len(m.fsProtectedRoots)+len(m.fsExcludedRoots))
	roots = append(roots, m.fsProtectedRoots...)
	roots = append(roots, m.fsExcludedRoots...)
	return roots
}

func (m *PermissionManager) isFilesystemProtectedPath(path string) bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, root := range m.fsProtectedRoots {
		if root != "" && pathWithinRoot(path, root) {
			return true
		}
	}
	return false
}

func (m *PermissionManager) isFilesystemExcludedPath(path string, roots []string) bool {
	for _, root := range roots {
		if root != "" && pathWithinRoot(path, root) {
			return true
		}
	}
	return false
}

func filesystemPatternTargetsExcludedRoot(pattern string, roots []string) bool {
	pattern = filepath.ToSlash(filepath.Clean(strings.TrimSpace(pattern)))
	if pattern == "" {
		return false
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		root = filepath.ToSlash(filepath.Clean(root))
		if pattern == root || strings.HasPrefix(pattern, root+"/") {
			return true
		}
	}
	return false
}

func pathWithinRoot(target, root string) bool {
	target = filepath.ToSlash(filepath.Clean(target))
	root = filepath.ToSlash(filepath.Clean(root))
	if root == "" {
		return false
	}
	if target == root {
		return true
	}
	return strings.HasPrefix(target, root+"/")
}

// CheckFileAccess validates filesystem access.
func (m *PermissionManager) CheckFileAccess(ctx context.Context, agentID string, action permissions.FileSystemAction, path string) error {
	if m == nil {
		return errors.New("permission manager missing")
	}
	clean, err := m.normalizePath(path)
	if err != nil {
		return m.deny(ctx, agentID, permissions.PermissionDescriptor{
			Type:     permissions.PermissionTypeFilesystem,
			Action:   string(action),
			Resource: path,
		}, fmt.Sprintf("path escapes workspace: %v", err))
	}
	perm := m.findFilesystemPermission(action, clean)
	if perm == nil {
		desc := permissions.PermissionDescriptor{
			Type:     permissions.PermissionTypeFilesystem,
			Action:   string(action),
			Resource: clean,
		}
		switch m.effectiveDefaultPolicy() {
		case "deny":
			return m.deny(ctx, agentID, desc, "not declared")
		default: // AgentPermissionAsk (Allow is rejected at registration time)
			desc.RequiresHITL = true
			return m.ensureGrant(ctx, agentID, desc)
		}
	}
	if perm.HITLRequired {
		if err := m.ensureGrant(ctx, agentID, permissions.PermissionDescriptor{
			Type:         permissions.PermissionTypeFilesystem,
			Action:       string(action),
			Resource:     perm.Path,
			RequiresHITL: true,
		}); err != nil {
			return err
		}
	}
	m.log(ctx, agentID, permissions.PermissionDescriptor{
		Type:     permissions.PermissionTypeFilesystem,
		Action:   string(action),
		Resource: clean,
	}, "granted", map[string]any{
		"pattern": perm.Path,
	})
	return nil
}

// StaticallyAllowsFileAccess reports whether the given filesystem access is
// permitted by a declared, non-HITL grant — without ever prompting for
// approval. It is the non-interactive counterpart to CheckFileAccess, intended
// for background/system operations (e.g. workspace indexing) that must skip
// disallowed paths rather than block on a HITL request that no human will
// answer. A path requiring HITL, lacking a static grant, or escaping the
// workspace returns false.
func (m *PermissionManager) StaticallyAllowsFileAccess(action permissions.FileSystemAction, path string) bool {
	if m == nil {
		return false
	}
	clean, err := m.normalizePath(path)
	if err != nil {
		return false
	}
	perm := m.findFilesystemPermission(action, clean)
	if perm == nil {
		return false
	}
	return !perm.HITLRequired
}

// CheckFilePermission implements permissions.FilePermissionChecker.
// It validates a file operation against the agent's file matrix.
func (m *PermissionManager) CheckFilePermission(ctx context.Context, agentID, basePath, action, absPath string, matrix AgentFileMatrix) error {
	rel := absPath
	if basePath != "" {
		if r, err := filepath.Rel(basePath, absPath); err == nil {
			rel = r
		}
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	rel = strings.TrimPrefix(rel, "./")
	perm := matrix.Write
	if action == "edit" {
		perm = matrix.Edit
	}
	if perm.DocumentationOnly && !strings.HasSuffix(strings.ToLower(rel), ".md") {
		return fmt.Errorf("file %s blocked: documentation_only enabled", rel)
	}
	decision, _ := DecideByPatterns(rel, perm.AllowPatterns, perm.DenyPatterns, permissions.AgentPermissionLevel(perm.Default))
	if perm.RequireApproval {
		decision = permissions.AgentPermissionAsk
	}
	switch decision {
	case permissions.AgentPermissionAllow:
		return nil
	case permissions.AgentPermissionDeny:
		return fmt.Errorf("file %s blocked: denied by file_permissions", rel)
	case permissions.AgentPermissionAsk:
		if m == nil {
			return fmt.Errorf("file %s blocked: approval required but permission manager missing", rel)
		}
		return m.RequireApproval(ctx, agentID, permissions.PermissionDescriptor{
			Type:         permissions.PermissionTypeHITL,
			Action:       fmt.Sprintf("file_matrix:%s", action),
			Resource:     rel,
			RequiresHITL: true,
		}, "file permission matrix", policy.GrantScopeOneTime, policy.RiskLevelMedium, 0)
	default:
		return nil
	}
}
