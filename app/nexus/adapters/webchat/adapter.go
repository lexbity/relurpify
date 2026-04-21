package webchat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/relurpnet/channel"
	"github.com/gorilla/websocket"
)

type Adapter struct {
	upgrader websocket.Upgrader
	sink     channel.EventSink

	mu      sync.RWMutex
	conns   map[string]*websocket.Conn
	status  channel.AdapterStatus
	counter int
}

func (a *Adapter) Name() string { return "webchat" }

func (a *Adapter) Start(_ context.Context, _ json.RawMessage, sink channel.EventSink) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sink = sink
	if a.conns == nil {
		a.conns = map[string]*websocket.Conn{}
	}
	a.status.Connected = true
	return nil
}

func (a *Adapter) Stop(_ context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for id, conn := range a.conns {
		_ = conn.Close()
		delete(a.conns, id)
	}
	a.status.Connected = false
	return nil
}

func (a *Adapter) Status() channel.AdapterStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.status
}

func (a *Adapter) Send(_ context.Context, msg channel.OutboundMessage) error {
	a.mu.RLock()
	conn, ok := a.conns[msg.ConversationID]
	a.mu.RUnlock()
	if !ok {
		return fmt.Errorf("conversation %s not connected", msg.ConversationID)
	}
	return conn.WriteMessage(websocket.TextMessage, []byte(msg.Content.Text))
}

func (a *Adapter) Handler() http.Handler {
	upgrader := a.upgrader
	if upgrader.CheckOrigin == nil {
		upgrader.CheckOrigin = func(*http.Request) bool { return true }
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		a.mu.Lock()
		a.counter++
		id := fmt.Sprintf("webchat-%d", a.counter)
		if a.conns == nil {
			a.conns = map[string]*websocket.Conn{}
		}
		a.conns[id] = conn
		a.mu.Unlock()
		go a.readLoop(id, conn)
	})
}

func (a *Adapter) readLoop(id string, conn *websocket.Conn) {
	defer func() {
		a.mu.Lock()
		delete(a.conns, id)
		a.mu.Unlock()
		_ = conn.Close()
	}()
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if a.sink == nil {
			continue
		}
		_ = a.emitInbound(id, string(data))
	}
}

func (a *Adapter) emitInbound(id, text string) error {
	if a.sink == nil {
		return nil
	}
	msg := channel.InboundMessage{
		Channel: "webchat",
		Sender: channel.Identity{
			ChannelID: id,
		},
		Conversation: channel.Conversation{
			Kind: "dm",
			ID:   id,
		},
		Content: channel.MessageContent{
			Text: text,
		},
	}
	payload, _ := json.Marshal(msg)
	return a.sink.Emit(context.Background(), core.FrameworkEvent{
		Timestamp: time.Now().UTC(),
		Type:      core.FrameworkEventMessageInbound,
		Payload:   payload,
		Actor:     core.EventActor{Kind: "channel", ID: "webchat"},
		Partition: "local",
	})
}
