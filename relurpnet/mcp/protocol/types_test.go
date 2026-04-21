package protocol

import "testing"

func TestNormalizeRevision(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "trims surrounding whitespace",
			input: "  2025-06-18\n",
			want:  "2025-06-18",
		},
		{
			name:  "preserves canonical revision",
			input: Revision20251125,
			want:  Revision20251125,
		},
		{
			name:  "collapses whitespace-only input",
			input: "\t  \n",
			want:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeRevision(tc.input); got != tc.want {
				t.Fatalf("NormalizeRevision(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
