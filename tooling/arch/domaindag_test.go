package arch

import (
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/platform/fs"
)

const (
	Capability_domaindag_test               = "/capability"
	Capabilitya_domaindag_test              = "/capability/a"
	Capabilityc_domaindag_test              = "/capability/c"
	Capabilityclassification_domaindag_test = "/capability/classification"
	Contexta_domaindag_test                 = "/context/a"
	Contextb_domaindag_test                 = "/context/b"
	Executiona_domaindag_test               = "/execution/a"
	Executionb_domaindag_test               = "/execution/b"
	Executionc_domaindag_test               = "/execution/c"
	Frameworktypes_domaindag_test           = "/framework/types"
	Frameworktypestypesgo_domaindag_test    = "/framework/types/types.go"
	Capability_domaindag_test_2             = "capability"
	Context_domaindag_test                  = "context"
	Execution_domaindag_test                = "execution"
	Fmt_domaindag_test                      = "fmt"
	Governance_domaindag_test               = "governance"
	Model_domaindag_test                    = "model"
	Platform_domaindag_test                 = "platform"
	TypesGoFile_domaindag_test              = "types.go"
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
	if !AllowedDomainImport(Execution_domaindag_test, Execution_domaindag_test) {
		t.Error("same domain should be allowed")
	}
}

func TestAllowedDomainImport_downward(t *testing.T) {
	tests := []struct {
		src, dst string
		want     bool
	}{
		{Execution_domaindag_test, Context_domaindag_test, true},
		{Execution_domaindag_test, Capability_domaindag_test_2, true},
		{Execution_domaindag_test, Governance_domaindag_test, true},
		{Context_domaindag_test, Capability_domaindag_test_2, true},
		{Context_domaindag_test, Governance_domaindag_test, true},
		{Capability_domaindag_test_2, Governance_domaindag_test, true},
		{Governance_domaindag_test, Model_domaindag_test, true},
		{"app", "named", true},
		{"named", "cognitionzoo", true},
		{"cognitionzoo", Execution_domaindag_test, true},
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
		{Context_domaindag_test, Execution_domaindag_test, false},
		{Capability_domaindag_test_2, Context_domaindag_test, false},
		{Capability_domaindag_test_2, Execution_domaindag_test, false},
		{Governance_domaindag_test, Capability_domaindag_test_2, false},
		{Governance_domaindag_test, Context_domaindag_test, false},
		{Governance_domaindag_test, Execution_domaindag_test, false},
		{Model_domaindag_test, Capability_domaindag_test_2, false},
		{"telemetry", Governance_domaindag_test, false},
		{"userconfig", Governance_domaindag_test, false},
	}
	for _, tt := range tests {
		got := AllowedDomainImport(tt.src, tt.dst)
		if got != tt.want {
			t.Errorf("AllowedDomainImport(%q, %q) = %v, want %v", tt.src, tt.dst, got, tt.want)
		}
	}
}

func TestAllowedDomainImport_platform(t *testing.T) {
	if !AllowedDomainImport(Platform_domaindag_test, Capability_domaindag_test_2) {
		t.Error("platform should be able to import capability")
	}
	if !AllowedDomainImport(Platform_domaindag_test, Governance_domaindag_test) {
		t.Error("platform should be able to import governance")
	}
	if !AllowedDomainImport(Platform_domaindag_test, Context_domaindag_test) {
		t.Error("platform should be able to import context")
	}
	if !AllowedDomainImport(Platform_domaindag_test, Model_domaindag_test) {
		t.Error("platform should be able to import model")
	}
	if AllowedDomainImport(Platform_domaindag_test, Execution_domaindag_test) {
		t.Error("platform should NOT be able to import execution")
	}
	if AllowedDomainImport(Platform_domaindag_test, "agents") {
		t.Error("platform should NOT be able to import agents")
	}
}

