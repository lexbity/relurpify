package telemetry

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestBroadcastFanOut(t *testing.T) {
	b := NewBroadcastSink()
	ch1, cancel1 := b.Subscribe(16)
	ch2, cancel2 := b.Subscribe(16)
	ch3, cancel3 := b.Subscribe(16)
	defer cancel1()
	defer cancel2()
	defer cancel3()

	ev := Event{Type: EventGraphStart, Message: "hello"}
	b.Emit(ev)

	check := func(t *testing.T, ch <-chan Event) {
		t.Helper()
		select {
		case got := <-ch:
			if got.Type != EventGraphStart {
				t.Fatalf("expected type %s, got %s", EventGraphStart, got.Type)
			}
			if got.Message != "hello" {
				t.Fatalf("expected message 'hello', got %q", got.Message)
			}
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for event")
		}
	}
	check(t, ch1)
	check(t, ch2)
	check(t, ch3)
}

func TestBroadcastDropOnFullBuffer(t *testing.T) {
	b := NewBroadcastSink()
	ch, cancel := b.Subscribe(4)
	defer cancel()

	for i := 0; i < 10; i++ {
		b.Emit(Event{Type: EventGraphStart, TaskID: "t"})
	}

	if d := b.DroppedTotal(); d == 0 {
		t.Fatal("expected drops due to full buffer, got 0")
	}

	received := 0
	for {
		select {
		case <-ch:
			received++
		default:
			goto done
		}
	}
done:
	t.Logf("received %d of 10 sent (buf=4)", received)
}

func TestBroadcastEmitNeverBlocks(t *testing.T) {
	b := NewBroadcastSink()
	ch, cancel := b.Subscribe(2)
	defer cancel()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			b.Emit(Event{Type: EventGraphStart})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Emit blocked on full buffer")
	}
	_ = ch
}

func TestBroadcastSeqStamped(t *testing.T) {
	b := NewBroadcastSink()
	ch, cancel := b.Subscribe(16)
	defer cancel()

	b.Emit(Event{Type: EventGraphStart, Seq: 0})
	b.Emit(Event{Type: EventGraphStart, Seq: 100})
	b.Emit(Event{Type: EventGraphStart, Seq: 0})

	var seqs []uint64
	for i := 0; i < 3; i++ {
		select {
		case ev := <-ch:
			seqs = append(seqs, ev.Seq)
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for event")
		}
	}
	if seqs[0] == 0 {
		t.Fatal("expected seq stamped for Seq=0 event")
	}
	if seqs[1] != 100 {
		t.Fatalf("expected seq 100 preserved, got %d", seqs[1])
	}
	if seqs[2] == 0 || seqs[2] <= seqs[0] {
		t.Fatalf("expected monotonic seq, got %d after %d", seqs[2], seqs[0])
	}
}

