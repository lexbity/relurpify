package fs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	SecureFileMode os.FileMode = 0o600
	SecureDirMode  os.FileMode = 0o700
	PublicFileMode os.FileMode = 0o644
	PublicDirMode  os.FileMode = 0o755
)

func WriteFileSecure(path string, data []byte) error {
	return os.WriteFile(path, data, SecureFileMode)
}

func MkdirAllSecure(path string) error {
	return os.MkdirAll(path, SecureDirMode)
}

var ErrPathEscapesBase = errors.New("path escapes base")

// ResolveWithinBase resolves candidate against base, follows symlinks on the
// existing portion of the path, and rejects any result outside base.
//
// For non-existent leaves, the nearest existing parent is resolved and the leaf
// is rejoined so create/write flows can still be validated structurally.
func ResolveWithinBase(base, candidate string) (string, error) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return "", fmt.Errorf("candidate path required")
	}

	baseResolved := ""
	if strings.TrimSpace(base) != "" {
		baseAbs, err := filepath.Abs(filepath.Clean(base))
		if err != nil {
			return "", fmt.Errorf("resolve base %q: %w", base, err)
		}
		baseResolved, err = filepath.EvalSymlinks(baseAbs)
		if err != nil {
			return "", fmt.Errorf("resolve base %q: %w", base, err)
		}
	}

	absCandidate, err := absCandidatePath(baseResolved, candidate)
	if err != nil {
		return "", err
	}
	resolved, err := resolveCandidatePath(baseResolved, absCandidate)
	if err != nil {
		return "", err
	}
	if err := ensureWithinBase(baseResolved, resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

func absCandidatePath(baseResolved, candidate string) (string, error) {
	if filepath.IsAbs(candidate) {
		return filepath.Abs(filepath.Clean(candidate))
	}
	if baseResolved != "" {
		return filepath.Abs(filepath.Join(baseResolved, candidate))
	}
	return filepath.Abs(filepath.Clean(candidate))
}

func resolveCandidatePath(baseResolved, absCandidate string) (string, error) {
	resolved, err := filepath.EvalSymlinks(absCandidate)
	if err == nil {
		return filepath.Clean(resolved), nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("resolve path %q: %w", absCandidate, err)
	}
	parent := filepath.Dir(absCandidate)
	if parent == absCandidate {
		return filepath.Clean(absCandidate), nil
	}
	resolvedParent, err := resolveCandidatePath(baseResolved, parent)
	if err != nil {
		return "", err
	}
	return filepath.Clean(filepath.Join(resolvedParent, filepath.Base(absCandidate))), nil
}

func ensureWithinBase(baseResolved, candidate string) error {
	candidate = filepath.Clean(candidate)
	if baseResolved == "" {
		return nil
	}
	rel, err := filepath.Rel(baseResolved, candidate)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPathEscapesBase, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: %s", ErrPathEscapesBase, candidate)
	}
	return nil
}
