package ingestion

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	fsandbox "codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/relurpnet/identity"
)

// SkillIngestionSource defines a path pattern for skill resource ingestion.
type SkillIngestionSource struct {
	Path       string
	SourceType string
}

// WorkspaceScanner implements bulk pre-runtime ingestion.
type WorkspaceScanner struct {
	Store         *knowledge.ChunkStore
	Events        EventLog
	Policy        interface{} // *contextpolicy.ContextPolicyBundle (avoid import cycle)
	Evaluator     interface{} // *contextpolicy.Evaluator
	Concurrency   int
	IncludeGlobs  []string
	ExcludeGlobs  []string
	Scanners      []Scanner
	QuarantineDir string
	FileScope     *fsandbox.FileScopePolicy
}

// Scan performs a full workspace scan.
func (s *WorkspaceScanner) Scan(ctx context.Context, root string) (*ScanReport, error) {
	start := time.Now()
	report := &ScanReport{}

	// Ensure quarantine directory exists
	if s.QuarantineDir != "" {
		if err := os.MkdirAll(s.QuarantineDir, 0750); err != nil {
			return report, fmt.Errorf("create quarantine dir: %w", err)
		}
	}

	// Use default scanners if none provided
	if len(s.Scanners) == 0 {
		s.Scanners = defaultScanners()
	}

	// Discover files
	files, err := s.discoverFiles(root)
	if err != nil {
		return report, fmt.Errorf("discover files: %w", err)
	}

	report.FilesScanned = len(files)

	// Process files with bounded concurrency
	sem := make(chan struct{}, s.concurrency())
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, file := range files {
		wg.Add(1)
		sem <- struct{}{} // acquire

		go func(file string) {
			defer wg.Done()
			defer func() { <-sem }() // release

			if err := s.processFile(ctx, root, file, report, &mu); err != nil {
				mu.Lock()
				report.Errors = append(report.Errors, fmt.Errorf("%s: %w", file, err))
				mu.Unlock()
			}
		}(file)
	}

	wg.Wait()

	report.Duration = time.Since(start)

	// Emit bootstrap complete event
	if s.Events != nil {
		s.Events.Emit(string(core.EventBootstrapComplete), map[string]any{
			"files_scanned":      report.FilesScanned,
			"chunks_created":     report.ChunksCreated,
			"chunks_quarantined": report.ChunksQuarantined,
			"chunks_rejected":    report.ChunksRejected,
			"duration_ms":        report.Duration.Milliseconds(),
			"workspace_root":     root,
		})
	}

	return report, nil
}

// SetFileScope configures the filesystem boundary enforced during workspace scans.
func (s *WorkspaceScanner) SetFileScope(scope *fsandbox.FileScopePolicy) {
	s.FileScope = scope
}

// ScanIncremental performs an incremental scan based on git changes.
func (s *WorkspaceScanner) ScanIncremental(ctx context.Context, root string, since string) (*ScanReport, error) {
	// Get changed files from git
	changedFiles, err := s.getGitChangedFiles(root, since)
	if err != nil {
		// Fall back to full scan if git fails
		return s.Scan(ctx, root)
	}

	start := time.Now()
	report := &ScanReport{
		FilesScanned: len(changedFiles),
	}

	if len(s.Scanners) == 0 {
		s.Scanners = defaultScanners()
	}

	// Process only changed files
	sem := make(chan struct{}, s.concurrency())
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, file := range changedFiles {
		wg.Add(1)
		sem <- struct{}{}

		go func(file string) {
			defer wg.Done()
			defer func() { <-sem }()

			// Check if file should be processed
			if !s.shouldInclude(file) || !s.allowsPath(filepath.Join(root, file), false) {
				return
			}

			fullPath := filepath.Join(root, file)
			if err := s.processFile(ctx, root, fullPath, report, &mu); err != nil {
				mu.Lock()
				report.Errors = append(report.Errors, fmt.Errorf("%s: %w", file, err))
				mu.Unlock()
			}
		}(file)
	}

	wg.Wait()

	report.Duration = time.Since(start)

	// Emit bootstrap complete event
	if s.Events != nil {
		s.Events.Emit(string(core.EventBootstrapComplete), map[string]any{
			"files_scanned":      report.FilesScanned,
			"chunks_created":     report.ChunksCreated,
			"chunks_quarantined": report.ChunksQuarantined,
			"chunks_rejected":    report.ChunksRejected,
			"duration_ms":        report.Duration.Milliseconds(),
			"workspace_root":     root,
			"incremental":        true,
			"since_ref":          since,
		})
	}

	return report, nil
}

