package nexustest

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	fwgateway "codeburg.org/lexbit/relurpify/relurpnet/gateway"
	fwnode "codeburg.org/lexbit/relurpify/relurpnet/node"
	"github.com/gorilla/websocket"
)

type TestNodeInvocation struct {
	CorrelationID string
	CapabilityID  string
	Args          map[string]any
}

type TestNodeDevice struct {
	credential core.NodeCredential
	privateKey ed25519.PrivateKey

	mu                  sync.RWMutex
	conn                *websocket.Conn
	writeMu             sync.Mutex
	sessionID           string
	capabilities        []core.CapabilityDescriptor
	offeredCapabilities []core.CapabilityDescriptor
	invokeHandler       func(TestNodeInvocation) *core.CapabilityExecutionResult

	invocations chan TestNodeInvocation
}

func NewTestNodeDevice(deviceID string, capabilities []core.CapabilityDescriptor) (*TestNodeDevice, error) {
	cred, priv, err := fwnode.GenerateCredential(deviceID)
	if err != nil {
		return nil, err
	}
	return &TestNodeDevice{
		credential:          cred,
		privateKey:          priv,
		offeredCapabilities: append([]core.CapabilityDescriptor(nil), capabilities...),
		invocations:         make(chan TestNodeInvocation, 32),
	}, nil
}

func (d *TestNodeDevice) Credential() core.NodeCredential {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.credential
}

func (d *TestNodeDevice) DeviceID() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.credential.DeviceID
}

func (d *TestNodeDevice) SessionID() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.sessionID
}

func (d *TestNodeDevice) Capabilities() []core.CapabilityDescriptor {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return append([]core.CapabilityDescriptor(nil), d.capabilities...)
}

func (d *TestNodeDevice) OfferedCapabilities() []core.CapabilityDescriptor {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return append([]core.CapabilityDescriptor(nil), d.offeredCapabilities...)
}

func (d *TestNodeDevice) Invocations() <-chan TestNodeInvocation {
	return d.invocations
}

func (d *TestNodeDevice) SetInvokeHandler(handler func(TestNodeInvocation) *core.CapabilityExecutionResult) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.invokeHandler = handler
}

func (d *TestNodeDevice) Connect(ctx context.Context, addr, token, nodeName string, platform core.NodePlatform) error {
	if d == nil {
		return fmt.Errorf("node device required")
	}
	wsAddr := gatewayWSURL(addr)
	headers := http.Header{}
	if token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}
	conn, _, err := websocket.DefaultDialer.Dial(wsAddr, headers)
	if err != nil {
		return err
	}
	issuedAt := time.Now().UTC()
	connect := map[string]any{
		"type":                      "connect",
		"version":                   "1.0",
		"role":                      "node",
		"last_seen_seq":             0,
		"node_id":                   d.DeviceID(),
		"node_name":                 firstNonEmpty(nodeName, d.DeviceID()),
		"node_platform":             firstNonEmpty(string(platform), string(core.NodePlatformHeadless)),
		"trust_domain":              "local",
		"runtime_id":                d.DeviceID() + "-runtime",
		"runtime_version":           "test",
		"compatibility_class":       "test",
		"supported_context_classes": []string{"workflow-runtime"},
		"transport_profile":         fwgateway.TransportProfileWebSocketLoopback,
		"session_nonce":             base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%s-%d", d.DeviceID(), issuedAt.UnixNano()))),
		"session_issued_at":         issuedAt,
		"session_expires_at":        issuedAt.Add(5 * time.Minute),
		"peer_key_id":               d.DeviceID() + "-peer",
		"capabilities":              d.OfferedCapabilities(),
	}
	if err := conn.WriteJSON(connect); err != nil {
		_ = conn.Close()
		return err
	}
	var connected connectedFrame
	if err := conn.ReadJSON(&connected); err != nil {
		_ = conn.Close()
		return err
	}
	if connected.Type != "connected" {
		_ = conn.Close()
		return fmt.Errorf("unexpected connect response %q", connected.Type)
	}
	var challenge struct {
		Type  string `json:"type"`
		Nonce string `json:"nonce"`
	}
	if err := conn.ReadJSON(&challenge); err != nil {
		_ = conn.Close()
		return err
	}
	if challenge.Type != "node.challenge" {
		_ = conn.Close()
		return fmt.Errorf("unexpected challenge response %q", challenge.Type)
	}
	nonce, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(challenge.Nonce))
	if err != nil {
		_ = conn.Close()
		return err
	}
	signature := ed25519.Sign(d.privateKey, nonce)
	if err := conn.WriteJSON(map[string]any{
		"type":      "node.challenge.response",
		"signature": base64.RawURLEncoding.EncodeToString(signature),
	}); err != nil {
		_ = conn.Close()
		return err
	}

	d.mu.Lock()
	d.conn = conn
	d.sessionID = connected.SessionID
	d.capabilities = append([]core.CapabilityDescriptor(nil), connected.Capabilities...)
	d.mu.Unlock()

	go d.readLoop()
	return nil
}

func (d *TestNodeDevice) Close() error {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	conn := d.conn
	d.conn = nil
	d.mu.Unlock()
	if conn == nil {
		return nil
	}
	return conn.Close()
}

func (d *TestNodeDevice) readLoop() {
	for {
		d.mu.RLock()
		conn := d.conn
		d.mu.RUnlock()
		if conn == nil {
			return
		}
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			continue
		}
		if envelope.Type == fwnode.TransportFrameType {
			var transport struct {
				Type    string          `json:"type"`
				Payload json.RawMessage `json:"payload"`
			}
			if err := json.Unmarshal(data, &transport); err != nil {
				continue
			}
			data = transport.Payload
			if err := json.Unmarshal(data, &envelope); err != nil {
				continue
			}
		}
		if envelope.Type != "capability.invoke" {
			continue
		}
		var frame struct {
			Type          string         `json:"type"`
			CorrelationID string         `json:"correlation_id"`
			CapabilityID  string         `json:"capability_id"`
			Args          map[string]any `json:"args"`
		}
		if err := json.Unmarshal(data, &frame); err != nil {
			continue
		}
		invocation := TestNodeInvocation{
			CorrelationID: frame.CorrelationID,
			CapabilityID:  frame.CapabilityID,
			Args:          frame.Args,
		}
		select {
		case d.invocations <- invocation:
		default:
		}
		result := d.invoke(invocation)
		if result == nil {
			result = &core.CapabilityExecutionResult{Success: true}
		}
		_ = d.writeJSON(map[string]any{
			"type":           "capability.result",
			"correlation_id": frame.CorrelationID,
			"result":         result,
		})
	}
}

func (d *TestNodeDevice) invoke(invocation TestNodeInvocation) *core.CapabilityExecutionResult {
	d.mu.RLock()
	handler := d.invokeHandler
	d.mu.RUnlock()
	if handler != nil {
		return handler(invocation)
	}
	return &core.CapabilityExecutionResult{
		Success: true,
		Data: map[string]any{
			"capability_id": invocation.CapabilityID,
			"args":          invocation.Args,
		},
	}
}

func (d *TestNodeDevice) writeJSON(v any) error {
	d.mu.RLock()
	conn := d.conn
	d.mu.RUnlock()
	if conn == nil {
		return fmt.Errorf("node device disconnected")
	}
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	return conn.WriteJSON(v)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
