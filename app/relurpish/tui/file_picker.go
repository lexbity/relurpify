package tui

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	filePickerMaxRows = 7
)

type fileEntry struct {
	Path          string
	DisplayPath   string
	SizeBytes     int64
	TokenEstimate int
	Score         int
}

type filePickerState struct {
	all      []fileEntry
	filtered []fileEntry
	selected int
	loading  bool
	err      error
	root     string
}

type fileIndexMsg struct {
	root  string
	files []fileEntry
	err   error
}

func fileIndexCmd(root string) func() tea.Msg {
	return func() tea.Msg {
		files, err := buildFileIndex(root)
		return fileIndexMsg{root: root, files: files, err: err}
	}
}

func buildFileIndex(root string) ([]fileEntry, error) {
	root = filepath.Clean(root)
	ignoreDirs := map[string]struct{}{
		".git":          {},
		"node_modules":  {},
		"relurpify_cfg": {},
		"vendor":        {},
	}
	var entries []fileEntry
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
		entries = append(entries, fileEntry{
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

func (m Model) initFilePicker() (Model, tea.Cmd) {
	root := ""
	if m.session != nil {
		root = m.session.Workspace
	}
	if root == "" {
		root = "."
	}
	m.filePicker.loading = true
	m.filePicker.err = nil
	m.filePicker.selected = 0
	m.filePicker.all = nil
	m.filePicker.filtered = nil
	m.filePicker.root = root
	return m, fileIndexCmd(root)
}

func (m Model) updateFilePickerFilter(query string) Model {
	query = strings.TrimSpace(query)
	all := m.filePicker.all
	filtered := make([]fileEntry, 0, len(all))
	if query == "" {
		filtered = append(filtered, all...)
	} else {
		for _, entry := range all {
			if ok, score := fuzzyMatchScore(query, entry.DisplayPath); ok {
				entry.Score = score
				filtered = append(filtered, entry)
			}
		}
		sort.SliceStable(filtered, func(i, j int) bool {
			if filtered[i].Score == filtered[j].Score {
				return filtered[i].DisplayPath < filtered[j].DisplayPath
			}
			return filtered[i].Score > filtered[j].Score
		})
	}
	if len(filtered) > filePickerMaxRows {
		filtered = filtered[:filePickerMaxRows]
	}
	m.filePicker.filtered = filtered
	if m.filePicker.selected >= len(filtered) {
		m.filePicker.selected = 0
	}
	return m
}

func (m Model) filePickerSelection() (fileEntry, bool) {
	if len(m.filePicker.filtered) == 0 {
		return fileEntry{}, false
	}
	if m.filePicker.selected < 0 || m.filePicker.selected >= len(m.filePicker.filtered) {
		return fileEntry{}, false
	}
	return m.filePicker.filtered[m.filePicker.selected], true
}

func renderFileEntry(entry fileEntry) string {
	return fmt.Sprintf("%s | %s", entry.DisplayPath, formatSizeToken(entry.SizeBytes, entry.TokenEstimate))
}

func fileEntryForSelection(root, selection string) (fileEntry, error) {
	path := selection
	if root != "" && !filepath.IsAbs(path) {
		path = filepath.Join(root, selection)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fileEntry{}, err
	}
	if info.IsDir() {
		return fileEntry{}, fmt.Errorf("%s is a directory", selection)
	}
	size := info.Size()
	display := selection
	if root != "" {
		if rel, err := filepath.Rel(root, path); err == nil {
			display = filepath.ToSlash(rel)
		}
	}
	return fileEntry{
		Path:          path,
		DisplayPath:   display,
		SizeBytes:     size,
		TokenEstimate: estimateTokensFromBytes(size),
	}, nil
}
