package netpolicy

import (
	"context"
	"errors"
	"net"
	"strconv"
	"testing"
	"time"
)

func fakeLookup(records map[string][]string) func(context.Context, string) ([]net.IP, error) {
	return func(_ context.Context, host string) ([]net.IP, error) {
		raw, ok := records[host]
		if !ok {
			return nil, errors.New("no such host")
		}
		out := make([]net.IP, 0, len(raw))
		for _, s := range raw {
			out = append(out, net.ParseIP(s))
		}
		return out, nil
	}
}

func TestResolveHostLiteralSkipsDNS(t *testing.T) {
	resetCache()
	opts := DefaultResolveOptions()
	opts.Lookup = func(context.Context, string) ([]net.IP, error) {
		t.Fatal("Lookup must not be called for an IP literal")
		return nil, nil
	}
	h, err := ResolveHost(context.Background(), "127.0.0.1", opts)
	if err != nil {
		t.Fatalf("ResolveHost(literal) = %v", err)
	}
	if ClassifyResolved(h) != ClassLoopback {
		t.Fatalf("literal class = %s, want loopback", ClassifyResolved(h))
	}
}

func TestResolveHostLookupFailureIsUnresolved(t *testing.T) {
	resetCache()
	calls := 0
	opts := DefaultResolveOptions()
	opts.Lookup = func(context.Context, string) ([]net.IP, error) {
		calls++
		return nil, errors.New("servfail")
	}
	if _, err := ResolveHost(context.Background(), "missing.example", opts); !errors.Is(err, ErrUnresolved) {
		t.Fatalf("ResolveHost error = %v, want ErrUnresolved", err)
	}
	// Negative caching: a second lookup inside the negative TTL must not hit DNS.
	if _, err := ResolveHost(context.Background(), "missing.example", opts); !errors.Is(err, ErrUnresolved) {
		t.Fatalf("second ResolveHost error = %v, want ErrUnresolved", err)
	}
	if calls != 1 {
		t.Fatalf("lookup calls = %d, want 1 (negative cache)", calls)
	}
}

func TestResolveHostEmptyRecordsIsUnresolved(t *testing.T) {
	resetCache()
	opts := DefaultResolveOptions()
	opts.Lookup = fakeLookup(map[string][]string{"empty.example": {}})
	if _, err := ResolveHost(context.Background(), "empty.example", opts); !errors.Is(err, ErrUnresolved) {
		t.Fatalf("ResolveHost error = %v, want ErrUnresolved", err)
	}
}

func TestResolveHostPrivateIsErrPrivateResolved(t *testing.T) {
	resetCache()
	opts := DefaultResolveOptions()
	opts.Lookup = fakeLookup(map[string][]string{"internal.example": {"127.0.0.1"}})
	h, err := ResolveHost(context.Background(), "internal.example", opts)
	if !errors.Is(err, ErrPrivateResolved) {
		t.Fatalf("ResolveHost error = %v, want ErrPrivateResolved", err)
	}
	if len(h.IPs) != 1 {
		t.Fatalf("ResolvedHost must carry the record set on ErrPrivateResolved, got %d IPs", len(h.IPs))
	}
}

func TestResolveHostMixedPublicPrivateIsPrivate(t *testing.T) {
	resetCache()
	opts := DefaultResolveOptions()
	opts.Lookup = fakeLookup(map[string][]string{"mixed.example": {"8.8.8.8", "10.0.0.1"}})
	h, err := ResolveHost(context.Background(), "mixed.example", opts)
	if !errors.Is(err, ErrPrivateResolved) {
		t.Fatalf("ResolveHost error = %v, want ErrPrivateResolved", err)
	}
	if class := ClassifyResolved(h); class != ClassPrivate {
		t.Fatalf("ClassifyResolved = %s, want private", class)
	}
}

func TestResolveHostPublic(t *testing.T) {
	resetCache()
	opts := DefaultResolveOptions()
	opts.Lookup = fakeLookup(map[string][]string{"public.example": {"8.8.8.8"}})
	h, err := ResolveHost(context.Background(), "public.example", opts)
	if err != nil {
		t.Fatalf("ResolveHost error = %v", err)
	}
	if class := ClassifyResolved(h); class != ClassPublic {
		t.Fatalf("ClassifyResolved = %s, want public", class)
	}
}

