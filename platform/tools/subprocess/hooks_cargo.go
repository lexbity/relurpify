package subprocess

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// applyCargoIsolation checks whether the tool invocation targets a nested Cargo
// workspace member and, if so, copies the working directory to an isolated temp
// location and injects --manifest-path. This prevents concurrent cargo runs
// inside the same workspace from interfering with each other.
//
// Returns the (possibly modified) command, workdir, cleanup function, and error.
// When no isolation is needed the returned values are unchanged and cleanup is a
// no-op.
func applyCargoIsolation(manifest contracts.ToolManifest, cmd []string, workdir string) ([]string, string, func(), error) {
	if !isCargoTool(manifest) {
		return cmd, workdir, func() {}, nil
	}
	return applyCargoIsolationCmd(cmd, workdir, manifest.SourcePath)
}

// applyCargoIsolationCmd takes the command, workdir, and sourcePath directly
// without requiring a full ToolManifest. This allows it to be called from
// the shared Run function and from go_native tools.
func applyCargoIsolationCmd(cmd []string, workdir string, sourcePath string) ([]string, string, func(), error) {
	noop := func() {}

	if workdir == "" || len(cmd) == 0 {
		return cmd, workdir, noop, nil
	}

	if !isCargoCmd(cmd) {
		return cmd, workdir, noop, nil
	}

	subcommand := cargoSubcommand(cmd)
	if subcommand == "" {
		return cmd, workdir, noop, nil
	}

	crateDir := workdir
	if !hasCargoToml(crateDir) {
		return cmd, workdir, noop, nil
	}

	basePath := filepath.Clean(sourcePath)
	if basePath == "" || basePath == "." {
		basePath = workdir
	}
	if !isNestedWorkspaceMember(crateDir, basePath) {
		return cmd, workdir, noop, nil
	}

	isolated, err := isolateCargoWorkdir(crateDir)
	if err != nil {
		return cmd, workdir, noop, fmt.Errorf("cargo isolation: %w", err)
	}
	cleanup := func() { os.RemoveAll(filepath.Dir(isolated)) }

	manifestPath := filepath.Join(isolated, "Cargo.toml")
	modified := injectManifestPath(cmd, subcommand, manifestPath)

	return modified, basePath, cleanup, nil
}

// isCargoCmd reports whether the first token of the command is "cargo".
func isCargoCmd(cmd []string) bool {
	return len(cmd) > 0 && cmd[0] == "cargo"
}

// isCargoTool returns true when the manifest describes a cargo subprocess tool.
func isCargoTool(manifest contracts.ToolManifest) bool {
	return manifest.Execution.Command != nil &&
		len(manifest.Execution.Command.Base) > 0 &&
		manifest.Execution.Command.Base[0] == "cargo"
}

// cargoSubcommand extracts the cargo subcommand from the full argv, returning
// empty when the subcommand is not one that triggers isolation.
func cargoSubcommand(cmd []string) string {
	// cmd[0] is "cargo" (the base command); subcommand is cmd[1]
	for i, token := range cmd {
		if i == 0 {
			continue
		}
		if strings.HasPrefix(token, "-") {
			continue // skip flags before subcommand (rare but possible)
		}
		sub := strings.ToLower(strings.TrimSpace(token))
		switch sub {
		case "test", "build", "check", "clippy", "metadata":
			return sub
		default:
			return ""
		}
	}
	return ""
}

func hasCargoToml(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "Cargo.toml"))
	return err == nil && !info.IsDir()
}

// isNestedWorkspaceMember checks whether dir is a subdirectory of a parent
// that also contains a Cargo.toml (indicating a workspace member). basePath
// is the workspace root where traversal stops.
func isNestedWorkspaceMember(dir, basePath string) bool {
	base := filepath.Clean(basePath)
	current := filepath.Dir(filepath.Clean(dir))
	for {
		if current == dir || current == "." || current == string(os.PathSeparator) {
			return false
		}
		if _, err := os.Stat(filepath.Join(current, "Cargo.toml")); err == nil {
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
		os.RemoveAll(tempRoot)
		return "", err
	}
	return target, nil
}

// injectManifestPath adds --manifest-path to the command after the subcommand.
func injectManifestPath(cmd []string, subcommand, manifestPath string) []string {
	for _, token := range cmd {
		if token == "--manifest-path" {
			return cmd // already present
		}
	}
	// Find the subcommand position and insert after it
	out := make([]string, 0, len(cmd)+2)
	found := false
	for _, token := range cmd {
		out = append(out, token)
		if !found && strings.ToLower(strings.TrimSpace(token)) == subcommand {
			out = append(out, "--manifest-path", manifestPath)
			found = true
		}
	}
	return out
}

// copyDir recursively copies src to dst, skipping .git, target, and .bak files.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "target" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if strings.HasSuffix(info.Name(), ".bak") {
			return nil
		}
		return copyFile(path, filepath.Join(dst, rel), info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
