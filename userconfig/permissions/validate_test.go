package permissions

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateAcceptsEmptyAndNilSets(t *testing.T) {
	require.NoError(t, ValidatePermissionSet(nil), "nil set is vacuously valid")
	require.NoError(t, (&PermissionSet{}).Validate())
}

func TestValidateAcceptsWellFormedSet(t *testing.T) {
	p := &PermissionSet{
		FileSystem: []FileSystemPermission{
			{Action: FileSystemRead, Path: "src/*.go"},
			{Action: FileSystemWrite, Path: "build/out.txt"},
		},
		Executables: []ExecutablePermission{
			{Binary: "rg", Args: []string{"-n"}, Env: []string{"LANG=C"}},
		},
		Network: []NetworkPermission{
			{Direction: "ingress", Protocol: "tcp", Port: 8080},
			{Direction: "egress", Protocol: "https", Host: "example.com"},
		},
	}
	require.NoError(t, p.Validate(), "well-formed set must validate")
}

func TestValidateRejectsFilesystemPermissionWithoutPath(t *testing.T) {
	p := &PermissionSet{FileSystem: []FileSystemPermission{{Action: FileSystemWrite}}}
	err := p.Validate()
	require.ErrorContains(t, err, "missing path")
	require.ErrorContains(t, err, string(FileSystemWrite))
}

func TestValidateRejectsEscapingFilesystemPath(t *testing.T) {
	for _, path := range []string{"..", "../secrets", "a/../..", "src/../../etc"} {
		p := &PermissionSet{FileSystem: []FileSystemPermission{
			{Action: FileSystemRead, Path: path},
		}}
		err := p.Validate()
		require.Error(t, err, "path %q must be rejected", path)
		require.ErrorContains(t, err, "invalid filesystem path")
	}
}

func TestValidateRejectsExecutableWithoutBinary(t *testing.T) {
	p := &PermissionSet{Executables: []ExecutablePermission{{Args: []string{"x"}}}}
	require.ErrorContains(t, p.Validate(), "executable permission missing binary")
}

func TestValidateRejectsExecutableReferencedByPath(t *testing.T) {
	p := &PermissionSet{Executables: []ExecutablePermission{{Binary: "/usr/bin/rg"}}}
	require.ErrorContains(t, p.Validate(), "must be referenced by name")
}

func TestValidateRejectsNetworkPermissionWithoutDirection(t *testing.T) {
	p := &PermissionSet{Network: []NetworkPermission{{Protocol: "tcp"}}}
	require.ErrorContains(t, p.Validate(), "network permission missing direction")
}

func TestValidateRejectsNetworkPermissionWithoutProtocol(t *testing.T) {
	p := &PermissionSet{Network: []NetworkPermission{{Direction: "egress", Host: "example.com"}}}
	require.ErrorContains(t, p.Validate(), "missing protocol")
}

func TestValidateRejectsEgressWithoutHost(t *testing.T) {
	p := &PermissionSet{Network: []NetworkPermission{{Direction: "egress", Protocol: "https"}}}
	require.ErrorContains(t, p.Validate(), "egress network permission must declare host")
}

func TestValidateStopsAtFirstOffendingEntry(t *testing.T) {
	p := &PermissionSet{
		FileSystem: []FileSystemPermission{
			{Action: FileSystemRead, Path: "ok/*.go"},
			{Action: FileSystemWrite, Path: ""},
		},
		Executables: []ExecutablePermission{{}},
	}
	err := p.Validate()
	require.ErrorContains(t, err, "missing path", "the filesystem error must win over later sections")
	require.ErrorContains(t, p.Validate(), "filesystem")
}

func TestValidateGlobPathAcceptsDocumentedCharset(t *testing.T) {
	for _, path := range []string{"src", "src/main.go", "a/b/c", "*.go", "x-{a}.yaml", "$HOME/logs", "dir-1/file_2"} {
		require.NoError(t, validateGlobPath(path), "path %q must be accepted", path)
	}
}

func TestValidateGlobPathRejections(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{"empty", "", "glob cannot be empty"},
		{"dotdot", "a/../b", "cannot contain '..'"},
		{"unsupported char", "src main.go", "unsupported characters"},
		{"backslash", `a\b`, "unsupported characters"},
		{"control char", "a\nb", "unsupported characters"},
		// The charset admits '{' and '}' but not ',', so comma-separated brace
		// lists are rejected even though the vocabulary documents braces.
		{"brace list comma", "x-{a,b}.yaml", "unsupported characters"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateGlobPath(tc.path)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

// FuzzValidateGlobPath pins the glob-path safety contract for arbitrary
// manifest-supplied paths: whatever is accepted matches the documented
// charset and never contains '..'.
func FuzzValidateGlobPath(f *testing.F) {
	f.Add("src/*.go")
	f.Add("..")
	f.Add("../etc/passwd")
	f.Add("")
	f.Add("a/{b,c}.yaml")
	f.Add(`C:\Windows`)
	f.Add("workspace/$VAR/file")

	f.Fuzz(func(t *testing.T, path string) {
		err := validateGlobPath(path)
		if err == nil {
			require.NotContains(t, path, "..", "accepted path must not contain '..'")
			for _, r := range path {
				require.True(t,
					strings.ContainsRune(".*-{}$", r) || r == '/' || r == '_' ||
						(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'),
					"accepted path %q contains character %q outside the documented charset", path, r)
			}
		}
	})
}

// FuzzValidatePermissionSet asserts validation is total (never panics) over
// arbitrary permission declarations, mirroring untrusted manifest input.
func FuzzValidatePermissionSet(f *testing.F) {
	f.Add("fs:read", "src/*.go", "rg", "egress", "https", "example.com")
	f.Add("fs:write", "..", "/bin/sh", "ingress", "", "")
	f.Add("", "", "", "", "", "")

	f.Fuzz(func(t *testing.T, action, path, binary, direction, protocol, host string) {
		p := &PermissionSet{
			FileSystem:  []FileSystemPermission{{Action: FileSystemAction(action), Path: path}},
			Executables: []ExecutablePermission{{Binary: binary}},
			Network:     []NetworkPermission{{Direction: direction, Protocol: protocol, Host: host}},
		}
		err := p.Validate()
		if direction == "egress" && protocol != "" && host == "" &&
			path != "" && !strings.Contains(path, "..") && binary != "" && !strings.Contains(binary, "/") {
			require.Error(t, err, "egress without host must be rejected")
		}
	})
}
