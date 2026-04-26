package ayenitd

import (
	"context"
	"sync"

	"codeburg.org/lexbit/relurpify/framework/ast"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
)

// WorkspaceBootstrapService runs a one-shot workspace indexing/bootstrap pass.
type WorkspaceBootstrapService struct {
	IndexManager   *ast.IndexManager
	EventBus       *knowledge.EventBus
	WorkspaceRoot  string
	IndexWorkspace func(context.Context) error
	LoadStats      func() (*ast.IndexStats, error)

	mu     sync.Mutex
	cancel context.CancelFunc
}

func (s *WorkspaceBootstrapService) Start(ctx context.Context) error {
	if s == nil || s.IndexManager == nil {
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()
	defer s.clearCancel()
	indexWorkspace := s.IndexWorkspace
	if indexWorkspace == nil {
		indexWorkspace = s.IndexManager.IndexWorkspaceContext
	}
	if err := indexWorkspace(runCtx); err != nil {
		return err
	}
	statsFn := s.LoadStats
	if statsFn == nil {
		statsFn = s.IndexManager.Stats
	}
	indexedFiles := 0
	if stats, err := statsFn(); err == nil && stats != nil {
		indexedFiles = stats.TotalFiles
	}
	if s.EventBus != nil {
		s.EventBus.EmitBootstrapComplete(knowledge.BootstrapCompletePayload{
			WorkspaceRoot: s.WorkspaceRoot,
			IndexedFiles:  indexedFiles,
		})
	}
	return nil
}

func (s *WorkspaceBootstrapService) Stop() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (s *WorkspaceBootstrapService) clearCancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancel = nil
}
