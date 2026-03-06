package cdp

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
	Close() error
}

type websocketTransport struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
	nextID  atomic.Int64
	closed  atomic.Bool

	mu      sync.Mutex
	pending map[int64]chan callResult
}

type callResult struct {
	result json.RawMessage
	err    error
}

type responseEnvelope struct {
	ID     int64           `json:"id,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *responseError  `json:"error,omitempty"`
}

type responseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type requestEnvelope struct {
	ID     int64          `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params,omitempty"`
}

func newWebsocketTransport(ctx context.Context, wsURL string) (*websocketTransport, error) {
	dialer := websocket.Dialer{}
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, err
	}
	t := &websocketTransport{
		conn:    conn,
		pending: make(map[int64]chan callResult),
	}
	go t.readLoop()
	return t, nil
}

func (t *websocketTransport) Call(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	if t.closed.Load() {
		return nil, errors.New("cdp transport closed")
	}
	id := t.nextID.Add(1)
	resultCh := make(chan callResult, 1)

	t.mu.Lock()
	t.pending[id] = resultCh
	t.mu.Unlock()

	req := requestEnvelope{
		ID:     id,
		Method: method,
		Params: params,
	}
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
		ch <- callResult{err: errors.New("cdp transport closed")}
		close(ch)
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
		if envelope.ID == 0 {
			continue
		}
		t.mu.Lock()
		ch, ok := t.pending[envelope.ID]
		if ok {
			delete(t.pending, envelope.ID)
		}
		t.mu.Unlock()
		if !ok {
			continue
		}
		if envelope.Error != nil {
			ch <- callResult{err: fmt.Errorf("cdp error %d: %s", envelope.Error.Code, envelope.Error.Message)}
			close(ch)
			continue
		}
		ch <- callResult{result: envelope.Result}
		close(ch)
	}
}

func (t *websocketTransport) removePending(id int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.pending, id)
}
