package keylock

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestLockerWithNilFunction(t *testing.T) {
	var l Locker
	if err := l.With("workflow-1", nil); err != nil {
		t.Fatalf("With nil fn returned error: %v", err)
	}
}

func TestLockerWithDefaultKeySerializesAccess(t *testing.T) {
	var l Locker

	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondEntered := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		err := l.With("", func() error {
			close(firstEntered)
			<-releaseFirst
			return nil
		})
		if err != nil {
			t.Errorf("first With returned error: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		<-firstEntered
		err := l.With("", func() error {
			close(secondEntered)
			return nil
		})
		if err != nil {
			t.Errorf("second With returned error: %v", err)
		}
	}()

	select {
	case <-firstEntered:
	case <-time.After(time.Second):
		t.Fatal("first goroutine did not enter lock")
	}

	select {
	case <-secondEntered:
		t.Fatal("second goroutine entered before first released lock")
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseFirst)

	select {
	case <-secondEntered:
	case <-time.After(time.Second):
		t.Fatal("second goroutine did not enter after release")
	}

	wg.Wait()
}

func TestLockerWithPropagatesErrorsAndReleasesLock(t *testing.T) {
	var l Locker
	expected := errors.New("boom")

	err := l.With("workflow-2", func() error { return expected })
	if !errors.Is(err, expected) {
		t.Fatalf("With returned %v, want %v", err, expected)
	}

	called := false
	if err := l.With("workflow-2", func() error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("With after error returned error: %v", err)
	}
	if !called {
		t.Fatal("expected lock to be reusable after error")
	}
}

func TestLockerReleaseNilEntryAndAcquireDirect(t *testing.T) {
	var l Locker

	l.release("", nil)

	entry := l.acquire("")
	if entry == nil {
		t.Fatal("expected entry for default key")
	}
	if entry.refs != 1 {
		t.Fatalf("expected refs=1, got %d", entry.refs)
	}
	l.release("", entry)

	entry = l.acquire("workflow-3")
	if entry == nil {
		t.Fatal("expected entry for workflow key")
	}
	l.release("workflow-3", entry)
}

func TestLockerConcurrentAcquireExercisesInternalRaces(t *testing.T) {
	var l Locker

	const workers = 16
	const iterations = 64

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < iterations; j++ {
				if err := l.With("contention-key", func() error { return nil }); err != nil {
					t.Errorf("With returned error: %v", err)
					return
				}
			}
		}()
	}
	close(start)
	wg.Wait()
}
