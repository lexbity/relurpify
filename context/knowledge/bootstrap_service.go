package knowledge

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/context/knowledge/ast"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// BootstrapService runs a one-shot workspace indexing/bootstrap pass. It is
// the knowledge-domain service the workspace session registers directly
// (S7): the ayenitd adapter that used to host it is gone. Its degraded-boot
// semantics are the pattern of record — an operational indexing failure
// degrades knowledge (log + boot.degraded telemetry), it never aborts boot;
// cancellation alone propagates as lifecycle.
type BootstrapService struct {
	IndexManager   *ast.IndexManager
	EventBus       *EventBus
	Telemetry      telemetry.Telemetry
	WorkspaceRoot  string
	IndexWorkspace func(context.Context) error
	LoadStats      func() (*ast.IndexStats, error)

	mu     sync.Mutex
	cancel context.CancelFunc
}

func (s *BootstrapService) Start(ctx context.Context) error {
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
		// Cancellation is lifecycle: surface it so Stop/Start coordination is
		// observed. An operational indexing failure degrades knowledge, it does
		// not abort boot — the workspace boots and logs the named condition.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		log.Printf("workspace bootstrap indexing failed (knowledge degraded): %v", err)
		s.emitBootDegraded(err)
		return nil
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
		s.EventBus.EmitBootstrapComplete(BootstrapCompletePayload{
			WorkspaceRoot: s.WorkspaceRoot,
			IndexedFiles:  indexedFiles,
		})
	}
	return nil
}

func (s *BootstrapService) Stop() error {
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

func (s *BootstrapService) clearCancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancel = nil
}

// emitBootDegraded surfaces an indexing failure as a boot.degraded warning on
// the workspace health instead of aborting boot.
func (s *BootstrapService) emitBootDegraded(err error) {
	if s == nil || s.Telemetry == nil || err == nil {
		return
	}
	s.Telemetry.Emit(telemetry.Event{
		Type:      telemetry.EventBootDegraded,
		Message:   "knowledge services degraded",
		Timestamp: time.Now().UTC(),
		Metadata: map[string]any{
			"reason":  "knowledge_services",
			"service": "knowledge.bootstrap",
			"error":   err.Error(),
		},
	})
}

// NewBootstrapService constructs the workspace bootstrap service from its
// explicit dependencies. IndexManager nil yields an inert service (Start is
// a no-op), which is how embedded scopes and unit tests stay dormant.
func NewBootstrapService(indexManager *ast.IndexManager, eventBus *EventBus, tel telemetry.Telemetry, workspaceRoot string, indexWorkspace func(context.Context) error, loadStats func() (*ast.IndexStats, error)) *BootstrapService {
	return &BootstrapService{
		IndexManager:   indexManager,
		EventBus:       eventBus,
		Telemetry:      tel,
		WorkspaceRoot:  workspaceRoot,
		IndexWorkspace: indexWorkspace,
		LoadStats:      loadStats,
	}
}
