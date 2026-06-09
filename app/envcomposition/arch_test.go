package envcomposition_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoPlatformImportsInAgentenv ensures execution/agentenv does not import
// platform packages for app-level construction. These are app/envcomposition's
// responsibility.
func TestNoPlatformImportsInAgentenv(t *testing.T) {
	agentenvDir := filepath.Join("..", "..", "execution", "agentenv")
	entries, err := os.ReadDir(agentenvDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		if strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(agentenvDir, entry.Name())
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(path, "codeburg.org/lexbit/relurpify/platform/") {
				t.Errorf("%s imports %s (should move to app/envcomposition)", entry.Name(), path)
			}
		}
	}
}

// TestNoEnvcompositionImportsInAgentenv ensures execution/agentenv does not
// import app/envcomposition. Agentenv is below app in the DAG and must not
// depend upward.
func TestNoEnvcompositionImportsInAgentenv(t *testing.T) {
	agentenvDir := filepath.Join("..", "..", "execution", "agentenv")
	entries, err := os.ReadDir(agentenvDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		if strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(agentenvDir, entry.Name())
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(path, "relurpify/app/envcomposition") {
				t.Errorf("%s imports app/envcomposition (directional violation)", entry.Name())
			}
		}
	}
}

// TestNoAgentenvImportsInCognitionzoo keeps generic agent paradigms decoupled
// from the broad execution workspace context. App/named packages adapt broad
// workspace state into cognitionzoo/paradigm.Deps.
func TestNoAgentenvImportsInCognitionzoo(t *testing.T) {
	root := filepath.Join("..", "..", "cognitionzoo")
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if importPath == "codeburg.org/lexbit/relurpify/execution/agentenv" {
				rel, _ := filepath.Rel(filepath.Join("..", ".."), path)
				t.Errorf("%s imports execution/agentenv", rel)
			}
		}
		return nil
	})
}

// TestBuildBuiltinCapabilityBundleIsDeprecated verifies production files no
// longer reference the legacy in-agentenv capability bundle builder.
func TestBuildBuiltinCapabilityBundleIsDeprecated(t *testing.T) {
	root := filepath.Join("..", "..")
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") || strings.Contains(path, "testsuite") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if !strings.Contains(string(data), "BuildBuiltinCapabilityBundle") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		t.Errorf("non-test file %s references legacy BuildBuiltinCapabilityBundle", rel)
		return nil
	})
}

// TestCompositionRootOwnsBuildFunctions verifies broad app composition builders
// are only defined in app/envcomposition.
func TestCompositionRootOwnsBuildFunctions(t *testing.T) {
	buildFuncs := map[string]string{
		"BuildCapabilityRuntime": "app/envcomposition/capabilities.go",
		"BuildKnowledgeRuntime":  "app/envcomposition/knowledge.go",
		"BuildSecurityRuntime":   "app/envcomposition/security.go",
		"BuildModelRuntime":      "app/envcomposition/model.go",
	}
	root := filepath.Join("..", "..")
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for fn, expectedLocation := range buildFuncs {
			if !strings.Contains(string(data), "func "+fn+"(") {
				continue
			}
			rel, _ := filepath.Rel(root, path)
			if rel != expectedLocation {
				t.Errorf("function %s defined in %s, expected %s", fn, rel, expectedLocation)
			}
		}
		return nil
	})
}
