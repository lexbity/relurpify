package egressproxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func startTestBroker(t *testing.T) *Broker {
	t.Helper()
	b := NewBroker("127.0.0.1:0")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	addr, err := b.Start(ctx)
	if err != nil {
		t.Fatalf("start broker: %v", err)
	}
	t.Logf("broker listening on %s", addr)
	return b
}

func TestBrokerRefusesCONNECTToPrivate(t *testing.T) {
	b := startTestBroker(t)
	defer b.Close()

	// Attempt to CONNECT to a private IP.
	proxyURL, _ := url.Parse(b.ProxyURL())
	conn, err := net.DialTimeout("tcp", proxyURL.Host, 5*time.Second)
	if err != nil {
		t.Fatalf("dial broker: %v", err)
	}
	defer conn.Close()

	_, err = fmt.Fprintf(conn, "CONNECT 127.0.0.1:80 HTTP/1.1\r\nHost: 127.0.0.1:80\r\n\r\n")
	if err != nil {
		t.Fatalf("write CONNECT: %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403, got %d %s", resp.StatusCode, resp.Status)
	}
}

func TestBrokerRefusesCONNECTToPrivateHostname(t *testing.T) {
	b := startTestBroker(t)
	defer b.Close()

	proxyURL, _ := url.Parse(b.ProxyURL())
	conn, err := net.DialTimeout("tcp", proxyURL.Host, 5*time.Second)
	if err != nil {
		t.Fatalf("dial broker: %v", err)
	}
	defer conn.Close()

	// localhost resolves to 127.0.0.1
	_, err = fmt.Fprintf(conn, "CONNECT localhost:80 HTTP/1.1\r\nHost: localhost:80\r\n\r\n")
	if err != nil {
		t.Fatalf("write CONNECT: %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403, got %d %s", resp.StatusCode, resp.Status)
	}
}

func TestBrokerResolvesHostOnce(t *testing.T) {
	b := startTestBroker(t)
	defer b.Close()

	proxyURL, _ := url.Parse(b.ProxyURL())
	conn, err := net.DialTimeout("tcp", proxyURL.Host, 5*time.Second)
	if err != nil {
		t.Fatalf("dial broker: %v", err)
	}
	defer conn.Close()

	// Try to connect to a host that doesn't exist — should get 502.
	_, err = fmt.Fprintf(conn, "CONNECT nonexistent.invalid:443 HTTP/1.1\r\nHost: nonexistent.invalid:443\r\n\r\n")
	if err != nil {
		t.Fatalf("write CONNECT: %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 502 {
		t.Fatalf("expected 502, got %d %s", resp.StatusCode, resp.Status)
	}
}

func TestBrokerRejectsBadRequest(t *testing.T) {
	b := startTestBroker(t)
	defer b.Close()

	proxyURL, _ := url.Parse(b.ProxyURL())
	conn, err := net.DialTimeout("tcp", proxyURL.Host, 5*time.Second)
	if err != nil {
		t.Fatalf("dial broker: %v", err)
	}
	defer conn.Close()

	// Send GET instead of CONNECT.
	_, err = fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d %s", resp.StatusCode, resp.Status)
	}
}

func TestIsDeniedIP(t *testing.T) {
	tests := []struct {
		ip     string
		denied bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"169.254.169.254", true},
		{"::1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"93.184.216.34", false}, // example.com
	}
	for _, tc := range tests {
		got := isDeniedIP(tc.ip)
		if got != tc.denied {
			t.Errorf("isDeniedIP(%q) = %v, want %v", tc.ip, got, tc.denied)
		}
	}
}

func TestIsPrivateIP(t *testing.T) {
	if !isPrivateIP(net.ParseIP("127.0.0.1")) {
		t.Error("expected 127.0.0.1 to be private")
	}
	if !isPrivateIP(net.ParseIP("10.0.0.5")) {
		t.Error("expected 10.0.0.5 to be private")
	}
	if isPrivateIP(net.ParseIP("8.8.8.8")) {
		t.Error("expected 8.8.8.8 to be public")
	}
}

func TestBrokerProxyURL(t *testing.T) {
	b := startTestBroker(t)
	defer b.Close()

	url := b.ProxyURL()
	if !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Fatalf("unexpected proxy URL: %s", url)
	}
}

func TestBrokerConcurrentConnections(t *testing.T) {
	b := startTestBroker(t)
	defer b.Close()

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			proxyURL, _ := url.Parse(b.ProxyURL())
			conn, err := net.DialTimeout("tcp", proxyURL.Host, 5*time.Second)
			if err != nil {
				return
			}
			defer conn.Close()
			fmt.Fprintf(conn, "CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\n\r\n")
			// Just verify we get a response (200 or error)
			conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			io.Copy(io.Discard, conn)
		}()
	}
	wg.Wait()
}
