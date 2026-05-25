package security

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/platform/contracts"
	"github.com/stretchr/testify/require"
)

// TestFileScopePolicy validates that filesystem scope policy correctly
// enforces workspace boundaries and protected path restrictions.
func TestFileScopePolicy(t *testing.T) {
	t.Run("policy canonicalizes workspace root", func(t *testing.T) {
		workspace := t.TempDir()
		policy := sandbox.NewFileScopePolicy(workspace, nil)

		if policy.Workspace != workspace {
			t.Errorf("workspace not canonicalized: got %s, want %s", policy.Workspace, workspace)
		}
	})

	t.Run("policy stores protected paths", func(t *testing.T) {
		workspace := t.TempDir()
		protectedPaths := []string{
			"/etc/passwd",
			"/etc/shadow",
			"/var/log/auth.log",
		}

		policy := sandbox.NewFileScopePolicy(workspace, protectedPaths)
		require.Contains(t, policy.ProtectedPaths, filepath.ToSlash(filepath.Join(workspace, "relurpify_cfg")))
		require.Contains(t, policy.ProtectedPaths, filepath.ToSlash(filepath.Join(workspace, ".git")))
		for _, path := range protectedPaths {
			require.Contains(t, policy.ProtectedPaths, path)
		}
	})

	t.Run("policy with no protected paths is valid", func(t *testing.T) {
		workspace := t.TempDir()
		policy := sandbox.NewFileScopePolicy(workspace, nil)

		require.Contains(t, policy.ProtectedPaths, filepath.ToSlash(filepath.Join(workspace, "relurpify_cfg")))
		require.Contains(t, policy.ProtectedPaths, filepath.ToSlash(filepath.Join(workspace, ".git")))
	})

	t.Run("policy handles relative workspace path", func(t *testing.T) {
		workspace := t.TempDir()
		relPath := filepath.Base(workspace)
		parentDir := filepath.Dir(workspace)

		// Change to parent directory and use relative path
		originalWd, _ := os.Getwd()
		_ = os.Chdir(parentDir)
		defer os.Chdir(originalWd)

		policy := sandbox.NewFileScopePolicy(relPath, nil)

		// Policy should canonicalize to absolute path
		if !filepath.IsAbs(policy.Workspace) {
			t.Error("workspace should be canonicalized to absolute path")
		}
	})
}

// TestFileScopeValidation validates that file scope policy correctly
// identifies paths that escape the workspace boundary.
func TestFileScopeValidation(t *testing.T) {
	t.Run("workspace file is within scope", func(t *testing.T) {
		workspace := t.TempDir()
		policy := sandbox.NewFileScopePolicy(workspace, nil)

		testFile := filepath.Join(workspace, "test.txt")
		err := policy.Check(contracts.FileSystemRead, testFile)

		if err != nil {
			t.Errorf("workspace file should be within scope: %v", err)
		}
	})

	t.Run("nested workspace file is within scope", func(t *testing.T) {
		workspace := t.TempDir()
		nestedDir := filepath.Join(workspace, "subdir", "nested")
		if err := os.MkdirAll(nestedDir, 0o755); err != nil {
			t.Fatalf("failed to create nested directory: %v", err)
		}

		policy := sandbox.NewFileScopePolicy(workspace, nil)

		testFile := filepath.Join(nestedDir, "test.txt")
		err := policy.Check(contracts.FileSystemRead, testFile)

		if err != nil {
			t.Errorf("nested workspace file should be within scope: %v", err)
		}
	})

	t.Run("path outside workspace is rejected", func(t *testing.T) {
		workspace := t.TempDir()
		policy := sandbox.NewFileScopePolicy(workspace, nil)

		outsidePath := "/etc/passwd"
		err := policy.Check(contracts.FileSystemRead, outsidePath)

		if err == nil {
			t.Error("path outside workspace should be rejected")
		}
		// Check if the error unwraps to ErrFileScopeOutsideWorkspace
		if !errors.Is(err, sandbox.ErrFileScopeOutsideWorkspace) {
			t.Errorf("expected error to unwrap to ErrFileScopeOutsideWorkspace, got %v", err)
		}
	})

	t.Run("path traversal is rejected", func(t *testing.T) {
		workspace := t.TempDir()
		policy := sandbox.NewFileScopePolicy(workspace, nil)

		traversalPath := filepath.Join(workspace, "..", "etc", "passwd")
		err := policy.Check(contracts.FileSystemRead, traversalPath)

		if err == nil {
			t.Error("path traversal should be rejected")
		}
	})

	t.Run("symlink to outside workspace is rejected", func(t *testing.T) {
		workspace := t.TempDir()
		policy := sandbox.NewFileScopePolicy(workspace, nil)

		// Create a symlink to outside the workspace
		outsideFile := filepath.Join(workspace, "outside_link")
		_ = os.Symlink("/etc/passwd", outsideFile)

		err := policy.Check(contracts.FileSystemRead, outsideFile)

		if err == nil {
			t.Error("symlink to outside workspace should be rejected")
		}
	})

	t.Run("absolute path outside workspace is rejected", func(t *testing.T) {
		workspace := t.TempDir()
		policy := sandbox.NewFileScopePolicy(workspace, nil)

		absOutsidePath := "/tmp/test.txt"
		err := policy.Check(contracts.FileSystemRead, absOutsidePath)

		if err == nil {
			t.Error("absolute path outside workspace should be rejected")
		}
	})
}

