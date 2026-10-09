package persistence

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWriterAuditRingConcurrentPersists hammers the writer from many
// goroutines and asserts the audit ring stays bounded, unique, and race-free.
func TestWriterAuditRingConcurrentPersists(t *testing.T) {
	store := newWriterTestStore(t)
	writer := NewWriter(store, nil, nil, nil)

	const goroutines = 32
	const perGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				req := dedupRequest(fmt.Sprintf("goroutine-%d-item-%d", g, i))
				_, err := writer.Persist(context.Background(), req)
				assert.NoError(t, err)
			}
		}(g)
	}
	wg.Wait()

	records := writer.GetAuditLog()
	require.LessOrEqual(t, len(records), auditLogCap)
	require.NotEmpty(t, records)

	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		_, duplicate := seen[record.AuditID]
		require.False(t, duplicate, "audit ID %q appeared twice", record.AuditID)
		seen[record.AuditID] = struct{}{}
	}
}

func TestWriterAuditLogReturnsCopy(t *testing.T) {
	writer := NewWriter(nil, nil, nil, nil)
	writer.writeAuditRecord(PersistenceRequest{}, &PersistenceResult{Action: ActionCreated}, "test")

	first := writer.GetAuditLog()
	require.Len(t, first, 1)
	first[0].Reason = "mutated"

	second := writer.GetAuditLog()
	require.Equal(t, "test", second[0].Reason, "GetAuditLog must return a copy")
}

func TestWriterAuditRingDropsOldest(t *testing.T) {
	writer := NewWriter(nil, nil, nil, nil)
	for i := 0; i < auditLogCap+17; i++ {
		writer.writeAuditRecord(PersistenceRequest{}, &PersistenceResult{Action: ActionCreated}, fmt.Sprintf("record-%d", i))
	}
	records := writer.GetAuditLog()
	require.Len(t, records, auditLogCap)
}
