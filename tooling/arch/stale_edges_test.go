package arch

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestNoStaleDirectionEdges verifies that the domain-direction exceptions
// retired in earlier slices that would be DAG-illegal if reintroduced remain
// absent. P7 (capability→platform) and P8 (governance→platform) are DAG-legal
// directions and are not checked here.
func TestNoStaleDirectionEdges(t *testing.T) {
	root := filepath.Join("..", "..")
	pkgs, err := ListPackages(root)
	if err != nil {
		t.Skipf("live tree listing from %s failed: %v", root, err)
	}

	stalePairs := []struct {
		src string
		dst string
		msg string
	}{
		{"capability", "context", "P10 retired: capability→context"},
		{"capability", "execution", "P12 retired: capability→execution"},
	}

	var failures []string
	for _, pkg := range pkgs {
		if pkg.OnlyTestGoFiles {
			continue
		}
		srcDomain := PackageDomain(pkg.ImportPath)
		if srcDomain == "" {
			continue
		}
		imports := append([]string{}, pkg.Imports...)
		for _, imp := range imports {
			if IsStandardLib(imp) {
				continue
			}
			dstDomain := PackageDomain(imp)
			if dstDomain == "" || dstDomain == srcDomain {
				continue
			}
			for _, pair := range stalePairs {
				if srcDomain == pair.src && dstDomain == pair.dst {
					failures = append(failures,
						"  "+pair.msg+": "+pkg.ImportPath+" imports "+imp)
				}
			}
		}
	}

	if len(failures) > 0 {
		t.Errorf("found %d stale edge(s) that should have been retired:\n%s",
			len(failures), strings.Join(failures, "\n"))
	}
}
