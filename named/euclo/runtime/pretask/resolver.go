package pretask

import (
	"path/filepath"
	"strings"

	"github.com/lexcodex/relurpify/framework/contextmgr"
)

// FileResolver validates and normalizes file paths from user input.
// It wraps framework/contextmgr.ExtractFileReferences for @mention parsing.
type FileResolver struct {
	Workspace string // absolute workspace root

	// CheckFileAccess, when non-nil, is called for each resolved path. Paths that
	// return a non-nil error are moved to Skipped (not loaded). This is the hook
	// for PermissionManager.CheckFileAccess without creating a hard dependency.
	CheckFileAccess func(path string) error
}

// ResolvedFiles holds the output of a resolution pass.
type ResolvedFiles struct {
	Paths   []string // validated absolute paths within workspace
	Skipped []string // paths that failed validation (logged, not fatal)
}

// Resolve processes file picker selections and @mentions from a user response.
// - selections: UserResponse.Selections (file picker results)
// - text: UserResponse.Text (parsed for @-prefixed mentions)
// All paths are validated to be within the workspace root.
// Symlinks are not followed. Paths escaping workspace are dropped into Skipped.
func (r *FileResolver) Resolve(selections []string, text string) ResolvedFiles {
	allPaths := append([]string{}, selections...)
	allPaths = append(allPaths, contextmgr.ExtractFileReferences(text)...)

	var paths []string
	var skipped []string
	for _, raw := range allPaths {
		clean := filepath.Clean(strings.TrimSpace(raw))
		if clean == "" {
			continue
		}
		abs := clean
		if !filepath.IsAbs(clean) {
			abs = filepath.Join(r.Workspace, clean)
		}
		rel, err := filepath.Rel(r.Workspace, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			skipped = append(skipped, raw)
			continue
		}
		if r.CheckFileAccess != nil {
			if accessErr := r.CheckFileAccess(abs); accessErr != nil {
				skipped = append(skipped, abs)
				continue
			}
		}
		paths = append(paths, abs)
	}
	return ResolvedFiles{Paths: paths, Skipped: skipped}
}

// computeFileDelta returns the files added and removed relative to prior.
// Used to produce the incremental update for each turn.
func computeFileDelta(prior, current []string) (added, removed []string) {
	priorSet := make(map[string]struct{})
	for _, p := range prior {
		priorSet[p] = struct{}{}
	}
	currentSet := make(map[string]struct{})
	for _, c := range current {
		currentSet[c] = struct{}{}
	}
	for c := range currentSet {
		if _, ok := priorSet[c]; !ok {
			added = append(added, c)
		}
	}
	for p := range priorSet {
		if _, ok := currentSet[p]; !ok {
			removed = append(removed, p)
		}
	}
	return added, removed
}
