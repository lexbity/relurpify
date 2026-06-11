package promptprovider

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "update golden test output files")

// goldenPath returns the path to a golden file in the testdata directory.
func goldenPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("testdata", name+".golden")
}

// assertGolden compares the actual output against a golden file. If the
// -update flag is set, it writes the actual output as the new golden content.
func assertGolden(t *testing.T, name, actual string) {
	t.Helper()
	path := goldenPath(t, name)

	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil { // public: golden test dir
			t.Fatalf("mkdir golden dir: %v", err)
		}
		clean := strings.TrimSpace(actual) + "\n"
		if err := os.WriteFile(path, []byte(clean), 0o600); err != nil { // public: golden test file
			t.Fatalf("write golden file: %v", err)
		}
		return
	}

	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read golden file %s: %v (run with -update to create)", path, err)
	}
	want := strings.TrimSpace(string(data))
	got := strings.TrimSpace(actual)
	if got != want {
		t.Errorf("golden mismatch for %s:\ngot:\n%s\n\nwant:\n%s", name, got, want)
	}
}
