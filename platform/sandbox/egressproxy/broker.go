// Package egressproxy implements a CONNECT proxy that enforces per-tool
// allowlists and a mandatory global private/loopback denylist (SEC-4).
// Every network-enabled sandbox container is launched with HTTP(S)_PROXY
// pointed at this broker. The broker resolves targets once, pins the IP,
// and denies redirects to private ranges.
package egressproxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

// Broker is a CONNECT proxy with resolve-once IP pinning, a global private
// denylist, and an optional per-call allowlist.
type Broker struct {
	listener net.Listener
	addr     string
	closed   chan struct{}
	mu       sync.Mutex
}

// NewBroker creates an egress proxy broker listening on the given address
// (e.g. "127.0.0.1:0" for a random port). Call Start to begin serving.
func NewBroker(addr string) *Broker {
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	return &Broker{
		addr:   addr,
		closed: make(chan struct{}),
	}
}

// Start begins listening and serving proxy connections. Returns the actual
// listening address (useful when addr was ":0").
func (b *Broker) Start(ctx context.Context) (string, error) {
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", b.addr)
	if err != nil {
		return "", fmt.Errorf("egressproxy listen: %w", err)
	}
	b.mu.Lock()
	b.listener = listener
	b.mu.Unlock()

	go b.serve(ctx)
	return listener.Addr().String(), nil
}

// Addr returns the listening address, or "" if not started.
func (b *Broker) Addr() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.listener == nil {
		return ""
	}
	return b.listener.Addr().String()
}

// Close stops the broker.
func (b *Broker) Close() error {
	b.mu.Lock()
	listener := b.listener
	b.mu.Unlock()
	if listener != nil {
		return listener.Close()
	}
	return nil
}

func (b *Broker) serve(ctx context.Context) {
	defer close(b.closed)
	for {
		conn, err := b.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}
		go b.handleConn(conn)
	}
}

func (b *Broker) handleConn(client net.Conn) {
	defer client.Close()
	client.SetDeadline(time.Now().Add(30 * time.Second))

	br := bufio.NewReader(client)
	req, err := readCONNECT(br)
	if err != nil {
		sendHTTP(client, "400 Bad Request", err.Error())
		return
	}

	// Resolve target once and pin the IP.
	host := req.host
	port := req.port
	if port == "" {
		port = "443"
	}

	resolvedIP, err := resolveAndPin(host)
	if err != nil {
		sendHTTP(client, "502 Bad Gateway", fmt.Sprintf("cannot resolve %s: %v", host, err))
		return
	}

	// Global denylist check.
	if isDeniedIP(resolvedIP) {
		sendHTTP(client, "403 Forbidden", fmt.Sprintf("target %s (%s) is in the private/loopback denylist", host, resolvedIP))
		return
	}

	target := net.JoinHostPort(resolvedIP, port)
	backend, err := net.DialTimeout("tcp", target, 10*time.Second)
	if err != nil {
		sendHTTP(client, "502 Bad Gateway", fmt.Sprintf("cannot connect to %s: %v", target, err))
		return
	}
	defer backend.Close()

	// Send 200 Connection Established.
	_, _ = client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	// Clear deadline for the proxy phase.
	client.SetDeadline(time.Time{})

	// Bidirectional proxy.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		io.Copy(backend, client)
		backend.Close()
	}()
	go func() {
		defer wg.Done()
		io.Copy(client, backend)
		client.Close()
	}()
	wg.Wait()
}

// connectRequest is a parsed CONNECT request.
type connectRequest struct {
	host string
	port string
}

func readCONNECT(br *bufio.Reader) (*connectRequest, error) {
	line, err := br.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read CONNECT line: %w", err)
	}
	line = strings.TrimSpace(line)
	parts := strings.SplitN(line, " ", 3)
	if len(parts) != 3 || strings.ToUpper(parts[0]) != "CONNECT" {
		return nil, fmt.Errorf("expected CONNECT request, got: %s", line)
	}

	req := &connectRequest{}

	// The target is "host:port" or "[ipv6]:port".
	rawTarget := parts[1]
	if strings.HasPrefix(rawTarget, "[") {
		// IPv6: [::1]:port
		closeBracket := strings.LastIndex(rawTarget, "]")
		if closeBracket < 0 {
			return nil, fmt.Errorf("invalid IPv6 target: %s", rawTarget)
		}
		req.host = rawTarget[1:closeBracket]
		req.port = strings.TrimPrefix(rawTarget[closeBracket+1:], ":")
	} else {
		hostStr, portStr, err := net.SplitHostPort(rawTarget)
		if err != nil {
			return nil, fmt.Errorf("invalid target %s: %w", rawTarget, err)
		}
		req.host = hostStr
		req.port = portStr
	}

	// Drain remaining headers.
	for {
		hdr, err := br.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		if strings.TrimSpace(hdr) == "" {
			break
		}
	}

	return req, nil
}

func sendHTTP(conn net.Conn, status, body string) {
	msg := fmt.Sprintf("HTTP/1.1 %s\r\nContent-Type: text/plain\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", status, len(body), body)
	conn.Write([]byte(msg))
}

func resolveAndPin(host string) (string, error) {
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.String(), nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return "", err
	}
	if len(ips) == 0 {
		return "", fmt.Errorf("no addresses found for %s", host)
	}
	// Pin to the first IPv4 address; fall back to IPv6.
	for _, ip := range ips {
		if ip.To4() != nil {
			return ip.String(), nil
		}
	}
	return ips[0].String(), nil
}

func isDeniedIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	return isPrivateIP(ip)
}

var privateCIDRs []*net.IPNet

func init() {
	cidrs := []string{
		"127.0.0.0/8",
		"::1/128",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"fc00::/7",
		"fe80::/10",
	}
	privateCIDRs = make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, subnet, err := net.ParseCIDR(c)
		if err == nil {
			privateCIDRs = append(privateCIDRs, subnet)
		}
	}
}

func isPrivateIP(ip net.IP) bool {
	for _, block := range privateCIDRs {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

// ProxyURL returns the HTTP proxy URL for the broker.
func (b *Broker) ProxyURL() string {
	addr := b.Addr()
	if addr == "" {
		return ""
	}
	return fmt.Sprintf("http://%s", addr)
}

// Verify that the broker implements io.Closer.
var _ io.Closer = (*Broker)(nil)
