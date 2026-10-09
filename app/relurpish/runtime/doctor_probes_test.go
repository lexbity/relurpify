package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/userconfig/config"
	"codeburg.org/lexbit/relurpify/userconfig/config/model"
)

// hangingProviderServer accepts connections and never answers.
func hangingProviderServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestProviderProbeTimeout: a hanging provider is recorded as "timeout"
// within the per-probe deadline plus slack — never the HTTP client's own
// long timeout (AC-7 doctor side).
func TestProviderProbeTimeout(t *testing.T) {
	srv := hangingProviderServer(t)
	cfg := Config{InferenceProvider: "hanging"}
	secrets := config.Secrets{}
	defs := []*model.ResolvedProvider{{
		Name:     "hanging",
		Kind:     "openai_compatible",
		Endpoint: srv.URL,
	}}

	start := time.Now()
	results := probeProviderCatalog(context.Background(), defs, cfg, secrets)
	elapsed := time.Since(start)

	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].State != "timeout" {
		t.Fatalf("state = %q, want timeout", results[0].State)
	}
	// Per-probe 2 s + construction slack; far below the old 3-minute client
	// timeout.
	if elapsed > 4*time.Second {
		t.Fatalf("probe took %v, want <= ~4s", elapsed)
	}
}

// TestProviderCatalogDeadline: eight hanging providers finish within the
// overall catalog deadline, recorded as timeouts — bounded concurrency, no
// serial accumulation (FR-39).
func TestProviderCatalogDeadline(t *testing.T) {
	srv := hangingProviderServer(t)
	cfg := Config{InferenceProvider: "hanging-0"}
	var defs []*model.ResolvedProvider
	for i := 0; i < 8; i++ {
		defs = append(defs, &model.ResolvedProvider{
			Name:     "hanging-" + itoa(i),
			Kind:     "openai_compatible",
			Endpoint: srv.URL,
		})
	}

	start := time.Now()
	results := probeProviderCatalog(context.Background(), defs, cfg, secretsFor(t))
	elapsed := time.Since(start)

	if len(results) != 8 {
		t.Fatalf("results = %d, want 8", len(results))
	}
	for i, r := range results {
		if r.State != "timeout" {
			t.Fatalf("provider %d state = %q, want timeout", i, r.State)
		}
	}
	// 8 providers at concurrency 4 with a 2 s per-probe bound: ~4 s wall
	// worst case; the pre-slice serial path would need 8 × client-timeout.
	if elapsed > providerCatalogDeadline+2*time.Second {
		t.Fatalf("catalog took %v, want <= %v + slack", elapsed, providerCatalogDeadline)
	}
}

// TestProviderProbeOrderPreserved: results keep config declaration order
// regardless of probe completion order.
func TestProviderProbeOrderPreserved(t *testing.T) {
	var probes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probes.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"m1"}]}`))
	}))
	t.Cleanup(srv.Close)

	cfg := Config{InferenceProvider: "fast-0"}
	var defs []*model.ResolvedProvider
	for i := 0; i < 5; i++ {
		defs = append(defs, &model.ResolvedProvider{
			Name:     "fast-" + itoa(i),
			Kind:     "openai_compatible",
			Endpoint: srv.URL,
		})
	}
	results := probeProviderCatalog(context.Background(), defs, cfg, secretsFor(t))
	if probes.Load() < 5 {
		t.Fatalf("only %d providers probed", probes.Load())
	}
	for i, r := range results {
		if r.Name != "fast-"+itoa(i) {
			t.Fatalf("position %d holds %q, want fast-%d", i, r.Name, i)
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	digits := []byte{}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}

func secretsFor(t *testing.T) config.Secrets {
	t.Helper()
	return config.Secrets{}
}
