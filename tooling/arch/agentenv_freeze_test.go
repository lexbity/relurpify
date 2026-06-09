package arch

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNoNewAgentenvImporters(t *testing.T) {
	root := filepath.Join("..", "..")
	pkgs, err := ListPackages(root)
	if err != nil {
		t.Skipf("live tree listing from %s failed: %v", root, err)
	}
	forward, _ := ImportGraph(pkgs)
	violations := CheckNoNewAgentenvImporters(pkgs, forward)
	if len(violations) > 0 {
		for _, v := range violations {
			t.Error(v)
		}
	}
}

func TestAllowedAgentenvImportersReport(t *testing.T) {
	root := filepath.Join("..", "..")
	pkgs, err := ListPackages(root)
	if err != nil {
		t.Skipf("live tree listing from %s failed: %v", root, err)
	}
	forward, _ := ImportGraph(pkgs)
	lines := AllowedAgentenvImportersReport(forward)

	t.Log("AgentEnv importer migration status:")
	for _, line := range lines {
		t.Log(line)
	}

	// Migrated: AllowedAgentenvImporters is empty.
	// No production packages import execution/agentenv any longer.
}

func TestNoAgentenvImportsInCognitionzoo(t *testing.T) {
	root := filepath.Join("..", "..")
	pkgs, err := ListPackages(root)
	if err != nil {
		t.Skipf("live tree listing from %s failed: %v", root, err)
	}
	forward, _ := ImportGraph(pkgs)

	for _, pkg := range pkgs {
		if !strings.HasPrefix(pkg.ImportPath, ModulePath+"/cognitionzoo") {
			continue
		}
		imports := forward[pkg.ImportPath]
		for _, imp := range imports {
			if imp == agentenvImportPath || strings.HasPrefix(imp, agentenvImportPath+"/") {
				rel := TrimModulePrefix(pkg.ImportPath)
				t.Errorf("%s imports execution/agentenv", rel)
			}
		}
	}
}

func TestNoAgentenvImportsInModel(t *testing.T) {
	root := filepath.Join("..", "..")
	pkgs, err := ListPackages(root)
	if err != nil {
		t.Skipf("live tree listing from %s failed: %v", root, err)
	}
	forward, _ := ImportGraph(pkgs)

	modelDomain := "model"
	for _, pkg := range pkgs {
		if PackageDomain(pkg.ImportPath) != modelDomain {
			continue
		}
		imports := forward[pkg.ImportPath]
		for _, imp := range imports {
			if imp == agentenvImportPath || strings.HasPrefix(imp, agentenvImportPath+"/") {
				rel := TrimModulePrefix(pkg.ImportPath)
				t.Errorf("%s imports execution/agentenv", rel)
			}
		}
	}
}
