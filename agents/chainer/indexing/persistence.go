package indexing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CodeIndexSnapshot represents a point-in-time snapshot of the code index.
type CodeIndexSnapshot struct {
	// Metadata
	SnapshotID   string
	TaskID       string
	Timestamp    time.Time
	LinkName     string // Last link executed when snapshot taken

	// Indexed snippets
	Snippets []*IndexedCodeSnippet

	// Statistics
	TotalSnippets int
	ErrorCount    int
	SuccessCount  int
}

// PersistedCodeIndexStore saves and loads indexed code snippets to disk.
//
// Phase 8: File-based persistence for code index, enabling recovery across
// checkpoint boundaries. Snapshots are stored hierarchically by task and link
// for efficient restoration.
type PersistedCodeIndexStore struct {
	basePath string
}

// NewPersistedCodeIndexStore creates a persistent store rooted at basePath.
func NewPersistedCodeIndexStore(basePath string) *PersistedCodeIndexStore {
	return &PersistedCodeIndexStore{
		basePath: basePath,
	}
}

// SaveSnapshot persists a snapshot of the current code index state.
func (s *PersistedCodeIndexStore) SaveSnapshot(snapshot *CodeIndexSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("cannot save nil snapshot")
	}

	if snapshot.SnapshotID == "" {
		return fmt.Errorf("snapshot must have an ID")
	}

	// Create directory structure: basePath/taskID/snapshots/
	snapshotDir := filepath.Join(s.basePath, snapshot.TaskID, "snapshots")
	if err := os.MkdirAll(snapshotDir, 0755); err != nil {
		return fmt.Errorf("failed to create snapshot directory: %w", err)
	}

	// Write snapshot file
	snapshotPath := filepath.Join(snapshotDir, snapshot.SnapshotID+".json")
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	if err := os.WriteFile(snapshotPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write snapshot: %w", err)
	}

	return nil
}

// LoadSnapshot retrieves a snapshot by task and snapshot ID.
func (s *PersistedCodeIndexStore) LoadSnapshot(taskID, snapshotID string) (*CodeIndexSnapshot, error) {
	snapshotPath := filepath.Join(s.basePath, taskID, "snapshots", snapshotID+".json")

	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("snapshot not found: %s", snapshotID)
		}
		return nil, fmt.Errorf("failed to read snapshot: %w", err)
	}

	var snapshot CodeIndexSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("failed to unmarshal snapshot: %w", err)
	}

	return &snapshot, nil
}

// ListSnapshots returns all snapshot IDs for a task.
func (s *PersistedCodeIndexStore) ListSnapshots(taskID string) ([]string, error) {
	snapshotDir := filepath.Join(s.basePath, taskID, "snapshots")

	entries, err := os.ReadDir(snapshotDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list snapshots: %w", err)
	}

	var result []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		result = append(result, strings.TrimSuffix(entry.Name(), ".json"))
	}

	return result, nil
}

// DeleteSnapshot removes a snapshot.
func (s *PersistedCodeIndexStore) DeleteSnapshot(taskID, snapshotID string) error {
	snapshotPath := filepath.Join(s.basePath, taskID, "snapshots", snapshotID+".json")

	if err := os.Remove(snapshotPath); err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to delete snapshot: %w", err)
	}

	return nil
}

// PersistenceManager integrates indexing with checkpoint persistence.
//
// Phase 8: Bridges CodeIndex and PersistedCodeIndexStore, enabling transparent
// snapshot creation at checkpoint boundaries and recovery from persisted snapshots.
type PersistenceManager struct {
	index      *CodeIndex
	store      *PersistedCodeIndexStore
	taskID     string
	lastSnap   string
}

// NewPersistenceManager creates a manager that bridges indexing and persistence.
func NewPersistenceManager(index *CodeIndex, store *PersistedCodeIndexStore, taskID string) *PersistenceManager {
	if index == nil || store == nil {
		return nil
	}

	return &PersistenceManager{
		index:  index,
		store:  store,
		taskID: taskID,
	}
}

// CreateSnapshot saves the current code index state.
func (pm *PersistenceManager) CreateSnapshot(linkName string) (string, error) {
	if pm == nil || pm.index == nil {
		return "", fmt.Errorf("persistence manager not initialized")
	}

	// Generate snapshot ID with nanosecond precision to avoid collisions
	snapshotID := fmt.Sprintf("snapshot-%s-%d", linkName, time.Now().UnixNano())

	snippets := pm.index.AllSnippets()

	// Copy snippets to avoid external mutation
	snippetCopies := make([]*IndexedCodeSnippet, len(snippets))
	copy(snippetCopies, snippets)

	// Count errors
	errorCount := 0
	for _, s := range snippets {
		if s.IsError {
			errorCount++
		}
	}

	snapshot := &CodeIndexSnapshot{
		SnapshotID:    snapshotID,
		TaskID:        pm.taskID,
		Timestamp:     time.Now(),
		LinkName:      linkName,
		Snippets:      snippetCopies,
		TotalSnippets: len(snippets),
		ErrorCount:    errorCount,
		SuccessCount:  len(snippets) - errorCount,
	}

	if err := pm.store.SaveSnapshot(snapshot); err != nil {
		return "", fmt.Errorf("failed to create snapshot: %w", err)
	}

	pm.lastSnap = snapshotID
	return snapshotID, nil
}

