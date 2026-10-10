package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

// TestCheckCanonicalRecipes_MissingOnEmptyWorkspace: a workspace without the
// canonical recipe set fails closed with the actionable re-init message and
// lists the missing IDs (FR-25).
func TestCheckCanonicalRecipes_MissingOnEmptyWorkspace(t *testing.T) {
	result := checkCanonicalRecipes(t.TempDir())
	if result.ready {
		t.Fatal("empty workspace must not report recipes ready")
	}
	if !strings.Contains(result.errText, "relurpish doctor --fix") {
		t.Fatalf("error text missing re-init hint: %q", result.errText)
	}
	if len(result.missing) != len(canonicalRecipeIDs) {
		t.Fatalf("missing = %d, want %d", len(result.missing), len(canonicalRecipeIDs))
	}
}

// TestCheckCanonicalRecipes_MaterializedWorkspace: a workspace whose
// relurpify_cfg/euclo carries the canonical set reports ready with all IDs
// found (AC-8 doctor side).
func TestCheckCanonicalRecipes_MaterializedWorkspace(t *testing.T) {
	workspace := t.TempDir()
	dst := filepath.Join(workspace, "relurpify_cfg", "euclo")
	if err := os.MkdirAll(dst, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"default", "code_review", "investigation", "debug_tdd_repair", "dep_upgrade", "test_synthesis", "extract_func"} {
		src := filepath.Join("..", "..", "..", "userconfig", "templates", "embedfs", "workspace", "euclo", name+".erpe")
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, name+".erpe"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	result := checkCanonicalRecipes(workspace)
	if !result.ready {
		t.Fatalf("materialized workspace not ready: %s", result.errText)
	}
	if len(result.found) != len(canonicalRecipeIDs) {
		t.Fatalf("found = %d, want %d", len(result.found), len(canonicalRecipeIDs))
	}
}

// TestCheckCanonicalRecipes_BrokenAuthoredRecipe: a recipe dir that exists but
// fails contract validation is its own diagnostic — the message says the
// authored recipe must be fixed, not that starters should be materialized.
func TestCheckCanonicalRecipes_BrokenAuthoredRecipe(t *testing.T) {
	workspace := t.TempDir()
	dst := filepath.Join(workspace, "relurpify_cfg", "euclo")
	if err := os.MkdirAll(dst, 0o700); err != nil {
		t.Fatal(err)
	}
	broken := "thoughtrecipe broken_fixture\n\"missing trigger block\"\nrun ghost:\n  from nothing.at.all\n"
	if err := os.WriteFile(filepath.Join(dst, "broken_fixture.erpe"), []byte(broken), 0o600); err != nil {
		t.Fatal(err)
	}
	result := checkCanonicalRecipes(workspace)
	if result.ready {
		t.Fatal("workspace with a contract-invalid recipe must not report ready")
	}
	if strings.Contains(result.errText, "to materialize starter recipes") {
		t.Fatalf("parse failure must not offer the starter-materialization hint: %q", result.errText)
	}
	if !strings.Contains(result.errText, "contract validation failed") {
		t.Fatalf("error text missing contract-validation diagnostic: %q", result.errText)
	}
}
