package permissions

import "context"

// PermissionManager defines the governance-owned permission surface consumed
// by execution and workspace session code. Governance owns policy/security
// meaning; this interface defines the narrow contract that execution consumes
// without defining policy semantics itself.
//
// The capability-declared PermissionManagerHandle is composed at the app layer
// and accessed via type assertion where needed (e.g., registry.UsePermissionManager).
type PermissionManager interface {
	CheckFileAccess(context.Context, string, FileSystemAction, string) error
	SetEventLogger(func(context.Context, PermissionDescriptor, string, string, map[string]interface{}))
	DefaultPolicy() string
}