// RestoreSnapshot loads a snapshot into the code index.
func (pm *PersistenceManager) RestoreSnapshot(snapshotID string) error {
	if pm == nil || pm.index == nil {
		return fmt.Errorf("persistence manager not initialized")
	}

	snapshot, err := pm.store.LoadSnapshot(pm.taskID, snapshotID)
	if err != nil {
		return fmt.Errorf("failed to load snapshot: %w", err)
	}

	// Clear current index
	pm.index.Clear()

	// Restore snippets
	for _, snippet := range snapshot.Snippets {
		if err := pm.index.Index(snippet); err != nil {
			return fmt.Errorf("failed to restore snippet: %w", err)
		}
	}

	pm.lastSnap = snapshotID
	return nil
}

// LatestSnapshot returns the most recent snapshot for this task.
func (pm *PersistenceManager) LatestSnapshot() (string, error) {
	if pm == nil {
		return "", fmt.Errorf("persistence manager not initialized")
	}

	snapshots, err := pm.store.ListSnapshots(pm.taskID)
	if err != nil {
		return "", err
	}

	if len(snapshots) == 0 {
		return "", fmt.Errorf("no snapshots found for task %s", pm.taskID)
	}

	// Return last snapshot (newest)
	return snapshots[len(snapshots)-1], nil
}

// ListAllSnapshots returns all snapshots for this task.
func (pm *PersistenceManager) ListAllSnapshots() ([]string, error) {
	if pm == nil {
		return nil, fmt.Errorf("persistence manager not initialized")
	}

	return pm.store.ListSnapshots(pm.taskID)
}

// ReportGenerator creates reports from indexed and persisted code.
//
// Phase 8: Generates human-readable reports of code analysis activities,
// useful for audit trails, debugging, and visualization.
type ReportGenerator struct {
	index *CodeIndex
	store *PersistedCodeIndexStore
}

// NewReportGenerator creates a generator for indexing reports.
func NewReportGenerator(index *CodeIndex, store *PersistedCodeIndexStore) *ReportGenerator {
	return &ReportGenerator{
		index: index,
		store: store,
	}
}

// ExecutionReport summarizes all activity in a task.
type ExecutionReport struct {
	TaskID          string
	GeneratedAt     time.Time
	TotalSnippets   int
	ErrorSnippets   int
	SuccessSnippets int
	UniqueLanguages map[string]int
	UniqueFiles     map[string]int
	SnapshotCount   int
	Timeline        []SnapshotTimelineEntry
}

// SnapshotTimelineEntry represents a point in the execution timeline.
type SnapshotTimelineEntry struct {
	SnapshotID string
	Timestamp  time.Time
	LinkName   string
	Snippets   int
	Errors     int
}

// GenerateExecutionReport creates a comprehensive report of task execution.
func (rg *ReportGenerator) GenerateExecutionReport(taskID string) (*ExecutionReport, error) {
	if rg == nil {
		return &ExecutionReport{}, fmt.Errorf("report generator not initialized")
	}

	report := &ExecutionReport{
		TaskID:          taskID,
		GeneratedAt:     time.Now(),
		UniqueLanguages: make(map[string]int),
		UniqueFiles:     make(map[string]int),
	}

	// Get snapshots
	snapshots, err := rg.store.ListSnapshots(taskID)
	if err != nil {
		return report, nil // Return empty report if no snapshots
	}

	report.SnapshotCount = len(snapshots)

	// Build timeline from snapshots
	for _, snapID := range snapshots {
		snap, err := rg.store.LoadSnapshot(taskID, snapID)
		if err != nil {
			continue
		}

		entry := SnapshotTimelineEntry{
			SnapshotID: snapID,
			Timestamp:  snap.Timestamp,
			LinkName:   snap.LinkName,
			Snippets:   snap.TotalSnippets,
			Errors:     snap.ErrorCount,
		}
		report.Timeline = append(report.Timeline, entry)

		// Aggregate stats
		report.TotalSnippets = snap.TotalSnippets
		report.ErrorSnippets = snap.ErrorCount
		report.SuccessSnippets = snap.SuccessCount

		// Aggregate languages and files
		for _, snippet := range snap.Snippets {
			if snippet.Language != "" {
				report.UniqueLanguages[snippet.Language]++
			}
			if snippet.FilePath != "" {
				report.UniqueFiles[snippet.FilePath]++
			}
		}
	}

	return report, nil
}

// FormatReport generates a human-readable text report.
func (rg *ReportGenerator) FormatReport(report *ExecutionReport) string {
	if report == nil {
		return ""
	}

	output := fmt.Sprintf(`
Code Index Execution Report
============================

Task ID: %s
Generated: %s

Summary:
--------
Total Snippets:   %d
Successful:       %d
Errors:           %d
Snapshots:        %d

Languages Found:
`, report.TaskID, report.GeneratedAt.Format(time.RFC3339), report.TotalSnippets,
		report.SuccessSnippets, report.ErrorSnippets, report.SnapshotCount)

	for lang, count := range report.UniqueLanguages {
		output += fmt.Sprintf("  %s: %d\n", lang, count)
	}

	output += "\nFiles Analyzed:\n"
	for file, count := range report.UniqueFiles {
		output += fmt.Sprintf("  %s: %d snippets\n", file, count)
	}

	if len(report.Timeline) > 0 {
		output += fmt.Sprintf("\nExecution Timeline (%d snapshots):\n", len(report.Timeline))
		for i, entry := range report.Timeline {
			output += fmt.Sprintf("  %d. [%s] Link: %s | Snippets: %d | Errors: %d\n",
				i+1, entry.Timestamp.Format("15:04:05"), entry.LinkName, entry.Snippets, entry.Errors)
		}
	}

	return output
}
