package bidi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
)

type Transport interface {
	Call(ctx context.Context, method string, params map[string]any) (json.RawMessage, error)
	Subscribe(method string) <-chan json.RawMessage
	Close() error
}

type websocketTransport struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
	nextID  atomic.Int64
	closed  atomic.Bool

	mu          sync.Mutex
	pending     map[int64]chan callResult
	subscribers map[string][]chan json.RawMessage
}

type callResult struct {
	result json.RawMessage
	err    error
}

type responseEnvelope struct {
	Type    string          `json:"type"`
	ID      int64           `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   string          `json:"error,omitempty"`
	Message string          `json:"message,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type requestEnvelope struct {
	ID     int64          `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

func newWebsocketTransport(ctx context.Context, wsURL string) (*websocketTransport, error) {
	dialer := websocket.Dialer{}
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, err
	}
	t := &websocketTransport{
		conn:        conn,
		pending:     make(map[int64]chan callResult),
		subscribers: make(map[string][]chan json.RawMessage),
	}
	go t.readLoop()
	return t, nil
}

func (t *websocketTransport) Call(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	if t.closed.Load() {
		return nil, errors.New("bidi transport closed")
	}
	id := t.nextID.Add(1)
	resultCh := make(chan callResult, 1)
	if params == nil {
		params = map[string]any{}
	}

	t.mu.Lock()
	t.pending[id] = resultCh
	t.mu.Unlock()

	req := requestEnvelope{ID: id, Method: method, Params: params}
	t.writeMu.Lock()
	err := t.conn.WriteJSON(req)
	t.writeMu.Unlock()
	if err != nil {
		t.removePending(id)
		return nil, err
	}

	select {
	case res := <-resultCh:
		return res.result, res.err
	case <-ctx.Done():
		t.removePending(id)
		return nil, ctx.Err()
	}
}

func (t *websocketTransport) Subscribe(method string) <-chan json.RawMessage {
	ch := make(chan json.RawMessage, 16)
	t.mu.Lock()
	t.subscribers[method] = append(t.subscribers[method], ch)
	t.mu.Unlock()
	return ch
}

func (t *websocketTransport) Close() error {
	if t == nil {
		return nil
	}
	if !t.closed.CompareAndSwap(false, true) {
		return nil
	}
	t.mu.Lock()
	for id, ch := range t.pending {
		delete(t.pending, id)
		ch <- callResult{err: errors.New("bidi transport closed")}
		close(ch)
	}
	for method, subs := range t.subscribers {
		delete(t.subscribers, method)
		for _, sub := range subs {
			close(sub)
		}
	}
	t.mu.Unlock()
	return t.conn.Close()
}

func (t *websocketTransport) readLoop() {
	for {
		var envelope responseEnvelope
		if err := t.conn.ReadJSON(&envelope); err != nil {
			_ = t.Close()
			return
		}
		switch envelope.Type {
		case "success":
			t.mu.Lock()
			ch, ok := t.pending[envelope.ID]
			if ok {
				delete(t.pending, envelope.ID)
			}
			t.mu.Unlock()
			if ok {
				ch <- callResult{result: envelope.Result}
				close(ch)
			}
		case "error":
			t.mu.Lock()
			ch, ok := t.pending[envelope.ID]
			if ok {
				delete(t.pending, envelope.ID)
			}
			t.mu.Unlock()
			if ok {
				ch <- callResult{err: fmt.Errorf("bidi error %s: %s", envelope.Error, envelope.Message)}
				close(ch)
			}
		case "event":
			t.mu.Lock()
			subs := append([]chan json.RawMessage(nil), t.subscribers[envelope.Method]...)
			t.mu.Unlock()
			for _, sub := range subs {
				select {
				case sub <- envelope.Params:
				default:
				}
			}
		}
	}
}

func (t *websocketTransport) removePending(id int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.pending, id)
}
