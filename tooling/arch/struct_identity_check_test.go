package arch

import (
	"os"
	"path/filepath"
	"testing"
)

// writeConvPkg writes a single-file package under root and returns the GoPackage.
func writeConvPkg(t *testing.T, root, rel, src string) GoPackage {
	t.Helper()
	dir := filepath.Join(root, rel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "conv.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return GoPackage{
		ImportPath: ModulePath + "/" + rel,
		Name:       filepath.Base(rel),
		GoFiles:    []string{"conv.go"},
	}
}

func TestStructIdentityConverters_flagsCrossPackageSameName(t *testing.T) {
	root := t.TempDir()
	pkg := writeConvPkg(t, root, "capability", `package capability

import "codeburg.org/lexbit/relurpify/capability/agentspec"

type RuntimeSafetySpec struct{ Max int }

func toRuntimeSafetySpec(in *agentspec.RuntimeSafetySpec) *RuntimeSafetySpec {
	return &RuntimeSafetySpec{Max: in.Max}
}
`)
	vios := CheckStructIdentityConverters([]GoPackage{pkg}, root, Allowlist{})
	if len(vios) != 1 {
		t.Fatalf("expected 1 converter violation, got %d: %v", len(vios), vios)
	}
}

func TestStructIdentityConverters_ignoresSamePackageAndDistinctNames(t *testing.T) {
	root := t.TempDir()
	pkg := writeConvPkg(t, root, "capability", `package capability

import "codeburg.org/lexbit/relurpify/capability/agentspec"

type Spec struct{ Max int }

// same package both sides — a legit defensive clone, not a fork
func cloneSpec(in *Spec) *Spec { c := *in; return &c }

// different type names — a legit constructor, not a converter
func newSpec(cfg *agentspec.Config) *Spec { return &Spec{Max: cfg.Max} }
`)
	vios := CheckStructIdentityConverters([]GoPackage{pkg}, root, Allowlist{})
	if len(vios) != 0 {
		t.Fatalf("expected no converter violations, got %v", vios)
	}
}

func TestStructIdentityConverters_respectsAllowlist(t *testing.T) {
	root := t.TempDir()
	pkg := writeConvPkg(t, root, "wire", `package wire

import "codeburg.org/lexbit/relurpify/model"

type Event struct{ ID string }

func toEvent(in *model.Event) *Event { return &Event{ID: in.ID} }
`)
	all := CheckStructIdentityConverters([]GoPackage{pkg}, root, Allowlist{})
	if len(all) != 1 {
		t.Fatalf("expected 1 violation before allowlist, got %v", all)
	}
	allow := Allowlist{entries: map[string]map[string]bool{"converter": {all[0]: true}}}
	if got := CheckStructIdentityConverters([]GoPackage{pkg}, root, allow); len(got) != 0 {
		t.Fatalf("expected allowlisted violation suppressed, got %v", got)
	}
}