func TestBroadcastCancelClosesChannel(t *testing.T) {
	b := NewBroadcastSink()
	ch, cancel := b.Subscribe(16)

	b.Emit(Event{Type: EventGraphStart})

	select {
	case _, ok := <-ch:
		if !ok {
			t.Fatal("channel should be open after subscribe")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}

	cancel()

	_, ok := <-ch
	if ok {
		t.Fatal("channel should be closed after cancel")
	}
}

func TestBroadcastEmitAfterCancelNoPanic(t *testing.T) {
	b := NewBroadcastSink()
	_, cancel := b.Subscribe(16)
	cancel()
	b.Emit(Event{Type: EventGraphStart})
	t.Log("no panic after Emit following cancel")
}

func TestBroadcastCloseClosesAll(t *testing.T) {
	b := NewBroadcastSink()
	ch1, _ := b.Subscribe(16)
	ch2, _ := b.Subscribe(16)
	ch3, _ := b.Subscribe(16)

	b.Close()

	assertClosed := func(t *testing.T, ch <-chan Event, name string) {
		t.Helper()
		_, ok := <-ch
		if ok {
			t.Fatalf("channel %s should be closed after Close", name)
		}
	}
	assertClosed(t, ch1, "ch1")
	assertClosed(t, ch2, "ch2")
	assertClosed(t, ch3, "ch3")

	if n := b.SubscriberCount(); n != 0 {
		t.Fatalf("expected subscriber count 0 after Close, got %d", n)
	}
}

func TestBroadcastSubscribeAfterCloseReturnsClosedChannel(t *testing.T) {
	b := NewBroadcastSink()
	b.Close()

	ch, cancel := b.Subscribe(16)
	_, ok := <-ch
	if ok {
		t.Fatal("expected closed channel after Close")
	}
	cancel()
}

func TestBroadcastCloseIdempotent(t *testing.T) {
	b := NewBroadcastSink()
	b.Close()
	b.Close()
	if n := b.SubscriberCount(); n != 0 {
		t.Fatalf("expected 0 after Close, got %d", n)
	}
}

func TestBroadcastCancelIdempotent(t *testing.T) {
	b := NewBroadcastSink()
	_, cancel := b.Subscribe(16)
	cancel()
	cancel()
	if n := b.SubscriberCount(); n != 0 {
		t.Fatalf("expected 0 after cancel, got %d", n)
	}
}

func TestBroadcastSubscriberCount(t *testing.T) {
	b := NewBroadcastSink()
	if n := b.SubscriberCount(); n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
	_, c1 := b.Subscribe(16)
	if n := b.SubscriberCount(); n != 1 {
		t.Fatalf("expected 1, got %d", n)
	}
	_, c2 := b.Subscribe(16)
	if n := b.SubscriberCount(); n != 2 {
		t.Fatalf("expected 2, got %d", n)
	}
	c1()
	if n := b.SubscriberCount(); n != 1 {
		t.Fatalf("expected 1 after cancel, got %d", n)
	}
	c2()
	if n := b.SubscriberCount(); n != 0 {
		t.Fatalf("expected 0 after both cancel, got %d", n)
	}
}

func TestBroadcastSubscribeDefaultsBuffer(t *testing.T) {
	b := NewBroadcastSink()
	ch, cancel := b.Subscribe(0)
	defer cancel()

	ev := Event{Type: EventGraphStart}
	b.Emit(ev)
	select {
	case got := <-ch:
		if got.Type != EventGraphStart {
			t.Fatalf("expected EventGraphStart, got %s", got.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: Subscribe(0) should default to buffer=256 and accept event")
	}
}

func TestBroadcastSubscribeNegativeBufferDefaults(t *testing.T) {
	b := NewBroadcastSink()
	ch, cancel := b.Subscribe(-1)
	defer cancel()

	ev := Event{Type: EventGraphStart}
	b.Emit(ev)
	select {
	case got := <-ch:
		if got.Type != EventGraphStart {
			t.Fatalf("expected EventGraphStart, got %s", got.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: Subscribe(-1) should default to buffer=256 and accept event")
	}
}

func TestBroadcastEmitWithNoSubscribers(t *testing.T) {
	b := NewBroadcastSink()
	b.Emit(Event{Type: EventGraphStart})
	t.Log("no panic from Emit with no subscribers")
}

func TestBroadcastEmitAfterCloseNoPanic(t *testing.T) {
	b := NewBroadcastSink()
	b.Close()
	b.Emit(Event{Type: EventGraphStart})
	t.Log("no panic from Emit after Close")
}

func TestBroadcastConcurrentEmitAndSubscribe(t *testing.T) {
	b := NewBroadcastSink()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.Emit(Event{Type: EventGraphStart})
		}()
	}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch, cancel := b.Subscribe(16)
			cancel()
			_ = ch
		}()
	}
	wg.Wait()
	if n := b.SubscriberCount(); n != 0 {
		t.Fatalf("expected 0 subscribers after concurrent test, got %d", n)
	}
}

func TestBroadcastLeakCheck(t *testing.T) {
	start := runtime.NumGoroutine()
	b := NewBroadcastSink()
	const N = 1000
	for i := 0; i < N; i++ {
		_, cancel := b.Subscribe(16)
		cancel()
	}
	if n := b.SubscriberCount(); n != 0 {
		t.Fatalf("expected 0 subscribers after %d cancels, got %d", N, n)
	}
	b.Close()
	got := runtime.NumGoroutine()
	if got > start+2 {
		t.Fatalf("possible goroutine leak: started at %d, now at %d", start, got)
	}
}

func TestBroadcastConcurrentClose(t *testing.T) {
	b := NewBroadcastSink()
	for i := 0; i < 10; i++ {
		b.Subscribe(16)
	}
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.Close()
		}()
	}
	wg.Wait()
	if n := b.SubscriberCount(); n != 0 {
		t.Fatalf("expected 0 subscribers after concurrent Close, got %d", n)
	}
}

// TestBroadcastBurstNoDrop asserts that 1,000 events emitted in <1 ms to a
// large-buffer subscriber with an active consumer are delivered with zero drops.
func TestBroadcastBurstNoDrop(t *testing.T) {
	if testing.Short() {
		t.Skip("burst test")
	}
	b := NewBroadcastSink()
	ch, cancel := b.Subscribe(4096)
	defer cancel()

	// Active consumer draining into a channel.
	received := make(chan struct{}, 4096)
	go func() {
		for range ch {
			received <- struct{}{}
		}
	}()

	const n = 1000
	for i := 0; i < n; i++ {
		b.Emit(Event{Type: EventGraphStart, TaskID: "burst", Seq: uint64(i + 1)})
	}

	// Collect all receipts with a generous timeout.
	for i := 0; i < n; i++ {
		select {
		case <-received:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for event %d/%d", i+1, n)
		}
	}

	if d := b.DroppedTotal(); d != 0 {
		t.Fatalf("expected zero dropped events in burst, got %d", d)
	}
}

func BenchmarkBroadcastEmit(b *testing.B) {
	sink := NewBroadcastSink()
	ch, cancel := sink.Subscribe(256)
	defer cancel()

	go func() {
		for ev := range ch {
			_ = ev
		}
	}()

	ev := Event{Type: EventGraphStart, TaskID: "bench", Seq: 1}
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		sink.Emit(ev)
	}
}

func BenchmarkBroadcastEmitNoSubscriber(b *testing.B) {
	sink := NewBroadcastSink()
	ev := Event{Type: EventGraphStart, TaskID: "bench", Seq: 1}
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		sink.Emit(ev)
	}
}
