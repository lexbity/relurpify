package authorization

import (
	"testing"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

func TestLiftShellCommand_FileSystem(t *testing.T) {
	testCases := []struct {
		name           string
		command        string
		expectedAction contracts.FileSystemAction
		expectedPath   string
		expectedCount  int
		hasDynamic     bool
	}{
		{
			name:           "cat a single file",
			command:        "cat src/app.go",
			expectedAction: contracts.FileSystemRead,
			expectedPath:   "src/app.go",
			expectedCount:  1,
		},
		{
			name:           "cat a file with flags",
			command:        "cat -n -v src/app.go",
			expectedAction: contracts.FileSystemRead,
			expectedPath:   "src/app.go",
			expectedCount:  1,
		},
		{
			name:           "rm force recursive",
			command:        "rm -rf /tmp/build",
			expectedAction: contracts.FileSystemDelete,
			expectedPath:   "/tmp/build",
			expectedCount:  1,
		},
		{
			name:           "shred delete file",
			command:        "shred -u key.pem",
			expectedAction: contracts.FileSystemDelete,
			expectedPath:   "key.pem",
			expectedCount:  1,
		},
		{
			name:           "mkdir directories",
			command:        "mkdir -p src/utils",
			expectedAction: contracts.FileSystemWrite,
			expectedPath:   "src/utils",
			expectedCount:  1,
		},
		{
			name:           "touch create file",
			command:        "touch src/main.go",
			expectedAction: contracts.FileSystemWrite,
			expectedPath:   "src/main.go",
			expectedCount:  1,
		},
		{
			name:           "outward redirection",
			command:        "echo 'hello' > output.log",
			expectedAction: contracts.FileSystemWrite,
			expectedPath:   "output.log",
			expectedCount:  1,
		},
		{
			name:           "append redirection",
			command:        "echo 'world' >> output.log",
			expectedAction: contracts.FileSystemWrite,
			expectedPath:   "output.log",
			expectedCount:  1,
		},
		{
			name:           "inward redirection",
			command:        "cat < input.txt",
			expectedAction: contracts.FileSystemRead,
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
	if res.FileSystem[0].Action != contracts.FileSystemRead || res.FileSystem[0].Path != "src/app.go" {
		t.Errorf("expected source to be Read src/app.go, got %+v", res.FileSystem[0])
	}
	// Check write of destination
	if res.FileSystem[1].Action != contracts.FileSystemWrite || res.FileSystem[1].Path != "build/app.go" {
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
