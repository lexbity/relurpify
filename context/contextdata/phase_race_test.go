package contextdata

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestEnvelopeConcurrentReadWrite exercises the hot-path readers that back
// execution-phase and interaction bookkeeping while writers mutate the same
// envelope. Before getWorkingValue acquired the read lock this test produced a
// fatal concurrent map read/write under -race: GetExecutionPhase and
// StringSliceFromContext read WorkingData while SetWorkingValueWithClass
// wrote it.
func TestEnvelopeConcurrentReadWrite(t *testing.T) {
	env := NewEnvelope("task-race", "session-race")
	env.SetExecutionPhase("seed")

	const writers = 8
	const readers = 8
	const iterations = 400

	done := make(chan struct{})
	go func() {
		defer close(done)
		var wg sync.WaitGroup
		for w := 0; w < writers; w++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				keys := []string{"_execution_phase", "shared.a", "shared.b"}
				for i := 0; i < iterations; i++ {
					key := keys[(id+i)%len(keys)]
					if key == "_execution_phase" {
						env.SetExecutionPhase(fmt.Sprintf("phase-%d-%d", id, i))
						continue
					}
					env.SetWorkingValueWithClass(key, i, MemoryClassTask)
				}
			}(w)
		}
		for r := 0; r < readers; r++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < iterations; i++ {
					_ = env.GetExecutionPhase()
					_ = env.GetInteractions()
					_ = env.StringSliceFromContext("shared.a")
					_ = env.WorkingDataSnapshot()
					_ = env.AssemblyMetadataSnapshot()
					_ = env.Clone()
					_ = env.HandoffClone()
				}
			}()
		}
		wg.Wait()
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("concurrent envelope access did not complete (possible deadlock)")
	}
}
