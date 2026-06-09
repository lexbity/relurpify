package arch

import (
	"testing"
)

func TestPackageDomain(t *testing.T) {
	tests := []struct {
		importPath string
		want       string
	}{
		{"codeburg.org/lexbit/relurpify", ""},
		{"codeburg.org/lexbit/relurpify/capability", "capability"},
		{"codeburg.org/lexbit/relurpify/app/relurpish", "app"},
		{"codeburg.org/lexbit/relurpify/governance/identity", "governance"},
		{"codeburg.org/lexbit/relurpify/context/persistence", "context"},
		{"codeburg.org/lexbit/relurpify/named/euclo", "named"},
		{"codeburg.org/lexbit/relurpify/testsuite", "testsuite"},
		{"codeburg.org/lexbit/relurpify/cognitionzoo/react", "cognitionzoo"},
		{"codeburg.org/lexbit/relurpify/ayenitd", "ayenitd"},
		{"codeburg.org/lexbit/relurpify/tooling/arch", "tooling"},
		{"some/other/module/foo", ""},
		{"codeburg.org/lexbit/relurpify/platform/llm/ollama", "platform"},
	}
	for _, tt := range tests {
		got := PackageDomain(tt.importPath)
		if got != tt.want {
			t.Errorf("PackageDomain(%q) = %q, want %q", tt.importPath, got, tt.want)
		}
	}
}

func TestTrimModulePrefix(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"codeburg.org/lexbit/relurpify", ""},
		{"codeburg.org/lexbit/relurpify/capability", "capability"},
	}
	for _, tt := range tests {
		got := TrimModulePrefix(tt.path)
		if got != tt.want {
			t.Errorf("TrimModulePrefix(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestIsStandardLib(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"fmt", true},
		{"os", true},
		{"codeburg.org/lexbit/relurpify/capability", false},
		{"github.com/stretchr/testify/assert", true},
	}
	for _, tt := range tests {
		got := IsStandardLib(tt.path)
		if got != tt.want {
			t.Errorf("IsStandardLib(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
