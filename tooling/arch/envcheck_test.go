package arch

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/tooling/arch/gatescan"
)

// parse builds a File from source, failing the test on a syntax error.
func parse(t *testing.T, path, src string) gatescan.File {
	t.Helper()
	f, err := gatescan.ParseSource(path, src)
	if err != nil {
		t.Fatalf("ParseSource(%s): %v", path, err)
	}
	return f
}

func TestCheckEnvAccessFlagsEveryReadForm(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "plain call",
			src: `package a

import "os"

func f() { _ = os.Getenv("X") }
`,
			want: "os.Getenv",
		},
		{
			name: "lookup env",
			src: `package a

import "os"

func f() { _, _ = os.LookupEnv("X") }
`,
			want: "os.LookupEnv",
		},
		{
			name: "uncalled reference",
			src: `package a

import "os"

var environ = os.Environ
`,
			want: "os.Environ",
		},
		{
			name: "aliased import",
			src: `package a

import o "os"

func f() { _ = o.Getenv("X") }
`,
			want: "os.Getenv",
		},
		{
			name: "dot import",
			src: `package a

import . "os"

func f() { _ = Getenv("X") }
`,
			want: "dot-imported Getenv",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := []gatescan.File{parse(t, "app/x/a.go", tc.src)}
			got := CheckEnvAccess(files, "userconfig")
			if len(got) != 1 {
				t.Fatalf("want 1 violation, got %v", got)
			}
			if !strings.Contains(got[0], tc.want) {
				t.Errorf("violation %q should name %q", got[0], tc.want)
			}
			if !strings.HasPrefix(got[0], "env: app/x/a.go:") {
				t.Errorf("violation %q should carry file:line", got[0])
			}
		})
	}
}

func TestCheckEnvAccessIgnoresLookAlikes(t *testing.T) {
	files := []gatescan.File{parse(t, "app/x/a.go", `package a

import "os"

type cfg struct{}

// Getenv is a method on cfg, not the os package function.
func (cfg) Getenv(string) string { return "" }

func f(c cfg) {
	_ = c.Getenv("X")
	_ = os.Getpid()
	_ = "os.Getenv"
}
`)}

	if got := CheckEnvAccess(files, "userconfig"); len(got) != 0 {
		t.Fatalf("want no violations, got %v", got)
	}
}

func TestCheckEnvAccessAllowsPermittedRoots(t *testing.T) {
	src := `package config

import "os"

func snapshot() string { return os.Getenv("HOME") }
`
	files := []gatescan.File{
		parse(t, "userconfig/config/env.go", src),
		parse(t, "userconfig/config/model/provider.go", src),
	}
	if got := CheckEnvAccess(files, "userconfig"); len(got) != 0 {
		t.Fatalf("want userconfig permitted, got %v", got)
	}

	outside := []gatescan.File{parse(t, "app/relurpish/a.go", src)}
	if got := CheckEnvAccess(outside, "userconfig"); len(got) != 1 {
		t.Fatalf("want the same read flagged outside userconfig, got %v", got)
	}
}

func TestCheckEnvAccessSortsViolations(t *testing.T) {
	files := []gatescan.File{
		parse(t, "b.go", "package a\n\nimport \"os\"\n\nvar _ = os.Environ\n"),
		parse(t, "a.go", "package a\n\nimport \"os\"\n\nfunc f() { _ = os.Getenv(\"X\") }\n"),
	}
	got := CheckEnvAccess(files, "userconfig")
	if len(got) != 2 {
		t.Fatalf("want 2 violations, got %v", got)
	}
	if got[0] >= got[1] {
		t.Errorf("violations should be sorted for stable output, got %v", got)
	}
}
