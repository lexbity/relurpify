package agenttest

import (
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// DocumentCoversFileAction returns true if the document explicitly permits
// the given action on the given path. Path may be absolute or relative to workspace.
func DocumentCoversFileAction(
	doc *config.Document,
	action permissions.FileSystemAction,
	path, workspace string,
) bool {
	if doc == nil {
		return false
	}
	node, ok := doc.Section("permissions")
	if !ok {
		return false
	}
	ps, err := permissions.DecodeSection(node)
	if err != nil || ps == nil {
		return false
	}

	// Normalize path to absolute
	absPath := path
	if !filepath.IsAbs(path) && workspace != "" {
		absPath = filepath.Join(workspace, path)
	}
	absPath = filepath.Clean(absPath)

	// Check each filesystem permission
	for _, fsPerm := range ps.FileSystem {
		// Check if action matches (single Action field, not Actions array)
		if fsPerm.Action != action {
			continue
		}

		// Expand glob patterns (handle ${workspace} variable)
		pattern := expandPathPattern(fsPerm.Path, workspace)

		// Check if path matches pattern
		if pathMatchesGlob(absPath, pattern) {
			return true
		}
	}

	return false
}

// DocumentCoversExecutable returns true if the document declares the given binary.
func DocumentCoversExecutable(doc *config.Document, binary string) bool {
	if doc == nil {
		return false
	}
	node, ok := doc.Section("permissions")
	if !ok {
		return false
	}
	ps, err := permissions.DecodeSection(node)
	if err != nil || ps == nil {
		return false
	}

	// Normalize binary name (remove path)
	binaryName := filepath.Base(binary)

	for _, exec := range ps.Executables {
		// Check exact match (Binary field, not Name)
		if exec.Binary == binaryName {
			return true
		}
		// Check glob match
		if matched, _ := filepath.Match(exec.Binary, binaryName); matched {
			return true
		}
	}

	return false
}

// DocumentCoversNetworkCall returns true if the document declares the given host:port.
func DocumentCoversNetworkCall(doc *config.Document, host string, port int) bool {
	if doc == nil {
		return false
	}
	node, ok := doc.Section("permissions")
	if !ok {
		return false
	}
	ps, err := permissions.DecodeSection(node)
	if err != nil || ps == nil {
		return false
	}

	for _, net := range ps.Network {
		// Check host match (exact or glob)
		hostMatches := net.Host == host
		if !hostMatches {
			hostMatches, _ = filepath.Match(net.Host, host)
		}

		if !hostMatches {
			continue
		}

		// Check port match (0 means any port)
		if net.Port == 0 || net.Port == port {
			return true
		}
	}

	return false
}

// expandPathPattern expands ${workspace} and other variables in path patterns.
func expandPathPattern(pattern, workspace string) string {
	result := pattern
	if workspace != "" {
		result = strings.ReplaceAll(result, "${workspace}", workspace)
	}
	return result
}

// pathMatchesGlob checks if a path matches a glob pattern.
// Supports ** for recursive matching and * for single-level matching.
func pathMatchesGlob(path, pattern string) bool {
	// Handle ** recursive matching
	if strings.Contains(pattern, "**") {
		return matchRecursiveGlob(path, pattern)
	}

	// Standard glob matching
	matched, _ := filepath.Match(pattern, path)
	return matched
}

// matchRecursiveGlob handles ** patterns that match across directory boundaries.
func matchRecursiveGlob(path, pattern string) bool {
	// Split pattern into parts
	patternParts := strings.Split(pattern, string(filepath.Separator))
	pathParts := strings.Split(path, string(filepath.Separator))

	return matchPartsRecursive(patternParts, pathParts, 0, 0)
}

// matchPartsRecursive recursively matches pattern parts against path parts.
func matchPartsRecursive(patternParts, pathParts []string, pIdx, pathIdx int) bool {
	// Base case: both exhausted
	if pIdx >= len(patternParts) && pathIdx >= len(pathParts) {
		return true
	}

	// Pattern exhausted but path not: fail
	if pIdx >= len(patternParts) {
		return false
	}

	// Path exhausted but pattern not
	if pathIdx >= len(pathParts) {
		// Only match if remaining pattern parts are all **
		for i := pIdx; i < len(patternParts); i++ {
			if patternParts[i] != "**" {
				return false
			}
		}
		return true
	}

	patternPart := patternParts[pIdx]
	pathPart := pathParts[pathIdx]

	// Handle ** (recursive match)
	if patternPart == "**" {
		// Try matching ** with zero path parts (skip **)
		if matchPartsRecursive(patternParts, pathParts, pIdx+1, pathIdx) {
			return true
		}

		// Try matching ** with one or more path parts
		for i := pathIdx + 1; i <= len(pathParts); i++ {
			if matchPartsRecursive(patternParts, pathParts, pIdx+1, i) {
				return true
			}
		}

		return false
	}

	// Handle single * (match any single path component)
	if patternPart == "*" {
		return matchPartsRecursive(patternParts, pathParts, pIdx+1, pathIdx+1)
	}

	// Standard match
	matched, _ := filepath.Match(patternPart, pathPart)
	if !matched {
		return false
	}

	return matchPartsRecursive(patternParts, pathParts, pIdx+1, pathIdx+1)
}