// TestProtectedPaths validates that protected path restrictions are
// enforced correctly as part of the file scope policy.
func TestProtectedPaths(t *testing.T) {
	t.Run("protected path within workspace is rejected", func(t *testing.T) {
		workspace := t.TempDir()
		// Create a protected directory within the workspace
		protectedDir := filepath.Join(workspace, "protected")
		if err := os.MkdirAll(protectedDir, 0o755); err != nil {
			t.Fatalf("failed to create protected directory: %v", err)
		}

		protectedPaths := []string{protectedDir}
		policy := sandbox.NewFileScopePolicy(workspace, protectedPaths)

		testFile := filepath.Join(protectedDir, "test.txt")
		err := policy.Check(contracts.FileSystemRead, testFile)

		if err == nil {
			t.Error("protected path should be rejected")
		}
		// Check if the error unwraps to ErrFileScopeProtectedPath
		if !errors.Is(err, sandbox.ErrFileScopeProtectedPath) {
			t.Errorf("expected error to unwrap to ErrFileScopeProtectedPath, got %v", err)
		}
	})

	t.Run("non-protected path is allowed", func(t *testing.T) {
		workspace := t.TempDir()
		// Create a protected directory within the workspace
		protectedDir := filepath.Join(workspace, "protected")
		if err := os.MkdirAll(protectedDir, 0o755); err != nil {
			t.Fatalf("failed to create protected directory: %v", err)
		}

		protectedPaths := []string{protectedDir}
		policy := sandbox.NewFileScopePolicy(workspace, protectedPaths)

		testFile := filepath.Join(workspace, "test.txt")
		err := policy.Check(contracts.FileSystemRead, testFile)

		if err != nil {
			t.Errorf("non-protected path should be allowed: %v", err)
		}
	})

	t.Run("workspace file matching protected pattern is allowed when path differs", func(t *testing.T) {
		workspace := t.TempDir()
		// Create a protected directory within the workspace
		protectedDir := filepath.Join(workspace, "protected")
		if err := os.MkdirAll(protectedDir, 0o755); err != nil {
			t.Fatalf("failed to create protected directory: %v", err)
		}

		// Create a file with same name outside protected directory
		localPasswd := filepath.Join(workspace, "passwd")
		if err := os.WriteFile(localPasswd, []byte("test"), 0o644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		protectedPaths := []string{protectedDir}
		policy := sandbox.NewFileScopePolicy(workspace, protectedPaths)

		err := policy.Check(contracts.FileSystemRead, localPasswd)

		if err != nil {
			t.Errorf("workspace file outside protected path should be allowed: %v", err)
		}
	})

	t.Run("multiple protected paths within workspace are enforced", func(t *testing.T) {
		workspace := t.TempDir()
		// Create protected directories within the workspace
		protectedDir1 := filepath.Join(workspace, "protected1")
		protectedDir2 := filepath.Join(workspace, "protected2")
		if err := os.MkdirAll(protectedDir1, 0o755); err != nil {
			t.Fatalf("failed to create protected directory 1: %v", err)
		}
		if err := os.MkdirAll(protectedDir2, 0o755); err != nil {
			t.Fatalf("failed to create protected directory 2: %v", err)
		}

		protectedPaths := []string{protectedDir1, protectedDir2}
		policy := sandbox.NewFileScopePolicy(workspace, protectedPaths)

		for _, path := range protectedPaths {
			testFile := filepath.Join(path, "test.txt")
			err := policy.Check(contracts.FileSystemRead, testFile)
			if err == nil {
				t.Errorf("protected path %s should be rejected", path)
			}
			// Check if the error unwraps to ErrFileScopeProtectedPath
			if !errors.Is(err, sandbox.ErrFileScopeProtectedPath) {
				t.Errorf("expected error to unwrap to ErrFileScopeProtectedPath for %s, got %v", path, err)
			}
		}
	})

	t.Run("empty protected paths list allows all workspace files", func(t *testing.T) {
		workspace := t.TempDir()
		policy := sandbox.NewFileScopePolicy(workspace, nil)

		testFile := filepath.Join(workspace, "test.txt")
		err := policy.Check(contracts.FileSystemRead, testFile)

		if err != nil {
			t.Errorf("workspace file should be allowed with no protected paths: %v", err)
		}
	})
}

// TestReadOnlyRoot validates that read-only root policy is correctly
// applied and visible in the sandbox policy state.
func TestReadOnlyRoot(t *testing.T) {
	t.Run("read-only root can be set in policy", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		policy := sandbox.SandboxPolicy{
			ReadOnlyRoot: true,
		}

		err := runtime.ApplyPolicy(nil, policy)
		if err != nil {
			t.Fatalf("failed to apply read-only root policy: %v", err)
		}

		retrieved := runtime.Policy()
		if !retrieved.ReadOnlyRoot {
			t.Error("read-only root should be set in policy state")
		}
	})

	t.Run("read-only root can be disabled in policy", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		policy := sandbox.SandboxPolicy{
			ReadOnlyRoot: false,
		}

		err := runtime.ApplyPolicy(nil, policy)
		if err != nil {
			t.Fatalf("failed to apply policy: %v", err)
		}

		retrieved := runtime.Policy()
		if retrieved.ReadOnlyRoot {
			t.Error("read-only root should be disabled when set to false")
		}
	})

	t.Run("read-only root is preserved across policy retrieval", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		policy := sandbox.SandboxPolicy{
			ReadOnlyRoot:    true,
			ProtectedPaths:  []string{"/etc/passwd"},
			NoNewPrivileges: true,
		}

		err := runtime.ApplyPolicy(nil, policy)
		if err != nil {
			t.Fatalf("failed to apply policy: %v", err)
		}

		// Retrieve multiple times to ensure stability
		for i := 0; i < 3; i++ {
			retrieved := runtime.Policy()
			if !retrieved.ReadOnlyRoot {
				t.Errorf("read-only root not preserved on retrieval %d", i)
			}
		}
	})
}

