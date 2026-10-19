package embedfs

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/platform/tools/native"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// The native registry is populated by package init in the composition root's
// import (platform/tools/native). Import it here so the lookup below sees the
// full registry the runtime sees.
var _ = native.AllConstructors

// TestEmbeddedToolManifests_ResolveNativeImplementations guards the shipped
// tool catalog against implementation drift: every embedded go_native tool
// manifest must name an implementation registered in the native registry.
// A manifest whose implementation is unregistered is skipped at every boot
// in strict mode — dead catalog weight the boot warns about forever.
func TestEmbeddedToolManifests_ResolveNativeImplementations(t *testing.T) {
	efs := DefaultFS()
	entries, err := fs.Glob(efs, "workspace/tools/**/*.tool.yaml")
	if err != nil {
		t.Fatalf("glob embedded tool manifests: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no embedded tool manifests found")
	}

	dir := t.TempDir()
	for _, entry := range entries {
		data, err := fs.ReadFile(efs, entry)
		if err != nil {
			t.Fatalf("read %s: %v", entry, err)
		}
		path := filepath.Join(dir, filepath.Base(entry))
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatalf("stage %s: %v", entry, err)
		}
		manifest, err := config.LoadToolManifest(path)
		if err != nil {
			t.Errorf("embedded manifest %s does not load: %v", entry, err)
			continue
		}
		if manifest.Execution.Backend != "go_native" {
			continue
		}
		impl := manifest.Execution.Implementation
		if impl == "" {
			impl = manifest.Name
		}
		if _, ok := ports.LookupNative(impl); !ok {
			t.Errorf("embedded manifest %s: implementation %q is not registered in the native registry (strict-mode boot skip)", entry, impl)
		}
	}
}
