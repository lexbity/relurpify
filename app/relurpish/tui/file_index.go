package tui

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// FileEntry represents a single indexable workspace file.
type FileEntry struct {
	Path          string
	DisplayPath   string
	SizeBytes     int64
	TokenEstimate int
	Score         int
}

// fileIndexMsg is the async message returned when file indexing completes.
type fileIndexMsg struct {
	root  string
	files []FileEntry
	err   error
}

// fileIndexCmd runs buildFileIndex asynchronously.
func fileIndexCmd(root string) tea.Cmd {
	return func() tea.Msg {
		files, err := buildFileIndex(root)
		return fileIndexMsg{root: root, files: files, err: err}
	}
}

// buildFileIndex walks root and returns sorted file entries, excluding common noise dirs.
func buildFileIndex(root string) ([]FileEntry, error) {
	root = filepath.Clean(root)
	ignoreDirs := map[string]struct{}{
		".git":          {},
		"node_modules":  {},
		"relurpify_cfg": {},
		"vendor":        {},
		"target":        {},
	}
	var entries []FileEntry
	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		base := filepath.Base(path)
		if d.IsDir() {
			if _, ok := ignoreDirs[base]; ok {
				return fs.SkipDir
			}
			if strings.HasPrefix(base, ".") && base != "." {
				return fs.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		size := info.Size()
		entries = append(entries, FileEntry{
			Path:          path,
			DisplayPath:   filepath.ToSlash(rel),
			SizeBytes:     size,
			TokenEstimate: estimateTokensFromBytes(size),
		})
		return nil
	}
	if err := filepath.WalkDir(root, walkFn); err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].DisplayPath < entries[j].DisplayPath
	})
	return entries, nil
}

// filterFileEntries returns entries matching query using fuzzy scoring.
func filterFileEntries(all []FileEntry, query string, limit int) []FileEntry {
	if query == "" {
		if limit > 0 && len(all) > limit {
			return all[:limit]
		}
		return all
	}
	var filtered []FileEntry
	for _, e := range all {
		if ok, score := fuzzyMatchScore(query, e.DisplayPath); ok {
			e.Score = score
			filtered = append(filtered, e)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Score == filtered[j].Score {
			return filtered[i].DisplayPath < filtered[j].DisplayPath
		}
		return filtered[i].Score > filtered[j].Score
	})
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered
}

// renderFileEntryLine formats a single file entry for display.
func renderFileEntryLine(e FileEntry) string {
	return fmt.Sprintf("%s | %s", e.DisplayPath, formatSizeToken(e.SizeBytes, e.TokenEstimate))
}
