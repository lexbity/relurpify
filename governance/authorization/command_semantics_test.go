package authorization

import (
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/governance/permissions"
)

func TestUnwrapCommand(t *testing.T) {
	cases := []struct {
		name         string
		argv         []string
		wantInner    [][]string
		wantWrappers []string
		dynamic      bool
	}{
		{
			name:         "bash -lc unwraps shell string",
			argv:         []string{"bash", "-lc", "curl http://x"},
			wantInner:    [][]string{{"bash", "-lc", "curl http://x"}},
			wantWrappers: []string{"bash"},
		},
		{
			name:         "env skips assignments and flags",
			argv:         []string{"env", "A=1", "-i", "curl", "http://x"},
			wantInner:    [][]string{{"curl", "http://x"}},
			wantWrappers: []string{"env"},
		},
		{
			name:         "nohup",
			argv:         []string{"nohup", "wget", "http://x"},
			wantInner:    [][]string{{"wget", "http://x"}},
			wantWrappers: []string{"nohup"},
		},
		{
			name:         "nice -n",
			argv:         []string{"nice", "-n", "5", "make"},
			wantInner:    [][]string{{"make"}},
			wantWrappers: []string{"nice"},
		},
		{
			name:         "timeout duration",
			argv:         []string{"timeout", "10", "curl", "http://x"},
			wantInner:    [][]string{{"curl", "http://x"}},
			wantWrappers: []string{"timeout"},
		},
		{
			name:         "stdbuf attached",
			argv:         []string{"stdbuf", "-o0", "cat", "f"},
			wantInner:    [][]string{{"cat", "f"}},
			wantWrappers: []string{"stdbuf"},
		},
		{
			name:         "setsid",
			argv:         []string{"setsid", "-w", "make"},
			wantInner:    [][]string{{"make"}},
			wantWrappers: []string{"setsid"},
		},
		{
			name:         "nested env nohup",
			argv:         []string{"env", "nohup", "curl", "http://x"},
			wantInner:    [][]string{{"nohup", "curl", "http://x"}, {"curl", "http://x"}},
			wantWrappers: []string{"env", "nohup"},
		},
		{name: "opaque sudo", argv: []string{"sudo", "rm", "-rf", "/"}, dynamic: true},
		{name: "opaque xargs", argv: []string{"xargs", "rm"}, dynamic: true},
		{name: "opaque find -exec", argv: []string{"find", ".", "-exec", "rm", "{}", ";"}, dynamic: true},
		{name: "opaque awk system", argv: []string{"awk", `BEGIN{system("sh")}`}, dynamic: true},
		{name: "opaque perl -e", argv: []string{"perl", "-e", "print 1"}, dynamic: true},
		{name: "opaque python -c", argv: []string{"python3", "-c", "import os"}, dynamic: true},
		{name: "plain curl", argv: []string{"curl", "http://x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inner, steps, dynamic := UnwrapCommand(tc.argv)
			require.Equal(t, tc.dynamic, dynamic, "dynamic")
			if !tc.dynamic {
				require.Equal(t, tc.wantInner, inner, "inner")
			}
			var wrappers []string
			for _, step := range steps {
				wrappers = append(wrappers, step.Wrapper)
			}
			require.Equal(t, tc.wantWrappers, wrappers, "wrappers")
		})
	}
}

func TestLiftShellCommand_UnwrapsWrappers(t *testing.T) {
	cases := []struct {
		command string
		host    string
	}{
		{"env curl http://169.254.169.254/", "169.254.169.254"},
		{"nohup wget http://10.0.0.1/", "10.0.0.1"},
		{"nice -n 5 curl http://8.8.8.8/", "8.8.8.8"},
		{"timeout 10 curl http://1.1.1.1/", "1.1.1.1"},
		{"bash -lc 'curl http://9.9.9.9/'", "9.9.9.9"},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			res, err := LiftShellCommand(tc.command)
			require.NoError(t, err)
			require.False(t, res.HasDynamic)
			require.Len(t, res.Network, 1, "inner command network must be lifted")
			require.Equal(t, tc.host, res.Network[0].Host)
		})
	}
}

func TestLiftShellCommand_OutputCarriers(t *testing.T) {
	cases := []struct {
		command string
		path    string
	}{
		{"curl -o /etc/passwd https://8.8.8.8/", "/etc/passwd"},
		{"curl --output=/tmp/curl.out https://8.8.8.8/", "/tmp/curl.out"},
		{"wget -O out.bin https://8.8.8.8/", "out.bin"},
		{"wget --output-document=out.bin https://8.8.8.8/", "out.bin"},
		{"dd if=/dev/zero of=/tmp/dd.out bs=1M", "/tmp/dd.out"},
		{"install src.bin /usr/local/bin/dst", "/usr/local/bin/dst"},
		{"truncate -s 0 /tmp/trunc.out", "/tmp/trunc.out"},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			res, err := LiftShellCommand(tc.command)
			require.NoError(t, err)
			found := false
			for _, fsPerm := range res.FileSystem {
				if fsPerm.Action == permissions.FileSystemWrite && fsPerm.Path == tc.path {
					found = true
				}
			}
			require.True(t, found, "expected a lifted write to %q, got %+v", tc.path, res.FileSystem)
		})
	}
}

func TestLiftShellCommand_OutputCarrierDoesNotBecomeHost(t *testing.T) {
	res, err := LiftShellCommand("curl -o out.txt https://example.com/api")
	require.NoError(t, err)
	require.Len(t, res.Network, 1, "the URL host must be lifted, not the output filename")
	require.Equal(t, "example.com", res.Network[0].Host)

	found := false
	for _, fsPerm := range res.FileSystem {
		if fsPerm.Action == permissions.FileSystemWrite && fsPerm.Path == "out.txt" {
			found = true
		}
	}
	require.True(t, found, "the output write must be lifted as well")
}

