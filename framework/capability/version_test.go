package capability

import (
	"testing"
)

func TestParseSemver(t *testing.T) {
	tests := []struct {
		input            string
		wantMajor, wantMinor, wantPatch int
	}{
		{"", 0, 0, 0},
		{"0", 0, 0, 0},
		{"1", 1, 0, 0},
		{"1.2", 1, 2, 0},
		{"1.2.3", 1, 2, 3},
		{"v1.2.3", 1, 2, 3},
		{"V1.2.3", 1, 2, 3},
		{"1.2.3-alpha", 1, 2, 3},
		{"1.2.3+build", 1, 2, 3},
		{"10.20.30", 10, 20, 30},
	}
	for _, tc := range tests {
		maj, min, pat := parseSemver(tc.input)
		if maj != tc.wantMajor || min != tc.wantMinor || pat != tc.wantPatch {
			t.Errorf("parseSemver(%q) = (%d,%d,%d), want (%d,%d,%d)",
				tc.input, maj, min, pat, tc.wantMajor, tc.wantMinor, tc.wantPatch)
		}
	}
}

func TestVersionGreater(t *testing.T) {
	tests := []struct {
		v1, v2 string
		want   bool
	}{
		{"1.0.0", "1.0.0", false},
		{"2.0.0", "1.0.0", true},
		{"1.0.0", "2.0.0", false},
		{"1.1.0", "1.0.0", true},
		{"1.0.1", "1.0.0", true},
		{"1.0.0", "1.0.1", false},
		{"v1.0.0", "v1.0.0", false},
		{"v2.0.0", "v1.0.0", true},
		{"1.0.0-alpha", "1.0.0", false},
		{"", "1.0.0", false},
		{"1.0.0", "", true},
	}
	for _, tc := range tests {
		got := versionGreater(tc.v1, tc.v2)
		if got != tc.want {
			t.Errorf("versionGreater(%q, %q) = %v, want %v", tc.v1, tc.v2, got, tc.want)
		}
	}
}

func TestBestVersion(t *testing.T) {
	tests := []struct {
		versions []string
		want    string
	}{
		{nil, ""},
		{[]string{}, ""},
		{[]string{"1.0.0"}, "1.0.0"},
		{[]string{"1.0.0", "2.0.0"}, "2.0.0"},
		{[]string{"2.0.0", "1.0.0", "1.5.0"}, "2.0.0"},
		{[]string{"v1.0.0", "v2.0.0"}, "v2.0.0"},
		{[]string{"1.0.0", "1.0.0-alpha"}, "1.0.0"},
		{[]string{"1.2.3", "1.2.4", "1.3.0"}, "1.3.0"},
	}
	for _, tc := range tests {
		got := bestVersion(tc.versions)
		if got != tc.want {
			t.Errorf("bestVersion(%v) = %q, want %q", tc.versions, got, tc.want)
		}
	}
}
