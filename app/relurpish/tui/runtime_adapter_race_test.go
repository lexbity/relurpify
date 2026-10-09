package tui

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	runtimesvc "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// TestRuntimeAdapterReloadUnderConcurrentReads hammers ReloadWorkspace against
// concurrent stateless adapter reads to prove the runtime pointer swap is read
// atomically (no plain-field race on `rt`, D10) under -race. The reader goroutine
// deliberately touches only methods that read immutable or mutex-guarded
// runtime state, so the race surface exercised is precisely the pointer swap.
func TestRuntimeAdapterReloadUnderConcurrentReads(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()

	rt, err := runtimesvc.New(context.Background(), runtimesvc.ConfigForWorkspace(runtimesvc.DefaultConfig(), t.TempDir()), config.Secrets{})
	require.NoError(t, err)
	adapter := newRuntimeAdapter(rt)
	require.NotNil(t, adapter)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = adapter.AvailableAgents()
			_ = adapter.RecordingMode()
			_ = adapter.SandboxBackend()
			_ = adapter.ExecutionMode()
			_ = adapter.ActiveWorkflowID()
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 3; i++ {
			dir := dirA
			if i%2 == 0 {
				dir = dirB
			}
			if err := adapter.ReloadWorkspace(context.Background(), dir); err != nil {
				t.Errorf("reload %d: %v", i, err)
				return
			}
		}
		close(stop)
	}()

	wg.Wait()
}
