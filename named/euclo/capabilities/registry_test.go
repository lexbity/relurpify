package capabilities_test

import (
	"context"
	"sync"
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/capabilities"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

// stubCapability is a minimal EucloCodingCapability for testing.
type stubCapability struct {
	id          string
	annotations map[string]any
}

func (s *stubCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{ID: s.id, Annotations: s.annotations}
}
func (s *stubCapability) Contract() euclotypes.ArtifactContract       { return euclotypes.ArtifactContract{} }
func (s *stubCapability) Eligible(euclotypes.ArtifactState, euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	return euclotypes.EligibilityResult{Eligible: true}
}
func (s *stubCapability) Execute(_ context.Context, _ euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	return euclotypes.ExecutionResult{}
}

func newStub(id string) *stubCapability {
	return &stubCapability{id: id}
}

func newStubWithProfiles(id string, profiles []string) *stubCapability {
	return &stubCapability{
		id:          id,
		annotations: map[string]any{"supported_profiles": profiles},
	}
}

func newStubWithAnyProfiles(id string, profiles []any) *stubCapability {
	return &stubCapability{
		id:          id,
		annotations: map[string]any{"supported_profiles": profiles},
	}
}

// ---------------------------------------------------------------------------
// NewEucloCapabilityRegistry
// ---------------------------------------------------------------------------

func TestNewEucloCapabilityRegistry_IsEmpty(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	if got := reg.ForProfile(""); len(got) != 0 {
		t.Fatalf("expected empty registry, got %d capabilities", len(got))
	}
}

// ---------------------------------------------------------------------------
// Register
// ---------------------------------------------------------------------------

func TestRegister_NilRegistryDoesNotPanic(t *testing.T) {
	var reg *capabilities.EucloCapabilityRegistry
	if err := reg.Register(newStub("x")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRegister_NilCapabilityDoesNotPanic(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	if err := reg.Register(nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRegister_EmptyIDIsIgnored(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	_ = reg.Register(newStub(""))
	_ = reg.Register(newStub("  "))
	if got := reg.ForProfile(""); len(got) != 0 {
		t.Fatalf("expected empty registry after empty-ID registrations, got %d", len(got))
	}
}

func TestRegister_OverwritesExistingID(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	_ = reg.Register(newStub("alpha"))
	_ = reg.Register(newStub("alpha")) // second registration with same ID
	if got := reg.ForProfile(""); len(got) != 1 {
		t.Fatalf("expected 1 capability after double-register, got %d", len(got))
	}
}

func TestRegister_TrimsWhitespaceFromID(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	_ = reg.Register(newStub("  padded  "))
	_, ok := reg.Lookup("padded")
	if !ok {
		t.Fatal("expected Lookup to find capability registered with padded ID")
	}
}

// ---------------------------------------------------------------------------
// Lookup
// ---------------------------------------------------------------------------

func TestLookup_NilRegistryReturnsFalse(t *testing.T) {
	var reg *capabilities.EucloCapabilityRegistry
	_, ok := reg.Lookup("anything")
	if ok {
		t.Fatal("expected Lookup on nil registry to return false")
	}
}

func TestLookup_UnknownIDReturnsFalse(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	_, ok := reg.Lookup("missing")
	if ok {
		t.Fatal("expected Lookup of unknown ID to return false")
	}
}

func TestLookup_EmptyIDReturnsFalse(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	_, ok := reg.Lookup("")
	if ok {
		t.Fatal("expected Lookup of empty ID to return false")
	}
}

func TestLookup_RegisteredIDReturnsCapability(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	cap := newStub("my-cap")
	_ = reg.Register(cap)
	got, ok := reg.Lookup("my-cap")
	if !ok {
		t.Fatal("expected Lookup to find registered capability")
	}
	if got.Descriptor().ID != "my-cap" {
		t.Fatalf("got ID %q, want %q", got.Descriptor().ID, "my-cap")
	}
}

func TestLookup_TrimsWhitespace(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	_ = reg.Register(newStub("x"))
	_, ok := reg.Lookup("  x  ")
	if !ok {
		t.Fatal("expected Lookup with padded ID to succeed")
	}
}

// ---------------------------------------------------------------------------
// ForProfile
// ---------------------------------------------------------------------------

func TestForProfile_NilRegistryReturnsNil(t *testing.T) {
	var reg *capabilities.EucloCapabilityRegistry
	if got := reg.ForProfile("anything"); got != nil {
		t.Fatalf("expected nil from nil registry, got %v", got)
	}
}

func TestForProfile_EmptyProfileReturnsAll(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	_ = reg.Register(newStub("a"))
	_ = reg.Register(newStub("b"))
	_ = reg.Register(newStub("c"))
	got := reg.ForProfile("")
	if len(got) != 3 {
		t.Fatalf("expected 3 capabilities for empty profile, got %d", len(got))
	}
}

func TestForProfile_ReturnsSortedByID(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	_ = reg.Register(newStub("z-cap"))
	_ = reg.Register(newStub("a-cap"))
	_ = reg.Register(newStub("m-cap"))
	got := reg.ForProfile("")
	ids := make([]string, len(got))
	for i, c := range got {
		ids[i] = c.Descriptor().ID
	}
	if ids[0] != "a-cap" || ids[1] != "m-cap" || ids[2] != "z-cap" {
		t.Fatalf("expected sorted order, got %v", ids)
	}
}

func TestForProfile_FiltersToMatchingProfile_StringSlice(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	_ = reg.Register(newStubWithProfiles("match", []string{"profile-a", "profile-b"}))
	_ = reg.Register(newStubWithProfiles("no-match", []string{"profile-c"}))
	_ = reg.Register(newStub("no-annotation")) // nil annotations → included for any profile
	got := reg.ForProfile("profile-a")
	ids := map[string]bool{}
	for _, c := range got {
		ids[c.Descriptor().ID] = true
	}
	if !ids["match"] {
		t.Error("expected 'match' to be included")
	}
	if ids["no-match"] {
		t.Error("expected 'no-match' to be excluded")
	}
	if !ids["no-annotation"] {
		t.Error("expected 'no-annotation' (nil annotations) to be included")
	}
}

func TestForProfile_FiltersToMatchingProfile_AnySlice(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	_ = reg.Register(newStubWithAnyProfiles("match", []any{"profile-x", "profile-y"}))
	_ = reg.Register(newStubWithAnyProfiles("no-match", []any{"profile-z"}))
	got := reg.ForProfile("profile-x")
	ids := map[string]bool{}
	for _, c := range got {
		ids[c.Descriptor().ID] = true
	}
	if !ids["match"] {
		t.Error("expected 'match' to be included via []any annotation")
	}
	if ids["no-match"] {
		t.Error("expected 'no-match' to be excluded")
	}
}

func TestForProfile_UnknownAnnotationTypeIncludesCapability(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	// supported_profiles is an unexpected type — should fall through to "include"
	cap := &stubCapability{
		id:          "unknown-type",
		annotations: map[string]any{"supported_profiles": 42},
	}
	_ = reg.Register(cap)
	got := reg.ForProfile("any-profile")
	if len(got) != 1 || got[0].Descriptor().ID != "unknown-type" {
		t.Fatalf("expected capability with unknown annotation type to be included, got %v", got)
	}
}

func TestForProfile_NilAnnotationValueIncludesCapability(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	cap := &stubCapability{
		id:          "nil-val",
		annotations: map[string]any{"supported_profiles": nil},
	}
	_ = reg.Register(cap)
	got := reg.ForProfile("any-profile")
	if len(got) != 1 {
		t.Fatalf("expected nil annotation value to include capability, got %v", got)
	}
}

func TestForProfile_MissingAnnotationKeyIncludesCapability(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	cap := &stubCapability{
		id:          "no-key",
		annotations: map[string]any{"other_key": "value"},
	}
	_ = reg.Register(cap)
	got := reg.ForProfile("any-profile")
	if len(got) != 1 {
		t.Fatalf("expected missing supported_profiles key to include capability, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// Concurrency
// ---------------------------------------------------------------------------

func TestRegistry_ConcurrentRegisterAndLookup(t *testing.T) {
	reg := capabilities.NewEucloCapabilityRegistry()
	const n = 50
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "cap-" + string(rune('a'+i%26))
			_ = reg.Register(newStub(id))
			reg.Lookup(id)
			reg.ForProfile("")
		}(i)
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// NewDefaultCapabilityRegistry
// ---------------------------------------------------------------------------

func TestNewDefaultCapabilityRegistry_IsNonEmpty(t *testing.T) {
	env := agentenv.AgentEnvironment{Registry: capability.NewRegistry(), Config: &core.Config{Name: "test", Model: "stub"}}
	reg := capabilities.NewDefaultCapabilityRegistry(env)
	got := reg.ForProfile("")
	if len(got) == 0 {
		t.Fatal("expected default registry to contain capabilities")
	}
}

func TestNewDefaultCapabilityRegistry_ContainsExpectedIDs(t *testing.T) {
	env := agentenv.AgentEnvironment{Registry: capability.NewRegistry(), Config: &core.Config{Name: "test", Model: "stub"}}
	reg := capabilities.NewDefaultCapabilityRegistry(env)
	wantIDs := []string{
		"euclo:artifact.diff_summary",
		"euclo:verification.execute",
		"euclo:repair.failed_verification",
		"euclo:test.regression_synthesize",
		"euclo:tdd.red_green_refactor",
		"euclo:review.findings",
	}
	for _, id := range wantIDs {
		if _, ok := reg.Lookup(id); !ok {
			t.Errorf("expected capability %q in default registry", id)
		}
	}
}
