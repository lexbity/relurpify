package execute

import (
	"os"
	"path/filepath"
	"strings"
)

// CargoIsolationHook is called by the executor to optionally isolate Cargo
// workspace directories. The default implementation applies the Cargo
// isolation heuristic; tests or alternate runtimes can replace it.
//
// The hook receives basePath (workspace root), workdir, and finalArgs.
// It returns (workdir, args, cleanup, error). Returning workdir unchanged
// with a no-op cleanup means no isolation was applied.
var CargoIsolationHook = defaultCargoIsolationHook

func defaultCargoIsolationHook(basePath, workdir string, args []string) (string, []string, func(), error) {
	if !shouldIsolateCargoRun(workdir, args) {
		return workdir, args, func() {}, nil
	}
	isolated, err := isolateCargoWorkdir(workdir)
	if err != nil {
		return workdir, args, func() {}, err
	}
	manifestPath := filepath.Join(isolated, "Cargo.toml")
	return basePath, withManifestPath(args, manifestPath), func() { _ = os.RemoveAll(filepath.Dir(isolated)) }, nil
}

// prepareArgsForWorkingDir is the Cargo-specific arg preparation that
// injects --manifest-path when running cargo commands from a subdirectory
// that belongs to a Cargo workspace.
func prepareArgsForWorkingDir(args []string, workdir string) []string {
	if workdir == "" {
		return args
	}
	for _, a := range args {
		if a == "--manifest-path" {
			return args
		}
	}
	manifestPath := filepath.Join(workdir, "Cargo.toml")
	if _, err := os.Stat(manifestPath); err != nil {
		return args
	}
	if len(args) == 0 {
		return []string{"--manifest-path", manifestPath}
	}
	prepared := make([]string, 0, len(args)+2)
	if !strings.HasPrefix(args[0], "-") {
		prepared = append(prepared, args[0], "--manifest-path", manifestPath)
		prepared = append(prepared, args[1:]...)
		return prepared
	}
	prepared = append(prepared, "--manifest-path", manifestPath)
	prepared = append(prepared, args...)
	return prepared
}

func shouldIsolateCargoRun(workdir string, args []string) bool {
	if workdir == "" || len(args) == 0 {
		return false
	}
	subcommand := strings.ToLower(strings.TrimSpace(args[0]))
	switch subcommand {
	case "test", "build", "check", "clippy", "metadata":
	default:
		return false
	}
	manifestPath := filepath.Join(workdir, "Cargo.toml")
	if _, err := os.Stat(manifestPath); err != nil {
		return false
	}
	// Find parent Cargo manifest to detect nested workspace membership.
	// When a subdirectory has its own Cargo.toml but also belongs to a
	// parent workspace, the run should be isolated to prevent interference.
	base := filepath.Clean(workdir)
	current := filepath.Dir(filepath.Clean(workdir))
	for {
		if current == workdir || current == "." || current == string(filepath.Separator) {
			return false
		}
		parentManifest := filepath.Join(current, "Cargo.toml")
		if _, err := os.Stat(parentManifest); err == nil {
			return true
		}
		if current == base {
			return false
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
		current = parent
	}
}

func isolateCargoWorkdir(workdir string) (string, error) {
	tempRoot, err := os.MkdirTemp("", "relurpify-cargo-*")
	if err != nil {
		return "", err
	}
	target := filepath.Join(tempRoot, filepath.Base(workdir))
	if err := copyDir(workdir, target); err != nil {
		_ = os.RemoveAll(tempRoot)
		return "", err
	}
	return target, nil
}

func withManifestPath(args []string, manifestPath string) []string {
	for i := 0; i < len(args); i++ {
		if args[i] == "--manifest-path" {
			return args
		}
	}
	if len(args) == 0 {
		return []string{"--manifest-path", manifestPath}
	}
	prepared := make([]string, 0, len(args)+2)
	if !strings.HasPrefix(args[0], "-") {
		prepared = append(prepared, args[0], "--manifest-path", manifestPath)
		prepared = append(prepared, args[1:]...)
		return prepared
	}
	prepared = append(prepared, "--manifest-path", manifestPath)
	prepared = append(prepared, args...)
	return prepared
}