func TestResolveHostCacheTTL(t *testing.T) {
	resetCache()
	now := time.Unix(1000, 0)
	calls := 0
	opts := DefaultResolveOptions()
	opts.Now = func() time.Time { return now }
	opts.Lookup = func(context.Context, string) ([]net.IP, error) {
		calls++
		return []net.IP{net.ParseIP("8.8.8.8")}, nil
	}
	if _, err := ResolveHost(context.Background(), "ttl.example", opts); err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	now = now.Add(59 * time.Second)
	if _, err := ResolveHost(context.Background(), "ttl.example", opts); err != nil {
		t.Fatalf("cached resolve: %v", err)
	}
	if calls != 1 {
		t.Fatalf("lookup calls at 59s = %d, want 1", calls)
	}
	now = now.Add(2 * time.Second) // 61s: past the 60s positive TTL
	if _, err := ResolveHost(context.Background(), "ttl.example", opts); err != nil {
		t.Fatalf("expired resolve: %v", err)
	}
	if calls != 2 {
		t.Fatalf("lookup calls at 61s = %d, want 2", calls)
	}
}

func TestResolveHostNegativeTTL(t *testing.T) {
	resetCache()
	now := time.Unix(2000, 0)
	calls := 0
	opts := DefaultResolveOptions()
	opts.Now = func() time.Time { return now }
	opts.Lookup = func(context.Context, string) ([]net.IP, error) {
		calls++
		return nil, errors.New("servfail")
	}
	_, _ = ResolveHost(context.Background(), "neg.example", opts)
	now = now.Add(4 * time.Second)
	_, _ = ResolveHost(context.Background(), "neg.example", opts)
	if calls != 1 {
		t.Fatalf("lookup calls at 4s = %d, want 1 (negative cache)", calls)
	}
	now = now.Add(2 * time.Second) // 6s: past the 5s negative TTL
	_, _ = ResolveHost(context.Background(), "neg.example", opts)
	if calls != 2 {
		t.Fatalf("lookup calls at 6s = %d, want 2", calls)
	}
}

func TestResolveHostCacheBound(t *testing.T) {
	resetCache()
	opts := DefaultResolveOptions()
	opts.MaxCacheEntries = 8
	opts.Lookup = func(_ context.Context, host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("8.8.8.8")}, nil
	}
	for i := 0; i < 40; i++ {
		host := "bounded-" + strconv.Itoa(i) + ".example"
		if _, err := ResolveHost(context.Background(), host, opts); err != nil {
			t.Fatalf("resolve %q: %v", host, err)
		}
	}
	cacheMu.Lock()
	size := len(cache)
	cacheMu.Unlock()
	if size > opts.MaxCacheEntries {
		t.Fatalf("cache size = %d, want <= %d", size, opts.MaxCacheEntries)
	}
}

func TestResolveTargetLiteral(t *testing.T) {
	resetCache()
	target, err := ResolveTarget(context.Background(), "10.0.0.1", DefaultResolveOptions())
	if err != nil {
		t.Fatalf("ResolveTarget literal: %v", err)
	}
	if !target.Literal || target.Class != ClassPrivate {
		t.Fatalf("target = %+v, want literal private", target)
	}
}

func TestResolveTargetPrivateNameYieldsClassWithoutError(t *testing.T) {
	resetCache()
	opts := DefaultResolveOptions()
	opts.Lookup = fakeLookup(map[string][]string{"internal.example": {"10.0.0.1"}})
	target, err := ResolveTarget(context.Background(), "internal.example", opts)
	if err != nil {
		t.Fatalf("ResolveTarget private name: %v", err)
	}
	if target.Class != ClassPrivate || target.Literal {
		t.Fatalf("target = %+v, want non-literal private", target)
	}
}

func TestResolveTargetUnresolvedIsError(t *testing.T) {
	resetCache()
	opts := DefaultResolveOptions()
	opts.Lookup = fakeLookup(nil)
	if _, err := ResolveTarget(context.Background(), "nowhere.example", opts); !errors.Is(err, ErrUnresolved) {
		t.Fatalf("ResolveTarget error = %v, want ErrUnresolved", err)
	}
}
