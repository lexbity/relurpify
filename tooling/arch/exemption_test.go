package arch

import "testing"

// archToolingDir is the tree that houses the gates themselves.
const archToolingDir = "tooling/arch"

func TestExemptionCoversPrefixesAndExactFiles(t *testing.T) {
	e := Exemption{
		Prefixes: []string{archToolingDir},
		Files:    []string{"named/euclo/capabilities/families.go"},
	}
	cases := []struct {
		path string
		want bool
	}{
		{"tooling/arch/shimcheck.go", true},
		{"tooling/arch/cmd/shimcheck/main.go", true},
		{"tooling/archx/a.go", false},
		{"tooling/archive/a.go", false},
		{"named/euclo/capabilities/families.go", true},
		{"named/euclo/capabilities/other.go", false},
		{"pkg/a.go", false},
	}
	for _, tc := range cases {
		if got := e.Covers(tc.path); got != tc.want {
			t.Errorf("Covers(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestExemptionCoversNothingByDefault(t *testing.T) {
	if (Exemption{}).Covers("pkg/a.go") {
		t.Error("a zero Exemption must cover nothing")
	}
}