// TestNoNewPrivileges validates that no-new-privileges policy is correctly
// applied and visible in the sandbox policy state.
func TestNoNewPrivileges(t *testing.T) {
	t.Run("no-new-privileges can be set in policy", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		policy := sandbox.SandboxPolicy{
			NoNewPrivileges: true,
		}

		err := runtime.ApplyPolicy(nil, policy)
		if err != nil {
			t.Fatalf("failed to apply no-new-privileges policy: %v", err)
		}

		retrieved := runtime.Policy()
		if !retrieved.NoNewPrivileges {
			t.Error("no-new-privileges should be set in policy state")
		}
	})

	t.Run("no-new-privileges can be disabled in policy", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		policy := sandbox.SandboxPolicy{
			NoNewPrivileges: false,
		}

		err := runtime.ApplyPolicy(nil, policy)
		if err != nil {
			t.Fatalf("failed to apply policy: %v", err)
		}

		retrieved := runtime.Policy()
		if retrieved.NoNewPrivileges {
			t.Error("no-new-privileges should be disabled when set to false")
		}
	})

	t.Run("no-new-privileges is preserved across policy retrieval", func(t *testing.T) {
		config := sandbox.SandboxConfig{
			RunscPath:        "runsc",
			ContainerRuntime: "docker",
			Platform:         "kvm",
		}
		runtime := sandbox.NewSandboxRuntime(config)

		policy := sandbox.SandboxPolicy{
			NoNewPrivileges: true,
			ReadOnlyRoot:    true,
		}

		err := runtime.ApplyPolicy(nil, policy)
		if err != nil {
			t.Fatalf("failed to apply policy: %v", err)
		}

		// Retrieve multiple times to ensure stability
		for i := 0; i < 3; i++ {
			retrieved := runtime.Policy()
			if !retrieved.NoNewPrivileges {
				t.Errorf("no-new-privileges not preserved on retrieval %d", i)
			}
		}
	})
}
