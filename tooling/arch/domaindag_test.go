package arch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mkPkg(importPath string, goFiles ...string) GoPackage {
	p := GoPackage{ImportPath: importPath}
	if len(goFiles) > 0 {
		p.GoFiles = goFiles
	} else {
		p.GoFiles = []string{"a.go"}
	}
	return p
}

func TestAllowedDomainImport_sameDomain(t *testing.T) {
	if !AllowedDomainImport("execution", "execution") {
		t.Error("same domain should be allowed")
	}
}

func TestAllowedDomainImport_downward(t *testing.T) {
	tests := []struct {
		src, dst string
		want     bool
	}{
		{"execution", "context", true},
		{"execution", "capability", true},
		{"execution", "governance", true},
		{"context", "capability", true},
		{"context", "governance", true},
		{"capability", "governance", true},
		{"governance", "model", true},
		{"app", "named", true},
		{"named", "agents", true},
		{"agents", "execution", true},
	}
	for _, tt := range tests {
		got := AllowedDomainImport(tt.src, tt.dst)
		if got != tt.want {
			t.Errorf("AllowedDomainImport(%q, %q) = %v, want %v", tt.src, tt.dst, got, tt.want)
		}
	}
}

func TestAllowedDomainImport_upward(t *testing.T) {
	tests := []struct {
		src, dst string
		want     bool
	}{
		{"context", "execution", false},
		{"capability", "context", false},
		{"capability", "execution", false},
		{"governance", "capability", false},
		{"governance", "context", false},
		{"governance", "execution", false},
		{"model", "capability", false},
		{"telemetry", "governance", false},
		{"userconfig", "governance", false},
	}
	for _, tt := range tests {
		got := AllowedDomainImport(tt.src, tt.dst)
		if got != tt.want {
			t.Errorf("AllowedDomainImport(%q, %q) = %v, want %v", tt.src, tt.dst, got, tt.want)
		}
	}
}

func TestAllowedDomainImport_platform(t *testing.T) {
	if !AllowedDomainImport("platform", "capability") {
		t.Error("platform should be able to import capability")
	}
	if !AllowedDomainImport("platform", "governance") {
		t.Error("platform should be able to import governance")
	}
	if !AllowedDomainImport("platform", "context") {
		t.Error("platform should be able to import context")
	}
	if !AllowedDomainImport("platform", "model") {
		t.Error("platform should be able to import model")
	}
	if AllowedDomainImport("platform", "execution") {
		t.Error("platform should NOT be able to import execution")
	}
	if AllowedDomainImport("platform", "agents") {
		t.Error("platform should NOT be able to import agents")
	}
}

func TestAllowedDomainImport_unrestricted(t *testing.T) {
	if !AllowedDomainImport("testsuite", "execution") {
		t.Error("testsuite should be unrestricted")
	}
	if !AllowedDomainImport("tooling", "capability") {
		t.Error("tooling should be unrestricted")
	}
	if !AllowedDomainImport("testsuite", "app") {
		t.Error("testsuite should be unrestricted")
	}
}

func TestCheckDomainDirection_noViolations(t *testing.T) {
	pkgs := []GoPackage{
		mkPkg(ModulePath+"/execution/a"),
		mkPkg(ModulePath+"/context/b"),
		mkPkg(ModulePath+"/capability/c"),
	}
	forward := map[string][]string{
		ModulePath + "/execution/a": {ModulePath + "/context/b"},
		ModulePath + "/context/b":   {ModulePath + "/capability/c"},
		ModulePath + "/capability/c": {"fmt"},
	}
	vios := CheckDomainDirection(pkgs, forward, "enforce", nil)
	if len(vios) != 0 {
		t.Errorf("expected no violations, got %d: %v", len(vios), vios)
	}
}

func TestCheckDomainDirection_upwardViolation(t *testing.T) {
	pkgs := []GoPackage{
		mkPkg(ModulePath + "/context/a"),
		mkPkg(ModulePath + "/execution/b"),
	}
	forward := map[string][]string{
		ModulePath + "/context/a":    {ModulePath + "/execution/b"},
		ModulePath + "/execution/b": {"fmt"},
	}
	vios := CheckDomainDirection(pkgs, forward, "enforce", nil)
	if len(vios) == 0 {
		t.Fatal("expected violation for context→execution")
	}
	if !strings.Contains(vios[0], "context") || !strings.Contains(vios[0], "execution") {
		t.Errorf("violation should mention domains, got: %s", vios[0])
	}
}

