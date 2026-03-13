package agenttest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func resolvePathWithin(root, rel string) (string, error) {
	root = filepath.Clean(root)
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("root path required")
	}
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", fmt.Errorf("relative path required")
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", rel)
	}
	candidate := filepath.Join(root, filepath.FromSlash(rel))
	return ensurePathWithin(root, candidate)
}

func ensurePathWithin(root, candidate string) (string, error) {
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes root %s: %s", root, candidate)
	}
	return candidate, nil
}