// discoverFiles discovers files in the workspace respecting .gitignore.
func (s *WorkspaceScanner) discoverFiles(root string) ([]string, error) {
	var files []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue walking
		}

		// Skip directories
		if info.IsDir() {
			if !s.allowsPath(path, true) {
				return filepath.SkipDir
			}
			// Skip hidden directories and common exclude patterns
			name := info.Name()
			if strings.HasPrefix(name, ".") && name != "." {
				if name == ".git" || name == "node_modules" || name == "vendor" {
					return filepath.SkipDir
				}
			}
			return nil
		}

		if !s.allowsPath(path, false) {
			return nil
		}
		// Check if file should be included
		if s.shouldInclude(path) {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}

// shouldInclude checks if a file should be included based on globs.
func (s *WorkspaceScanner) shouldInclude(path string) bool {
	// Check exclude globs first
	for _, glob := range s.ExcludeGlobs {
		if matched, _ := filepath.Match(glob, filepath.Base(path)); matched {
			return false
		}
		if matched, _ := filepath.Match(glob, path); matched {
			return false
		}
	}

	// If include globs specified, path must match one
	if len(s.IncludeGlobs) > 0 {
		included := false
		for _, glob := range s.IncludeGlobs {
			if matched, _ := filepath.Match(glob, filepath.Base(path)); matched {
				included = true
				break
			}
			if matched, _ := filepath.Match(glob, path); matched {
				included = true
				break
			}
		}
		if !included {
			return false
		}
	}

	return true
}

// processFile processes a single file.
func (s *WorkspaceScanner) processFile(ctx context.Context, root string, file string, report *ScanReport, mu *sync.Mutex) error {
	// Create pipeline
	pipeline, err := AcquireFromFile(ctx, file, identity.SubjectRef{ID: "scanner"}, nil, s.Store, s.FileScope)
	if err != nil {
		return err
	}

	pipeline.SetQuarantineDir(s.QuarantineDir)
	pipeline.SetScanners(s.Scanners)

	// Run pipeline
	result, err := pipeline.Run(ctx)
	if err != nil {
		return err
	}

	// Update report
	mu.Lock()
	report.ChunksCreated += result.ChunksCommitted
	report.ChunksQuarantined += result.ChunksQuarantined
	report.ChunksRejected += result.ChunksRejected
	mu.Unlock()

	return nil
}

// getGitChangedFiles gets the list of changed files from git.
func (s *WorkspaceScanner) getGitChangedFiles(root string, since string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", since)
	cmd.Dir = root

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}

	var files []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		file := strings.TrimSpace(scanner.Text())
		if file != "" {
			files = append(files, file)
		}
	}

	return files, scanner.Err()
}

// concurrency returns the concurrency level with default.
func (s *WorkspaceScanner) concurrency() int {
	if s.Concurrency <= 0 {
		return 4 // default
	}
	return s.Concurrency
}

// AddScanner adds a scanner to the workspace scanner.
func (s *WorkspaceScanner) AddScanner(scanner Scanner) {
	s.Scanners = append(s.Scanners, scanner)
}

// SetPolicy sets the context policy for the workspace scanner.
func (s *WorkspaceScanner) SetPolicy(policy interface{}) {
	s.Policy = policy
}

// AddSkillIngestionSources adds skill ingestion source paths to the scanner's include globs.
// This allows skills to contribute paths that should be scanned for resources.
func (s *WorkspaceScanner) AddSkillIngestionSources(sources []SkillIngestionSource) {
	for _, source := range sources {
		s.IncludeGlobs = append(s.IncludeGlobs, source.Path)
	}
}

func (s *WorkspaceScanner) allowsPath(path string, isDir bool) bool {
	if s == nil || s.FileScope == nil {
		return true
	}
	action := core.FileSystemRead
	if isDir {
		action = core.FileSystemList
	}
	return s.FileScope.Check(action, path) == nil
}