func TestAllowedDomainImport_unrestricted(t *testing.T) {
	if !AllowedDomainImport("testsuite", Execution_domaindag_test) {
		t.Error("testsuite should be unrestricted")
	}
	if !AllowedDomainImport("tooling", Capability_domaindag_test_2) {
		t.Error("tooling should be unrestricted")
	}
	if !AllowedDomainImport("testsuite", "app") {
		t.Error("testsuite should be unrestricted")
	}
}

func TestCheckDomainDirection_noViolations(t *testing.T) {
	pkgs := []GoPackage{
		mkPkg(ModulePath + Executiona_domaindag_test),
		mkPkg(ModulePath + Contextb_domaindag_test),
		mkPkg(ModulePath + Capabilityc_domaindag_test),
	}
	forward := map[string][]string{
		ModulePath + Executiona_domaindag_test:  {ModulePath + Contextb_domaindag_test},
		ModulePath + Contextb_domaindag_test:    {ModulePath + Capabilityc_domaindag_test},
		ModulePath + Capabilityc_domaindag_test: {Fmt_domaindag_test},
	}
	vios := CheckDomainDirection(pkgs, forward, "enforce", nil)
	if len(vios) != 0 {
		t.Errorf("expected no violations, got %d: %v", len(vios), vios)
	}
}

func TestCheckDomainDirection_upwardViolation(t *testing.T) {
	pkgs := []GoPackage{
		mkPkg(ModulePath + Contexta_domaindag_test),
		mkPkg(ModulePath + Executionb_domaindag_test),
	}
	forward := map[string][]string{
		ModulePath + Contexta_domaindag_test:   {ModulePath + Executionb_domaindag_test},
		ModulePath + Executionb_domaindag_test: {Fmt_domaindag_test},
	}
	vios := CheckDomainDirection(pkgs, forward, "enforce", nil)
	if len(vios) == 0 {
		t.Fatal("expected violation for context→execution")
	}
	if !strings.Contains(vios[0], Context_domaindag_test) || !strings.Contains(vios[0], Execution_domaindag_test) {
		t.Errorf("violation should mention domains, got: %s", vios[0])
	}
}

func TestCheckDomainDirection_warnModeException(t *testing.T) {
	pkgs := []GoPackage{
		mkPkg(ModulePath + Contexta_domaindag_test),
		mkPkg(ModulePath + Executionb_domaindag_test),
	}
	forward := map[string][]string{
		ModulePath + Contexta_domaindag_test:   {ModulePath + Executionb_domaindag_test},
		ModulePath + Executionb_domaindag_test: {Fmt_domaindag_test},
	}
	exceptions := map[string]map[string]bool{
		Context_domaindag_test: {Execution_domaindag_test: true},
	}
	vios := CheckDomainDirection(pkgs, forward, "warn", exceptions)
	if len(vios) != 0 {
		t.Errorf("expected exception to suppress violation, got %d: %v", len(vios), vios)
	}
}

func TestDomainCycleReport_mutualPair(t *testing.T) {
	pkgs := []GoPackage{
		mkPkg(ModulePath + Contexta_domaindag_test),
		mkPkg(ModulePath + Executionb_domaindag_test),
	}
	forward := map[string][]string{
		ModulePath + Contexta_domaindag_test:   {ModulePath + Executionb_domaindag_test},
		ModulePath + Executionb_domaindag_test: {ModulePath + Contexta_domaindag_test},
	}
	cycles := DomainCycleReport(pkgs, forward)
	if len(cycles) == 0 {
		t.Fatal("expected cycle between context and execution")
	}
	if !strings.Contains(cycles[0], Context_domaindag_test) || !strings.Contains(cycles[0], Execution_domaindag_test) {
		t.Errorf("cycle should mention both domains, got: %s", cycles[0])
	}
}

