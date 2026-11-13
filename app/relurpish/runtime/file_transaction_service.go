// Package runtime provides the Relurpish workspace owner and its runtime services.
// This file implements the FileTransactionService for P05.
package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"codeburg.org/lexbit/relurpify/userconfig/config"
	"codeburg.org/lexbit/relurpify/userconfig/securefile"
)

// FileTransactionService handles atomic multi-file configuration transactions
// with CAS checks and rollback capability (P05).
type FileTransactionService struct {
	mu            sync.RWMutex
	workspaceRoot string
	pendingTxns   map[string]*securefile.MultiFileTransaction
}

// NewFileTransactionService creates a new file transaction service.
func NewFileTransactionService(workspaceRoot string) *FileTransactionService {
	return &FileTransactionService{
		workspaceRoot: workspaceRoot,
		pendingTxns:   make(map[string]*securefile.MultiFileTransaction),
	}
}

// TransactionID is a unique identifier for a file transaction.
type TransactionID string

// FileChange represents a single file change in a transaction.
type FileChange struct {
	Path     string
	Content  []byte
	OldDigest string
}

// FileTransactionRequest represents a request to perform a multi-file transaction.
type FileTransactionRequest struct {
	TransactionID TransactionID
	Changes        []FileChange
	BackupEnabled  bool
	Description    string
}

// FileTransactionResult represents the result of a file transaction.
type FileTransactionResult struct {
	TransactionID TransactionID
	Success       bool
	NewDigests    map[string]string
	Error         error
	RollbackInfo  *RollbackInfo
}

// RollbackInfo contains information about a rollback.
type RollbackInfo struct {
	WasRolledBack bool
	FailedWrites  []string
	RestoredFiles []string
}

// PrepareTransaction prepares a multi-file transaction with validation.
func (s *FileTransactionService) PrepareTransaction(req FileTransactionRequest) (*securefile.MultiFileTransaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if req.TransactionID == "" {
		return nil, fmt.Errorf("transaction ID cannot be empty")
	}

	if len(req.Changes) == 0 {
		return nil, fmt.Errorf("no changes in transaction")
	}

	// Validate all paths are within workspace
	for _, change := range req.Changes {
		if !s.isWithinWorkspace(change.Path) {
			return nil, fmt.Errorf("path %s is outside workspace root", change.Path)
		}
	}

	// Create transaction
	txn := securefile.NewMultiFileTransaction()

	for _, change := range req.Changes {
		txn.AddFile(change.Path, change.Content)
		txn.WithExpectedDigest(change.Path, change.OldDigest)
		
		// Set up backup if enabled
		if req.BackupEnabled {
			backupPath := s.getBackupPath(change.Path, string(req.TransactionID))
			txn.WithBackup(change.Path, backupPath)
		}
	}

	// Store pending transaction
	s.pendingTxns[string(req.TransactionID)] = txn

	return txn, nil
}

// ExecuteTransaction executes a prepared transaction.
func (s *FileTransactionService) ExecuteTransaction(req FileTransactionRequest) FileTransactionResult {
	result := FileTransactionResult{
		TransactionID: req.TransactionID,
		NewDigests:    make(map[string]string),
	}

	// Prepare transaction
	txn, err := s.PrepareTransaction(req)
	if err != nil {
		result.Error = err
		return result
	}

	// Execute
	if err := txn.Execute(); err != nil {
		result.Error = err
		result.RollbackInfo = &RollbackInfo{
			WasRolledBack: true,
			FailedWrites:  []string{}, // Would need to track which writes failed
		}
		return result
	}

	// Compute new digests
	for _, change := range req.Changes {
		digest := securefile.ComputeDigest(change.Content)
		result.NewDigests[change.Path] = digest
	}

	result.Success = true

	// Clean up pending
	s.mu.Lock()
	delete(s.pendingTxns, string(req.TransactionID))
	s.mu.Unlock()

	return result
}

