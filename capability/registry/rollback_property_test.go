package registry

import (
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/ports"
)

type modelRingEntry struct {
	token     ports.RollbackToken
	expiresAt time.Time
}

// modelRing is a straightforward reference implementation of the bounded/TTL'd
// rollback store used to cross-check rollbackRing under randomized operations.
type modelRing struct {
	entries []modelRingEntry
	byID    map[string]modelRingEntry
	now     func() time.Time
}

func newModelRing(now func() time.Time) *modelRing {
	return &modelRing{byID: make(map[string]modelRingEntry), now: now}
}

func (m *modelRing) store(token ports.RollbackToken) {
	now := m.now()
	for len(m.entries) > 0 && !now.Before(m.entries[0].expiresAt) {
		delete(m.byID, m.entries[0].token.InvocationID)
		m.entries = m.entries[1:]
	}
	if len(m.entries) >= rollbackRingCapacity {
		delete(m.byID, m.entries[0].token.InvocationID)
		m.entries = m.entries[1:]
	}
	entry := modelRingEntry{token: token, expiresAt: now.Add(rollbackTokenTTL)}
	m.entries = append(m.entries, entry)
	m.byID[token.InvocationID] = entry
}

func (m *modelRing) take(id string) (ports.RollbackToken, error) {
	now := m.now()
	entry, ok := m.byID[id]
	if !ok {
		return ports.RollbackToken{}, errRollbackTokenNotFound
	}
	delete(m.byID, id)
	for i, e := range m.entries {
		if e.token.InvocationID == id {
			m.entries = append(m.entries[:i], m.entries[i+1:]...)
			break
		}
	}
	if !now.Before(entry.expiresAt) {
		return ports.RollbackToken{}, &rollbackExpiredError{tool: entry.token.ToolName}
	}
	return entry.token, nil
}

func (m *modelRing) size() int { return len(m.entries) }

// takeOutcome classifies a ring take result for cross-comparison.
type takeOutcome struct {
	kind    string // "ok" | "notfound" | "expired"
	tokenID string
}

func classifyTake(token ports.RollbackToken, err error) takeOutcome {
	switch {
	case err == nil:
		return takeOutcome{kind: "ok", tokenID: token.InvocationID}
	case isExpiredError(err):
		return takeOutcome{kind: "expired"}
	default:
		return takeOutcome{kind: "notfound"}
	}
}

func isExpiredError(err error) bool {
	var expired *rollbackExpiredError
	return errors.As(err, &expired)
}

func TestRollbackRing_PropertyAgainstModel(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	now := time.Unix(1_000_000, 0)
	ring := newRollbackRing()
	ring.now = func() time.Time { return now }
	model := newModelRing(func() time.Time { return now })

	var ids []string
	for i := 0; i < 1000; i++ {
		if len(ids) == 0 {
			storeID := fmt.Sprintf("tok-%d", i)
			ids = append(ids, storeID)
			ring.store(ports.RollbackToken{InvocationID: storeID, ToolName: "prop", Args: map[string]any{"i": i}})
			model.store(ports.RollbackToken{InvocationID: storeID, ToolName: "prop"})
			require.Equal(t, model.size(), ring.size(), "size mismatch after mandatory store")
			continue
		}
		switch rng.Intn(10) {
		case 0, 1, 2, 3, 4:
			storeID := fmt.Sprintf("tok-%d", i)
			ids = append(ids, storeID)
			ring.store(ports.RollbackToken{InvocationID: storeID, ToolName: "prop", Args: map[string]any{"i": i}})
			model.store(ports.RollbackToken{InvocationID: storeID, ToolName: "prop"})
			require.Equal(t, model.size(), ring.size(), "size mismatch after store %d", i)
		case 5, 6, 7:
			id := ids[rng.Intn(len(ids))]
			want := classifyTake(model.take(id))
			got := classifyTake(ring.take(id))
			require.Equal(t, want, got, "take divergence for %q at op %d", id, i)
		case 8:
			// Advance the shared clock; spans may or may not cross the TTL.
			now = now.Add(time.Duration(rng.Intn(20)) * time.Minute)
		case 9:
			id := "absent-token"
			want := classifyTake(model.take(id))
			got := classifyTake(ring.take(id))
			require.Equal(t, want, got, "absent-token divergence at op %d", i)
		}
		require.LessOrEqual(t, ring.size(), rollbackRingCapacity, "capacity violated at op %d", i)
	}
}

func TestRollbackRing_ConcurrentAccess(t *testing.T) {
	ring := newRollbackRing()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 300; i++ {
				id := ring.store(ports.RollbackToken{InvocationID: fmt.Sprintf("g%d-%d", g, i), Args: map[string]any{"g": g, "i": i}})
				// Randomly take back some tokens; the remainder exercise the
				// capacity eviction path.
				if i%3 == 0 {
					_, _ = ring.take(id)
				}
			}
		}(g)
	}
	wg.Wait()
	require.LessOrEqual(t, ring.size(), rollbackRingCapacity)
	require.Greater(t, ring.size(), 0)
}