func TestDomainCycleReport_noCycle(t *testing.T) {
	pkgs := []GoPackage{
		mkPkg(ModulePath + Executiona_domaindag_test),
		mkPkg(ModulePath + Contextb_domaindag_test),
	}
	forward := map[string][]string{
		ModulePath + Executiona_domaindag_test: {ModulePath + Contextb_domaindag_test},
		ModulePath + Contextb_domaindag_test:   {Fmt_domaindag_test},
	}
	cycles := DomainCycleReport(pkgs, forward)
	if len(cycles) != 0 {
		t.Errorf("expected no cycles, got %d: %v", len(cycles), cycles)
	}
}

func TestCheckNoBucket_threeDomains(t *testing.T) {
	dir := t.TempDir()
	mkDirAll(t, dir+Frameworktypes_domaindag_test)
	writeFile(t, dir+Frameworktypestypesgo_domaindag_test, "package types\ntype A struct { X int }\nconst Version = \"1.0\"\n")

	pkgs := []GoPackage{
		{
			ImportPath: ModulePath + Frameworktypes_domaindag_test,
			Dir:        dir + Frameworktypes_domaindag_test,
			GoFiles:    []string{TypesGoFile_domaindag_test},
		},
		mkPkg(ModulePath + Capabilitya_domaindag_test),
		mkPkg(ModulePath + Contextb_domaindag_test),
		mkPkg(ModulePath + Executionc_domaindag_test),
	}
	reverse := map[string][]string{
		ModulePath + Frameworktypes_domaindag_test: {
			ModulePath + Capabilitya_domaindag_test,
			ModulePath + Contextb_domaindag_test,
			ModulePath + Executionc_domaindag_test,
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
		mkPkg(ModulePath + Frameworktypes_domaindag_test),
		mkPkg(ModulePath + Capabilitya_domaindag_test),
		mkPkg(ModulePath + Contextb_domaindag_test),
	}
	reverse := map[string][]string{
		ModulePath + Frameworktypes_domaindag_test: {
			ModulePath + Capabilitya_domaindag_test,
			ModulePath + Contextb_domaindag_test,
		},
	}
	vios := CheckNoBucket(pkgs, reverse, ".")
	if len(vios) != 0 {
		t.Errorf("expected 0 violations for 2 domains, got %d: %v", len(vios), vios)
	}
}

func TestDomainCycleReport_liveTreeHasLessThan5Cycles(t *testing.T) {
	root := filepath.Join("..", "..")
	pkgs, err := ListPackages(root)
	if err != nil {
		t.Skipf("live tree listing from %s failed: %v", root, err)
	}
	forward, _ := ImportGraph(pkgs)
	cycles := DomainCycleReport(pkgs, forward)
	// Phase 3 retired capability↔model, so 9 remain.
	if len(cycles) > 9 {
		t.Errorf("there is more than 5 cycles, got %d", len(cycles))
	}
}

func TestIsDomainVocabPackage_exempt(t *testing.T) {
	dir := t.TempDir()
	// Create capability/classification as a type-only package
	mkDirAll(t, dir+Capabilityclassification_domaindag_test)
	writeFile(t, dir+"/capability/classification/effects.go", "package classification\ntype EffectKind string\nconst (\n\tEffectKindReadOnly EffectKind = \"read-only\"\n)\ntype ScopeKind string\nconst (\n\tScopeKindBuiltin ScopeKind = \"builtin\"\n)\n")

	pkg := GoPackage{
		ImportPath: ModulePath + Capabilityclassification_domaindag_test,
		Dir:        dir + Capabilityclassification_domaindag_test,
		GoFiles:    []string{"effects.go"},
	}

	if !isDomainVocabPackage(pkg, dir) {
		t.Error("capability/classification type-only should be exempted as domain vocab")
	}
}

func TestIsDomainVocabPackage_nonExempt(t *testing.T) {
	dir := t.TempDir()
	// framework/types is NOT a recognized domain root
	mkDirAll(t, dir+Frameworktypes_domaindag_test)
	writeFile(t, dir+Frameworktypestypesgo_domaindag_test, "package types\ntype A struct{}\n")

	pkg := GoPackage{
		ImportPath: ModulePath + Frameworktypes_domaindag_test,
		Dir:        dir + Frameworktypes_domaindag_test,
		GoFiles:    []string{TypesGoFile_domaindag_test},
	}

	if isDomainVocabPackage(pkg, dir) {
		t.Error("framework/types is not a recognized domain root, should NOT be exempted")
	}
}

func TestIsDomainVocabPackage_exemptAtDomainRoot(t *testing.T) {
	dir := t.TempDir()
	// capability at root — type-only
	mkDirAll(t, dir+Capability_domaindag_test)
	writeFile(t, dir+"/capability/doc.go", "package capability\n// doc only\n")

	pkg := GoPackage{
		ImportPath: ModulePath + Capability_domaindag_test,
		Dir:        dir + Capability_domaindag_test,
		GoFiles:    []string{"doc.go"},
	}

	if !isDomainVocabPackage(pkg, dir) {
		t.Error("capability root type-only should be exempted as domain vocab")
	}
}

func TestCheckNoBucket_exemptsDomainVocab(t *testing.T) {
	dir := t.TempDir()
	// Create capability/classification — type-only vocabulary
	mkDirAll(t, dir+Capabilityclassification_domaindag_test)
	writeFile(t, dir+"/capability/classification/types.go", "package classification\ntype EffectKind string\nconst (\n\tA EffectKind = \"a\"\n)\n")

	pkgs := []GoPackage{
		{
			ImportPath: ModulePath + Capabilityclassification_domaindag_test,
			Dir:        dir + Capabilityclassification_domaindag_test,
			GoFiles:    []string{TypesGoFile_domaindag_test},
		},
		mkPkg(ModulePath + "/governance/risk"),
		mkPkg(ModulePath + "/execution"),
		mkPkg(ModulePath + "/context/knowledge"),
	}
	reverse := map[string][]string{
		ModulePath + Capabilityclassification_domaindag_test: {
			ModulePath + "/governance/risk",
			ModulePath + "/execution",
			ModulePath + "/context/knowledge",
		},
	}

	vios := CheckNoBucket(pkgs, reverse, dir)
	if len(vios) != 0 {
		t.Errorf("expected 0 violations (domain vocab exempted by NFR-7), got %d: %v", len(vios), vios)
	}
}

func TestCheckNoBucket_stillFlagsFrameworkTypes(t *testing.T) {
	dir := t.TempDir()
	mkDirAll(t, dir+Frameworktypes_domaindag_test)
	writeFile(t, dir+Frameworktypestypesgo_domaindag_test, "package types\ntype A struct { X int }\n")

	pkgs := []GoPackage{
		{
			ImportPath: ModulePath + Frameworktypes_domaindag_test,
			Dir:        dir + Frameworktypes_domaindag_test,
			GoFiles:    []string{TypesGoFile_domaindag_test},
		},
		mkPkg(ModulePath + Capabilitya_domaindag_test),
		mkPkg(ModulePath + Contextb_domaindag_test),
		mkPkg(ModulePath + Executionc_domaindag_test),
	}
	reverse := map[string][]string{
		ModulePath + Frameworktypes_domaindag_test: {
			ModulePath + Capabilitya_domaindag_test,
			ModulePath + Contextb_domaindag_test,
			ModulePath + Executionc_domaindag_test,
		},
	}

	vios := CheckNoBucket(pkgs, reverse, dir)
	if len(vios) != 1 {
		t.Fatalf("expected 1 violation for framework/types, got %d: %v", len(vios), vios)
	}
}

func mkDirAll(t *testing.T, path string) {
	t.Helper()
	if err := fs.MkdirAllSecure(path); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := fs.WriteFileSecure(path, []byte(content)); err != nil {
		t.Fatal(err)
	}
}
