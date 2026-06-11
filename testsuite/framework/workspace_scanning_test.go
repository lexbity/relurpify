package framework

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// FileMetadata represents the metadata captured during workspace scanning.
type FileMetadata struct {
	Path         string
	Size         int64
	ModTime      time.Time
	IsDir        bool
	RelativePath string
}

// ScanWorkspace performs a deterministic scan of a workspace directory,
// returning file metadata for all discovered files.
// This uses the production file discovery logic from framework/ingestion's WorkspaceScanner
// but simplified for test purposes without the full pipeline overhead.
func ScanWorkspace(root string) ([]FileMetadata, error) {
	var files []FileMetadata

	// Use production-style file discovery logic (similar to WorkspaceScanner.discoverFiles)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the root directory itself
		if path == root {
			return nil
		}

		// Skip directories - we only want files
		if info.IsDir() {
			// Skip hidden directories and common exclude patterns (production behavior)
			name := info.Name()
			if strings.HasPrefix(name, ".") && name != "." {
				if name == ".git" || name == "node_modules" || name == "vendor" {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// Calculate relative path from root
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			relPath = path
		}
		// Normalize to forward slashes for consistency
		relPath = filepath.ToSlash(relPath)

		// Skip the manifests directory created by NewTestEnvironment
		if strings.Contains(relPath, "manifests") {
			return nil
		}

		metadata := FileMetadata{
			Path:         path,
			Size:         info.Size(),
			ModTime:      info.ModTime(),
			IsDir:        info.IsDir(),
			RelativePath: relPath,
		}

		files = append(files, metadata)
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Sort by relative path for deterministic ordering (production behavior)
	sort.Slice(files, func(i, j int) bool {
		return files[i].RelativePath < files[j].RelativePath
	})

	return files, nil
}

// TestSmallWorkspaceScan validates scanning a small, flat workspace.
func TestSmallWorkspaceScan(t *testing.T) {
	env := NewTestEnvironment(t)

	// Create a small workspace with known files
	workspaceFiles := map[string]string{
		"main.go":     "package main\n\nfunc main() {}\n",
		"README.md":   "# Test Project\n",
		"config.yaml": "key: value\n",
		".gitignore":  "*.log\n",
	}

	for path, content := range workspaceFiles {
		fullPath := filepath.Join(env.WorkspacePath, path)
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to create file %s: %v", path, err)
		}
	}

	// Scan the workspace
	files, err := ScanWorkspace(env.WorkspacePath)
	if err != nil {
		t.Fatalf("workspace scan failed: %v", err)
	}

	// Assert file count matches fixture expectation
	if len(files) != len(workspaceFiles) {
		t.Errorf("expected %d files, got %d", len(workspaceFiles), len(files))
	}

	// Verify each file was found with correct metadata
	for _, file := range files {
		if file.IsDir {
			t.Errorf("unexpected directory in scan results: %s", file.RelativePath)
		}

		// Verify file exists in fixture
		if _, ok := workspaceFiles[file.RelativePath]; !ok {
			t.Errorf("unexpected file in scan results: %s", file.RelativePath)
		}

		// Verify size is non-zero
		if file.Size == 0 {
			t.Errorf("file %s has zero size", file.RelativePath)
		}

		// Verify path is normalized
		if strings.Contains(file.RelativePath, "\\") {
			t.Errorf("relative path not normalized: %s", file.RelativePath)
		}

		// Verify mod time is set
		if file.ModTime.IsZero() {
			t.Errorf("file %s has zero mod time", file.RelativePath)
		}
	}

	// Verify ordering is deterministic by checking relative paths are sorted
	for i := 1; i < len(files); i++ {
		if files[i-1].RelativePath > files[i].RelativePath {
			t.Errorf("files not sorted: %s > %s", files[i-1].RelativePath, files[i].RelativePath)
		}
	}
}

// TestMixedWorkspaceScan validates scanning a workspace with mixed file types
// and nested structure.
func TestMixedWorkspaceScan(t *testing.T) {
	env := NewTestEnvironment(t)

	// Create a mixed-language workspace with directories
	workspaceStructure := map[string]string{
		"src/main.go":      "package main\n\nfunc main() {}\n",
		"src/helper.go":    "package src\n\nfunc Helper() {}\n",
		"python/script.py": "def main():\n    pass\n",
		"python/utils.py":  "def util():\n    pass\n",
		"config/app.yaml":  "app:\n  name: test\n",
		"docs/README.md":   "# Documentation\n",
		"data/input.txt":   "sample data\n",
		"Makefile":         "build:\n\tgo build\n",
		".env.example":     "VAR=value\n",
	}

	for path, content := range workspaceStructure {
		fullPath := filepath.Join(env.WorkspacePath, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create directory %s: %v", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to create file %s: %v", path, err)
		}
	}

	// Scan the workspace
	files, err := ScanWorkspace(env.WorkspacePath)
	if err != nil {
		t.Fatalf("workspace scan failed: %v", err)
	}

	// Assert file count matches fixture expectation
	if len(files) != len(workspaceStructure) {
		t.Errorf("expected %d files, got %d", len(workspaceStructure), len(files))
	}

	// Verify nested directory structure is preserved
	hasSrcFiles := false
	hasPythonFiles := false
	hasDocsFiles := false
	hasDataFiles := false

	for _, file := range files {
		switch {
		case strings.HasPrefix(file.RelativePath, "src/"):
			hasSrcFiles = true
		case strings.HasPrefix(file.RelativePath, "python/"):
			hasPythonFiles = true
		case strings.HasPrefix(file.RelativePath, "docs/"):
			hasDocsFiles = true
		case strings.HasPrefix(file.RelativePath, "data/"):
			hasDataFiles = true
		}

		// Verify file exists in fixture
		if _, ok := workspaceStructure[file.RelativePath]; !ok {
			t.Errorf("unexpected file in scan results: %s", file.RelativePath)
		}
	}

	if !hasSrcFiles {
		t.Error("expected src/ directory files")
	}
	if !hasPythonFiles {
		t.Error("expected python/ directory files")
	}
	if !hasDocsFiles {
		t.Error("expected docs/ directory files")
	}
	if !hasDataFiles {
		t.Error("expected data/ directory files")
	}
}