// RollbackTransaction rolls back a transaction by its ID.
func (s *FileTransactionService) RollbackTransaction(transactionID TransactionID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	txn, ok := s.pendingTxns[string(transactionID)]
	if !ok {
		return fmt.Errorf("transaction %s not found", transactionID)
	}

	// The transaction might already be executed, in which case we need to rollback
	// using the backup files
	if err := txn.Rollback(); err != nil {
		return fmt.Errorf("rollback transaction %s: %w", transactionID, err)
	}

	delete(s.pendingTxns, string(transactionID))
	return nil
}

// GetTransactionStatus returns the status of a transaction.
func (s *FileTransactionService) GetTransactionStatus(transactionID TransactionID) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.pendingTxns[string(transactionID)]
	return ok, nil
}

// CleanupOldBackups removes old backup files.
func (s *FileTransactionService) CleanupOldBackups() error {
	backupDir := filepath.Join(s.workspaceRoot, ".backup")
	return os.RemoveAll(backupDir)
}

// isWithinWorkspace checks if a path is within the workspace root.
func (s *FileTransactionService) isWithinWorkspace(path string) bool {
	// Get absolute paths
	absWorkspace, _ := filepath.Abs(s.workspaceRoot)
	absPath, _ := filepath.Abs(path)

	// Check if path is within workspace
	return filepath.Dir(absPath) == absWorkspace || 
		filepath.HasPrefix(absPath, absWorkspace+string(os.PathSeparator))
}

// getBackupPath generates a backup path for a file.
func (s *FileTransactionService) getBackupPath(filePath, transactionID string) string {
	backupDir := filepath.Join(s.workspaceRoot, ".backup", transactionID)
	return filepath.Join(backupDir, filepath.Base(filePath)+".backup")
}

// CreateConfigFilesTransaction creates a transaction for saving configuration files.
// This is used by the SettingsService for atomic config saves (P05).
func (s *FileTransactionService) CreateConfigFilesTransaction(
	configFiles map[string][]byte,
	oldDigests map[string]string,
	transactionID TransactionID,
) (*securefile.MultiFileTransaction, error) {
	var changes []FileChange
	for path, content := range configFiles {
		changes = append(changes, FileChange{
			Path:     path,
			Content:  content,
			OldDigest: oldDigests[path],
		})
	}

	return s.PrepareTransaction(FileTransactionRequest{
		TransactionID: transactionID,
		Changes:        changes,
		BackupEnabled:  true,
		Description:    "Configuration save",
	})
}

// VerifyFileIntegrity verifies that files match their expected digests.
func (s *FileTransactionService) VerifyFileIntegrity(path, expectedDigest string) (bool, error) {
	currentDigest, err := securefile.NewFileTransaction(path).CurrentDigest()
	if err != nil {
		return false, err
	}
	return currentDigest == expectedDigest, nil
}

// FileTransactionPort defines the interface for file transaction operations.
// This is used for dependency injection and testing.
type FileTransactionPort interface {
	PrepareTransaction(req FileTransactionRequest) (*securefile.MultiFileTransaction, error)
	ExecuteTransaction(req FileTransactionRequest) FileTransactionResult
	RollbackTransaction(transactionID TransactionID) error
	GetTransactionStatus(transactionID TransactionID) (bool, error)
	CreateConfigFilesTransaction(configFiles map[string][]byte, oldDigests map[string]string, transactionID TransactionID) (*securefile.MultiFileTransaction, error)
	VerifyFileIntegrity(path, expectedDigest string) (bool, error)
}

// Ensure FileTransactionService implements FileTransactionPort.
var _ FileTransactionPort = (*FileTransactionService)(nil)

// DocumentSnapshotToDigest converts a DocumentSnapshot to a digest string.
func DocumentSnapshotToDigest(snapshot *config.DocumentSnapshot) string {
	if snapshot == nil || snapshot.Document == nil {
		return ""
	}
	// In a real implementation, we'd serialize the document and compute its digest
	// For now, return the fingerprint as a hex string
	return fmt.Sprintf("%x", snapshot.Fingerprint[:])
}
