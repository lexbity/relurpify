package arch

import (
	"os"
	"path/filepath"
	"testing"
)

// principalTestPkg writes src to a temp dir and returns a GoPackage whose
// Dir/GoFiles point at it, so the call-based check parses real source.
func principalTestPkg(t *testing.T, importPath, src string) GoPackage {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	return GoPackage{
		ImportPath: importPath,
		Dir:        dir,
		GoFiles:    []string{"file.go"},
	}
}

const principalWriteSrc = `package p

import gp "codeburg.org/lexbit/relurpify/governance/ports"

func use(ctx context.Context, pr gp.Principal) context.Context {
	return gp.ContextWithPrincipal(ctx, pr)
}
`

const principalReadSrc = `package p

import gp "codeburg.org/lexbit/relurpify/governance/ports"

func use(ctx context.Context) gp.Principal {
	return gp.PrincipalFromContext(ctx)
}
`

// Read-side importers of governance/ports must not be flagged — this is the case
// the old import-based heuristic over-reported.
func TestCheckPrincipalContextWrite_noViolation_readSide(t *testing.T) {
	pkgs := []GoPackage{
		principalTestPkg(t, ModulePath+"/capability/descriptor", principalReadSrc),
	}
	if vios := CheckPrincipalContextWrite(pkgs); len(vios) != 0 {
		t.Errorf("read-side importer must not be flagged, got %d: %v", len(vios), vios)
	}
}

// A package that actually calls ContextWithPrincipal is a real violation.
func TestCheckPrincipalContextWrite_violation_writeSide(t *testing.T) {
	pkgs := []GoPackage{
		principalTestPkg(t, ModulePath+"/capability/descriptor", principalWriteSrc),
	}
	if vios := CheckPrincipalContextWrite(pkgs); len(vios) != 1 {
		t.Fatalf("write-side caller must be flagged, got %d: %v", len(vios), vios)
	}
}

// execution/agentlifecycle is the sole authorized writer.
func TestCheckPrincipalContextWrite_executionAgentlifecycleAllowed(t *testing.T) {
	pkgs := []GoPackage{
		principalTestPkg(t, ModulePath+"/execution/agentlifecycle", principalWriteSrc),
	}
	if vios := CheckPrincipalContextWrite(pkgs); len(vios) != 0 {
		t.Errorf("execution/agentlifecycle is the authorized writer, got %d: %v", len(vios), vios)
	}
}

// The live tree must satisfy NFR-8: only execution/agentlifecycle writes the key.
func TestCheckPrincipalContextWrite_liveTreeClean(t *testing.T) {
	root := filepath.Join("..", "..")
	pkgs, err := ListPackages(root)
	if err != nil {
		t.Skipf("live tree listing failed: %v", err)
	}
	if vios := CheckPrincipalContextWrite(pkgs); len(vios) != 0 {
		t.Errorf("only execution/agentlifecycle may write the principal key; got %d: %v", len(vios), vios)
	}
}