// TestNestedDirectoryScan validates scanning deeply nested directory structures.
func TestNestedDirectoryScan(t *testing.T) {
	env := NewTestEnvironment(t)

	// Create a deeply nested directory structure
	nestedStructure := map[string]string{
		"a/b/c/d/file1.txt": "content 1\n",
		"a/b/c/d/file2.txt": "content 2\n",
		"a/b/e/file3.txt":   "content 3\n",
		"a/f/file4.txt":     "content 4\n",
		"root.txt":          "content 5\n",
	}

	for path, content := range nestedStructure {
		fullPath := filepath.Join(env.WorkspacePath, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create directory %s: %v", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to create file %s: %v", path, err)
		}
	}

	// Scan the workspace
	files, err := ScanWorkspace(env.WorkspacePath)
	if err != nil {
		t.Fatalf("workspace scan failed: %v", err)
	}

	// Assert file count matches fixture expectation
	if len(files) != len(nestedStructure) {
		t.Errorf("expected %d files, got %d", len(nestedStructure), len(files))
	}

	// Verify nested paths are correctly normalized
	for _, file := range files {
		// Check that paths don't contain directory entries (we only want files)
		if file.IsDir {
			t.Errorf("unexpected directory in scan results: %s", file.RelativePath)
		}

		// Verify nested structure is preserved
		if !strings.Contains(file.RelativePath, "/") && file.RelativePath != "root.txt" {
			t.Errorf("expected nested path for file: %s", file.RelativePath)
		}

		// Verify file exists in fixture
		if _, ok := nestedStructure[file.RelativePath]; !ok {
			t.Errorf("unexpected file in scan results: %s", file.RelativePath)
		}
	}

	// Verify deepest nested files are found
	foundDeepest := false
	for _, file := range files {
		if strings.HasPrefix(file.RelativePath, "a/b/c/d/") {
			foundDeepest = true
			break
		}
	}
	if !foundDeepest {
		t.Error("expected to find files in deepest nested directory")
	}
}

// TestMalformedFileHandling validates that malformed or problematic files
// do not abort the scan of other files.
func TestMalformedFileHandling(t *testing.T) {
	env := NewTestEnvironment(t)

	// Create a workspace with some valid files and one problematic file
	workspaceFiles := map[string]string{
		"valid1.txt":  "valid content 1\n",
		"valid2.txt":  "valid content 2\n",
		"valid3.txt":  "valid content 3\n",
		"binary.bin":  string([]byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE}),
		"empty.txt":   "",
		"unicode.txt": "Hello 世界\n",
	}

	for path, content := range workspaceFiles {
		fullPath := filepath.Join(env.WorkspacePath, path)
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to create file %s: %v", path, err)
		}
	}

	// Scan the workspace - should not fail even with binary content
	files, err := ScanWorkspace(env.WorkspacePath)
	if err != nil {
		t.Fatalf("workspace scan failed with mixed content: %v", err)
	}

	// Assert scan completed and found all files
	if len(files) != len(workspaceFiles) {
		t.Errorf("expected %d files, got %d", len(workspaceFiles), len(files))
	}

	// Verify valid files were found
	validCount := 0
	for _, file := range files {
		if strings.HasPrefix(file.RelativePath, "valid") {
			validCount++
		}
	}
	if validCount != 3 {
		t.Errorf("expected 3 valid files, got %d", validCount)
	}

	// Verify binary file was found
	binaryFound := false
	for _, file := range files {
		if file.RelativePath == "binary.bin" {
			binaryFound = true
			// Binary file should have non-zero size
			if file.Size == 0 {
				t.Errorf("binary file should have non-zero size")
			}
			break
		}
	}
	if !binaryFound {
		t.Error("expected to find binary file")
	}

	// Verify empty file was found
	emptyFound := false
	for _, file := range files {
		if file.RelativePath == "empty.txt" {
			emptyFound = true
			// Empty file should have zero size
			if file.Size != 0 {
				t.Errorf("empty file should have zero size, got %d", file.Size)
			}
			break
		}
	}
	if !emptyFound {
		t.Error("expected to find empty file")
	}

	// Verify unicode file was found
	unicodeFound := false
	for _, file := range files {
		if file.RelativePath == "unicode.txt" {
			unicodeFound = true
			break
		}
	}
	if !unicodeFound {
		t.Error("expected to find unicode file")
	}
}

