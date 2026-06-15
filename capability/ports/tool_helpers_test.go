package ports

import "testing"

func TestNormalizeToolName_Aliases(t *testing.T) {
	cases := map[string]string{
		"File-Read":      "file_read",
		"file read":      "file_read",
		"FILE.READ":      "file_read",
		"file/read":      "file_read",
		"  file__read  ": "file_read",
	}
	for input, want := range cases {
		if got := NormalizeToolName(input); got != want {
			t.Fatalf("NormalizeToolName(%q) = %q, want %q", input, got, want)
		}
	}
}
