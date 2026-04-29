package testsuite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestFileToolGranularPermissionEnforcement(t *testing.T) {
	base := t.TempDir()

	// Create permission set that requires HITL for everything
	perms := core.NewFileSystemPermissionSet(base, core.FileSystemRead)
	for i := range perms.FileSystem {
		perms.FileSystem[i].HITLRequired = true
	}

	manager, err := authorization.NewPermissionManager(base, perms, nil, nil) // No HITL provider -> Fail
	if err != nil {
		t.Fatalf("manager init failed: %v", err)
	}

	err = manager.CheckFileAccess(context.Background(), "test-agent", core.FileSystemRead, filepath.Join(base, "secret.txt"))
	if err == nil {
		t.Fatal("expected HITL error, got success")
	}
	if !strings.Contains(err.Error(), "hitl approval required") {
		t.Fatalf("expected hitl approval required error, got: %v", err)
	}
}

func TestWriteToolBackupPermissionEnforcement(t *testing.T) {
	base := t.TempDir()

	// Permission to write everything, BUT with HITL
	perms := core.NewFileSystemPermissionSet(base, core.FileSystemWrite)
	for i := range perms.FileSystem {
		perms.FileSystem[i].HITLRequired = true
	}

	manager, err := authorization.NewPermissionManager(base, perms, nil, nil) // No HITL provider -> Fail
	if err != nil {
		t.Fatalf("manager init failed: %v", err)
	}

	err = manager.CheckFileAccess(context.Background(), "test-agent", core.FileSystemWrite, filepath.Join(base, "manifest.json"))
	if err == nil {
		t.Fatal("expected HITL error, got success")
	}
	if !strings.Contains(err.Error(), "hitl approval required") {
		t.Fatalf("expected hitl approval required error, got: %v", err)
	}
}
