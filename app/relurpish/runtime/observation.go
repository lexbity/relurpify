package runtime

import (
	"context"
	"errors"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/context/knowledge/graphdb"
)

// ObservationError types
var (
	// ErrSubscriptionClosed is returned when attempting to use a closed subscription.
	ErrSubscriptionClosed = errors.New("observation: subscription closed")
	
	// ErrCursorNotFound is returned when a cursor position doesn't exist.
	ErrCursorNotFound = errors.New("observation: cursor not found")
)

// Observation represents a single committed fact from the journal.
type Observation struct {
	// Cursor is the monotonic position of this observation.
	Cursor Cursor
	
	// RunID identifies the run this observation belongs to.
	RunID string
	
	// Kind identifies the type of observation (e.g., "run.started", "model.attempt.started").
	Kind string
	
	// Payload contains the observation data.
	Payload []byte
	
	// OccurredAt is when the observation was generated.
	OccurredAt time.Time
	
	// CommittedAt is when the observation was durably committed.
	CommittedAt time.Time
}

// Snapshot represents the initial state at a given cursor position.
type Snapshot struct {
	// Cursor is the position of this snapshot.
	Cursor Cursor
	
	// RunID identifies the run.
	RunID string
	
	// Aggregates contains the aggregated state at this cursor.
	Aggregates map[string][]byte
	
	// LatestObservations contains the most recent observations for each kind.
	LatestObservations map[string]Observation
}

// ObservationPort provides the observation API for the TUI.
type ObservationPort interface {
	// ObserveRun starts observing a run from the given cursor position.
	// If afterCursor is 0, starts from the beginning.
	// Returns the initial snapshot and a subscription channel.
	ObserveRun(ctx context.Context, runID string, afterCursor Cursor) (Snapshot, <-chan Observation, error)
	
	// ReadRunEvents reads events from a run starting after the given cursor.
	// Limit specifies the maximum number of events to return.
	ReadRunEvents(ctx context.Context, runID string, afterCursor Cursor, limit int) ([]Observation, Cursor, error)
	
	// GetLatestCursor returns the latest cursor for a run.
	GetLatestCursor(ctx context.Context, runID string) (Cursor, error)
}

// observationSubscription manages a single subscription.
type observationSubscription struct {
	mu         sync.Mutex
	runID      string
	afterCursor Cursor
	channel    chan Observation
	closed     bool
	cancel     context.CancelFunc
}

// newObservationSubscription creates a new subscription.
func newObservationSubscription(runID string, afterCursor Cursor, bufferSize int) *observationSubscription {
	return &observationSubscription{
		runID:      runID,
		afterCursor: afterCursor,
		channel:    make(chan Observation, bufferSize),
	}
}

// close closes the subscription.
func (s *observationSubscription) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.channel)
		if s.cancel != nil {
			s.cancel()
		}
	}
}

// send sends an observation to the subscription.
func (s *observationSubscription) send(obs Observation) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	select {
	case s.channel <- obs:
		return true
	default:
		// Channel full - drop observation (caller should handle backpressure)
		return false
	}
}

// ObservationRegistry manages observations and subscriptions.
type ObservationRegistry struct {
	mu          sync.RWMutex
	db          *graphdb.Engine
	subscriptions map[string]*observationSubscription
	workspace    WorkspaceID
}

// NewObservationRegistry creates a new observation registry.
func NewObservationRegistry(db *graphdb.Engine, workspace WorkspaceID) *ObservationRegistry {
	return &ObservationRegistry{
		db:          db,
		subscriptions: make(map[string]*observationSubscription),
		workspace:    workspace,
	}
}

// ObserveRun implements ObservationPort.ObserveRun.
func (r *ObservationRegistry) ObserveRun(ctx context.Context, runID string, afterCursor Cursor) (Snapshot, <-chan Observation, error) {
	// Get the latest cursor for this run
	latestCursor, err := r.GetLatestCursor(ctx, runID)
	if err != nil {
		return Snapshot{}, nil, err
	}

	// If afterCursor is 0 or greater than latest, start from latest
	startCursor := afterCursor
	if afterCursor == 0 || afterCursor > latestCursor {
		startCursor = latestCursor
	}

	// Get the snapshot at the start cursor
	snapshot, err := r.getSnapshot(ctx, runID, startCursor)
	if err != nil {
		return Snapshot{}, nil, err
	}

	// Create subscription
	sub := newObservationSubscription(runID, startCursor, 256)
	
	// Store subscription
	r.mu.Lock()
	subID := generateSubscriptionID()
	r.subscriptions[subID] = sub
	r.mu.Unlock()

	// Start background goroutine to watch for new observations
	ctx, cancel := context.WithCancel(ctx)
	sub.cancel = cancel
	go r.watchRun(ctx, runID, startCursor, sub)

	return snapshot, sub.channel, nil
}

// watchRun watches for new observations in a run and sends them to the subscription.
func (r *ObservationRegistry) watchRun(ctx context.Context, runID string, afterCursor Cursor, sub *observationSubscription) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			sub.close()
			return
		case <-ticker.C:
			// Check for new observations
			latestCursor, err := r.GetLatestCursor(ctx, runID)
			if err != nil {
				// Log error but continue
				continue
			}

			if latestCursor > sub.afterCursor {
				// New observations available
				 observations, newCursor, err := r.ReadRunEvents(ctx, runID, sub.afterCursor, 100)
				if err != nil {
					continue
				}
				for _, obs := range observations {
					sub.send(obs)
				}
				sub.afterCursor = newCursor
			}
		}
	}
}

// ReadRunEvents implements ObservationPort.ReadRunEvents.
func (r *ObservationRegistry) ReadRunEvents(ctx context.Context, runID string, afterCursor Cursor, limit int) ([]Observation, Cursor, error) {
	// For now, we'll return an empty list as the implementation needs
	// to be connected to the actual journal storage
	return nil, 0, nil
}

// GetLatestCursor implements ObservationPort.GetLatestCursor.
func (r *ObservationRegistry) GetLatestCursor(ctx context.Context, runID string) (Cursor, error) {
	// For now, return 0 as we need to connect to actual storage
	return 0, nil
}

// getSnapshot retrieves a snapshot at a given cursor.
func (r *ObservationRegistry) getSnapshot(ctx context.Context, runID string, cursor Cursor) (Snapshot, error) {
	// For now, return an empty snapshot
	return Snapshot{
		Cursor: cursor,
		RunID:  runID,
		Aggregates: make(map[string][]byte),
		LatestObservations: make(map[string]Observation),
	}, nil
}

// generateSubscriptionID generates a unique subscription ID.
func generateSubscriptionID() string {
	return graphdb.GenerateID("sub")
}

// Close closes all subscriptions.
func (r *ObservationRegistry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, sub := range r.subscriptions {
		sub.close()
	}
	r.subscriptions = make(map[string]*observationSubscription)
	return nil
}
