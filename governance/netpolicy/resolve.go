package netpolicy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// ErrUnresolved reports that a hostname did not resolve (lookup failure or an
// empty record set). It is fail-closed: a caller that cannot prove a host
// public must not dial it.
var ErrUnresolved = errors.New("host did not resolve")

// ErrPrivateResolved reports that a hostname resolved to at least one
// non-public address. The accompanying ResolvedHost carries the record set so
// the caller can classify the most restrictive answer.
var ErrPrivateResolved = errors.New("host resolved to a non-public address")

// ResolvedHost is the value produced by DNS resolution.
type ResolvedHost struct {
	Token      string
	IPs        []net.IP
	ResolvedAt time.Time
}

// ResolveOptions bounds and caches DNS resolution.
type ResolveOptions struct {
	Timeout         time.Duration
	PositiveTTL     time.Duration
	NegativeTTL     time.Duration
	MaxCacheEntries int

	// Resolver is the DNS resolver. nil means net.DefaultResolver.
	Resolver *net.Resolver

	// Lookup overrides DNS for tests. When non-nil it is used instead of
	// Resolver and must be safe for concurrent use.
	Lookup func(ctx context.Context, host string) ([]net.IP, error)

	// Now overrides the clock for tests. nil means time.Now.
	Now func() time.Time
}

// DefaultResolveOptions returns the canonical resolution budgets: 2s timeout,
// 60s positive / 5s negative TTL, 4096 cache entries.
func DefaultResolveOptions() ResolveOptions {
	return ResolveOptions{
		Timeout:         2 * time.Second,
		PositiveTTL:     60 * time.Second,
		NegativeTTL:     5 * time.Second,
		MaxCacheEntries: 4096,
	}
}

func (o ResolveOptions) norm() ResolveOptions {
	d := DefaultResolveOptions()
	if o.Timeout <= 0 {
		o.Timeout = d.Timeout
	}
	if o.PositiveTTL <= 0 {
		o.PositiveTTL = d.PositiveTTL
	}
	if o.NegativeTTL <= 0 {
		o.NegativeTTL = d.NegativeTTL
	}
	if o.MaxCacheEntries <= 0 {
		o.MaxCacheEntries = d.MaxCacheEntries
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return o
}

type cacheEntry struct {
	ips       []net.IP
	expiresAt time.Time
	negative  bool
}

var (
	cacheMu sync.Mutex
	cache   = make(map[string]cacheEntry)
)

// resetCache clears the process-wide resolution cache. Test-only.
func resetCache() {
	cacheMu.Lock()
	cache = make(map[string]cacheEntry)
	cacheMu.Unlock()
}

// ResolveHost resolves a host token, returning ErrUnresolved on lookup failure
// or zero records and ErrPrivateResolved when any returned address is
// non-public. IP literals never touch DNS and never hit the cache.
//
// The cache stores raw IP sets only; classification is recomputed on every
// read, so a cached entry can never launder a private answer as public.
func ResolveHost(ctx context.Context, token string, opts ResolveOptions) (ResolvedHost, error) {
	opts = opts.norm()
	token = strings.TrimSpace(token)
	if token == "" {
		return ResolvedHost{Token: token}, ErrUnresolved
	}
	now := opts.Now()
	if ip, ok := ParseHostToken(token); ok {
		return ResolvedHost{Token: token, IPs: []net.IP{ip}, ResolvedAt: now}, nil
	}
	key := strings.ToLower(token)

	cacheMu.Lock()
	if entry, ok := cache[key]; ok && now.Before(entry.expiresAt) {
		ips := copyIPs(entry.ips)
		negative := entry.negative
		cacheMu.Unlock()
		if negative {
			return ResolvedHost{Token: token, ResolvedAt: now}, ErrUnresolved
		}
		h := ResolvedHost{Token: token, IPs: ips, ResolvedAt: now}
		if ClassifyResolved(h) != ClassPublic {
			return h, ErrPrivateResolved
		}
		return h, nil
	}
	cacheMu.Unlock()

	lookupCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	ips, err := lookupHost(lookupCtx, token, opts)
	if err != nil || len(ips) == 0 {
		storeCache(key, cacheEntry{expiresAt: now.Add(opts.NegativeTTL), negative: true}, opts.MaxCacheEntries)
		if err != nil {
			return ResolvedHost{Token: token}, fmt.Errorf("%w: %q: %v", ErrUnresolved, token, err)
		}
		return ResolvedHost{Token: token}, fmt.Errorf("%w: %q: no records", ErrUnresolved, token)
	}
	h := ResolvedHost{Token: token, IPs: ips, ResolvedAt: now}
	storeCache(key, cacheEntry{ips: copyIPs(ips), expiresAt: now.Add(opts.PositiveTTL)}, opts.MaxCacheEntries)
	if ClassifyResolved(h) != ClassPublic {
		return h, ErrPrivateResolved
	}
	return h, nil
}

// ClassifyResolved returns the most restrictive class across all resolved
// addresses. An empty record set classifies as public (callers only reach this
// with a successful, non-empty resolution).
func ClassifyResolved(h ResolvedHost) HostClass {
	best := ClassPublic
	bestSeverity := classSeverity[ClassPublic]
	for _, ip := range h.IPs {
		class := ClassifyIP(ip)
		if severity := classSeverity[class]; severity > bestSeverity {
			best = class
			bestSeverity = severity
		}
	}
	return best
}

// ResolveTarget returns a policy-layer Target for a host token. IP literals
// are classified without I/O. A name that resolves to only public addresses
// yields a public Target; a name that resolves to any non-public address
// yields a non-public Target (classification, not an error — the policy layer
// needs the class to report the denial). A name that does not resolve returns
// ErrUnresolved, which callers MUST treat as a denial.
func ResolveTarget(ctx context.Context, token string, opts ResolveOptions) (Target, error) {
	token = strings.TrimSpace(token)
	if ip, ok := ParseHostToken(token); ok {
		return Target{Token: token, Class: ClassifyIP(ip), Literal: true}, nil
	}
	h, err := ResolveHost(ctx, token, opts)
	if err != nil {
		if errors.Is(err, ErrPrivateResolved) {
			return Target{Token: token, Class: ClassifyResolved(h)}, nil
		}
		return Target{}, err
	}
	return Target{Token: token, Class: ClassifyResolved(h)}, nil
}

func lookupHost(ctx context.Context, host string, opts ResolveOptions) ([]net.IP, error) {
	if opts.Lookup != nil {
		return opts.Lookup(ctx, host)
	}
	resolver := opts.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		ips = append(ips, addr.IP)
	}
	return ips, nil
}

func storeCache(key string, entry cacheEntry, maxEntries int) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if len(cache) >= maxEntries {
		evictOldestLocked(len(cache) / 8)
	}
	cache[key] = entry
}

// evictOldestLocked removes up to n entries with the earliest expiry.
func evictOldestLocked(n int) {
	if n < 1 {
		n = 1
	}
	for i := 0; i < n && len(cache) > 0; i++ {
		var oldestKey string
		var oldestExpiry time.Time
		first := true
		for k, e := range cache {
			if first || e.expiresAt.Before(oldestExpiry) {
				oldestKey = k
				oldestExpiry = e.expiresAt
				first = false
			}
		}
		delete(cache, oldestKey)
	}
}

func copyIPs(ips []net.IP) []net.IP {
	out := make([]net.IP, len(ips))
	for i, ip := range ips {
		out[i] = append(net.IP(nil), ip...)
	}
	return out
}