func TestCheckDomainDirection_warnModeException(t *testing.T) {
	pkgs := []GoPackage{
		mkPkg(ModulePath + "/context/a"),
		mkPkg(ModulePath + "/execution/b"),
	}
	forward := map[string][]string{
		ModulePath + "/context/a":    {ModulePath + "/execution/b"},
		ModulePath + "/execution/b": {"fmt"},
	}
	exceptions := map[string]map[string]bool{
		"context": {"execution": true},
	}
	vios := CheckDomainDirection(pkgs, forward, "warn", exceptions)
	if len(vios) != 0 {
		t.Errorf("expected exception to suppress violation, got %d: %v", len(vios), vios)
	}
}

func TestDomainCycleReport_mutualPair(t *testing.T) {
	pkgs := []GoPackage{
		mkPkg(ModulePath + "/context/a"),
		mkPkg(ModulePath + "/execution/b"),
	}
	forward := map[string][]string{
		ModulePath + "/context/a":    {ModulePath + "/execution/b"},
		ModulePath + "/execution/b": {ModulePath + "/context/a"},
	}
	cycles := DomainCycleReport(pkgs, forward)
	if len(cycles) == 0 {
		t.Fatal("expected cycle between context and execution")
	}
	if !strings.Contains(cycles[0], "context") || !strings.Contains(cycles[0], "execution") {
		t.Errorf("cycle should mention both domains, got: %s", cycles[0])
	}
}

func TestDomainCycleReport_noCycle(t *testing.T) {
	pkgs := []GoPackage{
		mkPkg(ModulePath + "/execution/a"),
		mkPkg(ModulePath + "/context/b"),
	}
	forward := map[string][]string{
		ModulePath + "/execution/a": {ModulePath + "/context/b"},
		ModulePath + "/context/b":   {"fmt"},
	}
	cycles := DomainCycleReport(pkgs, forward)
	if len(cycles) != 0 {
		t.Errorf("expected no cycles, got %d: %v", len(cycles), cycles)
	}
}

func TestCheckNoBucket_threeDomains(t *testing.T) {
	dir := t.TempDir()
	mkDirAll(t, dir+"/framework/types")
	writeFile(t, dir+"/framework/types/types.go", "package types\ntype A struct { X int }\nconst Version = \"1.0\"\n")

	pkgs := []GoPackage{
		{
			ImportPath: ModulePath + "/framework/types",
			Dir:        dir + "/framework/types",
			GoFiles:    []string{"types.go"},
		},
		mkPkg(ModulePath + "/capability/a"),
		mkPkg(ModulePath + "/context/b"),
		mkPkg(ModulePath + "/execution/c"),
	}
	reverse := map[string][]string{
		ModulePath + "/framework/types": {
			ModulePath + "/capability/a",
			ModulePath + "/context/b",
			ModulePath + "/execution/c",
		},
	}
	vios := CheckNoBucket(pkgs, reverse, dir)
	if len(vios) != 1 {
		t.Fatalf("expected 1 bucket violation, got %d: %v", len(vios), vios)
	}
	if !strings.Contains(vios[0], "framework/types") {
		t.Errorf("violation should mention the bucket package, got: %s", vios[0])
	}
}

func TestCheckNoBucket_twoDomains(t *testing.T) {
	pkgs := []GoPackage{
		mkPkg(ModulePath + "/framework/types"),
		mkPkg(ModulePath + "/capability/a"),
		mkPkg(ModulePath + "/context/b"),
	}
	reverse := map[string][]string{
		ModulePath + "/framework/types": {
			ModulePath + "/capability/a",
			ModulePath + "/context/b",
		},
	}
	vios := CheckNoBucket(pkgs, reverse, ".")
	if len(vios) != 0 {
		t.Errorf("expected 0 violations for 2 domains, got %d: %v", len(vios), vios)
	}
}

func TestDomainCycleReport_liveTreeHas10Cycles(t *testing.T) {
	root := filepath.Join("..", "..")
	pkgs, err := ListPackages(root)
	if err != nil {
		t.Skipf("live tree listing from %s failed: %v", root, err)
	}
	forward, _ := ImportGraph(pkgs)
	cycles := DomainCycleReport(pkgs, forward)
	// Phase 3 retired capability↔model, so 9 remain.
	if len(cycles) < 9 {
		t.Errorf("expected at least 9 domain cycles in live tree, got %d", len(cycles))
	}
}

func mkDirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