// TestScanDeterminism validates that scanning the same workspace
// produces identical results across multiple scans.
func TestScanDeterminism(t *testing.T) {
	env := NewTestEnvironment(t)

	// Create a workspace
	workspaceFiles := map[string]string{
		"file1.txt":     "content 1\n",
		"file2.txt":     "content 2\n",
		"dir/file3.txt": "content 3\n",
	}

	for path, content := range workspaceFiles {
		fullPath := filepath.Join(env.WorkspacePath, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create directory %s: %v", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to create file %s: %v", path, err)
		}
	}

	// Perform first scan
	scan1, err := ScanWorkspace(env.WorkspacePath)
	if err != nil {
		t.Fatalf("first scan failed: %v", err)
	}

	// Perform second scan
	scan2, err := ScanWorkspace(env.WorkspacePath)
	if err != nil {
		t.Fatalf("second scan failed: %v", err)
	}

	// Assert scan results are identical
	if len(scan1) != len(scan2) {
		t.Errorf("scan results differ in count: %d vs %d", len(scan1), len(scan2))
	}

	for i := range scan1 {
		if scan1[i].RelativePath != scan2[i].RelativePath {
			t.Errorf("scan results differ at index %d: %s vs %s", i, scan1[i].RelativePath, scan2[i].RelativePath)
		}
		if scan1[i].Size != scan2[i].Size {
			t.Errorf("file size differs for %s: %d vs %d", scan1[i].RelativePath, scan1[i].Size, scan2[i].Size)
		}
		// Mod times may differ slightly due to filesystem precision, but should be close
		timeDiff := scan1[i].ModTime.Sub(scan2[i].ModTime)
		if timeDiff < 0 {
			timeDiff = -timeDiff
		}
		if timeDiff > time.Second {
			t.Errorf("mod time differs significantly for %s: %v", scan1[i].RelativePath, timeDiff)
		}
	}
}

// TestScanEmptyWorkspace validates scanning an empty workspace.
func TestScanEmptyWorkspace(t *testing.T) {
	env := NewTestEnvironment(t)

	// Scan empty workspace
	files, err := ScanWorkspace(env.WorkspacePath)
	if err != nil {
		t.Fatalf("empty workspace scan failed: %v", err)
	}

	// Should return empty list, not error
	if len(files) != 0 {
		t.Errorf("expected 0 files in empty workspace, got %d", len(files))
	}
}

// TestScanHiddenFiles validates that hidden files are included in scan results.
func TestScanHiddenFiles(t *testing.T) {
	env := NewTestEnvironment(t)

	// Create workspace with hidden files
	hiddenFiles := map[string]string{
		".hidden":        "hidden content\n",
		".config/config": "config content\n",
		"normal.txt":     "normal content\n",
		".gitignore":     "*.log\n",
	}

	for path, content := range hiddenFiles {
		fullPath := filepath.Join(env.WorkspacePath, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create directory %s: %v", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to create file %s: %v", path, err)
		}
	}

	// Scan the workspace
	files, err := ScanWorkspace(env.WorkspacePath)
	if err != nil {
		t.Fatalf("workspace scan failed: %v", err)
	}

	// Assert hidden files are included
	if len(files) != len(hiddenFiles) {
		t.Errorf("expected %d files including hidden ones, got %d", len(hiddenFiles), len(files))
	}

	// Verify hidden files are present (files with names starting with ".")
	hiddenCount := 0
	for _, file := range files {
		if strings.HasPrefix(filepath.Base(file.RelativePath), ".") {
			hiddenCount++
		}
	}

	if hiddenCount != 2 { // .hidden, .gitignore (.config/config has base name "config")
		t.Errorf("expected 2 hidden files, got %d", hiddenCount)
	}
}
