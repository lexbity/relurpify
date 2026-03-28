package runtime

import (
	"context"
	"sync"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

const sessionWorkerIdleTimeout = 5 * time.Minute
const sessionWorkerQueueDepth = 64

// sessionBoundaryKey returns a stable routing key from an event so that all
// events belonging to the same logical session are dispatched to the same
// worker goroutine — guaranteeing sequential execution within a session while
// allowing parallel execution across different sessions.
func sessionBoundaryKey(event core.FrameworkEvent, channel string) string {
	return event.Partition + "|" + channel + "|" + event.Actor.ID
}

// nexusWorkItem is one unit of work queued to a session worker.
type nexusWorkItem struct {
	event       core.FrameworkEvent
	instruction string
	sessionKey  string
	metadata    map[string]any
}

// sessionWorker owns a single goroutine that drains its queue sequentially.
type sessionWorker struct {
	queue  chan nexusWorkItem
	cancel context.CancelFunc
}

// sessionDispatcher routes nexus events to per-session workers, ensuring
// sequential execution within each session boundary.
type sessionDispatcher struct {
	rt          *Runtime
	client      *NexusClient
	idleTimeout time.Duration

	mu      sync.Mutex
	workers map[string]*sessionWorker
}

func newSessionDispatcher(rt *Runtime, client *NexusClient) *sessionDispatcher {
	return &sessionDispatcher{
		rt:          rt,
		client:      client,
		idleTimeout: sessionWorkerIdleTimeout,
		workers:     make(map[string]*sessionWorker),
	}
}

// Dispatch decodes an event and routes it to the appropriate session worker.
// Events that cannot be decoded or have no instruction are silently dropped.
func (d *sessionDispatcher) Dispatch(ctx context.Context, event core.FrameworkEvent) {
	if event.Type != core.FrameworkEventMessageInbound && event.Type != core.FrameworkEventSessionMessage {
		return
	}
	if event.Actor.Kind == "agent" {
		return
	}
	instruction, sessionKey, metadata, ok := decodeNexusInstruction(event)
	if !ok {
		return
	}
	channel, _ := metadata["channel"].(string)
	key := sessionBoundaryKey(event, channel)
	item := nexusWorkItem{
		event:       event,
		instruction: instruction,
		sessionKey:  sessionKey,
		metadata:    metadata,
	}
	d.getOrCreateWorker(ctx, key).queue <- item
}

// getOrCreateWorker returns an existing worker for the key or starts a new one.
func (d *sessionDispatcher) getOrCreateWorker(ctx context.Context, key string) *sessionWorker {
	d.mu.Lock()
	defer d.mu.Unlock()
	if w, ok := d.workers[key]; ok {
		return w
	}
	workerCtx, cancel := context.WithCancel(ctx)
	w := &sessionWorker{
		queue:  make(chan nexusWorkItem, sessionWorkerQueueDepth),
		cancel: cancel,
	}
	d.workers[key] = w
	go d.runWorker(workerCtx, key, w)
	return w
}

// runWorker processes items from the worker queue sequentially and exits after
// the idle timeout elapses with no new work.
func (d *sessionDispatcher) runWorker(ctx context.Context, key string, w *sessionWorker) {
	defer func() {
		w.cancel()
		d.mu.Lock()
		if d.workers[key] == w {
			delete(d.workers, key)
		}
		d.mu.Unlock()
	}()
	idle := time.NewTimer(d.idleTimeout)
	defer idle.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case item, ok := <-w.queue:
			if !ok {
				return
			}
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(d.idleTimeout)
			if err := d.process(ctx, item); err != nil && d.rt != nil && d.rt.Logger != nil {
				d.rt.Logger.Printf("nexus session %s: event handling failed: %v", key, err)
			}
		case <-idle.C:
			return
		}
	}
}

// process executes a single work item and sends the response back via the client.
func (d *sessionDispatcher) process(ctx context.Context, item nexusWorkItem) error {
	if d.rt == nil || d.client == nil {
		return nil
	}
	result, err := d.rt.ExecuteInstructionStream(ctx, item.instruction, core.TaskTypeCodeModification, item.metadata, func(string) {})
	if err != nil {
		return d.client.SendResponse(ctx, item.sessionKey, err.Error())
	}
	return d.client.SendResponse(ctx, item.sessionKey, formatNexusResult(result))
}

// consumeNexusEventsWithDispatcher drives the session dispatcher from the
// client subscription channel.  It replaces the old consumeNexusEvents loop.
func consumeNexusEventsWithDispatcher(ctx context.Context, rt *Runtime, client *NexusClient) {
	dispatcher := newSessionDispatcher(rt, client)
	ch, unsub := client.Subscribe(32)
	defer unsub()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			dispatcher.Dispatch(ctx, event)
		}
	}
}