func TestLiftShellCommand_OpaqueConstructors(t *testing.T) {
	for _, command := range []string{
		"xargs rm",
		"sudo rm -rf /",
		"su root",
		"watch ls",
		"find . -exec rm {} ;",
		`awk 'BEGIN{system("sh")}'`,
		"perl -e 'print 1'",
		"python3 -c 'import os'",
		"node -e 'process.exit(0)'",
	} {
		res, err := LiftShellCommand(command)
		require.NoError(t, err, command)
		require.True(t, res.HasDynamic, "expected %q to be dynamic", command)
	}
}

func TestLiftShellCommand_FileSystem(t *testing.T) {
	testCases := []struct {
		name           string
		command        string
		expectedAction permissions.FileSystemAction
		expectedPath   string
		expectedCount  int
		hasDynamic     bool
	}{
		{
			name:           "cat a single file",
			command:        "cat src/app.go",
			expectedAction: permissions.FileSystemRead,
			expectedPath:   "src/app.go",
			expectedCount:  1,
		},
		{
			name:           "cat a file with flags",
			command:        "cat -n -v src/app.go",
			expectedAction: permissions.FileSystemRead,
			expectedPath:   "src/app.go",
			expectedCount:  1,
		},
		{
			name:           "rm force recursive",
			command:        "rm -rf /tmp/build",
			expectedAction: permissions.FileSystemDelete,
			expectedPath:   "/tmp/build",
			expectedCount:  1,
		},
		{
			name:           "shred delete file",
			command:        "shred -u key.pem",
			expectedAction: permissions.FileSystemDelete,
			expectedPath:   "key.pem",
			expectedCount:  1,
		},
		{
			name:           "mkdir directories",
			command:        "mkdir -p src/utils",
			expectedAction: permissions.FileSystemWrite,
			expectedPath:   "src/utils",
			expectedCount:  1,
		},
		{
			name:           "touch create file",
			command:        "touch src/main.go",
			expectedAction: permissions.FileSystemWrite,
			expectedPath:   "src/main.go",
			expectedCount:  1,
		},
		{
			name:           "outward redirection",
			command:        "echo 'hello' > output.log",
			expectedAction: permissions.FileSystemWrite,
			expectedPath:   "output.log",
			expectedCount:  1,
		},
		{
			name:           "append redirection",
			command:        "echo 'world' >> output.log",
			expectedAction: permissions.FileSystemWrite,
			expectedPath:   "output.log",
			expectedCount:  1,
		},
		{
			name:           "inward redirection",
			command:        "cat < input.txt",
			expectedAction: permissions.FileSystemRead,
			expectedPath:   "input.txt",
			expectedCount:  1, // one for redirect (1), cat has 0 path args
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := LiftShellCommand(tc.command)
			if err != nil {
				t.Fatalf("unexpected error parsing command: %v", err)
			}
			if res.HasDynamic != tc.hasDynamic {
				t.Errorf("expected HasDynamic=%t, got %t", tc.hasDynamic, res.HasDynamic)
			}
			if len(res.FileSystem) != tc.expectedCount {
				t.Fatalf("expected FileSystem count %d, got %d. Result: %+v", tc.expectedCount, len(res.FileSystem), res.FileSystem)
			}
			// Find expected permission
			found := false
			for _, perm := range res.FileSystem {
				if perm.Action == tc.expectedAction && perm.Path == tc.expectedPath {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected to find permission Action=%s Path=%s, got %+v", tc.expectedAction, tc.expectedPath, res.FileSystem)
			}
		})
	}
}

func TestLiftShellCommand_CopyMove(t *testing.T) {
	res, err := LiftShellCommand("cp src/app.go build/app.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.FileSystem) != 2 {
		t.Fatalf("expected 2 filesystem operations, got %d", len(res.FileSystem))
	}
	// Check read of source
	if res.FileSystem[0].Action != permissions.FileSystemRead || res.FileSystem[0].Path != "src/app.go" {
		t.Errorf("expected source to be Read src/app.go, got %+v", res.FileSystem[0])
	}
	// Check write of destination
	if res.FileSystem[1].Action != permissions.FileSystemWrite || res.FileSystem[1].Path != "build/app.go" {
		t.Errorf("expected destination to be Write build/app.go, got %+v", res.FileSystem[1])
	}
}

func TestLiftShellCommand_Network(t *testing.T) {
	testCases := []struct {
		name         string
		command      string
		expectedHost string
	}{
		{
			name:         "curl simple URL",
			command:      "curl https://example.com/api",
			expectedHost: "example.com",
		},
		{
			name:         "wget URL with port",
			command:      "wget http://localhost:8080/file",
			expectedHost: "localhost",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := LiftShellCommand(tc.command)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(res.Network) != 1 {
				t.Fatalf("expected 1 network permission, got %d", len(res.Network))
			}
			if res.Network[0].Direction != "egress" || res.Network[0].Host != tc.expectedHost {
				t.Errorf("expected network host %q, got %+v", tc.expectedHost, res.Network[0])
			}
		})
	}
}

func TestLiftShellCommand_Dynamic(t *testing.T) {
	testCases := []struct {
		name    string
		command string
	}{
		{
			name:    "eval command",
			command: "eval $(something)",
		},
		{
			name:    "backticks execution",
			command: "echo `cat file.txt`",
		},
		{
			name:    "command substitution",
			command: "rm -rf $(find . -name '*.log')",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := LiftShellCommand(tc.command)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.HasDynamic {
				t.Errorf("expected command %q to be flagged as dynamic", tc.command)
			}
		})
	}
}
